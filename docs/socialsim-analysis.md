# SocialSim × WE/TH 集成分析

> 目标仓库：`D:\ai-game-workshop\plagiarism-and-secret\SocialSim`（ESSEX-CV9/SocialSim）
> 分析日期：2026-04-04

---

## 目录

1. [SocialSim 是什么](#socialsim-是什么)
2. [SocialSim 架构速览](#socialsim-架构速览)
3. [与 WE（backend-v2）的重叠分析](#与-webackend-v2的重叠分析)
4. [与 TH（TavernHeadless）的重叠分析](#与-thtavernheadless的重叠分析)
5. [可降低工作量的具体模块](#可降低工作量的具体模块)
6. [不可被替代的核心创新](#不可被替代的核心创新)
7. [集成路线建议](#集成路线建议)
8. [本地运行说明（测试用）](#本地运行说明测试用)

---

## SocialSim 是什么

SocialSim 是一个 **AI 驱动的虚拟 Twitter/X 模拟器**，设计为 SillyTavern（ST）插件。
它在角色扮演世界中注入一个活跃的社交媒体层：NPC 账号会主动发帖、转帖、回复，形成动态叙事背景。

**核心设计哲学：**
- LLM 只处理需要"智能"的部分（角色发言、内容生成）；约 80% 的行为由代码规则决定
- 玩家体验一个完整的 X 风格 UI，而非一个 AI 工具
- 三档 LLM 消耗模式（Economy 1-3 次 / Standard 4-8 次 / Quality 8-15+ 次）

**技术栈：** Vue 3 + TypeScript 前端，Fastify 5 + TypeScript 后端，SQLite (WAL)，Zod monorepo 契约。

---

## SocialSim 架构速览

```
SocialSim/
├── client/           # Vue 3 前端（8 页面：Timeline/Explore/Profile/Settings...）
├── contracts/        # Zod Schema 契约包（跨 monorepo 共享）
├── server/src/
│   ├── modules/
│   │   ├── agent-system/         # LLM 编排（18 工具，3 Tier 分发策略）
│   │   ├── account-system/       # NPC 账号模板 + Branch 实例
│   │   ├── social-content/       # 帖子 / 回复 / 互动 / 通知
│   │   ├── state-management/     # 事件溯源 + 物化视图 + 快照
│   │   ├── content-scheduler/    # 压力驱动的内容生成调度器
│   │   ├── trend-system/         # 趋势话题追踪
│   │   ├── materials/            # 预生成内容素材库（FTS5 检索）
│   │   ├── media-proxy/          # 图片代理（Pinterest / Pixiv / DiceBear）
│   │   └── world-config/         # Worldpack ZIP 导入导出
│   └── infra/
│       ├── database/             # SQLite WAL，10 条迁移
│       └── event-bus/            # 进程内事件总线 → WebSocket 推送
```

**核心创新：压力调度器**
不是基于时间，而是基于事件累积"压力"（用户发言 +30、点赞 +20、新帖 +40…），
达到阈值时触发一轮 NPC 内容生成。好处：高活跃时密集响应，空闲时不浪费算力。

**三 Tier NPC 体系：**
- Tier 1（5–20 账号）：主要角色，每帖独立 LLM 调用，完整 persona prompt
- Tier 2（20–100 账号）：机构/媒体，批量生成（单次 LLM 处理 2-5 个账号）
- Tier 3（无上限）：背景 NPC，优先复用素材库（80%），不足时才调用 LLM 批量生成

---

## 与 WE（backend-v2）的重叠分析

> WE 是 Go 语言服务器；SocialSim 是 TypeScript/Node.js 服务器。
> **直接代码复用不可行**，但架构模式的对齐价值很高。

### 可对齐的模式

| SocialSim 当前实现 | WE 对应组件 | 价值 |
|---|---|---|
| `agent-system/adapters/openai-adapter.ts` | `core/llm/client.go` | 接口语义完全一致（Chat/Stream/Options/Usage），TypeScript 重实现可参考 WE 的指数退避/nil 区分 |
| `agent-system/agent.service.ts` 工具循环（最多 5 轮） | `engine/api/game_loop.go` Agentic Loop | 两者逻辑一致（append assistant 消息 → 执行工具 → 追加 tool 结果） |
| `agent-system/default-tools.ts` 工具注册（18 个） | `engine/tools/registry.go` + `builtins.go` | WE 的 `ReplaySafety` 分类（safe / confirm_on_replay / never_auto_replay）可直接移植到 SocialSim |
| 手动 context 裁剪（48,000 token 预算） | `core/tokenizer/estimate.go` | WE 的 ASCII/CJK 分类估算（ASCII ÷4，CJK ×⅔）逻辑可直接用于 SocialSim |
| `state-management/state-builder.ts` 物化视图 | `engine/session/manager.go` Floor/Page | Floor = Branch，Page = Swipe；WE 的 CommitTurn/FailTurn 语义与 SocialSim 的事件提交完全对应 |
| `world-config/world-config.service.ts` Worldpack 定义 | `engine/prompt_ir/pipeline.go` GameConfig | WE 的 WorldbookEntry（关键词触发注入）直接对应 SocialSim 的 `keywords.json` 触发逻辑 |

### 不匹配或低价值部分

| 原因 |
|---|
| WE 是 **单 LLM 单玩家**；SocialSim 是**多 Agent 并发**，WE 的 session/floor 是 per-user 设计 |
| WE 没有 Tier 1/2/3 生成策略分发 |
| WE 没有压力调度器、素材库、媒体代理 |
| WE 没有社交媒体 UI（Timeline / Explore / Notifications）|
| 语言不同（Go vs TypeScript），无直接代码复用 |

**对 WE 本身的价值：** 高。SocialSim 的三 Tier 体系、Worldpack 格式、压力调度器都是 WE 的中期扩展方向的优质参考实现。

---

## 与 TH（TavernHeadless）的重叠分析

> SocialSim 本身就是 ST 插件设计的——TH 是 ST 的无头化版本。**生态完全吻合。**

### 高度重叠（可直接替代的模块）

| SocialSim 模块 | TH 对应能力 | 替代程度 |
|---|---|---|
| `state-management/` （事件溯源 + 快照 + Branch） | TH `Session/Floor/Page` + Fork API | ★★★★★ 完整替代。TH 的三层消息结构 (Session→Floor→Page) 语义与 SocialSim 的 (chatGroup→branch→swipe) 完全等价 |
| `agent-system/provider.service.ts`（Per-tier 模型路由） | TH `LLMProfile + Binding`（narrator/director/verifier + slot 优先级） | ★★★★☆ Tier 1/2/3 → 三个 LLM slot；slot 绑定机制完全对应 |
| `agent-system/default-tools.ts`（18 个工具） | TH `ResourceToolProvider`（23 个资源工具） | ★★★★☆ `get_account_profile` → TH `character:read`；`get_world_setting` → TH `worldbook:read`；`get_current_narrative_context` → TH `session:read`（8 个工具直接映射）|
| `world-config/` Worldpack 导入 | `@tavern/sdk` 全资源 CRUD | ★★★☆☆ 账号/世界书/素材管理可用 SDK 替代；ZIP 格式需要适配层 |
| `infra/event-bus/` WebSocket 推送 | TH SSE 流式输出 | ★★★☆☆ 传输机制不同（TH 用 SSE，SocialSim 用 WS），语义可映射 |
| `agent-system/adapters/openai-adapter.ts` | TH 多 Provider 注册表（openai/anthropic/google/xai） | ★★★★★ 直接替代，且 TH 支持更多 Provider |
| ST 事件同步（st_message_created / st_swipe_activated） | TH `@tavern/client-helpers` (`buildTimelineMessages`, `reduceRespondStream`) | ★★★★☆ TH client-helpers 正是处理 ST 消息树的工具 |

### 中度重叠（可降低工作量的部分）

| SocialSim 需求 | TH 能力 | 说明 |
|---|---|---|
| `keywords.json` 关键词触发叙事事件 | TH Worldbook（`regex:` 关键词触发注入） | SocialSim 的关键词用于触发调度压力；TH 的世界书关键词触发 prompt 注入。机制类似但目的不同——可以用 TH worldbook 来替代 keywords.json，让关键词命中时注入背景信息而非手动修改压力 |
| 多语言支持（en/zh/ja） | TH `GameTemplate.Config.memory_label` / `fallback_options` 可按语言配置 | 部分对齐 |
| 对话历史上下文窗口管理 | TH `node_history`（`maxHistoryFloors` 控制历史长度） | SocialSim 的 48K token 上下文管理可借鉴 TH 的 tokenBudget 机制 |

### 不重叠（SocialSim 独有，TH 无对应）

| SocialSim 独有功能 | 原因无法替代 |
|---|---|
| **压力驱动调度器**（PressureScheduler） | TH 是 turn-based（玩家输入触发），SocialSim 是 event-pressure 模型；两者根本调度哲学不同 |
| **三 Tier NPC 生成策略**（Tier 1/2/3 分发） | TH 的 narrator/director/verifier 是串行协作，不是并发多账号生成 |
| **素材库**（Material Library，FTS5 检索） | TH 无此概念；这是 SocialSim 降低 LLM 调用的关键机制 |
| **社交媒体 UI**（Timeline / Explore / Notifications） | TH 无前端；SocialSim 的 Vue 3 UI 是核心游玩层 |
| **媒体代理**（Pinterest / Pixiv / DiceBear） | TH 无图片搜索能力 |
| **趋势话题系统**（Trend System） | TH 无类似系统 |
| **批量并发内容生成**（同一轮生成 N 个账号的帖子） | TH 的 one-shot 模型不支持并发多账号 |

---

## 可降低工作量的具体模块

### 如果迁移到 TH 生态（TypeScript，生态完全匹配）

预计可以 **删除或大幅简化** 以下代码（约减少 35–50% 的后端基础设施代码）：

```
server/src/modules/
├── state-management/          ← 完整删除，用 TH Session/Floor/Page 替代
│   ├── state-management.service.ts  (~250 行)
│   ├── state-builder.ts              (~300 行)
│   └── ports.ts                      (~80 行)
│
├── agent-system/
│   ├── provider.service.ts    ← 删除，用 TH LLMProfile/Binding 替代
│   ├── agent-tooling.service.ts ← 简化（保留 18 工具 Execute 逻辑，删除注册框架）
│   └── adapters/              ← 删除，TH provider 注册表接管
│       └── openai-adapter.ts
│
├── world-config/              ← 简化（保留 ZIP 解析，删除账号/世界书 CRUD）
│   └── world-config.service.ts
│
└── account-system/            ← 简化（账号定义 CRUD 交给 @tavern/sdk）
    └── account.service.ts
```

**保留不变：** `content-scheduler/`、`social-content/`、`materials/`、`trend-system/`、`media-proxy/`、整个 `client/` 前端

### 如果与 WE 概念对齐（Go 语言，无直接复用，但有架构参考价值）

- 参考 WE 的 `tokenizer.Estimate` 改进 SocialSim 的中英文混合 token 估算
- 参考 WE 的 `ReplaySafety` 枚举为 SocialSim 的 18 个工具补充重放安全标注
- 参考 WE 的 `node_worldbook` 递归激活机制改进 keywords.json 的触发逻辑

---

## 不可被替代的核心创新

SocialSim 的这些设计是目前 WE 和 TH **都没有的**，也是它最有价值的部分：

### 1. 压力驱动调度器（核心）
```
用户行为 → 累积压力值 → 达到阈值 → 触发 NPC 生成循环
user_message +30, user_interaction +20, user_new_post +40, manual_trigger +1000
```
这解决了"NPC 何时该发帖"的问题，比定时轮询更自然、更节省 LLM 调用。
**WE/TH 方向**：这个模型值得直接在 WE 中引入，作为"非交互式 NPC 生成触发器"。

### 2. 三层 Token 预算（Economy/Standard/Quality）
同一个游戏功能提供三种算力消耗模式，让不同 API 配额的玩家都能游玩。
WE 目前是固定调用链，无此分级。

### 3. 素材库（Material Library）
预先生成的帖子/回复模板，LLM 使用前先检索素材库（80% 覆盖率），不足时才调用 LLM。
这让 Tier 3（背景 NPC）的运行成本接近于零。

### 4. 社交媒体 UI 层
Twitter-like 界面（Timeline / PostDetail / Explore / Notifications / Profile）
是 SocialSim 游玩体验的核心，WE/TH 均无此前端。

---

## 集成路线建议

### 近期（可直接降低工作量）

**选项 A：SocialSim 对接 WE API**
- SocialSim 在"玩家 → AI 对话"层调用 WE 的 `/api/v2/play/turn`
- WE 负责处理 narrative/options（角色扮演叙事）
- SocialSim 负责并行处理 NPC 社交内容
- 两者通过 Session ID 关联
- **优势**：无需大量重构，两个系统各司其职
- **接入成本**：中（需要 WE session 和 SocialSim branch 的 ID 映射）

**选项 B：SocialSim 仅参考 WE 的 tokenizer**
- 把 `internal/core/tokenizer/estimate.go` 的逻辑移植到 TypeScript
- 改善 SocialSim 的 48K token 预算裁剪精度
- **接入成本**：低（约 30 行 TypeScript）

### 中期（需要较大重构，但长期价值高）

**选项 C：SocialSim 迁移到 TH 生态**
- 用 TH Session/Floor/Fork 替换 state-management 模块
- 用 TH LLMProfile/Binding 替换 provider.service.ts 的 Tier 路由
- 用 @tavern/sdk 替换 world-config 的账号/世界书 CRUD
- 保留压力调度器、素材库、媒体代理、社交媒体 UI
- **预计工作量减少**：后端 35–50%，但迁移成本本身需要 2–4 周
- **接入成本**：高（需要深度重构 3 个核心模块）

### 暂不建议

- 把 WE 引擎替换 SocialSim 后端（语言不同，架构目标不同）
- 把 TH 的 turn-based 模型直接应用于 SocialSim（调度哲学根本不同）

---

## 本地运行说明（测试用）

```bash
# 1. 进入 SocialSim 目录
cd "D:\ai-game-workshop\plagiarism-and-secret\SocialSim"

# 2. 安装依赖（pnpm monorepo）
npm install   # 或 pnpm install

# 3. 配置环境变量（server/.env）
cat > server/.env << 'EOF'
PORT=4455
# LLM Provider（OpenAI 兼容）
OPENAI_API_KEY=your-key-here
OPENAI_BASE_URL=https://open.bigmodel.cn/api/paas/v4   # 智谱 GLM
# 可选：图片搜索
# PINTEREST_COOKIES=...
# PIXIV_REFRESH_TOKEN=...
EOF

# 4. 构建 contracts 包（monorepo 共享契约）
npm run build --workspace=contracts

# 5. 启动后端（开发模式）
npm run dev --workspace=server
# → 监听 http://localhost:4455

# 6. 启动前端（另开终端）
npm run dev --workspace=client
# → 监听 http://localhost:5173（或 Vite 自动分配端口）

# 7. 访问 UI
# http://localhost:5173
# Settings → Agent → 配置 LLM Provider（base_url + api_key + model）
# Settings → WorldConfig → 导入 Worldpack ZIP（或使用空世界）
# Settings → Scheduler → 调整压力阈值（默认 100）

# 8. 创建测试场景
# POST http://localhost:4455/api/seed   ← 生成测试数据
# POST http://localhost:4455/api/scheduler/trigger ← 手动触发一轮 NPC 生成
```

**最小测试流程：**
1. 配置 LLM Provider（GLM-4-Flash 可用，Economy 模式 1-3 次调用）
2. 导入或创建一个 Worldpack（至少含 2 个 Tier 1 账号）
3. 发一条帖子，观察 NPC 的自动响应
4. 查看 `/api/llm-calls` 端点确认 LLM 调用日志
5. 对比三档模式（Economy → Standard → Quality）的调用次数差异

**与 WE 联动测试（选项 A）：**
1. 启动 WE backend-v2（`go run cmd/server/main.go`）
2. 在 SocialSim 设置中配置 WE 的 session endpoint
3. 玩家消息同时触发 WE narrative 生成 + SocialSim NPC 响应
4. 比较单独使用 WE 和联动 SocialSim 的叙事深度差异
