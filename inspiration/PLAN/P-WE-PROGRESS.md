# WorkshopEngine — 里程碑进度记录

> 最后更新：2026-04-08（M13 并发生成保护完成）
> 代码库：`backend-v2/`，主语言：Go 1.22 + Gin + GORM + PostgreSQL

WorkshopEngine (WE) 是面向 AI 文字游戏的无头 REST API 引擎。
本文记录各里程碑的完成状态，是接手新开发者的第一读物。

---

## 系统定位

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
│   │   └── tokenizer/  Token 估算（BPE 兼容启发式）
│   ├── platform/
│   │   ├── auth/       账户中间件（X-Account-ID，Phase 4-B 迁移为 JWT）
│   │   └── provider/   LLM Profile 注册表（slot 优先级解析）
│   ├── engine/
│   │   ├── api/        HTTP 层（PlayTurn / StreamTurn / PromptPreview 等）
│   │   ├── macros/     宏展开（{{char}}/{{user}}/{{getvar::key}} 等）
│   │   ├── memory/     记忆存取 + 异步整合 Worker
│   │   ├── parser/     AI 响应解析（三层回退：XML → 编号列表 → fallback）
│   │   ├── pipeline/   Prompt 组装流水线（PromptBlock IR）
│   │   ├── processor/  Regex 后处理（user_input / ai_output / all）
│   │   ├── prompt_ir/  流水线上下文类型（ContextData / PromptBlock / ...）
│   │   ├── scheduled/  ScheduledTurn 触发规则求值
│   │   ├── session/    Session/Floor/Page ��态机
│   │   ├── tools/      工具注册表 + 内置工具 + ResourceToolProvider
│   │   ├── types/      共享消息类型
│   │   └── variable/   五层变量沙箱
│   ├── creation/
│   │   ├── api/        HTTP 层（模板 / 角色卡 / 世界书 / 素材等 CRUD）
│   │   ├── asset/      素材上传与管理
│   │   ├── card/       角色卡 PNG 解析（ST V2/V3）
│   │   ├── lorebook/   世界书管理
│   │   └── template/   游戏模板 CRUD + 游戏包导入导出
│   ├── social/         (空，Phase 5-E 启动)
│   └── integration/    (仅集成测试，Phase 5-A 迁移整理)
└── docs/
    ├── database.md     DB schema 字段说明
    └── architecture.md 架构决策文档（本文配套）
```

---

## 关键 API 端点速查

### 游玩层 `/api/v2/play`

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/games` | 已发布游戏列表（公开字段） |
| POST | `/sessions` | 创建会话（自动注入 first_mes） |
| POST | `/sessions/:id/turn` | 同步一回合 |
| POST | `/sessions/:id/regen` | 重新生成（Swipe） |
| GET | `/sessions/:id/stream` | SSE 流式一回合 |
| GET | `/sessions/:id/state` | 会话状态快照 |
| PATCH | `/sessions/:id` | 更新会话标题/状态 |
| DELETE | `/sessions/:id` | 删除会话及关联数据 |
| GET | `/sessions/:id/variables` | 变量快照 |
| PATCH | `/sessions/:id/variables` | 合并更新变量 |
| GET | `/sessions` | 列出会话（?game_id=&user_id=&limit=&offset=） |
| GET | `/sessions/:id/floors` | 楼层列表（含激活页摘要） |
| GET | `/sessions/:id/floors/:fid/pages` | Swipe 页列表 |
| PATCH | `/sessions/:id/floors/:fid/pages/:pid/activate` | Swipe 选页 |
| GET | `/sessions/:id/memories` | 记忆列表 |
| POST | `/sessions/:id/memories` | 手动创建记忆（创作者/调试用） |
| PATCH | `/sessions/:id/memories/:mid` | 更新记忆字段 |
| DELETE | `/sessions/:id/memories/:mid` | 删除记忆（?hard=true 物理删除） |
| POST | `/sessions/:id/memories/consolidate` | 立即触发记忆整合（同步，调试用） |
| POST | `/sessions/:id/fork` | 分叉会话（平行时间线 / 存档点） |
| GET | `/sessions/:id/prompt-preview` | Prompt dry-run（不调用 LLM） |
| GET | `/sessions/:id/floors/:fid/snapshot` | Prompt 快照（Verifier 结果 + 命中词条） |
| GET | `/sessions/:id/tool-executions` | 工具执行记录（?floor_id=&limit=） |
| GET | `/sessions/:id/memory-edges` | 记忆关系边列表 |
| GET | `/sessions/:id/memories/:mid/edges` | 某记忆的所有双向边 |
| POST | `/sessions/:id/memory-edges` | 手动创建关系边 |
| PATCH | `/sessions/:id/memory-edges/:eid` | 修改 relation 类型 |
| DELETE | `/sessions/:id/memory-edges/:eid` | 删除关系边 |

