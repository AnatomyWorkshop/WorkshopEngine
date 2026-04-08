# TavernHeadless Beta3 架构分析：Integration Kit / 聚合根 / Runtime Substrate

> 时间：2026-04-08
> TH 版本：`bdafb0a`（2026-04-07），标签 v0.2.0-beta.3
> 本文分析范围：Beta3 更新内容、官方 Integration Kit 设计、聚合根语义、Runtime Substrate 三层、对 WE 的参考意义、WE 引擎解耦状态。

---

## 一、TH Beta3 更新内容（2026-04-04 ~ 2026-04-07）

### 1.1 关键 commit

| 日期 | commit | 内容 |
|------|--------|------|
| 2026-04-07 | bdafb0a | **Beta3 docs 发布**：Integration Kit 文档整合进 Vitepress guide |
| 2026-04-07 | 5fa511a | **feat(beta3): close plan implementation and sync docs**（160 文件变更） — Beta3 正式封板 |
| 2026-04-06 | (series) | **Character Card V3 export**：`characterBook`（worldbook）合并进角色卡导出；`alternateGreetings` 作为 floor 0 的 swipe pages；`systemPrompt` / `postHistoryInstructions` 字段注入 Preset Entry |
| 2026-04-05 | (series) | **Native Runtime Semantics**：TurnOrchestrator 执行语义稳定，`floor.finalState = "committed"` 作为公开提交边界信号 |
| 2026-04-05 | (series) | **Worldbook `position=outlet` 修复**：`at_depth` 注入现在正确按 `scan_depth` 计算，outlet 位置可用 |
| 2026-04-04 | (series) | **Dry-run match trace**：`debugOptions.includeWorldbookMatches` 暴露触发词条调试信息，`dry-run` 端点现在返回完整 match trace |
| 2026-04-04 | (series) | **Soft-supersede for regenerated floors**：重新生成时旧 Floor 状态置 `superseded`（软删除），不物理删除，保留时间线完整性 |

### 1.2 Beta3 完成标准（14 / 14 全达成）

TH PROGRESS.md 中的 Beta3 Ready 标准：

1. `floor_run_state` 精细状态机（running/completed/failed/cancelled/superseded）
2. Branch-scoped variables（分支级变量）
3. Secret 存储加密（API Key AES）
4. Runtime Job 系统（queued → leased → running → succeeded / dead_letter）
5. Mutation Runtime（变更执行器 + 并发 CAS）
6. Deferred MCP tool execution（用户授权 → 延迟执行）
7. Character Card V3 export（含 characterBook）
8. Worldbook outlet position 修复
9. Dry-run match trace
10. Soft-supersede for regenerated floors
11. Official Integration Kit 文档发布
12. @tavern/sdk 正式版（typed resource methods + 版本并发控制）
13. @tavern/client-helpers 正式版（stream state reducer）
14. Architecture.md 文档完整

---

## 二、官方 Integration Kit

### 2.1 组成

TH 官方 Integration Kit = 两个 npm 包 + 一个内部共享包：

```
packages/official-integration-kit/
├── sdk/            → @tavern/sdk（HTTP 客户端）
├── client-helpers/ → @tavern/client-helpers（状态规范化工具）
└── （shared → @tavern/shared，内部包，不对外发布）
```

### 2.2 @tavern/sdk — HTTP 传输层

**定位：** 替代手写 fetch + SSE 的类型化 HTTP 客户端。

**核心能力：**

| 能力 | 说明 |
|------|------|
| 资源方法 | `sessions.create()`, `sessions.turn()`, `floors.list()`, `pages.activate()`, `worldbook.entries.list()` 等 typed CRUD |
| SSE 流式支持 | `sessions.turn()` 返回 `TurnStream`，自动解析 `event: token / done / error` |
| 版本并发控制 | 关键写操作（turn、page activate）接受 `expectedVersion`（乐观锁），服务端校验后 409 冲突返回 |
| 错误类型体系 | `TavernError`（基类）→ `ConflictError`（409）/ `NotFoundError`（404）/ `ValidationError`（422）等 |
| 重试策略 | 内置指数退避 + 抖动，可自定义 `maxRetries` 和 `retryCondition` |

