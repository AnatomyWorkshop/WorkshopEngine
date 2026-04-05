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

// Message OpenAI 兼容消息格式（支持 tool_calls 字段）
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool 时使用
	Name       string     `json:"name,omitempty"`         // role=tool 时函数名
}

// ToolDefinition OpenAI function calling 工具定义
type ToolDefinition struct {
	Type     string          `json:"type"` // 固定为 "function"
	Function ToolFunctionDef `json:"function"`
}

// ToolFunctionDef 工具函数描述（JSON Schema 参数规范）
type ToolFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolCall LLM 回包中的工具调用请求
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用的函数名与参数
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// Options 调用选项（覆盖客户端默认值）。
//
// 指针字段使用 nil 表示「未配置」，不会发送给 API，让服务端使用模型默认值。
// 这允许 temperature=0（贪婪解码）与「不发送 temperature」两种情形精确区分。
type Options struct {
	// 路由
	Model string // 模型 ID，为空时使用 client 初始值

	// 生成上限
	MaxTokens int // 最大输出 token 数，0 = 不限制（由 API 决定）

	// 采样参数（nil = 不发送，使用 API / 模型默认值）
	Temperature      *float64 // 0–2；0 = 贪婪解码
	TopP             *float64 // 0–1；nucleus 采样
	TopK             *int     // ≥0；top-K 采样（OpenAI 不支持，本地模型适用）
	FrequencyPenalty *float64 // -2–2；降低重复词频
	PresencePenalty  *float64 // -2–2；鼓励涉及新话题

	// 高级控制
	ReasoningEffort string   // "low"|"medium"|"high"；空 = 不发送（o1/o3 系列）
	Stop            []string // 停止序列，nil/空 = 不发送

	// 工具调用（nil = 不发送，不启用 function calling）
	Tools []ToolDefinition

	// 传输模式（由调用方设置，不开放给用户配置）
	Stream bool
}

// Usage Token 用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Response 非流式响应
type Response struct {
	Content   string
	ToolCalls []ToolCall // 非空时表示 LLM 请求调用工具；Content 可能为空
	Usage     Usage
}

// Client OpenAI 兼容 LLM 客户端
type Client struct {
	baseURL     string
	apiKey      string
	model       string
	httpClient  *http.Client
	maxRetries  int
	defaultOpts Options // 来自配置，作为 per-request 参数的兜底
}

// NewClient 创建客户端
func NewClient(baseURL, apiKey, model string, timeoutSec, maxRetries int) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		maxRetries: maxRetries,
		httpClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

// BaseURL 返回客户端的 API 地址（供需要临时覆盖 key/url 的调用方读取）
func (c *Client) BaseURL() string { return c.baseURL }

// WithDefaults 返回一个携带默认采样参数的新客户端（来自配置层，不影响原客户端）
func (c *Client) WithDefaults(defaults Options) *Client {
	clone := *c
	clone.defaultOpts = defaults
	return &clone
}

// Chat 非流式调用（One-Shot，主链路使用）
func (c *Client) Chat(ctx context.Context, messages []Message, opts Options) (*Response, error) {
	merged := c.mergeOpts(opts)
	merged.Stream = false

	body, _ := json.Marshal(buildBody(merged, messages))

	var result *Response
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
		result, lastErr = c.doChat(ctx, body)
		if lastErr == nil {
			return result, nil
		}
		if !isRetryable(lastErr) {
			break
		}
	}
	return nil, lastErr
}

func (c *Client) doChat(ctx context.Context, body []byte) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, string(b))
	}

	var payload struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(payload.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in response")
	}
	return &Response{
		Content:   payload.Choices[0].Message.Content,
		ToolCalls: payload.Choices[0].Message.ToolCalls,
		Usage:     payload.Usage,
	}, nil
}

// ChatStream SSE 流式调用（供前端打字动画）
// 返回三个 channel：token 碎片、Usage（流结束时推送一次）、error
func (c *Client) ChatStream(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan Usage, <-chan error) {
	tokenCh := make(chan string, 64)
	usageCh := make(chan Usage, 1)
	errCh := make(chan error, 1)

	merged := c.mergeOpts(opts)
	merged.Stream = true

	// 请求 usage 统计（OpenAI/DeepSeek 支持 stream_options）
	body := buildBody(merged, messages)
	body["stream_options"] = map[string]any{"include_usage": true}
	bodyBytes, _ := json.Marshal(body)

	go func() {
		defer close(tokenCh)
		defer close(usageCh)
		defer close(errCh)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
		if err != nil {
			errCh <- err
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			errCh <- fmt.Errorf("http stream: %w", err)
			return
		}
		defer resp.Body.Close()

		var finalUsage Usage
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Usage *Usage `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Usage != nil && chunk.Usage.TotalTokens > 0 {
				finalUsage = *chunk.Usage
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case tokenCh <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
			}
		}
		usageCh <- finalUsage
	}()

	return tokenCh, usageCh, errCh
}

// ── 内部工具 ─────────────────────────────────────────────

// mergeOpts 将 per-request opts 叠加在客户端默认值之上。
// 规则：non-nil / non-zero 的 per-request 字段优先，否则继承 defaultOpts。
func (c *Client) mergeOpts(req Options) Options {
	merged := c.defaultOpts
	if req.Model != "" {
		merged.Model = req.Model
	}
	if merged.Model == "" {
		merged.Model = c.model
	}
	if req.MaxTokens > 0 {
		merged.MaxTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		merged.Temperature = req.Temperature
	}
	if req.TopP != nil {
		merged.TopP = req.TopP
	}
	if req.TopK != nil {
		merged.TopK = req.TopK
	}
	if req.FrequencyPenalty != nil {
		merged.FrequencyPenalty = req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		merged.PresencePenalty = req.PresencePenalty
	}
	if req.ReasoningEffort != "" {
		merged.ReasoningEffort = req.ReasoningEffort
	}
	if len(req.Stop) > 0 {
		merged.Stop = req.Stop
	}
	if len(req.Tools) > 0 {
		merged.Tools = req.Tools
	}
	merged.Stream = req.Stream
	return merged
}

// buildBody 构建发往 API 的 JSON 请求体，只包含明确设置的字段。
func buildBody(opts Options, messages []Message) map[string]any {
	m := map[string]any{
		"model":    opts.Model,
		"messages": messages,
		"stream":   opts.Stream,
	}
	if opts.MaxTokens > 0 {
		m["max_tokens"] = opts.MaxTokens
	}
	if opts.Temperature != nil {
		m["temperature"] = *opts.Temperature
	}
	if opts.TopP != nil {
		m["top_p"] = *opts.TopP
	}
	if opts.TopK != nil {
		m["top_k"] = *opts.TopK
	}
	if opts.FrequencyPenalty != nil {
		m["frequency_penalty"] = *opts.FrequencyPenalty
	}
	if opts.PresencePenalty != nil {
		m["presence_penalty"] = *opts.PresencePenalty
	}
	if opts.ReasoningEffort != "" {
		m["reasoning_effort"] = opts.ReasoningEffort
	}
	if len(opts.Stop) > 0 {
		m["stop"] = opts.Stop
	}
	if len(opts.Tools) > 0 {
		m["tools"] = opts.Tools
	}
	return m
}

// ── 错误类型 ─────────────────────────────────────────────

type retryableError struct{ msg string }

func (e *retryableError) Error() string { return e.msg }

func isRetryable(err error) bool {
	_, ok := err.(*retryableError)
	return ok
}
