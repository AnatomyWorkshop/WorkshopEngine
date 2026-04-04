# WorkshopEngine 能力全景对比

> 更新于 2026-04-04（第二版 — 完整 TavernHeadless 架构分析）
>
> 参考源：TavernHeadless monorepo（core / adapters-sillytavern / official-integration-kit）
> + SillyTavern 功能文档

---

## 一、比较框架

三方参与比较：

| 系统 | 定位 |
|------|------|
| **SillyTavern（ST）** | 一体化桌面应用（UI + 后端 + 插件生态） |
| **TavernHeadless（TH）** | 无头 REST API 服务，紧密对齐 ST 数据格式，TypeScript/Fastify |
| **WorkshopEngine（WE）** | 无头 REST API 运行时，API-first 多租户设计，Go/Gin |

本文档聚焦**引擎层**差距。UI 层、TTS/STT、图像生成等客户端功能均超出范围。

---

## 二、TavernHeadless 架构速览

TH 是一个 8 阶段 Turn Orchestrator 驱动的多层系统：

```
Turn 请求
  │
  ├─ 1. transition          Floor 状态机迁移（draft → generating）
  ├─ 2. director            Director 槽：上下文分析 / 剧情控制指令
  ├─ 3. tool_setup          工具注册表装载
  ├─ 4. memory_retrieval    记忆注入准备
  ├─ 5. generation          LLM 流式生成（Vercel AI SDK）
  ├─ 6. verifier            Verifier 槽：输出格式/安全校验
  ├─ 7. memory_consolidation 摘要提取 + 记忆整合（可选异步）
  └─ 8. commit              事务提交（含重试）
```

关键架构组件：
- **PromptAssembler**：加载 Preset + Worldbook（含 trigger 逻辑） + RegexProfile → 组装 PromptIR → 应用 Regex 后处理
- **ToolRegistry**：4 类 Provider（Builtin / Preset / MCP / Resource）
- **ResourceToolProvider**：23 个内置资源工具（CRUD: character / worldbook / preset / regex）
- **Memory V2**：双层摘要（compact + extended）+ MemoryEdge（关系图）+ MemoryScope（范围紧缩）
- **EventBus**：50+ 事件类型（emittery），供插件/监控消费
- **Multi-provider**：Vercel AI SDK 抽象（OpenAI、Anthropic、Google、xAI）

---

## 三、逐项对比

### 3.1 会话结构

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| Session / Floor / Page 三层 | ✅ sessions / floors / messagePages | ✅ 完整实现 | 对等 |
| Swipe 多页选择 | ✅ | ✅ `PATCH /floors/:fid/pages/:pid/activate` | 对等 |
| 会话 Fork / 平行时间线 | ✅ floor branch（按单 Floor 分叉） | ✅ Session Fork（按任意 FloorSeq 复制全段历史） | WE 语义更强（批量分叉） |
| Floor 实时运行状态追踪 | ✅ FloorRunSnapshot（phase / pendingOutput / verifier） | ❌ | TH 独有，用于前端展示生成进度 |
| 乐观锁（expectedVersion） | ✅ 所有可变资源更新携带版本号 | ❌ | TH 独有，防并发编辑冲突 |
| 对话导入/导出（thchat / jsonl） | ✅ ChatTransferJob | ❌ | TH 独有 |

---

### 3.2 Prompt 编排

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| System Prompt 模板 | ✅ | ✅ | 对等 |
| Preset Entry（injection_order / position） | ✅ | ✅ | 对等 |
| Prompt 格式模板（ChatML / Llama3 / Alpaca 等） | ✅（ST adapter 兼容模式） | ❌ | TH 独有，本地模型必需 |
| **Regex Profile（可复用规则集）** | ✅ 独立资源（RegexProfile + RegexRule），支持 AI_OUTPUT / USER_INPUT | ❌ | **近期目标** |
| Prompt Dry-Run（不调用 LLM） | ✅ `POST /prompt/dry-run` | ✅ `GET /sessions/:id/prompt-preview` | 对等 |
| Prompt 快照（frozen IR，用于调试/重放） | ✅ promptSnapshots 表 | ❌ | TH 独有，可后续补 |