**SDK 典型用法：**
```typescript
import { TavernClient } from '@tavern/sdk'

const client = new TavernClient({ baseUrl: 'http://localhost:3000', apiKey: '...' })

const session = await client.sessions.create({ templateId: 'game-001' })
const stream  = await client.sessions.turn(session.id, { content: '你好' })

for await (const event of stream) {
  if (event.type === 'token') process.stdout.write(event.data)
  if (event.type === 'done')  console.log('meta:', event.meta)
}
```

### 2.3 @tavern/client-helpers — 状态规范化

**定位：** 规范化使用层（组件/视图层），不负责 HTTP，负责把 SDK 返回的原始数据组装成可渲染的视图状态。

**核心能力：**

| 工具 | 说明 |
|------|------|
| `buildTimeline()` | 把 `floors[]` 展开为平坦时间线（含 branch 合并），UI 无需理解 floor/page 层次 |
| `StreamStateReducer` | SSE token 流 → `{ buffer: string, parsed: VNLines[], completed: bool }` 状态机 |
| `selectActivePage()` | 自动选出每个 floor 的 active page（即当前 swipe 位置） |
| `normalizeUsage()` | 把 API usage（prompt/completion tokens）规范化为统一格式 |

**设计原则：** client-helpers 是**纯函数工具集**，无副作用，无 HTTP 调用，便于框架无关地在 Vue/React/Svelte/原生中使用。

### 2.4 WE 参考

WE 当前无 SDK。对应参考：

| TH SDK 能力 | WE 现状 | 建议 |
|-------------|---------|------|
| typed HTTP client | `test-plan/from-traeCN/src/api/client.js`（手写 fetch + SSE） | 暂够用；API 稳定后可用 codegen 或手写 TS SDK |
| StreamStateReducer | `GamePlay.vue` 中的 `_twQueue` + `parseVN()` | 已实现等价功能，可提取为 composable（Tier 3）|
| buildTimeline | `GamePlay.vue` 中 `mounted()` 的 `paired` 构建逻辑 | 同上，可提取 |
| 版本并发控制 | 无 | 低优先级，当前单用户场景不需要 |

---

## 三、聚合根（Aggregate Root）

### 3.1 概念来源

来自 DDD（领域驱动设计）：**聚合根**是一组关联对象（聚合）的唯一外部访问入口，外部只能通过聚合根修改聚合内部的状态。

### 3.2 TH 中的聚合根

**`Session` 是 TH 的聚合根。**

Session 聚合边界（owned entities）：

```
Session
├── Floor[]          # 对话楼层（ordered by seq_num）
│   └── Page[]       # 每层的多版本（swipe pages）
│       └── Message[]# 消息（user / assistant）
├── Variable[]       # 五层变量快照
├── Memory[]         # 记忆（含 MemoryEdge）
└── FloorRunState    # 最新回合执行状态
```

**聚合规则：**
- 外部代码**不能直接修改 Floor 或 Page**，只能通过 Session 方法（如 `Session.CommitTurn()`）
- `floor.finalState = "committed"` 是**聚合提交边界信号**：所有写操作（prompt snapshot、tool records、variable flush、memory consolidation trigger）在这个状态之后才对消费者可见
- 乐观锁 `expectedVersion` 保护 Session 整体版本，防止并发 turn 产生竞态

### 3.3 WE 的聚合根对应

WE 中隐含的聚合边界：

| TH 聚合根 | WE 对应 |
|-----------|---------|
| Session | `GameSession`（`internal/engine/models/`）|
| Session.CommitTurn() | `CommitTurn()` in engine pipeline |
| floor.finalState | 无精细状态机（仅 `is_generated bool`）|
| expectedVersion（乐观锁）| 无 |

**WE 当前的简化合理：** 目前是单用户、单回合串行，聚合边界不需要严格防护。中期加 JWT Auth 后，若支持多用户并发游玩同一个 Session（多人游戏），需引入乐观锁。

**对 WE 设计的启示：** 在 API 层面，保持"所有 Session 内部写操作都通过 `/sessions/:id/turn` 入口"的原则，避免直接暴露 Floor/Page/Variable 的 PATCH 绕过事务边界。

