package service

import (
	"context"
	"fmt"
	"strings"

	"blog/server/internal/config"
	"blog/server/internal/llm"
	"blog/server/internal/model"
	"blog/server/internal/qdrant"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// RagService 基于 Qdrant 向量检索的「全站技术问答」：
// 文章发布后切块向量化入库；提问时检索相关片段交由 LLM 流式回答并附引用。
type RagService struct {
	db       *gorm.DB
	chat     *llm.Client
	embedder *llm.Embedder
	store    *qdrant.Client
	cfg      config.RAG
	log      *zap.Logger
}

// NewRagService 创建问答服务（各依赖可为 nil，配合 rag.enabled=false 关闭功能）。
func NewRagService(db *gorm.DB, chat *llm.Client, embedder *llm.Embedder,
	store *qdrant.Client, cfg config.RAG, log *zap.Logger) *RagService {
	return &RagService{db: db, chat: chat, embedder: embedder, store: store, cfg: cfg, log: log}
}

// Enabled 判断问答功能是否可用。
func (s *RagService) Enabled() bool {
	return s != nil && s.cfg.Enabled && s.chat != nil && s.embedder != nil && s.store != nil
}

// Citation 回答引用的文章出处，随 SSE citations 事件发给前台。
type Citation struct {
	Title   string  `json:"title"`
	Slug    string  `json:"slug"`
	Score   float32 `json:"score"`
	Snippet string  `json:"snippet"`
}

// Turn 一轮对话历史（携带最近几轮做上下文）。
type Turn struct {
	Role    string `json:"role"` // user / assistant
	Content string `json:"content"`
}

// IndexArticle 把文章正文切块向量化写入 Qdrant（发布后异步调用）。
// 幂等：先清除该文旧向量再写入，重复发布/重试安全。
func (s *RagService) IndexArticle(ctx context.Context, art *model.Article) error {
	if !s.Enabled() {
		return nil
	}
	// 旧向量清理失败不阻断（确定性 point ID 会覆盖同 ID 点，残余仅在块数变少时出现）
	if err := s.store.RemoveArticle(ctx, art.ID); err != nil {
		s.log.Warn("清理文章旧向量失败，继续覆盖写入", zap.Uint("article_id", art.ID), zap.Error(err))
	}
	clean := cleanMarkdownForIndex(art.ContentMD)
	chunks := chunkMarkdown(clean, s.cfg.ChunkSize, s.cfg.ChunkOverlap)
	if len(chunks) == 0 {
		s.log.Info("文章无可索引正文，跳过向量化", zap.Uint("article_id", art.ID))
		return nil
	}
	vectors, err := s.embedder.Embed(ctx, chunks)
	if err != nil {
		return fmt.Errorf("文章 %d 向量化失败: %w", art.ID, err)
	}
	if err := s.store.EnsureCollection(ctx, s.cfg.EmbeddingDimensions); err != nil {
		return fmt.Errorf("确保集合存在: %w", err)
	}
	points := make([]qdrant.ChunkPoint, 0, len(chunks))
	for i, chunk := range chunks {
		points = append(points, qdrant.ChunkPoint{
			ArticleID:  art.ID,
			ChunkIndex: i,
			Content:    chunk,
			Title:      art.Title,
			Slug:       art.Slug,
			Vector:     vectors[i],
		})
	}
	if err := s.store.UpsertChunks(ctx, points); err != nil {
		return fmt.Errorf("文章 %d 向量入库失败: %w", art.ID, err)
	}
	s.log.Info("文章已向量化入库",
		zap.Uint("article_id", art.ID), zap.String("slug", art.Slug), zap.Int("chunks", len(chunks)))
	return nil
}

// RemoveArticle 删除文章的全部向量（下线/退回草稿时调用）。
func (s *RagService) RemoveArticle(ctx context.Context, articleID uint) {
	if !s.Enabled() {
		return
	}
	if err := s.store.RemoveArticle(ctx, articleID); err != nil {
		s.log.Warn("删除文章向量失败", zap.Uint("article_id", articleID), zap.Error(err))
		return
	}
	s.log.Info("文章向量已删除", zap.Uint("article_id", articleID))
}

// askSystemPrompt 回答规则：仅依据片段、标注引用、不足则明说。
const askSystemPrompt = `你是技术博客的问答助手。回答规则：
1. 仅依据「参考文章片段」回答问题，用中文回答；
2. 在依据对应片段的句子末尾标注引用编号，如 [1]；
3. 片段不足以回答时，直接说明「暂未找到相关内容」，不要编造；
4. 回答简洁准确，可用 markdown 组织（列表/代码块）。`

// Ask 检索相关片段并流式生成回答：每段增量文本回调 onDelta，
// 返回本次回答引用的文章出处。history 携带最近几轮对话上下文（可空）。
func (s *RagService) Ask(ctx context.Context, question string, history []Turn, onDelta func(string)) ([]Citation, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("%w: 问题不能为空", ErrInvalid)
	}
	if !s.Enabled() {
		return nil, fmt.Errorf("问答功能未启用")
	}

	// 1. 问题向量化 + 检索
	vecs, err := s.embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}
	hits, err := s.store.Search(ctx, vecs[0], s.cfg.TopK)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	// 按文章分组：同一文章的多个命中块合并进同一组上下文，「资料来源」每篇只出一条。
	// 分组必须在组装上下文时完成而非前端展示层去重——正文 [n] 标注引用的是分组后的
	// 文章序号，展示层去重会让 [3] 指向列表里不存在的条目。
	// hits 按得分降序，每组首个块即组内最高分，其摘要与得分代表该文章。
	type articleGroup struct {
		slug     string
		title    string
		score    float32
		contents []string
	}
	groups := make([]*articleGroup, 0, len(hits))
	bySlug := make(map[string]*articleGroup, len(hits))
	for _, h := range hits {
		if h.Score < s.cfg.ScoreThreshold {
			continue // 低于阈值的命中视为噪声
		}
		g, ok := bySlug[h.Slug]
		if !ok {
			g = &articleGroup{slug: h.Slug, title: h.Title, score: h.Score}
			bySlug[h.Slug] = g
			groups = append(groups, g)
		}
		g.contents = append(g.contents, h.Content)
	}

	// 2. 无相关内容：不调 LLM，直接给固定话术（省 token 且不编造）；
	//    问题顺手落库供选题回流——vecs[0] 是检索时已算好的向量，白拿不重算
	if len(groups) == 0 {
		s.CaptureUnanswered(ctx, question, vecs[0])
		onDelta("暂未在博客文章中找到与该问题相关的内容，换个问法或浏览文章列表试试。")
		return nil, nil
	}

	// 3. 组装消息：system 规则 + 片段与历史 + 问题
	citations := make([]Citation, 0, len(groups))
	var ctxParts strings.Builder
	for i, g := range groups {
		snippet := g.contents[0]
		if r := []rune(snippet); len(r) > 300 {
			snippet = string(r[:300]) + "..."
		}
		ctxParts.WriteString(fmt.Sprintf("\n[%d] 《%s》\n%s\n", i+1, g.title, strings.Join(g.contents, "\n\n")))
		citations = append(citations, Citation{
			Title:   g.title,
			Slug:    g.slug,
			Score:   g.score,
			Snippet: snippet,
		})
	}
	msgs := []llm.Message{{Role: "system", Content: askSystemPrompt}}
	for _, t := range history {
		if t.Content == "" {
			continue
		}
		role := t.Role
		if role != "user" && role != "assistant" {
			continue
		}
		msgs = append(msgs, llm.Message{Role: role, Content: truncateRunes(t.Content, 2000)})
	}
	msgs = append(msgs, llm.Message{Role: "user", Content: fmt.Sprintf(
		"参考文章片段：\n%s\n\n用户问题：%s", ctxParts.String(), question)})

	// 4. 流式生成
	if err := s.chat.StreamChat(ctx, msgs, onDelta); err != nil {
		return nil, fmt.Errorf("生成回答失败: %w", err)
	}
	return citations, nil
}

