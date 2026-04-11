package llm

import "context"

// Provider 统一 LLM 调用接口。
// 所有 provider 实现此接口，上层代码不关心底层 API 差异。
type Provider interface {
	// Chat 非流式调用，返回完整响应。
	Chat(ctx context.Context, messages []Message, opts Options) (*Response, error)

	// ChatStream 流式调用，返回 token/usage/error 三个 channel。
	ChatStream(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan Usage, <-chan error)

	// ID 返回 provider 标识（如 "openai-compatible"、"anthropic"）。
	ID() string
}
