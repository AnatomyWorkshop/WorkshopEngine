# Engine Audit — 后端组件说明与硬编码清单

> 更新日期：2026-04-04（第三轮 — Preset Entry 系统）
> 对照参考：TavernHeadless (TH) TypeScript 实现

---

## 目录

1. [整体架构](#整体架构)
2. [各组件职责](#各组件职责)
3. [硬编码清单](#硬编码清单)
4. [TH 兼容性对照](#th-兼容性对照)
5. [扩充路线图](#扩充路线图)

---

## 整体架构

```
cmd/
  server/main.go          HTTP 服务入口（依赖注入 + 路由注册）
  worker/main.go          异步记忆摘要 Worker（每 30s 扫描）

internal/
  core/
    config/config.go      强类型环境变量配置（唯一入口）
    db/connect.go         GORM 连接 + AutoMigrate
    db/models.go          数据库模型定义
    llm/client.go         OpenAI 兼容 HTTP 客户端（非流式 + SSE）

  engine/                 游戏引擎核心（不混入论坛/创作逻辑）
    api/
      routes.go           /api/v2/play/* 路由注册
      game_loop.go        PlayTurn 主链路（One-Shot LLM）
      engine_methods.go   Session CRUD、变量操作
    pipeline/
      runner.go           Prompt 流水线执行器（按优先级排序 Block）
      node_template.go    System Prompt 展开（宏替换）
      node_worldbook.go   世界书关键词触发注入
      node_memory.go      记忆摘要注入
      node_history.go     近期历史消息注入
    prompt_ir/pipeline.go Prompt IR 类型定义（PromptBlock、GameConfig）
    variable/sandbox.go   五层变量沙箱（Page→Floor→Branch→Chat→Global）
    memory/store.go       记忆 CRUD + 整合触发 + Session 缓存更新
    session/manager.go    Floor/Page 生命周期（StartTurn/CommitTurn/FailTurn）
    parser/parser.go      LLM 响应解析（XML / JSON / Plaintext 三层回退）
    types/messages.go     三层消息结构辅助类型

  creation/               创作工具（角色卡、世界书、模板、素材）
    api/routes.go         /api/v2/create/* 路由
    asset/handler.go      素材上传（图片/音频，10MB 限制）
    card/parser.go        SillyTavern 角色卡 PNG 解析（chara_card_v2/v3）

  user/middleware.go      鉴权中间件（X-Account-ID + X-Api-Key/Bearer）
```

---

## 各组件职责

### `internal/core/llm/client.go`

**职责**：向任何 OpenAI 兼容 API 发送请求。

| 接口 | 说明 |
|------|------|
| `Chat(ctx, messages, opts)` | 非流式，带指数退避重试（429/5xx） |
| `ChatStream(ctx, messages, opts)` | SSE 流式，token channel |

**`Options` 支持字段（v2 扩充后）**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Model` | `string` | 模型 ID，为空时使用 client 默认值 |
| `MaxTokens` | `int` | 最大输出 token 数 |
| `Temperature` | `*float64` | nil = 不发送，让 API 使用默认值 |
| `TopP` | `*float64` | nil = 不发送 |
| `TopK` | `*int` | nil = 不发送（OpenAI 不支持，但本地模型支持） |
| `FrequencyPenalty` | `*float64` | nil = 不发送 |
| `PresencePenalty` | `*float64` | nil = 不发送 |
| `ReasoningEffort` | `string` | `"low"\|"medium"\|"high"`, "" = 不发送（o1/o3 系列） |
| `Stop` | `[]string` | 停止序列，空则不发送 |
| `Stream` | `bool` | 由调用方填入，不由用户配置 |

> **重要**：所有 `*float64` / `*int` 字段使用 nil 区分「未配置」与「显式设为 0」，零值会合法发送给 API（temperature=0 = 贪婪解码）。

---

### `internal/core/config/config.go`

**职责**：从环境变量构建强类型配置，唯一配置来源，`main()` 调用一次后传入所有组件。

**当前环境变量完整列表**：

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PORT` | `8080` | HTTP 监听端口 |
| `CORS_ORIGINS` | `http://localhost:5173` | 逗号分隔允许来源 |
| `DATABASE_URL` | postgres localhost | PostgreSQL DSN |
| `LLM_BASE_URL` | BigModel API | OpenAI 兼容 API 地址 |
| `LLM_API_KEY` | 必填 | LLM 鉴权 Key |
| `LLM_MODEL` | `glm-4-flash` | 默认模型 |
| `LLM_TIMEOUT_SEC` | `60` | 请求超时秒数 |
| `LLM_MAX_RETRIES` | `2` | 失败重试次数（429/5xx）|
| `LLM_MAX_TOKENS` | `2048` | 最大输出 token 数 |
| `LLM_TOKEN_BUDGET` | `8000` | Prompt 输入 token 预算 |
| `LLM_MAX_HISTORY_FLOORS` | `20` | 加入上下文的最多历史楼层 |
| `LLM_TEMPERATURE` | 不发送 | 采样温度（0–2），空=不发送 |
| `LLM_TOP_P` | 不发送 | nucleus 采样（0–1），空=不发送 |
| `LLM_TOP_K` | 不发送 | top-K 采样，空=不发送 |
| `LLM_FREQUENCY_PENALTY` | 不发送 | 频率惩罚（-2–2），空=不发送 |
| `LLM_PRESENCE_PENALTY` | 不发送 | 出现惩罚（-2–2），空=不发送 |
| `LLM_REASONING_EFFORT` | 不发送 | `low\|medium\|high`，空=不发送 |
| `LLM_STOP_SEQUENCES` | 无 | 逗号分隔停止序列 |
| `MEMORY_TRIGGER_ROUNDS` | `10` | 每隔多少回合触发记忆整合 |
| `MEMORY_MODEL` | 同 LLM_MODEL | 记忆整合使用的廉价模型 |
| `MEMORY_MAX_TOKENS` | `512` | 记忆摘要最大输出 token |
| `MEMORY_TOKEN_BUDGET` | `600` | 记忆注入 token 预算 |
| `MEMORY_DEPRECATE_AFTER_DAYS` | `7` | summary 记忆超过 N 天后自动废弃（0 = 禁用） |
| `MEMORY_PURGE_AFTER_DAYS` | `30` | deprecated 记忆超过 N 天后物理删除（0 = 禁用） |
| `MEMORY_MAINTENANCE_INTERVAL_SEC` | `3600` | 维护扫描间隔（独立于整合轮询） |
| `ADMIN_KEY` | 空=开发模式放行 | 服务端全局 API Key |
| `ALLOW_ANONYMOUS` | `true` | 允许无 X-Account-ID 请求 |
| `UPLOAD_DIR` | `./uploads` | 素材存储目录 |
| `UPLOAD_BASE_URL` | `/uploads` | 素材对外访问 URL 前缀 |

---

### `internal/engine/api/game_loop.go`

**职责**：One-Shot 主链路，一次用户输入 → 一次 LLM 调用 → 结构化响应。

**数据流**：
```
TurnRequest
  │
  ├─ 1. 加载 GameSession + GameTemplate
  ├─ 2. StartTurn / RegenTurn (Floor + Page)
  ├─ 3. 构建变量沙箱
  ├─ 4. 读 MemorySummary 快照（0 延迟，Worker 异步写入）
  ├─ 5. 加载历史消息
  ├─ 6. 世界书词条转 IR
  ├─ 7. 构建 recentMsgs
  ├─ 8. Pipeline 执行（4 个节点 → 排序 → 展开）
  ├─ 9. One-Shot LLM 调用（解析 profile + 合并 GenerationParams）
  ├─ 10. Parse 响应（XML / JSON / Plaintext）
  ├─ 11. 更新沙箱变量
  ├─ 12. CommitTurn
  └─ 13. 异步：写摘要 + 触发记忆整合
```

**`TurnRequest` 字段（v2 扩充后）**：

| 字段 | 说明 |
|------|------|
| `session_id` | 必填 |
| `user_input` | 用户输入文本 |
| `is_regen` | Swipe 重新生成 |
| `api_key` | 可选，覆盖服务器 Key（前端自带 Key 模式） |
| `base_url` | 可选，覆盖服务器 BaseURL |
| `model` | 可选，覆盖模型 |
| `generation_params` | 可选，覆盖全部采样参数 |

**LLM Profile 解析优先级**（v2 实现，对齐 TH）：
1. `req.generation_params`（本轮请求最高优先级）
2. Session 级 `narrator` slot binding
3. Global 级 `narrator` slot binding
4. Session 级 `*` wildcard binding
5. Global 级 `*` wildcard binding
6. 环境变量默认值（最低优先级）

---

### `internal/engine/pipeline/`

**职责**：将 `ContextData` 转换为 `[]llm.Message`，多节点组合排序。

| 节点 | 优先级 | 说明 |
|------|--------|------|
| `node_preset` | — | 条目化 Prompt（InjectionOrder 直接作为 Priority，空时为 no-op） |
| `node_template` | 1000 | SystemPromptTemplate 单字符串兜底（无 preset entries 时生效） |
| `node_worldbook` | 900–500 | 关键词触发世界书词条（支持 `regex:<pattern>` 前缀） |
| `node_memory` | 400 | 记忆摘要注入（标签由 `GameConfig.MemoryLabel` 控制） |
| `node_history` | 0–(-N) | 历史消息（按楼层倒序） |

**`GameConfig` 字段一览**：

| 字段 | 说明 |
|------|------|
| `SystemPromptTemplate` | 单字符串兜底（有 PresetEntries 时并存，共同注入） |
| `PresetEntries` | 条目列表，InjectionOrder 控制排序 |
| `WorldbookEntries` | 世界书词条（关键词/正则触发） |
| `MemorySummary` | 异步 Worker 生成的摘要快照 |
| `MemoryLabel` | 记忆标签前缀（可覆盖） |
| `FallbackOptions` | parser fallback 默认选项 |

**当前限制**：
- 不支持 TH 的 entry-based preset 系统（InjectionPosition/Depth/Order 三级排序）
- 不支持 Regex pre/post-processing rules
- 不支持 prompt dry-run 快照（assembly digest）

---

### `internal/engine/memory/store.go`

**职责**：记忆 CRUD + 整合触发判断 + Session 摘要缓存。

**关键接口**：
- `FindSessionsNeedingConsolidation(triggerRounds, batchSize)` — SQL 扫描，`floor_count - max(source_floor) >= triggerRounds`
- `BuildConsolidationPrompt(sessionID, history)` — 构建整合 prompt
- `ParseConsolidationResult(sessionID, content, floor)` — 解析并落库
- `GetForInjection(sessionID, tokenBudget)` — 取摘要用于注入
- `UpdateSessionSummaryCache(sessionID, summary)` — 更新 session 快照

**`StoreConfig` 可配置项**（`NewStore(db, StoreConfig{...})` 注入）：

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `HalfLifeDays` | `7.0` | 记忆衰减半衰期（天），对应 TH `MemoryDecayConfig.halfLife` |
| `MinDecayFactor` | `0.0` | 最小衰减系数（> 0 则老记忆有保底权重），对应 TH `minFactor` |
| `MaxCandidates` | `50` | 注入前从 DB 取的最大候选条数 |
| `ConsolidationInstruction` | 内置指令 | 发给 LLM 的整合指令头，语言无关（空=使用内置） |
| `FactPrefix` | `"事实："` | 事实条目行前缀，`ParseConsolidationResult` 同时兼容 ASCII 冒号变体 |

**当前状态**：
- ✅ 时间衰减已实现（指数半衰期 + `MinDecayFactor` 保底）
- ⚠️ 维护策略（deprecate/purge policies）未实现

---

### `internal/engine/parser/parser.go`

**职责**：将 LLM 原始文本解析为结构化响应。

**解析策略三层回退**：
1. XML 严格模式（`<narrative>`, `<options>`, `<state_patch>`, `<summary>`, `<vn>`）
2. JSON 模式
3. Plaintext 降级（整段作为 narrative，空 options）

---

### `internal/creation/asset/handler.go`

**职责**：游戏素材文件上传（POST `/api/v2/assets/:slug/upload`）。

- MIME 白名单：PNG / JPG / GIF / WebP / MP3 / OGG / WAV
- 文件大小限制：10MB（`http.MaxBytesReader`）
- 存储路径：`UPLOAD_DIR/<slug>/<timestamp>.<ext>`
- 双重 MIME 检测：`http.DetectContentType`（读前 512 字节） + 文件名后缀回退

---

### `internal/user/middleware.go`

**职责**：HTTP 鉴权中间件。

**鉴权流程**：
1. 若 `ADMIN_KEY` 已配置，校验 `X-Api-Key` header 或 `Authorization: Bearer <key>`
2. 从 `X-Account-ID` header 提取账户 ID（Query 参数 `account_id` 作为备选）
3. 若 `ALLOW_ANONYMOUS=false` 且无账户 ID，返回 401

**GetAccountID 降级顺序**：context → X-Account-ID header → `"anonymous"`

---

## 硬编码清单

| 位置 | 硬编码值 | 影响 | 状态 |
|------|---------|------|------|
| `config.go` `LLMConfig.Model` | `"glm-4-flash"` | 默认模型 | ✅ 可通过 `LLM_MODEL` 覆盖 |
| `memory/store.go` `halfLifeDays` | `7.0` | 记忆衰减速率 | ✅ `StoreConfig.HalfLifeDays` 可配置 |
| `memory/store.go` `MinDecayFactor` | 不存在（可衰减至 0） | 老记忆被全部抛弃 | ✅ `StoreConfig.MinDecayFactor` 可配置 |
| `memory/store.go` `LIMIT 50` | 最多候选 50 条 | 注入记忆数量上限 | ✅ `StoreConfig.MaxCandidates` 可配置 |
| `memory/store.go` | 硬编码中文整合指令 | 语言绑定 | ✅ `StoreConfig.ConsolidationInstruction` 可配置 |
| `memory/store.go` | `"事实："` 前缀 | 语言绑定 | ✅ `StoreConfig.FactPrefix` 可配置 |
| `pipeline/node_memory.go` | `"[剧情记忆摘要]\n"` 标签 | 语言绑定 | ✅ `GameConfig.MemoryLabel` 可配置（per-game） |
| `pipeline/node_worldbook.go` | 仅子串匹配 | 正则关键词不支持 | ✅ `regex:<pattern>` 前缀已支持 |
| `parser/parser.go` | `["继续", "环顾四周"]` 默认选项 | 语言绑定 | ✅ 由 `GameTemplate.Config.fallback_options` 配置 |
| `internal/creation/api/routes.go` | `Limit(50)` 角色卡列表 | 分页上限 | ⚠️ 应加 `?limit=` 参数支持 |
| `internal/creation/api/routes.go` | `status = "published"` 模板过滤 | 硬编码过滤条件 | ⚠️ 应支持 `?status=` 参数 |

---

## TH 兼容性对照

| 功能 | TavernHeadless | backend-v2 | 差距 |
|------|----------------|------------|------|
| **采样参数（temperature, topP, topK, penalties, reasoning_effort）** | 完整，per-slot 覆盖 | ✅ v2 已加入 Options，Config 可配置 | — |
| **LLM Profile 生成参数（Params JSONB）** | Profile + Binding 均存 params | ✅ 已加入 models.go | — |
| **Per-request GenerationParams 覆盖** | `TurnRequest.generation_params` | ✅ 已加入 TurnRequest | — |
| **Profile slot 解析（narrator/memory/\* 优先级）** | 4 级回退 | ✅ ResolveSlot 已实现 | — |
| **Entry-based Preset 系统** | 完整（injection_position/order/identifier）| ✅ `PresetEntry` 模型 + `node_preset` + CRUD API | — |
| **Prompt Dry-Run** | 完整快照 + digest | ✅ `GET /sessions/:id/prompt-preview`（messages + est_tokens + block counts） | — |
| **Worldbook 正则关键词** | `regex:` 前缀触发正则 | ✅ 已支持（`matchWorldbookKey`） | — |
| **Memory 时间衰减** | 半衰期指数衰减 + minFactor | ✅ `StoreConfig.HalfLifeDays` + `MinDecayFactor` | — |
| **Memory 维护策略（deprecate/purge）** | 完整 | ✅ 全局维护 Worker（DeprecateAfterDays + PurgeAfterDays + MaintenanceInterval 独立定时器） | — |
| **Worker 轮询参数（批次/租约/重试）** | 全部可配置 | ✅ `PollInterval`/`BatchSize`/`MaxConcurrent`/`LeaseTTL` 均可配置 | — |
| **多 Provider 注册表** | 支持 openai/anthropic/google/xai | ⚠️ 仅 openai-compatible | 中期 |
| **Auth（JWT / API key 账户映射）** | 完整 | ⚠️ 仅 admin key + X-Account-ID | 中期 |
| **Tools / Function Calling** | MCP 工具调用 | ❌ 未实现 | 中期 |
| **多 LLM 角色槽（director/verifier）** | narrator + director + verifier | ❌ 仅 narrator | 中期 |
| **Session Fork** | 从任意 Floor 分叉新会话 | ❌ 未实现（设计已有） | 中期 |
| **SSE 流式** | 完整 | ✅ 已实现 | — |
| **三层消息结构** | Session→Floor→Page | ✅ 完整 | — |
| **五层变量沙箱** | Page→Floor→Branch→Chat→Global | ✅ 完整 | — |

---

## 扩充路线图

### ✅ 已完成（第五轮 — Memory 维护策略）

- `memory/store.go`：`DeprecateOldMemoriesGlobal(days)` + `PurgeDeprecatedMemoriesGlobal(days)` — 无 session_id 过滤，全量扫描，单条 SQL
- `memory/worker.go`：`WorkerConfig` 新增 `DeprecateAfterDays`、`PurgeAfterDays`、`MaintenanceInterval`；`Run()` 增加独立 `maintenanceTicker`；新增 `runMaintenance()`
- `config.go`：新增 `MEMORY_DEPRECATE_AFTER_DAYS`（默认 7）、`MEMORY_PURGE_AFTER_DAYS`（默认 30）、`MEMORY_MAINTENANCE_INTERVAL_SEC`（默认 3600）
- `cmd/worker/main.go`：三个新字段透传至 `WorkerConfig`

### ✅ 已完成（第四轮 — Session/Memory/Floor 管理 API + StreamTurn 对齐）

- `engine/api/engine_methods.go`：`ListSessions`、`ListFloors`、`ListPages`、`SetActivePage`（Swipe 选页）
- `engine/api/engine_methods.go`：`ListMemories`、`CreateMemory`、`UpdateMemory`、`DeleteMemory`（软/硬删除）
- `engine/api/engine_methods.go`：`ConsolidateNow`（同步记忆整合，调试用）
- `engine/api/engine_methods.go`：`PromptPreview`（dry-run，不调用 LLM，返回 messages + token 估算 + block 计数）
- `engine/api/routes.go`：新增以下端点（全部完整实现）：
  - `GET  /play/sessions` — 列出会话（`?game_id=&user_id=&limit=&offset=`）
  - `GET  /play/sessions/:id/floors` — 楼层列表（含激活页快照）
  - `GET  /play/sessions/:id/floors/:fid/pages` — Swipe 页列表
  - `PATCH /play/sessions/:id/floors/:fid/pages/:pid/activate` — Swipe 选页
  - `GET  /play/sessions/:id/memories` — 记忆条目列表
  - `POST /play/sessions/:id/memories` — 手动创建记忆
  - `PATCH /play/sessions/:id/memories/:mid` — 更新记忆字段
  - `DELETE /play/sessions/:id/memories/:mid` — 删除记忆（`?hard=true` 物理删除）
  - `POST /play/sessions/:id/memories/consolidate` — 立即触发记忆整合
  - `GET  /play/sessions/:id/prompt-preview` — Prompt dry-run
- `engine/api/engine_methods.go`：`StreamTurn` 完整重构，对齐 `PlayTurn`：
  - 加载 worldbook + preset entries + tmplCfg（memory_label / fallback_options）
  - 完整 Pipeline 执行（4 节点）
  - `resolveSlot` + `applyGenParams` + 前端自带 APIKey 支持
  - Regen 支持（`IsRegen` 字段，复用当前楼层而非新建）
  - 流结束后解析 StatePatch / Summary，CommitTurn 正常入库
  - 错误/取消时调用 `FailTurn` 标记楼层状态

### ✅ 已完成（第三轮 — Preset Entry 系统）

- `db/models.go` 加入 `PresetEntry`：identifier / name / role / content / injection_position / injection_order / is_system_prompt
- `connect.go` AutoMigrate 新增 `PresetEntry`
- `prompt_ir/pipeline.go`：`GameConfig.PresetEntries`、IR `PresetEntry` 类型、`BlockPreset` 常量
- `pipeline/node_preset.go`：新节点，InjectionOrder 直接作为 Priority，宏替换，空时 no-op
- `pipeline/runner.go`：PresetNode 注册在 TemplateNode 之前（两者共存）
- `creation/api/routes.go`：`/templates/:id/preset-entries` CRUD（GET/POST/PATCH/DELETE/PUT reorder）
- `creation/api/routes.go`：`GET /templates` 加入 `?status=all|published|draft` 参数
- `engine/api/game_loop.go`：加载 preset entries，转换为 IR 传入 pipelineCtx

- `memory/store.go` 加入 `StoreConfig`：`HalfLifeDays`、`MinDecayFactor`（保底）、`MaxCandidates`、`ConsolidationInstruction`、`FactPrefix` 全部可配置
- `pipeline/node_memory.go`：记忆标签从 `GameConfig.MemoryLabel` 读取，可按游戏覆盖（`GameTemplate.Config.memory_label`）
- `pipeline/node_worldbook.go`：关键词支持 `regex:<pattern>` 前缀（大小写不敏感，错误时降级为字面量匹配）
- `parser/parser.go`：移除硬编码中文默认选项，由 `GameTemplate.Config.fallback_options` 按游戏配置
- `prompt_ir/pipeline.go`：`GameConfig` 加入 `MemoryLabel` 和 `FallbackOptions`
- `engine/api/game_loop.go`：从模板 Config JSONB 解析 `memory_label` / `fallback_options` 并注入管道

### ⚠️ 近期目标

（全部核心能力已对齐参考实现）

### 📋 中期目标（引擎层真实差距）

| 功能 | 复杂度 | 价值 |
|------|--------|------|
| **Session Fork** | 低 | 高 — 从任意 Floor 分叉新会话，存档/平行时间线基础 |
| **Tools / Function Calling** | 中 | 极高 — 引擎通用化入口，驱动 vibe coding / 项目可视化 |
| **MCP 协议接入** | 中 | 高 — 复用社区 MCP 服务，Tools 层建成后自然衔接 |
| **多 LLM 角色槽（director/verifier）** | 中 | 中 — 多 AI 协作；director 控制剧情走向，verifier 校验输出格式 |
| **多 Provider 注册表（Anthropic/Google）** | 中 | 中 — 非 OpenAI 兼容路径；目前 BYO Key 已覆盖大部分场景 |
| **Auth（JWT + 账户映射）** | 中 | 中 — 当前 admin key + X-Account-ID 足够单机/小团队 |

设计思路见 [`docs/prompt-block-design.md`](prompt-block-design.md)（PromptBlock 扩展路径 + MVM 分层 + 存档分析）。
