# TavernHeadless → WorkshopEngine 代码位置对照

> 版本：2026-04-08
> TH 版本：`bdafb0a`（2026-04-07，v0.2.0-beta.3）
> TH 根目录：`D:\ai-game-workshop\plagiarism-and-secret\TavernHeadless\`

本文档逐条列出 TavernHeadless 每个核心功能的**源码位置**，并标注 WE 对应的实现位置。
用途：借鉴 TH 的实现细节时，直接定位文件，不用盲目猜测。

---

## 一、代码库结构对比

| 层次 | TH 路径 | WE 路径 | 说明 |
|------|---------|---------|------|
| 核心引擎 | `packages/core/src/` | `internal/engine/` + `internal/core/` | TH 把引擎提取为独立 npm 包 |
| ST 适配器 | `packages/adapters-sillytavern/src/` | `internal/creation/card/` + `internal/creation/lorebook/` | TH 单独一个 package |
| HTTP 层 | `apps/api/src/routes/` + `services/` | `internal/engine/api/` + `internal/creation/api/` | TH 用 Fastify，WE 用 Gin |
| DB schema | `apps/api/src/db/schema.ts` + `drizzle/*.sql` | `internal/core/db/models.go` | TH 用 Drizzle ORM，WE 用 GORM |
| 官方 SDK | `packages/official-integration-kit/` + `packages/shared/` | 未实现（P-5B 计划）| TH 已发布 @tavern/sdk |
| 前端 | `apps/web/src/` | `frontend-v2/` | TH 是创作工作台；WE 是游玩界面 |

---

## 二、消息层级（Session / Floor / Page）

### TH

| 组件 | 文件 |
|------|------|
| DB schema | `apps/api/src/db/schema.ts`（`chat`, `floor`, `message_page` 表）|
| Floor 状态机 | `packages/core/src/floor/floor-state-machine.ts`（draft→generating→committed/failed/superseded）|
| Floor 生命周期 | `packages/core/src/floor/floor-lifecycle.ts` |
| Floor 仓库接口 | `packages/core/src/ports/floor-repository.ts` |
| Drizzle 实现 | `apps/api/src/adapters/drizzle-floor-repository.ts` |
| Floor Run State | `apps/api/drizzle/0026_floor_run_state.sql`（精细状态机，M17 功能）|
| 会话路由 | `apps/api/src/routes/sessions.ts` |
| 楼层路由 | `apps/api/src/routes/floors.ts` |
| 消息页路由 | `apps/api/src/routes/messages.ts` |
| 业务逻辑 | `apps/api/src/services/chat-service.ts` |
| 页激活 | `apps/api/src/services/page-activation-service.ts` |
| 消息持久化 | `apps/api/src/services/chat-message-persistence.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| DB model | `internal/core/db/models.go`（`GameSession`, `Floor`, `MessagePage`）|
| Session 状态机 | `internal/engine/session/manager.go` |
| HTTP 路由 | `internal/engine/api/routes.go` + `engine_methods.go` |

---

## 三、Prompt Pipeline（提示词组装）

### TH

| 组件 | 文件 |
|------|------|
| Native Pipeline 主入口 | `packages/core/src/prompt/native-pipeline.ts` |
| Prompt Graph（DAG） | `packages/core/src/prompt-graph/`（Beta3 新增，WE 无对等）|
| Compat Assembler（ST 格式） | `packages/adapters-sillytavern/src/compat-assembler.ts` |
| IR 中间表示 | `packages/core/src/prompt/`（IRSection / IRMessage 等类型）|
| Token Budget | `packages/core/src/prompt/token-budget.ts`（prunable/pinned 策略）|
| Worldbook 触发引擎 | `packages/adapters-sillytavern/src/worldbook/trigger-engine.ts` |
| Regex 引擎 | `packages/adapters-sillytavern/src/regex/regex-engine.ts` |
| Preset Utils | `apps/api/src/lib/preset-utils.ts` |
| Prompt 资源加载 | `apps/api/src/services/prompt-resource-loader.ts` |
| Prompt 组装服务 | `apps/api/src/services/prompt-assembler.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| PromptBlock IR | `internal/engine/prompt_ir/pipeline.go` |
| Pipeline Runner | `internal/engine/pipeline/runner.go` |
| TemplateNode | `internal/engine/pipeline/node_template.go` |
| CharacterInjectionNode（M11）| `internal/engine/pipeline/node_character.go` |
| WorldbookNode（含 GroupCap + TokenBudget）| `internal/engine/pipeline/node_worldbook.go` |
| MemoryNode | `internal/engine/pipeline/node_memory.go` |
| HistoryNode | `internal/engine/pipeline/node_history.go` |
| PresetNode | `internal/engine/pipeline/node_preset.go` |
| Regex 后处理 | `internal/engine/processor/` |
| 宏展开 | `internal/engine/macros/expand.go` |

**关键差异：**
- TH 有两条路径：Native Pipeline（原生）+ Compat Assembler（ST 兼容）；WE 只有一条 PromptBlock IR 路径
- TH 有 PromptGraph（DAG 依赖编排）；WE 用全局 Priority 数字排序（更简单但无法表达节点依赖）
- TH `token-budget.ts` 比 WE 的 `applyTokenBudget` 更通用（覆盖所有 Section，而非只有 worldbook）

---

## 四、变量系统

### TH

| 组件 | 文件 |
|------|------|
| 核心类型 | `packages/core/src/variables/types.ts` |
| Variable Scope | `packages/core/src/variables/variable-scope.ts` |
| Variable Store | `packages/core/src/variables/variable-store.ts` |
| Variable Mutation | `packages/core/src/variables/variable-mutation.ts` |
| 服务层 | `apps/api/src/services/variable-service.ts` |
| 提交服务 | `apps/api/src/services/variable-commit-service.ts` |
| Branch Scope（Beta3）| `apps/api/drizzle/0028_variable_branch_scope.sql` |
| 路由 | `apps/api/src/routes/variables.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| 五层沙箱 | `internal/engine/variable/sandbox.go` |
| DB 存储 | `GameSession.Variables`（JSONB，chat 层持久化）|
| 路由 | `internal/engine/api/routes.go`（`/sessions/:id/variables`）|

---

## 五、记忆系统

### TH

| 组件 | 文件 |
|------|------|
| 核心类型 | `packages/core/src/memory/types.ts` |
| Memory Store | `packages/core/src/memory/memory-store.ts` |
| Memory Consolidator | `packages/core/src/memory/memory-consolidator.ts` |
| 摄入处理器（micro 摘要）| `packages/core/src/memory/memory-ingest-processor.ts` |
| 压缩任务（macro 摘要）| `packages/core/src/memory/memory-compaction-processor.ts` / `memory-compaction-*.ts` |
| 注入选择器（dual_summary 策略）| `packages/core/src/memory/memory-injection-selector.ts` |
| Scope 解析 | `packages/core/src/memory/memory-scope-resolver.ts` |
| Revision Guard（并发写保护）| `packages/core/src/memory/memory-revision-guard.ts` |
| DB schema | `apps/api/drizzle/0020_memory_v2_schema_infra.sql` |
| 服务层 | `apps/api/src/services/memory-worker.ts` / `memory-job-scheduler.ts` / `memory-maintenance-service.ts` |
| 路由 | `apps/api/src/routes/memories.ts` + `memory-jobs.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| Memory Store | `internal/engine/memory/store.go` |
| Memory Worker（整合触发）| `internal/engine/memory/worker.go` |（goroutine 版，非 DB 持久化）|
| MemoryEdge（关系图）| `internal/engine/memory/edge.go` |
| 注入（带衰减 + stage_tags）| `store.go#GetForInjection` |

**关键差异：**
- TH 实现了双层摘要（micro per-floor + macro 批量压缩）；WE 当前单层（P-4F 计划）
- TH 用 DB runtime_job 持久化记忆整合任务；WE 用 goroutine（P-4G 计划）
- WE 有 `stage_tags` 多幕过滤（TH 无此功能）
- WE 有 `MemoryEdge` 关系图（TH 无等价）

---

## 六、LLM 调度

### TH

| 组件 | 文件 |
|------|------|
| LLM 核心类型 | `packages/core/src/llm/types.ts` |
| Provider 注册表 | `packages/core/src/llm/provider-registry.ts` |
| LLM Service | `packages/core/src/llm/llm-service.ts` |
| Profile 服务 | `apps/api/src/services/llm-profile-service.ts` |
| Instance 服务 | `apps/api/src/services/llm-instance-service.ts` |
| Secret Vault（AES）| `apps/api/src/lib/secrets.ts` + `drizzle/0005_llm_profile_vault.sql` |
| 路由 | `apps/api/src/routes/llm-profiles.ts` + `llm-instances.ts` |
| Slot 绑定 | `apps/api/drizzle/0006_instance_slot.sql` |

### WE 对应

| 组件 | 文件 |
|------|------|
| LLM Client | `internal/core/llm/client.go` |
| Provider Registry | `internal/platform/provider/registry.go` |
| LLM Profile CRUD | `internal/creation/api/`（profiles 路由）|
| Slot 优先级解析 | `provider.Registry.ResolveForSlot()` |

---

## 七、工具调用（Tool Calling）

### TH

| 组件 | 文件 |
|------|------|
| 核心类型 | `packages/core/src/tools/types.ts` |
| Tool Registry | `packages/core/src/tools/tool-registry.ts` |
| Tool Executor | `packages/core/src/tools/tool-executor.ts` |
| 内置工具 Provider | `packages/core/src/tools/builtin-provider.ts` |
| Preset/自定义工具 Provider | `packages/core/src/tools/preset-provider.ts` |
| Resource Tool Provider | `apps/api/src/tools/resource-tool-provider.ts` |
| Tool 执行记录 | `apps/api/src/adapters/drizzle-tool-execution-repository.ts` |
| DB schema | `apps/api/drizzle/0014_tool_calling.sql` |
| 服务层 | `apps/api/src/services/tool-service.ts` |
| 路由 | `apps/api/src/routes/tools.ts` |
| MCP 工具 | `apps/api/src/mcp/mcp-tool-provider.ts` + `mcp-connection-manager.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| Tool Registry | `internal/engine/tools/registry.go` |
| 内置工具 | `internal/engine/tools/builtin.go`（get/set_variable + search_memory/material）|
| Resource Tool Provider | `internal/engine/tools/resource_tool_provider.go`（14 个工具）|
| HTTP 回调工具 | `internal/engine/tools/http_tool.go`（Preset Tool）|
| ToolExecutionRecord | `internal/core/db/models.go` + `registry.go#ExecuteAndRecord` |

---

## 八、回合编排（Turn Orchestration）

### TH

| 组件 | 文件 |
|------|------|
| Turn Orchestrator | `packages/core/src/orchestration/turn-orchestrator.ts` |
| Director | `packages/core/src/orchestration/director.ts` |
| Verifier | `packages/core/src/orchestration/verifier.ts` |
| Generation Pipeline | `packages/core/src/generation/generation-pipeline.ts` |
| Generation Guard | `apps/api/src/services/generation-guard-service.ts`（并发保护，P-3K 参照）|
| Turn Commit | `apps/api/src/services/turn-commit-service.ts` |
| Orchestration Factory | `apps/api/src/services/orchestration-factory.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| PlayTurn（主回合链路）| `internal/engine/api/game_loop.go` |
| StreamTurn | `internal/engine/api/engine_methods.go#StreamTurn` |
| Director 槽 | `game_loop.go`（inline，非独立文件）|
| Verifier 槽 | `internal/engine/api/verifier.go` |
| PromptSnapshot | `engine_methods.go#savePromptSnapshot` |

---

## 九、Runtime Substrate（运行时底层）

### TH

| 组件 | 文件 |
|------|------|
| runtime_job 表 | `apps/api/drizzle/0024_background_job_runtime.sql` |
| Runtime Worker | `apps/api/src/services/runtime-worker.ts` |
| Job Scheduler | `apps/api/src/services/runtime-job-scheduler.ts` |
| Job Query | `apps/api/src/services/runtime-job-query-service.ts` |
| Job Catalog | `apps/api/src/services/runtime-job-catalog.ts` |
| Processor Registry | `apps/api/src/services/runtime-job-processor-registry.ts` |
| Revision Guard（CAS）| `apps/api/src/services/runtime-revision-guard.ts` |
| floor_run_state 表 | `apps/api/drizzle/0026_floor_run_state.sql` |

### WE 对应

WE 当前用 goroutine 实现，**无 DB 持久化**：
- 记忆整合：`game_loop.go#triggerMemoryConsolidation`（goroutine）
- ScheduledTurn：`internal/engine/scheduled/`（同步调用）
- 无 floor_run_state；无 Revision Guard

计划在 P-4G 实现 `runtime_job` 表。

---

## 十、SillyTavern 兼容（适配器）

### TH

| 组件 | 文件 |
|------|------|
| 角色卡解析 | `packages/adapters-sillytavern/src/parsers/character-parser.ts` + `character-normalizer.ts` |
| 角色卡序列化（导出）| `packages/adapters-sillytavern/src/serializers/character-serializer.ts` |
| 聊天历史解析 | `packages/adapters-sillytavern/src/parsers/chat-parser.ts` |
| 预设解析 | `packages/adapters-sillytavern/src/parsers/preset-parser.ts` |
| Regex 解析 | `packages/adapters-sillytavern/src/parsers/regex-parser.ts` |
| 世界书解析 | `packages/adapters-sillytavern/src/parsers/worldbook-parser.ts` |
| 导入路由 | `apps/api/src/routes/imports.ts` |
| 导出路由 | `apps/api/src/routes/exports.ts` |

### WE 对应

| 组件 | 文件 |
|------|------|
| 角色卡 PNG 解析 | `internal/creation/card/parser.go` |
| ST 预设导入 | `internal/creation/template/import_st_preset.go` |
| ST 世界书导入 | `internal/creation/lorebook/import_st.go` |
| 游戏包 import/export | `internal/creation/template/package_export.go` + `package_import.go` |

---

## 十一、DB Schema 迁移对照

TH 用增量 SQL 迁移（drizzle/0000_*.sql），WE 用 GORM AutoMigrate。
以下列出 TH 迁移文件对应的 WE 字段/模型位置：

| TH 迁移文件 | 内容 | WE 对应 |
|-------------|------|---------|
| `0000_initial_schema.sql` | 基础表（chat/floor/page/character/preset/worldbook）| `models.go` 全部初始字段 |
| `0001_import_resources.sql` | 资源导入元数据 | `GameTemplate` 导入字段 |
| `0004_character_binding.sql` | 角色卡与 Session 绑定 | `GameSession.CharacterCardID` + `CharacterSnapshot`（M11）|
| `0005_llm_profile_vault.sql` | API Key AES 加密 + mask | 计划 P-4A |
| `0006_instance_slot.sql` | LLM Profile Binding slot | `LLMProfileBinding.Slot` |
| `0009_llm_binding_params.sql` | LLM Binding Params 覆盖 | `LLMProfileBinding.Params` |
| `0012_worldbook_entries.sql` | 世界书词条（正式表）| `WorldbookEntry` 模型 |
| `0014_tool_calling.sql` | 工具调用记录表 | `ToolExecutionRecord` 模型 |
| `0015_mcp_server_config.sql` | MCP 服务器配置 | 计划 P-3H |
| `0019_tool_execution_journal.sql` | 工具执行审计 | `ToolExecutionRecord.Audit` 字段（部分）|
| `0020_memory_v2_schema_infra.sql` | Memory V2（micro/macro）| 计划 P-4F |
| `0024_background_job_runtime.sql` | runtime_job 表 | 计划 P-4G |
| `0026_floor_run_state.sql` | Floor 精细状态机 | 计划 P-4H |
| `0028_variable_branch_scope.sql` | Branch-scoped 变量 | 计划 P-3G（Floor.BranchID）|

---

## 十二、官方集成包（Integration Kit）

### TH

| 组件 | 文件 |
|------|------|
| 主包目录 | `packages/official-integration-kit/` |
| @tavern/sdk（HTTP 客户端）| `packages/official-integration-kit/sdk/src/` |
| @tavern/client-helpers（状态工具）| `packages/official-integration-kit/client-helpers/src/` |
| 共享类型 | `packages/shared/src/` |
| 自动生成 API 类型 | `packages/shared/src/generated/openapi-types.ts` |

### WE 对应

WE 无等价实现，计划在 P-5B 实现：
- `@gw/sdk`：typed HTTP + SSE 客户端
- `@gw/play-helpers`：`buildMessageTimeline` / `reduceGameStream` / `applyVNDirectives`

---

## 十三、WE 独有功能（TH 无对应）

| WE 功能 | WE 代码位置 | 说明 |
|---------|-----------|------|
| game-package.json 游戏包 | `internal/creation/template/package_*.go` | 一文件包含全部游戏资源 |
| VN 指令解析（[bg|], [sprite|] 等）| `internal/engine/parser/vn_parser.go` | 后端解析，前端渲染 |
| 素材库 + search_material 工具 | `internal/creation/asset/` + `internal/engine/tools/` | AI 按标签检索注入 |
| ScheduledTurn（NPC 自主回合）| `internal/engine/scheduled/` | variable_threshold 触发 |
| creation-agent（AI 辅助创作）| `internal/creation/api/`（creation-agent 路由）| 对话式修改游戏规则 |
| Memory stage_tags 分阶段过滤 | `internal/engine/memory/store.go#GetForInjection` | TH 无此功能 |
| MemoryEdge 关系图 | `internal/engine/memory/edge.go` | TH 无此功能 |
