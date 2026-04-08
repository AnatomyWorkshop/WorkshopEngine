# WorkshopEngine — 社区边缘大模型接入愿景

> 版本：2026-04-08
> 背景：WE 的 LLM 接入层当前基于 OpenAI 兼容协议，天然适合接入任何社区托管的模型端点。

---

## 核心洞察

WE 的 LLM 客户端从第一天起就设计为 `(baseURL, apiKey, model)` 三元组驱动，而不是绑定某家供应商。这意味着只要有一个 OpenAI 兼容的 `/v1/chat/completions` 端点，WE 就能调用。社区自托管模型（Ollama、LMDeploy、vLLM、text-generation-webui）均属于这个范畴。

更进一步：**WE 的多槽 LLM 架构**（Director 槽 + Verifier 槽 + Narrator 槽 + Memory 槽）天然适合"廉价本地模型做后台分析，高质量远端模型做叙事生成"的混合部署模式。

---

## 接入方向

### 1. 零成本接入（当前已支持）

任何用户只需在创作层配置一个 LLM Profile，填入：
- `base_url`：社区端点地址（如 `http://localhost:11434/v1`）
- `api_key`：社区 API Key（或空字符串）
- `model`：社区模型 ID（如 `deepseek-r1:8b`、`qwen3:14b`）

WE 立即可用，无需代码修改。

**典型场景：**
- 玩家在自己的 PC 上跑 Ollama，让 WE 连接 `localhost:11434`
- 社区服务器部署了 vLLM 集群，共享端点给成员
- 小型创作团队在内网部署了专门为 RP 调优的模型

---

### 2. 多槽经济模型（近期可做）

WE 的槽位系统让"廉价模型 + 高质量模型"混合使用成为一等公民：

```
Director 槽  → 社区轻量模型（0.5B–4B）  ← 上下文分析、剧情方向预判
Memory 槽    → 社区轻量模型（0.5B–4B）  ← 记忆整合摘要
Verifier 槽  → 社区轻量模型（0.5B–4B）  ← 一致性校验
Narrator 槽  → 高质量模型（社区 RP 专模型 / 云端旗舰）← 实际生成
```

**经济效益：** 一次完整回合中，只有 Narrator 槽调用高质量模型（产生主要费用），其他槽可以完全由社区免费端点承担。创作者可以在游戏包的 `llm_profiles` 字段中声明推荐的槽位配置，玩家按需覆盖。

---

### 3. 游戏包内的模型推荐（近期规划）

`game-package.json` 的 `config` 字段扩展 `recommended_llm_profiles`：

```json
{
  "config": {
    "recommended_llm_profiles": [
      {
        "slot": "narrator",
        "hint": "需要支持中文 + 创意写作的模型，推荐 deepseek-v3 或 qwen3-72b",
        "min_context_tokens": 32000
      },
      {
        "slot": "director",
        "hint": "轻量分析模型即可，推荐 qwen3-4b 或 gemma3-4b",
        "min_context_tokens": 8000
      }
    ]
  }
}
```

WE 前端在创建会话时展示此推荐，降低玩家配置门槛。

---

### 4. 社区 API 注册表（中期愿景）

**问题：** 玩家不知道有哪些社区端点可用，也不知道哪些端点质量好、延迟低。

**方案：** WE 可选维护一个轻量的社区 LLM 注册表服务（与 WE 引擎完全解耦，独立部署）：

```
GET  /registry/providers          列出已注册的社区端点
GET  /registry/providers/:id      端点详情（模型列表 + 统计 + 标签）
POST /registry/providers          注册新端点（自愿加入）
POST /registry/providers/:id/ping 触发一次延迟测试
```

注册表只做发现和质量统计，不代理流量。WE 引擎在"选择端点"时可以查询注册表，玩家看到一个"社区端点浏览器"而不是一个空白表单。

**隐私优先设计：**
- 注册表不记录任何游玩内容，只统计端点可用性和延迟
- 支持私有端点（不对外注册，仅团队内部使用）
- 注册是完全自愿的；WE 引擎在离线/私有部署时完全不依赖注册表

