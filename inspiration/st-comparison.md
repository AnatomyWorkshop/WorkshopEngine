# WorkshopEngine vs TavernHeadless 全景对比

> 更新于 2026-04-08（第五版 — TH Beta3 封板分析 + Integration Kit + 聚合根 + Runtime Substrate）
>
> 参考源：TavernHeadless monorepo（core / adapters-sillytavern / architecture.md），本地 clone 最新 commit `bdafb0a`（2026-04-07，v0.2.0-beta.3）
> WE 当前代码库状态：backend-v2
>
> 详细渲染架构与宏指令分析见：`inspiration/2026-04-08-th-rendering-and-macro-analysis.md`
> TH Beta3 架构分析（Integration Kit / 聚合根 / Runtime Substrate）见：`inspiration/2026-04-08-th-beta3-architecture.md`

---

## 一、系统定位

| 系统 | 定位 | 技术栈 |
|------|------|--------|
| **SillyTavern（ST）** | 一体化桌面应用（UI + 后端 + 插件生态），浏览器端执行正则/变量 | Node.js + Express |
| **TavernHeadless（TH）** | 无头 REST API 服务，复刻 ST 格式与行为，适合服务器部署 | TypeScript + Fastify + SQLite |
| **WorkshopEngine（WE）** | 无头 REST API 运行时，游戏打包/发布为核心差异化，多租户 API-first | Go + Gin + PostgreSQL |

WE 和 TH 共同方向：把 ST 的前端逻辑迁移到服务器后端。差异在于：TH 严格对齐 ST 格式，WE 用更清晰的抽象（PromptBlock IR、游戏包）解决相同问题。

---

## 二、TavernHeadless 回合流水线（参考基准）

```
Turn 请求
  │
  ├─ 1. transition          Floor 状态机迁移（draft → generating）
  ├─ 2. director            Director 槽：上下文分析 / 剧情控制（廉价模型）
  ├─ 3. tool_setup          工具注册表装载（Builtin + Preset + Resource + MCP）
  ├─ 4. memory_retrieval    记忆检索 + 注入准备
  ├─ 5. generation          Narrator LLM 流式生成（Vercel AI SDK）
  ├─ 6. verifier            Verifier 槽：输出格式/一致性/安全校验（廉价模型）
  ├─ 7. memory_consolidation 摘要提取 + 记忆整合（异步，Background Job Runtime）
  └─ 8. commit              短事务提交（prompt_snapshot + tool_record + variable flush）
```

WE 当前实现了所有 8 个阶段（Verifier 和 PromptSnapshot 刚于 2026-04-06 完成）。

---

## 三、逐项对比

### 3.1 会话结构

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| Session / Floor / Page 三层 | ✅ | ✅ 完整实现 | 对等 |
| Swipe 多页选择 | ✅ | ✅ `PATCH /floors/:fid/pages/:pid/activate` | 对等 |
| 会话 Fork / 平行时间线 | ✅ Floor 内 branch_id | ✅ 创建新 Session（复制历史段） | WE 语义更强（批量分叉），但无 branch_id |
| Session 内分支（branch_id） | ✅ 会话内多时间线共存 | ❌ | TH 独有，中期目标 |
| FloorRunSnapshot（生成阶段实时追踪） | ✅ phase / pendingOutput | ❌ | TH 独有，前端展示生成进度用 |
| 乐观锁（expectedVersion） | ✅ 防并发编辑冲突 | ❌ | TH 独有，低优先级 |
| 对话导入/导出（.thchat / .jsonl） | ✅ ChatTransferJob | ❌ | TH 独有，中期 |

---

### 3.2 Prompt 编排

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| System Prompt 模板（宏替换） | ✅ | ✅ `{{var}}` 宏展开 | 对等 |
| Preset Entry（injection_order / position） | ✅ | ✅ 含 CRUD + reorder | 对等 |
| **PromptBlock 优先级 IR** | ❌（位置式组装） | ✅ 每节点产出 Block，Runner 按 Priority 统一排序 | **WE 独有** |
| Prompt 格式模板（ChatML / Llama3 / Alpaca） | ✅ ST adapter | ❌ 仅 OpenAI message 格式 | TH 独有，本地模型必需 |
| Regex Profile（可复用规则集） | ✅ | ✅ 完整 CRUD + enabled 控制 | 对等 |
| Prompt Dry-Run（不调用 LLM） | ✅ `POST /prompt/dry-run` | ✅ `GET /sessions/:id/prompt-preview` | 对等 |
| **Prompt 快照（PromptSnapshot）** | ✅ 冻结版本 + 命中词条 | ✅ 刚完成（worldbook IDs + preset_hits + est_tokens + verifier 结果） | 对等 |

---

