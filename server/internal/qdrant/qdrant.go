// Package qdrant Qdrant 向量数据库 REST API 轻封装，供 RAG 技术问答存取文章向量。
// 仅封装本项目需要的四个操作，刻意不引入官方 gRPC SDK 的依赖链。
package qdrant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Client Qdrant REST 客户端。
type Client struct {
	baseURL    string
	collection string
	http       *http.Client
	log        *zap.Logger
}

// New 创建客户端（不立即建连，REST 无状态）。
func New(baseURL, collection string, log *zap.Logger) *Client {
	return &Client{
		baseURL:    baseURL,
		collection: collection,
		http:       &http.Client{Timeout: 30 * time.Second},
		log:        log,
	}
}

// pointID 由文章 ID 与块下标派生确定性整数 ID：重复索引时自然覆盖同 ID 点。
// 每篇文章块数上限 9999，远超实际规模。
func pointID(articleID uint, chunkIndex int) uint64 {
	return uint64(articleID)*10000 + uint64(chunkIndex)
}

// ChunkPoint 单个文章块的向量点。ID 派生规则见 pointID。
type ChunkPoint struct {
	ArticleID  uint
	ChunkIndex int
	Content    string
	Title      string
	Slug       string
	Vector     []float32
}

// qdrant 通用响应包装。
type qdrantResponse struct {
	Result json.RawMessage `json:"result"`
	Status string          `json:"status"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// EnsureCollection 幂等创建集合（cosine 距离，指定向量维度）；已存在则直接复用。
func (c *Client) EnsureCollection(ctx context.Context, dims int) error {
	exists, err := c.collectionExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	body := map[string]any{
		"vectors": map[string]any{"size": dims, "distance": "Cosine"},
	}
	if _, err := c.do(ctx, http.MethodPut, "/collections/"+c.collection, body); err != nil {
		return fmt.Errorf("创建集合 %s: %w", c.collection, err)
	}
	c.log.Info("已创建 Qdrant 集合", zap.String("collection", c.collection), zap.Int("dims", dims))
	return nil
}

// collectionExists 判断集合是否已存在。
func (c *Client) collectionExists(ctx context.Context) (bool, error) {
	code, _, err := c.raw(ctx, http.MethodGet, "/collections/"+c.collection, nil)
	if err != nil {
		return false, err
	}
	if code == http.StatusNotFound {
		return false, nil
	}
	if code != http.StatusOK {
		return false, fmt.Errorf("查询集合状态 HTTP %d", code)
	}
	return true, nil
}

// GetPointVector 按确定性 ID 取文章首块（chunk_index=0）的存量向量，
// 供相关文章推荐作查询向量；点不存在（文章未向量化过）返回 ok=false。
// 走按 IDs 批量取点接口而非 GET 单点：缺失的点直接不出现在结果里，省去 404 分支。
func (c *Client) GetPointVector(ctx context.Context, articleID uint) (vector []float32, ok bool, err error) {
	body := map[string]any{
		"ids":         []uint64{pointID(articleID, 0)},
		"with_vector": true,
	}
	rawRes, err := c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points", body)
	if err != nil {
		if IsNotFound(err) {
			// 集合尚未创建：等价于从未索引，按 ok=false 处理
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("取文章 %d 首块向量: %w", articleID, err)
	}
	var result []struct {
		Vector []float32 `json:"vector"`
	}
	if err := json.Unmarshal(rawRes, &result); err != nil {
		return nil, false, fmt.Errorf("解析点向量: %w", err)
	}
	if len(result) == 0 || len(result[0].Vector) == 0 {
		return nil, false, nil
	}
	return result[0].Vector, true, nil
}

// UpsertChunks 批量写入（或覆盖）文章块向量，wait=true 确保返回即落盘。
func (c *Client) UpsertChunks(ctx context.Context, points []ChunkPoint) error {
	if len(points) == 0 {
		return nil
	}
	type pointDTO struct {
		ID      uint64         `json:"id"`
		Vector  []float32      `json:"vector"`
		Payload map[string]any `json:"payload"`
	}
	dtos := make([]pointDTO, 0, len(points))
	for _, p := range points {
		dtos = append(dtos, pointDTO{
			ID:     pointID(p.ArticleID, p.ChunkIndex),
			Vector: p.Vector,
			Payload: map[string]any{
				"article_id":  p.ArticleID,
				"chunk_index": p.ChunkIndex,
				"content":     p.Content,
				"title":       p.Title,
				"slug":        p.Slug,
			},
		})
	}
	_, err := c.do(ctx, http.MethodPut, "/collections/"+c.collection+"/points?wait=true",
		map[string]any{"points": dtos})
	return err
}
