# WE 复杂逻辑 + 多玩家联动 + TH 作者视角

> 日期：2026-04-06
> 来源：Doubao 对话主题整理（豆包分析文件 + TH 作者提问整理）

---

## Q1：如果 LLM 模型和前端请求速率支持复杂逻辑，WE 也可以做到吗？

### 答

**可以，而且复杂度限制主要来自设计，不来自 WE 引擎本身。**

WE 现在的并行调用链：

```
PlayTurn
  ├── Worldbook 扫描（本地，纯 CPU）
  ├── Pipeline 组装（本地）
  ├── 主 LLM（Narrator 槽）         ← 一次 LLM 调用
  ├── Director 分析（可选，goroutine） ← 并行轻量 LLM
  └── Verifier 校验（可选，goroutine） ← 并行轻量 LLM
```

**如果 LLM 速度足够快，可以扩展到：**

| 复杂逻辑 | 技术路径 | WE 现有基础 |
|---|---|---|
| 多 Agent 协同叙事（主持人 + 多角色分别生成独白）| 并行多个 LLM slot | Director/Verifier goroutine 已有范例 |
| 生成后自动裁判/评分 | Verifier 扩展 | Verifier 已实现 |
| 世界状态自洽检查（变量约束验证）| 变量 Sandbox + 裁判 LLM | Variable Sandbox 已实现 |
| 分支预生成（Preflight Rendering） | 主回合后异步生成 N 条分支 | goroutine + 变量缓存 |
| 动态 Worldbook 条目（LLM 决定激活哪些）| Director 槽输出激活列表 | 理论可行，需扩展 node_worldbook |
| 记忆 Lint（自动检测矛盾条目）| 廉价模型定期扫描 Memory | Memory Edge `contradicts` 已建模 |

**瓶颈在哪里：**

1. **token 预算**：每回合 context 窗口是硬上限。复杂逻辑 = 更多 tokens = 更贵 + 更慢。WE 的 `TokenBudget` 机制限制注入量，是正确的工程决策。
2. **串行 vs. 并行**：WE 已经用 goroutine 把 Director/Verifier 并行化了。进一步复杂化需要更细粒度的 DAG 调度（类似 LangGraph 的 Node 图），但这是将来的事。
3. **前端请求速率**：SSE 流式推送已经解决了延迟感知问题。"感觉慢"通常是 LLM 本身的首 token 延迟，不是 WE 网络层。

**结论**：WE 的架构不是瓶颈。想做多复杂，做就行——goroutine 加新 slot 即可。真正的约束是 API 成本和设计合理性，不是代码。

---

## Q2：多玩家联动 — WE 有相关设想吗？通过什么技术实现？

### TH 的立场（先看对比）

TH 没有任何多玩家游戏逻辑。它有：
- **WebSocket Event Bus**（27+ 事件类型）：`floor.stateChanged`、`generation.chunk`、`memory.created` 等
- **session_id 过滤订阅**：一个 session 的事件只推给订阅该 session 的客户端
- **多账户隔离**（ACCOUNT_MODE=multi）：每个账号的数据完全隔离

但这些全部是"一个管理员监控一个 session"的架构，而不是"多个玩家共享一个 session"。TH **没有** 房间/大厅、presence 同步、并发输入仲裁、或协作叙事逻辑。

**TH 的 WS 基础设施是多玩家的地基，但 TH 没有盖楼。**

---

### WE 的四种多玩家模型

#### 模型 A：旁观者模式（最容易实现）

```
玩家甲 →── PlayTurn ──→ session
                         │
                         ├── SSE 推送给甲（已有）
                         └── SSE 广播给订阅该 session 的所有旁观者
```

**改动量**：极小。WE 的 SSE 现在是"一请求一连接"。只需允许多个客户端订阅同一 `session_id` 的事件流（类似 TH 的 WsBridge sessionId 过滤）。

**场景**：
- 作者直播游戏过程，观众实时看到 LLM 输出
- 创作者给玩家演示新游戏

**WE 需要加的**：Session Subscriber 注册表（`sync.Map[sessionID][]chan Event]`）+ `/sessions/:id/watch` SSE 端点。约 50 行。

