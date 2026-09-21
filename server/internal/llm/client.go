// Package llm 实现 LLM Chat 客户端：JSON 结构化输出校验、
// 失败自动重试（指数退避）与基于 Redis 的简易限流。
//
// "OpenAI 兼容" 指一套请求/响应的 JSON 格式，而非 OpenAI 公司的服务——
// 智谱、DeepSeek、SiliconFlow 等厂商都按这套格式开放端点，换厂商只需换 base_url。
// base_url 含 "/anthropic" 时切换到 Anthropic Messages 格式（见 client_anthropic.go）。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"blog/server/internal/config"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	maxCallRetries    = 3 // 网络/接口错误重试次数
	maxOutputAttempts = 3 // JSON 校验失败重试次数
)

// Client 是 LLM 服务客户端，支持 OpenAI 兼容与 Anthropic Messages 两种请求格式，
// 由 cfg.BaseURL 自动判别；无论哪种格式，请求都发往同一个厂商端点（如智谱）。
type Client struct {
	cfg  config.LLM
	http *http.Client
	rdb  *redis.Client // 可为 nil，跳过限流
	log  *zap.Logger
}

func NewClient(cfg config.LLM, rdb *redis.Client, log *zap.Logger) *Client {
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		rdb: rdb,
		log: log,
	}
}

// ChatJSON 发起一次对话并要求模型输出 JSON，解析到 out；
// validate 为nil 时仅做 JSON 解析校验，否则追加语义校验，
// 任一校验失败会把错误信息反馈给模型重试（最多 maxOutputAttempts 次）。
func (c *Client) ChatJSON(ctx context.Context, system, user string, out any, validate func() error) error {
	messages := []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	var lastErr error
	for attempt := 0; attempt < maxOutputAttempts; attempt++ {
		if attempt > 0 {
			hint := "你上一次的输出存在问题，无法通过校验，错误信息：" + lastErr.Error() +
				"。请严格按要求重新输出，直接给出合法 JSON，不要包含任何解释、markdown 代码块或多余文本。"
			messages = append(messages,
				Message{Role: "assistant", Content: "好的。"},
				Message{Role: "user", Content: hint},
			)
		}
		content, err := c.chatWithRetry(ctx, messages)
		if err != nil {
			return fmt.Errorf("调用大模型失败: %w", err)
		}
		raw, err := extractJSON(content)
		if err != nil {
			lastErr = err
			continue
		}
		if err := json.Unmarshal(raw, out); err != nil {
			lastErr = fmt.Errorf("JSON 解析失败: %w", err)
			continue
		}
		if validate != nil {
			if err := validate(); err != nil {
				lastErr = err
				continue
			}
		}
		return nil
	}
	return fmt.Errorf("模型输出 %d 次均未通过校验: %w", maxOutputAttempts, lastErr)
}

// chatWithRetry 处理网络/HTTP 层错误，指数退避重试。
func (c *Client) chatWithRetry(ctx context.Context, messages []Message) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= maxCallRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		if err := c.throttle(ctx); err != nil {
			return "", err
		}
		content, err := c.chatOnce(ctx, messages)
		if err == nil {
			return content, nil
		}
		lastErr = err
		c.log.Warn("LLM 调用失败，准备重试", zap.Int("attempt", attempt+1), zap.Error(err))
	}
	return "", lastErr
}

// chatOnceOpenAI 调用 OpenAI 兼容的 /chat/completions（默认协议）。
// "调用大模型 API" 本质就是一个 HTTP 请求，四要素在此凑齐：
// 地址 = base_url+/chat/completions；身份 = api_key 认证头；
// 内容 = {model,messages} JSON；发送 = c.http.Do（请求真正上网络的时刻）。
func (c *Client) chatOnceOpenAI(ctx context.Context, messages []Message) (string, error) {
	reqBody := chatRequest{
		Model:          c.cfg.Model,
		Messages:       messages,
		Temperature:    c.cfg.Temperature,
		MaxTokens:      c.cfg.MaxTokens,
		ResponseFormat: &responseFormat{Type: "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req) // 此前都是本地打包，这一行才真正发往厂商服务器
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var cr chatResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&cr); err != nil {
		return "", fmt.Errorf("响应解析失败(HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := http.StatusText(resp.StatusCode)
		if cr.Error != nil {
			msg = cr.Error.Message
		}
		return "", fmt.Errorf("接口返回 HTTP %d: %s", resp.StatusCode, msg)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("接口返回错误: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("响应中没有 choices")
	}
	content := cr.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		return "", errors.New("响应内容为空")
	}
	return content, nil
}

// throttle 基于 Redis 固定窗口限流；rdb 为 nil 时直接放行。
func (c *Client) throttle(ctx context.Context) error {
	if c.rdb == nil || c.cfg.RatePerMinute <= 0 {
		return nil
	}
	key := fmt.Sprintf("llm:ratelimit:%d", time.Now().Unix()/60)
	for {
		n, err := c.rdb.Incr(ctx, key).Result()
		if err != nil {
			return nil // 限流依赖不可用时放行，不阻塞主流程
		}
		if n == 1 {
			c.rdb.Expire(ctx, key, 2*time.Minute)
		}
		if n <= int64(c.cfg.RatePerMinute) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// extractJSON 从模型输出中提取 JSON 体（容忍 markdown 代码块包裹）。
func extractJSON(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, errors.New("输出中未找到 JSON 对象")
	}
	return []byte(s[start : end+1]), nil
}
