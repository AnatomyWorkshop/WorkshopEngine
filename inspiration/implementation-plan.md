# backend-v2 中期实现计划

> 原则：深度解耦、One-Shot LLM、每个节点对外有明确接口
> 状态更新：2026-04-06（Phase 3 重排：新增变量门控 + 分阶段记忆标签，Session 分支移后）

---

## 当前状态（已完成）

### 核心基础设施
- `internal/core/db` — PostgreSQL + GORM，AutoMigrate，全部模型已定义
- `internal/core/llm` — OpenAI 兼容客户端，Chat + ChatStream（SSE），精确 token 计数（`stream_options.include_usage`）
- `internal/platform/provider` — LLM Profile 动态解析，slot 优先级系统（session > global，精确 slot > 通配 `*`）

### 引擎层
- `internal/engine/prompt_ir` — Prompt 中间表示（PromptBlock IR），含 `ActivatedWorldbookIDs` 回传字段
- `internal/engine/pipeline` — 流水线节点（SystemPrompt / Worldbook / Memory / History / PresetEntry）
- `internal/engine/variable` — 五级变量沙箱（Global → Chat → Floor → Branch → Page）
- `internal/engine/parser` — AI 响应结构化解析（三层回退：XML → 编号列表 → fallback）
- `internal/engine/memory` — 记忆存取 + 结构化整合（fact_key 系统 + 指数衰减注入）
- `internal/engine/session` — Floor/Page 状态机（StartTurn / CommitTurn / RegenTurn / FailTurn）
- `internal/engine/processor` — Regex 后处理（user_input / ai_output / all）
- `internal/engine/scheduled` — 定时触发规则（variable_threshold 模式）
- `internal/engine/tools` — 工具注册表 + 内置工具（14 个资源工具 + get/set_variable + search_memory/material）
- `internal/engine/tools/http_tool.go` — Preset Tool HTTP 回调执行器

### API 层
- `internal/engine/api` — 游玩接口（PlayTurn / StreamTurn / ForkSession / PromptPreview）
- `internal/creation/api` — 创作接口（角色卡 / 模板 / 世界书 / PresetEntry / LLMProfile / Regex / Material / PresetTool）

### 已实现的 API 路由

**游玩层 `/api/v2/play`**
```
GET    /games                                   已发布游戏列表（公开字段）
POST   /sessions                                创建会话
POST   /sessions/:id/turn                       一回合（PlayTurn）
POST   /sessions/:id/regen                      重新生成（Swipe）
GET    /sessions/:id/stream                     SSE 流式输出
GET    /sessions/:id/state                      会话状态快照
PATCH  /sessions/:id                            更新会话标题/状态
DELETE /sessions/:id                            删除会话及关联数据
GET    /sessions/:id/variables                  变量快照
PATCH  /sessions/:id/variables                  合并更新变量
GET    /sessions                                列出会话（?game_id=&user_id=&limit=&offset=）
GET    /sessions/:id/floors                     楼层列表（含激活页摘要）
GET    /sessions/:id/floors/:fid/pages          Swipe 页列表
PATCH  /sessions/:id/floors/:fid/pages/:pid/activate  Swipe 选页
GET    /sessions/:id/memories                   记忆列表
POST   /sessions/:id/memories                   手动创建记忆
PATCH  /sessions/:id/memories/:mid              更新记忆字段
DELETE /sessions/:id/memories/:mid              删除记忆（?hard=true 物理删除）
POST   /sessions/:id/memories/consolidate       立即触发记忆整合（同步，调试用）
POST   /sessions/:id/fork                       分叉会话（平行时间线 / 存档点）
GET    /sessions/:id/prompt-preview             Prompt dry-run（不调用 LLM）
GET    /sessions/:id/floors/:fid/snapshot       Prompt 快照（Verifier 结果 + 命中词条）
GET    /sessions/:id/tool-executions            工具执行记录（?floor_id=&limit=）
```