### 3.3 世界书（WorldInfo）

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 主关键词触发 | ✅ | ✅ | 对等 |
| 正则关键词（`/pattern/flags`） | ✅ | ✅ `regex:` 前缀 | 对等 |
| 次级关键词 + 逻辑门（AND_ANY / AND_ALL / NOT_ANY / NOT_ALL） | ✅ | ✅ | 对等 |
| 扫描深度（scan_depth） | ✅ | ✅ | 对等 |
| 注入位置（before / after / at_depth） | ✅ | ✅ | 对等 |
| 全词匹配（whole_word） | ✅ | ✅ | 对等 |
| 常驻词条（constant） | ✅ | ✅ | 对等 |
| **递归激活** | ✅ | ✅ 已激活词条内容触发二次扫描 | 对等 |
| **命中词条 ID 暴露** | ✅ prompt_snapshot | ✅ 刚完成（ActivatedWorldbookIDs → ContextData） | 对等 |
| 互斥分组（group，同组最多 N 条） | ✅ | ❌ | 中期 |
| 大小写敏感（per-entry） | ✅ | ⚠️ 全局 case-insensitive | 低优先级 |

---

### 3.4 记忆系统

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 记忆存储 / CRUD | ✅ | ✅ | 对等 |
| **时间衰减排序** | ❌（静态 importance） | ✅ 指数半衰期 + MinDecayFactor | **WE 更强** |
| 维护策略（deprecate / purge） | ✅ 生命周期状态机 | ✅ 全局维护 Worker | 对等 |
| 异步整合 Worker（N 回合触发） | ✅ MemoryJob 队列 | ✅ 独立 Worker + 租约 + 批次并发 | 对等 |
| **结构化整合输出（JSON facts）** | ✅ `{turn_summary, facts_add, facts_update, facts_deprecate}` | ❌ 仅自由文本摘要 | TH 更完整，中期目标 |
| **记忆边（MemoryEdge）** | ✅ supports / contradicts / updates 关系图 | ❌ | TH 独有，中期 |
| 记忆范围紧缩（MemoryScope compaction） | ✅ | ❌ | TH 独有，低优先级 |
| 双层摘要（compact + extended） | ✅ Memory V2 | ❌ 单层 summary + fact | TH 更完整 |

---

### 3.5 变量系统

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 五层变量（global / chat / branch / floor / page） | ✅ DB 行存储 | ✅ 内存 Sandbox + CommitTurn 持久化 | 对等（实现不同） |
| Macro 宏替换（`{{var}}`） | ✅ | ✅ | 对等 |
| 变量批量操作（batch PATCH） | ✅ | ❌ 仅单次 PATCH | 小差距 |
| 变量层级可视化 | ✅ `flattenVariableSnapshot` | ❌（API 返回 Flatten，无分层） | 客户端层 |

---

### 3.6 工具系统

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 原生 Agentic Loop | ✅ | ✅（最多 5 轮） | 对等 |
| Tool ReplaySafety 分级 | ✅ | ✅ safe / confirm_on_replay / never_auto_replay | 对等 |
| **ResourceToolProvider（资源工具）** | ✅ 23 个（character + worldbook + preset + regex CRUD） | ✅ 14 个（worldbook / preset / memory / material 读写 + create） | 对等（WE 范围窄，按需扩展） |
| **Preset 工具（用户自定义 HTTP 回调）** | ✅ PresetToolProvider | ✅ `preset:*` / `preset:<name>` 动态加载 | 对等 |
| **Tool 执行记录（ToolExecutionRecord）** | ✅ DB 持久化 + 审计 | ✅ DB 持久化 + `GET /tool-executions` 查询 | 对等 |
| **MCP 协议（stdio + HTTP transport）** | ✅ 完整 McpConnectionManager | ❌ | TH 独有，中期目标 |
| 内置工具（变量读写 + 记忆搜索） | ✅ | ✅ get/set_variable + search_memory/material | 对等 |
| **search_material（素材检索工具）** | ❌ | ✅ JSONB 标签/情绪/风格检索 | **WE 独有** |
| Tool 异步/延迟执行 | ✅ | ❌ 仅同步 | 低优先级 |

---

### 3.7 LLM 多角色槽

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| LLM Profile + Binding 5 级优先级 | ✅ | ✅ ResolveForSlot | 对等 |
| SSE 流式生成 | ✅ | ✅ | 对等 |
| 精确 Token 计数（分词器） | ✅ provider-specific | ⚠️ 启发式估算（BPE 兼容） | 差距小 |
| **Director 槽**（廉价模型预分析） | ✅ | ✅ `director_prompt` in Config，绑定 "director" slot | 对等 |
| **Verifier 槽**（输出一致性校验） | ✅ | ✅ 刚完成（`verifier_prompt` in Config，绑定 "verifier" slot） | 对等 |
| Memory 槽（专用摘要模型） | ✅ 独立 Memory 实例，结构化 JSON 输出 | ⚠️ Worker 用同一 LLM，自由文本输出 | TH 更完整 |
| Anthropic / Google / xAI 原生适配 | ✅ Vercel AI SDK | ❌ 仅 OpenAI compat | 中期 |
| 生成队列（fifo / priority） | ✅ | ❌ | 低优先级 |

