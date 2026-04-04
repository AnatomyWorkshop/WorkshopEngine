# WorkshopEngine

一个以 **LLM 对话为核心的无头交互运行时**（Headless AI Interaction Runtime）。

WorkshopEngine 不绑定任何前端框架，也不限定使用场景。它提供一套通用的 **会话管理 → Prompt 编排 → LLM 调用 → 状态持久化** 管道，驱动任何需要"与 AI 来回推进"的应用：互动叙事游戏、对话式工具、角色扮演……都是同一条主链路的不同配置。

---

## 核心能力

### 会话结构（Session / Floor / Page）

```
Session（一次游戏/交互）
  └── Floor（一个回合，提交后不可改）
        └── Page（同一回合的多个生成结果，可 Swipe 切换）
```

- 每个 Floor 对应一条用户输入 + 一条 AI 回复
- 同一 Floor 支持多个 Page（重新生成/Swipe），切换激活 Page 时变量快照自动提升
- **Session Fork**：从任意 FloorSeq 复制全段历史，创建平行时间线

### 变量沙箱（五层）

```
Page → Floor → Branch → Chat → Global
```

AI 回复中的 `<UpdateState>` 写入 Page 层；Floor 提交后提升至 Chat；创作者可在任意层写入全局变量。所有层级在 Prompt 组装前合并展开，用于宏替换 `{{var}}`。

### Prompt Pipeline（可编排节点）

| 节点 | 职责 | 优先级 |
|------|------|--------|
| TemplateNode | System Prompt 模板 + `{{var}}` 宏替换 | 1000（兜底） |
| PresetNode | 条目化 Prompt（injection_order 可排序） | 由 injection_order 直接决定 |
| WorldbookNode | 关键词 / 正则 / 次级逻辑门触发注入 | 10–510 |
| MemoryNode | 时间衰减记忆摘要 | 400 |
| HistoryNode | 近期历史消息 | 0 ~ -N |

优先级数值越小越靠近 System Prompt 顶部。节点可单独开关，便于调试。

### 世界书（Worldbook）— 完整对齐 SillyTavern

| 功能 | 说明 |
|------|------|
| 主关键词触发 | 子串匹配，大小写不敏感 |
| 正则关键词 | `regex:pattern` 前缀，支持 `/pattern/flags` 语法 |
| 全词匹配 | `whole_word: true` |
| 次级关键词 + 逻辑门 | `AND_ANY / AND_ALL / NOT_ANY / NOT_ALL` |
| 扫描深度 | `scan_depth`：只扫最近 N 条消息 |
| 注入位置 | `before_template / after_template`（对应优先级） |
| 常驻词条 | `constant: true`，不论关键词是否命中，永远注入 |
| 递归激活 | 已激活词条内容触发二次扫描，激活更多词条 |

### 记忆系统

- 每轮 AI 回复的 `<Summary>` 字段**异步**写入 Memory 表
- **时间衰减排序**：指数半衰期（可配 `HalfLifeDays` + `MinDecayFactor`），旧记忆自然降权
- **维护 Worker**：按回合数触发整合（廉价模型重新摘要）、自动废弃/清理过期记忆
- Memory 类型：`summary`（叙事摘要）+ `fact`（关键事实）

### LLM 响应解析（三层回退）

```
<Narrative>/<game_response> XML  →  编号列表（1. / ①.）  →  纯文本降级
```

解析结果统一为 `ParsedResponse`：

| 字段 | 来源 |
|------|------|
| `narrative` | `<Narrative>` 标签内容，或纯文本 |
| `options` | `<Options><option>` 或编号列表 |
| `state_patch` | `<UpdateState>` JSON 内联 patch |
| `summary` | `<Summary>` 标签 |
| `vn` | `<game_response>` 内的 VN 指令（见下） |

### 视觉小说指令集（VN Directives）

LLM 在 `<game_response>` 块内输出结构化场景指令：

```
[bg|city_night.jpg]          背景图
[bgm|rain_ambient]           背景音乐
旁白||夜色深沉...             旁白行（无立绘）
夜歌|nightsong_sad.png|……    角色台词行（角色名 + 立绘 + 台词）
[choice|回应她|保持沉默]     选项
```

引擎解析为 `VNDirectives{BG, BGM, Lines[], Options[]}`，前端按帧渲染。