**创作层 `/api/v2/create`**
```
POST/GET/DELETE /cards                          角色卡 CRUD
POST            /cards/import                   导入角色卡 PNG
GET/POST/DELETE /templates/:id/lorebook         世界书词条 CRUD
POST            /templates/:id/lorebook/import-st  ST 世界书 JSON 批量导入
GET/POST/PATCH/DELETE /templates                游戏模板 CRUD
POST            /templates/import               游戏包导入（game-package.json）
GET             /templates/:id/export           游戏包导出
POST            /templates/:id/preset/import-st ST 预设 JSON 批量导入
GET/POST/PATCH/DELETE /templates/:id/preset-entries  PresetEntry CRUD
PUT             /templates/:id/preset-entries/reorder 批量调整顺序
GET/POST/PATCH/DELETE /templates/:id/tools      Preset Tool CRUD
GET/POST/PATCH/DELETE /llm-profiles             LLM Profile CRUD
POST            /llm-profiles/:id/activate      绑定 Profile 到 slot
GET/POST/PATCH/DELETE /regex-profiles           Regex Profile CRUD
GET/POST/PATCH/DELETE /materials                素材库 CRUD
POST            /materials/batch                批量导入素材
```

---

## Phase 2（已完成，2026-04-06 截止）

- [x] **Preset Tool（用户自定义工具）**：HTTP 回调工具，`preset:*` / `preset:<name>` 动态加载
- [x] **ToolExecutionRecord 持久化**：异步写入，`GET /sessions/:id/tool-executions` 查询
- [x] **精确 token 计数**：SSE `stream_options.include_usage`，三通道返回
- [x] **Director 槽**：廉价模型预分析，结果注入主 LLM 上下文首位；`director_prompt` 在 `GameTemplate.Config` 配置；绑定 `slot="director"` 的 LLMProfile 即启用
- [x] **Verifier 槽**：主生成后一致性校验，`verifier_prompt` 在 `GameTemplate.Config` 配置；绑定 `slot="verifier"` 的 LLMProfile 即启用；失败不阻断回合，仅影响 PromptSnapshot `verify_passed` 标记
- [x] **PromptSnapshot 持久化**：每个 Floor 异步写入一条快照，记录命中词条 ID、preset_hits、worldbook_hits、est_tokens、verifier 结果
- [x] **ST 预设导入**：`POST /create/templates/:id/preset/import-st`
- [x] **ST 世界书导入**：`POST /create/templates/:id/lorebook/import-st`
- [x] **游戏包打包/解包**：`POST /templates/import` + `GET /templates/:id/export`，game-package.json 格式
- [x] **结构化 Memory 整合**：Memory Worker 输出 JSON `{turn_summary, facts_add, facts_update, facts_deprecate}`，`fact_key` 系统支持 upsert/deprecate，`GetForInjection` 带 `[key]` 前缀；旧格式（`<Summary>` + `事实：`）回退兼容

---

## Phase 3 — 引擎能力补全（当前阶段）

### 工作安排（按优先级顺序）

#### ~~3-A  Memory Edge（记忆关系图）~~ ✅ 2026-04-06 完成

`MemoryEdge` 表 + 4 种 relation（updates/contradicts/supports/resolves）。
`UpsertFact` 改为废弃旧行 + 新建行，`applyStructuredResult` 在 `facts_update` 时自动写 `updates` 边。
5 个路由：GET×2 / POST / PATCH / DELETE。双层压缩专用的 `derived_from`/`compacts` 推迟到双层摘要架构落地后再加。

#### ~~3-B  LLM 模型发现 + 连通性测试~~ ✅ 2026-04-06 完成

`DiscoverModels(ctx, baseURL, apiKey)` 调用 `/models` 接口返回 `[]ModelInfo{ID, Label}`；
`TestConnection(ctx, baseURL, apiKey, model)` 发送单字探测，返回 `{latency_ms, response_text}`。
2 个路由：`POST /api/v2/create/llm-profiles/models/discover` + `/models/test`。
错误返回 502 Bad Gateway（区分 Provider 侧错误与 WE 内部错误）。

