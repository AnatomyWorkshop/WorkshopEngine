# Engine Audit — 后端组件说明与配置清单

> 更新日期：2026-04-11（Phase 4 进行中：P-4A/P-4B/P-4C/P-4D/P-4E/P-4G 已完成）
> 主文档：`inspiration/PLAN/P-WE-OVERVIEW.md`（开发计划 + API 速查 + 包结构）

本文聚焦于**运行时组件职责**和**可配置项清单**，供开发者快速理解引擎内部结构。
架构决策和开发路线图见 P-WE-OVERVIEW.md。

---

## 核心组件职责

### `internal/core/llm/client.go`

向任何 OpenAI 兼容 API 发送请求。

| 接口 | 说明 |
|------|------|
| `Chat(ctx, messages, opts)` | 非流式，带指数退避重试（429/5xx） |
| `ChatStream(ctx, messages, opts)` | SSE 流式，token channel |

`Provider` 接口（`provider.go`）：`Chat` + `ChatStream` + `ID`。
`NewProvider(type, ...)` 工厂函数（`factory.go`）：`"anthropic"` → `AnthropicClient`，其余 → `Client`。
`AnthropicClient`（`anthropic.go`）：`x-api-key` 鉴权 + `/v1/messages` + system 字段提取 + `content_block_delta` 流 + `tool_use` 转换。

`Options` 字段：`Model` / `MaxTokens` / `Temperature *float64` / `TopP *float64` / `TopK *int` / `FrequencyPenalty *float64` / `PresencePenalty *float64` / `ReasoningEffort string` / `Stop []string` / `Stream bool`。

所有 `*float64` / `*int` 字段使用 nil 区分「未配置」与「显式设为 0」（temperature=0 = 贪婪解码）。

支持 Function Calling：`ToolDefinition` / `ToolCall` / `ToolCallFunction` 类型，`Options.Tools` 传入定义，`Response.ToolCalls` 返回调用。

---

### `internal/engine/api/game_loop.go`

PlayTurn 主链路（16 步）：

```
TurnRequest
  ├─ 1.  加载 GameSession + GameTemplate
  ├─ 2.  StartTurn / RegenTurn（Floor + Page，含并发保护 SELECT FOR UPDATE）
  ├─ 3.  构建变量沙箱（五层级联）
  ├─ 3b. 初始化工具注册表（enabled_tools 按需注册）
  ├─ 4.  读 MemorySummary（按 stage_tags 过滤）
  ├─ 5.  加载历史消息（支持 branch_id）
  ├─ 6.  世界书词条 → IR（含 Group/GroupWeight/Depth）
  ├─ 6b. Regex 规则 → IR
  ├─ 7a. Preset Entry → IR
  ├─ 7.  构建 recentMsgs（用户输入经 Regex 预处理）
  ├─ 8.  Pipeline 执行（6 节点 → 排序 → at_depth 插入 → 展开）
  ├─ 9.  resolveSlot + GenerationParams 覆盖
  ├─ 9b. Director 槽（可选，廉价模型预分析）
  ├─ 10. Agentic Tool Loop（最多 N 轮，无工具时单次直通）
  ├─ 11. Parse 响应 + Regex 后处理 + 变量更新
  ├─ 12. CommitTurn + ClearGenerating
  ├─ 13. Verifier 槽（可选，一致性校验）
  ├─ 14. PromptSnapshot 异步写入
  ├─ 15. 异步记忆整合触发
  └─ 16. ScheduledTurn 规则求值
```

`Suggest()`（AI 帮答）复用步骤 1-8 的完整管线，追加 impersonate 指令，调用 narrator 槽，不写 Floor。

---

### `internal/engine/pipeline/`

将 `ContextData` 转换为 `[]llm.Message`，多节点组合排序。

| 节点 | 优先级 | 说明 |
|------|--------|------|
| `node_character` | 9 | 角色卡注入（pin/latest 策略，宏展开） |
| `node_preset` | InjectionOrder | 条目化 Prompt（InjectionPosition 控制位置） |
| `node_template` | 1000 | SystemPromptTemplate 兜底 |
| `node_worldbook` | 按 Position 映射 | 关键词触发（递归激活 + 互斥分组 + Token Budget + 变量门控 + at_depth） |
| `node_memory` | 400 | 记忆摘要注入（stage_tags 过滤） |
| `node_history` | 0–(-N) | 历史消息（按楼层倒序） |

---

### `internal/engine/memory/store.go`

记忆 CRUD + 整合触发 + 衰减排序。

| 接口 | 说明 |
|------|------|
| `GetForInjection(sessionID, tokenBudget, currentStage)` | 按 importance × decay 排序，stage_tags 过滤 |
| `BuildConsolidationPrompt` | 构建整合 prompt |
| `ParseConsolidationResult` | 解析 JSON facts（fact_key upsert/deprecate） |
| `DeprecateOldMemoriesGlobal` / `PurgeDeprecatedMemoriesGlobal` | 全局维护 |

`MemoryEdge` 表：`updates` / `contradicts` / `supports` / `resolves` 关系图。

---

### `internal/engine/scheduler/` （P-4G 新增）

DB 持久化后台任务调度器，替代 memory/worker 的 `sync.Map` 内存租约。

| 接口 | 说明 |
|------|------|
| `Enqueue(jobType, sessionID, dedupeKey)` | 去重入队（`ON CONFLICT DO NOTHING`） |
| `LeaseJob(ctx, jobType)` | `FOR UPDATE SKIP LOCKED` 原子租约 |
| `Complete(jobID)` / `Fail(jobID, errMsg)` | 完成/失败（失败自动重试，超限进 dead） |
| `RecoverStale()` | 启动时恢复超时租约 |
| `EnqueueIfDue(sessionID, floorCount, triggerRounds)` | 条件入队（记忆整合触发判断） |

