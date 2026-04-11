# WorkshopEngine — 整体开发计划

> 版本：2026-04-10（自审：P-5C/P-5F/P-5X1 → P-4J/P-4K/P-4L，引擎优先）
> 代码库：`backend-v2/`，主语言：Go 1.22 + Gin + GORM + PostgreSQL
> 编号规则：`P-<阶段><序号>` — 例：`P-1A`（Phase 1，第 A 项）

WorkshopEngine (WE) 是面向 AI 文字游戏的无头 REST API 引擎 + 游戏发布平台。

```
SillyTavern (ST)       一体化桌面应用，浏览器端执行所有逻辑
TavernHeadless (TH)    无头 REST API，严格对齐 ST 格式，面向开发者
WorkshopEngine (WE)    无头 REST API 引擎 + 游戏发布平台，面向玩家和创作者
```

WE 与 TH 的核心差异：
- **游戏包**（game-package.json）：Preset + Worldbook + Regex + 素材一键打包/发布
- **VN 指令**（`[bg|...]`、`[sprite|...]`、`[choice|A|B]`）：后端解析、前端渲染
- **素材库**（Material + `search_material` 工具）：AI 按标签检索注入上下文
- **ScheduledTurn**：变量阈值触发 NPC 自主回合
- **前端游玩 UI**（TH 无）

---

## 快速状态总览

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | 核心基础设施 | ✅ 全部完成 |
| Phase 2 | 工具 + 多槽 LLM + 创作层 | ✅ 全部完成 |
| Phase 3 | 引擎能力补全 | ✅ M11–M14 全部完成 |
| Phase 4 | 安全 + 引擎完善 + 平台工程 | 📋 规划中 |
| Phase 5 | 集成包 + 架构治理 | ⚡ Social 层已提前完成（GW 平台层），其余规划中 |

---

## 包结构总览

```
backend-v2/
├── cmd/
│   ├── server/         主服务入口（HTTP 服务器 + 路由注册）
│   └── worker/         异步任务处理器（Memory 整合 Worker）
├── internal/
│   ├── core/
│   │   ├── db/         PostgreSQL + GORM（所有 DB 模型定义在此）
│   │   ├── llm/        LLM 客户端（Chat / ChatStream / SSE）
│   │   └── tokenizer/  Token 估算（BPE 兼容启发式）← 已从 core/ 移入 engine/
│   ├── platform/
│   │   ├── auth/       账户中间件（X-Account-ID，Phase 4-B 迁移为 JWT）
│   │   ├── gateway/    CORS / RequestID / StructuredLogger 中间件
│   │   ├── play/       玩家发现层（游戏列表/详情/worldbook/存档/创建会话）
│   │   └── provider/   LLM Profile 注册表（slot 优先级解析）
│   ├── engine/
│   │   ├── api/        HTTP 层（执行层：turn/stream/regen/fork/memories 等）
│   │   ├── macros/     宏展开（{{char}}/{{user}}/{{getvar::key}} 等）
│   │   ├── memory/     记忆存取 + 异步整合 Worker
│   │   ├── parser/     AI 响应解析（三层回退：XML → 编号列表 → fallback）
│   │   ├── pipeline/   Prompt 组装流水线（PromptBlock IR）
│   │   ├── processor/  Regex 后处理（user_input / ai_output / all）
│   │   ├── prompt_ir/  流水线上下文类型（ContextData / PromptBlock / ...）
│   │   ├── scheduled/  ScheduledTurn 触发规则求值
│   │   ├── session/    Session/Floor/Page 状态机
│   │   ├── tools/      工具注册表 + 内置工具 + ResourceToolProvider
│   │   ├── types/      共享消息类型
│   │   └── variable/   五层变量沙箱
│   ├── creation/
│   │   ├── api/        HTTP 层（模板 / 角色卡 / 世界书 / 素材等 CRUD）
│   │   ├── asset/      素材上传与管理
│   │   ├── card/       角色卡 PNG 解析（ST V2/V3）
│   │   ├── lorebook/   世界书管理
│   │   └── template/   游戏模板 CRUD + 游戏包导入导出
│   ├── social/
│   │   ├── reaction/   点赞/收藏（game/comment/post 通用）
│   │   ├── comment/    游戏评论区（linear/nested 模式）
│   │   └── forum/      社区论坛帖子（热度排序 + 全文搜索）
│   └── integration/    (仅集成测试，Phase 5-A 迁移整理)
└── docs/
    ├── database.md     DB schema 字段说明
    └── architecture.md 架构决策文档
```

---

## 关键 API 端点速查

### 游玩层 `/api/play`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/games` | 已发布游戏列表（公开字段）← platform/play |
| GET | `/games/:slug` | 游戏详情（slug 或 UUID，含 comment_config）← platform/play |
| GET | `/games/worldbook/:id` | 玩家只读世界书 ← platform/play |
| GET | `/sessions` | 存档列表 ← platform/play |
| POST | `/sessions` | 创建会话（自动注入 first_mes）← platform/play |
| POST | `/sessions/:id/turn` | 同步一回合 |
| POST | `/sessions/:id/regen` | 重新生成（Swipe） |
| POST | `/sessions/:id/suggest` | AI 帮答（Impersonate，全管线，不写 Floor） |
| GET | `/sessions/:id/stream` | SSE 流式一回合 |
| GET | `/sessions/:id/state` | 会话状态快照 |
| PATCH | `/sessions/:id` | 更新会话标题/状态 |
| DELETE | `/sessions/:id` | 删除会话及关联数据 |
| GET | `/sessions/:id/variables` | 变量快照 |
| PATCH | `/sessions/:id/variables` | 合并更新变量 |
| GET | `/sessions/:id/floors` | 楼层列表（含激活页摘要） |
| GET | `/sessions/:id/floors/:fid/pages` | Swipe 页列表 |
| PATCH | `/sessions/:id/floors/:fid/pages/:pid/activate` | Swipe 选页 |
| GET | `/sessions/:id/memories` | 记忆列表 |
| POST | `/sessions/:id/memories` | 手动创建记忆 |
| PATCH | `/sessions/:id/memories/:mid` | 更新记忆字段 |
| DELETE | `/sessions/:id/memories/:mid` | 删除记忆（?hard=true 物理删除） |
| POST | `/sessions/:id/memories/consolidate` | 立即触发记忆整合（同步，调试用） |
| POST | `/sessions/:id/fork` | 分叉会话（平行时间线 / 存档点） |
| GET | `/sessions/:id/prompt-preview` | Prompt dry-run（不调用 LLM） |
| GET | `/sessions/:id/floors/:fid/snapshot` | Prompt 快照（Verifier 结果 + 命中词条） |
| GET | `/sessions/:id/tool-executions` | 工具执行记录 |
| GET | `/sessions/:id/memory-edges` | 记忆关系边列表 |
| POST | `/sessions/:id/memory-edges` | 手动创建关系边 |
| GET | `/sessions/:id/branches` | 分支列表 |
| POST | `/sessions/:id/floors/:fid/branch` | 从楼层创建分支 |
| GET | `/sessions/:id/export` | 导出会话（?format=thchat\|jsonl） |
| POST | `/sessions/import` | 导入 .thchat 会话 |