---

## 四、Runtime Substrate（运行时底层）

### 4.1 定义

TH 把"保证长时运行操作在 HTTP 生命周期之外存活"的基础设施统称为 **Runtime Substrate**。由三个子系统构成：

```
Runtime Substrate
├── runtime_job 表         # 持久化 Job 队列（进程重启不丢）
├── floor_run_state 状态机  # Turn 执行阶段精细追踪
└── RuntimeRevisionGuard   # CAS 并发写入保护
```

### 4.2 runtime_job 表

**解决的问题：** 异步任务（Memory 整合、MCP 工具延迟执行）使用 goroutine/setTimeout 会在进程崩溃时丢失。

**设计：**
```sql
CREATE TABLE runtime_job (
  id          TEXT PRIMARY KEY,
  type        TEXT NOT NULL,          -- 'memory_consolidation' | 'mcp_tool_exec' | ...
  payload     JSONB,
  status      TEXT DEFAULT 'queued',  -- queued → leased → running → succeeded / dead_letter
  lease_until TIMESTAMPTZ,            -- 租约到期时间（防僵尸 worker）
  retry_count INT DEFAULT 0,
  created_at  TIMESTAMPTZ,
  updated_at  TIMESTAMPTZ
)
```

**Lease 机制：** Worker 拿到 Job 时写入 `lease_until`（当前时间 + N 秒），持续延续租约；超时未续租的 Job 可被其他 Worker 重新拾取。重试超过阈值进入 `dead_letter`。

### 4.3 floor_run_state 状态机

**解决的问题：** Turn 执行是多阶段异步过程（Director → Memory → Narrator → Verifier → Commit），需要精细追踪当前阶段以便前端实时进度显示、后端 retry/cancel。

**状态迁移：**
```
idle → running(phase=director) → running(phase=generation) → running(phase=verifier) → committed
                                                                                    ↗
                                                                              → superseded（重新生成）
                                                          ↘
                                                    failed → (retry → running)
                                                    cancelled
```

**暴露方式：** `GET /sessions/:id/floors/:fid/run-state` 返回当前 phase + pendingOutput（部分生成内容），前端可轮询显示"Director 分析中…"、"正在生成…"。

### 4.4 RuntimeRevisionGuard

**解决的问题：** 两个并发 HTTP 请求同时触发同一 Session 的 turn，导致 Floor seq_num 冲突或变量竞态。

**实现：** 写 Session 前 CAS（Compare-And-Swap）检查版本号，不匹配返回 409。前端 SDK 用 `expectedVersion` 传入，冲突时重试。

### 4.5 WE 的对应

| TH Runtime Substrate | WE 现状 | 评注 |
|----------------------|---------|------|
| runtime_job 表 | goroutine + 内存 Worker（进程重启丢失）| 够用；Production 级需改为 DB 持久化 |
| floor_run_state | 无精细状态机（仅 SSE phase 事件） | SSE Phase 是轻量替代，前端无法重连后恢复进度 |
| RuntimeRevisionGuard | 无 | 单用户无并发问题，多用户后需要 |

**WE 近期无需做：** 当前是单用户原型，goroutine Worker + SSE 已足够。当 WE 进入"多用户游玩同一 Session"或"生产稳定性"阶段，才需要 runtime_job 表和 floor_run_state 精细状态机。

---

## 五、Prompt / Runtime / State 三层分离

### 5.1 TH 的三层架构

TH architecture.md 明确定义了三个正交层：

```
┌─────────────────────────────────────────────────────┐
│  Prompt Layer       组装送给 LLM 的消息序列            │
│  ─ PromptGraph compiler（native path，主路径）         │
│  ─ PromptIR assembler（ST 兼容路径）                   │
│  ─ Worldbook matching（含 outlet position）           │
│  ─ Regex 后处理（带扫描深度上下文）                     │
└─────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────┐
│  Runtime Layer      执行 + 协调 LLM 调用               │
│  ─ TurnOrchestrator（Director→Memory→Narrator→Verifier→Commit）│
│  ─ Tool calling（inline / deferred async_job）       │
│  ─ MCP Connection Manager（stdio + HTTP）            │
│  ─ floor_run_state 状态机                            │
│  ─ RuntimeRevisionGuard（CAS 并发保护）               │
└─────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────┐
│  State Layer        持久化状态（不含 Prompt 编排）      │
│  ─ 五层变量（global/chat/branch/floor/page）          │
│  ─ Memory V2（micro/macro/fact，lifecycle 状态机）    │
│  ─ Branch-scoped variables                          │
│  ─ Session/Floor/Page 版本链                         │
└─────────────────────────────────────────────────────┘
```

