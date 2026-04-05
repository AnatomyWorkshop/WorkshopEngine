# backend-v2 中期实现计划

> 原则：深度解耦、One-Shot LLM、每个节点对外有明确接口
> 状态更新：2026-04-05

---

## 当前状态（已完成）

### 核心基础设施
- `internal/core/db` — PostgreSQL + GORM，AutoMigrate，全部模型已定义
- `internal/core/llm` — OpenAI 兼容客户端，Chat + ChatStream（SSE），精确 token 计数（`stream_options.include_usage`）
- `internal/platform/provider` — LLM Profile 动态解析，slot 优先级系统（session > global，精确 slot > 通配 `*`）

### 引擎层
- `internal/engine/prompt_ir` — Prompt 中间表示（PromptBlock IR）
- `internal/engine/pipeline` — 流水线节点（SystemPrompt / Worldbook / Memory / History / PresetEntry）
- `internal/engine/variable` — 五级变量沙箱（Global → Chat → Floor → Branch → Page）
- `internal/engine/parser` — AI 响应结构化解析（三层回退：XML → 编号列表 → fallback）
- `internal/engine/memory` — 记忆存取 + 滚动摘要压缩（复刻 ST/TH 模式）
- `internal/engine/session` — Floor/Page 状态机（StartTurn / CommitTurn / RegenTurn / FailTurn）
- `internal/engine/processor` — Regex 后处理（user_input / ai_output / all）
- `internal/engine/scheduled` — 定时触发规则（variable_threshold 模式）
- `internal/engine/tools` — 工具注册表 + 内置工具（12 个资源工具 + get/set_variable + search_memory/material）
- `internal/engine/tools/http_tool.go` — Preset Tool HTTP 回调执行器

### API 层
- `internal/engine/api` — 游玩接口（PlayTurn / StreamTurn / ForkSession / PromptPreview）
- `internal/creation/api` — 创作接口（角色卡 / 模板 / 世界书 / PresetEntry / LLMProfile / Regex / Material / PresetTool）

### 已实现的 API 路由

**游玩层 `/api/v2/play`**
```
POST   /sessions                        创建会话
POST   /sessions/:id/turn               一回合（PlayTurn）
POST   /sessions/:id/regen              重新生成
GET    /sessions/:id/stream             SSE 流式输出
GET    /sessions/:id/state              会话状态
DELETE /sessions/:id                    删除会话
GET    /sessions/:id/variables          变量快照
GET    /sessions                        列出会话
GET    /sessions/:id/floors             楼层列表
GET    /sessions/:id/floors/:fid/pages  Swipe 页列表
GET    /sessions/:id/memories           记忆列表
POST   /sessions/:id/memories           手动创建记忆
DELETE /sessions/:id/memories/:mid      删除记忆
POST   /sessions/:id/memories/consolidate  立即触发记忆整合
POST   /sessions/:id/fork               分叉会话
GET    /sessions/:id/prompt-preview     Prompt dry-run
GET    /sessions/:id/tool-executions    工具执行记录
```

**创作层 `/api/v2/create`**
```
POST/GET/DELETE /cards                  角色卡 CRUD
POST            /cards/import           导入角色卡 PNG
GET/POST/DELETE /templates/:id/lorebook 世界书 CRUD
GET/POST/PATCH/DELETE /templates        游戏模板 CRUD
GET/POST/PATCH/DELETE /templates/:id/preset-entries  PresetEntry CRUD
PUT             /templates/:id/preset-entries/reorder 批量调整顺序
GET/POST/PATCH/DELETE /templates/:id/tools  Preset Tool CRUD
GET/POST/PATCH/DELETE /llm-profiles     LLM Profile CRUD
POST            /llm-profiles/:id/activate  绑定 Profile 到 slot
```

---

## 中期工作（按优先级）

### Phase 2 — 已完成
- [x] **Preset Tool（用户自定义工具）**：HTTP 回调工具，`preset:*` / `preset:<name>` 动态加载
- [x] **ToolExecutionRecord 持久化**：异步写入，`GET /sessions/:id/tool-executions` 查询
- [x] **精确 token 计数**：SSE `stream_options.include_usage`，三通道返回