### 创作层 `/api/create`

| 方法 | 路径 | 说明 |
|------|------|------|
| POST/GET/DELETE | `/cards` | 角色卡 CRUD |
| POST | `/cards/import` | 导入角色卡 PNG |
| GET/POST/DELETE | `/templates/:id/lorebook` | 世界书词条 CRUD |
| POST | `/templates/:id/lorebook/import-st` | ST 世界书 JSON 批量导入 |
| GET/POST/PATCH/DELETE | `/templates` | 游戏模板 CRUD |
| POST | `/templates/import` | 游戏包导入（game-package.json） |
| GET | `/templates/:id/export` | 游戏包导出 |
| POST | `/templates/:id/preset/import-st` | ST 预设 JSON 批量导入 |
| GET/POST/PATCH/DELETE | `/templates/:id/preset-entries` | PresetEntry CRUD |
| PUT | `/templates/:id/preset-entries/reorder` | 批量调整顺序 |
| GET/POST/PATCH/DELETE | `/templates/:id/tools` | Preset Tool CRUD |
| GET/POST/PATCH/DELETE | `/llm-profiles` | LLM Profile CRUD |
| POST | `/llm-profiles/:id/activate` | 绑定 Profile 到 slot |
| POST | `/llm-profiles/models/discover` | 发现可用模型 |
| POST | `/llm-profiles/models/test` | LLM 连通性测试 |
| GET/POST/PATCH/DELETE | `/regex-profiles` | Regex Profile CRUD |
| GET/POST/PATCH/DELETE | `/materials` | 素材库 CRUD |
| POST | `/materials/batch` | 批量导入素材 |
| POST | `/sessions/:id/archive` | 边界归档（生成结构化摘要） |

### 社交层 `/api/social`

| 方法 | 路径 | 说明 |
|------|------|------|
| POST/DELETE | `/reactions/:target_type/:target_id/:type` | 点赞/收藏（game/comment/post）|
| GET | `/reactions/mine/:target_type/:target_id` | 查自己的 reaction 状态 |
| GET | `/reactions/counts` | 批量查计数 |
| POST | `/games/:id/comments` | 发主楼评论 |
| GET | `/games/:id/comments` | 评论列表（linear/nested）|
| POST | `/comments/:id/replies` | 回复评论 |
| POST/DELETE | `/comments/:id/vote` | 评论点赞/取消 |
| GET | `/posts` | 帖子列表（?game_tag=&sort=hot\|new&q=）|
| POST | `/posts` | 发帖 |
| GET/PATCH/DELETE | `/posts/:id` | 帖子详情/编辑/删除 |
| POST/DELETE | `/posts/:id/vote` | 帖子点赞/取消 |
| GET/POST | `/posts/:id/replies` | 盖楼列表/盖楼 |
| GET | `/games/:id/stats` | 游戏社交统计 |

---

## Phase 1 — 核心基础设施 ✅

### P-1A  基础服务 ✅
PostgreSQL + GORM AutoMigrate，全部模型定义于 `internal/core/db/models.go`。
OpenAI 兼容 LLM 客户端（Chat + ChatStream SSE），`internal/core/llm/`。
Gin 路由框架，`/api/v2/play` + `/api/v2/create` 双命名空间。

### P-1B  Prompt 流水线 ✅
`PromptBlock IR`：每节点产出带 Priority 的 Block，Runner 按 Priority 排序后输出消息列表。
流水线节点：TemplateNode / WorldbookNode / MemoryNode / HistoryNode / PresetNode。
世界书触发逻辑：主/次关键词 + 逻辑门 + 正则 + 全词 + 扫描深度 + 常驻 + **递归激活**。
WorldbookEntry 互斥分组（`applyGroupCap`）+ Token Budget（`applyTokenBudget`）。
Worldbook 变量门控（`var:key=value` / `var:key!=value` / `var:key` 硬条件）。

### P-1C  变量沙箱 ✅
`variable.Sandbox`：五级（Global → Chat → Floor → Branch → Page）级联读取。
`Sandbox.Set()` 写入 Page 层；`CommitTurn` 时 `Flatten()` 提升到 Chat 层。

### P-1D  Session 状态机 ✅
`session.Manager`：StartTurn / CommitTurn / RegenTurn / FailTurn。
Floor 状态枚举：`generating → committed / failed`。
MessagePage：每 Floor 可有多个 Page（Swipe），只有一个 `is_active`。
SSE 流式输出 + `POST /sessions/:id/fork`（平行时间线）。

### P-1E  解析器 + Regex 后处理 ✅
`parser.Parse()`：三层回退（XML → 编号列表 → fallback 纯文本）。
VN 指令解析：`[bg|]` / `[bgm|]` / `[sprite|]` / `[choice|A|B|C]` / `[cg|]` / `[hide_cg]`。
对话格式解析：`角色|情绪|台词` → `{type:dialogue, speaker, text}`。
`processor.ApplyToUserInput()` / `ApplyToAIOutput()`（Regex 规则管道）。

