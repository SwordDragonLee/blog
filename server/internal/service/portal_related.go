package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"blog/server/internal/model"

	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 本文件集中相关文章推荐逻辑（GET /portal/articles/:slug/related）：
// 语义向量为主（RagService 复用发布时入库的块向量），同仓库 / 最新发布规则兜底；
// 候选一律回库校验 published，防止推荐出已下线文章；结果缓存 10 分钟。
// 方法与 portal.go 同属 PortalService，按功能拆文件存放。

// relatedColumns 相关推荐查询字段（不含大字段 content_md）。
const relatedColumns = "id, title, slug, summary, tags, status, published_at"

// PortalRelatedArticle 相关文章推荐条目：回库校验后补齐的展示字段。
type PortalRelatedArticle struct {
	Title       string         `json:"title"`
	Slug        string         `json:"slug"`
	Summary     string         `json:"summary"`
	Tags        datatypes.JSON `json:"tags"`
	PublishedAt *time.Time     `json:"published_at"`
}

// PortalRelatedKey 相关文章推荐缓存 key：portal:related:{slug}。
func PortalRelatedKey(slug string) string { return portalRelatedKeyPfx + slug }

// GetRelated 文章相关推荐。
// 流程：Redis 缓存 → 语义向量候选（RagService，不可用则跳过）→ 回库校验 published
// 并补展示字段 → 不足时「同仓库文章 → 最新发布」规则兜底补齐 → 写缓存返回。
// 语义链路任何故障都在 RagService 内部吞掉降级，本接口永不因推荐而失败。
func (s *PortalService) GetRelated(ctx context.Context, slug string, limit int) ([]PortalRelatedArticle, error) {
	limit = clampRelatedLimit(limit)
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug 不能为空", ErrInvalid)
	}
	key := PortalRelatedKey(slug)
	if data, ok := s.cacheGet(ctx, key); ok {
		var items []PortalRelatedArticle
		if err := json.Unmarshal(data, &items); err == nil {
			return items, nil
		}
	}

	// 本文必须已发布：推荐挂在详情页下，文章不在直接 404
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

	included := map[string]bool{slug: true}
	items := make([]PortalRelatedArticle, 0, limit)

	// 1. 语义候选：slug 按相似度降序，回库校验后保持原顺序
	var ordered []string
	if s.related != nil {
		for _, c := range s.related.RelatedArticles(ctx, art.ID, slug, limit) {
			if !included[c.Slug] {
				ordered = append(ordered, c.Slug)
			}
		}
	}

	if len(ordered) > 0 {
		var arts []model.Article
		if err := s.db.WithContext(ctx).Model(&model.Article{}).
			Select(relatedColumns).
			Where("status = ? AND slug IN ?", model.ArticleStatusPublished, ordered).
			Find(&arts).Error; err != nil {
			return nil, fmt.Errorf("查询相关文章候选: %w", err)
		}
		bySlug := make(map[string]model.Article, len(arts))
		for _, a := range arts {
			bySlug[a.Slug] = a
		}
		for _, sl := range ordered {
			if a, ok := bySlug[sl]; ok && !included[sl] {
				items = append(items, toRelatedArticle(a))
				included[sl] = true
			}
		}
	}

	// 2. 兜底 1：同仓库（同一次分析生成的）其他已发布文章，互为强相关
	if len(items) < limit {
		var siblings []model.Article
		if err := s.db.WithContext(ctx).Model(&model.Article{}).
			Select(relatedColumns).
			Where("repo_analysis_id = ? AND status = ?", art.RepoAnalysisID, model.ArticleStatusPublished).
			Order("published_at DESC, id DESC").Limit(limit).
			Find(&siblings).Error; err != nil {
			return nil, fmt.Errorf("查询同仓库文章: %w", err)
		}
		for _, a := range siblings {
			if !included[a.Slug] && len(items) < limit {
				items = append(items, toRelatedArticle(a))
				included[a.Slug] = true
			}
		}
	}

	// 3. 兜底 2：最新发布补位，保证栏目尽量不空
	if len(items) < limit {
		var latest []model.Article
		if err := s.db.WithContext(ctx).Model(&model.Article{}).
			Select(relatedColumns).
			Where("status = ?", model.ArticleStatusPublished).
			Order("published_at DESC, id DESC").Limit(limit + len(items)).
			Find(&latest).Error; err != nil {
			return nil, fmt.Errorf("查询最新文章: %w", err)
		}
		for _, a := range latest {
			if !included[a.Slug] && len(items) < limit {
				items = append(items, toRelatedArticle(a))
				included[a.Slug] = true
			}
		}
	}

	s.cacheSetTTL(ctx, key, items, portalRelatedTTL)
	return items, nil
}

// toRelatedArticle 模型转推荐条目。
func toRelatedArticle(a model.Article) PortalRelatedArticle {
	return PortalRelatedArticle{
		Title:       a.Title,
		Slug:        a.Slug,
		Summary:     a.Summary,
		Tags:        a.Tags,
		PublishedAt: a.PublishedAt,
	}
}

// clampRelatedLimit 收敛 limit：缺省 4，上限 10。
func clampRelatedLimit(limit int) int {
	if limit <= 0 {
		return relatedDefaultLimit
	}
	if limit > relatedMaxLimit {
		return relatedMaxLimit
	}
	return limit
}

// cacheSetTTL 写入缓存并指定 TTL，失败仅告警（与 portal.go 的 cacheSet 同源，仅 TTL 可变）。
func (s *PortalService) cacheSetTTL(ctx context.Context, key string, v any, ttl time.Duration) {
	if s.rdb == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, redisReadTimeout)
	defer cancel()
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := s.rdb.Set(cctx, key, data, ttl).Err(); err != nil {
		s.log.Warn("写入前台缓存失败", zap.String("key", key), zap.Error(err))
	}
}
