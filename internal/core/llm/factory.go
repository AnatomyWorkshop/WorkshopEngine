package llm

// NewProvider 根据 provider type 创建对应的 Provider 实现。
// openai/openai-compatible/deepseek/xai 全部走 OpenAI 兼容客户端。
func NewProvider(providerType, baseURL, apiKey, model string, timeoutSec, maxRetries int) Provider {
	switch providerType {
	case "anthropic":
		return NewAnthropicClient(baseURL, apiKey, model, timeoutSec, maxRetries)
	default:
		return NewClient(baseURL, apiKey, model, timeoutSec, maxRetries)
	}
}
