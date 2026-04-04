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
7. [本地运行说明（测试用）](#本地运行说明测试用)

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
