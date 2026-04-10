# WorkshopEngine — 整体开发计划

> 版本：2026-04-10
> 来源：`implementation-plan.md` + `PROGRESS.md` 合并整理
> 编号规则：`P-<阶段><序号>` — 例：`P-1A`（Phase 1，第 A 项）

---

## 快速状态总览

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | 核心基础设施 | ✅ 全部完成 |
| Phase 2 | 工具 + 多槽 LLM + 创作层 | ✅ 全部完成 |
| Phase 3 | 引擎能力补全 | ✅ M11–M14 全部完成 |
| Phase 4 | 安全 + 平台工程 | 📋 规划中 |
| Phase 5 | 集成包 + 社交层 + 文档 | ⚡ Social 层已提前完成（GW 平台层），其余规划中 |

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

## Phase 4 — 安全与平台工程 📋

> **执行顺序说明：** P-4A 和 P-4B 是其他所有 Phase 4 工作的硬性前置条件，必须第一批完成。其他条目可在此之后并行推进。

### 第一批（上线阻断，必须先完成）

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

### 第二批（平台工程，可并行）

### P-4C  多 Provider 原生适配 ⬜

**目标：** 接入 Anthropic 原生 API（claude-opus/sonnet/haiku）；Google Gemini 按需。
**参照：** TH `packages/core/src/llm/provider-registry.ts`

---

### P-4D  OpenAPI 文档（swaggo）⬜

**目标：** 从代码注释自动生成 Swagger UI，优先覆盖游玩层路由。
**参照：** TH `apps/api/src/plugins/openapi.ts`（Fastify openapi plugin）

---

### P-4E  对话导入 / 导出 ⬜

**目标：** ST 格式（.jsonl）互转，供玩家备份存档或跨平台迁移。
导出格式：`.thchat`（WE 原生）+ `.jsonl`（ST 兼容）。
**参照：** TH `apps/api/src/services/chat-export.ts` + `chat-import-manifest.ts` + `routes/exports.ts` + `routes/imports.ts`

---

### P-4G  Background Job Runtime（DB 持久化）⬜

**为什么做：** 进程重启时丢失未完成记忆整合任务是确定性的生产问题（服务器重启、OOM kill、部署更新），不是低概率风险。

**具体工作：**
1. 新增 `runtime_job` 表：`id`, `type`（`memory_consolidation`/`macro_compaction`/`archive`）, `session_id`, `payload JSONB`, `status`（`queued → leased → done / failed / dead`）, `lease_until`, `retry_count`, `error_log`
2. `internal/engine/scheduler` — 替换现有 goroutine 为 job worker：`EnqueueJob` + `LeaseJob` + `CompleteJob`/`FailJob`
3. 进程启动时执行 lease 恢复：将 `status=leased AND lease_until < now()` 的任务重置为 `queued`
4. `dead letter` 策略：`retry_count >= 3` 时标记为 `dead`，写入 error_log，不再重试

**改动量：** DB 迁移 + scheduler 重写约 200 行。

**参照：** TH `apps/api/src/services/runtime-worker.ts` + `runtime-job-scheduler.ts` + `drizzle/0024_background_job_runtime.sql`

---

### 第三批（体验增强）

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

## Phase 5 — 集成包 + 社区层 + 架构治理 📋

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

禁止带副作用的宏（`{{setvar}}`）在 Verifier 阶段执行（只读上下文）。

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
| Social 层（Post / Comment / ShareLink）| P-5E ✅ | Phase 5（已提前完成）|
| Session 内分支（branch_id）| P-3G ✅ | Phase 3（已完成）|

### WE 明确不做

| 功能 | 不做原因 |
|------|---------|
| **Character 版本管理（rollback）** | WE 用游戏包版本控制游戏内容；角色卡跟随游戏包迭代，不需要 session pin 到角色版本 |
| **Mutation Runtime / MutationBatch** | TH 的通用变更执行引擎适合插件化 SaaS；WE 的简单直接写入足够；审计需求通过 PromptSnapshot 覆盖 |
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
| P-5X1 | Preflight Rendering（异步预生成 3 个选项，零等待点选）| 候选 |
| P-5X2 | Director → 世界书激活控制（语义替代关键词匹配）| 候选 |
| P-5X3 | 玩家专属进化世界书（边界归档→衍生词条→下局注入）| 候选 |
| P-5X4 | 角色卡游玩模式（无游戏包，纯角色卡+系统提示）| P-3I 完成后可实现 |
| P-5X5 | MVM 渲染层（游记导出 vn-full/narrative/minimal）| 候选 |