// RelatedCandidate 相关文章候选：文章 slug 与其最高块相似度。
type RelatedCandidate struct {
	Slug  string
	Score float32
}

// RelatedArticles 语义相似文章检索：取本文首块（chunk_index=0）的存量向量作查询向量，
// 全库检索最相似的块（排除本文），按文章聚合（同篇多块取首遇最高分，hits 本身按得分降序）。
// 首块通常为文章开篇，最能代表全文主题；复用发布时入库的向量，读路径零 embedding 调用。
// RAG 未启用 / 文章未索引 / 依赖故障一律返回 nil 并仅记日志——推荐是增强能力，
// 由调用方降级为规则兜底，不作为错误上抛。
func (s *RagService) RelatedArticles(ctx context.Context, articleID uint, excludeSlug string, limit int) []RelatedCandidate {
	if !s.Enabled() {
		return nil
	}
	// 取本文首块（chunk_index=0）的存量向量作查询向量，point ID = articleID*10000 确定性可推。
	// ok=false 表示该点不存在（文章从未向量化，如 RAG 关闭期发布的历史文章）；
	// err 是 Qdrant 网络/服务故障。两种情况都不上抛，返回 nil 由调用方走规则兜底。
	vector, ok, err := s.store.GetPointVector(ctx, articleID)
	if err != nil {
		s.log.Warn("取文章首块向量失败，相关推荐降级",
			zap.Uint("article_id", articleID), zap.Error(err))
		return nil
	}
	if !ok {
		return nil
	}
	hits, err := s.store.SearchExclude(ctx, vector, limit*3, excludeSlug)
	if err != nil {
		s.log.Warn("相关文章向量检索失败，推荐降级",
			zap.Uint("article_id", articleID), zap.Error(err))
		return nil
	}
	candidates := make([]RelatedCandidate, 0, limit)
	seen := make(map[string]bool, len(hits)) // 文章去重
	for _, h := range hits {
		if h.Score < s.cfg.ScoreThreshold || seen[h.Slug] {
			continue
		}
		seen[h.Slug] = true
		candidates = append(candidates, RelatedCandidate{Slug: h.Slug, Score: h.Score})
		if len(candidates) >= limit {
			break
		}
	}
	return candidates
}