### P-1F  记忆系统 ✅
`memory.Store`：记忆 CRUD + 生命周期（active → deprecated → purged）。
指数半衰期衰减排序（`GetForInjection` 按 importance × decay 得分排序）。
Memory Worker：每 N 回合异步触发，调用廉价模型提取结构化整合结果。
`MemoryEdge` 表：updates / contradicts / supports / resolves 关系图。
Memory 分阶段标签（`stage_tags`）：按 `game_stage` 变量过滤注入。

### P-1G  工具系统 ✅
`tools.Registry`：工具注册、定义序列化（`ToLLMDefinitions()`）、带审计的执行。
内置工具：`get_variable` / `set_variable` / `search_memory` / `search_material`。
`ResourceToolProvider`：14 个 AI 可操作资源工具（worldbook / preset / memory / material 读写）。
Preset Tool：创作者自定义 HTTP 回调工具（`preset:*` / `preset:<name>` 动态加载）。
Agentic Loop：最多 5 轮工具调用（无工具时单次直通）。

### P-1H  多角色 LLM 槽 ✅
LLM Profile CRUD + `POST /llm-profiles/:id/activate`（绑定到 slot）。
`provider.Registry.ResolveForSlot()`：5 级优先级（session X → global X → session * → global * → env）。
Director 槽（廉价模型预分析）+ Verifier 槽（生成后一致性校验）。
PromptSnapshot：每 Floor 异步写入，记录激活词条 / preset_hits / worldbook_hits / est_tokens / verifier 结果。

### P-1I  创作工具层 ✅
角色卡 PNG 导入（ST V2/V3）+ 结构化存储。
游戏包打包/解包：`POST /templates/import` + `GET /templates/:id/export`（game-package.json 格式）。
ST 预设 / 世界书批量导入。
边界归档 API：`POST /sessions/:id/archive`。
AI 辅助创作（creation-agent）：`resource:*` 工具对话式修改游戏规则。

### P-1J  宏展开 ✅
`internal/engine/macros/expand.go`：`MacroContext` + `Expand(text, ctx)` 函数。
支持宏：`{{char}}` / `{{user}}` / `{{persona}}` / `{{getvar::key}}` / `{{time}}` / `{{date}}`。
注入点：TemplateNode / PresetNode / WorldbookNode 均在输出内容前调用 `macros.Expand()`。
→ 扩展计划见 P-4K（注册表重构 + 副作用宏 + 嵌套求值）。

---

## Phase 2 — 工具 + 多槽 LLM + 创作层 ✅

所有条目已并入 Phase 1（P-1G、P-1H、P-1I）。

---

## Phase 3 — 引擎能力补全 ✅

### P-3A  ~~Memory Edge（记忆关系图）~~ ✅ 2026-04-06

### P-3B  ~~LLM 模型发现 + 连通性测试~~ ✅ 2026-04-06

### P-3C  ~~Worldbook 互斥分组（group）~~ ✅ 2026-04-06

### P-3D  ~~Worldbook 变量门控（`var:` 语法）~~ ✅ 2026-04-06

### P-3D1（补丁）  ~~Worldbook Token Budget~~ ✅ 2026-04-07
`applyTokenBudget`：Constant=pinned，其余按 Priority 升序贪心保留到预算上限。

### P-3D2（补丁）  ~~Initial Variables 不注入扫描文本~~ ✅ 2026-04-07
`buildScanText` 只扫描对话历史，不包含变量树内容。

### P-3E  ~~Memory 分阶段标签（stage_tags）~~ ✅ 2026-04-07

### P-3F  ~~边界归档 API~~ ✅（含在 P-1I 中）

### P-3I1（补丁）  ~~Macro Expand（宏展开）~~ ✅ 2026-04-08

### P-3I  ~~角色注入管线（CharacterInjectionNode）~~ ✅ 2026-04-08
`CharacterInjectionNode`（Priority=9）：提取 ST V2/V3 描述字段，经宏展开后注入 System Prompt。
`pin`（默认）/ `latest` 两种同步策略。`GameSession.CharacterSnapshot` 冻结 pin 版本。

### P-3J  ~~at_depth 世界书注入位置~~ ✅ 2026-04-08

`WorldbookEntry` + `prompt_ir.WorldbookEntry` 新增 `Depth int` 字段；
`node_worldbook.go` 将 `at_depth` 词条路由到 `ctx.AtDepthBlocks`；
`Runner.Execute` 在普通 Blocks 组装后按 `idx = clamp(len-depth, 1, len)` 插入，从大 idx 开始。
ST 导入时 position 数字（0–4）映射到 WE 字符串 + depth 透传。

### P-3K  ~~Generation Coordination（并发生成保护）~~ ✅ 2026-04-08

`GameSession` 新增 `Generating bool` + `GenerationMode string`（默认 `reject`）。
`StartTurn` 用 `SELECT ... FOR UPDATE` 原子检测并设置 `generating=true`；
`ClearGenerating` 在 CommitTurn / FailTurn 后复位；
路由层返回 HTTP 409 + `code: "concurrent_generation"`。

---

### P-3G  Session 内分支（branch_id）✅ 2026-04-08

**完成内容：**
- `Floor` 新增 `BranchID string`（GORM default `"main"`，带 index）
- 新增 `SessionBranch` 表（`branch_id` / `parent_branch` / `origin_seq` / `session_id`）并加入 AutoMigrate
- `session.Manager.StartTurn` 新增 `branchID string` 参数（空字符串视为 "main"）
- `session.Manager.GetHistory` 新增 `branchID string` 参数：
  - "main" → 只查 `branch_id = 'main'` 的已提交楼层
  - 非 main → 查父分支 `seq ≤ OriginSeq` + 本分支所有楼层（按 seq 升序合并）
- `session.Manager.ListFloors` 新增 `branchID string` 参数（空 = 全分支）
- 新增 `session.BranchInfo` + `ListBranches` / `CreateBranch` / `DeleteBranch` 方法
- `TurnRequest` 新增 `BranchID string`（JSON: `branch_id`，可选，空 = "main"）
- 全链路透传：`game_loop.go` / `engine_methods.go` / `memory/worker.go` 均已更新
- `CreateSession` first_mes 楼层、`ForkSession` 复制楼层均显式设置 `BranchID = "main"`
- 新增 3 个路由：`GET /sessions/:id/branches` / `POST /sessions/:id/floors/:fid/branch` / `DELETE /sessions/:id/branches/:bid`

