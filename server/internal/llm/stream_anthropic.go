package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// streamAnthropic 调 Anthropic Messages 协议（stream=true）：
// system 提为顶层参数、关闭思考模式（与 chatOnceAnthropic 一致），
// SSE 帧中 type=content_block_delta 时取 delta.text 增量，message_stop 后自然结束。
func (c *Client) streamAnthropic(ctx context.Context, messages []Message, onDelta func(string)) error {
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
		"thinking":    map[string]string{"type": "disabled"},
		"stream":      true,
		"messages":    msgs,
	}
	if len(systemParts) > 0 {
		body["system"] = strings.Join(systemParts, "\n\n")
	}
	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/v1/messages"
	headers := map[string]string{
		"x-api-key":         c.cfg.APIKey,
		"anthropic-version": "2023-06-01",
	}
	return c.streamSSE(ctx, url, headers, body, func(data []byte) error {
		var evt struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &evt); err != nil {
			return err
		}
		if evt.Error != nil {
			return fmt.Errorf("接口返回错误: %s", evt.Error.Message)
		}
		if evt.Type == "content_block_delta" && evt.Delta.Type == "text_delta" && evt.Delta.Text != "" {
			onDelta(evt.Delta.Text)
		}
		return nil
	})
}
