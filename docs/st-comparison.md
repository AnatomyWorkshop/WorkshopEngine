# WorkshopEngine vs SillyTavern 能力对比

> 编写于 2026-04-04
>
> 分析目的：明确 WorkshopEngine 作为无头运行时的边界，识别引擎层真实缺口，
> 确定下一步工作方向。

---

## 一、比较框架的前提

SillyTavern 是"前端 + 后端 + 插件生态"的一体化桌面应用，而 WorkshopEngine
是纯粹的 **无头 API 运行时**。直接比较会产生大量"ST 有 / 引擎没有"的误报。

正确的比较框架是三层分类：

| 类别 | 含义 | 是否引擎责任 |
|------|------|------------|
| **超出范围** | 纯前端渲染、桌面 UI、TTS/STT、图像生成集成 | ❌ 客户端层 |
| **创作/平台层** | 角色卡管理、角色库、标签分类、分享 | 🟡 creation / platform 层 |
| **引擎层差距** | Prompt 编排、上下文管理、会话结构、AI 后端对接 | ✅ 引擎核心责任 |

以下分析只关注**引擎层差距**。

---

## 二、WorkshopEngine 已经具备的引擎能力

| 能力 | ST 实现 | WorkshopEngine 实现 | 优劣 |
|------|---------|---------------------|------|
| 会话结构 | 线性 chat history | Session → Floor → Page 三层 | **WE 更强**：支持 Swipe 多页、Fork 平行时间线 |
| Preset / 作者注释 | Story String + Author's Note | PresetEntry（injection_order + position） | 对等 |
| 世界书 | WorldInfo（关键词触发，depth/scan） | WorldbookEntry（关键词/正则触发） | ST 有 depth/scan 深度控制，WE 缺这部分 |
| 记忆系统 | 无原生结构化记忆，依赖插件 | 时间衰减 + 整合 Worker + 维护策略 | **WE 显著更强** |
| 变量系统 | 无五层沙箱；有 macro 宏替换 | 五层变量沙箱（Page→Floor→Branch→Chat→Global） | **WE 更强** |
| LLM 采样参数 | Connection Profiles（per-profile 覆盖） | GenParams + ResolveSlot（5 级优先级） | 对等 |
| Tools / Function Calling | 通过扩展脚本，非原生 | 原生 Agentic Loop（3 内置工具，注册表可扩展） | **WE 更强**（原生） |
| SSE 流式 | ✅ | ✅ | 对等 |
| 角色卡解析 | TavernCardV2/V3（原生） | PNG 解析（chara_card_v2/v3） | 对等 |
| Prompt Dry-Run | 无 | `GET /sessions/:id/prompt-preview` | **WE 独有** |
| 多租户 | 单用户桌面 | X-Account-ID + ALLOW_ANONYMOUS | **WE 更强** |
| Session Fork | 无（只能复制整个对话） | `POST /sessions/:id/fork`（从任意楼层分叉） | **WE 独有** |
| LLM 响应结构化解析 | Regex 后处理（非结构化） | XML/JSON/Plaintext 三层回退 → 结构化 `ParsedResponse` | **WE 更强** |

---

## 三、SillyTavern 引擎层能力 — WorkshopEngine 缺口

### 🔴 优先级高（直接影响可用性）

#### 3.1 Prompt 格式模板（ChatML / Llama3 / Mistral / Alpaca / 自定义）

ST 支持十余种 chat 模板，把 `[{role: "user", content: "..."}]` 转换为模型原生期望的
字符串格式（`<|im_start|>user\n...<|im_end|>`）。

WorkshopEngine 目前只做 OpenAI messages 格式，无法对接**不支持标准 chat API 的
本地模型**（Ollama 原始模式、KoboldAI text-completion、vLLM 部分端点）。

**影响**：本地大模型（Qwen、DeepSeek、Yi 等）如果通过 text-completion 接口而非
chat-completion 接口暴露，引擎无法对接。

#### 3.2 Regex 脚本 / 输出后处理规则

ST 的 Regex 脚本允许对 AI 输入/输出做任意 find-replace，用途包括：
- 去除 AI 输出中的前缀套话（"Certainly! As an AI..."）
- 格式规范化（统一换行、去除星号动作描述）
- 标签注入（把 AI 输出中特定词汇替换为触发关键词）
- 隐私过滤

WorkshopEngine 目前在 `parser.go` 只做结构化解析，没有可配置的后处理规则管道。

#### 3.3 精确 Token 计数

ST 使用 tiktoken / HuggingFace 分词器做精确 token 计数，驱动上下文裁剪。
WorkshopEngine 使用粗估（`1 token ≈ 1.5 汉字`），实际 token 使用量可能大幅偏差。

**影响**：`TOKEN_BUDGET` 边界不准，可能导致上下文溢出或大量浪费。

---

### 🟡 优先级中（影响生态完整性）

#### 3.4 世界书深度控制（depth / scan_depth / position）

ST WorldInfo 每条词条有：
- `position`：在 prompt 中的插入位置（before char def / after char def / before example messages / at depth N in conversation）
- `scan_depth`：只扫描最近 N 条消息寻找触发关键词（而不是全文扫描）
- `group`：多个词条的互斥分组（同组最多触发 N 条）

WorkshopEngine 当前 WorldbookNode 只做全文扫描 + 关键词/正则触发，缺少
`depth` 扫描窗口和 `position` 精确插入控制。

#### 3.5 递归世界书激活（Recursion）

ST 支持被触发的词条内容本身成为新的关键词扫描源，触发更多词条（可配置递归深度）。
WorkshopEngine 目前只做单次扫描，无递归。

#### 3.6 服务端事件钩子 / 插件 API

ST 拥有完整的后端插件系统（metadata、事件监听、API 注册）。WorkshopEngine
没有任何 server-side hook 机制。