---

### P-3H  MCP 协议接入 ⬜（明确暂缓）

**现状：** Preset Tool（HTTP 回调）已覆盖大多数云端集成场景。

**不做的理由（明确）：**
1. WE 定位是游戏发布平台引擎，本地工具接入（文件系统、代码执行）不在核心场景
2. Preset Tool（HTTP 回调）已覆盖云端集成；`resource:*` 工具已覆盖 AI 修改游戏规则的场景
3. Deferred 执行模型（MCP 工具调用需用户确认）需要前端配合，GW 前端目前不具备此能力

**重新评估条件：** GW 前端具备用户确认交互能力，且有创作者明确提出本地工具接入需求。

**参照：** TH `apps/api/src/mcp/`（完整实现，含 stdio + HTTP 双传输模式）

---

## Phase 4 — 安全 + 引擎完善 + 平台工程 📋

> **执行顺序说明：** P-4A 和 P-4B 是上线硬性前置条件，必须第一批完成。第二批为平台工程基础设施。第三批为引擎能力完善（宏/管线/预飞/VN/记忆），是 WE 引擎推进到"基本完成"的核心工作。

### 第一批（上线阻断，必须先完成）

### P-4A  API Key 加密存储（AES-256-GCM）✅ 2026-04-10

**实现：** `internal/core/secrets/secrets.go` — AES-256-GCM + HKDF-SHA256 派生密钥。
存储格式：`v1:<base64url(salt)>:<base64url(iv)>:<base64url(ciphertext+tag)>`。
环境变量：`SECRETS_MASTER_KEY`（空 = 开发模式不加密，打印警告）。

**改动：**
- `LLMProfile` 新增 `APIKeyEncrypted` + `APIKeyMasked` 列；旧 `APIKey` 列保留兼容
- `creation/api/routes.go`：POST/PATCH 时加密写入，GET 返回 `api_key_masked`
- `provider/registry.go`：`ResolveForSlot` 解密后传给 `llm.NewClient`；`api_key_encrypted` 为空时回退明文列
- `config.go` 新增 `Secrets.MasterKey`

---

### P-4B  JWT Auth（X-Account-ID → Bearer token）✅ 2026-04-10

**实现：** `internal/platform/auth/jwt.go` — HS256 签名，`sub` = account_id，默认 TTL 7 天。
`middleware.go` 新增 `ModeJWT`：`AUTH_MODE=jwt` + `AUTH_JWT_SECRET` 启用。

**改动：**
- `auth.Config` 新增 `JWTSecret`；`NewConfigFromEnv` 新增 `authMode, jwtSecret` 参数
- `Middleware` 新增 `case ModeJWT`：Bearer token → `ParseToken` → 注入 account_id
- `config.go` 新增 `Auth.Mode` / `Auth.JWTSecret` / `Auth.JWTTTLHours`
- `cmd/server/main.go` 新增 `POST /api/auth/token`（admin_key 验证后签发 JWT）
- 向后兼容：`AUTH_MODE` 未设置时保持旧自动检测逻辑

---

### 第二批（平台工程，可并行）

### P-4C  多 Provider 原生适配 ✅ 2026-04-10

**目标：** 抽象 `llm.Provider` 接口，支持 OpenAI 兼容（含 DeepSeek/xAI/Ollama）+ Anthropic 原生 + Google Gemini（延后）。

**设计详见：** `inspiration/logs/2026-04-10-multi-provider-design.md`

**实现：**
1. `llm.Provider` 接口（`Chat` + `ChatStream` + `ID`），现有 `Client` 天然满足
2. `llm.NewProvider(type, ...)` 工厂函数，按 `provider` 字段路由到对应实现
3. `anthropic.go`（~250 行）：`x-api-key` 鉴权 + `/v1/messages` 端点 + system 字段提取 + `content_block_delta` 流解析 + `tool_use` 格式转换
4. `provider/registry.go`：`ResolveForSlot` 返回 `llm.Provider`；SQL 新增 `p.provider` 列；`NewProvider` 替代 `NewClient`
5. `engine/api/`：`GameEngine.llmClient` 类型 `*llm.Client` → `llm.Provider`；director/verifier 闭包返回类型同步更新
6. `engine/memory/`：`Worker.llmClient` 类型同步更新
7. OpenAI/DeepSeek/xAI/Ollama/vLLM/LM Studio 全部走现有 `Client`（零改动）
8. Google Gemini 延后（`google.go`，Phase 2）

**参照：** TH `packages/core/src/llm/provider-registry.ts`（Vercel AI SDK 抽象）

---

### P-4D  OpenAPI 文档（swaggo）✅ 2026-04-10

**实现：** `swaggo/swag` + `swaggo/gin-swagger` 自动生成 Swagger UI。

**策略：** 所有 handler 为匿名闭包，采用独立 `swagger_docs.go` 注释文件（每包一个，空函数 + `@Router` 注释），零改动现有 handler 逻辑。唯一例外：`platform/play/handler.go` 已有命名方法，直接加注释。

**改动：**
- `cmd/server/main.go`：顶部 `@title/@version/@BasePath/@securityDefinitions` + `GET /swagger/*any` 路由
- 7 个 `swagger_docs.go` 新文件：`cmd/server/` + `engine/api/` + `creation/api/` + `creation/asset/` + `social/reaction/api/` + `social/comment/api/` + `social/forum/api/`
- `platform/play/handler.go`：5 个命名方法加 swag 注释
- `docs/swagger/`：`swag init` 生成（docs.go + swagger.json + swagger.yaml）
- 覆盖 73 个路径，全部端点按 Tags 分组（play-engine / play-discovery / creation-* / social-* / system / auth）

---

### P-4E  对话导入 / 导出 ✅ 2026-04-10

**目标：** 会话完整备份与跨平台迁移。

**导出端点：** `GET /api/play/sessions/:id/export?format=thchat|jsonl`
**导入端点：** `POST /api/play/sessions/import`

#### `.thchat` 格式（WE 原生，无损）

