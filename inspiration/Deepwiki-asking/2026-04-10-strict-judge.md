Let me read the key files in depth before giving a rigorous assessment.

基于对全部文档的完整阅读，以下是严苛评委视角的评分。

---

## 一、作为后端引擎架构

### 1.1 架构设计（7/10）

**得分点：**

`core → platform → engine → social/creation` 的单向依赖规则是真正执行的，不是写在文档里的空话。`engine` 不导入 `creation`，`social` 不导入 `engine` 内部包，这个边界在代码层面有实际约束。 [1](#1-0) 

PromptBlock Priority IR 是这个引擎最有价值的设计决策之一。每个节点只产出带 Priority 整数的 Block，Runner 统一排序，这比 TH 的位置枚举（`before_system`/`after_world`/`at_depth`）更灵活——任何节点可以产出任意 Priority 的 Block，新增节点不需要修改 Runner。 [2](#1-1) 

**扣分点：**

`game_loop.go` 既组装 Prompt 又调用 LLM，单文件职责过重，文档自己承认"测试覆盖困难"，Phase 4 才拆分。这是一个已知的设计债务，不是未来风险，是现在的问题。 [3](#1-2) 

DB 模型全部集中在 `core/db/models.go`，修改任何模型导致所有包重新编译。这在项目规模增大后会成为开发效率瓶颈。 [4](#1-3) 

### 1.2 运行时可靠性（5/10）

**严重扣分：**

Background Job 是 goroutine 内存级，进程重启丢失任务。这不是"低概率"问题，而是任何生产部署都会遇到的确定性问题（服务器重启、OOM kill、部署更新）。M19 才修复，但 M19 在 Phase 4，路线图上没有明确时间节点。 [5](#1-4) 

并发保护是 `generating bool` 字段 + `SELECT ... FOR UPDATE`，只有 `reject` 模式，没有 `queue` 模式。TH 的 RuntimeRevisionGuard 有乐观锁 + CAS 写入保护，WE 的方案在高并发场景下是粗暴的拒绝而非优雅的排队。 [6](#1-5) 

### 1.3 可扩展性（7.5/10）

工具注册表（`Tool` 接口 + `Registry`）设计干净，新增工具不需要修改 Agentic Loop。Pipeline 节点扩展点清晰，实现 `Build(ctx) []PromptBlock` 接口即可。 [7](#1-6) 

扣分：宏系统当前是单一函数 `Expand()`，宏集合硬编码在代码中，Phase 5-C 才重构为注册表。现在第三方无法扩展宏，这与"可扩展引擎"的定位矛盾。 [8](#1-7) 

### 1.4 Token 精度（5/10）

Token 计数是 BPE 兼容启发式估算（ASCII ÷4，CJK ×⅔），误差 ±15%。对于 Token Budget 裁剪这种精度敏感的操作，±15% 的误差意味着实际上下文可能比预算多出 15%，或者浪费 15% 的可用窗口。TH 使用 provider-specific 分词器。 [9](#1-8) 

---

## 二、作为面向市场的完整商品底座

### 2.1 差异化产品能力（8/10）

`game-package.json` 是真正的差异化，不是功能堆砌。一个 JSON 文件打包 Preset + Worldbook + Regex + 素材，`POST /templates/import` 一步导入，这是 TH 没有的，ST 的角色卡 PNG 也只含角色信息。这个设计让"游戏发布"成为一等公民操作。 [10](#1-9) 

VN 指令解析（`[bg|...]`、`[sprite|...]`、`[choice|A|B|C]`）、素材库 + `search_material`、ScheduledTurn 三者组合，构成了一个其他引擎没有的"内容运营"能力层。 [11](#1-10) 

### 2.2 安全与合规（2/10）

这是最严重的问题，不是"技术债务"，是**上线阻断项**：

- `LLMProfile.api_key` 明文存储在 PostgreSQL，文档自己标注"生产应加密"
- `X-Account-ID` 无签名，任何人可伪造，没有 JWT
- 这两个问题在 Phase 4 才修复（P-4A、P-4B） [12](#1-11) 

一个面向市场的商品底座，在 API Key 明文存储和身份认证可伪造的状态下，不能上线。这不是评分扣分，这是硬性阻断。

### 2.3 开发者生态（4/10）

- 无 OpenAPI 文档（P-4D），第三方开发者只能读源码
- 无官方 TypeScript SDK（P-5B），前端集成需要手写 HTTP 客户端
- 无对话导入/导出（P-4E），玩家数据无法迁移
- 多 Provider 只支持 OpenAI compat，Anthropic/Google 原生不支持（P-4C） [13](#1-12) 

这些不是"锦上添花"，是商品底座的基础设施。没有 SDK 和文档，第三方开发者接入成本极高。

### 2.4 平台完整性（6/10）

社交层（论坛/评论/点赞）已实现，CW+GW 双端架构设计清晰，创作者发布流程（draft → published）完整。但 VN 资产系统（P-4I Stage B/C）未完成，`rich` 类型游戏的立绘/背景图/BGM 无法完整运行，只有文字占位。 [14](#1-13) 

---

## 三、和成熟商品对打

### 3.1 对比 TavernHeadless（同类竞品）

```
WE 领先：
  game-package.json（TH 无）
  VN 指令解析（TH 无）
  素材库 + search_material（TH 无）
  ScheduledTurn（TH 无）
  creation-agent（TH 无）
  记忆时间衰减（TH 只有静态 importance）
  Worldbook 变量门控（TH 无）

TH 领先：
  MCP 协议（完整 McpConnectionManager，stdio + HTTP）
  Background Job Runtime（DB 持久化，进程重启安全）
  双层记忆压缩（micro/macro）
  JWT Auth + API Key AES-256-GCM 加密
  官方 Integration Kit（@tavern/sdk + @tavern/client-helpers）
  OpenAPI 文档
  多 Provider 原生适配（Anthropic/Google/xAI）
  Floor Run State 精细状态机
``` [11](#1-10) 

**评分：6.5/10**。WE 在游戏发布平台方向有真实的差异化，但工程基础设施（安全、持久化、文档、SDK）落后 TH 约一个 Phase。TH 是 Beta3 封板状态，WE 是 Phase 3 完成状态，差距是真实存在的。

### 3.2 对比 SillyTavern（生态竞品）

ST 有庞大的插件生态（数百个社区插件）、完整的宏系统、本地运行的隐私优势。WE 的宏系统只实现了基础 6 个宏，`setvar`/`addvar`/`lastMessage` 等未实现，Phase 5-C 才完整。 [15](#1-14) 

WE 的定位是"无头引擎 + 游戏发布平台"，不是 ST 的替代品。但如果创作者从 ST 迁移过来，宏系统的不完整会是摩擦点。**评分：不直接竞争，但生态差距巨大。**

### 3.3 对比商业消费者产品（Character.ai、NovelAI）

WE 是开发者工具/平台底座，不是直接面向消费者的产品。没有用户注册系统、支付、内容审核、推荐算法。这不是缺陷，是定位选择。但如果创作者想在 WE 上运营一个面向大众的平台，这些能力需要自己在 WE 之上构建。**评分：定位不同，不宜直接对比。**

---

## 四、MCP 接口方向是否清晰

**不清晰，评分：3/10**

路线图中 P-3H 的状态是"暂缓"，触发条件是"创作者需要接入本地 MCP 工具（文件系统、代码执行）时再做"。这个触发条件是被动的、模糊的。 [16](#1-15) 

文档中对 MCP 的描述停留在"Tools 层已建好，MCP 是自然的下一步"，但没有：
- 具体的接口设计（stdio vs Streamable HTTP 传输协议选择）
- 安全模型（MCP 工具的授权/沙箱机制）
- 与现有 Preset Tool（HTTP 回调）的边界划分
- 进入正式里程碑的条件

TH 的 MCP 实现（`McpConnectionManager`，stdio + HTTP 双传输，Deferred 执行需用户授权）是完整的参照，但 WE 只有"参照 TH 实现"这一句话。 [17](#1-16) 

**扩展设计思路：** 如果要做，最小路径是在现有 `Tool` 接口上增加 `MCPToolProvider`，实现 `stdio` 传输（本地工具）和 `Streamable HTTP` 传输（远端工具），复用现有 `Registry.RegisterProvider()` 机制。安全上需要参考 TH 的 Deferred 执行模型——MCP 工具调用需要用户确认后才执行，对应现有的 `ReplaySafety` 分级扩展一个 `require_user_confirm` 等级。

---

## 五、RAG 方向是否清晰

**方向清晰，但优先级模糊，评分：6/10**

技术方案已经设计完整：`pgvector` 扩展，`memories` 表加 `embedding vector(1536)` 列，`ivfflat` 索引，`<=>` 余弦相似度检索。这不是空想，是可以直接执行的 SQL 和 Go 代码。 [18](#1-17) 

但问题在于：
1. 没有进入正式里程碑（M1-M23 中没有 RAG 条目）
2. 触发条件是"常驻角色 Phase 2 时引入"，而"常驻角色"本身也没有进入路线图
3. 当前记忆系统（`importance × decay`）对短期游戏（<50 回合）已经足够，RAG 的价值在长期角色（>500 回合）场景，而这个场景目前没有用户验证 [19](#1-18) 

**扩展设计思路：** RAG 的正确接入点是 `memory/store.go` 的 `GetForInjection()` 方法，在现有 `importance × decay` 排序之上增加一个 `mode` 参数：`mode=semantic` 时调用 embedding API 向量化当前用户输入，用 `pgvector` 检索语义最近的 Top-K 记忆，替代时间衰减排序。两种模式可以共存，由 `GameTemplate.Config.memory_retrieval_mode` 控制，不破坏现有行为。

---

## 总评

```
维度                        得分    主要问题
─────────────────────────────────────────────────────
后端引擎架构设计              7/10   game_loop.go 职责过重，宏系统不可扩展
后端引擎运行时可靠性          5/10   Background Job 不持久化，Token 计数不精确
面向市场安全合规              2/10   API Key 明文，Auth 可伪造（上线阻断项）
面向市场产品差异化            8/10   game-package、VN、素材库、ScheduledTurn 真实差异化
面向市场开发者生态            4/10   无 SDK、无文档、无多 Provider
对比 TH 竞争力               6.5/10  游戏平台方向领先，工程基础设施落后
MCP 接口方向清晰度            3/10   暂缓状态，无具体设计
RAG 方向清晰度               6/10   技术方案清晰，优先级和触发条件模糊
```

最核心的问题不是功能缺失，而是**安全基础设施（P-4A API Key 加密、P-4B JWT）必须在任何商业化动作之前完成**，这两项是硬性前置条件，不是可以并行推进的优化项。 [20](#1-19)

### Citations

**File:** docs/architecture.md (L86-101)
```markdown
**严格的单向依赖规则：**

```
core  ←  platform  ←  engine  ←  social
                   ←  creation
                   ←  integration
```

- `engine` 不能导入 `creation` 或 `social`
- `creation` 不能导入 `engine/api`
- `social` 只能通过 HTTP 调用 engine（不能导入 engine 内部包）

**为什么这样分？**

引擎（engine）是"游戏运行时"，创作（creation）是"资源管理"。如果 engine 导入 creation，就无法独立部署引擎、无法为创作层单独扩容，也无法把引擎打包给第三方使用。保持这个边界让 WE 引擎在未来可以作为独立 Go 模块分发。

```

**File:** docs/architecture.md (L104-132)
```markdown
## 3. 决策：PromptBlock Priority IR

**问题：** Prompt 组装涉及多个来源（System Prompt、世界书词条、记忆、历史对话、预设条目），插入位置和顺序复杂，每个 Pipeline 节点各自决定插入位置，难以统一调整。

**选择：** 每个 Pipeline 节点产出一组带 `Priority` 整数标签的 `PromptBlock`，Runner 最后统一按 Priority 全局排序，输出最终消息列表。

```
Priority 映射约定：
  0–99   = before_template（高优系统提示）
  100–199 = template（角色/游戏系统提示）
  200–299 = worldbook entries（世界书注入）
  300–399 = memories（记忆注入）
  400–499 = history（对话历史）
  500–599 = after_template（预设末尾条目）
  600+    = user turn
```

**理由：**
- 创作者只需理解一个数字（Priority），不需要理解 Pipeline 节点执行顺序
- 任何节点可以产出任意 Priority 的 Block，即 WorldbookNode 可以产出 Priority=50 的"钉住在最前面的词条"，无需特殊处理
- 新增节点只需实现 `Build(ctx) []PromptBlock` 接口，Runner 无需修改
- Token Budget 裁剪可以操作 Block 列表（按 Priority 排序后贪心保留），与组装逻辑完全解耦

**代价：**
- Priority 数字是约定，不是类型系统保证；如果两个节点用了重叠的 Priority 值，顺序依赖 slice 稳定性
- 调试时需要查看全部 Block 的 Priority 排序结果，才能看出"最终送给 LLM 的消息是什么顺序"
- 解决：`GET /sessions/:id/prompt-preview` 干运行端点，返回完整 Block 列表（含 Priority 和来源节点）

**参照：** TH 用 position 字段（`before_system`/`after_world`/`at_depth` 等枚举）描述插入位置，含义更直观但扩展性较弱——加一个新位置需要修改枚举定义。WE 的 Priority 数字更灵活，但需要文档约定来避免歧义。
```

**File:** docs/architecture.md (L167-195)
```markdown
## 5. 决策：game-package.json 游戏包格式

**问题：** 一个 AI 游戏由多个资源组成（角色、预设条目、世界书词条、Regex 规则、素材文件），如何让创作者一键分享、一键部署？

**选择：** 定义 `game-package.json` 格式，把一个游戏的所有资源序列化为单个 JSON 文件。导出时打包，导入时解包并重新建立关联。

```json
{
  "version": "1.0",
  "template": { ...GameTemplate fields },
  "character": { ...CharacterCard fields },
  "preset_entries": [...],
  "worldbook_entries": [...],
  "regex_rules": [...],
  "materials": [...],
  "llm_profile_hints": [...]
}
```

**理由：**
- 创作者只需要分享一个文件，接收方 `POST /templates/import` 即可复现完整游戏环境
- 与 ST 的角色卡 PNG（只含角色信息）不同，game-package 包含游戏运行所需的全部规则，是 WE 的核心差异化
- 版本字段允许未来格式演进时的迁移处理
- `llm_profile_hints` 是建议字段（不是强制），创作者可以指定推荐的模型规格，平台方可以忽略

**代价：**
- 如果游戏包含大量素材（图片、音频），单个 JSON 文件不适合；当前素材仅存储元数据（标签、描述），文件本体在素材库另存
- 导入时 ID 需要重新生成（不能直接用原 ID，可能冲突），外键关系需要 ID 映射表重建

```

**File:** docs/architecture.md (L225-238)
```markdown
**宏展开的未来规划（Phase 5-C）：**

当前 `Expand()` 是单一函数，宏集合写死在代码中。Phase 5-C.1 将重构为注册表模式：

```go
// 目标接口（尚未实现）
var DefaultRegistry = NewRegistry()
DefaultRegistry.Register("char",       func(ctx MacroContext, _ string) string { return ctx.CharName })
DefaultRegistry.Register("getvar",     func(ctx MacroContext, key string) string { ... })
DefaultRegistry.Register("roll",       func(ctx MacroContext, expr string) string { ... })
// 第三方插件可调用 Register() 扩充宏集合
```

这样第三方可以不修改核心代码地注册自定义宏，与 ST 宏扩展生态对齐。
```

**File:** docs/architecture.md (L402-402)
```markdown
| runtime_job 持久化 | goroutine（进程重启丢失）| M19 |
```

**File:** docs/architecture.md (L422-428)
```markdown
| 债务 | 位置 | 影响 | 计划 |
|------|------|------|------|
| DB 模型定义在 `core/db/`，与 engine/creation 业务逻辑分离不彻底 | `internal/core/db/models.go` | 修改模型需要改 core 包，导致所有包重新编译 | Phase 4 重构时按领域分拆 |
| `engine/api/game_loop.go` 既组装 Prompt 又调用 LLM，单文件职责偏重 | `internal/engine/api/game_loop.go` | 不影响功能，测试覆盖困难 | Phase 4 拆分为 Runner + API 两层 |
| 异步 Memory Worker 用 goroutine，进程重启丢失 | `internal/engine/memory/worker.go` | 低概率丢失整合任务 | M19 runtime_job 表 |
| `X-Account-ID` 认证无加密，可伪造 | `internal/platform/auth/` | 内测可接受，上线前必须修复 | M16 JWT Auth |

```

**File:** docs/architecture.md (L429-437)
```markdown
### 13.2 扩展点

新增 Pipeline 节点：实现 `Build(ctx ContextData) []PromptBlock` 并在 `RunPipeline()` 中注册即可，其他节点不受影响。

新增工具：实现 `Tool` 接口（`Definition()` + `Execute()`），调用 `registry.Register()` 注册，无需修改 Agentic Loop。

新增宏（Phase 5-C.1 后）：调用 `macros.DefaultRegistry.Register(name, handler)` 注册，无需修改 `Expand()`。

新增 API 路由：实现 Handler 函数，在 `cmd/server/main.go` 挂载到对应命名空间（`/api/play` 或 `/api/create`）。
```

**File:** inspiration/PLAN/P-WE-PROGRESS.md (L313-325)
```markdown

防止并发 PlayTurn 产生竞态（两个请求同时 commit 同一 Floor 的情况）。

**完成内容：**
- `GameSession` 新增 `Generating bool`（默认 false）+ `GenerationMode string`（默认 `"reject"`）字段（GORM AutoMigrate）
- `session.ErrConcurrentGeneration`：导出错误哨兵值，供路由层检测
- `session.Manager.StartTurn`：改写为 DB 事务 + `SELECT ... FOR UPDATE` 锁住 session 行，检查 `generating` 后原子设置为 `true`；`reject` 模式下返回 `ErrConcurrentGeneration`
- `session.Manager.ClearGenerating`：专用复位方法，在 CommitTurn / FailTurn 后调用
- `game_loop.go`：`PlayTurn` 的 regen 分支增加 `generating` 检查 + 设置；pipeline 失败 / LLM 失败 / CommitTurn 失败 / 成功均调用 `ClearGenerating`
- `engine_methods.go`：`StreamTurn` 的所有失败路径（工具循环、流式错误、context cancel）和成功路径均调用 `ClearGenerating`
- `routes.go`：`/turn` 和 `/regen` 路由检测 `ErrConcurrentGeneration` 返回 HTTP 409 + `code: "concurrent_generation"`；SSE `/stream` 路由在 errCh 中推送结构化错误事件

---
```

**File:** inspiration/engine-audit.md (L433-436)
```markdown
### ✅ 已完成（近期目标 — 精确 Token 计数）

- `internal/core/tokenizer/estimate.go`：BPE 兼容启发式（ASCII ÷4，CJK ×⅔），误差 ±15%
- 替换 `memory/store.go` 和 `engine_methods.go` 中的粗估
```

**File:** inspiration/engine-audit.md (L487-489)
```markdown
| **ScheduledTurn 定时模式**（后台 goroutine + SSE 推送） | 中 | 高 — 玩家不操作时 NPC 自主发帖；MVP 已完成后置检查模式，定时模式依赖 SSE 架构 |
| **MCP 协议接入** | 中 | 高 — Tools 层已建好，MCP 标准化接入社区工具生态 |
| **多 LLM 角色槽（director/verifier）** | 中 | 中 — 多 AI 协作；director 剧情控制，verifier 输出校验 |
```

**File:** inspiration/st-comparison.md (L182-194)
```markdown
## 四、WE 的差异化优势

| 特性 | 描述 |
|------|------|
| **PromptBlock 优先级 IR** | 每节点只产出 Block，Runner 统一按 Priority 排序，比 TH 的位置式组装更灵活 |
| **One-Shot 结构化 Parser** | 三层回退解析（XML → 编号列表 → fallback），产出 `{narrative, options, state_patch, vn}`，TH 依赖正则后处理 |
| **VN Directives** | LLM 输出 `[bg|...]`、`[sprite|...]`、`[choice|A|B]` 等指令，前端按游戏类型渲染，TH 无此概念 |
| **游戏包（game-package.json）** | 一个 JSON 文件打包 Preset + Worldbook + Regex + 素材，`POST /import` 一步导入，`draft→published` 发布 |
| **ScheduledTurn** | 根据变量阈值/随机概率自动触发 NPC 回合，Cooldown 持久化到会话变量 |
| **素材库 + search_material** | 游戏设计师预备文本素材（对话/氛围/事件），AI 按标签/情绪检索注入上下文，TH 无此概念 |
| **记忆时间衰减** | 指数半衰期 + MinDecayFactor 动态排序，TH 只有静态 importance |
| **Session Fork（批量平行时间线）** | 从任意 FloorSeq 复制全段历史创建新 Session，TH 只能从单 Floor 分叉 |
| **GW + CW 双端架构** | GW（游戏平台+论坛）和 CW（创作工具）共用 WE 引擎，游戏包是两端的连接纽带 |
```

**File:** inspiration/PLAN/P-WE-OVERVIEW.md (L151-157)
```markdown
### P-3H  MCP 协议接入 ⬜（暂缓）

**现状：** Preset Tool（HTTP 回调）已覆盖大多数云端集成场景。
**触发条件：** 创作者需要接入本地 MCP 工具（文件系统、代码执行）时再做。
**参照：** TH `apps/api/src/mcp/`（完整实现，含 stdio + HTTP 双传输模式）

---
```

**File:** inspiration/PLAN/P-WE-OVERVIEW.md (L159-175)
```markdown
## Phase 4 — 安全与平台工程 📋

### P-4A  API Key 加密存储（AES-256-GCM）⬜

**目标：** `LLMProfile.APIKey` 目前明文存储；上线前必须修复。
**工作：** `internal/core/secrets` 新增 `Encrypt/Decrypt`；LLMProfile 存密文 + mask；读取接口只返回 mask。
**参照：** TH `apps/api/src/lib/secrets.ts` + `drizzle/0005_llm_profile_vault.sql`

---

### P-4B  JWT Auth（X-Account-ID → Bearer token）⬜

**目标：** 当前 `X-Account-ID` 无签名，任何人可伪造。
**工作：** `internal/platform/auth` 新增 JWT 中间件（HS256）；`AUTH_MODE=off|jwt`；`off` 兼容开发环境。
**参照：** TH `apps/api/src/plugins/auth.ts`

---
```

**File:** inspiration/PLAN/P-WE-OVERVIEW.md (L246-264)
```markdown
### P-4I  VN 渲染引擎（资产系统 + 前端渲染层）⬜

**Stage A（已完成）：** 前端占位渲染（打字机效果 + 场景状态栏文字 badge + 头像占位符 + 选项按钮）。

**Stage B（后端先做）：** 资产系统
1. `Material` 表新增 `filename string`、`asset_type string`（`sprite/scene/bgm/cg/sfx`）字段
2. `POST /api/v2/assets/:game_id/upload`：单文件上传（multipart/form-data）
3. `POST /api/v2/assets/:game_id/upload-pack`：ZIP 包批量上传，按目录名推断 `asset_type`
4. `GET /api/v2/play/games/:id/assets`：返回 `{sprites:{name:url}, scenes:{name:url}, bgms:{name:url}, cgs:{name:url}}`
5. 静态文件服务：`router.Static("/assets", "./data/assets")`

**Stage C（前端）：** 完整 VN 渲染器
- 背景层：`div.bg-layer` 以 `background-image` 切换，CSS `transition: opacity 0.5s` 淡入淡出
- 立绘层：左/中/右三槽位，每个槽位一个 `<img>` + CSS `transition: opacity 0.3s`，shake/jump CSS 动画（`@keyframes`）
- CG 覆盖层：`div.cg-overlay`（`position:fixed, z-index:100`）点击关闭
- BGM 层：两个 `<audio>` 标签交叉淡入淡出（crossfade），用 `requestAnimationFrame` 控制音量渐变

**阻塞依赖：** Stage B 后端 → Stage B 前端缓存 → Stage C 渲染器

```

**File:** inspiration/PLAN/P-WE-OVERVIEW.md (L325-350)
```markdown
### P-5C  宏注册表重构 + 完整 ST 宏集合 ⬜

**P-5C1：可扩展注册表重构**

```go
// internal/engine/macros/registry.go
type Handler func(name string, args []string, ctx MacroContext) (string, bool)

type Registry struct { handlers map[string]Handler }
func (r *Registry) Register(name string, h Handler)
func (r *Registry) Expand(text string, ctx MacroContext) string

var DefaultRegistry = newDefaultRegistry()
func Expand(text string, ctx MacroContext) string { return DefaultRegistry.Expand(text, ctx) }
```

**P-5C2：完整宏集合**

| 宏 | 状态 |
|----|------|
| `{{char}}` / `{{user}}` / `{{persona}}` / `{{getvar::key}}` / `{{time}}` / `{{date}}` | ✅ 已实现 |
| `{{setvar::key::value}}` / `{{addvar::key::n}}` | Phase 5（副作用，需可写沙箱） |
| `{{lastMessage}}` / `{{lastMessageId}}` | Phase 5 |
| `{{random::n}}` | 低优先级 |
| 嵌套宏求值（`{{getvar::{{char}}_stage}}`）| Phase 5 |

```

**File:** inspiration/PLAN/P-WE-OVERVIEW.md (L429-445)
```markdown
### WE 计划做（Phase 4 / 5）

| 功能 | 计划编号 | 预期阶段 |
|------|---------|---------|
| API Key AES-256-GCM 加密 | P-4A | Phase 4 |
| JWT Auth | P-4B | Phase 4 |
| 多 Provider 原生适配（Anthropic / Gemini）| P-4C | Phase 4 |
| OpenAPI 文档（Swagger）| P-4D | Phase 4 |
| 对话导入/导出（ST JSONL + WE 原生格式）| P-4E | Phase 4 |
| 双层记忆压缩（micro / macro）| P-4F | Phase 4 |
| Background Job Runtime（DB 持久化）| P-4G | Phase 4 |
| Floor Run Phase SSE（生成阶段推送）| P-4H | Phase 4 |
| VN 渲染资产系统 | P-4I | Phase 4 |
| 包结构治理（social/ 填充）| P-5A | Phase 5 |
| 官方集成包（@gw/sdk + @gw/play-helpers）| P-5B | Phase 5 |
| 宏注册表重构 + 完整宏集合 | P-5C | Phase 5 |
| Social 层（Post / Comment / ShareLink）| P-5E | Phase 5 |
```

**File:** inspiration/talk-with-alice/2026-04-09-vector-memory-style-drift-client-domain.md (L55-63)
```markdown
### 1.5 向量记忆的实际价值评估

| 场景 | 向量记忆的价值 | 传统排序的价值 |
|------|--------------|--------------|
| 短期游戏（< 50 回合） | 低（记忆少，排序够用） | 高（简单可靠） |
| 长期角色（> 500 回合） | 高（语义检索找到"3个月前提到的事"） | 低（时间衰减会把旧记忆淘汰） |
| 常驻角色（跨游戏积累） | **非常高** | 中（记忆量大时排序失效） |

**结论**：向量记忆对 WE 的**常驻角色**（Resident Character）场景价值最大。普通游戏 session 用当前排序方案足够。
```

**File:** inspiration/talk-with-alice/2026-04-09-vector-memory-style-drift-client-domain.md (L66-96)
```markdown

**方案：pgvector**（PostgreSQL 扩展，不引入新基础设施）

```sql
-- 在 memories 表加 embedding 列
ALTER TABLE memories ADD COLUMN embedding vector(1536);  -- OpenAI text-embedding-3-small 维度

-- 创建向量索引
CREATE INDEX ON memories USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

检索时：

```go
// 把当前用户输入向量化
embedding := llm.Embed(userInput)  // 调用 embedding API

// 语义检索 Top-10
db.Raw(`
    SELECT *, 1 - (embedding <=> ?) AS similarity
    FROM memories
    WHERE session_id = ?
    ORDER BY similarity DESC
    LIMIT 10
`, pgvector.NewVector(embedding), sessionID).Scan(&memories)
```

**成本**：每次对话多一次 embedding API 调用（约 $0.00002 / 1K tokens，极低）。  
**依赖**：PostgreSQL 需安装 `pgvector` 扩展（`CREATE EXTENSION vector`）。

**实现优先级**：常驻角色 Phase 2 时引入，普通游戏 session 暂不需要。
```
