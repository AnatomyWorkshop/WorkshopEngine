# P-4C 多 Provider 抽象设计

> 日期：2026-04-10
> 状态：设计阶段，准备实现

---

## 现状

`internal/core/llm/client.go` 是一个硬编码的 OpenAI 兼容客户端：
- 固定 `/chat/completions` 端点
- 固定 `Authorization: Bearer` 鉴权
- 固定 OpenAI SSE 格式解析
- 固定 `tool_calls` 工具调用格式

这意味着 Anthropic（`x-api-key` 头 + `content_block_delta` 流 + `tool_use` 块）和 Google Gemini（`x-goog-api-key` + `functionCalls`）无法直接接入。

TH 通过 Vercel AI SDK 一行代码切换 provider。WE 是 Go，没有等价 SDK，需要自己做抽象层。

---

## 设计原则

1. **现有 `llm.Client` 不动**——它已经是一个完善的 OpenAI 兼容客户端，继续作为默认实现
2. **新增 `Provider` 接口**——抽象 Chat/Stream/DiscoverModels 三个操作
3. **OpenAI 兼容 = 零成本**——现有 client 直接包装为 Provider，不重写
4. **非 OpenAI provider 按需实现**——先 Anthropic，再 Google，每个 ~150 行
5. **本地模型（Ollama/vLLM）天然支持**——它们都是 OpenAI 兼容 API

---

## 接口设计

```go
// internal/core/llm/provider.go（新增）

// Provider 统一 LLM 调用接口。
// 所有 provider 实现此接口，上层代码不关心底层 API 差异。
type Provider interface {
    // Chat 非流式调用，返回完整响应。
    Chat(ctx context.Context, messages []Message, opts Options) (*Response, error)

    // ChatStream 流式调用，返回 token/usage/error 三个 channel。
    ChatStream(ctx context.Context, messages []Message, opts Options) (<-chan string, <-chan Usage, <-chan error)

    // ID 返回 provider 标识（如 "openai"、"anthropic"）。
    ID() string
}
```

### 为什么不改 `Client` 的签名

`Client` 已经满足 `Provider` 接口（`Chat` + `ChatStream` 签名完全匹配），只需加一个 `ID() string` 方法。这意味着：
- 所有现有调用点零改动
- `Client` 本身就是 `openai-compatible` Provider 的实现
- 新 provider 只需实现同样的三个方法

---

## Provider 实现矩阵

| Provider | 鉴权 | 端点 | 流格式 | 工具调用格式 | 实现方式 |
|----------|------|------|--------|-------------|---------|
| `openai` | `Bearer` | `/chat/completions` | SSE `data:` | `tool_calls[]` | 现有 `Client` |
| `openai-compatible` | `Bearer` | `/chat/completions` | SSE `data:` | `tool_calls[]` | 现有 `Client` |
| `deepseek` | `Bearer` | `/chat/completions` | SSE `data:` | `tool_calls[]` | 现有 `Client`（baseURL 不同） |
| `xai` | `Bearer` | `/chat/completions` | SSE `data:` | `tool_calls[]` | 现有 `Client`（baseURL 不同） |
| `anthropic` | `x-api-key` | `/v1/messages` | SSE `event:` 类型 | `tool_use` content block | 新增 `anthropic.go` |
| `google` | `x-goog-api-key` | `/v1beta/models/:model:generateContent` | SSE | `functionCalls` | 新增 `google.go` |
| `ollama` | 无 | `/api/chat` 或 `/v1/chat/completions` | SSE | `tool_calls[]` | 现有 `Client`（Ollama 兼容 OpenAI） |

**关键洞察：** 6/7 种 provider 都是 OpenAI 兼容的。只有 Anthropic 和 Google 需要独立实现。

---

## 文件结构

```
internal/core/llm/
├── provider.go          ← Provider 接口定义（新增，~20 行）
├── client.go            ← 现有 OpenAI 兼容客户端（加 ID() 方法，改动 1 行）
├── anthropic.go         ← Anthropic Messages API 客户端（新增，~150 行）
├── google.go            ← Google Gemini API 客户端（新增，~150 行，可延后）
└── factory.go           ← NewProvider(type, baseURL, apiKey, ...) 工厂函数（新增，~30 行）
```

### `factory.go`

```go
// NewProvider 根据 provider type 创建对应的 Provider 实现。
func NewProvider(providerType, baseURL, apiKey, model string, timeoutSec, maxRetries int) Provider {
    switch providerType {
    case "anthropic":
        return NewAnthropicClient(baseURL, apiKey, model, timeoutSec, maxRetries)
    case "google":
        return NewGoogleClient(baseURL, apiKey, model, timeoutSec, maxRetries)
    default:
        // openai, openai-compatible, deepseek, xai, ollama 全部走 OpenAI 兼容
        return NewClient(baseURL, apiKey, model, timeoutSec, maxRetries)
    }
}
```