```json
{
  "version": "1.0",
  "format": "thchat",
  "exported_at": "2026-04-10T12:00:00Z",
  "game_id": "uuid",
  "game_title": "游戏标题",
  "session": {
    "title": "存档标题",
    "status": "active",
    "variables": { "affection": 50, "stage": "chapter2" },
    "memory_summary": "...",
    "character_snapshot": { ... }
  },
  "floors": [
    {
      "seq": 1,
      "branch_id": "main",
      "pages": [
        {
          "is_active": true,
          "messages": [{ "role": "assistant", "content": "..." }],
          "page_vars": {},
          "token_used": 150
        }
      ]
    }
  ],
  "memories": [
    {
      "fact_key": "npc_mood",
      "content": "NPC 好感度 50",
      "type": "fact",
      "importance": 1.0,
      "source_floor": 5,
      "stage_tags": ["chapter2"]
    }
  ],
  "memory_edges": [
    { "from_key": "old_key", "to_key": "new_key", "relation": "updates" }
  ],
  "branches": [
    { "branch_id": "what-if", "parent_branch": "main", "origin_seq": 5 }
  ]
}
```

设计要点：
- 导出保留原始 UUID 供溯源，导入时全部重映射为新 UUID
- `floors[].pages[]` 保留全部 Swipe 历史；`is_active` 标记当前选中页
- `memory_edges` 用 `fact_key` 引用（比 UUID 更稳定，跨导入可重建）
- `character_snapshot` 包含在内，导入时即使角色卡不存在也能恢复

#### `.jsonl` 格式（ST 兼容，有损）

每行一个 JSON 对象，仅 main 分支 active page 消息，按楼层顺序：

```jsonl
{"role":"assistant","content":"欢迎来到冒险..."}
{"role":"user","content":"我环顾四周"}
{"role":"assistant","content":"你看到一间昏暗的房间..."}
```

有损格式：无记忆、无变量、无分支、无 Swipe 历史。

#### 实现文件

| 文件 | 内容 |
|------|------|
| `internal/engine/api/session_export.go`（新建，~280 行） | `ThchatExport` 类型族 + `ExportThchat` / `ExportJSONL` / `ImportThchat` + 路由 handler |
| `internal/engine/api/routes.go`（修改） | 末尾调用 `registerExportImportRoutes(play, engine)` |

**改动：**
- `ExportThchat`：单次查询 session + floors + pages + memories + edges + branches，组装为 ThchatExport
- `ExportJSONL`：仅 main 分支 committed floors 的 active page 消息，逐行 JSON 编码
- `ImportThchat`：事务内创建，所有 UUID 重映射（session/floor/page/memory/edge/branch），记忆边跳过引用不存在的记忆
- Swagger 注释已加入 `swagger_docs.go`，`swag init` 后 73 个路径

**参照：** TH `apps/api/src/services/chat-export.ts` + `chat-import-manifest.ts`

---

### P-4G  Background Job Runtime（DB 持久化）✅ 2026-04-10

**为什么做：** 进程重启时丢失未完成记忆整合任务是确定性的生产问题（服务器重启、OOM kill、部署更新），不是低概率风险。

**具体工作：**
1. 新增 `runtime_job` 表：`id`, `job_type`, `session_id`, `payload JSONB`, `status`（`queued → leased → done / failed / dead`）, `lease_until`, `retry_count`, `max_retries`, `error_log`, `dedupe_key`（唯一索引去重）
2. `internal/engine/scheduler` 包：`Enqueue`（去重入队）+ `LeaseJob`（`FOR UPDATE SKIP LOCKED` 原子租约）+ `Complete` + `Fail` + `RecoverStale` + `CleanDone` + `EnqueueIfDue`
3. 进程启动时 `RecoverStale()`：将 `status=leased AND lease_until < now()` 的任务重置为 `queued`
4. Dead letter 策略：`retry_count >= max_retries`（默认 3）时标记为 `dead`
5. `memory/worker.go` 重构：移除 `sync.Map` 内存租约，改为从 scheduler 消费 `memory_consolidation` 类型 Job
6. `GameEngine.triggerMemoryConsolidation` 实现：调用 `sched.EnqueueIfDue()` 入队而非 goroutine 直接执行

**实现文件：**
| 文件 | 改动 |
|------|------|
| `internal/core/db/models_engine.go` | 新增 `RuntimeJob` 模型 + `JobStatus` 枚举 |
| `internal/core/db/connect.go` | AutoMigrate 加入 `RuntimeJob` |
| `internal/engine/scheduler/scheduler.go`（新建） | ~160 行，完整调度器 |
| `internal/engine/memory/worker.go` | 重写为 scheduler 消费者，移除 sync.Map |
| `internal/engine/memory/store.go` | 新增 `GetFloorCount` 辅助方法 |
| `internal/engine/api/game_loop.go` | GameEngine 加入 `sched` 字段 + `triggerMemoryConsolidation` 实现 |
| `cmd/server/main.go` | 创建 scheduler 实例并注入 GameEngine |

**参照：** TH `apps/api/src/services/runtime-worker.ts` + `runtime-job-scheduler.ts` + `drizzle/0024_background_job_runtime.sql`

---

### 第三批（引擎完善 + 体验增强）

### P-4F  双层记忆压缩（micro / macro Memory）⬜

**为什么做：** 当前单层 fact 记忆；长时间游玩后记忆条目本身超出注入预算。双层记忆让近期行为保持细粒度，历史行为自动压缩为高层摘要。

**具体工作：**
1. `Memory.Type` 新增枚举值 `micro`（单回合摘要）/ `macro`（压缩摘要），现有 `fact` 类型不变
2. Memory Worker 策略：每 N 回合触发一次 `micro` → `macro` 压缩（N 可配置，默认 10）
3. `GetForInjection` 支持 `dual_summary` 策略：`macro` 以 pinned 方式注入最多 2-3 句 compact 版本，`micro` 按预算注入最近 N 条
4. `GameTemplate.Config.memory_strategy`：`facts_only`（现有行为）/ `dual_summary`（新策略）
5. `MemoryEdge.Relation` 新增 `compacts` 类型（`compact_macro` 操作时自动写入，指向被压缩的 micro 集合）

**改动量：** DB 迁移（Memory.Type 新枚举）+ Memory Worker + GetForInjection 约 150 行。

**参照：** TH `packages/core/src/memory/memory-compaction-*.ts` + `memory-ingest-processor.ts` + `memory-injection-selector.ts`