**层间关系：**
- Prompt Layer 只读 State Layer（读变量、读 Memory）
- Runtime Layer 协调 Prompt Layer + State Layer（触发组装、持久化结果）
- State Layer 只被 Runtime Layer 写入（变量/Memory 只在 CommitTurn 时落库）

### 5.2 WE 的三层对应

| TH 层 | WE 对应组件 | 状态 |
|-------|------------|------|
| **Prompt Layer** | `internal/engine/pipeline/` (nodes: PresetEntryNode, WorldbookNode, CharacterInjectionNode, …) | ✅ 已有 PromptBlock IR |
| **Runtime Layer** | `internal/engine/runner/` (TurnRunner, ToolExecutor, AgenticLoop) | ✅ 已有，Director/Verifier slot 刚完成 |
| **State Layer** | `internal/engine/models/` (GameSession, Floor, Variable, Memory) + Workers | ✅ 基本完整 |

**WE 与 TH 三层的最大差异：**

1. **Prompt Layer：** WE 有 **PromptBlock 优先级 IR**（TH 没有），WE 更灵活；但 WE 无 PromptGraph compiler（TH 的新增 native path），WE 用 Runner 顺序组装节点，不支持 DAG 拓扑
2. **Runtime Layer：** WE 无 `floor_run_state` 精细状态机；WE 无 Runtime Job 持久化；WE 无乐观锁
3. **State Layer：** WE Memory 无 `MemoryEdge`（关系图）；WE 无 Branch-scoped variables；WE Memory 无双层摘要（micro/macro）

---

## 六、WE 引擎是否可以脱离界面独立运行

**答案：是的，WE 引擎完全可以脱离前端独立运行。**

**原因：**
- WE 是纯 Go HTTP 服务，使用 Gin 框架，入口在 `cmd/server/main.go`
- 前端代码（`frontend-v2/` 或 `test-plan/from-traeCN/`）是独立 Vite 项目，与后端完全分离
- 后端没有引入任何前端依赖；所有接口都是 REST / SSE
- 可以独立 `go run cmd/server/main.go` 启动，然后用任何 HTTP 客户端（curl、Postman、AI Agent）调用

**验证：** creation-agent（AI 辅助创作）本身就是在无 UI 的情况下通过 API 对话式修改游戏规则，证明引擎可以无界面使用。

**未来场景：**
- 嵌入式 CLI 工具：`we-engine serve --port 8080`，本地单机运行
- Server-side：部署为无状态 API Pod，多实例水平扩展
- Desktop App（Electron）：Electron 主进程启动 Go 子进程，WebView 访问 `http://localhost:8080`

---

## 七、creation / social 等功能的解耦状态

### 7.1 WE 当前包结构

```
internal/
├── engine/          ← 游戏引擎（会话、回合、流式生成）
│   └── api/         ← Game-facing API routes
├── creation/        ← 创作工具（模板、角色卡、世界书、预设、素材）
│   └── api/         ← Creation-facing API routes
├── worker/          ← 后台 Worker（Memory 整合、ScheduledTurn）
└── ...（models、llm、pipeline 等共享包）
```

**social / integration 层：不存在。**
- `internal/social/` → 目录不存在
- `internal/integration/` → 目录不存在

### 7.2 当前解耦程度

| 层 | 状态 | 备注 |
|----|------|------|
| **engine vs creation** | ✅ 独立 Go 包，相互无直接导入 | 共享 `models/` 下的 DB 模型 |
| **engine vs frontend** | ✅ 完全解耦 | 前端是独立 Vite 项目 |
| **engine vs worker** | ⚠️ Worker 与 engine 共享 models + DB | 可接受，Worker 是 engine 的异步延伸 |
| **social 层** | ❌ 不存在 | GW 的帖子/游记/评论未启动 |
| **integration 层** | ❌ 不存在 | 跨服务集成（webhook、event bus）未启动 |

