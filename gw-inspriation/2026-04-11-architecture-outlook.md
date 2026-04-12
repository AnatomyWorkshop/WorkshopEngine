0# GW 架构展望：存档形态、分享、记忆与未来功能

> 编写于 2026-04-11，更新于 2026-04-11（补充 light 模拟器参考、llmwiki 定位、CW 解耦、技术债务）
> 性质：设计意图与时机判断，不是已实现功能的说明。

---

## 一、个人游戏库与游戏存档的形态

### 1.1 两个独立概念

**个人游戏库（LibraryEntry）** 是"我拥有/关注这个游戏"的声明，类似 Steam 库。
**游戏存档（GameSession）** 是"我在这个游戏里的一次游玩记录"，类似存档槽。

两者解耦：
- 库里有游戏，不代表有存档（刚导入还没玩）
- 有存档，不代表在库里（存档可以独立存在，例如从分享链接 fork 的临时存档）
- 一个游戏可以有多个存档（多周目、多分支）

```
LibraryEntry
  user_id + game_id (UNIQUE)
  series_key        ← 前端生成，通常 = game.slug，用于多版本归组
  source            ← catalog | local
  last_played_at    ← 最近游玩时间（用于排序）

GameSession
  game_id + user_id
  floor_count       ← 已玩楼层数
  variables         ← 当前沙箱变量状态
  memory_summary    ← 最新记忆摘要
  is_public         ← 是否公开（用于游记分享）
  branch_id         ← 当前所在分支（默认 main）
```

### 1.2 存档的内部结构

一个 GameSession 的完整存档由以下层次构成：

```
GameSession
  └── Floor[]          ← 每个回合（用户输入 + AI 回复）
        └── MessagePage[]  ← 每次生成尝试（Swipe）
              └── Messages[]  ← 实际消息列表（user/assistant/tool）
  └── Memory[]         ← 记忆条目（fact / summary / open_loop）
  └── MemoryEdge[]     ← 记忆关系图（updates / contradicts / supports / resolves）
  └── PromptSnapshot[] ← 每楼层的 Prompt 资源快照（世界书命中、token 用量等）
  └── Variables        ← 沙箱变量（JSONB，实时状态）
```

存档可以通过 `.thchat` 格式导出（`GET /api/play/sessions/:id/export`），包含完整的 Floor/Page/Memory/Branch 数据，可在不同实例间迁移。

---

## 二、存档翻译为 MVM 文件

### 2.1 什么是 MVM 文件

`.mvm` 文件是 MVM 渲染协议的序列化格式（见 `inspiration/mvm-rendering.md`）。
它将游戏内容分为三层：MODEL（类型 Schema）/ MSG（数据实例）/ VIEW（渲染规则）。

当前 `parser.go` 的 `ParsedResponse` 已经是简化版 MVM MSG；
`VNDirectives` 是 VN 场景的 MODEL；`ParseMode` 决定用哪条 VIEW 渲染链。

### 2.2 存档 → MVM 的转换路径

将 GameSession 翻译为 `.mvm` 文件，本质是将每个 Floor 的 `MessagePage.Messages` 重新解析为结构化 MSG：

```
GameSession.Floors
  → 每个 Floor 取 active MessagePage
  → 取 assistant 消息的 Content
  → parser.Parse(content, parseMode) → ParsedResponse（MVM MSG）
  → 序列化为 .mvm 的 ===MSG=== 段
```

**需要做的工作：**
1. 新建 `POST /api/play/sessions/:id/export/mvm` 端点
2. 遍历 Floors，对每个 active page 的 assistant content 调用 `parser.Parse`
3. 将 ParsedResponse 序列化为 MVM 格式（`===MODEL===` 段引用 GameTemplate.Config 的 schema）
4. 返回 `.mvm` 文件流

**时机：** 当前 `parser.go` 已稳定，MVM 导出可以在前端 Phase 3（剪辑分享）之前实现，作为分享功能的数据基础。

### 2.3 `.thread` 文件在这里的作用

`.thread` 文件（如果存在）是对话线程的序列化格式，与 `.thchat` 类似但更轻量——只包含消息序列，不包含 Memory/Branch 等引擎状态。