#### ~~3-C  Worldbook 互斥分组（group）~~ ✅ 2026-04-06 完成

`WorldbookEntry` 新增 `Group string`、`GroupWeight float64` 字段。
`node_worldbook.go` 激活阶段后调用 `applyGroupCap`：同组条目按 `GroupWeight` 降序，最多保留 N 条（`GameTemplate.Config.worldbook_group_cap`，默认 1）。
Group 为空的词条不参与裁剪，常驻词条不受影响。
创作层 CRUD 自动透传（直接绑定 DB 结构体），ST 导入适配 `group`/`groupWeight` 字段。

#### ~~3-D  Worldbook 变量门控（`var:` 语法）~~ ✅ 2026-04-06 完成

`matchKey` 新增 `var:` 前缀识别，支持三种形式：
- `var:key=value` — 变量等值条件（引擎层强制，与扫描文本无关）
- `var:key!=value` — 不等条件
- `var:key` — 变量存在且非空

`matches` 和 `matchKey` 签名新增 `vars map[string]any` 参数，从 `ctx.Variables` 传入。
无 schema 变更，无 API 变更。典型用法：`"keys": ["var:game_stage=confrontation"]`。

---

#### 3-E  Memory 分阶段标签（stage_tags）

**为什么做：** 多幕式叙事中，前期调查线索/红鲱鱼条目在后期剧情中持续占用 token 并干扰 LLM。需要让创作者为 Memory 条目打上阶段标签，引擎在注入时按当前阶段过滤，实现"LLM 只看当前阶段需要的记忆"。

**与 TH 对比：** TH 无此设计，`GetForInjection` 全量注入所有非废弃记忆。

**具体工作：**
1. `Memory` 新增 `StageTags datatypes.JSON`（JSONB，`default:'[]'`，存 `[]string`）
2. `GetForInjection(sessionID, tokenBudget, currentStage string)` 新增过滤：
   - `stage_tags = '[]'` 的条目无阶段限制，始终注入
   - `stage_tags @> '["current_stage"]'` 的条目只在匹配阶段注入
3. `currentStage` 来源：`ctx.Variables["game_stage"]`，由引擎在调用前读取；为空时不过滤（兼容不使用分阶段的游戏）
4. Memory CRUD API（`POST/PATCH /sessions/:id/memories`）支持 `stage_tags` 字段读写
5. `applyStructuredResult` 可从 `facts_add` 的可选字段 `stage` 中读取标签，自动打标（无则留空）

**改动量：** DB 迁移 + `store.go` 约 30 行 + API 透传约 10 行，共约 50 行。

---

#### 3-F  边界归档 API

**为什么做：** 游戏结束 / 分享时需要一个结构化摘要，让后续游玩或论坛帖子有上下文。Karpathy LLM Wiki 分析（`karpathy-llmwiki-analysis.md`）指出：好的发现不应该消失在对话历史里，应该归档回知识库。

**与 TH 对比：** TH 有 `ChatTransferJob`（部分等价），WE 无此 API。

**具体工作：**
1. `POST /sessions/:id/archive`：调用廉价模型（Memory 槽）生成结构化 Markdown 摘要（主线事件 + 关键事实 + 当前变量快照）
2. 摘要写入 `importance=1.5`（高重要性）的 Memory，同时更新 `session.status = "archived"`
3. 响应体直接返回 Markdown 文本，供 GW 论坛帖子使用

---

#### 3-G  Session 内分支（branch_id）

**为什么做：** 当前 ForkSession 创建新 Session，平行时间线游玩体验割裂（两个存档互不关联）。

**与 TH 对比：** TH M13 `Floor.branch_id` 支持同会话多时间线，`GET /sessions/:id/branches` 列出所有分支。

