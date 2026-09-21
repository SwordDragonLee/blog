package qdrant

import (
	"context"
	"fmt"
	"net/http"
)

// RemoveArticle 按 payload.article_id 删除某篇文章的全部向量（下线/退回草稿时调用）。
func (c *Client) RemoveArticle(ctx context.Context, articleID uint) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []any{
				map[string]any{
					"key":   "article_id",
					"match": map[string]any{"value": articleID},
				},
			},
		},
	}
	if _, err := c.do(ctx, http.MethodPost,
		"/collections/"+c.collection+"/points/delete?wait=true", body); err != nil {
		return fmt.Errorf("删除文章 %d 向量: %w", articleID, err)
	}
	return nil
}