### Phase 2 — 进行中
- [x] **Director 槽**：廉价模型预分析，结果注入主 LLM 上下文首位；`director_prompt` 在 `GameTemplate.Config` 配置；绑定 `slot="director"` 的 LLMProfile 即启用

### Phase 2 — 待完成
- [ ] **Verifier 槽**：主生成后校验，失败时触发重试或降级；绑定 `slot="verifier"` 启用
- [ ] **ST 预设导入**：`POST /create/templates/:id/preset/import-st`，解析 ST 预设 JSON → 批量写入 PresetEntry（丢弃 marker 条目，用 prompt_order 下标 × 10 作为 injection_order）
- [ ] **边界归档 API**：`POST /sessions/:id/archive`，结局/分享时生成结构化 Markdown 摘要，写入高重要性 Memory

### Phase 3 — 计划中
- [ ] **MCP 协议接入**：stdio + HTTP transport，工具发现，McpConnectionManager；比 Preset Tool 工作量大，但覆盖现有 MCP 生态工具
- [ ] **创作引擎 MCP**：creation 模块的 AI 协作层（import_st_preset / edit_preset_entry / package_game / unpack_game）
- [ ] **跨会话知识库**：game 级 Memory 表，归档内容可提升为全局世界书条目
- [ ] **Memory Lint**：廉价模型定期扫描矛盾/过时条目，标记 `deprecated=true`
- [ ] **多 Provider 原生适配**：Anthropic / Google / xAI 原生 SDK（当前只有 OpenAI 兼容接口）

---

## 需要学习的 TH 设计

### 已借鉴
- 三层消息结构（Session → Floor → MessagePage）
- 五级变量沙箱
- Prompt Pipeline + PromptBlock IR
- 世界书触发逻辑（primary/secondary keys，scan_depth）
- 滚动摘要压缩（ST Memory 扩展模式）
- LLM Profile + slot 优先级系统
- Regex 后处理（user_input / ai_output / all）
- ReplaySafety 等级（safe / confirm_on_replay / never_auto_replay / uncertain）

### 待学习
- **Background Job Runtime**（`runtime_scope_state` + `runtime_job`）：TH 用统一作业表管理记忆整合、chat transfer 等异步任务，有 scope lease、revision、dedupe_key、进度追踪。WE 目前用 goroutine 直接异步，适合轻量场景，但长期需要类似机制保证可靠性。
- **prompt_snapshot 表**：TH 冻结每个 floor 实际使用的 Prompt 资源版本（preset_id、worldbook_id、regex_profile_id 的快照），用于审计和 replay。WE 目前没有这层快照，调试时无法还原某回合的完整 prompt。
- **memory_edge 表**：TH 记录记忆条目之间的关系（supports / contradicts / updates），支持 Lint 时找矛盾。WE 的 Memory 是平铺的，没有关系图。
- **character_version 表**：TH 对角色卡做版本管理，session 可以 pin 到特定版本。WE 目前角色卡没有版本控制。
- **llm_instance_config 表**：TH 把 Profile 绑定（llm_profile_binding）和实例配置（enabled、params_json）分离。WE 的 LLMProfileBinding.Params 合并了两者，长期可能需要拆分。

---

## 文档规范

所有 `docs/` 文档遵循以下约定：

- **database.md**：字段含义、枚举约束、索引约定（参考 TH database.md 格式）
- **preset-tool-plan.md**：Preset Tool 实现计划 + ST 预设兼容分析 + WE 游戏分层设计
- **karpathy-llmwiki-analysis.md**：LLM Wiki 模式对照分析 + 边界归档设计方向
- **prompt-block-design.md**：PromptBlock IR 设计 + MVM 渲染展望 + 存档设计
- **architecture-and-roadmap.md**：整体架构与路线图
- **st-comparison.md**：ST/TH 功能对照