---

### 5. 专属 RP 模型适配（中期研究）

社区中已有专门为角色扮演微调的模型（如各类 NSFW/SFW 微调模型、叙事写作专项模型）。这些模型往往：
- 不完全兼容 OpenAI 的 system/user/assistant 格式
- 有自己的特殊 instruct 格式（如 Alpaca / ChatML / Vicuna 格式变体）
- 在某些创作领域（角色情感细腻度、世界描写风格）远超通用模型

**WE 适配路径（P-4C 扩展方向）：**

```go
// internal/platform/provider 扩展
type ProviderAdapter interface {
    FormatMessages(msgs []Message) any   // 转换为 provider 特定格式
    ParseResponse(raw any) (string, error)
    StreamResponse(ctx context.Context, req StreamRequest) (<-chan string, error)
}

// 已有
type OpenAICompatAdapter struct{}   // 当前唯一实现

// 计划
type OllamaAdapter      struct{}    // Ollama 原生 API（更丰富的元数据）
type AnthropicAdapter   struct{}    // Anthropic 原生（P-4C）
type CommunityAdapter   struct {    // 自定义 instruct 格式
    SystemTemplate  string          // e.g., "<|im_start|>system\n{{system}}<|im_end|>"
    UserTemplate    string          // e.g., "<|im_start|>user\n{{user}}<|im_end|>"
    AssistantPrefix string          // e.g., "<|im_start|>assistant\n"
}
```

`CommunityAdapter` 让创作者在游戏包中声明目标模型的 instruct 格式，WE 自动转换消息列表。这消除了"模型明明支持该语言/风格，但因为格式不对而输出降质"的问题。

---

### 6. 本地推理隐私保护（长期定位）

RP 游戏涉及大量用户个人偏好和创作内容，很多玩家不愿意将对话发送到外部云端。WE 的本地部署能力（单二进制 + SQLite 模式）已经支持完全离线运行；与 Ollama 等本地推理框架结合后，整个游玩体验可以完全发生在玩家设备上：

```
玩家设备
├── WE 引擎（Go 二进制，~20MB）
├── SQLite 数据库（游玩存档）
├── Ollama（本地推理）
│   └── qwen3-14b-q4（~8GB，支持 4K 上下文）
└── WE 前端（Vue SPA，~2MB）
```

这个配置不产生任何外部流量，完全本地运行，适合：
- 对内容隐私有高要求的用户
- 无网络环境（离线游玩）
- 希望 100% 控制游玩数据的创作者/玩家

---

## 技术路径总结

| 阶段 | 内容 | 对应计划条目 |
|------|------|------------|
| **已支持** | OpenAI compat 任意端点（Ollama/vLLM/社区 API）| M1 基础设施 |
| **已支持** | 多槽经济模型（Director/Memory 用廉价端点）| M8 多槽 LLM |
| **近期（P-4C）** | Anthropic 原生 API 适配 | Phase 4 |
| **近期** | 游戏包内声明推荐 LLM Profile | 扩展 game-package.json |
| **中期** | OllamaAdapter + CommunityAdapter（自定义 instruct 格式）| P-4C 扩展 |
| **中期** | 社区 LLM 注册表（独立微服务）| 新项目 |
| **长期** | 完全本地推理套件（WE + Ollama + 前端一体包）| 打包方案 |

---

## 设计原则

1. **引擎不绑定供应商。** `(baseURL, apiKey, model)` 三元组是唯一接口契约，无论背后是 OpenAI、Anthropic、Ollama 还是社区微服务。

2. **廉价模型是一等公民。** Director / Verifier / Memory 槽的存在让廉价本地模型参与每一次生成，而不是只有"买不起旗舰模型"时才用。这是架构设计，不是降级方案。

3. **隐私保护是可选路径，不是额外功能。** 完全本地运行应该是零配置可达的，而不是需要玩家手动绕过云端强制流量的"高级设置"。

4. **注册表是发现工具，不是控制节点。** 社区端点的价值由社区评价，WE 引擎在注册表不可用时应完全正常运行。
