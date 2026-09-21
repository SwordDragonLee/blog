package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"blog/server/internal/config"

	"go.uber.org/zap"
)

// Embedder OpenAI 兼容 /embeddings 端点客户端：文本向量化，供 RAG 检索使用。
// 与 Chat 客户端解耦——chat 可能走 Anthropic 协议，embedding 只有 OpenAI 形态。
type Embedder struct {
	baseURL        string
	apiKey         string
	model          string
	dims           int
	sendDimensions bool
	http           *http.Client
	log            *zap.Logger
}

// NewEmbedder 创建 embedding 客户端；base_url/api_key 未单独配置时回落到 chat 配置。
func NewEmbedder(cfg config.RAG, chat config.LLM, log *zap.Logger) *Embedder {
	base := cfg.EmbeddingBaseURL
	if base == "" {
		base = chat.BaseURL
	}
	key := cfg.EmbeddingAPIKey
	if key == "" {
		key = chat.APIKey
	}
	return &Embedder{
		baseURL:        base,
		apiKey:         key,
		model:          cfg.EmbeddingModel,
		dims:           cfg.EmbeddingDimensions,
		sendDimensions: cfg.EmbeddingSendDimensions,
		http:           &http.Client{Timeout: 60 * time.Second},
		log:            log,
	}
}

// embedRequest /embeddings 请求体；dimensions 用指针区分「未配置不发」与「发 0」，
// 部分 provider（如 SiliconFlow 的 bge-m3）不接受该参数。
type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions *int     `json:"dimensions,omitempty"`
}

// embedResponse /embeddings 响应体；data[i].index 标记与输入的对应关系。
type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Embed 批量向量化文本，返回向量顺序与输入一致；网络/HTTP 错误指数退避重试。
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, len(texts))
	var lastErr error
	for attempt := 0; attempt <= maxCallRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		err := e.embedOnce(ctx, texts, out)
		if err == nil {
			return out, nil
		}
		e.log.Warn("embedding 调用失败，准备重试", zap.Int("attempt", attempt+1), zap.Error(err))
		lastErr = err
	}
	return nil, lastErr
}

// embedOnce 单次调用 /embeddings，按响应中的 index 回填结果。
func (e *Embedder) embedOnce(ctx context.Context, texts []string, out [][]float32) error {
	var dims *int
	if e.sendDimensions {
		dims = &e.dims
	}
	payload, err := json.Marshal(embedRequest{
		Model:      e.model,
		Input:      texts,
		Dimensions: dims,
	})
	if err != nil {
		return err
	}
	url := strings.TrimSuffix(e.baseURL, "/") + "/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var er embedResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&er); err != nil {
		return fmt.Errorf("响应解析失败(HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || er.Error != nil {
		msg := http.StatusText(resp.StatusCode)
		if er.Error != nil {
			msg = er.Error.Message
		}
		return fmt.Errorf("接口返回 HTTP %d: %s", resp.StatusCode, msg)
	}
	if len(er.Data) != len(texts) {
		return fmt.Errorf("返回向量数 %d 与输入 %d 不一致", len(er.Data), len(texts))
	}
	for _, d := range er.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return fmt.Errorf("非法向量下标 %d", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	for i, v := range out {
		if len(v) == 0 {
			return fmt.Errorf("第 %d 条文本未返回向量", i)
		}
	}
	return nil
}