---

#### 模型 B：轮流制协作（真正的多玩家）

```
玩家甲、乙、丙共享同一个 session。
每回合只有一个"主控玩家"可以发言，其他人旁观。
主控权在回合结束后自动转移（或由 LLM 决定下一个主控是谁）。
```

**技术方案**：
- `Session.Variables` 新增 `active_player_id` 和 `player_roster: [{id, name}]`
- `PlayTurn` 校验 `req.PlayerID == active_player_id`，否则 403
- LLM 的 SystemPrompt 注入玩家花名册和当前主控玩家
- 回合结束后，变量更新 `active_player_id`（LLM 输出或轮询算法）

**场景**：
- 多人 TRPG（桌游）：GM 是 LLM，每个玩家轮流行动
- 剧本杀：每人有专属角色，按场景顺序发言

**WE 需要加的**：
1. `GameSession` 新增 `player_roster` JSONB 字段
2. `PlayTurn` 增加 `player_id` 字段 + 主控校验
3. `POST /sessions/:id/join` — 玩家加入 session 并注册到花名册
4. 前端展示花名册 + 当前主控指示器

---

#### 模型 C：异步论坛协作（最契合 WE 现有架构）

**这是最"WE 原生"的多玩家模式，几乎不需要改后端。**

```
玩家甲在论坛发帖："我主张选择 A 路线"
玩家乙在论坛发帖："我认为应该先收集情报"
玩家丙在论坛发帖："支持甲，另外提醒角色目前体力不足"

─── 创作者/GM 或定时任务 ───────────────────────────────
每隔 N 分钟，把论坛讨论摘要写入 session Memory
  POST /sessions/:id/memories  {content: "玩家多数支持 A 路线，同时注意体力状态"}

─── 下一回合 ──────────────────────────────────────────
PlayTurn，LLM 带着这条集体意志 Memory 生成剧情推进
```

**这已经能用了。** 唯一需要的是 GM 或自动化脚本把论坛讨论摘要注入 Memory。

**场景**：
- 异步多人小说共创（每天一更，玩家讨论方向，GM 汇总后推进）
- 众包世界构建（不同玩家贡献不同世界书条目）

---

#### 模型 D：观众投票（最有表演性）

**"Twitch Plays Pokemon"的 AI 叙事版本。**

```
主播在 GW 里直播游玩一个 AI 游戏。
每个回合结束后，LLM 输出 3 个选项（Preflight Rendering 已设想）。
观众在弹幕/聊天室投票选哪个选项。
弹幕机器人统计投票 → 自动发送 PlayTurn。
```

**WE 需要加的**：
- 选项投票 API：`POST /sessions/:id/vote {option_index: 1}` + 票数统计变量
- 定时触发：投票窗口到期后，自动以得票最高选项调用 PlayTurn
- `GameTemplate.Config.vote_window_seconds` 控制投票窗口

**场景**：
- 流媒体表演性游玩（观众决策，内容意外性极高）
- AI 游戏的公开演示

---

### 四种模型对比

| 模型 | 实时性 | 后端改动 | 使用场景 |
|---|---|---|---|
| A 旁观者 | 实时 | 极小（~50行） | 直播、演示 |
| B 轮流制 | 实时 | 中等（~200行） | TRPG、剧本杀 |
| C 异步论坛 | 非实时 | 几乎零 | 共创小说、众包世界书 |
| D 观众投票 | 近实时 | 小（~100行） | 流媒体表演 |

**WE 的推荐路径**：先做 C（已有能力，零成本验证场景），再做 A（技术最简单，体验提升显著），最后做 B（正式多人游戏需求确认后再投入）。

---

## Q3：TH 作者视角 — 挑有价值的问题回应

### Q3-1：TH 为什么选择 Node.js/TypeScript，而不是 Go？

这是架构性的选择，各有道理：

