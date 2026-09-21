package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"blog/server/internal/model"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	portalListKeyPrefix   = "portal:articles:list:" // + {page}:{tag}
	portalArticleKeyPfx   = "portal:article:"       // + {slug}
	portalListKeyScanSize = 100
	portalCacheTTL        = 60 * time.Second // 前台缓存 TTL 兜底
	portalTagAll          = "all"            // 列表缓存 key 中「不筛选标签」的占位

	likeDedupKeyPfx = "portal:like:"      // + {slug}:{ip}
	likeDedupTTL    = 30 * 24 * time.Hour // 同一 IP 对同一文章的点赞去重窗口
)

// PortalService 前台只读查询：已发布文章列表/详情，结果走 Redis 缓存。
type PortalService struct {
	db  *gorm.DB
	rdb *redis.Client
	log *zap.Logger
}

// NewPortalService 创建门户服务。
func NewPortalService(db *gorm.DB, rdb *redis.Client, log *zap.Logger) *PortalService {
	return &PortalService{db: db, rdb: rdb, log: log}
}

// PortalListKey 前台文章列表缓存 key：portal:articles:list:{page}:{tag}。
func PortalListKey(page int, tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = portalTagAll
	}
	return fmt.Sprintf("%s%d:%s", portalListKeyPrefix, page, tag)
}

// PortalArticleKey 前台文章详情缓存 key：portal:article:{slug}。
func PortalArticleKey(slug string) string { return portalArticleKeyPfx + slug }

// PortalFigure 前台配图元数据。
type PortalFigure struct {
	ID          uint   `json:"id"`
	Placeholder string `json:"placeholder"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
}

// PortalCover 前台封面元数据。
type PortalCover struct {
	ID    uint   `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// PortalArticleDetail 前台文章详情：正文 + 封面与插图元数据。
type PortalArticleDetail struct {
	model.Article
	Cover   *PortalCover   `json:"cover"`
	Figures []PortalFigure `json:"figures"`
}

// ListArticles 已发布文章分页列表，可按标签筛选，结果缓存 60s。
func (s *PortalService) ListArticles(ctx context.Context, page, pageSize int, tag string) (*PageData, error) {
	page, pageSize = NormalizePage(page, pageSize)
	tag = strings.TrimSpace(tag)
	key := PortalListKey(page, tag)

	if data, ok := s.cacheGet(ctx, key); ok {
		var pd PageData
		if err := json.Unmarshal(data, &pd); err == nil {
			return &pd, nil
		}
	}

	q := s.db.WithContext(ctx).Model(&model.Article{}).
		Where("status = ?", model.ArticleStatusPublished)
	if tag != "" {
		// tags 为 JSON 数组，用 JSON_CONTAINS 精确匹配标签
		q = q.Where("JSON_CONTAINS(tags, JSON_QUOTE(?))", tag)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("统计已发布文章数: %w", err)
	}
	items := make([]model.Article, 0)
	err := q.Select(articleListColumns).Order("published_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("查询已发布文章列表: %w", err)
	}
	pd := &PageData{Items: items, Total: total, Page: page, PageSize: pageSize}
	s.cacheSet(ctx, key, pd)
	return pd, nil
}

// GetArticle 已发布文章详情（含封面与插图元数据），结果缓存 60s。
func (s *PortalService) GetArticle(ctx context.Context, slug string) (*PortalArticleDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug 不能为空", ErrInvalid)
	}
	key := PortalArticleKey(slug)
	if data, ok := s.cacheGet(ctx, key); ok {
		var detail PortalArticleDetail
		if err := json.Unmarshal(data, &detail); err == nil {
			return &detail, nil
		}
	}

	var art model.Article
	err := s.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, model.ArticleStatusPublished).
		First(&art).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章不存在或未发布", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %s: %w", slug, err)
	}

	detail := &PortalArticleDetail{Article: art, Figures: make([]PortalFigure, 0)}

	var cover model.SvgAsset
	err = s.db.WithContext(ctx).
		Where("article_id = ? AND kind = ?", art.ID, model.FigureKindCover).
		First(&cover).Error
	switch {
	case err == nil:
		detail.Cover = &PortalCover{ID: cover.ID, Kind: cover.Kind, Title: cover.Title}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 无封面时保持 nil
	default:
		return nil, fmt.Errorf("查询文章 %s 封面: %w", slug, err)
	}

	err = s.db.WithContext(ctx).Model(&model.SvgAsset{}).
		Select("id, placeholder, kind, title").
		Where("article_id = ? AND kind <> ?", art.ID, model.FigureKindCover).
		Order("id ASC").Find(&detail.Figures).Error
	if err != nil {
		return nil, fmt.Errorf("查询文章 %s 配图: %w", slug, err)
	}

	s.cacheSet(ctx, key, detail)
	return detail, nil
}