### Agentic Tool Loop

- 引擎内置，最多 5 轮工具调用迭代
- **Tool 重放安全分级**：`safe / confirm_on_replay / never_auto_replay / uncertain`
- 每个工具实现 `ReplaySafety()` 接口，重放时引擎按分级决定是否重新执行

**内置工具**

| 工具 | 功能 |
|------|------|
| `get_variable` | 读取变量沙箱（支持点分路径 `emotion.tension`） |
| `set_variable` | 写入变量沙箱 |
| `search_memory` | 按相关度检索记忆摘要 |
| `create_worldbook_entry` | 在当前游戏创建��词条 |
| `update_worldbook_entry` | 更新词条内容或启用状态 |
| `get_preset_entries` | 查询 Preset Entry 列表 |
| `create_preset_entry` | 动态注入新 Prompt 条目 |
| `update_preset_entry` | 修改 Preset Entry 内容 |
| `create_memory` | 主动写入记忆 |
| `update_memory` | 修改记忆重要度或内容 |
| `search_material` | 按标签/情绪/风格检索素材库（JSONB tag 检索，按使用频次分发） |
| `read_material` | 读取指定素材内容 |

### Regex Profile（LLM 响应后处理）

- 独立 `RegexProfile` 资源（可跨游戏复用）
- `ApplyTo`：`ai_output / user_input / all`
- 支持 `$1` 捕获组替换、`/pattern/i` 大小写不敏感标志
- 规则可单独禁用，多条规则按顺序链式应用

### ScheduledTurn（自动回合）

- 配置在 `GameTemplate.Config.scheduled_turns`，无需写代码
- 模式：`variable_threshold`（变量达到阈值时触发）
- 参数：`condition_var` / `threshold` / `probability` / `cooldown_floors` / `event_pool`（随机事件池）
- 冷却记录写入变量沙箱（键名 `__sched.<id>.last_floor`）

### MaterialLibrary（素材库）

- 每个游戏模板独立的素材内容池
- JSONB `tags` + `world_tags` 双标签体系
- `search_material` 工具：任意标签交集匹配，按 `used_count ASC` 分发（优先使用少的素材）
- 异步 `used_count` 递增，不阻塞响应

---

## 目录结构

```
backend-v2/
├── cmd/
│   ├── server/main.go          HTTP 服务入口（:8080）
│   └── worker/main.go          异步记忆维护 Worker
├── internal/
│   ├── core/
│   │   ├── config/             配置加载（.env / 环境变量）
│   │   ├── db/                 GORM 连接 + AutoMigrate + 所有模型定义
│   │   ├── llm/                OpenAI 兼容 HTTP 客户端（One-Shot + SSE 流式）
│   │   ├── queue/              内存任务队列（异步 Worker）
│   │   └── tokenizer/          Token 粗估（CJK / ASCII 分类计算）
│   ├── engine/
│   │   ├── api/                HTTP 路由（PlayTurn / StreamTurn / Fork / Prompt-Preview）
│   │   ├── pipeline/           Prompt 流水线节点（template/preset/worldbook/memory/history）
│   │   ├── prompt_ir/          PromptBlock 中间表示 + ContextData
│   │   ├── variable/           五层变量沙箱（NewSandbox / Get / Set / Flatten / Commit）
│   │   ├── memory/             记忆存储（时间衰减）+ 异步整合 Worker
│   │   ├── session/            Floor / Page 生命周期管理
│   │   ├── parser/             LLM 响应解析（XML / 列表 / 纯文本三层回退）
│   │   ├── processor/          Regex 后处理（ApplyToAIOutput / ApplyToUserInput）
│   │   ├── scheduled/          ScheduledTurn 触发器（Evaluate / GetFloat / PickInput）
│   │   └── tools/              工具注册表 + 12 个内置工具
│   ├── creation/
│   │   └── api/                创作工具 API（模板 / Preset / 世界书 / Regex / 素材库）
│   ├── platform/               Provider 注册表（LLM Slot 解析）
│   ├── user/                   账户鉴权中间件
│   └── integration/            集成测试（build tag: integration）
├── .test/
│   ├── .env                    LLM API 配置（不提交）
│   ├── run.sh                  一键运行全部测试
│   └── REPORT.md               测试报告（含 bug 修复记录）
└── docs/
    ├── st-comparison.md        WorkshopEngine vs TavernHeadless vs SillyTavern 全景对比
    ├── engine-audit.md         引擎审计 + 硬编码清单 + 路线图
    └── implementation-plan.md  实现计划
```

