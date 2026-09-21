package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// do 发起 REST 请求：序列化 body、校验 HTTP 状态、解析通用包装并返回 result 字段。
func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	code, rawBody, err := c.raw(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("Qdrant %s %s 返回 HTTP %d: %s", method, path, code, truncate(rawBody))
	}
	var resp qdrantResponse
	if err := json.Unmarshal(rawBody, &resp); err != nil {
		return nil, fmt.Errorf("解析 Qdrant 响应: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("Qdrant 返回错误: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

// raw 执行一次 HTTP 请求，返回状态码与原始响应体。
func (c *Client) raw(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 上限，足够覆盖本项目的响应
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("读取响应: %w", err)
	}
	return resp.StatusCode, data, nil
}

// truncate 截断错误响应体用于日志展示。
func truncate(b []byte) string {
	const max = 200
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}