---

### 3.3 世界书（WorldInfo）

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 主关键词触发（primaryKeys） | ✅ | ✅ | 对等 |
| 正则关键词（`/pattern/flags`） | ✅ | ✅ `regex:` 前缀 | 对等 |
| **次级关键词 + 逻辑门**（AND_ANY / AND_ALL / NOT_ANY / NOT_ALL） | ✅ | ❌ | **近期目标** |
| **扫描深度**（scan_depth：只扫最近 N 条消息） | ✅ | ❌ | **近期目标** |
| **注入位置**（BEFORE_TEMPLATE / AFTER_TEMPLATE / AT_DEPTH） | ✅ | ❌ | **近期目标** |
| 全词匹配（whole_word） | ✅ | ❌ | 低优先级 |
| 大小写敏感控制（per-entry） | ✅ | ⚠️ 仅全局 case-insensitive | 低优先级 |
| 常驻词条（constant） | ✅ | ✅ | 对等 |
| **递归激活** | ✅ | ❌ | **近期目标** |
| 互斥分组（group，同组最多激活 N 条） | ✅ | ❌ | 中期 |

---

### 3.4 记忆系统

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 记忆存储 / CRUD | ✅ | ✅ | 对等 |
| 时间衰减排序 | ❌（按 importance 排序，无显式衰减） | ✅ 指数半衰期 + MinDecayFactor | **WE 更强** |
| 维护策略（deprecate / purge） | ✅ 生命周期状态机 | ✅ 全局维护 Worker | 对等 |
| **双层摘要**（compact summary + extended summary） | ✅ Memory V2 | ❌ 仅单层 summary + fact 两种类型 | TH 更完整 |
| **记忆边（MemoryEdge）** | ✅ 关系图（支持 mutual_implication / contradiction 等） | ❌ | TH 独有，高优先级低 |
| **记忆范围紧缩（MemoryScope compaction）** | ✅ 可 rebuild 指定范围的记忆 | ❌ | TH 独有 |
| 异步整合 Worker | ✅ MemoryJob 队列 | ✅ 独立 Worker 进程 | 对等 |
| 整合触发（N 回合触发） | ✅ | ✅ | 对等 |

---

### 3.5 变量系统

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 五层变量（global / chat / branch / floor / page） | ✅ DB 行存储，按 scope 读写 | ✅ 内存 Sandbox，CommitTurn 后持久化 | 对等（实现不同） |
| Macro 宏替换（`{{var}}`） | ✅ template-engine.ts | ✅ pipeline/node_template.go | 对等 |
| **变量批量操作** | ✅ batch PATCH | ❌ 仅单次 PATCH | 小差距 |
| 变量层级可视化（供前端 inspector 使用） | ✅ client-helpers `flattenVariableSnapshot` | ❌（API 已返回 Flatten，但无分层） | 客户端层 |

---

### 3.6 工具系统（Tools / Function Calling）

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 原生 Agentic Loop | ✅ | ✅（最多 5 轮） | 对等 |
| **Tool 重放安全分级** | ✅ safe / confirm_on_replay / never_auto_replay / uncertain | ❌ | **今日目标（零成本加入）** |
| **ResourceToolProvider（23 个资源管理工具）** | ✅ CRUD: character / worldbook / preset / regex | ❌ 仅 3 个内置工具 | 中期目标 |
| **MCP 协议接入**（stdio + HTTP transport） | ✅ 完整 McpConnectionManager | ❌ | 中期目标 |
| **Preset 工具（用户自定义工具）** | ✅ PresetToolProvider | ❌ | 中期目标 |
| 内置工具（变量读写 + 记忆搜索） | ✅（memory builtin） | ✅ get_variable / set_variable / search_memory | 对等 |
| Tool 执行记录（ToolExecutionRecord） | ✅ DB 持久化 | ❌ 无持久化 | 中期 |
| Tool 异步/延迟执行（deferred delivery） | ✅ | ❌ 仅同步 | 中期 |