---

### P-4H  Floor Run Phase SSE（生成阶段实时推送）⬜

**为什么做：** 前端无法知道 Director/Memory 整合/Verifier 等阶段的进度；移动端长时间等待时用户只能看到"生成中…"。

**具体工作：**
1. SSE 事件新增 `phase` 事件类型：`{"event":"phase","data":{"phase":"director_running"}}`
2. Phase 枚举：`preparing` | `director_running` | `prompt_assembling` | `generating` | `verifying` | `committing`
3. 前端可订阅 `GET /sessions/:id/stream` 获取 phase + token 混合流
4. `PromptSnapshot` 可选存储各阶段耗时（`phase_timings JSONB`），用于性能分析

**参照：** TH `packages/core/src/floor/floor-state-machine.ts` + `drizzle/0026_floor_run_state.sql`

---

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

---

### P-4J  game_loop.go 拆分（Pipeline Context 提取）⬜

**为什么做：** `game_loop.go` 的 `PlayTurn` 步骤 1-8（加载模板/世界书/Preset/Regex/变量/记忆/历史/Pipeline 执行）在 `PlayTurn`、`StreamTurn`、`Suggest` 三处重复。新增 `Suggest` 全管线后重复代码进一步增加，测试覆盖困难。

**具体工作：**

1. 新增 `internal/engine/api/pipeline_context.go`，提取共享函数：
   ```go
   type PipelineInput struct {
       SessionID string
       BranchID  string
       UserInput string // 空 = Suggest/Preflight 模式，由调用方追加指令
   }
   type PipelineOutput struct {
       Messages []llm.Message
       Sandbox  *variable.Sandbox
       ToolReg  *tools.Registry
       TmplCfg  parsedTemplateConfig
       Template dbmodels.GameTemplate
       Session  dbmodels.GameSession
   }
   func (e *GameEngine) buildPipelineContext(ctx context.Context, input PipelineInput) (*PipelineOutput, error)
   ```
2. `PlayTurn` / `StreamTurn` 改为调用 `buildPipelineContext()` 后继续各自的 Floor 创建 + LLM 调用 + 提交逻辑
3. `Suggest` 改为调用 `buildPipelineContext()` + 追加 impersonate 指令 + narrator 槽调用
4. Preflight（P-4L）直接复用 `buildPipelineContext()` + 预测指令

**改动量：** ~200 行提取 + 三处调用点改写。不改变任何外部行为。

**前置条件：** 无。可随时执行，建议在 P-4K/P-4L 之前完成。

---

### P-4K  宏注册表重构 + 完整 ST 宏集合 ⬜

**为什么做：** 当前 `macros/expand.go` 是硬编码 `strings.ReplaceAll` 链，无法扩展。新增宏需要修改 `Expand()` 函数本身。副作用宏（`{{setvar}}`）需要可写沙箱访问，但 Verifier 阶段必须只读——当前架构无法区分。

**P-4K1：可扩展注册表（~100 行）**

```go
// internal/engine/macros/registry.go
type Handler func(name string, args []string, ctx *MacroContext) (string, bool)

type Registry struct {
    handlers []registeredHandler // 有序，支持优先级
}

func (r *Registry) Register(name string, h Handler)
func (r *Registry) Expand(text string, ctx *MacroContext) string
```

实现要点：
- `Expand()` 用单次正则扫描 `\{\{([^}]+)\}\}` 提取所有宏调用，逐个查表执行
- Handler 返回 `(replacement, ok)`；`ok=false` 时保留原文
- `MacroContext` 新增 `ReadOnly bool` 字段：Verifier 阶段设为 `true`
- 默认注册表 `DefaultRegistry` 包含所有现有宏（`char/user/persona/getvar/time/date`）
- 保持 `macros.Expand(text, ctx)` 签名不变（内部委托 `DefaultRegistry`），零破坏性

**P-4K2：副作用宏（~60 行）**

| 宏 | Handler 行为 |
|----|-------------|
| `{{setvar::key::value}}` | `ctx.ReadOnly` 时返回空串不执行；否则 `ctx.Sandbox.Set(key, value)` |
| `{{addvar::key::n}}` | 同上，读取当前值 + n 后写入 |
| `{{lastMessage}}` | `ctx.LastAssistantMessage`（新增字段） |
| `{{lastMessageId}}` | `ctx.LastFloorID`（新增字段） |
| `{{random::n}}` | `rand.Intn(n)` 转字符串 |

**P-4K3：嵌套宏求值（~30 行）**

`{{getvar::{{char}}_stage}}` → 两轮展开：
1. 第一轮：展开内层 `{{char}}` → `{{getvar::Alice_stage}}`
2. 第二轮：展开外层 `{{getvar::Alice_stage}}` → 变量值

实现：`Expand()` 循环最多 3 轮，直到输出不再变化或达到上限。每轮用 `strings.Contains(result, "{{")` 快速判断是否需要继续。

**安全约束：**
- `ReadOnly=true` 时，`setvar/addvar` 静默跳过（不报错，不执行）
- 嵌套深度硬限 3 轮，防止 `{{getvar::{{getvar::...}}}}` 无限递归
- 副作用宏在 Pipeline 组装阶段执行（PresetNode/WorldbookNode），不在 LLM 输出后处理中执行

**改动量：** ~190 行总计。`expand.go` 保留为 `DefaultRegistry` 的初始化入口。

---

### P-4L  Preflight Rendering（预飞渲染 + 完整预生成）⬜

**与 Suggest 的关系：** `Suggest()`（AI 帮答）已升级为全管线模式（世界书/Preset/记忆/角色注入），与 Preflight 共享同一个 `buildPipelineContext()` 基础设施。二者的区别仅在于触发方式和最终指令：

| | Suggest | Preflight lazy | Preflight eager |
|---|---|---|---|
| 触发 | 用户手动 | PlayTurn 完成后自动 | PlayTurn 完成后自动 |
| 指令 | "扮演玩家给出行动" | "预测 3 个选项" | 对每个预测选项跑完整 PlayTurn |
| LLM 调用 | 1 次（narrator 槽） | 1 次（director 槽） | 1+3 次 |
| 写入 Floor | 否 | 否 | 缓存到变量，不写 Floor |
| 成本 | 1x | ~0.3x（廉价模型） | ~4x |

