# TavernHeadless → WorkshopEngine 代码位置对照

> 版本：2026-04-10（TH Beta3 `bdafb0a` 2026-04-07 + WE Phase 3 全部完成）
> TH 根目录：`D:\ai-game-workshop\plagiarism-and-secret\TavernHeadless\`
> 用途：借鉴 TH 实现细节时直接定位文件

---

## 一、代码库结构

| 层次 | TH 路径 | WE 路径 |
|------|---------|---------|
| 核心引擎 | `packages/core/src/` | `internal/engine/` + `internal/core/` |
| ST 适配器 | `packages/adapters-sillytavern/src/` | `internal/creation/card/` + `lorebook/` |
| HTTP 层 | `apps/api/src/routes/` + `services/` | `internal/engine/api/` + `creation/api/` + `platform/play/` |
| DB schema | `apps/api/src/db/schema.ts` + `drizzle/*.sql` | `internal/core/db/models*.go` |
| 官方 SDK | `packages/official-integration-kit/` | 计划 P-5B |
| 前端 | `apps/web/src/`（创作工作台） | `GameWorkshop/`（游玩界面） |

---

## 二、逐模块对照

### 消息层级（Session / Floor / Page）

| TH | WE |
|----|-----|
| `packages/core/src/floor/floor-state-machine.ts` | `internal/engine/session/manager.go` |
| `apps/api/src/services/chat-service.ts` | `internal/engine/api/engine_methods.go` |
| `apps/api/src/services/page-activation-service.ts` | `session/manager.go#SetActivePage` |
| `apps/api/drizzle/0026_floor_run_state.sql` | 计划 P-4H（SSE phase 替代） |

### Prompt Pipeline

| TH | WE |
|----|-----|
| `packages/core/src/prompt/native-pipeline.ts` | `internal/engine/pipeline/runner.go` |
| `packages/core/src/prompt/types.ts`（IRSection/IRMessage） | `internal/engine/prompt_ir/pipeline.go` |
| `packages/core/src/prompt/token-budget.ts` | `pipeline/node_worldbook.go#applyTokenBudget` |
| `packages/core/src/prompt/message-builder.ts` | `pipeline/runner.go#Execute` |
| `packages/adapters-sillytavern/src/worldbook/trigger-engine.ts` | `pipeline/node_worldbook.go` |
| `packages/adapters-sillytavern/src/regex/regex-engine.ts` | `internal/engine/processor/regex.go` |
| `packages/adapters-sillytavern/src/compat-assembler.ts` | 无对等（WE 只有一条路径） |
| `packages/core/src/prompt-graph/`（DAG） | 无对等（WE 用 Priority 数字排序） |

**差异：** TH 有 native + compat 双路径 + PromptGraph DAG；WE 只有 PromptBlock IR + Priority 排序（更简单，够用）。TH `token-budget.ts` 覆盖所有 Section 裁剪；WE 只对 worldbook 做 token budget。

### 变量系统

| TH | WE |
|----|-----|
| `packages/core/src/variables/variable-scope.ts` | `internal/engine/variable/sandbox.go` |
| `packages/core/src/variables/variable-mutation.ts` | `sandbox.go#Set` + `CommitTurn` 提升 |
| `apps/api/drizzle/0028_variable_branch_scope.sql` | `Floor.BranchID`（P-3G ✅） |

### 记忆系统

| TH | WE |
|----|-----|
| `packages/core/src/memory/memory-store.ts` | `internal/engine/memory/store.go` |
| `packages/core/src/memory/memory-consolidator.ts` | `store.go#BuildConsolidationPrompt` + `ParseConsolidationResult` |
| `packages/core/src/memory/memory-ingest-processor.ts`（micro） | 计划 P-4F |
| `packages/core/src/memory/memory-compaction-processor.ts`（macro） | 计划 P-4F |
| `packages/core/src/memory/memory-injection-selector.ts` | `store.go#GetForInjection` |
| `packages/core/src/memory/memory-mutation-applier.ts` | `store.go#ParseConsolidationResult`（内联） |
| `packages/core/src/memory/types.ts`（MemoryItem.summaryTier） | 计划 P-4F（Memory.Type 新增 micro/macro） |

**差异：** TH 双层摘要（micro per-floor + macro 批量压缩）已完整实现；WE 当前单层 fact。WE 独有 `stage_tags` 多幕过滤 + `MemoryEdge` 关系图。

### LLM 调度

| TH | WE |
|----|-----|
| `packages/core/src/llm/provider-registry.ts` | `internal/platform/provider/registry.go` |
| `packages/core/src/llm/llm-service.ts`（Vercel AI SDK） | `internal/core/llm/client.go`（直接 HTTP） |
| `apps/api/src/lib/secrets.ts`（AES 加密） | 计划 P-4A |
| Provider 工厂：openai/anthropic/google/deepseek/xai | 仅 openai-compatible（计划 P-4C） |

**差异：** TH 通过 Vercel AI SDK 原生支持 5 个 provider；WE 仅 OpenAI 兼容路径。P-4C 计划接入 Anthropic/Gemini 原生 API。

### 工具系统

| TH | WE |
|----|-----|
| `packages/core/src/tools/tool-registry.ts` | `internal/engine/tools/registry.go` |
| `packages/core/src/tools/tool-executor.ts` | `registry.go#ExecuteAndRecord` |
| `packages/core/src/tools/builtin-provider.ts` | `tools/builtin.go` |
| `packages/core/src/tools/preset-provider.ts` | `tools/http_tool.go` |
| `apps/api/src/tools/resource-tool-provider.ts`（23 工具） | `tools/resource_tool_provider.go`（14 工具） |
| `apps/api/src/mcp/mcp-connection-manager.ts` | 明确暂缓（P-3H） |
| `packages/core/src/tools/tool-mutation-buffer.ts` | 无对等（WE 工具直接写变量） |

**差异：** TH 有 `ToolMutationBuffer`（工具变量写入缓冲到 floor commit）；WE 工具 `set_variable` 直接写入 sandbox。TH 23 个资源工具 vs WE 14 个。

### 回合编排

| TH | WE |
|----|-----|
| `packages/core/src/orchestration/turn-orchestrator.ts` | `internal/engine/api/game_loop.go` |
| `packages/core/src/orchestration/director.ts` | `game_loop.go`（inline） |
| `packages/core/src/orchestration/verifier.ts` | `game_loop.go`（inline） |
| `apps/api/src/services/generation-guard-service.ts` | `session/manager.go#StartTurn`（SELECT FOR UPDATE） |

### Runtime Substrate

| TH | WE |
|----|-----|
| `apps/api/src/services/runtime-job-catalog.ts` | 计划 P-4G |
| `apps/api/src/services/runtime-worker.ts` | `internal/engine/memory/worker.go`（goroutine） |
| `apps/api/src/services/runtime-revision-guard.ts`（CAS） | 无对等（明确不做） |
| `apps/api/drizzle/0024_background_job_runtime.sql` | 计划 P-4G |

### ST 兼容（导入/导出）

| TH | WE |
|----|-----|
| `packages/adapters-sillytavern/src/parsers/character-parser.ts` | `internal/creation/card/parser.go` |
| `packages/adapters-sillytavern/src/parsers/preset-parser.ts` | `creation/template/import_st_preset.go` |
| `packages/adapters-sillytavern/src/parsers/worldbook-parser.ts` | `creation/lorebook/import_st.go` |
| `apps/api/src/routes/exports.ts`（对话导出） | 计划 P-4E |
| `packages/adapters-sillytavern/src/serializers/character-serializer.ts` | 无对等（角色卡导出） |

### 宏系统

| TH | WE |
|----|-----|
| `apps/api/src/lib/preset-utils.ts`（基础替换） | `internal/engine/macros/expand.go` |
| ST MacroEngine 2.0（Chevrotain AST） | 无对等（WE 用正则替换） |

**差异：** TH 宏系统不完整，委托调用方。WE 已实现 `{{char}}/{{user}}/{{persona}}/{{getvar::key}}/{{time}}/{{date}}`，比 TH 后端更完整。P-4K 计划扩展为可注册 Registry + 副作用宏 + 嵌套求值。

---

## 三、WE 独有功能（TH 无对应）

| WE 功能 | 代码位置 |
|---------|---------|
| game-package.json 游戏包 | `creation/template/package_*.go` |
| VN 指令解析 | `engine/parser/vn_parser.go` |
| 素材库 + search_material | `creation/asset/` + `engine/tools/material_tool.go` |
| ScheduledTurn | `engine/scheduled/trigger.go` |
| creation-agent（AI 辅助创作） | `creation/api/`（creation-agent 路由） |
| Memory stage_tags 分阶段过滤 | `memory/store.go#GetForInjection` |
| MemoryEdge 关系图 | `memory/edge.go` |
| Worldbook 变量门控（`var:` 语法） | `pipeline/node_worldbook.go` |
| Impersonate 全管线 Suggest | `engine/api/engine_methods.go#Suggest` |
| platform/play 玩家发现层 | `platform/play/handler.go` |