**具体工作：**
1. `Floor` 新增 `BranchID string`（默认 `"main"`）
2. `session.Manager` 的 `StartTurn` 接受可选 `branchID` 参数；`GetHistory` 按 `branch_id` 过滤楼层
3. 新增路由：
   - `GET  /sessions/:id/branches`：列出所有分支（branch_id + floor_count）
   - `POST /sessions/:id/floors/:fid/branch`：从指定楼层创建新分支
   - `DELETE /sessions/:id/branches/:bid`：删除分支（保护 main）
4. `ForkSession` 保留为跨 Session 存档，`branch` 为 Session 内时间线

> **注意**：这是 Phase 3 改动量最大的项（涉及 Floor 读写全链路），排在新增的 3-D/E/F 之后进行。

---

#### 3-H  MCP 协议接入（暂缓，候选）

**为什么暂缓：** MCP 工具需要稳定的外部进程管理（stdio），在当前轻量 goroutine 架构下实现复杂度较高，且现有 Preset Tool（HTTP 回调）已能覆盖大多数集成场景。

**与 TH 对比：** TH M（MCP 集成） 有完整的 `McpConnectionManager` + 12 个 API 端点。WE 暂时用 Preset Tool 替代。

**触发条件**：当创作者需要接入本地 MCP 工具（如文件系统、代码执行）时再实现；云端 HTTP MCP 可直接用 Preset Tool。

---

## Phase 4 — 安全与平台工程

### 工作安排

#### 4-A  API Key 加密存储
**为什么做：** `LLMProfile.APIKey` 目前明文写入 DB，公网部署存在安全风险。
与 TH 对比：TH M18 `LLM Profile Vault` 用 AES-256-GCM 加密，存密文 + mask（前4位明文）。

具体工作：
1. `internal/core/secrets` — 新增 `Encrypt(plaintext, masterKey)` / `Decrypt(ciphertext, masterKey)` 函数（AES-256-GCM）
2. `LLMProfile.APIKey` 改为存密文，新增 `APIKeyMask string`（如 `sk-ab**...1234`）
3. 创作层 `POST /llm-profiles` 写入时加密；`ResolveForSlot` 解密后传给 LLM 客户端
4. 读取接口只返回 `api_key_mask`，不返回原文

#### 4-B  JWT Auth
**为什么做：** 当前 `X-Account-ID` Header 无签名，任何人都可以伪造账号 ID。
与 TH 对比：TH M17 `AUTH_MODE=off|api_key|jwt`，WE 目前等价于 `AUTH_MODE=off`。

具体工作：
1. `internal/platform/auth` — 新增 JWT 验证中间件（HS256，环境变量注入 secret）
2. Gin 路由组按 `/api/v2/play`（公开 + 可选鉴权）和 `/api/v2/create`（需要鉴权）分层
3. 支持 `AUTH_MODE=off|jwt`；`off` 模式维持现有 `X-Account-ID` 行为，兼容开发环境

#### 4-C  多 Provider 原生适配
当前只有 OpenAI compat 路径。接入 Anthropic（claude-opus/sonnet/haiku）原生 API；Google Gemini 按需。

#### 4-D  OpenAPI 文档（swaggo）
从代码注释自动生成 Swagger UI，优先覆盖游玩层路由。

#### 4-E  对话导入/导出
ST 格式（.jsonl）互转，供玩家备份存档或在 ST 和 WE 之间迁移对话历史。

---

## Phase 5 — WE 独有扩展

| 任务 | 描述 |
|------|------|
| **VN 渲染引擎（前端）** | rich 类型游戏的立绘/背景图/BGM/选项 directive 解析；backend 已输出 VNDirectives |
| **Preflight Rendering（预飞渲染）** | 主回合完成后异步预测下一轮 3 个选项（lazy：仅选项文本；eager：完整预生成 3 条分支回答），缓存至 Session 变量；前端零等待点选。由 `GameTemplate.Config.preflight_mode` 控制（lazy/eager/off）。参见 talk-with-alice 第一篇 Q2。 |
| **MVM 渲染层** | 游记从游玩片段导出，按 vn-full/narrative/minimal/pure-text 降级渲染 |
| **创作层 AI 工具补全** | creation-agent 工具扩展：package_game / unpack_game / edit_preset_entry |
| **论坛/社区层** | GW 的帖子/游记/评论 API，与 WE 游玩层解耦 |
| **玩家专属进化世界书** | 边界归档时，将本局衍生的新世界书条目写入玩家专属 Memory（`type=worldbook_evolution`），下局开始时作为高优先级词条注入，形成每个玩家独属的"故事沉淀层" |