---

### 3.8 创作工具

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 角色卡（TavernCardV2/V3）导入 | ✅ | ✅ PNG 解析 + 结构化存储 | 对等 |
| **游戏包打包/解包** | ❌ | ✅ `POST /templates/import` / `GET /templates/:id/export` | **WE 独有** |
| **ST 预设批量导入** | ✅ | ✅ `POST /templates/:id/preset/import-st` | 对等 |
| **ST 世界书批量导入** | ✅ | ✅ `POST /templates/:id/lorebook/import-st` | 对等 |
| Preset Entry CRUD + reorder | ✅ | ✅ | 对等 |
| Worldbook Entry CRUD | ✅ | ✅ | 对等 |
| Regex Profile CRUD | ✅ | ✅ | 对等 |
| **素材库（Material Library）** | ❌ | ✅ CRUD + batch 导入 + search_material 工具 | **WE 独有** |
| **模板发布状态（draft → published）** | ❌（dev only） | ✅ 状态机控制，published 才对玩家可见 | **WE 独有** |
| **AI 辅助创作（creation-agent）** | ❌ | ✅ 使用 resource:* 工具对话式修改游戏规则 | **WE 独有** |
| **ScheduledTurn（NPC 自主回合）** | ❌ | ✅ variable_threshold 触发 + cooldown 持久化 | **WE 独有** |
| 角色版本管理（rollback） | ✅ characterVersions | ❌ | TH 独有，低优先级 |

---

### 3.9 工程基础设施

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| **Event Bus（50+ 事件类型）** | ✅ emittery | ❌ | TH 独有，中期（插件/监控基础） |
| **Background Job Runtime** | ✅ runtime_job / lease / retry / dead letter | ⚠️ goroutine + 租约（内存级，进程重启丢失） | TH 更可靠，长期目标 |
| **Mutation Runtime** | ✅ 统一变更语义 + confirm_on_replay | ❌ | TH 独有，长期 |
| OpenAPI 文档（Swagger UI） | ✅ | ❌ | 中期（swaggo） |
| 官方 TypeScript SDK | ✅ @tavern/sdk | ❌ | API 冻结后生成 |
| JWT Auth | ✅ | ❌ | 中期 |
| 多账户（Multi-Account） | ✅ accounts 表 | ✅ X-Account-ID / user_id | 对等（实现不同） |

---

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

---

## 五、接下来的工作

### Tier 1 — 引擎能力补全（中期，高价值）

| 任务 | 描述 | 对应 TH 功能 | 复杂度 |
|------|------|-------------|--------|
| **结构化 Memory 整合** | Memory Worker 输出 JSON `{turn_summary, facts_add, facts_update, facts_deprecate}`，分离 fact_key 系统 | Memory V2 | 中 |
| **MCP 协议接入** | McpConnectionManager（stdio + HTTP），McpToolProvider 注册到 Registry | McpConnectionManager | 中-高 |
| **Worldbook 互斥分组** | `group` 字段，同组最多激活 N 条词条 | WorldInfo group | 低 |
| **Session 内分支（branch_id）** | Floor 层加 branch_id，支持同会话多时间线，目前 Fork 创建新 Session 只是近似替代 | floor.branch_id | 中-高 |
| **Memory Edge（关系图）** | supports / contradicts / updates 关系，供 Memory Lint 使用 | memory_edge 表 | 中 |
| **边界归档 API** | `POST /sessions/:id/archive`，生成结构化摘要写入高重要性 Memory | ChatTransferJob（部分） | 低-中 |

### Tier 2 — 平台工程（长期）

| 任务 | 描述 | 优先级 |
|------|------|--------|
| **Event Bus** | Floor/Memory/Tool/Variable 50+ 事件，供 webhook/监控/插件消费 | 中 |
| **多 Provider 原生适配** | Anthropic / Google / xAI 原生 API（非 OpenAI compat 路径） | 中 |
| **JWT Auth** | 标准 JWT 鉴权 + 账户资源隔离 | 中 |
| **OpenAPI 文档（swaggo）** | 自动从代码注释生成 Swagger UI | 低 |
| **对话导入/导出** | ST 格式（.jsonl）互转 | 低 |
| **Background Job Runtime** | runtime_job 表 + lease + retry + dead letter，替代当前 goroutine 方案 | 低（当前 goroutine 够用） |

### Tier 3 — WE 独有扩展