在 GW 的语境下：
- `.thchat` = 完整存档（可迁移、可恢复游玩）
- `.thread` = 纯对话记录（用于分享阅读，不可恢复游玩）
- `.mvm` = 结构化内容（用于跨平台渲染，携带 MODEL/VIEW 信息）

**当前状态：** GW 后端已实现 `.thchat` 导出（`session_export.go`）和 JSONL 导出（ST 兼容格式）。`.thread` 格式尚未定义，可以在剪辑分享功能启动时一并设计。

---

## 三、剪辑分享何时可以启动

### 3.1 前置条件

剪辑分享（Clip）依赖以下已完成的基础设施：

| 依赖 | 状态 |
|------|------|
| Floor/Page 数据模型 | ✅ 已完成 |
| `parser.Parse` 结构化解析 | ✅ 已完成 |
| `GET /sessions/:id/floors?from=&to=` 范围查询 | ✅ 已完成（A-4）|
| `is_public` 存档公开标记 | ✅ 已完成（A-4）|
| 论坛帖子系统 | ✅ 已完成（Task 4-C）|

### 3.2 需要新增的工作

```
1. Clip 数据模型（internal/platform/clip/model.go）
   Clip { id, session_id, game_id, title, frames[], cover, created_by }
   ClipFrame { floor_id, page_id, seq, model_type, data(ParsedResponse), thumbnail }

2. POST /api/play/sessions/:id/clips { from_floor, to_floor }
   → 读取 floors[from..to] active pages
   → 每个 floor 调用 parser.Parse → ClipFrame
   → 写入 DB，返回 clip_id

3. GET /api/clips/:id
   → 返回 Clip + ClipFrames（用于前端播放）

4. 论坛帖子引用 clip_id（PostBlock.ClipBlock）
   → 前端渲染为嵌入卡片（minimal VIEW）
```

### 3.3 启动时机

**可以在前端 Phase 3 完成后启动**（Phase 3 = 评论 Thread 模式 + 消息 Markdown 渲染）。
剪辑分享不阻塞任何当前 Phase 1/2 的工作，可以作为独立功能并行开发。

---

## 四、light 和 rich 游戏类型

### 4.1 当前状态

`GameTemplate.Type` 字段已有 `text | light | rich` 三个值，但目前只有 `text` 类型有完整实现。

### 4.2 light 类型：模拟器范畴

**重新定义：** `light` 不只是"轻量视觉小说"，而是**所有带状态 UI 的模拟器类游戏**的统称。
凡是需要在对话流之外展示持久状态面板（角色立绘、属性栏、地图、关系网络、物品栏等）的游戏，都属于 `light` 范畴。

**典型场景：**
- 视觉小说：立绘 + 背景图 + 对话框（最简单的 light）
- 养成模拟：角色好感度/属性面板 + 日历/时间轴
- 经营模拟：资源面板 + 地图/建筑布局
- 角色扮演：HP/MP/装备栏 + 技能列表
- 社交模拟：关系网络图 + 角色状态

**开源参考实现：**