---

## TH 功能对照与取舍

### WE 已对齐（核心功能同等能力）

| TH 功能 | WE 实现 | 差异说明 |
|---------|---------|---------|
| Session / Floor / MessagePage 三层 | ✅ | 对等 |
| Swipe 多页选择 | ✅ `PATCH .../pages/:pid/activate` | 对等 |
| Director / Verifier / Narrator 槽 | ✅ `ResolveForSlot` | 对等 |
| Prompt Pipeline + Block IR | ✅ 比 TH 更灵活（优先级排序） | **WE 更强** |
| 世界书（全部触发规则 + 递归激活） | ✅ | 对等 |
| Memory 衰减注入（半衰期） | ✅ | 对等 |
| 结构化 Memory 整合（JSON facts） | ✅ 2026-04-06 完成 | 对等 |
| Tool Registry + Agentic Loop | ✅ 最多 5 轮 | 对等 |
| ResourceToolProvider | ✅ 14 工具（TH 23 工具，范围窄） | WE 略少，按需扩展 |
| Preset Tool（HTTP 回调） | ✅ | 对等 |
| ToolExecutionRecord | ✅ | 对等 |
| PromptSnapshot（命中词条 + verifier） | ✅ | 对等 |
| ST 预设 / 世界书 / 角色卡导入 | ✅ | 对等 |
| 游戏包打包/解包 | ✅ game-package.json | **WE 独有** |
| ScheduledTurn（NPC 自主回合） | ✅ variable_threshold | **WE 独有** |
| 素材库 + search_material | ✅ | **WE 独有** |
| Session Fork（批量平行时间线） | ✅ 创建新 Session | WE 语义更强，但无 branch_id |

### WE 计划做（Phase 3 / 4）

| TH 功能 / 设计需求 | WE 计划 | 预期阶段 |
|---------|---------|---------|
| ~~memory_edge（关系图）~~ | ~~3-A~~ ✅ | Phase 3 |
| ~~LLM 模型发现 + 连通性测试~~ | ~~3-B~~ ✅ | Phase 3 |
| ~~Worldbook 互斥分组（group）~~ | ~~3-C~~ ✅ | Phase 3 |
| ~~Worldbook 变量门控（`var:` 语法）~~ | ~~3-D~~ ✅ | Phase 3 |
| Memory 分阶段标签（stage_tags） | 3-E | Phase 3 |
| 边界归档 API | 3-F | Phase 3 |
| Session 内 branch_id | 3-G | Phase 3 |
| API Key AES-256-GCM 加密 | 4-A | Phase 4 |
| JWT Auth | 4-B | Phase 4 |
| MCP 协议接入 | 3-H（暂缓） | Phase 3 候选 |
| 多 Provider 原生适配 | 4-C | Phase 4 |
| OpenAPI 文档（Swagger） | 4-D | Phase 4 |
| 对话导入/导出（ST JSONL） | 4-E | Phase 4 |

### WE 明确不做（定位不同 / 成本收益不足）