**TH 选 TypeScript 的合理性：**
- LLM SDK 生态以 Python/TypeScript 为主；`@anthropic-ai/sdk`、`openai` npm 包比 Go 的替代品成熟得多
- MCP 协议的官方 SDK 是 TypeScript-first（Anthropic 自家的）
- Fastify + Drizzle ORM + Emittery 的异步范式和 LLM streaming 天然契合
- 类型体操（`CoreEventMap` 的 typed event bus）在 TypeScript 里更自然

**WE 选 Go 的合理性：**
- goroutine 比 Node.js 的 async/await 更适合"每回合多 LLM 并行"的场景
- 单二进制部署，没有 `node_modules` 地狱
- 内存占用更低，适���手机端/边缘部署
- GORM + PostgreSQL 在 Go 里是成熟组合

**两者的本质差别**：TH 是"AI 工具箱平台"（MCP + 工具调用是核心），Go 的优势在这里不大。WE 是"高并发游戏服务器"，goroutine 更合适。

---

### Q3-2：TH 的 MCP 集成是正确的赌注吗？

TH 用 `ResourceToolProvider` 实现了 23 个工具，完整接入 MCP 协议。这是一个相当激进的决定，因为 MCP 在 TH 开始实现时还是实验性的。

**作者的判断是正确的：**

MCP（Model Context Protocol）的核心价值是：**让 LLM 能够"插件化"访问任意外部数据和能力**，而不需要为每个工具写一套专用代码。

从设计角度看：
- 没有 MCP：TH 自己实现工具 → 每个工具是私有接口 → 用户无法带入自己的工具
- 有 MCP：TH 是 MCP 宿主 → 用户可以接入任意兼容 MCP 的工具服务器 → TH 变成一个开放平台

WE 的 `tools/` 目录（`builtins.go`、`http_tool.go`、`resource_provider.go`）走的是"内置工具 + HTTP 调用"路径，功能上类似，但不如 MCP 的标准化接口开放。

**WE 要不要做 MCP？** 实现-plan.md 里标记为"候选/推迟"是正确的——MCP 的价值在复杂 agent 场景，WE 的主场景是游戏叙事，内置工具已经够用。

---

### Q3-3：TH 的事件总线粒度是否过细？

TH 定义了 27+ 事件类型，每个细小状态变化都是一个事件（`floor.stateChanged`、`commit.retry`、`runtime.job_leased` 等）。

**优点**：
- 调试极其方便：任何状态问题都可以通过订阅对应事件观察
- 扩展性好：新功能只需新增事件类型，不修改已有流
- 前端可以做非常细粒度的 UI 更新（"正在重试…"、"记忆已整合"）

**缺点**：
- 27 个事件类型需要写 27 个 handler 才能完整处理
- 测试负担重（TH 确实写了 14 个 WS 单元测试）
- 文档维护成本高

**WE 的选择**：用 SSE 流式推送（`token`、`done`、`error` 三类事件），简洁但观测性弱。

**判断**：对于 WE 的游戏叙事场景，3 类 SSE 事件足够了。对于 TH 这种"agent 运行时平台"，27+ 事件类型是合理的——不同类型的客户端（前端、监控、调试工具）各自订阅自己关心的事件。

---

### Q3-4：LLM Profile Vault（AES-256-GCM 加密存储）值得做吗？

TH M18 实现了 API Key 的加密存储，用 `AES-256-GCM + PBKDF2 密钥派生`。

**值不值得：取决于部署模式。**

| 场景 | 是否需要加密存储 |
|---|---|
| 本地单用户部署 | 不需要（DB 文件本身在本地，加密意义有限） |
| 私有化多用户部署（公司内部） | 需要（防止 DB 泄露暴露所有用户的 API Key） |
| 公网 SaaS 服务 | 强烈需要 |

WE 目前把 API Key 明文存在 `LLMProfile.APIKey` 列，`toProfileResp` 里做了掩码处理（只返回 `sk-...***`），这对本地/开发场景够用。上 SaaS 之前需要参考 TH M18 做加密。

---

### Q3-5：Character Versioning（角色版本管理）是否必要？

TH 有完整的角色版本历史。WE 明确不做这个（见 implementation-plan.md"明确不做"章节）。

