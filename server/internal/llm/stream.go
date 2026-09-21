package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// StreamChat 流式对话：按端点协议分发，每收到一段增量文本回调一次 onDelta。
// 用于 RAG 问答的打字机式输出；流式调用不计入用量记录（消耗小、面板已隐藏）。
func (c *Client) StreamChat(ctx context.Context, messages []Message, onDelta func(string)) error {
	if c.isAnthropic() {
		return c.streamAnthropic(ctx, messages, onDelta)
	}
	return c.streamOpenAI(ctx, messages, onDelta)
}

// streamOpenAI 调 OpenAI 兼容 /chat/completions（stream=true），
// 解析 SSE 数据行中 choices[0].delta.content 增量。
func (c *Client) streamOpenAI(ctx context.Context, messages []Message, onDelta func(string)) error {
	reqBody := map[string]any{
		"model":       c.cfg.Model,
		"messages":    messages,
		"temperature": c.cfg.Temperature,
		"max_tokens":  c.cfg.MaxTokens,
		"stream":      true,
	}
	url := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"
	headers := map[string]string{"Authorization": "Bearer " + c.cfg.APIKey}

	return c.streamSSE(ctx, url, headers, reqBody, func(data []byte) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if chunk.Error != nil {
			return fmt.Errorf("接口返回错误: %s", chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onDelta(chunk.Choices[0].Delta.Content)
		}
		return nil
	})
}

// streamSSE 通用 SSE 消费：POST JSON body，逐个 data: 帧回调 handle；
// data: [DONE] 结束；handle 返回错误则整体中止。
func (c *Client) streamSSE(ctx context.Context, url string, headers map[string]string, body any, handle func([]byte) error) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("接口返回 HTTP %d", resp.StatusCode)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				return nil
			}
			continue
		}
		if err := handle([]byte(data)); err != nil {
			return err
		}
	}
	return sc.Err()
}