| 项目 | 类型 | 参考价值 |
|------|------|---------|
| [Ren'Py](https://github.com/renpy/renpy) | 视觉小说引擎（Python）| 立绘/背景/BGM 的资源管理和场景切换逻辑；`show`/`hide`/`scene` 指令对应我们的 `VNDirectives` |
| [Twine](https://github.com/klembot/twinejs) | 超文本叙事（JS）| 变量驱动的条件分支；`<<if>>`/`<<set>>` 对应我们的 `variable.Sandbox` |
| [Ink](https://github.com/inkle/ink) | 叙事脚本语言（C#）| 分支权重、knot/stitch 结构；`CHOICE`/`GATHER` 对应我们的 `options[]` |
| [RPG Maker MZ](https://github.com/rpgmakermz) | RPG 引擎（JS）| 事件系统和变量开关；`$gameVariables`/`$gameSwitches` 对应我们的 `Variables` JSONB |
| [Bitsy](https://github.com/le-doux/bitsy) | 极简叙事游戏（JS）| 最小可行的房间/物品/对话系统；适合理解 light 的最小实现边界 |
| [GB Studio](https://github.com/chrismaltby/gb-studio) | 像素 RPG（JS/C）| 场景切换和角色移动的状态机；适合理解 rich-B（iframe 沙箱）的边界 |

**与 WE 引擎的对应关系：**

这些模拟器的核心都是**变量驱动的状态机**——与 WE 的 `variable.Sandbox` + `set_variable`/`get_variable` 工具完全对应。
区别在于：传统模拟器的状态转换由脚本硬编码，WE 的状态转换由 LLM 决策 + 工具调用驱动。

**实现可行性：高。**

当前 `VNDirectives` 和 `VNScene` 已经在 `parser.go` 中实现，`engine/api` 已有 VN 渲染路径。
前端需要：
- 立绘/背景图渲染组件（`VNRenderer`）
- 状态面板组件（`StatusBar`，读取 `session.variables`）
- 对话框 UI（区别于 `TextSessionPage` 的消息气泡）
- 素材路由 `/api/game-assets/:slug` 已实现

**缺口：**
- 前端 `TextSessionPage` 当前只有 `narrative` VIEW，需要增加 `vn-full` VIEW 分支
- 素材（立绘/背景）需要在 `GameTemplate.Config` 中声明映射关系
- `StatusBar` 需要知道哪些变量要展示（`config.display_vars` 字段，待定义）

**时机：** 前端 Phase 2 完成后可以启动，不依赖记忆系统改进。

### 4.3 rich 类型

**定义：** 富媒体游戏，分两个子类型：
- `rich-A`：完整视觉小说引擎（背景 + 三槽立绘 + BGM + 动画），后端驱动
- `rich-B`：iframe 沙箱，嵌入独立 JS 游戏，通过 postMessage 桥接 WE 引擎

**实现可行性：中（rich-A）/ 低（rich-B）。**

`AudioEvent` 和 `StateNotice` 在 `mvm-rendering.md` 中已有设计，但尚未实现。
主要挑战是前端的音频管理和状态 UI 的复杂度，后端改动较小。

**时机：** light 类型稳定后再考虑，不是近期优先项。

---

## 五、llmwiki 属于哪类游戏

**结论：llmwiki 是 `text` 类型游戏的一种特殊配置，不需要新的游戏类型。**

Karpathy 的 LLM Wiki 模式（见 `inspiration/karpathy-llmwiki-analysis.md`）本质是：
> LLM 增量维护一个持久化知识库，每次摄入新源时主动整合、更新交叉引用。

对应到 WE 引擎：

| Karpathy 概念 | WE 原语 |
|---|---|
| Raw sources（不可变原始文档）| `Material` 表 |
| Wiki（LLM 维护的结构化知识）| `Memory` 表 + `WorldbookEntry` |
| Schema（配置 LLM 行为）| `GameTemplate.SystemPromptTemplate` + `PresetEntry` |
| Ingest（摄入新源）| 用户发一条消息 = 一次 PlayTurn |
| Query（查询）| 同上，PlayTurn 的另一种用法 |

**实现方式：** 将 `GameTemplate` 配置为"知识库维护助手"模式，`enabled_tools` 包含 `search_memory`/`search_material`，`SystemPromptTemplate` 指示 LLM 如何摄入和整合知识。零额外开发，配置即可。

**为什么不是 `light` 或 `rich`：** llmwiki 不需要状态面板或立绘，纯文本对话流足够。如果需要展示知识图谱（关系网络），可以用 `light` 类型的 `StatusBar` 渲染 `session.variables` 中的图谱数据，但这是可选增强，不是必须。

---

### 5.1 当前记忆系统

当前记忆系统是**显式结构化记忆**：
- `Memory.Type = fact`：明确事实，有 `fact_key` 稳定键，可被 `updates` 关系覆盖
- `Memory.Type = summary`：剧情摘要，自由文本
- `Memory.Type = open_loop`：待解决悬念
- `MemoryEdge`：记忆关系图（updates / contradicts / supports / resolves）
- `Memory.Importance`：衰减权重，控制注入优先级
- `Memory.StageTags`：阶段标签，多幕叙事时按 `game_stage` 变量过滤

这套系统是**显式记忆**：内容由 LLM 生成，结构由代码约束，检索是确定性的（按 importance 排序 + stage 过滤）。

### 5.2 向量记忆库的价值

向量记忆库（pgvector 或外部向量 DB）提供**语义检索**：给定当前场景描述，找出语义最相关的记忆条目，而不是按时间/重要性排序。

**三个应用场景：**

#### 场景 A：Material 库语义检索（最重要）

当前 `search_material` 工具用标签匹配（`WHERE ? = ANY(tags)`），是精确匹配。
向量化后可以做"找和当前场景氛围最接近的素材"，不依赖标签命中。

**为什么这是最重要的应用：**

Material 库的核心价值在于**对抗对话风格和故事走向的过度偏向**。
当玩家持续选择某种风格的选项（如一直选择"温柔回应"），LLM 会逐渐强化这个方向，导致故事走向单一、玩家迅速厌倦。
向量化的 Material 检索可以在每回合注入"当前场景语义相关但风格多样"的素材，打破这种正反馈循环：

```
当前场景向量 → 检索 Material 库 → 返回语义相关但风格不同的素材
→ 注入 Prompt → LLM 有更多叙事可能性 → 故事走向更丰富
```

这比"创作者手动打标签"更可靠，因为标签是离散的，向量是连续的，能捕捉到标签无法描述的细微风格差异。

**时机：** Material 库内容量达到 100+ 条时，标签匹配开始出现漏召回，此时引入向量检索有明显收益。当前 seed 数据量不足，暂不需要。

#### 场景 B：常驻角色记忆迁移（A-11）

常驻角色从一个 Session 迁移到另一个 Session 时，需要携带"重要记忆"（`importance >= 7`）。
向量化后可以做"找和新游戏世界观最相关的记忆"，而不是简单按 importance 阈值截断。

**时机：** A-11（常驻角色）进入开发计划时一并考虑。

#### 场景 C：玩家画像

玩家的游玩行为（选择倾向、对话风格、偏好场景）可以向量化为"玩家画像"，用于：
- 个性化推荐（推荐相似风格的游戏）
- 游戏内 NPC 对玩家风格的感知

**时机：** 这是平台级功能，需要足够的用户数据积累，不是近期优先项。

### 5.3 向量记忆与显式记忆的区分

**显式记忆（当前已实现）：**
- 内容：LLM 生成的结构化事实/摘要
- 检索：确定性（importance 排序 + stage 过滤）
- 可视化：直接展示 `Memory.Content` 文本
- 可编辑：用户/创作者可以直接修改 `Memory.Content`

**向量记忆（待实现）：**
- 内容：文本的 embedding 向量（不可直接阅读）
- 检索：语义相似度（近似最近邻）
- 可视化：需要将向量"解释"为人类可读文本（通常是原始文本本身）
- 可编辑：修改原始文本后需要重新 embed

**关键区分：** 向量记忆不是替代显式记忆，而是为显式记忆提供更好的检索入口。
实现时应保持两套系统独立：显式记忆表不变，向量索引作为附加层（`memory_embeddings` 表，存 `memory_id + embedding`）。

---

## 六、重大节点和结局的记忆修改

### 6.1 当前机制

当前记忆写入是**异步后台任务**（`memory_consolidation` job），由 `scheduler` 在每 N 楼后触发，LLM 自动从对话历史中提取 fact/summary。

这套机制适合"渐进式记忆积累"，但不适合"重大节点"——例如：
- 玩家做出了不可逆的选择（杀死了某个角色）
- 游戏进入了新的章节（`game_stage` 变量变化）
- 结局触发（游戏结束）

### 6.2 重大节点记忆的实现思路

**方案：事件驱动的记忆写入**

在 `variable.Sandbox` 的变量变化监听中，当特定变量（如 `game_stage`、`ending_triggered`）发生变化时，触发一次**同步**的记忆写入，而不是等待后台任务。

```go
// 伪代码
if sandbox.Changed("game_stage") || sandbox.Changed("ending_triggered") {
    // 立即触发记忆整合，不等待 N 楼计数器
    memoryWorker.ConsolidateNow(sessionID, "stage_transition")
}
```

同时，为重大节点记忆打上特殊标签：
```go
Memory{
    Type:      MemoryFact,
    FactKey:   "stage_transition:act2",
    Content:   "玩家在第二幕选择了背叛路线，陈天已死",
    Importance: 10.0,  // 最高重要性，不衰减
    StageTags: []string{"act2", "act3"},  // 在后续所有幕次注入
}
```

**时机：** 这个功能依赖 `variable.Sandbox` 的变量变化事件机制，需要在 Sandbox 中加入 `OnChange` 钩子。当前 Sandbox 是无状态的（每楼重建），需要先改造为有状态的（跨楼持久化变化记录）。这是中期工作，不阻塞当前 Phase 1/2。

### 6.3 记忆可视化

**显式记忆的可视化（相对容易）：**
- `Memory.Content` 是人类可读文本，直接展示即可
- `MemoryEdge` 可以渲染为关系图（节点 = Memory，边 = Relation）
- `Memory.Importance` 可以用颜色/大小表示衰减程度
- `Memory.StageTags` 可以用标签展示当前激活状态

**向量记忆的可视化（困难）：**
- embedding 向量本身不可读，需要降维（t-SNE/UMAP）才能可视化
- 降维后的 2D 坐标可以展示"记忆聚类"，但解释性差
- 实用的做法：不可视化向量本身，只展示"语义检索结果"（给定查询，返回最相关的 N 条显式记忆）

**建议：** 记忆可视化 UI 应该只展示显式记忆（`Memory` 表），向量层对用户透明。
创作者调试面板可以展示"本回合注入了哪些记忆"（来自 `PromptSnapshot.ActivatedWorldbookIDs` 的扩展），这比展示向量更有实用价值。

---

## 八、CW 后端与 GW 的解耦设计

### 8.1 CW 是什么

CW（Creation Workshop）是游戏创作工具层，当前以 `internal/creation/` 包的形式存在于 `backend-v2` 中。
它负责：角色卡管理、世界书编辑、Preset 配置、LLM Profile 管理、素材库、游戏模板 CRUD。

**当前状态：** CW 和 GW 共享同一个 Go 服务（`cmd/server/main.go`），共享同一个数据库。
这在 MVP 阶段是合理的，但长期来看需要解耦。

### 8.2 为什么需要解耦

1. **用户群体不同：** GW 面向玩家（消费内容），CW 面向创作者（生产内容）。两者的认证需求、权限模型、UI 复杂度差异很大。
2. **部署需求不同：** GW 需要高可用、低延迟（玩家随时在线）；CW 可以接受更高延迟（创作是低频操作）。
3. **扩展方向不同：** GW 未来会加向量检索、记忆系统、实时 SSE；CW 未来会加协作编辑、版本控制、AI 辅助创作。

### 8.3 当前的耦合点（技术债务）

**已知耦合：**

| 耦合点 | 位置 | 影响 |
|--------|------|------|
| `CharacterCard.GameID` 暂用 `CharacterCard.ID` | `creation/api/routes.go:68` | 语义混乱，角色卡的世界书条目用角色卡 ID 作为 game_id，与 GameTemplate 的 game_id 不一致 |
| `creation/api` 直接用 `core/llm` 而非 `platform/provider` | `creation/api/routes.go` imports | 绕过了 Provider 注册表，无法享受多 Provider 路由和重试逻辑 |
| `WorldbookEntry` / `PresetEntry` 的 `game_id` 字段语义双关 | `models_creation.go` | 既可以是 `GameTemplate.ID`，也可以是 `CharacterCard.ID`，没有外键约束 |
| `LLMProfile` 存在 `core/db/models_creation.go` | 被 `platform/provider` import | `platform/` 层 import 了 `creation/` 的模型，方向反了（应该是 creation import platform）|
| `engine/api/routes.go` 仍有 564→437 行，混合了部分创作者调试路由 | `engine/api/routes.go` | 路由迁移未完全完成，创作者调试端点（`/sessions/:id/snapshot` 等）应归属 CW |

**未来解耦路径（不是近期工作）：**

```
当前：
  cmd/server/main.go
    ├── engine/api      ← GW 引擎
    ├── platform/play   ← GW 玩家层
    ├── social/         ← GW 社交层
    └── creation/api    ← CW 创作层（混在一起）

目标：
  cmd/gw-server/main.go   ← GW 服务（玩家 + 社交 + 引擎）
  cmd/cw-server/main.go   ← CW 服务（创作工具，独立部署可选）
  共享：core/db, core/llm, core/util（只读 GameTemplate）
```

**解耦的前置条件：**
1. 修复 `CharacterCard.GameID` 语义（明确区分"角色卡的世界书"和"游戏的世界书"）
2. 将 `LLMProfile` 从 `models_creation.go` 移到 `models_shared.go` 或独立包（因为 `platform/provider` 需要读它）
3. `creation/api` 改用 `platform/provider` 而非直接 `core/llm`

这三项是技术债务，不阻塞当前 Phase 1/2，但在 CW 独立部署之前必须完成。

### 8.4 CW 的设计雏形

CW 目前没有具体雏形，但可以从以下方向思考：

**CW 的核心能力（不依赖 GW 引擎）：**
- 角色卡 CRUD + 世界书编辑（已有）
- Preset/Regex 配置（已有）
- LLM Profile 管理（已有）
- 素材库（已有）
- **游戏模板版本控制**（未有，创作者需要"草稿/发布/回滚"）
- **协作编辑**（未有，多人共同创作一个游戏）
- **AI 辅助创作**（未有，用 LLM 帮助生成世界书条目、角色设定）

**CW 与 GW 的边界：**
- CW 写入 `GameTemplate`，GW 只读 `GameTemplate`
- CW 管理 `CharacterCard`/`WorldbookEntry`/`PresetEntry`，GW 通过 `game_id` 引用
- CW 不感知 `GameSession`/`Floor`/`Memory`（这些是 GW 引擎的私有状态）

---

## 九、当前技术债务清单

| 债务 | 位置 | 优先级 | 说明 |
|------|------|--------|------|
| `engine/api` 编译错误（`resolveSlot`/`applyGenParams` 未定义）| `engine_methods.go`, `game_loop.go` | 高 | 阻塞 `go build ./...`，需要尽快修复 |
| `CharacterCard.GameID` 语义混乱 | `creation/api/routes.go:68` | 中 | CW 解耦前置条件 |
| `LLMProfile` 在 `models_creation.go` 但被 `platform/provider` import | `models_creation.go` | 中 | 依赖方向反了，CW 解耦前置条件 |
| `creation/api` 直接用 `core/llm` 绕过 Provider 注册表 | `creation/api/routes.go` | 低 | 多 Provider 场景下会有问题 |
| `GET /play/sessions` 分页参数与 `util.ParsePage` 不一致 | `platform/play/handler.go` | 低 | 见 ENGINE-ROUTE-MIGRATION-PLAN 3-C |
| `forum.MigrateSQL()` 全文搜索触发器未手动执行 | 部署操作 | 低 | 首次部署后需手动执行一次 |
| `publicGameView` 无 `author_name`（无 users 表）| `platform/play/handler.go` | 低 | 待 `platform/user/` 包建立后补充 |

---

## 十、功能时序总结（更新）

```
当前（Phase 1/2）
  ├── 前端 Phase 1：页面重命名 + TopBar + GameDetailPage 修复
  ├── 前端 Phase 2：MyLibraryPage（A-15 后端已就绪）
  └── 后端：修复 engine 编译错误（resolveSlot/applyGenParams）

近期（Phase 3 前后）
  ├── 剪辑分享（Clip API + 论坛 ClipBlock）
  ├── .mvm 导出端点
  └── light 类型前端渲染（VNRenderer + StatusBar）

中期
  ├── A-11 常驻角色（platform/user/ 包）
  ├── 重大节点记忆（Sandbox OnChange 钩子 + 同步记忆写入）
  ├── Material 向量检索（pgvector，Material 量 > 100 时）
  └── CW 解耦前置条件（CharacterCard.GameID + LLMProfile 归属）

长期
  ├── CW 独立服务（cmd/cw-server）
  ├── rich 类型（音效/动画）
  ├── 玩家画像向量化
  └── 记忆可视化 UI（创作者调试面板）
```

---

## 十一、玩家修改世界书

### 11.1 可行性分析

**结论：可以实现，且不需要改动引擎层。**

当前世界书的读写路径：
- **创作者写**：`POST /api/create/templates/:id/lorebook`（`creation/api`，需登录）
- **玩家只读**：`GET /api/play/games/worldbook/:id`（`platform/play`，需 `allow_player_worldbook_view=true`）

玩家修改世界书的核心问题是**作用域**：玩家的修改不应该影响游戏本体（其他玩家看到的世界书），而应该只影响自己的 Session。

### 11.2 两种实现方案

**方案 A：Session 级世界书覆盖（推荐）**

在 `GameSession` 的 `Variables` JSONB 中存储玩家对世界书词条的覆盖：

```json
{
  "wb_overrides": {
    "<entry_id>": { "content": "玩家修改后的内容", "enabled": true }
  }
}
```

引擎在 `WorldbookNode` 组装时，先读游戏本体词条，再用 `session.Variables["wb_overrides"]` 覆盖对应词条的 `Content`/`Enabled`。

**优点：**
- 零新表，零 schema 变更
- 完全隔离（玩家修改只影响自己的 Session）
- 可随时重置（删除 `wb_overrides` 键即可）
- 与现有 `variable.Sandbox` 机制一致

**缺点：**
- `Variables` JSONB 会随覆盖内容增大（词条内容可能较长）
- 覆盖逻辑需要在 `WorldbookNode` 中增加一个合并步骤

**方案 B：Session 级世界书副本表**

新建 `SessionWorldbookOverride` 表（`session_id, entry_id, content, enabled`），引擎查询时 LEFT JOIN 覆盖。

**优点：** 结构清晰，可独立查询
**缺点：** 新增表和 JOIN，复杂度更高，MVP 阶段不值得

**当前决策：** 方案 A，待玩家世界书编辑功能进入开发计划时实现。

### 11.3 前端接口设计（备忘）

```
PATCH /api/play/sessions/:id/worldbook/:entry_id
  body: { content?: string, enabled?: bool }
  → 写入 session.Variables["wb_overrides"][entry_id]

DELETE /api/play/sessions/:id/worldbook/:entry_id
  → 删除 session.Variables["wb_overrides"][entry_id]（恢复默认）

GET /api/play/sessions/:id/worldbook
  → 返回游戏本体词条 + 玩家覆盖合并后的结果（标注哪些被覆盖）
```

**前置条件：** `GameTemplate.Config.allow_player_worldbook_edit = true`（创作者显式开启）

---

## 十二、Flow 评论区游戏化：宏指令驱动的富媒体评论

### 12.1 设计构想

**核心想法：** 评论区的每条 `Comment.Content` 支持宏指令，使评论不只是纯文本，而是可以携带渲染指令——图片、折叠块、角色立绘、变量展示、甚至简单的条件逻辑。

这本质上是把 WE 引擎已有的宏系统（`engine/macros/expand.go`）的一个**只读子集**暴露给评论区。

### 12.2 现有宏系统的能力

`engine/macros/expand.go` 当前支持：
- `{{char}}` / `{{user}}` / `{{persona}}` — 角色/玩家名替换
- `{{getvar::key}}` — 读取 Session 变量
- `{{time}}` / `{{date}}` — 时间

这些宏在 Pipeline 组装时展开，上下文是 `MacroContext`（含 Session 变量快照）。

### 12.3 评论宏的设计边界

**评论宏与引擎宏的关键区别：**

| 维度 | 引擎宏（Pipeline）| 评论宏（Flow）|
|------|------------------|--------------|
| 执行时机 | 每回合 LLM 调用前 | 评论渲染时（前端）|
| 上下文 | Session 变量 + 角色信息 | 评论作者的 Session 快照（可选）|
| 副作用 | 允许（写变量、调工具）| 禁止（只读渲染）|
| 安全要求 | 创作者可信 | 用户输入，必须沙箱化 |

**评论宏应该是只读渲染指令，不能有副作用。**

### 12.4 两层实现路径

**第一层：Markdown 扩展（近期可做，无需新系统）**

评论内容已经走 Goldmark（Markdown → HTML）+ Bluemonday（XSS 净化）管线（`forum/service.go`）。
可以在 Goldmark 上注册自定义扩展，支持简单的富媒体语法：

```markdown
![portrait:alice]          → 渲染角色立绘（从游戏素材库查找）
![clip:session_id:floor_3] → 嵌入游戏剪辑帧
> [!spoiler]               → 折叠块（剧透警告）
```

这些是**静态渲染**，不依赖 Session 状态，Bluemonday 白名单控制输出 HTML，安全可控。

**第二层：动态宏（中期，需要 social 层感知 Session）**

如果评论宏需要读取 Session 变量（如"展示我当前的好感度"），则需要：
1. 评论存储时附带 `session_snapshot`（发评论时的变量快照，JSONB）
2. 渲染时用快照展开 `{{getvar::affection}}` 等宏
3. **不需要 social 层注册宏系统**——宏展开逻辑复用 `engine/macros.Expand`，只是上下文从 Session 实时状态改为快照

```go
// 评论发布时
comment.SessionSnapshot = session.Variables  // 快照，不随 Session 变化

// 评论渲染时（后端或前端）
expanded := macros.Expand(comment.Content, macros.MacroContext{
    Variables: comment.SessionSnapshot,
    CharName:  game.CharName,
})
```

### 12.5 是否需要 social 层注册宏系统？

**不需要。**

原因：
1. 宏展开是**无状态的字符串变换**，不需要注册表或服务发现
2. `engine/macros.Expand` 已经是独立包，`social/comment` 可以直接 import（`social` → `engine/macros` 方向合法，因为 macros 包无 DB 依赖）
3. 宏的**定义**（支持哪些宏）由 `engine/macros` 包维护，不需要 social 层参与

**唯一需要 social 层做的事：** 在 `Comment` 模型上增加 `SessionSnapshot datatypes.JSON` 字段（可选，发评论时传入），渲染时传给 `macros.Expand`。

### 12.6 Flow 评论游戏化的完整愿景

把这个想法推到极致：**Flow 评论区本身就是一个 text 游戏的游玩记录展示层**。

```
玩家游玩 TextSessionPage
  → 某个精彩回合触发"分享这一刻"
  → 自动生成一条 Flow 评论，携带：
      - 当前楼层的 narrative 文本（作为评论正文）
      - session_snapshot（当前变量状态）
      - 可选：嵌入 ClipFrame（该楼层的渲染结果）
  → 其他玩家在评论区看到这条评论时：
      - 看到叙事文本 + 角色立绘（如果是 light 游戏）
      - 看到"好感度：85 / 路线：真实结局"（来自 session_snapshot 宏展开）
      - 点击"从这里开始游玩"→ fork 一个新 Session 从该楼层继续
```

这个愿景不需要 social 层有任何游戏逻辑——它只是把游戏引擎的输出（Floor 内容 + 变量快照）序列化为评论的一种特殊格式。

**实现依赖：**
- Clip API（剪辑分享，Phase 3）
- `Comment.SessionSnapshot` 字段（中期）
- 前端评论渲染支持宏展开（中期）

### 12.7 时序

```
近期（无需后端修改）
  └── Goldmark 自定义扩展：portrait / spoiler / clip 嵌入语法

中期（配合 Clip API）
  ├── Comment.SessionSnapshot 字段
  ├── 评论发布时可选传入 session_id（后端自动快照变量）
  └── 前端评论渲染支持 {{getvar::key}} 展开

长期（Flow 游戏化完整愿景）
  └── "分享这一刻"按钮 → 自动生成富媒体评论 + ClipFrame 嵌入
```