**WE 的理由是正确的：**
- WE 的"角色"是 `CharacterCard`（PNG 导入后的静态数据）+ `GameTemplate`（游戏逻辑）
- 版本变更追踪靠 git（创作者在 CW 里修改，靠 game-package.json 的导出/导入做版本快照）
- 在运行时为每次角色编辑维护版本历史会让数据模型复杂很多，而收益仅限于"回滚到上个版本"

**什么时候需要：** 当 WE 变成多创作者协同创作平台，且版本冲突成为真实问题时，再做。现在是单人创作工具，不需要。

---

## 小结：WE vs TH 的设计哲学差异

| 维度 | TH | WE |
|---|---|---|
| **核心定位** | 可组合 AI agent 运行时 | 手机端轻量游戏引擎 |
| **扩展机制** | MCP 协议（开放标准） | 内置工具 + HTTP tool |
| **状态观测** | 27+ 事件类型 + WS 总线 | SSE 3 类事件（简洁） |
| **API Key 安全** | AES-256-GCM 加密存储 | 明文 + 掩码返回 |
| **多用户** | 多账户隔离（完整） | session_id 隔离（轻量） |
| **多玩家** | 无（技术基础有但未建） | 无（4 种模型已设想） |
| **语言生态** | TypeScript（AI SDK 生态更好） | Go（并发性能更好） |

TH 是在为"AI 能力平台"打地基，WE 是在为"游戏体验产品"打地基。两者的技术选择都和各自的方向一致，比较优劣没有意义。

---

## 补充：TH 明确做不到的 7 件事（豆包分析整理）

这是 TH 架构的硬边界，对 WE 选方向有参考价值：

| 能力 | TH 做不到的原因 | WE 的情况 |
|---|---|---|
| 跨用户实时多人联机 | 无房间/分布式状态/实时同步 | 同样无，但 4 种模型已设想 |
| 高频 Tick / 实时游戏循环 | 事件响应式，不支持每秒更新 | 同样无，ScheduledTurns 是轮数驱动 |
| 全局唯一道具/经济/全服状态 | 无全局 ID/事务/防作弊 | 同样无，session 级隔离 |
| 强确定性竞技逻辑（PVP/战斗公式）| LLM 不可控，无法保证计算一致性 | 同样不适合；变量+LLM只能近似 |
| 地图网格/A*寻路/空间计算 | 只有变量，无空间数据结构 | 同样无；VN 场景不需要 |
| 防作弊/GM后台/操作审计 | 无账户体系/日志/权限 | 初步有 account_id 隔离，审计无 |
| 云存档/跨设备同步/成就系统 | 无平台化能力 | 无，GW 只做会话级存档 |

**结论**：TH 和 WE 的硬边界几乎完全重叠——两者都是为叙事型 AI 游戏设计的，不是通用游戏引擎。不要试图用它们做 MMORPG。

---

## 补充：TH 作者对 WE 的 14 个关键架构问题

豆包分析模拟了 TH 作者（熟悉 TH 设计的人）会问 WE 什么问题。挑最有价值的几个回答：

### Q1：PromptBlock 拆分后，compat_strict 严格兼容酒馆还能 1:1 吗？

**WE 的立场**：基本兼容，但不是目标。
WE 的 `compat_strict` 模式复刻了酒馆消息注入顺序（System → Worldbook → History → User），但 PromptBlock 的 Priority 数字化排序给了比酒馆固定顺序更大的自由度。1:1 不是设计约束，而是起点。如果某个酒馆功能在 WE 里行为略有不同，优先保证 WE 的行为合理，而不是优先兼容酒馆。

### Q2：预飞（Preflight）预测用户输入还是 LLM 输出？预测错误如何回滚？

**预测的是用户选项，不是 LLM 输出。**

两种模式：
- **Lazy**：预测"用户最可能选什么"（生成 3 个选项文本），主回合 LLM 仍需等待
- **Eager**：对每个选项完整跑一次 LLM，缓存全部输出

回滚不是问题：Lazy 模式下预测的只是选项文本，不影响任何状态，用户自由输入会覆盖预测。Eager 模式下预测结果存在 Session 变量，用户点击选项时直接取缓存而不跑新 LLM，只有缓存 miss 时（用户自由输入）才回落到普通等待。

