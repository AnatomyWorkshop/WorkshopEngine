# SocialSim × WE 引擎——零后端实现分析

> 参考仓库：`D:\ai-game-workshop\plagiarism-and-secret\SocialSim`（ESSEX-CV9/SocialSim）
> 分析日期：2026-04-04
>
> **核心命题：** SocialSim 类游戏（多 NPC 社交模拟）的表现能力，能否通过 WE 引擎的**配置**实现——
> 游戏设计师只定义 Worldbook / PresetEntry / 变量 / 工具，不需要写任何���端代码？

---

## 目录

1. [SocialSim 是什么（功能快照）](#socialsim-是什么功能快照)
2. [功能映射：SocialSim → WE 配置层](#功能映射socialsim--we-配置层)
3. [当前已可零后端实现的部分](#当前已可零后端实现的部分)
4. [当前仍需代码的缺口](#当前仍需代码的缺口)
5. [缺口对应 WE 路线图](#缺口对应-we-路线图)
6. [SocialSim 作为 WE 路线图的设计参考](#socialsim-作为-we-路线图的设计参考)
7. [AI 作为 Player 2：情绪驱动生成模型](#ai-作为-player-2情绪驱动生成模型设计分析)
8. [本地运行说明（测试用）](#本地运行说明测试用)

---

## SocialSim 是什么（功能快照）

SocialSim 实现了一个**嵌入角色扮演世界的虚拟 Twitter/X 平台**。核心能力：

| 功能 | 说明 |
|------|------|
| **多 NPC 自主发帖** | 多个 AI 角色账号在同一游戏世界中独立发帖/回复/互动 |
| **压力驱动调度** | 玩家行为累积"压力"，达到阈值自动触发一轮 NPC 内容生成 |
| **三 Tier NPC 体系** | Tier 1 主角（精细生成）→ Tier 2 机构（批量）→ Tier 3 背景（素材库） |
| **社交状态追踪** | 点赞 / 转帖 / 关注 / 趋势话题 —— 全部持久化 |
| **素材库** | 预生成内容池，80% Tier 3 内容从库中取，不调用 LLM |
| **Twitter-like 前端** | Timeline / Explore / Profile / Notifications — 完整 UI |

SocialSim 为此写了约 **8,000 行自定义后端代码**（10 个模块 + 事件溯源 + SQLite 迁移）。
**问题是：如果用 WE 作为引擎，这 8,000 行里有多少不再需要写？**

---

## 功能映射：SocialSim → WE 配置层

下表以"设计师视角"分析每项功能：

| SocialSim 功能 | WE 配置层实现方式 | 是否已支持 | 说明 |
|---|---|:---:|---|
| **NPC 角色定义（persona / style）** | WorldbookEntry（Constant 或关键词触发） | ✅ 已支持 | 每个 NPC 一条 WorldbookEntry，Content = 角色 persona；名字作为触发关键词 |
| **NPC 互动背景知识** | WorldbookEntry（SecondaryKeys + ScanDepth 精控） | ✅ 已支持 | 当某 NPC 被提及时，其关联背景（朋友/敌人/历史）自动注入 |
| **社交媒体世界规则** | PresetEntry（InjectionOrder 排在最前） | ✅ 已支持 | "你正在模拟一个虚拟推特平台，用以下 JSON 格式输出帖子…" 写为 PresetEntry |
| **输出格式（帖子 JSON / XML）** | PresetEntry（规定 LLM 输出结构） + Parser（XML 三层解析） | ✅ 已支持 | 让 LLM 在 `<vn>` 标签内输出帖子对象数组；WE parser 自动解析 |
| **社交状态（点赞/关注/帖子列表）** | 变量沙箱（Chat/Session 层变量） | ✅ 已支持 | `feed`、`likes`、`follows` 存为 JSON 变量；每回合 LLM 读写 |
| **读取当前社交状态** | `get_variable` 工具 | ✅ 已支持 | LLM 调用 `get_variable("feed")` 读出已有帖子，决策时作为上下文 |
| **写入新帖子/互动** | `set_variable` 工具 | ✅ 已支持 | LLM 调用 `set_variable("feed", updated_feed)` 追加新帖；WE 持久化 |
| **记忆与历史叙事** | MemorySummary + 记忆整合 Worker | ✅ 已支持 | 趋势话题 / 重大事件可归纳进记忆，后续回合自动注入 |
| **关键词触发叙事注入** | WorldbookEntry（regex 关键词 + 递归激活） | ✅ 已支持 | `SocialSim.keywords.json` 的触发逻辑 = WE WorldbookEntry 关键词匹配 |
| **LLM 输出后处理（格式清洗）** | RegexProfile / RegexRule（ai_output 规则） | ✅ 已支持 | 清理 LLM 输出中的多余前缀、修正 JSON 格式等 |
| **Swipe 重新生成** | IsRegen=true，复用当前 Floor | ✅ 已支持 | 玩家不满意这轮 NPC 内容，可 Swipe 重新生成 |
| **多角色卡导入** | creation/card/parser.go（ST PNG 格式解析） | ✅ 已支持 | SillyTavern 角色卡 = WE 的 WorldbookEntry / PresetEntry 素材来源 |
| **前端接收结构化内容** | TurnResponse.VN（VNDirectives）+ Variables 快照 | ✅ 已支持 | 帖子数组编码进 `<vn>` 块；前端按照 VN 指令渲染 Twitter-like UI |
| **多 NPC 同时发帖（单次 LLM）** | PresetEntry 指导 LLM 批量输出 N 个帖子 | ✅ 有限支持 | 单次 one-shot，LLM 在一个响应里输出多个角色的帖子；依赖 prompt 设计 |
| **资源 CRUD 工具（创建/更新角色）** | ResourceToolProvider（23 工具，计划中） | 🔲 待实现 | LLM 动态创建新 NPC、更新 WorldbookEntry 内容 |
| **自主触发（NPC 不等用户就发帖）** | ScheduledTurn（自动回合，计划中） | 🔲 待实现 | 定时/压力驱动自动向 PlayTurn 发一条 `[SYSTEM]` 输入 |
| **素材库（预生成内容池）** | MaterialLibrary + `search_material` 工具（计划中） | 🔲 待实现 | Tier 3 背景 NPC 先查素材库，不足时调用 LLM；WE 可通过 Memory Store 近似实现 |
| **媒体代理（图片搜索）** | 无 | ❌ 范围外 | 与核心叙事引擎无关，前端独立实现 |
| **Twitter-like 前端 UI** | 无（WE 只是 API） | ❌ 范围外 | 前端设计与引擎无关，与 WE 对接只需改 API 调用层 |

---

## 当前已可零后端实现的部分

使用**当前 WE 版本**（不需要写任何 Go 代码），游戏设计师可以通过 WE 的 API 配置出以下能力：

### 1. 多 NPC 角色定义

在 WE 的 `POST /api/v2/create/templates/:id/worldbook` 创建 WorldbookEntries：

```json
{
  "keys": ["夜歌", "Yege"],
  "content": "夜歌 (@yege_official)：独立音乐人，玩世不恭，喜欢在深夜发帖。\n发帖风格：短句、碎碎念、偶尔分享歌词片段。\n最近在意的话题：新专辑录制、粉丝见面会、二次元。",
  "constant": false,
  "scan_depth": 5,
  "position": "before_template",
  "enabled": true
}
```

夜歌的名字被提到时，她的 persona 自动注入 prompt。不需要写任何代码。

### 2. 社交媒体输出格式

在 `POST /api/v2/create/templates/:id/preset-entries` 创建 PresetEntry：

```json
{
  "name": "social_feed_format",
  "role": "system",
  "content": "你正在模拟一个虚拟社交平台。每轮请以如下 XML 格式输出内容：\n\n<narrative>对玩家可见的描述文字</narrative>\n<vn>{\"posts\": [{\"author\": \"夜歌\", \"content\": \"...\", \"likes\": 0}]}</vn>\n<state_patch>{\"feed\": [...]}</state_patch>\n\n根据当前剧情，让 1–3 个 NPC 发帖，内容符合各自角色。",
  "injection_order": 100,
  "is_system_prompt": true,
  "enabled": true
}
```

所有输出解析、state_patch 持久化由 WE 自动处理。不需要写代码。

### 3. 社交状态追踪（变量沙箱）

初始化会话时传入初始变量：

```json
{
  "session_id": "...",
  "variables": {
    "feed": [],
    "likes": {},
    "follows": {"player": ["夜歌", "破晓新闻"]},
    "trending": ["#新专辑", "#虚拟城市"]
  }
}
```

LLM 通过 `get_variable` / `set_variable` 工具在每轮读写，WE 持久化到数据库。
**这完全替代了 SocialSim 的 `state-management/` 模块（~630 行）。**

### 4. 关键词触发叙事背景

利用 WorldbookEntry 的 `secondary_keys` + `secondary_logic`：

```json
{
  "keys": ["#新专辑"],
  "secondary_keys": ["夜歌", "音乐"],
  "secondary_logic": "and_any",
  "content": "【背景】夜歌即将发布第三张专辑《深夜电台》，录制过程因制作人临时退出出现波折，粉丝高度关注。",
  "scan_depth": 10
}
```

**完全替代了 SocialSim 的 `keywords.json` 触发系统，且支持正则和递归激活。**

### 5. NPC 记忆与趋势演化

WE 的记忆整合 Worker 会自动把"玩家发了什么帖子、哪个 NPC 回应了"归纳进 MemorySummary。
后续回合自动注入历史叙事，NPC 的反应会随时间演化。

**这部分 SocialSim 完全没有——WE 的记忆系统是 WE 对此类游戏的原生优势。**

---

## 当前仍需代码的缺口

有三个功能 WE 目前无法纯配置实现：

### 缺口 1：自主触发（NPC 不等玩家就发帖）

SocialSim 的核心体验之一：玩家不操作时，NPC 也会发帖，形成"活着的世界"感。

**WE 当前：** 严格 turn-based，每次 PlayTurn 都需要玩家输入。

**需要的能力：** `ScheduledTurn`——引擎后台按压力/时间自动调用一次 PlayTurn，`user_input = "[SYSTEM_TICK]"`，LLM 根据此信号产出 NPC 自主内容。

这在架构上是干净的：只是一个向已有 PlayTurn 发请求的定时器，不需要改动引擎核心。

### 缺口 2：ResourceToolProvider（游戏内 CRUD）

SocialSim 的 LLM 可以"创建新 NPC""修改角色关系"——这需要工具能够读写 Worldbook / PresetEntry / GameTemplate 资源。

**WE 当前：** 只有 `get_variable`、`set_variable`、`search_memory` 三个内置工具。

**需要的能力：** ResourceToolProvider 的 `worldbook:create`、`worldbook:update`、`character:read` 等工具——LLM 自己动态扩展世界定义，不需要人工干预。

### 缺口 3：素材库（Material Library）

SocialSim 的 Tier 3 背景 NPC 优先复用预生成内容（80% 命中），大幅降低 LLM 调用次数。

**WE 当前近似方案：** `search_memory` 工具可以搜索已有记忆条目；但无"素材库"概念（与特定会话/角色无关的通用内容池）。

**需要的能力：** 一个 `search_material(tags)` 工具，可以从游戏模板级别的素材库里按标签检索预生成内容，LLM 直接引用。

---

## 缺口对应 WE 路线图

三个缺口直接对应 WE 中期路线图中的待实现项：

| 缺口 | WE 路线图项 | 实现后能力 |
|---|---|---|
| 自主触发 | **ScheduledTurn**（独立定时器，调用 PlayTurn 传入 SYSTEM 输入） | NPC 在玩家不操作时自主发帖；也可以做日出日落、天气变化等环境事件 |
| ResourceToolProvider | **ResourceToolProvider（23 工具）** | LLM 可在游戏内 CRUD 角色卡、世界书、素材；游戏世界动态演化，不需要手动配置 |
| 素材库 | **MaterialLibrary + search_material 工具** | Tier 3 背景内容零 LLM 成本；也可用于角色台词模板、场景描述预设 |

**三项全部实现后，SocialSim 类游戏的后端代码需求量降至近乎零**——设计师只需要：
1. 导入角色卡（角色 persona + 触发关键词）
2. 写 1–2 条 PresetEntry（定义社交媒体输出格式）
3. 配置初始变量（空的 feed / likes / follows）
4. 配置 ScheduledTurn 频率（多久自动触发一次 NPC 活动）

---

## SocialSim 作为 WE 路线图的设计参考

SocialSim 独立完整地实现了 WE 路线图中多项"待实现"功能。它是最好的参考实现：

### 压力驱动调度器 → WE `ScheduledTurn` 设计参考

SocialSim 的调度模型：
```
用户行为 → 累积压力：user_message +30 / interaction +20 / new_post +40
累积 ≥ 阈值（默认100）→ 触发一轮 NPC 生成 → 队列释放（分层延迟）→ 压力归零
```

WE 的 `ScheduledTurn` 可以简化为两种模式参考此实现：
- **定时模式**：每 N 秒无论如何触发一次（简单，适合轻量游戏）
- **压力模式**：每次玩家 PlayTurn 后累积分值，达到阈值才触发（SocialSim 方案，更节省算力）

### 三 Tier 生成策略 → WE `director/verifier` slot 设计参考

| SocialSim Tier | WE LLM Slot |
|---|---|
| Tier 1（主角，独立精细生成） | `narrator` slot（主生成，完整 persona） |
| Tier 2（机构，批量） | `director` slot（批量规划，较低成本模型） |
| Tier 3（背景，素材优先） | 素材库直通（不调用 LLM；WE `MaterialLibrary`） |

### 素材库 → WE `search_material` 工具设计参考

SocialSim 素材条目结构可直接作为 WE MaterialLibrary 的 schema 参考：

```json
{
  "materialId": "mat-001",
  "type": "post",
  "content": "又是凌晨三点，录音棚的灯还亮着。",
  "genericTags": ["夜晚", "音乐", "独白"],
  "worldTags": ["yege", "indie"],
  "mood": "melancholy",
  "style": "lyrical",
  "functionTag": "atmosphere"
}
```

WE 的 `search_material(tags: ["夜晚", "melancholy"])` 工具可以按 genericTags 全文检索。

### 18 工具清单 → WE ResourceToolProvider 设计参考

SocialSim 已验证了以下工具对社交类游戏的必要性，WE ResourceToolProvider 应覆盖：

| SocialSim 工具 | WE 对应 | 优先级 |
|---|---|---|
| `get_account_profile` | `character:read` | 高 |
| `get_account_recent_posts` | `search_memory(session, author)` | 中（Memory 近似） |
| `get_world_setting` | `game_template:read` | 高 |
| `get_current_narrative_context` | `session:read(summary)` | 高 |
| `get_current_trends` | `worldbook:search(constant)` | 中（Worldbook 近似） |
| `get_post_detail` | `variable:get("feed[id]")` | 中 |
| `search_accounts` | `worldbook:search(keys)` | 中 |
| `get_account_relationships` | `variable:get("follows")` | 低（变量近似） |

---

## AI 作为 Player 2：情绪驱动生成模型设计分析

> 设想：设计一组**全局情绪值**，玩家行为或游戏数值映射到情绪值变化，
> 情绪值达到阈值后触发 NPC 内容生成，生成结束后情绪值反向更新；
> 前端负责阈值检测和随机因子，后端负责存储状态和驱动 LLM。

---

### 概念核心（与 SocialSim 压力调度器对比）

| 维度 | SocialSim 压力调度器 | 情绪驱动模型（新设想） |
|---|---|---|
| **驱动量** | 单一标量"压力值" | 多维情绪向量（紧张度/好感度/热度…） |
| **触发条件** | 压力 ≥ 阈值 | 情绪值跨阈 × 随机因子 |
| **触发后** | 生成一批 NPC 帖子，压力归零 | 生成内容**同时**情绪值向新均衡收敛 |
| **前端职责** | 仅渲染 | 阈值决策 + 随机数 + UI 风格映射 |
| **Prompt 影响** | 无直接影响（靠 Tier 路由） | 情绪值注入 Prompt，LLM 产出风格随情绪变化 |

情绪驱动模型的核心优势：**NPC 的行为风格会随世界氛围连续演变**，而非只有"有/无内容生成"的二值响应。

---

### 两种流派：TH 隐式涌现 vs WE 显式情绪值

TavernHeadless 和 WE 在 NPC 情绪建模上走了两条不同的路线，各有其适用场景：

**TH 的做法（隐式涌现）：**

| 机制 | 说明 |
|---|---|
| **Director 槽位** | 每轮主生成之前，一个独立的"剧情导演" LLM 先运行，产出文字意图（"本回合夜歌处于防御状态，回避与玩家的正面接触"），再传给主叙述者 Narrator。情绪是文字指令，不是数字。 |
| **Memory V2 + MemoryEdge** | 记忆系统跟踪 NPC 与玩家的关系图（MemoryEdge），通过关系历史自然涌现情感状态——不存储"好感 = 67"，而是存储"玩家曾在公开场合让夜歌难堪"这一事实。 |
| **核心哲学** | LLM 本身是最好的情绪引擎。给足人格描述 + 历史记忆，情绪变化自然涌现，无需系统介入。 |

**WE 显式情绪值方向的不可替代优势：**

- **可控节奏**：游戏设计师精确控制"什么事件触发什么效果"，LLM 不会随机偏移剧情节奏
- **前端可视化**：情绪数值作为游戏状态，直接驱动 UI 变化（背景音乐/氛围光效/角色表情）
- **触发系统基础**：ScheduledTurn 的触发规则需要一个可量化的驱动量——没有显式数值就无法做阈值判断
- **成本控制**：Director 槽位每轮多一次 LLM 调用；显式情绪值是纯客户端计算

**结论：** WE 的显式情绪值方向合理，但 **delta 映射不应是固定算术公式**——这正是下面 改进 3（重写版）要解决的问题。

---

### 当前 WE 已经支持的部分

**情绪值存储（变量沙箱）：完整支持**
```json
// 初始化会话变量
{
  "emotion": {
    "tension":    40,
    "affinity":   60,
    "viral_heat": 20
  },
  "npc_mood": {
    "夜歌":   { "trust": 55, "excitement": 30 },
    "破晓新闻": { "aggression": 20 }
  }
}
```
变量随每次 `state_patch` 持久化，跨回合保留。`get_variable` / `set_variable` 工具让 LLM 读写这些值。

**情绪值注入 Prompt（PresetEntry + 变量宏）：完整支持**
```
[PresetEntry, InjectionOrder=1]
当前世界情绪状态：紧张度 {{emotion.tension}}/100，玩家好感 {{emotion.affinity}}/100。
请根据以上状态调整你的叙事语气和 NPC 行为倾向。
```
变量宏在每轮展开——情绪值**直接影响 LLM 输出风格**，零后端代码。

**情绪值更新（state_patch）：完整支持**

LLM 在响应中输出：
```xml
<state_patch>
{"emotion": {"tension": 55, "affinity": 52, "viral_heat": 35}}
</state_patch>
```
WE 自动将 state_patch 写回变量沙箱，下一回合生效。

---

### 当前方案的 7 个改进点

#### 改进 1：情绪值需要时间衰减

**问题：** 没有衰减机制时，一次极端事件（tension 冲到 95）会永久锁定世界氛围，后续所有回合的 NPC 都是敌对状态。

**建议：** 参考 WE 的记忆半衰期（`HalfLifeDays`），为情绪值设计"每回合衰减系数"。
实现方式不需要后端改动：在游戏的 PresetEntry 里告诉 LLM：

```
每个回合，请在 state_patch 中将 emotion.tension 向基准值（40）回归：
新值 = 当前值 × 0.85 + 基准值 × 0.15
除非本回合有明确的紧张事件，否则不要将 tension 提升超过 10。
```

或在 `floor_history` 工具中读取最近 N 回合的值，计算趋势后决定是否衰减。

#### 改进 2：全局情绪 vs. 个体情绪需要分层

**问题：** 单一全局情绪值无法区���"玩家和 A 角色关系很好，但和 B 角色关系很差"。

**建议：** 两层结构：
```
emotion.global      → 影响所有 NPC 的世界氛围（气候）
emotion.npc.<name>  → 影响单个 NPC 对玩家的态度（个体天气）
```
WorldbookEntry 的关键词触发可以让个体情绪只在该角色相关的上下文中注入：
```json
{ "keys": ["夜歌", "@yege_official"],
  "content": "夜歌对玩家当前信任度：{{emotion.npc.夜歌.trust}}/100。" }
```

#### 改进 3（重写）：情绪 delta 不应是固定公式——以人格模型为参考

**原方案的局限：** 将"玩家争论 → tension +15"硬编码为常量，假设所有 NPC 对同一行为反应相同。但傲娇角色被点赞可能"无所谓"（affinity 几乎不变），渴望认可的角色却可能 affinity 激增；高神经质的角色受到批评比普通角色波动大两倍。

**大五人格（OCEAN）维度与情绪反应的映射参考：**

| 人格维度 | 低分行为模式 | 高分行为模式 | 对情绪变化的影响 |
|---|---|---|---|
| **Neuroticism（神经质）** | 情绪稳定，外部刺激影响小 | 情绪波动大，极易被刺激 | 高 N → 单次 tension 变化幅度翻倍 |
| **Agreeableness（宜人性）** | 冷漠、不在乎他人评价 | 重视关系、容易被感动 | 低 A → affinity 对善意行为几乎无响应 |
| **Extraversion（外向性）** | 内向、倾向独处 | 渴望互动、容易兴奋 | 高 E → 社交互动后 viral_heat 显著提升 |
| **Conscientiousness（尽责性）** | 随意、不在乎承诺 | 认真、重视信任 | 高 C → 被欺骗后 trust 暴跌幅度更大 |
| **Openness（开放性）** | 保守、排斥新事物 | 好奇、接受变化 | 高 O → 对新奇事件 curiosity 激增 |

MBTI 类型提供更直觉的设计语言（INTJ 的冷淡、ENFP 的热情），底层可映射到 OCEAN 维度。两套体系都可以用，关键是用**自然语言描述**而非数字公式。

**实现方式：人格描述写入 WorldbookEntry，LLM 自主判断 delta**

```json
{
  "keys": ["夜歌", "@yege_official"],
  "content": "【夜歌的情绪人格】\nN=高：情绪起伏大，对批评极度敏感，容易从兴奋跌入沮丧。\nA=中低：有防御性，不轻易相信陌生人，好感需长期积累。\nE=中：表演时光芒万丈，私下享受独处，过度社交后能量下降。\n\n💡 情绪指导：玩家积极互动让她 affinity 缓慢提升，但负面评论让 tension 急剧上升（约普通人的 2 倍）。在 state_patch 中，单次 affinity 上升不应超过 +8，除非是她亲自认可的深度互动。",
  "constant": false,
  "scan_depth": 0
}
```

`emotion_config` 改为**软约束边界 + 行为语义描述**，LLM 在边界内自行决定幅度：

```json
"emotion_config": {
  "dimensions": ["tension", "affinity", "viral_heat", "trust"],
  "global_baseline": { "tension": 40, "viral_heat": 10 },
  "npc_delta_limits": {
    "default": { "single_turn_max": 20, "single_turn_min": -20 },
    "夜歌":    { "tension_max": 40, "affinity_max": 8 }
  },
  "player_action_hints": {
    "player_argue":   "激烈争论，情绪往往向负面方向演化，但具体幅度因角色人格而异",
    "player_praise":  "赞美，通常提升好感，但某些角色会觉得被居高临下对待反而反感",
    "player_ignore":  "忽视，外向角色可能感到受伤，内向角色反而放松",
    "player_share":   "分享/转发，提升话题热度，对渴望认可的角色好感上升"
  }
}
```

`player_action_hints` 是**自然语言语义提示**，不是数字。前端把对应的 hint 文本拼入 PlayTurn 的 `user_input` 前缀，LLM 结合角色人格自主决定本轮情绪变化，再通过 `state_patch` 写回。

**两种方案对比：**

| 维度 | 固定公式（旧方案） | 人格导向（新方案） |
|---|---|---|
| 不同 NPC 的反应差异 | 无（所有角色相同） | 自然，由人格描述驱动 |
| 极端/边缘角色 | 无法自然表达 | 天然支持，见下节 |
| 新增行为类型 | 需要修改配置数字 | 补充 hint 文本即可 |
| 前端计算 | 精确，零延迟 | LLM 判断（少量延迟） |
| 可预测性 | 高（完全确定性） | 中（可用 delta_limits 约束边界） |

对于节奏控制（何时触发生成），前端继续维护**门控阈值**（trigger_rules，见改进 4），但具体情绪值的涨落由 LLM 根据人格自主决定。

#### 改进 4：前端随机因子需要明确的触发规则契约

**问题：** 前端要判断"情绪值跨阈是否触发生成"，但触发规则散落在设计文档里，不在 WE 数据里，前端无法自动化。

**建议：** 在 `GameTemplate.Config` 中定义触发规则（同时也作为 ScheduledTurn 的配置来源）：
```json
"trigger_rules": [
  {
    "id": "hostile_npc",
    "condition_var": "emotion.tension",
    "threshold": 70,
    "probability": 0.75,
    "cooldown_floors": 3,
    "generation_hint": "[SYSTEM: 紧张状态，生成一条 NPC 敌对帖子]"
  },
  {
    "id": "viral_explosion",
    "condition_var": "emotion.viral_heat",
    "threshold": 80,
    "probability": 1.0,
    "cooldown_floors": 10,
    "generation_hint": "[SYSTEM: 话题爆炸，生成一波 Tier 2 机构跟帖]"
  }
]
```
前端逻辑：
```js
const rule = triggerRules.find(r =>
  getVar(r.condition_var) >= r.threshold &&
  Math.random() < r.probability &&
  floorsElapsed(r.id) >= r.cooldown_floors
)
if (rule) callPlayTurn({ user_input: rule.generation_hint })
```
**好处：** 触发规则是游戏数据，不是硬编码；同一套规则可以被 ScheduledTurn 复用。

#### 改进 5：情绪值到 LLM 风格的映射需要梯度设计，不只是开/关

**问题：** 当前 PresetEntry 是静态文本，只能传递"tension=55"这个数字，LLM 需要自己理解这个数字的含义，理解质量不稳定。

**建议：** 用多档 WorldbookEntry 描述情绪含义，在对应变量范围内激���：
```
WorldbookEntry A: keys=["{{emotion.tension}} > 80"]（或通过 Constant + 前端 enable/disable 控制）
  content: "【世界警告】全局紧张度极高，NPC 普遍处于防御和攻击状态，任何对话都可能演变成冲突。"

WorldbookEntry B: keys=常驻 content:
  "当前情绪状态摘要：紧张度 {{emotion.tension}}/100（{{tension_label}}）"
```
或更直接：通过 `worldbook_update` 工具在触发时动态更新常驻词条内容，让语言描述而非数字驱动 LLM 风格。

#### 改进 6：生成后情绪值反馈需要闭环稳定性设计

**问题：** 高 tension → 触发生成 → LLM 生成敌对内容 → state_patch 设置 tension += 20 → 立刻再次触发 → 无限循环。

**建议：** 明确冷却机制和衰减契约：
- 触发规则中的 `cooldown_floors` 防止连续触发（见改进 4）
- 生成后 state_patch 应**降低**触发维度的情绪值（tension 回到阈值以下），而不是升高
- LLM 的 state_patch 指令中明确写入："生成完成后，将 emotion.tension 降低 25，代表紧张状态已释放"
- `emotion_config.npc_delta_limits` 中可以添加 `after_generation_cooldown` 字段，明确生成后的冷却衰减

#### 改进 7：情绪变化需要作为叙事钩子（不只是数字）

**问题：** 情绪值是隐藏数字，玩家无法感知世界"为什么变了"。

**建议：** 当情绪值跨越重要阈值时，**同步写入一条 Memory（事实记忆）和一条新 WorldbookEntry**：
```
[generation_hint 触发后，LLM 被指示执行:]
<state_patch>{"emotion": {"tension": 48}}</state_patch>
[同时 LLM 调用工具:]
memory_create("由于玩家的激烈争论，全场气氛陷入紧张，NPC 们开始表现出不安。", importance=0.9)
worldbook_create(keys=["紧张事件", "气氛"], content="玩家昨日的言论引发了一场公开争议，各方情绪尚未平息。", constant=false)
```
这样情绪值的变化就变成了**有叙事意义的世界事件**，后续回合的 NPC 会"记得"这件事。

---

### 极端角色与创作自由

AI 世界中，极端角色（偏执者、反派、高度不稳定的人格）是正当的创作需求，也是最能检验情绪系统弹性的场景。WE 的人格导向模型天然支持这类角色，不需要任何特殊代码处理：

**构造极端角色的参数组合：**

| 目标性格 | 人格描述要点 | delta_limits 配置 |
|---|---|---|
| 多疑偏执型 | N=极高、A=极低、"对一切善意都解读为阴谋" | trust_max=5（几乎不可建立信任） |
| 情绪爆炸型 | N=极高、C=低、"情绪零门槛崩溃" | tension_max=100（可达到暴走状态） |
| 冷漠反社会型 | A=极低、E=低、"他人的情绪对其无意义" | affinity_max=3，tension_min=0（外部刺激几乎无效） |
| 狂热崇拜者 | N=高、O=高、"对目标对象情感依附极深" | 对特定 NPC 的 trust_max=100，对他人 trust_max=10 |

**WorldbookEntry 示例（反社会型）：**

```json
{
  "keys": ["凌寒"],
  "content": "【凌寒的情绪人格】\nN=极高：情绪像烈火，瞬间点燃，瞬间熄灭，没有中间状态。\nA=极低：不理解同情心，他人的眼泪让他感到不耐烦而非怜悯。\n\n💡 情绪指导：玩家的任何善意行为对他的 affinity 影响几乎为零（上限 +2）；玩家挑战或冒犯他，tension 不升反降（他欣赏强硬的对手）。这个角色的情绪轨迹与常规逻辑相反——请如实呈现，不要强行向'正常'方向修正。",
  "constant": false
}
```

**关键设计原则：** 内容层面的极端完全由游戏设计师掌控，引擎层不做内容限制。极端角色的可信度来自人格描述的一致性，而非固定公式——这正是 LLM 驱动的优势所在。

---

### 情绪配置的前端调用接口

`emotion_config` 存储在 `GameTemplate.Config` JSONB 字段中，前端通过现有 creation API 读取，**无需新增后端接口**：

```
GET /api/v2/create/templates/:id
```

响应体包含 `config` 字段，前端解析其中的 `emotion_config`：

```js
// 前端初始化时读取情绪配置（一次性，不随每轮变化）
const template = await fetch(`/api/v2/create/templates/${gameId}`).then(r => r.json())
const emotionConfig = template.config?.emotion_config ?? {}

// 玩家操作时，把语义 hint 拼入下一轮 PlayTurn 的用户输入前缀
function onPlayerAction(action, userInput = '') {
  const hint = emotionConfig.player_action_hints?.[action] ?? ''
  const prefix = hint ? `[行为上下文：${hint}] ` : ''

  callPlayTurn({
    user_input: prefix + userInput,
    variables:  currentEmotionState   // 当前情绪状态跟随变量一起发送
  })
}
```

**配置层 vs 状态层分离：**

| 层次 | 来源 | 更新频率 | 接口 |
|---|---|---|---|
| **情绪配置**（规则/边界） | `template.config.emotion_config` | 游戏设计时一次性配置 | `GET/PATCH /api/v2/create/templates/:id` |
| **情绪状态**（当前数值） | 会话变量 `emotion.*` | 每回合通过 state_patch 更新 | `POST /api/v2/play/turn` 的 `variables` 字段 |

更新情绪配置（游戏设计师操作，需编辑权限）：

```
PATCH /api/v2/create/templates/:id
Body: { "config": { "emotion_config": { "npc_delta_limits": { "凌寒": { "affinity_max": 2 } } } } }
```

这个接口已经存在（creation API 的 template PATCH），不需要写任何新代码。

---

### mirofish 情绪系统思路（待补充）

> ⚠️ **注意**：在本地代码库和公开仓库搜索中均未找到 "mirofish" 相关参考实现。
> 如果您有相关链接、代码库地址或文档，请提供后补充到本节。
>
> 需要对比的问题：mirofish 的 NPC 情绪触发机制是显式数值驱动、隐式涌现，还是混合模式？
> 它对"玩家行为影响 NPC 情绪"这一问题的解法与上述两种流派有何异同？

---

### 完整情绪驱动流程（设计师零后端视角）

```
玩家操作（点赞/评论/争论）
    │
    ▼ [前端] 读 emotion_config.player_action_hints → 拼 hint 前缀到 user_input
    │
    ▼ [前端] 检查 trigger_rules × random() → 决定是否触发（阈值判断仍用显式数值）
    │
   ┌┤ 不触发：用 hint 前缀 + 当前 variables 调用 PlayTurn（正常回合）
   ││
   └┤ 触发：调用 PlayTurn，user_input = rule.generation_hint
    │
    ▼ [WE] 变量宏展开 → 情绪值注入 Prompt（PresetEntry + WorldbookEntry 人格描述）
    │
    ▼ [WE] LLM 生成内容（风格由人格描述 + 当前情绪值共同驱动，不是固定公式）
    │
    ▼ [WE] LLM 在 state_patch 中输出本轮情绪 delta（人格约束在 system prompt 中，边界在 emotion_config 中）
    │
    ▼ [WE] 解析 state_patch → 情绪值写回变量沙箱
    │
    ▼ [WE] LLM 可选调用 memory_create / worldbook_create（叙事钩子）
    │
    ▼ [前端] 渲染响应 + 根据新情绪值更新 UI 风格（背景色/音乐/动画）
```

**当前 WE 支持以上流程的全部步骤，不需要写任何后端代码。**
`emotion_config`（配置层）和情绪变量（状态层）均已有存储和读取路径：
- 配置：`GET /api/v2/create/templates/:id` 响应体中的 `config.emotion_config`
- 状态：会话变量沙箱，每轮 `state_patch` 自动持久化

---

### 与 ScheduledTurn 的关系（待实现）

情绪驱动模型目前依赖**前端检测触发**。当 ScheduledTurn 实现后：

```json
"scheduled_turns": [
  {
    "mode": "variable_threshold",
    "condition_var": "emotion.tension",
    "threshold": 70,
    "probability": 0.6,
    "cooldown_floors": 3,
    "user_input": "[SYSTEM: 紧张状态自主触发]"
  }
]
```

服务端在每次 PlayTurn 完成后检查规则，自动调度下一回合，**无需前端轮询**。
这是情绪驱动模型的最终形态：玩家不操作时，世界也会因为情绪值达到阈值自动演化。

---

## 本地运行说明（测试用）

```bash
# 1. 进入 SocialSim 目录
cd "D:\ai-game-workshop\plagiarism-and-secret\SocialSim"

# 2. 安装依赖（monorepo）
npm install

# 3. 配置环境变量
# 在 server/ 目录下手动创建 .env：
# PORT=4455
# 启动后在 UI 的 Settings → Agent 里配置 LLM Provider

# 4. 构建 contracts 包
npm run build --workspace=contracts

# 5. 启动后端（开发模式，自动重编译）
npm run dev --workspace=server
# → 监听 http://localhost:4455

# 6. 另开终端启动前端
npm run dev --workspace=client
# → 监听 http://localhost:5173

# 7. 首次配置
# Settings → Agent → Add Provider（填入 base_url + api_key + model）
# Settings → WorldConfig → 创建一个空世界（或导入 Worldpack ZIP）
# Settings → Scheduler → 压力阈值设为 50（方便测试触发）

# 8. 触发测试
# 在 Timeline 发一条帖子
# 或直接调用：POST http://localhost:4455/api/scheduler/trigger
# 查看 NPC 自动响应与 LLM 调用日志：GET http://localhost:4455/api/llm-calls
```

**测试重点：**
1. 对比 Economy / Standard / Quality 三档的 LLM 调用次数（Settings → Agent → Mode）
2. 验证三 Tier 体系：Tier 1 角色内容质量 vs Tier 3 背景 NPC 的素材库命中率
3. 压力调度器的触发频率与游戏沉浸感的关系

**与 WE 联动验证（零后端方案原型）：**
- 启动 WE backend-v2（`go run cmd/server/main.go`，端口 8080）
- 创建一个 WE 游戏模板，PresetEntry 定义社交媒体输出格式
- 玩家操作 → 前端同时调用 WE `/play/turn` + SocialSim `/api/scheduler/trigger`
- 对比两种方案输出：纯 WE（配置驱动）vs SocialSim（专用后端）的叙事深度与互动密度
