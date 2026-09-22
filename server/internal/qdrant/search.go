package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"go.uber.org/zap"
)

// SearchHit 检索命中：相似度得分 + payload 携带的块信息。
type SearchHit struct {
	Score   float32
	Content string
	Title   string
	Slug    string
}

// Scroll 全量滚动集合内的点，按点 ID 升序分页拉取。
// 只取计数所需的 payload 字段（不含 content，正文体积是块头的百倍），
// 供管理端索引总览按文章聚合使用；每页 256 条，直到 next_page_offset 为空。
func (c *Client) Scroll(ctx context.Context) ([]ChunkPoint, error) {
	var out []ChunkPoint
	var offset any // 上一页返回的 next_page_offset（点 ID）
	for {
		body := map[string]any{
			"limit":        256,
			"with_payload": []string{"article_id", "chunk_index", "title", "slug"},
		}
		if offset != nil {
			body["offset"] = offset
		}
		rawRes, err := c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points/scroll", body)
		if err != nil {
			if IsNotFound(err) {
				// 集合尚未创建（全新部署、还没有文章发布过）：按空索引返回
				c.log.Info("Qdrant 集合不存在，滚动按空索引处理", zap.String("collection", c.collection))
				return nil, nil
			}
			return nil, fmt.Errorf("滚动集合 %s: %w", c.collection, err)
		}
		var result struct {
			Points []struct {
				Payload struct {
					ArticleID  uint   `json:"article_id"`
					ChunkIndex int    `json:"chunk_index"`
					Title      string `json:"title"`
					Slug       string `json:"slug"`
				} `json:"payload"`
			} `json:"points"`
			NextPageOffset *uint64 `json:"next_page_offset"`
		}
		if err := json.Unmarshal(rawRes, &result); err != nil {
			return nil, fmt.Errorf("解析滚动结果: %w", err)
		}
		for _, p := range result.Points {
			out = append(out, ChunkPoint{
				ArticleID:  p.Payload.ArticleID,
				ChunkIndex: p.Payload.ChunkIndex,
				Title:      p.Payload.Title,
				Slug:       p.Payload.Slug,
			})
		}
		if result.NextPageOffset == nil {
			return out, nil
		}
		offset = *result.NextPageOffset
	}
}

// Search 以问题向量检索最相关的 topK 个块（按得分降序）。
func (c *Client) Search(ctx context.Context, vector []float32, topK int) ([]SearchHit, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": true,
	}
	rawRes, err := c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points/search", body)
	if err != nil {
		if IsNotFound(err) {
			// 集合尚未创建：按无命中处理，问答会走「没有相关内容」而非报错
			c.log.Info("Qdrant 集合不存在，检索按无命中处理", zap.String("collection", c.collection))
			return []SearchHit{}, nil
		}
		return nil, fmt.Errorf("向量检索: %w", err)
	}
	var result []struct {
		Score   float32 `json:"score"`
		Payload struct {
			Content string `json:"content"`
			Title   string `json:"title"`
			Slug    string `json:"slug"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(rawRes, &result); err != nil {
		return nil, fmt.Errorf("解析检索结果: %w", err)
	}
	hits := make([]SearchHit, 0, len(result))
	for _, h := range result {
		hits = append(hits, SearchHit{
			Score:   h.Score,
			Content: h.Payload.Content,
			Title:   h.Payload.Title,
			Slug:    h.Payload.Slug,
		})
	}
	return hits, nil
}

// RelatedHit 相关文章检索命中：块所属文章 slug 与相似度得分（结果按得分降序）。
type RelatedHit struct {
	Slug  string
	Score float32
}

// SearchExclude 以查询向量检索最相关的 topK 个块，但排除 excludeSlug 的块
// （推荐场景里即当前文章自己）。标题/正文等展示字段不取，由调用方回数据库补齐。
func (c *Client) SearchExclude(ctx context.Context, vector []float32, topK int, excludeSlug string) ([]RelatedHit, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        topK,
		"with_payload": []string{"slug"},
		"filter": map[string]any{
			"must_not": []any{
				map[string]any{"key": "slug", "match": map[string]any{"value": excludeSlug}},
			},
		},
	}
	rawRes, err := c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points/search", body)
	if err != nil {
		if IsNotFound(err) {
			// 集合尚未创建：按无命中处理，推荐由调用方规则兜底
			c.log.Info("Qdrant 集合不存在，排除式检索按无命中处理", zap.String("collection", c.collection))
			return []RelatedHit{}, nil
		}
		return nil, fmt.Errorf("排除式检索: %w", err)
	}
	var result []struct {
		Score   float32 `json:"score"`
		Payload struct {
			Slug string `json:"slug"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(rawRes, &result); err != nil {
		return nil, fmt.Errorf("解析排除式检索结果: %w", err)
	}
	hits := make([]RelatedHit, 0, len(result))
	for _, h := range result {
		hits = append(hits, RelatedHit{Slug: h.Payload.Slug, Score: h.Score})
	}
	return hits, nil
}
