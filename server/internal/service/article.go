package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"blog/server/internal/llm"
	"blog/server/internal/mailer"
	"blog/server/internal/model"
	"blog/server/internal/svggen"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// articleListColumns 列表查询字段（排除大字段 content_md）。
const articleListColumns = "id, repo_analysis_id, title, slug, summary, word_count, like_count, tags, " +
	"status, sort_order, cover_asset_id, published_at, created_at, updated_at"

// ArticleService 文章的查询、编辑、发版/下线与配图重生成。
type ArticleService struct {
	db     *gorm.DB
	rdb    *redis.Client
	llm    *llm.Client
	log    *zap.Logger
	portal *PortalService // 复用缓存失效逻辑
	mailer *mailer.Mailer // 发布成功后的邮件通知
	rag    *RagService    // RAG 技术问答：发布索引 / 下线删除（可为 nil）
}

// NewArticleService 创建文章服务。
func NewArticleService(db *gorm.DB, rdb *redis.Client, client *llm.Client,
	notify *mailer.Mailer, rag *RagService, log *zap.Logger) *ArticleService {
	return &ArticleService{db: db, rdb: rdb, llm: client, log: log,
		portal: NewPortalService(db, rdb, log), mailer: notify, rag: rag}
}

// FigureMeta 文章配图元数据（不含 SVG 原文）。
type FigureMeta struct {
	ID          uint      `json:"id"`
	ArticleID   uint      `json:"article_id"`
	Placeholder string    `json:"placeholder"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	CreatedAt   time.Time `json:"created_at"`
}

// ArticleDetail 文章详情（含配图元数据数组）。
type ArticleDetail struct {
	model.Article
	Figures []FigureMeta `json:"figures"`
}

// List 文章列表，status 为空时返回全部。
func (s *ArticleService) List(ctx context.Context, status string, page, pageSize int) (*PageData, error) {
	page, pageSize = NormalizePage(page, pageSize)
	q := s.db.WithContext(ctx).Model(&model.Article{})
	if status = strings.TrimSpace(status); status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("统计文章数: %w", err)
	}
	items := make([]model.Article, 0)
	err := q.Select(articleListColumns).Order("id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("查询文章列表: %w", err)
	}
	return &PageData{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// Get 文章详情，含全部配图（含封面）元数据。
func (s *ArticleService) Get(ctx context.Context, id uint) (*ArticleDetail, error) {
	var art model.Article
	err := s.db.WithContext(ctx).First(&art, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %d: %w", id, err)
	}
	figs, err := s.figures(ctx, art.ID)
	if err != nil {
		return nil, err
	}
	return &ArticleDetail{Article: art, Figures: figs}, nil
}

// Update 编辑文章（标题/摘要/正文/标签），重算字数；
// 已发布文章保存后回到 draft，需重新发版，并失效前台缓存。
func (s *ArticleService) Update(ctx context.Context, id uint, title, summary, contentMD string, tags []string) (*model.Article, error) {
	title = truncateRunes(strings.TrimSpace(title), 256)
	if title == "" {
		return nil, fmt.Errorf("%w: 标题不能为空", ErrInvalid)
	}
	var art model.Article
	err := s.db.WithContext(ctx).First(&art, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %d: %w", id, err)
	}

	wasPublished := art.Status == model.ArticleStatusPublished
	art.Title = title
	art.Summary = truncateRunes(summary, 512)
	art.ContentMD = contentMD
	art.Tags = jsonifyTags(tags)
	art.WordCount = CountWords(contentMD)
	if wasPublished {
		art.Status = model.ArticleStatusDraft
	}
	if err := s.db.WithContext(ctx).Save(&art).Error; err != nil {
		return nil, fmt.Errorf("保存文章 %d: %w", id, err)
	}
	// 原先对外可见（已发布）时才需失效前台缓存
	if wasPublished {
		s.portal.InvalidateCache(ctx, art.Slug)
		// 已发布文章被编辑退回草稿：内容将改变，向量即刻作废
		if s.rag != nil {
			s.rag.RemoveArticle(ctx, art.ID)
		}
	}
	s.log.Info("文章已编辑", zap.Uint("article_id", id), zap.Bool("was_published", wasPublished))
	return &art, nil
}

// Publish 发版：draft → published，写 published_at 并失效前台缓存；
// 成功后异步邮件通知发布者（通知失败不影响发布结果）。
func (s *ArticleService) Publish(ctx context.Context, id, publisherID uint) (*model.Article, error) {
	var art model.Article
	err := s.db.WithContext(ctx).First(&art, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %d: %w", id, err)
	}
	if art.Status != model.ArticleStatusDraft {
		return nil, fmt.Errorf("%w: 仅待审核文章可发版，当前状态为 %s", ErrConflict, art.Status)
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&art).Updates(map[string]any{
		"status":       model.ArticleStatusPublished,
		"published_at": now,
	}).Error; err != nil {
		return nil, fmt.Errorf("发布文章 %d: %w", id, err)
	}
	s.portal.InvalidateCache(ctx, art.Slug)
	// 异步邮件通知发布者（mailer 内部处理长文章生成时的重复通知）...
	if s.mailer != nil {
		s.mailer.NotifyArticlePublished(publisherID, art.Title, art.Slug, now)
	}
	// 异步向量化入库，供 RAG 技术问答检索（失败仅记日志，不影响发布结果）
	if s.rag != nil {
		s.indexForRAG(ctx, art)
	}
	s.log.Info("文章已发版", zap.Uint("article_id", id), zap.String("slug", art.Slug))
	art.Status = model.ArticleStatusPublished
	art.PublishedAt = &now
	return &art, nil
}

// indexForRAG 异步向量化入库：与请求上下文解耦（HTTP 返回后仍继续），
// 限时 60s；失败仅记日志，不影响发布结果。传值拷贝避免与调用方后续字段赋值竞争。
func (s *ArticleService) indexForRAG(ctx context.Context, art model.Article) {
	go func() {
		ictx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 60*time.Second)
		defer cancel()
		if err := s.rag.IndexArticle(ictx, &art); err != nil {
			s.log.Warn("文章向量化入库失败（问答暂不可检索该文）",
				zap.Uint("article_id", art.ID), zap.String("slug", art.Slug), zap.Error(err))
		}
	}()
}

// Offline 下线：published → draft，失效前台缓存。
func (s *ArticleService) Offline(ctx context.Context, id uint) (*model.Article, error) {
	var art model.Article
	err := s.db.WithContext(ctx).First(&art, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %d: %w", id, err)
	}
	if art.Status != model.ArticleStatusPublished {
		return nil, fmt.Errorf("%w: 仅已发布文章可下线，当前状态为 %s", ErrConflict, art.Status)
	}
	if err := s.db.WithContext(ctx).Model(&art).Update("status", model.ArticleStatusDraft).Error; err != nil {
		return nil, fmt.Errorf("下线文章 %d: %w", id, err)
	}
	s.portal.InvalidateCache(ctx, art.Slug)
	// 下线即对外不可见：同步删除 RAG 向量，问答不再引用
	if s.rag != nil {
		s.rag.RemoveArticle(ctx, art.ID)
	}
	s.log.Info("文章已下线", zap.Uint("article_id", id), zap.String("slug", art.Slug))
	art.Status = model.ArticleStatusDraft
	return &art, nil
}

// RegenerateFigures 重新生成文章配图：读取现有非 cover 配图元数据，
// 调 LLM 重新设计图表数据，逐张重渲染并更新 spec/svg_content；
// LLM 未返回的旧图保持不变。
func (s *ArticleService) RegenerateFigures(ctx context.Context, id uint) (*ArticleDetail, error) {
	var art model.Article
	err := s.db.WithContext(ctx).First(&art, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: 文章 %d 不存在", ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("查询文章 %d: %w", id, err)
	}
	var assets []model.SvgAsset
	if err := s.db.WithContext(ctx).
		Where("article_id = ? AND kind <> ?", art.ID, model.FigureKindCover).
		Order("id ASC").Find(&assets).Error; err != nil {
		return nil, fmt.Errorf("查询文章 %d 配图: %w", id, err)
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("%w: 文章《%s》暂无配图，无法重新生成", ErrInvalid, art.Title)
	}

	byPlaceholder := make(map[string]*model.SvgAsset, len(assets))
	meta := make([]string, 0, len(assets))
	for i := range assets {
		a := &assets[i]
		byPlaceholder[a.Placeholder] = a
		meta = append(meta, fmt.Sprintf("- id: %s\n  kind: %s\n  title: %s", a.Placeholder, a.Kind, a.Title))
	}

	system, user := llm.BuildFigureRegenPrompt(art.Title, art.ContentMD, strings.Join(meta, "\n"))
	var out llm.FiguresOut
	err = s.llm.ChatJSON(ctx, system, user, &out, func() error {
		seen := map[string]bool{}
		for i, f := range out.Figures {
			if strings.TrimSpace(f.ID) == "" {
				return fmt.Errorf("figures[%d].id 不能为空", i)
			}
			if seen[f.ID] {
				return fmt.Errorf("figures[%d].id 重复: %s", i, f.ID)
			}
			seen[f.ID] = true
			if _, ok := byPlaceholder[f.ID]; !ok {
				return fmt.Errorf("figures[%d].id 不在已有配图清单中: %s", i, f.ID)
			}
			if len(f.Data) == 0 {
				return fmt.Errorf("figures[%d].data 不能为空", i)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("重新生成配图: %w", err)
	}

	updated := 0
	for _, f := range out.Figures {
		asset := byPlaceholder[f.ID]
		title := strings.TrimSpace(f.Title)
		if title == "" {
			title = asset.Title
		}
		svg, rerr := svggen.Render(asset.Kind, title, string(f.Data))
		if rerr != nil {
			// 渲染是确定性流程，失败保留原图
			s.log.Warn("重渲染配图失败，保留原图",
				zap.Uint("article_id", art.ID), zap.String("figure", f.ID), zap.Error(rerr))
			continue
		}
		spec := llm.FigureOut{ID: asset.Placeholder, Kind: asset.Kind, Title: title, Data: f.Data}
		if uerr := s.db.WithContext(ctx).Model(&model.SvgAsset{}).
			Where("id = ?", asset.ID).
			Updates(map[string]any{"title": title, "spec": jsonify(spec), "svg_content": svg}).
			Error; uerr != nil {
			return nil, fmt.Errorf("更新配图 %s: %w", f.ID, uerr)
		}
		asset.Title = title
		updated++
	}
	s.portal.InvalidateCache(ctx, art.Slug)
	s.log.Info("配图重新生成完成",
		zap.Uint("article_id", art.ID), zap.Int("total", len(assets)), zap.Int("updated", updated))
	return s.Get(ctx, id)
}

// figures 查询文章全部配图元数据（含封面）。
func (s *ArticleService) figures(ctx context.Context, articleID uint) ([]FigureMeta, error) {
	figs := make([]FigureMeta, 0)
	err := s.db.WithContext(ctx).Model(&model.SvgAsset{}).
		Select("id, article_id, placeholder, kind, title, created_at").
		Where("article_id = ?", articleID).
		Order("id ASC").Find(&figs).Error
	if err != nil {
		return nil, fmt.Errorf("查询文章 %d 配图: %w", articleID, err)
	}
	return figs, nil
}

// CountWords 统计字数：CJK 字符按字计，连续拉丁字母/数字串按词计。
func CountWords(s string) int {
	n := 0
	inWord := false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			n++
			inWord = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if !inWord {
				n++
				inWord = true
			}
		default:
			inWord = false
		}
	}
	return n
}

// jsonifyTags 标签数组序列化为 datatypes.JSON（去掉空白项）。
func jsonifyTags(tags []string) datatypes.JSON {
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		if t = strings.TrimSpace(t); t != "" {
			clean = append(clean, t)
		}
	}
	return jsonify(clean)
}

// jsonify 序列化为 datatypes.JSON（存库用），失败返回空对象。
func jsonify(v any) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(b)
}

// truncateRunes 按字符数截断字符串。
func truncateRunes(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n])
}
