package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicAPIVersion     = "2023-06-01"
)

// AnthropicClient 实现 Anthropic Messages API（/v1/messages）。
type AnthropicClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	streamHTTP *http.Client
	maxRetries int
}

// NewAnthropicClient 创建 Anthropic 客户端。
func NewAnthropicClient(baseURL, apiKey, model string, timeoutSec, maxRetries int) *AnthropicClient {
	if baseURL == "" {
		baseURL = anthropicDefaultBaseURL
	}
	return &AnthropicClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		streamHTTP: &http.Client{Timeout: 0},
		maxRetries: maxRetries,
	}
}

func (c *AnthropicClient) ID() string { return "anthropic" }

// Chat 非流式调用。
func (c *AnthropicClient) Chat(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	body := c.buildBody(messages, opts, false)
	bodyBytes, _ := json.Marshal(body)

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		resp, err := c.doRequest(ctx, bodyBytes, false)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryable(err) {
			break
		}
	}
	return nil, lastErr
}

// ChatStream 流式调用。
func (c *AnthropicClient) ChatStream(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan Usage, <-chan error) {
	tokenCh := make(chan string, 64)
	usageCh := make(chan Usage, 1)
	errCh := make(chan error, 1)

	body := c.buildBody(messages, opts, true)
	bodyBytes, _ := json.Marshal(body)

	go func() {
		defer close(tokenCh)
		defer close(usageCh)
		defer close(errCh)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
		if err != nil {
			errCh <- err
			return
		}
		c.setHeaders(req)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := c.streamHTTP.Do(req)
		if err != nil {
			errCh <- fmt.Errorf("http stream: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(b))
			return
		}

		var finalUsage Usage
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			var event struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(data), &event) != nil {
				continue
			}

			switch event.Type {
			case "content_block_delta":
				if event.Delta.Text != "" {
					select {
					case tokenCh <- event.Delta.Text:
					case <-ctx.Done():
						return
					}
				}
			case "message_delta":
				if event.Usage != nil {
					finalUsage.CompletionTokens = event.Usage.OutputTokens
				}
			case "message_start":
				if event.Usage != nil {
					finalUsage.PromptTokens = event.Usage.InputTokens
				}
			}
		}
		finalUsage.TotalTokens = finalUsage.PromptTokens + finalUsage.CompletionTokens
		usageCh <- finalUsage
	}()

	return tokenCh, usageCh, errCh
}

func (c *AnthropicClient) doRequest(ctx context.Context, body []byte, stream bool) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, &retryableError{msg: "rate limited (429)"}
	}
	if resp.StatusCode >= 500 {
		return nil, &retryableError{msg: fmt.Sprintf("server error (%d)", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic error %d: %s", resp.StatusCode, string(b))
	}

	var payload struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	result := &Response{
		Usage: Usage{
			PromptTokens:     payload.Usage.InputTokens,
			CompletionTokens: payload.Usage.OutputTokens,
			TotalTokens:      payload.Usage.InputTokens + payload.Usage.OutputTokens,
		},
	}

	for _, block := range payload.Content {
		switch block.Type {
		case "text":
			result.Content += block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}
	return result, nil
}

func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
}

// buildBody 将 llm.Message 转换为 Anthropic Messages API 格式。
func (c *AnthropicClient) buildBody(messages []Message, opts Options, stream bool) map[string]any {
	model := opts.Model
	if model == "" {
		model = c.model
	}

	// 提取 system prompt（Anthropic 要求 system 在顶层，不在 messages 里）
	var system string
	var apiMsgs []map[string]any
	for _, m := range messages {
		if m.Role == "system" {
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue
		}

		if m.Role == "tool" {
			// 工具结果：Anthropic 用 user role + tool_result content block
			apiMsgs = append(apiMsgs, map[string]any{
				"role": "user",
				"content": []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
			continue
		}

		msg := map[string]any{"role": m.Role, "content": m.Content}
		apiMsgs = append(apiMsgs, msg)
	}

	body := map[string]any{
		"model":    model,
		"messages": apiMsgs,
		"stream":   stream,
	}
	if system != "" {
		body["system"] = system
	}

	// max_tokens 是 Anthropic 必填字段
	maxTokens := opts.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	body["max_tokens"] = maxTokens

	if opts.Temperature != nil {
		body["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		body["top_p"] = *opts.TopP
	}
	if opts.TopK != nil {
		body["top_k"] = *opts.TopK
	}
	if len(opts.Stop) > 0 {
		body["stop_sequences"] = opts.Stop
	}

	// 工具定义转换
	if len(opts.Tools) > 0 {
		var tools []map[string]any
		for _, t := range opts.Tools {
			tools = append(tools, map[string]any{
				"name":         t.Function.Name,
				"description":  t.Function.Description,
				"input_schema": json.RawMessage(t.Function.Parameters),
			})
		}
		body["tools"] = tools
	}

	return body
}