---

## Anthropic 适配要点

Anthropic Messages API 与 OpenAI 的核心差异：

| 差异 | OpenAI | Anthropic |
|------|--------|-----------|
| 鉴权头 | `Authorization: Bearer` | `x-api-key` + `anthropic-version: 2023-06-01` |
| 端点 | `/chat/completions` | `/v1/messages` |
| System prompt | `messages[0].role=system` | 顶层 `system` 字段 |
| 响应格式 | `choices[0].message.content` | `content[0].text` |
| 流事件 | `data: {"choices":[{"delta":{"content":"..."}}]}` | `event: content_block_delta` + `data: {"delta":{"text":"..."}}` |
| 工具调用 | `tool_calls[{id, function:{name, arguments}}]` | `content[{type:"tool_use", id, name, input:}]` |
| 工具结果 | `role: "tool", tool_call_id, content` | `role: "user", content:[{type:"tool_result", tool_use_id, content}]` |
| Token 用量 | `usage.prompt_tokens` | `usage.input_tokens` |
| Max tokens | `max_tokens`（可选） | `max_tokens`（必填） |

**实现策略：** `anthropic.go` 内部做消息格式转换（`[]llm.Message` → Anthropic 格式），对外暴露完全相同的 `Provider` 接口。调用方无感知。

---

## 上层改动

### `provider/registry.go`

`ResolveForSlot` 当前直接调 `llm.NewClient`。改为调 `llm.NewProvider`：

```go
// 改动前
profileClient := llm.NewClient(baseURL, apiKey, row.ModelID, timeoutSec, maxRetries)

// 改动后
profileClient := llm.NewProvider(row.ProviderType, baseURL, apiKey, row.ModelID, timeoutSec, maxRetries)
```

`ResolveForSlot` 返回类型从 `*llm.Client` 改为 `llm.Provider`。

### `game_loop.go` / `engine_methods.go`

所有 `*llm.Client` 引用改为 `llm.Provider`。由于 `Provider` 接口的 `Chat`/`ChatStream` 签名与 `Client` 完全一致，调用点代码不变，只改类型声明。

### `LLMProfile` 模型

`Provider` 字段已存在（`default:'openai-compatible'`），无需改 schema。`ResolveForSlot` SQL 已读取 `p.provider`，只需传给 `NewProvider`。

---

## 实现计划

### Phase 1（本次做）：接口 + 工厂 + Anthropic

1. 新增 `provider.go`（接口定义）
2. `Client` 加 `ID() string` 方法
3. 新增 `factory.go`（工厂函数）
4. 新增 `anthropic.go`（Anthropic Messages API）
5. `provider/registry.go` 改用 `llm.Provider` 接口
6. `game_loop.go` / `engine_methods.go` 类型声明改为 `llm.Provider`

### Phase 2（延后）：Google Gemini

- 新增 `google.go`
- `factory.go` 加 `case "google"`

### Phase 3（延后）：高级特性

- Provider 级别的 token 计数器（不同 provider 的 tokenizer 不同）
- Provider 能力声明（是否支持 tool_calls、vision、reasoning_effort）
- 自动 fallback（主 provider 失败时切换备用）

---

## 本地模型支持

Ollama、vLLM、LM Studio、LocalAI 全部暴露 OpenAI 兼容 API：

| 本地方案 | 默认端点 | 兼容性 |
|---------|---------|--------|
| Ollama | `http://localhost:11434/v1` | 完全兼容 |
| vLLM | `http://localhost:8000/v1` | 完全兼容 |
| LM Studio | `http://localhost:1234/v1` | 完全兼容 |
| LocalAI | `http://localhost:8080/v1` | 完全兼容 |

用户只需在 LLM Profile 中设置 `provider: "openai-compatible"` + `base_url: "http://localhost:11434/v1"` + `api_key: "ollama"`（Ollama 不校验 key 但字段必填）。

**无需任何额外代码。** 现有 `Client` 已经支持。

---

## 与 TH 的差异

| 维度 | TH | WE |
|------|----|----|
| 抽象层 | Vercel AI SDK（TypeScript 生态） | 自建 `Provider` 接口（Go） |
| Provider 数量 | 5（openai/anthropic/google/deepseek/xai） | 同等（openai-compat 覆盖 4 个 + anthropic 独立） |
| 工具调用 | Vercel AI SDK 统一 | 各 Provider 内部转换为 `llm.ToolCall` |
| 流式 | Vercel AI SDK `textStream` | 各 Provider 内部转换为 `chan string` |
| 本地模型 | 需要 OpenAI-compat 代理 | 同（Ollama 等天然兼容） |

**WE 的优势：** 不依赖第三方 SDK，每个 provider 实现完全可控，可以针对特定 provider 做深度优化（如 Anthropic 的 prompt caching、Google 的 grounding）。