`runtime_job` 表状态机：`queued → leased → done / failed / dead`

---

### `internal/engine/tools/`

| 组件 | 说明 |
|------|------|
| `Registry` | 工具注册 + 定义序列化 + 带审计执行 |
| 内置工具 | `get_variable` / `set_variable` / `search_memory` / `search_material` |
| `ResourceToolProvider` | 14 个 AI 可操作资源工具（worldbook/preset/memory/material CRUD） |
| `HttpCallTool` | 创作者自定义 HTTP 回调（`preset:*` / `preset:<name>`） |
| `ReplaySafety` | safe / confirm_on_replay / never_auto_replay / uncertain |

---

### `internal/platform/auth/middleware.go`

鉴权中间件。

| 模式 | 说明 |
|------|------|
| JWT | `AUTH_MODE=jwt` + `AUTH_JWT_SECRET` 非空时，校验 Bearer JWT（HS256），`sub` = account_id |
| Admin Key | `ADMIN_KEY` 非空时校验 `X-Api-Key` 或 `Authorization: Bearer` |
| API Key 映射 | `AUTH_API_KEYS` + `AUTH_KEY_ACCOUNT_MAP` 多 Key 映射到不同账户 |
| 匿名 | `ALLOW_ANONYMOUS=true` 时允许无身份请求 |

Token 签发：`POST /api/auth/token`（admin_key 验证后签发，在 auth 中间件之外）。

---

### `internal/platform/provider/registry.go`

LLM Profile 动态解析，5 级优先级：

1. Session 级精确 slot（如 `narrator`）
2. Global 级精确 slot
3. Session 级通配 `*`
4. Global 级通配 `*`
5. 环境变量默认值

---

## 环境变量配置清单

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
| `LLM_MAX_TOOL_ITER` | `5` | Agentic Loop 最大工具调用轮数 |
| `LLM_TEMPERATURE` | 不发送 | 采样温度（0–2） |
| `LLM_TOP_P` | 不发送 | nucleus 采样（0–1） |
| `LLM_TOP_K` | 不发送 | top-K 采样 |
| `LLM_FREQUENCY_PENALTY` | 不发送 | 频率惩罚（-2–2） |
| `LLM_PRESENCE_PENALTY` | 不发送 | 出现惩罚（-2–2） |
| `LLM_REASONING_EFFORT` | 不发送 | `low\|medium\|high` |
| `LLM_STOP_SEQUENCES` | 无 | 逗号分隔停止序列 |
| `MEMORY_TRIGGER_ROUNDS` | `10` | 每隔多少回合触发记忆整合 |
| `MEMORY_MODEL` | 同 LLM_MODEL | 记忆整合使用的廉价模型 |
| `MEMORY_MAX_TOKENS` | `512` | 记忆摘要最大输出 token |
| `MEMORY_TOKEN_BUDGET` | `600` | 记忆注入 token 预算 |
| `MEMORY_DEPRECATE_AFTER_DAYS` | `7` | summary 记忆超过 N 天后自动废弃 |
| `MEMORY_PURGE_AFTER_DAYS` | `30` | deprecated 记忆超过 N 天后物理删除 |
| `MEMORY_MAINTENANCE_INTERVAL_SEC` | `3600` | 维护扫描间隔 |
| `ADMIN_KEY` | 空=开发模式放行 | 服务端全局 API Key |
| `AUTH_API_KEYS` | 空 | 逗号分隔的多 API Key |
| `AUTH_KEY_ACCOUNT_MAP` | 空 | Key→AccountID 映射（`key1:acc1,key2:acc2`） |
| `ALLOW_ANONYMOUS` | `true` | 允许无 X-Account-ID 请求 |
| `SECRETS_MASTER_KEY` | 空=不加密 | AES-256-GCM 主密钥（hex，≥32 字节）|
| `AUTH_MODE` | 空=自动检测 | 鉴权模式：`off` / `jwt`（空时按 ADMIN_KEY/AUTH_API_KEYS 自动检测）|
| `AUTH_JWT_SECRET` | 空 | JWT HS256 签名密钥（AUTH_MODE=jwt 时必填）|
| `AUTH_JWT_TTL_HOURS` | `168` | JWT 有效期（小时，默认 7 天）|
| `UPLOAD_DIR` | `./uploads` | 素材存储目录 |
| `UPLOAD_BASE_URL` | `/uploads` | 素材对外访问 URL 前缀 |

---

## 已知技术债务

| 位置 | 问题 | 触发条件 |
|------|------|---------|
| Token 计数 | BPE 启发式估算，±15% 误差 | 需要支持 4K 以下上下文窗口的模型时，引入 provider-specific 分词器 |
| `game_loop.go` | PlayTurn/StreamTurn/Suggest 三处重复管线构建代码 | P-4J 提取 `buildPipelineContext()` |
| `LLMProfile.APIKey` | ~~明文存储~~ ✅ P-4A 已完成（AES-256-GCM + HKDF） | — |
| `X-Account-ID` | ~~无签名，可伪造~~ ✅ P-4B 已完成（JWT HS256） | — |
| Memory Worker | ~~goroutine 内存级，进程重启丢失任务~~ ✅ P-4G 已完成（`runtime_job` 表 + `scheduler` 包） | — |
| 并发保护 | 仅 reject 模式 | queue 模式为 UX 优化，Phase 4 |
