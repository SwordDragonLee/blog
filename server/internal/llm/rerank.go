package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"blog/server/internal/config"

	"go.uber.org/zap"
)

// Reranker 交叉编码重排客户端（SiliconFlow /v1/rerank 等 Jina 风格端点）。
// 与 Embedder 一样只有 OpenAI 兼容形态，但端点是 /rerank：不生成文本，
// 仅对每个 (query, document) 对输出相关性分，供向量粗排后精排使用。
type Reranker struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
	log     *zap.Logger
}

// NewReranker 创建重排客户端；rag.rerank_enabled=false 时返回 nil（功能整体关闭）。
// base_url/api_key 未单独配置时逐级回落：rerank → embedding → llm 主配置
// （bge-reranker 与 bge-m3 同在 SiliconFlow，通常直接复用 embedding 凭据）。
func NewReranker(cfg config.RAG, chat config.LLM, log *zap.Logger) *Reranker {
	if !cfg.RerankEnabled {
		return nil
	}
	base := firstNonEmpty(cfg.RerankBaseURL, cfg.EmbeddingBaseURL, chat.BaseURL)
	key := firstNonEmpty(cfg.RerankAPIKey, cfg.EmbeddingAPIKey, chat.APIKey)
	return &Reranker{
		baseURL: base,
		apiKey:  key,
		model:   cfg.RerankModel, // applyDefaults 已保证非空
		http:    &http.Client{Timeout: 30 * time.Second},
		log:     log,
	}
}

// firstNonEmpty 返回第一个非空串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// RerankResult 单个候选的重排结果：Index 对应输入 docs 的下标，Score 为相关性分。
type RerankResult struct {
	Index int
	Score float32
}

// rerankRequest /rerank 请求体（SiliconFlow / Jina 兼容格式）。
type rerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	ReturnDocuments bool     `json:"return_documents"` // false：只要分数，不回传文档省流量
}

// rerankResponse /rerank 响应体；results[i].index 标记对应输入文档。
type rerankResponse struct {
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float32 `json:"relevance_score"`
	} `json:"results"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"` // SiliconFlow 错误体为顶层 message
}

// Rerank 对候选文档按与 query 的相关性重排，返回按分数降序的结果。
// 网络/HTTP 错误指数退避重试（与 Embedder 同策略）。
func (r *Reranker) Rerank(ctx context.Context, query string, docs []string) ([]RerankResult, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	var lastErr error
	for attempt := 0; attempt <= maxCallRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		res, err := r.rerankOnce(ctx, query, docs)
		if err == nil {
			return res, nil
		}
		r.log.Warn("rerank 调用失败，准备重试", zap.Int("attempt", attempt+1), zap.Error(err))
		lastErr = err
	}
	return nil, lastErr
}

// rerankOnce 单次调用 /rerank，校验下标合法性并按分数降序排序。
func (r *Reranker) rerankOnce(ctx context.Context, query string, docs []string) ([]RerankResult, error) {
	payload, err := json.Marshal(rerankRequest{
		Model:           r.model,
		Query:           query,
		Documents:       docs,
		ReturnDocuments: false,
	})
	if err != nil {
		return nil, err
	}
	url := strings.TrimSuffix(r.baseURL, "/") + "/rerank"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rr rerankResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&rr); err != nil {
		return nil, fmt.Errorf("响应解析失败(HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || rr.Error != nil {
		msg := http.StatusText(resp.StatusCode)
		if rr.Error != nil {
			msg = rr.Error.Message
		} else if rr.Message != "" {
			msg = rr.Message
		}
		return nil, fmt.Errorf("接口返回 HTTP %d: %s", resp.StatusCode, msg)
	}
	out := make([]RerankResult, 0, len(rr.Results))
	for _, res := range rr.Results {
		if res.Index < 0 || res.Index >= len(docs) {
			return nil, fmt.Errorf("非法候选下标 %d（候选共 %d）", res.Index, len(docs))
		}
		out = append(out, RerankResult{Index: res.Index, Score: res.RelevanceScore})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("响应未包含任何重排结果")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}
