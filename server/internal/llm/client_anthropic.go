// client_anthropic.go 支持 Anthropic Messages 协议端点：当 base_url 包含
// "/anthropic" 时自动切换（如智谱开放平台 https://open.bigmodel.cn/api/anthropic）。
//
// Anthropic Messages 只是 Claude 定义的另一种请求格式（端点、认证头、字段名与
// OpenAI 格式不同）。智谱为兼容 Claude Code 等只认该格式的工具，专门开放了
// 此端点——请求仍发往智谱服务器、扣智谱额度，与 Anthropic 公司无关。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// chatOnce 按端点类型分发到对应协议实现。
func (c *Client) chatOnce(ctx context.Context, messages []Message) (string, error) {
	if c.isAnthropic() {
		return c.chatOnceAnthropic(ctx, messages)
	}
	return c.chatOnceOpenAI(ctx, messages)
}

// isAnthropic 判断是否使用 Anthropic Messages 协议。
func (c *Client) isAnthropic() bool {
	return strings.Contains(strings.ToLower(c.cfg.BaseURL), "/anthropic")
}

// chatOnceAnthropic 调用 Anthropic Messages 协议 /v1/messages：
// system 消息提取为顶层 system 参数，不支持 response_format，
// 结构化输出由 ChatJSON 的 JSON 提取与校验重试兜底。
func (c *Client) chatOnceAnthropic(ctx context.Context, messages []Message) (string, error) {
	var systemParts []string
	msgs := make([]Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			systemParts = append(systemParts, m.Content)
			continue
		}
		msgs = append(msgs, m)
	}
	body := map[string]any{
		"model":       c.cfg.Model,
		"max_tokens":  c.cfg.MaxTokens,
		"temperature": c.cfg.Temperature,
		// 关闭思考模式：GLM 会把思考计入 max_tokens，关闭后预算全部留给正文
		"thinking": map[string]string{"type": "disabled"},
		"messages": msgs,
	}
	if len(systemParts) > 0 {
		body["system"] = strings.Join(systemParts, "\n\n")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败(HTTP %d): %w", resp.StatusCode, err)
	}
	// 非 200 时也尝试从响应体解析 error 信息，便于定位 401/404/429 等问题
	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err != nil && resp.StatusCode == http.StatusOK {
		return "", fmt.Errorf("响应解析失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if ar.Error != nil {
			return "", fmt.Errorf("接口返回 HTTP %d: %s", resp.StatusCode, ar.Error.Message)
		}
		return "", fmt.Errorf("接口返回 HTTP %d: %s", resp.StatusCode, snippet(string(raw)))
	}
	if ar.Error != nil {
		return "", fmt.Errorf("接口返回错误: %s", ar.Error.Message)
	}
	var b strings.Builder
	for _, blk := range ar.Content {
		if blk.Type != "text" {
			continue
		}
		b.WriteString(blk.Text)
	}
	content := strings.TrimSpace(b.String())
	if content == "" {
		return "", errors.New("响应内容为空")
	}
	return content, nil
}

// snippet 压缩空白并截断到 200 字符，用于错误信息中展示响应体片段。
func snippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	rs := []rune(s)
	if len(rs) > 200 {
		return string(rs[:200]) + "..."
	}
	return s
}
