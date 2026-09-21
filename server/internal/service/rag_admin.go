package service

// RAG 管理端能力：向量索引总览与检索测试。
// 都挂在 RagService 上，复用问答同一套 embedder / store；只读，不影响线上问答。

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// RagIndexStat 单篇已索引文章的向量块统计。
type RagIndexStat struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Chunks int    `json:"chunks"`
}

// RagIndexOverview 向量索引总览：按文章分组的块数 + 全库合计。
type RagIndexOverview struct {
	Articles    []RagIndexStat `json:"articles"`
	TotalChunks int            `json:"total_chunks"`
}

// IndexOverview 滚动全量向量点、按文章分组统计。供管理端总览索引健康度：
// 某篇已发布文章不在此列即漏索引，已下线文章仍在此列即残留向量。
func (s *RagService) IndexOverview(ctx context.Context) (*RagIndexOverview, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("问答功能未启用")
	}
	points, err := s.store.Scroll(ctx)
	if err != nil {
		return nil, fmt.Errorf("读取向量集合: %w", err)
	}
	bySlug := make(map[string]*RagIndexStat)
	for _, p := range points {
		g, ok := bySlug[p.Slug]
		if !ok {
			g = &RagIndexStat{Slug: p.Slug, Title: p.Title}
			bySlug[p.Slug] = g
		}
		g.Chunks++
	}
	stats := make([]RagIndexStat, 0, len(bySlug))
	for _, g := range bySlug {
		stats = append(stats, *g)
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Slug < stats[j].Slug })
	return &RagIndexOverview{Articles: stats, TotalChunks: len(points)}, nil
}

// ProbeHit 检索测试的单条命中。
type ProbeHit struct {
	Score   float32 `json:"score"`
	Slug    string  `json:"slug"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Pass    bool    `json:"pass"` // 得分是否达到阈值（达到才会被真实问答采纳）
}

// Probe 检索测试：问题向量化后检索 topK 条，返回原始命中与得分分布。
// 与 Ask 的差异：不做文章分组、不调对话模型——目的是观测检索层本身，
// 为校准 score_threshold 提供依据（短查询下得分分布平坦的问题就是在这里定位的）。
func (s *RagService) Probe(ctx context.Context, question string, topK int, threshold float32) ([]ProbeHit, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("问答功能未启用")
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("%w: 问题不能为空", ErrInvalid)
	}
	vecs, err := s.embedder.Embed(ctx, []string{question})
	if err != nil {
		return nil, fmt.Errorf("问题向量化失败: %w", err)
	}
	hits, err := s.store.Search(ctx, vecs[0], topK)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	out := make([]ProbeHit, 0, len(hits))
	for _, h := range hits {
		snippet := h.Content
		if r := []rune(snippet); len(r) > 120 {
			snippet = string(r[:120]) + "..."
		}
		out = append(out, ProbeHit{
			Score:   h.Score,
			Slug:    h.Slug,
			Title:   h.Title,
			Snippet: snippet,
			Pass:    h.Score >= threshold,
		})
	}
	return out, nil
}

// ScoreThreshold 当前配置的相似度阈值，作为检索测试的默认值。
func (s *RagService) ScoreThreshold() float32 { return s.cfg.ScoreThreshold }