**Stage 1 — lazy 模式（最小可用版本，~80 行后端）：**

1. `GameTemplate.Config` 新增 `preflight_mode: "off"|"lazy"|"eager"`（默认 `"off"`）
2. `PlayTurn` / `StreamTurn` 完成后，若 `preflight_mode != "off"`，异步 `go e.preflight(ctx, sessionID)`
3. `preflight()` 调用 `buildPipelineContext()` + director 槽，指令为"预测 3 个选项，JSON 数组返回"
4. 结果写入 `Session.Variables["predicted_choices"]`（`[{label, subtext}]`）
5. 前端通过 `GET /sessions/:id/variables` 读取，展示在输入框上方
6. 用户点选项 → `POST /turn { user_input: "选项文本" }`，与普通输入无异

**Stage 2 — eager 模式（完整预生成，~150 行后端）：**

1. `preflight()` 在 lazy 预测完成后，对每个选项调用 `buildPipelineContext()` + narrator 槽完整生成
2. 结果缓存到 `Session.Variables["predicted_pages"]`（`[{choice, narrative, vn, options}]`）
3. 前端点选项时先检查缓存命中 → 命中则直接渲染（零等待），未命中降级为普通 PlayTurn
4. 缓存命中后仍需调用 `POST /turn` 将结果正式写入 Floor（后端验证 + 记忆整合）

**Stage 3 — 前端联动：**

1. `ChatInput` 上方新增选项行组件，读取 `predicted_choices` 变量
2. 选项行在预飞完成前隐藏，完成后淡入
3. 用户自由输入时选项行自动收起

**前置条件：** P-4J（pipeline context 提取）。

---

## Phase 5 — 集成包 + 架构治理 📋

### P-5A  包结构治理 ⚡（部分完成）

**已完成：**
- `social/` 三个子包（reaction / comment / forum）已实现，依赖方向正确
- `platform/play/` 新增，玩家发现层从 engine 分离
- `engine/tokenizer/` 已从 `core/tokenizer/` 迁移
- `core/db/models.go` 已拆分为 models_shared / models_engine / models_creation 三文件

**目标状态（三层硬分离）：**

```
internal/
├── core/           ← DB连接、LLM客户端（无业务依赖）
├── platform/       ← auth、gateway、play、provider（跨层共享平台服务）
├── engine/         ← 游戏运行时（纯业务，无 HTTP 依赖）
│   └── api/        ← HTTP 层（执行层路由）
├── creation/       ← 创作工具（独立于 engine，仅共享 core/db 模型）
│   └── api/        ← HTTP 层
├── social/         ← 社区层（完全独立，不引用 engine 内部包）✅
└── integration/    ← 集成层（webhook、事件分发、集成测试）
    └── tests/      ← 原 llm_test.go 移入此处
```

**层间依赖规则（严格单向）：**
- `engine` 不得引用 `creation` 或 `social` ✅
- `social` 不得引用 `engine` 内部包 ✅
- `creation` 不得引用 `engine/api` ✅

---

### P-5B  官方集成包（@gw/sdk + @gw/play-helpers）⬜

**目标：** 第三方开发者用 TypeScript 构建自定义游玩界面。

**`@gw/sdk`（HTTP 传输层）：**

```typescript
const client = new GameWorkshopClient({ baseUrl, apiKey })
const session = await client.sessions.create({ gameId: 'xxx' })

for await (const event of client.sessions.stream(session.id, { content: '行动' })) {
  if (event.type === 'token') appendToken(event.data.text)
  if (event.type === 'phase') showPhase(event.data.phase)
  if (event.type === 'done')  applyMeta(event.data)
}
```

**`@gw/play-helpers`（状态规范化工具，无 HTTP 依赖）：**

```typescript
import { buildMessageTimeline } from '@gw/play-helpers/timeline'
import { reduceGameStream }     from '@gw/play-helpers/stream'
import { applyVNDirectives }    from '@gw/play-helpers/vn'
```

**发布阶段：** 内部版（已在 client.js 中实现）→ Phase 4-D 后提取为 monorepo 包 → Phase 5-B 发布 npm。

**参照：** TH `packages/official-integration-kit/` + `packages/shared/src/api/`

---

### P-5D  架构文档 ✅ 2026-04-08

里程碑追踪文档已迁移至 `inspiration/PLAN/P-WE-PROGRESS.md`（本次合并完成）。
`docs/architecture.md`（架构决策文档）已完成。

---

### P-5E  Social 层 ✅ 2026-04-08（已提前完成）

`internal/social/` 已完整实现，包含 reaction / comment / forum 三个子包。详见 `GW-BACKEND-REFACTOR-PLAN.md` Task 4。

**实际实现（与原计划有差异）：**
- `Post` → `forum.Post`（含 Markdown 渲染 + XSS 净化 + 热度公式 + 全文搜索）
- `Comment` → `comment.Comment`（linear/nested 双模式 + 树形重建 + 软删除）
- `ShareLink` → 未实现（当前通过 `is_public` session 替代，足够 MVP）
- 新增 `reaction.Reaction`（game/comment/post 通用点赞/收藏）
- 新增 `comment.GameCommentConfig`（评论区配置，含 default_mode）

---

## TH 功能对照与取舍

### WE 已对齐（核心功能同等能力）