### 创作层 `/api/v2/create`

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

---

## 里程碑记录

### [M1] 核心基础设施 ✅

**目标：** 可工作的 HTTP 服务器，具备 DB 连接、LLM 调用能力、账户隔离。

**完成内容：**
- PostgreSQL + GORM，AutoMigrate，全部模型定义
- OpenAI 兼容 LLM 客户端（Chat + ChatStream SSE）
- LLM Profile 动态解析，slot 优先级系统（session > global，精确 slot > 通配 `*`）
- X-Account-ID 账户中间件
- Gin 路由框架，`/api/v2/play` + `/api/v2/create` 双命名空间

---

### [M2] Prompt 流水线 ✅

**目标：** 可组装高质量上下文的无副作用 Prompt Pipeline，不依赖任何 HTTP 调用。

**完成内容：**
- `PromptBlock IR`：每节点产出带 Priority 的 Block，Runner 统一按 Priority 排序后输出消息列表
- 流水线节点：TemplateNode / WorldbookNode / MemoryNode / HistoryNode / PresetNode
- 世界书触发逻辑：主关键词 + 次级关键词 + 逻辑门（AND_ANY/AND_ALL/NOT_ANY/NOT_ALL）+ 正则模式 + 全词匹配 + 扫描深度 + 常驻词条 + **递归激活**
- 注入位置：`before_template` / `after_template` / `at_depth`（Priority 映射）
- Worldbook 互斥分组（`applyGroupCap`）
- Worldbook Token Budget（`applyTokenBudget`，按 Priority 贪心保留，Constant pinned）
- Worldbook 变量门控（`var:key=value` / `var:key!=value` / `var:key` 硬条件）
- 宏展开节点集成（PresetNode / WorldbookNode / TemplateNode 均调用 `macros.Expand()`）

---

### [M3] 变量沙箱 ✅

**目标：** 五层变量隔离，Page 级修改不影响上层，CommitTurn 时提升为 Chat 级。

**完成内容：**
- `variable.Sandbox`：五级（Global → Chat → Floor → Branch → Page）级联读取
- `Sandbox.Set()` 写入 Page 层；`Sandbox.Flatten()` 输出合并后的完整快照
- CommitTurn 将 `Flatten()` 结果持久化到 `MessagePage.PageVars`，再提升到 `GameSession.Variables`
- `GET /sessions/:id/variables` + `PATCH /sessions/:id/variables`

---

### [M4] Session 状态机 ✅

**目标：** Floor/Page 双层结构，支持多版本（Swipe）和会话分叉。

**完成内容：**
- `session.Manager`：StartTurn / CommitTurn / RegenTurn / FailTurn
- Floor 状态枚举：`generating` → `committed` / `failed`
- MessagePage：每个 Floor 可有多个 Page（Swipe），只有一个 `is_active`
- `PATCH /sessions/:id/floors/:fid/pages/:pid/activate`（Swipe 选页）
- `POST /sessions/:id/fork`（平行时间线，复制 [1..seq] 的 Floor/Page 到新 Session）
- `GET /sessions/:id/floors`（含激活页摘要）
- SSE 流式输出（`GET /sessions/:id/stream`）

---

### [M5] 解析器 + Regex 后处理 ✅

**目标：** AI 响应结构化解析，支持 VN 指令，Regex 后处理管道。

**完成内容：**
- `parser.Parse()`：三层回退（`<game_response>` XML → 编号列表 → fallback 纯文本）
- VN 指令解析：`[bg|名]` / `[bgm|名]` / `[sprite|名|表情|位置]` / `[choice|A|B|C]` / `[cg|名]` / `[hide_cg]`
- 对话格式解析：`角色|情绪|台词` → `{type:dialogue, speaker, text}`
- `processor.ApplyToUserInput()` / `ApplyToAIOutput()`（Regex 规则管道）
- Prompt Dry-Run：`GET /sessions/:id/prompt-preview`

