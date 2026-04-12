# TavernHeadless → WorkshopEngine 代码位置对照

> 版本：2026-04-12（TH `4399619` 2026-04-12 + WE P-4G 完成）
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
| `apps/api/src/services/runtime-job-catalog.ts` | `internal/engine/scheduler/scheduler.go`（P-4G ✅） |
| `apps/api/src/services/runtime-worker.ts` | `internal/engine/memory/worker.go`（P-4G ✅ DB 调度） |
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
| `apps/api/src/services/st-macros/runtime.ts` | `internal/engine/macros/expand.go` |
| `apps/api/src/services/st-macros/types.ts` | 无对等（WE 无结构化 warning/trace） |
| `apps/api/src/services/st-macros/if-condition.ts` | 无对等（WE 无 `{{if}}` 块） |
| `apps/api/src/services/st-macros/variable-path.ts` | 无对等（WE 无结构化路径） |
| `apps/api/src/lib/preset-utils.ts`（基础替换） | `internal/engine/macros/expand.go` |

#### 设计对比

**TH 宏系统（ST Macro Compatibility Core Profile）**

TH 在 `runtime.ts` 实现了一个完整的 AST 求值器：

- **解析层**：`parseMacroNodes` 将输入文本解析为节点树（`text` / `raw` / `macro` / `if`），而非直接字符串替换
- **`{{if}}` 块**：完整支持 `{{if cond}}...{{else}}...{{/if}}`，含短路求值（未命中分支的写宏不执行）
- **变量读写宏**：`getvar` / `setvar` / `addvar` / `incvar` / `decvar` / `deletevar` 及 global 变体，支持结构化路径（`资产.金币`、`角色['属性'].力量`）
- **副作用隔离**：写宏不直接落库，而是进入 `variableOverlay`（staged buffer），只有 turn commit 时才真正持久化；dry-run / preview 阶段只记录 `mutationPreview`，无副作用
- **执行阶段**：`import` 阶段完全禁止求值；`preview` / `dry_run` / `assemble` / `commit_consume` 各有不同的副作用权限
- **安全限制**：`maxDepth`（16）、`maxSteps`（256）、`maxExpandedLength`（32768）、`maxMutationCount`（128）
- **结构化输出**：`StMacroEvalResult` 包含 `warnings`（带 code）、`usedMacros`、`mutationPreview`、`stagedMutations`、`traces`，可用于调试和审计
- **循环检测**：`evaluationStack` 检测重复展开路径，防止无限递归

**WE 宏系统（当前 `expand.go`）**

WE 当前是纯字符串替换：

- `strings.ReplaceAll` 链式替换固定宏（`{{char}}`、`{{user}}`、`{{persona}}`、`{{time}}`、`{{date}}`）
- `regexp.ReplaceAllStringFunc` 处理 `{{getvar::key}}`（正则匹配，直接查 map）
- 无 AST、无 `{{if}}` 块、无写宏、无副作用隔离、无 warning/trace 输出
- 未知宏保留原文（不报错）
- 展开结果不递归（防止无限循环，但也无法处理宏嵌套）

#### 优劣分析

| 维度 | TH | WE（当前） |
|------|-----|-----------|
| 实现复杂度 | 高（~1000 行 TS，4 个文件） | 低（~110 行 Go，1 个文件） |
| `{{if}}` 条件块 | ✅ 完整支持 | ❌ 不支持 |
| 变量写宏 | ✅ setvar/addvar/incvar/deletevar | ❌ 不支持 |
| 副作用隔离 | ✅ staged buffer，commit 才落库 | N/A（无写宏） |
| 结构化路径 | ✅ 点路径 + 引号 key | ❌ 仅 flat key |
| 执行阶段控制 | ✅ import/preview/dry_run/assemble/commit | ❌ 无阶段概念 |
| Warning/Trace | ✅ 结构化，16 种 warning code | ❌ 无 |
| 安全限制 | ✅ depth/steps/length/mutation 四重限制 | ❌ 无 |
| 循环检测 | ✅ evaluationStack | ❌ 无（靠"不递归"规避） |
| 可扩展性 | 中（新宏需改 runtime.ts） | 低（新宏需改 expand.go） |
| 性能 | 中（AST 解析有开销） | 高（纯字符串操作） |

#### WE 是否需要改思路？

**短期（P-4K 之前）：不需要大改。**

WE 当前宏系统的覆盖范围（`{{char}}`/`{{user}}`/`{{persona}}`/`{{getvar::key}}`/`{{time}}`/`{{date}}`）已经满足现有游戏包的实际需求。TH 的复杂度来自于它需要兼容 ST 的完整宏语法（包括 `{{if}}`、写宏、legacy alias），而 WE 的游戏包是自己设计的，可以选择不依赖这些特性。

**中期（P-4K）：按需扩展，不必全量复制 TH。**

P-4K 计划的"可注册 Registry + 副作用宏 + 嵌套求值"方向是正确的，但实现时可以参考 TH 的以下设计：

1. **副作用隔离**：写宏（如果引入）应该进入 staged buffer，不直接写 DB。TH 的 `variableOverlay` 模式值得借鉴。
2. **执行阶段**：引入 `MacroPhase`（`assemble` / `dry_run` / `preview`），控制写宏是否执行。
3. **安全限制**：至少加 `maxDepth` 和 `maxSteps`，防止恶意游戏包触发无限展开。
4. **`{{if}}` 块**：如果游戏包需要条件文本，可以参考 TH 的 `tryParseIfBlock` 实现，但可以先只支持简单的 truthy 判断，不需要完整的比较运算符。

**不需要复制的部分：**

- TH 的 legacy alias（`<USER>` → `{{user}}`）：WE 游戏包不用 ST 旧语法
- TH 的 `getglobalvar` / `setglobalvar` 区分：WE 变量系统已有 scope 层级，宏层不需要再区分
- TH 的结构化路径（`资产.金币`）：WE 推荐扁平前缀，宏层直接 flat key 查找即可
- TH 的完整 warning/trace 系统：WE 可以简化，只在 dry-run 时输出调试信息

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