| 任务 | 描述 |
|------|------|
| **VN 渲染引擎（前端）** | rich 类型游戏的立绘/背景图/BGM/选项 directive 解析 |
| **MVM 渲染层** | 游记从游玩片段导出，按 vn-full/narrative/minimal/pure-text 降级渲染 |
| **创作层 AI 工具补全** | creation-agent 工具扩展：package_game / unpack_game / edit_preset_entry |
| **论坛/社区层** | GW 的帖子/游记/评论 API，与 WE 游玩层解耦 |

---

## 六、TH 近期重要更新（2026-03 ~ 2026-04，第五版补充）

### TH Beta3 封板（`bdafb0a`，2026-04-07，v0.2.0-beta.3）

14/14 Beta3 Ready 标准全部达成，主要新增能力：

| 特性 | 说明 | WE 参考 |
|------|------|---------|
| **`floor_run_state` 精细状态机** | `running/completed/failed/cancelled/superseded`，支持 retry/cancel | WE 4-H SSE Phase 事件是轻量替代 |
| **Branch-scoped variables** | 分支级变量作用域，不同 branch_id 的 Floor 拥有独立变量沙箱 | WE 3-G 落地后再评估 |
| **Secret 存储加密** | `secret_config_encrypted` 字段，API Key AES-256-GCM 加密 | WE 4-A |
| **Deferred MCP tool runtime** | MCP 工具需用户授权后延迟执行（对话框 confirm → 入 job queue） | WE 暂不需要（Preset Tool 已够用）|
| **Runtime Job 系统** | `runtime_job` 表持久化，lease/retry/dead_letter；进程重启不丢任务 | WE 4-G |
| **对话导入/导出** | ST JSONL + `.thchat` 原生格式，完整四层树（session/floor/page/message）| WE 4-E |
| **ResourceToolProvider 23 个工具** | AI 可直接 CRUD 角色卡/世界书/预设/正则 | WE 已有 14 个，可按需扩展 |
| **MCP 客户端集成** | stdio + Streamable HTTP 双传输，`McpConnectionManager` | WE 3-H 候选 |
| **Character Card V3 export with characterBook** | 角色卡导出含内嵌世界书；`alternateGreetings` 作为 floor 0 的 swipe pages；`systemPrompt`/`postHistoryInstructions` 自动注入 Preset | WE 3-I 设计参考 |
| **Worldbook outlet position 修复** | `position=at_depth` 现在按 `scan_depth` 正确计算 outlet 位置 | WE 3-J 实现参考 |
| **Dry-run worldbook match trace** | `debugOptions.includeWorldbookMatches` 暴露触发词条调试信息 | WE `prompt-preview` 可扩展 |
| **Soft-supersede for regenerated floors** | 重新生成时旧 Floor 置 `superseded`（软删除），保留时间线完整性 | WE 中期可参考，当前直接覆盖 |
| **Mutation Runtime** | 统一变更语义 + CAS 并发写入保护（RuntimeRevisionGuard） | WE 3-K（轻量版：DB `generating` 字段）|
| **Official Integration Kit** | `@tavern/sdk`（typed HTTP + SSE client）+ `@tavern/client-helpers`（StreamStateReducer / buildTimeline）| WE API 稳定后可参考生成 TS SDK |

**TH 宏指令状态：** 基础 `{{char}}/{{user}}` 替换委托给调用方，后端未实现完整宏系统。WE 需自建 `macros.Expand()`（3-I.1）。

**TH 渲染层：** TH 的 `apps/web` 是**创作工作台**（Narrative Workspace），不是游玩 UI，无 VN 渲染。ST 不使用 WebGL，是纯 HTML/CSS/JS。详见 `2026-04-08-th-rendering-and-macro-analysis.md`。

**TH 架构定位（Beta3 稳定后）：** TH 目标是"开发者友好的无头 API 服务"，官方 Integration Kit（`@tavern/sdk` + `@tavern/client-helpers`）是面向第三方开发者的 SDK。WE 目标是"游戏发布平台"，不需要对外发布 SDK，但可参考 Integration Kit 的 StreamStateReducer 设计提取前端 composable。详见 `2026-04-08-th-beta3-architecture.md`。

---

## 七、一句话定位

> WorkshopEngine 不是 SillyTavern 的替代品，也不试图完整复制 TavernHeadless。
>
> WE 的差异化在于：**游戏打包发布**（game-package.json）、**One-Shot 结构化 Parser**（含 VN Directives）、
> **素材库**（search_material）、**ScheduledTurn**（NPC 自主回合）和**前端游玩 UI**（TH 无此能力）。
>
> 与 TH 的主要差距在于：**宏展开**（3-I.1）、**VN 渲染资产系统**（4-I）、**MCP**（3-H 候选）和 **Session 内分支**（3-G）。