| TH 功能 | WE 实现 | 差异说明 |
|---------|---------|---------|
| Session / Floor / MessagePage 三层 | ✅ | 对等 |
| Swipe 多页选择 | ✅ `PATCH .../pages/:pid/activate` | 对等 |
| Director / Verifier / Narrator 槽 | ✅ `ResolveForSlot` | 对等 |
| Prompt Pipeline + Block IR | ✅ 优先级排序 | **WE 更灵活** |
| 世界书（全部触发规则 + 递归激活）| ✅ | 对等 |
| Memory 衰减注入（半衰期）| ✅ | 对等 |
| 结构化 Memory 整合（JSON facts）| ✅ | 对等 |
| Tool Registry + Agentic Loop | ✅ 最多 5 轮 | 对等 |
| ResourceToolProvider | ✅ 14 工具 | WE 略少，按需扩展 |
| Preset Tool（HTTP 回调）| ✅ | 对等 |
| ToolExecutionRecord | ✅ | 对等 |
| PromptSnapshot（命中词条 + verifier）| ✅ | 对等 |
| ST 预设 / 世界书 / 角色卡导入 | ✅ | 对等 |
| 游戏包打包/解包 | ✅ game-package.json | **WE 独有** |
| ScheduledTurn（NPC 自主回合）| ✅ variable_threshold | **WE 独有** |
| 素材库 + search_material | ✅ | **WE 独有** |
| Session Fork（批量平行时间线）| ✅ 创建新 Session | WE 语义更强，但无 branch_id |
| Memory Edge 关系图 | ✅ | 对等 |
| Worldbook 互斥分组 | ✅ | 对等 |
| Worldbook 变量门控 | ✅ | **WE 独有**（TH 无此设计）|
| Memory 分阶段标签（stage_tags）| ✅ | **WE 独有**（TH 无此设计）|
| at_depth 世界书注入位置 | ✅ M12 | 对等 |
| 并发生成保护（reject/queue 模式）| ✅ M13 | 对等 |
| Impersonate（AI 帮答）| ✅ 全管线 Suggest | **WE 更强**（ST 纯客户端，WE 服务端全管线）|

### WE 计划做（Phase 4 / 5）

| 功能 | 计划编号 | 预期阶段 |
|------|---------|---------|
| API Key AES-256-GCM 加密 | P-4A ✅ | Phase 4（第一批，已完成） |
| JWT Auth | P-4B ✅ | Phase 4（第一批，已完成） |
| 多 Provider 原生适配（Anthropic / Gemini）| P-4C ✅ | Phase 4（第二批，已完成） |
| OpenAPI 文档（Swagger）| P-4D ✅ | Phase 4（第二批，已完成） |
| 对话导入/导出（ST JSONL + WE 原生格式）| P-4E ✅ | Phase 4（第二批，已完成） |
| 双层记忆压缩（micro / macro）| P-4F | Phase 4（第三批） |
| Background Job Runtime（DB 持久化）| P-4G ✅ | Phase 4（第二批，已完成） |
| Floor Run Phase SSE（生成阶段推送）| P-4H | Phase 4（第三批） |
| VN 渲染资产系统 | P-4I | Phase 4（第三批） |
| game_loop.go 拆分（Pipeline Context 提取）| P-4J | Phase 4（第三批，P-4K/P-4L 前置） |
| 宏注册表重构 + 完整宏集合 | P-4K | Phase 4（第三批） |
| Preflight Rendering（预飞 + 完整预生成）| P-4L | Phase 4（第三批，依赖 P-4J） |
| 包结构治理 | P-5A | Phase 5 |
| 官方集成包（@gw/sdk + @gw/play-helpers）| P-5B | Phase 5 |
| Social 层（Post / Comment / ShareLink）| P-5E ✅ | Phase 5（已提前完成）|

### WE 明确不做

| 功能 | 不做原因 |
|------|---------|
| **Character 版本管理（rollback）** | WE 用游戏包版本控制游戏内容；角色卡跟随游戏包迭代，不需要 session pin 到角色版本 |
| **Mutation Runtime / MutationBatch / ToolMutationBuffer** | TH 的通用变更执行引擎适合插件化 SaaS；TH 的 `ToolMutationBuffer` 将工具变量写入缓冲到 floor commit，WE 工具直接写入 sandbox 更简单且够用；审计需求通过 PromptSnapshot 覆盖 |
| **llm_instance_config 独立表** | TH 为多账户 SaaS 场景设计；WE 的 LLMProfileBinding.Params 合并了两者，单租户场景无需拆分 |
| **WebSocket Event Bus（50+ 事件）** | WE 的 SSE 已覆盖前端实时需求；Event Bus 适合插件/监控生态，WE 当前没有插件扩展需求 |
| **Account User Binding 深度绑定** | WE 通过 `user_id` 字段简化处理，不需要 TH 的 account_user + session.user_snapshot 完整方案 |
| **记忆维护 CLI（dry-run 脚本）** | WE 有 `POST /sessions/:id/memories/consolidate` API 触发，CLI 对当前部署方式意义不大 |
| **OpenAPI 中英文文档站（VitePress）** | 面向创作者的文档是 CW 职责，WE 引擎只需 Swagger UI 供开发联调 |
| **真实 provider 最小回归 CI 脚本** | WE 用手动冒烟 + `.env` 覆盖；自动化回归脚本适合 monorepo + CI 场景 |
| **WebGal 式 KV + 状态机（确定性数值）** | 适合严格规则型跑团；WE 面向开放叙事，精确数值可用 Director 槽计算、Narrator 槽叙事渲染 |
| **Floor Run State 精细状态机（DB 持久化）** | TH 记录每个生成阶段到 DB，适合多副本调试；WE 用 SSE phase 事件（P-4H）替代，不持久化到 DB |
| **真正的实时多人联机（Room/分布式状态）** | 需要 WebSocket + 分布式锁 + conflict resolution；WE 的四种轻量多人模式已覆盖叙事游戏多人需求 |
| **lorebook_overrides（角色卡覆盖全局世界书）** | WE 导入时直接合并进 game_id 下的 WorldbookEntry，游戏包即唯一版本，不区分来源优先级 |

---

## 候选 / 延后项

| 编号 | 内容 | 状态 |
|------|------|------|
| P-5X2 | Director → 世界书激活控制（语义替代关键词匹配）| 候选 |
| P-5X3 | 玩家专属进化世界书（边界归档→衍生词条→下局注入）| 候选 |
| P-5X4 | 角色卡游玩模式（无游戏包，纯角色卡+系统提示）| P-3I 完成后可实现 |
| P-5X5 | MVM 渲染层（游记导出 vn-full/narrative/minimal）| 候选 |
| P-5X6 | Soft-supersede（重生成时旧 Floor 标记 `superseded` 而非覆盖，保留时间线完整性）| 候选，参照 TH Beta3 |
| P-5X7 | alternateGreetings 多开场白（角色卡 `alternate_greetings` 导入为 floor 0 的多个 swipe page）| 候选，ST 兼容性 |