---

### [M6] 记忆系统 ✅

**目标：** 长期记忆注入，异步整合，时间衰减排序，结构化 Fact 管理。

**完成内容：**
- `memory.Store`：记忆 CRUD + 生命周期（active → deprecated → purged）
- 指数半衰期衰减排序（`GetForInjection` 按 importance × decay 得分排序）
- Memory 分阶段标签（`stage_tags`，按 `game_stage` 变量过滤注入）
- `Memory Worker`：每 N 回合异步触发，调用廉价模型提取 `{turn_summary, facts_add, facts_update, facts_deprecate}`
- 结构化整合输出（`ParseConsolidationResult`，`fact_key` 系统，upsert/deprecate）
- `MemoryEdge` 表：`updates` / `contradicts` / `supports` / `resolves` 关系图
- `POST /sessions/:id/memories/consolidate`（手动触发，调试用）

---

### [M7] 工具系统 ✅

**目标：** LLM Agentic 工具调用，内置工具 + 创作者自定义工具 + 资源读写工具。

**完成内容：**
- `tools.Registry`：工具注册、定义序列化（`ToLLMDefinitions()`）、带审计的执行（`ExecuteAndRecord()`）
- ReplaySafety 等级：`safe` / `confirm_on_replay` / `never_auto_replay` / `uncertain`
- Agentic Loop：最多 5 轮工具调用（无工具时单次直通）
- 内置工具：`get_variable` / `set_variable` / `search_memory` / `search_material`
- `ResourceToolProvider`：14 个 AI 可操作资源工具（worldbook / preset / memory / material 读写）
- `Preset Tool`：创作者自定义 HTTP 回调工具（`preset:*` / `preset:<name>` 动态加载）
- `ToolExecutionRecord`：DB 持久化，`GET /sessions/:id/tool-executions` 查询

---

### [M8] 多角色 LLM 槽 ✅

**目标：** Director / Verifier / Narrator 三槽，廉价模型分工，精确 token 计数。

**完成内容：**
- LLM Profile CRUD + `POST /llm-profiles/:id/activate`（绑定到 slot）
- `provider.Registry.ResolveForSlot()`：5 级优先级（session slot X → global slot X → session * → global * → env 兜底）
- **Director 槽**：廉价模型预分析上下文，结果插入主 LLM 消息首位；失败静默跳过
- **Verifier 槽**：主生成后一致性校验；失败不阻断回合，只影响 PromptSnapshot 标记
- **PromptSnapshot**：每 Floor 异步写入，记录 ActivatedWorldbookIDs / preset_hits / worldbook_hits / est_tokens / verifier 结果
- 精确 token 计数：SSE `stream_options.include_usage`，三通道返回
- LLM 模型发现（`POST /llm-profiles/models/discover`）+ 连通性测试（`/models/test`）

---

### [M9] 创作工具层 ✅

**目标：** 完整的游戏包创作工具链，ST 格式兼容导入，ScheduledTurn，素材库。

**完成内容：**
- 角色卡 PNG 导入（ST V2/V3）+ 结构化存储
- 游戏包打包/解包：`POST /templates/import` + `GET /templates/:id/export`（game-package.json 格式）
- ST 预设批量导入：`POST /templates/:id/preset/import-st`
- ST 世界书批量导入：`POST /templates/:id/lorebook/import-st`
- PresetEntry CRUD + reorder
- Regex Profile CRUD
- 素材库 CRUD + 批量导入 + `search_material` AI 工具
- **ScheduledTurn**：`variable_threshold` 触发模式，Cooldown 持久化到 session.variables
- 模板发布状态机：`draft → published`
- **AI 辅助创作**（creation-agent）：`resource:*` 工具对话式修改游戏规则
- 边界归档 API：`POST /sessions/:id/archive`

---

### [M10] 宏展开 ✅

**目标：** ST 宏系统基础兼容，`{{char}}/{{user}}/{{getvar::key}}` 在 Pipeline 组装时展开。

