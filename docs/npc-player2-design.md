# NPC 与 Player 2 系统设计全景

> 更新日期：2026-04-04
>
> 本文整合 WE、TH、SocialSim 及社区生态的 NPC 主动行为设计思路，
> 包含当前已实现的、暂缓的、以及多真人参与等前瞻设计方向。

---

## 目录

1. [设计空间：两个核心维度](#设计空间两个核心维度)
2. [各实现方案横向对比](#各实现方案横向对比)
3. [WE 当前已实现能力](#we-当前已实现能力)
4. [为节省时间暂缓的部分](#为节省时间暂缓的部分)
5. [NPC 主动行为的触发钩子](#npc-主动行为的触发钩子)
6. [模拟刷新机制（随机事件池）](#模拟刷新机制随机事件池)
7. [多真人参与设计](#多真人参与设计)
8. [未来 MCP 集成方向](#未来-mcp-集成方向)

---

## 设计空间：两个核心维度

所有 NPC/Player 2 系统都在两个轴上做选择：

| 维度 | 极端 A | 极端 B |
|---|---|---|
| **情绪建模** | 隐式涌现（LLM 自主推断，靠 persona + 记忆） | 显式数值（tension/affinity 等变量，精确可控） |
| **主动行为触发** | 被动响应（等玩家操作才生成） | 完全自主（后台定时/事件驱动，玩家不在也运行） |

这两个维度是独立的：可以有"显式情绪 + 被动响应"（WE 当前默认），也可以有"隐式情绪 + 完全自主"（TH 的目标）。

---

## 各实现方案横向对比

| 方案 | 情绪建模 | 主动触发 | 多 NPC | 代码量 | 适用场景 |
|---|---|---|---|---|---|
| **WE（当前）** | 显式变量 + 人格描述约束 | 变量阈值触发（ScheduledTurn MVP） | 单次 LLM 批量输出多角色 | **零后端**（纯配置） | 轻量社交模拟、情绪驱动叙事 |
| **TH** | 隐式涌现（Director 规划 + MemoryEdge 关系图） | Director 每轮主动规划（不等阈值） | narrator/director 双槽分工 | 需写 Binding 配置 | 深度角色扮演、长篇叙事 |
| **SocialSim** | 无显式情绪（行为直接调度） | 压力累积调度器（玩家操作 → 压力 → NPC 生成） | 三 Tier 生成（Tier1 精细/Tier2 批量/Tier3 素材库） | 8000 行专用后端 | 虚拟 Twitter 类社交平台 |
| **酒馆（ST）插件** | 靠 Character Note + Lorebook 手动描述 | 无原生机制（依赖 World Info 触发） | 多角色卡轮换（不同 persona） | 用户自己写 prompt | 通用 RPG 角色扮演 |
| **ST CYOA 插件** | 无 | 无，由玩家选项驱动 | 否 | 低 | 选择型叙事游戏 |

### TH Director 槽位的实际工作方式

```
每轮 PlayTurn：
  [director LLM] → 产出"本回合剧情意图"（低成本模型，短输出）
      ↓ 作为系统消息注入 ↓
  [narrator LLM] → 基于意图生成正式叙事内容
```

优点：意图与生成分离，NPC 行为更有目的性；
代价：每轮多一次 LLM 调用（成本 × 2）。

### SocialSim 压力调度器

```
玩家行为 → 累积压力（user_message +30 / interaction +20 / new_post +40）
       ↓
压力 ≥ 阈值（默认 100）→ 分层延迟队列 → Tier1 精生成 + Tier2 批量 + Tier3 素材库
       ↓
生成完成 → 压力归零
```

WE 的 ScheduledTurn（`variable_threshold` 模式）= SocialSim 压力调度器的通用化抽象。

---

## WE 当前已实现能力

### 情绪/状态建模层

- **变量沙箱**（5 层）：`emotion.tension`、`npc.夜歌.trust` 等任意维度，JSON 存储，`state_patch` 更新
- **人格描述约束**（WorldbookEntry）：Big Five/MBTI 人格写入 NPC 的世界书词条，LLM 读取后自主决定 delta 幅度，而非硬编码公式
- **emotion_config**（GameTemplate.Config）：`npc_delta_limits`（软边界）+ `player_action_hints`（语义提示），前端通过 `GET /api/v2/create/templates/:id` 读取

### 主动行为触发层

- **ScheduledTurn MVP**（`variable_threshold` 模式）：PlayTurn 完成后检查规则，命中时响应携带 `scheduled_input`，前端触发 NPC 自主回合
- 冷却记录写入变量沙箱（`__sched.<id>.last_floor`），无额外表

### 记忆与叙事钩子

- **memory_create 工具**：LLM 在情绪跨阈时主动写入事实记忆，后续回合 NPC "记得"这件事
- **worldbook_create 工具**：LLM 动态新建世界书词条（`resource:*` 工具集），游戏世界随 NPC 行为演化

---

## 为节省时间暂缓的部分

### 暂缓 1：TH Director 槽位

**是什么：** 每轮先用廉价模型产出一条剧情意图，再传给主 narrator 生成。
**为何暂缓：** 每轮 +1 次 LLM 调用；WE 的 PresetEntry 可以让主 LLM 自己兼任"导演"，效果接近但成本更低。
**何时值得做：** 当游戏叙事深度要求高、且有足够 token 预算时（如 RPG 主线剧情）。

### 暂缓 2：ScheduledTurn 定时模式（后台 goroutine）

**是什么：** 服务端独立 goroutine，每 N 秒/分钟触发一次 PlayTurn，玩家不操作时 NPC 也自主发帖。
**为何暂缓：** 需要 SSE 长连接把结果推送给前端；当前 MVP 的前端轮询方案已足够验证场景。
**何时值得做：** 游戏需要"活着的世界"沉浸感（social sim、持续直播型游戏）。

### 暂缓 3：CharacterCard 结构化 NPC 卡

**是什么：** Creation 模块中的独立 `CharacterCard` 实体，包含 persona + stat schema + 默认值；session 启动时自动注入变量沙箱。
**为何暂缓：** WorldbookEntry（persona 文本）+ 会话变量（数值）的组合已满足当前需求；CharacterCard 是便利性而非必要性。
**何时值得做：** 需要跨 session 复用角色（多游戏共用 NPC）或开放 NPC 市场时。

### 暂缓 4：MemoryEdge 关系图

**是什么：** TH 的 Memory V2 特性，将 NPC 与玩家的关系历史建图（"玩家曾让夜歌难堪"这条边）。
**为何暂缓：** 当前变量沙箱中 `emotion.npc.<name>.trust` 可近似表达；关系图的价值在于跨 session 持久化和复杂的关系推理。
**何时值得做：** 长期运营型游戏，角色关系影响多 session 连续剧情。

### 暂缓 5：多 Provider 按 NPC 分配

**是什么：** 不同 NPC 使用不同 LLM（主角用 claude-opus，背景 NPC 用 glm-4-flash）。
**为何暂缓：** 当前单 narrator slot；多 slot 需要 director/verifier 架构支撑。
**何时值得做：** 算力成本是关键约束时。

---

## NPC 主动行为的触发钩子

NPC 主动行为（不等玩家操作就产生内容）的触发钩子可以有以下几类：

### 钩子 1：变量阈值（已实现）

```json
{ "mode": "variable_threshold", "condition_var": "emotion.tension", "threshold": 70 }
```

玩家行为 → 情绪变量变化 → 超过阈值 → NPC 自主回合。

### 钩子 2：时间定时器（待实现，需 SSE）

```json
{ "mode": "time_based", "interval_seconds": 300, "probability": 0.6 }
```

后台 goroutine 每 5 分钟随机触发一次，玩家离线时世界持续演化。

### 钩子 3：模拟刷新 / 随机事件池（见下节）

```json
{ "mode": "variable_threshold", "event_pool": ["事件A", "事件B", "事件C"] }
```

阈值触发时，从事件池随机抽取一条作为 `user_input`，NPC 对随机世界事件作出反应。

### 钩子 4：楼层计数器（零代码可用）

```json
{ "mode": "variable_threshold", "condition_var": "floor_count", "threshold": 5, "cooldown_floors": 5 }
```

把楼层数本身作为触发条件：每 5 回合强制触发一次世界事件（无需情绪系统）。
（注：`floor_count` 需要在 state_patch 中由 LLM 维护，或通过 `session_summary` 工具读取）

### 钩子 5：WebSocket 断连检测（未来）

检测玩家离线事件，触发"NPC 因玩家消失而产生的内容"（等待、思念、继续行动）。
需要 WebSocket + 会话状态管理。

### 钩子 6：MCP 外部事件源（未来，见最后一节）

第三方系统（RSS、Webhook、Calendar）向 WE 推送事件，触发 NPC 内容生成。

---

## 模拟刷新机制（随机事件池）

> **核心思路：** 不是等玩家行为改变情绪值，而是世界本身按节律"刷新"——注入一个随机事件，NPC 对此作出反应，玩家的介入影响后续走向。

类似手游的"活动刷新"概念，但内容由 LLM 生成或从事件池采样。

### 设计模型

```
定时/阈值触发
    ↓
从 event_pool 随机抽取一个事件描述
    ↓
以此为 user_input 触发 PlayTurn（NPC 自主回合）
    ↓
LLM 根据事件 + 当前世界状态生成 NPC 反应内容
    ↓
玩家看到 NPC 内容后选择是否参与（评论/点赞/忽略）
    ↓
玩家行为触发情绪 delta → 下一轮循环
```

### 与 ScheduledTurn 的集成

`TriggerRule` 扩展 `event_pool` 字段，触发时随机选取一条作为 `user_input`：

```json
"scheduled_turns": [
  {
    "id": "world_refresh",
    "mode": "variable_threshold",
    "condition_var": "viral_heat",
    "threshold": 30,
    "probability": 0.8,
    "cooldown_floors": 5,
    "event_pool": [
      "[WORLD: 夜歌在深夜突然发布了一条语音消息，内容神秘]",
      "[WORLD: 破晓新闻发布突发报道，提及了一个与玩家相关的谣言]",
      "[WORLD: 一个新账号 @shadow_voice 开始关注玩家，没有任何帖子]",
      "[WORLD: 全城热搜词条突然出现了玩家的 ID]"
    ]
  }
]
```

不设 `user_input` 时，从 `event_pool` 随机选一条。若两者都有，`user_input` 优先。

### 与 MaterialLibrary 的关系

`event_pool` = 设计师手写的有限事件集（适合小型游戏）
`MaterialLibrary` = 设计师预生成的大量内容池（Tier 3 NPC 素材），支持 `search_material(tags)` 工具按情绪标签检索匹配事件

两者互补：event_pool 是轻量内联方案，MaterialLibrary 是可扩展的内容数据库方案。

---

## 多真人参与设计

> 当前 WE 是单会话架构（一个 GameSession = 一个玩家 + 一套变量）。
> 多真人参与需要重新思考"谁拥有会话状态"。

### 模式 1：共享会话（Shared Session）

多个玩家向同一个 `session_id` 发送 PlayTurn，变量状态共享。

```
玩家A → PlayTurn(session_id="shared_001", user_input="我点赞了夜歌的新帖")
玩家B → PlayTurn(session_id="shared_001", user_input="我转发了这条帖子并加了评论")
```

**挑战：** 并发写入变量冲突；LLM 不知道当前谁在说话（需要在 user_input 前加说话人标签）。
**实现路径：** 极小改动——PlayTurn 支持 `speaker_id` 字段，注入 `[玩家B：xxx]` 前缀即可。

### 模式 2：观察者 + 主操控者

一名玩家是"主角"（发送 PlayTurn），其他玩家是"观众"（只读，通过 SSE 订阅会话流）。

```
主操控者 → PlayTurn → 叙事内容
观察者 → SSE /play/sessions/:id/stream → 实时接收同一叙事
```

**实现路径：** SSE 事件推送到订阅该 session_id 的所有 clients。
**价值：** 直播/观战场景，游戏主播带粉丝互动。

### 模式 3：情绪众投（Crowd Voting）

多个玩家向会话"投票"影响世界情绪，定时聚合后触发一次 NPC 生成：

```
玩家A: player_action=argue  → 情绪 delta 记入"待聚合池"
玩家B: player_action=praise → 情绪 delta 记入"待聚合池"
玩家C: player_action=share  → 情绪 delta 记入"待聚合池"
    ↓（每 60 秒聚合一次）
聚合后的净情绪变化 → 写入会话变量 → ScheduledTurn 检查 → NPC 响应
```

**价值：** 类 Twitch Plays Pokémon 模式，玩家群体共同"调教" NPC 世界的情绪氛围。
**实现路径：** 需要聚合定时器 + 新 API `POST /sessions/:id/emotion-vote`。

### 模式 4：Session Fork（分叉多线）

每个玩家从同一基础世界 Fork 出自己的平行会话，体验个性化叙事，但 NPC 的基础状态共享。

```
基础世界（shared template + global variables）
    ├→ 玩家A 的 Fork session（个人的 chat 变量，个人的 floor 历史）
    └→ 玩家B 的 Fork session（独立状态，独立叙事线）
```

**已有基础：** WE 的 `POST /sessions/:id/fork` 已实现 Session Fork 机制。
**待补充：** 共享"全局变量层"跨 session 同步。

---

## 未来 MCP 集成方向

WE 的 Tools 层已建好（Tool 接口 + Registry），MCP 是自然的下一步接入点。

| MCP 工具分类 | 用途 | NPC 行为影响 |
|---|---|---|
| **RSS/新闻源** | 抓取真实世界事件 | NPC "报道"真实新闻，评论时事 |
| **日历/时区** | 知道当前时间 | NPC 按时区作息（深夜发不同内容） |
| **天气 API** | 知道玩家当地天气 | 天气驱动世界氛围（下雨→忧郁 NPC） |
| **跨 Session 全局事件总线** | WE 内部 MCP 服务器 | NPC A 的行为触发 NPC B 的世界书词条 |
| **外部 Webhook 接收** | 第三方推事件 | 游戏外行为（购买/签到）影响 NPC 状态 |

**未来架构（MCP 集成后的 NPC 触发钩子完整图）：**

```
外部事件（RSS / Webhook / Calendar）
    ↓ MCP 工具调用
WE 事件总线
    ├→ 写入会话变量（emotion.tension += X）
    ├→ 触发 ScheduledTurn（variable_threshold 检查）
    └→ 直接调用 worldbook_create（动态世界信息注入）
         ↓
       PlayTurn → NPC 内容生成 → SSE 推送给所有订阅者
```

---

## 阶段小结：WE 中期工作优先级

| 功能 | 当前状态 | 依赖 | 建议优先级 |
|---|---|---|---|
| **ScheduledTurn event_pool**（随机事件池） | 待实现（小，30 分钟） | ScheduledTurn MVP（✅） | ⭐⭐⭐ 立即做 |
| **StreamTurn Agentic Loop** | 待实现（小，1-2 小时） | Tools 层（✅） | ⭐⭐⭐ 近期 |
| **MaterialLibrary + search_material** | 待实现（中，1-2 天） | 无 | ⭐⭐⭐ 近期 |
| **ScheduledTurn 定时模式** | 待实现（中） | SSE 架构 | ⭐⭐ 中期 |
| **多真人情绪众投** | 设计阶段 | 聚合定时器 | ⭐⭐ 中期 |
| **Director 槽位** | 设计阶段 | 多 LLM 槽 | ⭐ 按需 |
| **MCP 接入** | 待实现（中） | Tools 层（✅） | ⭐⭐ 中期 |
| **共享 Session / 多人参与** | 设计阶段 | SSE + 并发控制 | ⭐ 长期 |