---

### 3.7 LLM 调用层

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| OpenAI 兼容 API | ✅ | ✅ | 对等 |
| **Anthropic / Google / xAI 原生** | ✅ Vercel AI SDK 抽象 | ❌ 仅 OpenAI compat | 中期 |
| 采样参数（temperature / topP / topK / penalties） | ✅ | ✅ | 对等 |
| Per-request 参数覆盖 | ✅ | ✅ | 对等 |
| Profile / Binding 5 级优先级 | ✅ | ✅ ResolveSlot | 对等 |
| **Director 角色槽**（上下文分析/剧情控制） | ✅ | ❌ | 中期 |
| **Verifier 角色槽**（输出校验） | ✅ | ❌ | 中期 |
| 生成队列（fifo / priority / direct） | ✅ | ❌ | 低优先级 |
| SSE 流式 | ✅ | ✅ | 对等 |
| **精确 Token 计数**（分词器） | ✅ provider-specific | ❌ 粗估 | **近期目标** |

---

### 3.8 创作工具

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 角色卡（TavernCardV2/V3） | ✅ 解析 + 导入 | ✅ PNG 解析 | 对等 |
| **角色版本管理（rollback）** | ✅ characterVersions 表 | ❌ | 低优先级 |
| 素材上传 | ❌（ST 本身管理） | ✅ `/api/v2/assets/:slug/upload` | WE 独有 |
| Preset Entry CRUD | ✅ | ✅ 含 reorder | 对等 |
| Worldbook Entry CRUD | ✅ | ✅ | 对等 |
| **Regex Profile CRUD** | ✅ 独立资源 | ❌ | **近期目标** |

---

### 3.9 用户与鉴权

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| 管理员 API Key | ✅ | ✅ | 对等 |
| JWT Auth | ✅（auth_mode=jwt） | ❌ | 中期 |
| 多账户（Multi-Account） | ✅ accounts 表 | ✅ X-Account-ID | 对等（实现不同） |
| 用户 Persona（accountUsers） | ✅ | ❌ | 低优先级 |

---

### 3.10 工程基础设施

| 特性 | TH | WE | 评注 |
|------|----|----|------|
| OpenAPI 文档自动生成 | ✅ Swagger UI | ❌ | 中期（可用 swaggo） |
| **Event Bus（50+ 事件类型）** | ✅ emittery | ❌ | 中期（插件/监控基础） |
| **官方 TypeScript SDK** | ✅ @tavern/sdk + @tavern/client-helpers | ❌ | API 稳定后生成 |

---

## 四、WorkshopEngine 的差异化优势

| 特性 | 描述 |
|------|------|
| **PromptBlock 优先级 IR** | 每个节点只产出 Block，Runner 按 Priority 统一排序。TH 是位置式组装，WE 优先级模型更灵活 |
| **XML/JSON/Plaintext 三层回退解析** | 结构化 ParsedResponse（narrative / options / state_patch / vn）。TH 依赖 LLM 自行格式化 + regex 后处理 |
| **Session Fork（批量平行时间线）** | 从任意 FloorSeq 复制全段历史创建新 Session。TH 只能从单 Floor 分叉 |
| **原生 Agentic Tool Loop** | 引擎内置，无需扩展脚本 |
| **五层 Page 沙箱 + CommitTurn 提升** | 内存级变量隔离，Regen 时自动丢弃 Page 层变化，不污染 Chat 层 |
| **多租户 API-first 无状态设计** | 无 session cookie / 桌面依赖，天然适合服务端嵌入 |
| **记忆时间衰减（指数半衰期）** | TH 使用静态 importance 排序，WE 有动态衰减 |
| **VN Directives（视觉小说指令集）** | LLM 输出 `<vn>` 标签 → 结构化场景指令，TH 无此概念 |
| **MVM 渲染分层设计** | MODEL/MSG/VIEW 三层分离，前端客户端与引擎解耦 |