没有钩子，就无法构建：
- 第三方工具接入（SD 图像生成 trigger、TTS 触发）
- 外部状态同步（接入 Notion、GitHub 等）
- 自定义 context 变换器

#### 3.7 对话导入 / 导出（SillyTavern JSON 格式）

ST 有完整的 chat export（.jsonl、ST 私有格式）和 import（跨客户端恢复）。
WorkshopEngine 没有对话序列化接口（只能直接查 DB）。

---

### 🟢 优先级低（边际收益）

| 功能 | ST 有 | 备注 |
|------|-------|------|
| 多角色群聊（Group Chat） | ✅ | 引擎可以建模（多 session 或单 session 多角色），复杂度高 |
| Persona 管理（用户角色设定） | ✅ | 可通过 tmplCfg 字段间接实现 |
| Quick Reply（预设用户消息快捷键） | ✅ | 前端功能，引擎无需实现 |
| Message 移动 / 编辑历史 | ✅ | 可通过 Pages 覆盖实现，非刚需 |
| CFG（引导比例） | ✅ | 模型特定，仅部分后端支持 |
| Character Book 的 key/secondaryKey | ✅ | 可扩展 WorldbookEntry 表 |
| 角色卡标签 / 分类 | ✅ | 创作层，非引擎层 |

---

## 四、TavernHeadless 对比 — 我们尚未对齐的部分

TH 作为最接近的参考实现，以下是 TH 已有/规划但 WorkshopEngine 当前未实现的：

| 功能 | TH 状态 | WorkshopEngine 状态 | 差距评估 |
|------|---------|---------------------|---------|
| 多 Provider（Anthropic / Google / xAI） | ✅ 完整注册表 | ⚠️ 仅 OpenAI 兼容 | provider.go 框架已存在，需实现各家格式转换 |
| 完整 JWT / 账户映射 | ✅ | ⚠️ admin key + X-Account-ID | 中期目标，当前单机/小团队够用 |
| WorldInfo `depth` / `position` | ✅ | ⚠️ 仅全文扫描 | 优先级高 |
| Regex 预/后处理规则 | ✅ | ❌ | 优先级高 |
| MCP 协议接入 | 🔄 规划中 | ❌ | Tools 层已建好，MCP 是下一步 |
| 多 LLM 角色槽（director / verifier） | ✅ | ❌ | 中期目标 |
| 对话分支导出 | ✅ | ❌ | 中期 |
| 精确 tokenizer | ⚠️ 部分 | ❌ | 建议用 tiktoken-go 或 API 反馈值 |

---

## 五、引擎能做到"完整替代 SillyTavern 全部工作"吗？

**结论：不需要，也不应该。**

ST 是端到端的用户产品（UI + 桌面客户端 + 插件商店）。
WorkshopEngine 是 **基础设施层**，对应的是 ST 后端 API 的职责，
而非 ST 整体。

正确的类比关系：

```
WorkshopEngine          ≈  ST 的 server.js + routes/* + world-info engine
                           + chat engine + memory (if ST had one)

Creation Layer（待建）  ≈  ST 的角色卡管理 UI + WorldInfo 编辑器
Platform Layer（待建）  ≈  ST 的扩展商店 + 用户账户系统

WorkshopEngine 暂不覆盖  =  ST 的前端渲染、TTS、STT、SD 集成、扩展 UI 组件
```

WorkshopEngine 在以下方向上**超越了 ST 的服务端能力**：
- 结构化三层会话（Floor/Page/Fork）
- 五层变量沙箱
- 原生结构化 LLM 响应解析
- 多租户设计
- 原生 Agentic Tool Loop
- 记忆衰减系统

对"有 SillyTavern 插件生态"的需求，正确答案是接入 **MCP 协议**（工具调用标准化），
而非逐一复现 ST 插件系统的内部 API。

---

## 六、下一步工作方向建议

基于以上分析，推荐按以下优先级推进：

### 第一优先：补全 Prompt 编排能力

| 任务 | 内容 | 复杂度 |
|------|------|--------|
| **Regex 后处理规则** | `GameTemplate.Config` 配置 `output_transforms`（find/replace/regex），`parser.go` 后应用 | 低 |
| **WorldInfo scan_depth + position** | WorldbookEntry 增加 `scan_depth int`、`position string` 字段；node_worldbook.go 按 depth 窗口触发 | 低-中 |
| **WorldInfo 递归激活** | node_worldbook.go 二次扫描已激活词条内容 | 低 |

这三项完成后，Prompt 编排能力可以认为**完整覆盖** ST 的同类功能。

### 第二优先：精确 Token 管理

| 任务 | 内容 | 复杂度 |
|------|------|--------|
| **tiktoken-go 集成** | 引入 `tiktoken-go` 或用 API 返回的 `usage.prompt_tokens` 做反馈校准 | 低-中 |
| **智能上下文裁剪** | Pipeline Runner 超出预算时优先裁剪低优先级 History Block | 中 |

### 第三优先：MCP 协议接入

Tools 层已建好，MCP 是将其标准化的下一步。引擎对外暴露 `/tools/mcp` 端点，
接受标准 MCP 工具描述，自动注册到 Registry，使任意社区 MCP 服务器的工具
无需手写适配代码即可在游戏中使用。

### 中期目标（维持当前节奏）

```
多 LLM 角色槽（director / verifier）→ 多 Provider 注册表 → JWT Auth
```

---

## 七、一句话定位

> WorkshopEngine 是"比 ST 服务端更有结构的 AI 对话运行时"。
> 它不复制 ST 的插件生态，而是通过 MCP 标准接入整个工具生态。
> 它不是 ST 的替代品，而是 ST 想做但受限于单体架构没做成的东西的 API 化版本。