// GetFigure 按 id 查询 SVG 素材原文（供 /portal/figures/:file 输出）。
func (s *PortalService) GetFigure(ctx context.Context, id uint) (*model.SvgAsset, error) {
	var asset model.SvgAsset
	err := s.db.WithContext(ctx).First(&asset, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 配图 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询配图 %d: %w", id, err)
	}
	return &asset, nil
}

// PortalLikeResult 点赞结果：最新计数 + 是否为重复点赞。
type PortalLikeResult struct {
	LikeCount  int  `json:"like_count"`
	Duplicated bool `json:"duplicated"`
}

// LikeArticle 前台点赞：以 slug+客户端 IP 在 Redis 去重（30 天窗口），
// 原子自增 like_count 并失效前台缓存。重复点赞跳过自增、直接返回当前计数；
// Redis 不可用或去重检查失败时放行（沿用「依赖失败不阻塞主流程」的降级基调）。
func (s *PortalService) LikeArticle(ctx context.Context, slug, ip string) (*PortalLikeResult, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug 不能为空", ErrInvalid)
	}
	var art model.Article
	err := s.db.WithContext(ctx).
		Where("slug = ? AND status = ?", slug, model.ArticleStatusPublished).
		First(&art).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章不存在或未发布", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %s: %w", slug, err)
	}

	result := &PortalLikeResult{}
	if s.rdb != nil {
		cctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
		defer cancel()
		ok, err := s.rdb.SetNX(cctx, likeDedupKeyPfx+slug+":"+ip, 1, likeDedupTTL).Result()
		if err != nil {
			s.log.Warn("点赞去重检查失败，本次放行", zap.String("slug", slug), zap.Error(err))
		} else if !ok {
			result.Duplicated = true
		}
	}
	if !result.Duplicated {
		if err := s.db.WithContext(ctx).Model(&model.Article{}).
			Where("id = ?", art.ID).
			UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error; err != nil {
			return nil, fmt.Errorf("文章 %s 点赞自增: %w", slug, err)
		}
		art.LikeCount++
	}
	result.LikeCount = art.LikeCount
	s.InvalidateCache(ctx, slug)
	s.log.Info("文章点赞", zap.Uint("article_id", art.ID), zap.String("slug", slug),
		zap.String("ip", ip), zap.Bool("duplicated", result.Duplicated))
	return result, nil
}

// InvalidateCache 失效前台缓存：指定文章的详情 key + 全部列表 key。
// 删除失败仅告警，由 TTL 兜底，不阻塞调用方。
func (s *PortalService) InvalidateCache(ctx context.Context, slugs ...string) {
	if s.rdb == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()

	keys := make([]string, 0, len(slugs))
	for _, slug := range slugs {
		if slug = strings.TrimSpace(slug); slug != "" {
			keys = append(keys, PortalArticleKey(slug))
		}
	}
	var cursor uint64
	for {
		batch, next, err := s.rdb.Scan(cctx, cursor, portalListKeyPrefix+"*", portalListKeyScanSize).Result()
		if err != nil {
			s.log.Warn("扫描前台列表缓存失败", zap.Error(err))
			break
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(keys) == 0 {
		return
	}
	if err := s.rdb.Del(cctx, keys...).Err(); err != nil {
		s.log.Warn("删除前台缓存失败（等待 TTL 兜底）",
			zap.Strings("keys", keys), zap.Error(err))
	}
}

// cacheGet 读取缓存，未命中或出错返回 false。
func (s *PortalService) cacheGet(ctx context.Context, key string) ([]byte, bool) {
	if s.rdb == nil {
		return nil, false
	}
	cctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()
	data, err := s.rdb.Get(cctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return data, true
}

// cacheSet 写入缓存并设置 TTL，失败仅告警。
func (s *PortalService) cacheSet(ctx context.Context, key string, v any) {
	if s.rdb == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := s.rdb.Set(cctx, key, data, portalCacheTTL).Err(); err != nil {
		s.log.Warn("写入前台缓存失败", zap.String("key", key), zap.Error(err))
	}
}