---

## 五、长期工作方向

### 第一阶段：完成 Prompt 编排能力（近期，进行中）

目标：引擎层 Prompt 编排能力完整对齐 TH + ST。

| 任务 | 描述 | 复杂度 |
|------|------|--------|
| ✅ **Regex Profile 系统** | `RegexProfile` + `RegexRule` 资源，AI 输出/用户输入后处理 | 低 |
| ✅ **WorldInfo 次级关键词 + 逻辑门** | AND_ANY / AND_ALL / NOT_ANY / NOT_ALL | 低 |
| ✅ **WorldInfo scan_depth + position** | 扫描窗口 + 注入位置（before/after/at_depth） | 低-中 |
| ✅ **WorldInfo 递归激活** | 已激活词条内容再次触发扫描 | 低 |
| ✅ **Tool ReplaySafety 分级** | safe / confirm_on_replay / never_auto_replay / uncertain | 低 |
| **精确 Token 计数** | tiktoken-go 或 API 反馈值校准 | 低-中 |

### 第二阶段：工具生态扩展（中期）

目标：工具调用覆盖引擎自身资源 + 外部生态接入。

| 任务 | 描述 | 复杂度 |
|------|------|--------|
| **ResourceToolProvider（创作工具）** | 在工具调用中读写 character / worldbook / preset / regex | 中 |
| **MCP 协议接入** | McpConnectionManager（stdio + HTTP），McpToolProvider 注册到 Registry | 中 |
| **用户自定义工具（Preset Tool）** | 通过 API 注册自定义工具定义，运行时动态加载 | 中 |
| **工具执行持久化** | ToolExecutionRecord 表，支持查询和 replay | 低-中 |

### 第三阶段：多 LLM 角色槽（中期）

目标：Director + Verifier 槽，构建多 AI 协作管道。

| ���务 | 描述 | 复杂度 |
|------|------|--------|
| **Director 槽** | generation 前的上下文分析/剧情控制指令（廉价模型） | 中 |
| **Verifier 槽** | generation 后的输出校验（格式/安全/一致性） | 中 |
| **多 Provider 注册表** | Anthropic / Google / xAI 原生适配（非 OpenAI compat 路径） | 中 |

### 第四阶段：平台工程（长期）

目标：生态可扩展性和运维可观测性。

| 任务 | 描述 | 复杂度 |
|------|------|--------|
| **Event Bus（引擎事件系统）** | 50+ 事件类型（Floor/Memory/Tool/Variable），供监控和 webhook 消费 | 中 |
| **OpenAPI 文档（swaggo）** | 自动从代码注释生成 Swagger UI | 低 |
| **官方 TypeScript SDK** | API 冻结后，基于 OpenAPI spec 生成类型 + 封装 resource 方法 | 中 |
| **JWT + 账户映射** | 标准 JWT 鉴权，账户资源隔离 | 中 |
| **对话导入/导出** | ST 格式 (.jsonl) + 原生格式互转 | 低 |

---

## 六、一句话定位

> WorkshopEngine 不是 SillyTavern 的替代品，
> 也不试图复制 TavernHeadless 的完整设计。
>
> 它是一个**结构更清晰的 API 运行时**：
> 用 PromptBlock 优先级 IR 代替位置式组装，
> 用结构化 ParsedResponse 代替正则后处理，
> 用 Session/Floor/Page 三层 + Fork 代替线性历史，
> 用内存变量沙箱代替 DB 行级 scope。
>
> 差距在于**工具生态**（MCP / ResourceTools）、**多 LLM 角色槽**、
> 和 **Regex / WorldInfo 编排能力**。
> 这三个方向是接下来 2 个阶段的核心工作。