**完成内容：**
- `internal/engine/macros/expand.go`：`MacroContext` + `Expand(text, ctx)` 函数
- 支持宏：`{{char}}` / `{{user}}` / `{{persona}}` / `{{getvar::key}}` / `{{time}}` / `{{date}}`
- 注入点：TemplateNode / PresetNode / WorldbookNode 均在输出内容前调用 `macros.Expand()`
- `GameTemplate.Config` 新增 `char_name` / `player_name` / `persona_name` 字段
- 废弃各 Node 内的零散 `resolveMacros` / `presetResolveMacros` 函数

---

### [M11] 角色注入管线 ✅

将 `CharacterCard` 内容自动注入为系统提示（CharacterInjectionNode），支持 `pin`/`latest` 同步策略。

**完成内容：**
- `GameSession` 新增 `character_card_id`、`character_snapshot` 字段（DB AutoMigrate）
- `GameTemplate.Config` 新增 `character_card_id`、`character_sync_policy`（`pin`/`latest`）字段
- `CreateSession` 在 `pin` 策略下于会话创建时冻结角色卡快照到 `character_snapshot`
- `CharacterInjectionNode`（`internal/engine/pipeline/node_character.go`）：Priority=9，位于 TemplateNode(0) 之后、WorldbookNode(10+) 之前
- `buildCharacterDescriptionFromCardData`：提取 ST V2/V3 角色卡的 description / personality / scenario 字段
- `loadCharacterDescription`：自动判断 pin（用快照）/ latest（查 DB）策略
- 内容在注入前经过 `macros.Expand()`，支持 `{{char}}` / `{{user}}` / `{{persona}}` 宏
- 修复 game_loop.go 中 worldbook IR 转换缺失 `Group`/`GroupWeight` 字段的 bug
- 三条代码路径均已更新：PlayTurn / StreamTurn / PromptPreview

---

### [M12] at_depth 世界书注入位置 ✅

将 `position=at_depth` 的世界书词条插入对话历史的特定深度，完整对齐 SillyTavern 行为。

**完成内容：**
- `WorldbookEntry` DB 模型（`models.go`）新增 `Depth int` 字段（GORM AutoMigrate）
- IR `WorldbookEntry`（`prompt_ir/pipeline.go`）新增 `Depth int` 字段
- 新增 `AtDepthBlock` 类型和 `ContextData.AtDepthBlocks []AtDepthBlock`（在 prompt_ir 层）
- `WorldbookNode`（`node_worldbook.go`）：`at_depth` 词条不再走 `ctx.Blocks` 排序路径，改为追加到 `ctx.AtDepthBlocks`
- `Runner.Execute`（`runner.go`）：在普通 Blocks 组装完成后，对每个 `AtDepthBlock` 计算插入位置 `idx = clamp(len(msgs) - depth, 1, len(msgs))`，从最大 idx 开始插入（避免位移），同深度时按 Priority 升序
- ST 世界书导入（`import_export.go`）：新增 ST `position`（0–4 数字）→ WE 字符串映射，`depth` 字段透传
- `game_loop.go` + `engine_methods.go`：WorldbookEntry → IR 转换均补全 `Depth` 字段

---

### [M13] 并发生成保护 ✅

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

## 待办路线图

| 里程碑 | 关键内容 | 阶段 |
|--------|---------|------|
| M11 ✅ | 角色注入管线（CharacterCard → System Prompt，pin/latest 策略） | Phase 3 |
| M12 ✅ | at_depth 世界书注入位置（Depth 字段 + Runner 动态插入）| Phase 3 |
| M13 ✅ | 并发生成保护（Generating 字段 + FOR UPDATE + reject 模式）| Phase 3 |
| M14 | Session 内分支（branch_id，Floor 内多时间线） | Phase 3 |
| M15 | API Key AES-256-GCM 加密 | Phase 4 |
| M16 | JWT Auth（X-Account-ID → Bearer token） | Phase 4 |
| M17 | Floor Run Phase SSE（生成阶段实时推送） | Phase 4 |
| M18 | 双层记忆压缩（micro / macro Memory 类型） | Phase 4 |
| M19 | Background Job Runtime（runtime_job 表，进程重启安全） | Phase 4 |
| M20 | VN 渲染资产系统（Material.filename + 上传 API + 资产 URL 解析）| Phase 4 |
| M21 | 官方集成包（@gw/sdk + @gw/play-helpers） | Phase 5 |
| M22 | Social 层（Post / Comment / ShareLink） | Phase 5 |
| M23 | 宏注册表重构 + 完整 ST 宏集合 | Phase 5 |