### 7.3 关于路由挂载

目前 `cmd/server/main.go` 把 engine 和 creation 的路由都挂在同一个 Gin Router 上：

```go
gameapi.NewGameEngine(db, cfg).Register(r)
creationapi.NewCreationAPI(db, cfg).Register(r)
```

这是**单体部署**，两者共享 HTTP 服务器和数据库连接池。如果未来需要独立部署（e.g. creation 层高流量），可以拆分为两个 `cmd/server-engine` 和 `cmd/server-creation`，分别打包。当前规模不需要。

### 7.4 与 TH 的对比

TH 采用了更严格的模块边界：
- `packages/core`：引擎核心（无 HTTP 依赖）
- `packages/adapters-sillytavern`：ST 格式适配器
- `apps/api`：HTTP 层（Fastify）

TH 的优势是 core 可以独立单测，无需启动 HTTP 服务器。WE 目前的 `engine/` 包也不强依赖 HTTP（Gin handlers 在 `engine/api/` 内，核心逻辑在 `engine/runner/` 和 `engine/pipeline/`），但没有做到 TH 那样的 monorepo 包级隔离。

---

## 八、对 WE 的综合影响

### 8.1 近期可借鉴（无破坏性变更）

| TH Beta3 特性 | WE 对应行动 | 优先级 |
|---------------|------------|--------|
| **dry-run worldbook match trace** | `GET /sessions/:id/prompt-preview` 已有；可扩展返回 `activatedWorldbookIDs` 和触发详情 | 低（调试工具，已有基础）|
| **Character Card V3 `characterBook` 合并导出** | 角色卡导入已解析 `character_book`；导出 API 可补充此字段 | 低（功能完整性）|
| **soft-supersede for regenerated floors** | 当前重生成直接覆盖；可改为 mark `superseded` 保留历史 | 中（时间线保护）|
| **`alternateGreetings` as floor 0 swipes** | 当前 first_mes 是单一楼层；可扩展为多个 swipe page | 中（ST 兼容）|

### 8.2 中期需做（有明确价值）

| 项目 | 来源 | WE 编号 |
|------|------|---------|
| **宏展开 Macro Expand** | TH 也缺，WE 独立实现 | 3-I.1 |
| **结构化 Memory 整合** | TH Memory V2 参考 | 3-D.1 |
| **MCP 协议接入** | TH Beta3 完整实现，可参考 | 3-H |
| **Session 内分支** | TH branch_id + branch-scoped vars | 3-G |

### 8.3 长期不需要

| TH 特性 | WE 不需要的理由 |
|---------|---------------|
| `@tavern/sdk` npm 包 | WE API 消费方是自己的前端，不是第三方开发者；API 稳定后可按需生成 |
| Mutation Runtime | WE 面向单用户游戏，不需要 UI 可撤销的变更历史 |
| PromptGraph DAG compiler | WE 的 PromptBlock IR + Priority 排序已满足需求 |
| Generation Guard (queue mode) | 单用户游玩无并发排队需求 |

---

## 九、总结

```
TH Beta3 = 稳定性 + 生产就绪 + 开发者友好
  - runtime_job 持久化：进程重启不丢任务
  - floor_run_state 精细状态机：前端可显示生成进度
  - Secret AES 加密：API Key 安全存储
  - Official Integration Kit：@tavern/sdk + @tavern/client-helpers，开发者体验
  - Character Card V3 完整 export：ST 生态闭环

WE 目标不同：游戏发布平台
  - 已有 TH 没有的核心差异化（游戏包、VN 指令、素材库、ScheduledTurn）
  - 近期优先补：3-I.1 宏展开（{{char}}/{{user}}/{{getvar}}，~90 行 Go，无 DB 变更）
  - WE 引擎完全可以脱离界面运行（纯 Go HTTP 服务）
  - creation / social 解耦：engine 和 creation 是独立 Go 包 ✅；social 层尚不存在 ❌
```