### Q3：提前渲染会不会破坏 MVU 单一数据源？

**不破坏，因为预测是只读的。**

WE 的 MVU 单一数据源是 Session 变量 + Floor 历史。预飞预测的结果写入 `predicted_choices` 变量，前端读取展示，但这个变量不参与 LLM context 构建（不注入 Prompt）。用户做出真实选择后，真实的 PlayTurn 写入 Floor，才是"数据源更新"。预测结果被新的 Floor 覆盖后自然作废。

### Q4：结构化输出是扩展 Message 还是新开 ContentLayer？

**WE 已有答案：扩展 VNDirective，不新开 ContentLayer。**

VNDirective（`[scene:...]`、`[emotion:...]` 等）是在 LLM 文本输出里内嵌的结构化指令，Parser 提取后从主文本中删除，前端按照指令更新视觉元素。这是"扩展 Message"的方案，比新开 ContentLayer 代价低得多，且对不支持 VN 的场景完全无感（Parser 降级时直接忽略未识别指令）。

### Q7：世界书从静态改为玩家可进化，多存档如何隔离？

**这是 WE 真正有独特设计的地方。**

WE 的 WorldbookEntry 是绑定到 `game_id`（游戏模板）的，所有玩家共享同一份世界书基础定义。"玩家可进化"的实现路径是：
1. Session 级变量可以覆盖/扩展世界书内容（现在通过 `resolveMacros` 支持 `{{var}}` 注入已经部分实现）
2. 边界归档 API（3-E，待实现）：在结局时，将本局推导出的新世界书条目写入玩家专属的 Memory 条目（`type=worldbook_evolution`），下次开局时把这些条目重新注入为高优先级世界书词条

这样既保留了创作者的基础世界书，又让每个玩家的存档拥有独属于自己的"进化层"。多存档天然隔离，因为进化结果挂在 session 的 Memory 下。

### Q14：你的引擎是 TH 超集，还是全新平行引擎？

**平行引擎，战略借鉴，不是超集。**

WE 刻意不做 TH 做的事（MCP 协议、角色版本管理、Branch Governance），同时做了 TH 没做的事（VN 叙事指令、边界归档、Preflight Rendering、Memory Edge 关系图、结构化 Memory 整合）。两者的交集是"OpenAI-compatible LLM + 会话管理 + 世界书 + 记忆压缩"这个基础层，在此之上各自向不同方向演化。

---

## 补充：WE 相比 TH 的 4 个架构创新（豆包分析）

豆包分析归纳的 WE 真正区别于 TH 的地方，值得记录：

**1. PromptBlock 细粒度调用**

TH 是整轮调用：每回合把完整 Prompt 一次性发给 LLM。  
WE 通过 Pipeline Node 系统把 Worldbook / Memory / Template / History 拆分为独立 Block，可以单独测试、单独启停。这允许"跳过某个 Block"（比如简短回合不注入完整历史）来节省 token。

**2. 预飞预测 + 提前渲染**

TH 无此设计。WE 设想的 Preflight Rendering 让"像制成游戏一样无等待"成为可能，同时保留自由输入的开放性（见 talk-with-alice 第一篇 Q2）。

**3. 结构化输出 + 游戏外打包降级**

TH 输出是纯文本。WE 的 VNDirective 系统输出结构化场景指令，支持导出为剧本/小说/视频脚本格式。Game Package 导出/导入机制让游戏可以离线打包分发。

**4. 归档边界 + 玩家专属进化世界书**

TH 没有结局固化机制。WE 的边界归档（3-E，待实现）在结局时触发，将本局关键决策和衍生世界书条目写入玩家的持久 Memory，形成每个玩家独属的"故事沉淀层"。这使得世界书不是静态预设，而是每个玩家通过游玩持续构建的活的知识库。

---

> 续篇：WebGal 状态模块设计 vs TH/WE 哲学差异 + WE 的分阶段记忆控制 → 见 2026-04-06-webgal-state-and-memory-staging.md