| TH / 其他引擎功能 | 不做原因 |
|---------|---------|
| **Character 版本管理（rollback）** | WE 用游戏包版本控制游戏内容，角色卡跟随游戏包迭代；不需要 session pin 到角色版本 |
| **Background Job Runtime（DB 持久化 job 表）** | 当前 goroutine + in-memory lease 对 WE 场景够用；引入 DB job 表增加运维复杂度，收益不足 |
| **llm_instance_config 独立表** | TH 为多账户 SaaS 场景设计；WE 的 LLMProfileBinding.Params 合并了两者，单租户场景无需拆分 |
| **WebSocket Event Bus（50+ 事件）** | WE 的 SSE 已覆盖前端实时需求；Event Bus 适合插件/监控生态，WE 当前没有插件扩展需求 |
| **Account User Binding 深度绑定** | WE 通过 `user_id` 字段简化处理，不需要 TH 的 account_user + session.user_snapshot 完整方案 |
| **记忆维护 CLI（dry-run 脚本）** | WE 有 `POST /sessions/:id/memories/consolidate` API 触发，CLI 工具对当前部署方式意义不大 |
| **OpenAPI 中英文文档站（VitePress）** | 面向创作者的文档是 CW 的职责，WE 引擎只需要 Swagger UI 供开发联调 |
| **真实 provider 最小回归 CI 脚本** | WE 用手动冒烟 + `.env` 覆盖；TH 的自动化回归脚本适合其 monorepo + CI 场景 |
| **WebGal 式 KV + 状态机（确定性数值系统）** | WebGal 的 KV + 快照重算适合严格规则型跑团（COC/DND 战斗公式）；WE 面向开放叙事，数值精确性不是核心需求。精确数值计算的折中方案：用 Director 槽计算，Narrator 槽叙事渲染，两者分工。 |
| **真正的实时多人联机（Room/分布式状态）** | 需要 WebSocket + 分布式锁 + conflict resolution，架构复杂度极高。WE 的四种轻量多人模式（旁观/轮流/异步论坛/观众投票）已能覆盖叙事游戏的多人需求，无需完整 MMO 基础设施。 |

---

## 需要学习的 TH 设计

### 已借鉴
- 三层消息结构（Session → Floor → MessagePage）
- 五级变量沙箱
- Prompt Pipeline + PromptBlock IR
- 世界书触发逻辑（primary/secondary keys，scan_depth，递归激活）
- 滚动摘要压缩（ST Memory 扩展模式）
- LLM Profile + slot 优先级系统（Director / Verifier / Memory / Narrator）
- Regex 后处理（user_input / ai_output / all）
- ReplaySafety 等级（safe / confirm_on_replay / never_auto_replay / uncertain）
- PromptSnapshot（命中词条 ID + preset_hits + verifier 结果）
- 结构化 Memory 整合（JSON facts + fact_key upsert/deprecate）
- **Memory Edge 关系图**（updates/contradicts/supports/resolves，✅ 3-A 完成）
- **LLM 模型发现 + 连通性测试**（DiscoverModels / TestConnection，✅ 3-B 完成）
- **Worldbook 互斥分组**（Group + GroupWeight + applyGroupCap，✅ 3-C 完成）

### 待学习 / 待实现
- **Worldbook 变量门控**：`var:key=value` 条件激活语法，引擎层强制阶段门控（3-D）
- **Memory 分阶段标签**：`stage_tags` 过滤注入，多幕叙事 token 精确控制（3-E）
- **边界归档**：`POST /sessions/:id/archive`，结局时固化关键记忆（3-F）
- **Session branch_id**：TH M13 分支治理完整实现；WE Phase 3-G（改动较大，排后）
- **AES-256-GCM 密钥加密**：TH M18 LLM Profile Vault；WE Phase 4-A

---

## 文档规范

所有 `docs/` 文档遵循以下约定：

- **database.md**：字段含义、枚举约束、索引约定
- **preset-tool-plan.md**：Preset Tool 实现计划 + ST 预设兼容分析 + WE 游戏分层设计
- **karpathy-llmwiki-analysis.md**：LLM Wiki 模式对照分析 + 边界归档设计方向
- **prompt-block-design.md**：PromptBlock IR 设计 + MVM 渲染展望 + 存档设计
- **architecture-and-roadmap.md**：整体架构与路线图
- **st-comparison.md**：ST/TH 功能对照（最新版：2026-04-06）
- **logs/**：实验性功能日志（记录做了什么、可能回滚的点、与 TH 的对比）