---

## 快速启动

**依赖：** Go 1.22+，PostgreSQL 14+

```bash
# 最少环境变量
export LLM_BASE_URL=https://api.deepseek.com   # 任何 OpenAI 兼容接口
export LLM_API_KEY=your_key
export LLM_MODEL=deepseek-chat
export DATABASE_URL="host=localhost user=postgres password=postgres dbname=workshop sslmode=disable"

# 启动主服务
go run ./cmd/server        # :8080

# 启动记忆维护 Worker（可选，独立进程）
go run ./cmd/worker
```

表结构由 GORM AutoMigrate 在启动时自动创建，无需手动执行 SQL。

---

## API 路由

```
# 游玩
POST   /api/v2/play/sessions                          创建会话
POST   /api/v2/play/sessions/:id/turn                 推进回合（One-Shot）
GET    /api/v2/play/sessions/:id/stream               推进回合（SSE 流式）
POST   /api/v2/play/sessions/:id/fork                 分叉平行时间线
GET    /api/v2/play/sessions/:id/floors               楼层历史
PATCH  /api/v2/play/floors/:fid/pages/:pid/activate   切换激活 Page（Swipe）
GET    /api/v2/play/sessions/:id/memories             记忆列表
GET    /api/v2/play/sessions/:id/prompt-preview       Prompt 预览（dry-run）

# 创作
POST   /api/v2/create/templates                        游戏模板 CRUD
POST   /api/v2/create/templates/:id/preset-entries     Preset Entry 管理
POST   /api/v2/create/worldbook-entries                世界书词条 CRUD
POST   /api/v2/create/regex-profiles                   Regex Profile 管理
POST   /api/v2/create/templates/:id/materials          素材库 CRUD + 批量导入
GET    /api/v2/game-assets/:slug                       静态素材文件
```

---

## 运行测试

```bash
# 单元测试（无需网络，共 66 个）
go test ./internal/core/tokenizer/... \
        ./internal/engine/parser/... \
        ./internal/engine/processor/... \
        ./internal/engine/scheduled/... \
        ./internal/engine/variable/... \
        ./internal/engine/pipeline/...

# 集成测试（需 LLM API Key，call DeepSeek 验证端到端）
set -a && source .test/.env && set +a
go test -tags integration ./internal/integration/... -v -timeout 120s

# 一键全跑（单元 + 集成）
bash .test/run.sh
```

---

## 待完成（中期路线图）

### Phase 1 收尾
| 任务 | 描述 |
|------|------|
| **精确 Token 计数** | 接入 tiktoken-go 或用 API 返回值校准，替换当前粗估逻辑 |

### Phase 2 — 工具生态
| 任务 | 描述 |
|------|------|
| **MCP 协议接入** | McpConnectionManager（stdio + HTTP transport），让引擎调用任意 MCP 工具服务 |
| **用户自定义工具（Preset Tool）** | 通过 API 注册工具定义，运行时动态加载，游戏创作者可扩展工具集 |
| **工具执行持久化** | ToolExecutionRecord 表，记录每次工具调用的入参/出参/耗时，支持 replay 审计 |

### Phase 3 — 多 LLM 角色槽
| 任务 | 描述 |
|------|------|
| **Director 槽** | generation 前调用廉价模型做上下文分析/剧情控制，输出指令注入主 Prompt |
| **Verifier 槽** | generation 后校验输出格式/安全性，不通过则触发重试 |
| **多 Provider 注册表** | Anthropic / Google / xAI 原生适配（脱离 OpenAI compat 路径） |

### Phase 4 — 平台工程
| 任务 | 描述 |
|------|------|
| **Event Bus** | 引擎事件系统（Floor/Memory/Tool/Variable 事件），供监控和 webhook 消费 |
| **OpenAPI 文档** | swaggo 自动从注释生成 Swagger UI |
| **JWT 鉴权** | 标准 JWT + 账户资源隔离 |
| **对话导入/导出** | ST jsonl 格式互转 |

完整差距分析见 [docs/st-comparison.md](docs/st-comparison.md)。

---

## License

见 [LICENSE](../LICENSE)（BUSL-1.1）。
