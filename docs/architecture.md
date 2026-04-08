# WorkshopEngine — 架构决策文档

> 版本：2026-04-08
> 代码库：`backend-v2/`，语言：Go 1.22 + Gin + GORM + PostgreSQL

本文记录 WE 每项关键架构决策的**原因**，而不仅仅是"它是什么"。
每节先陈述问题，再给出选择，最后说明代价。
接手新开发者应以本文为第一阅读材料，再去读代码。

---

## 目录

1. [系统定位](#1-系统定位)
2. [包结构与三层分离](#2-包结构与三层分离)
3. [决策：PromptBlock Priority IR](#3-决策promtblock-priority-ir)
4. [决策：Floor / Page 双层结构](#4-决策floor--page-双层结构)
5. [决策：game-package.json 游戏包格式](#5-决策game-packagejson-游戏包格式)
6. [决策：宏展开在后端执行](#6-决策宏展开在后端执行)
7. [决策：WorldInfo 在服务端匹配](#7-决策worldinfo-在服务端匹配)
8. [决策：Session Fork 而不是 Branch ID](#8-决策session-fork-而不是-branch-id)
9. [决策：One-Shot LLM 纪律](#9-决策one-shot-llm-纪律)
10. [决策：变量五级沙箱](#10-决策变量五级沙箱)
11. [决策：工具系统可扩展注册表](#11-决策工具系统可扩展注册表)
12. [参照物：TavernHeadless 架构对比](#12-参照物tavernheadless-架构对比)
13. [演进路线与待解决问题](#13-演进路线与待解决问题)

---

## 1. 系统定位

```
SillyTavern (ST)     — 一体化桌面应用，浏览器端执行所有逻辑，面向高级玩家
TavernHeadless (TH)  — 无头 REST API，严格对齐 ST 格式，面向开发者
WorkshopEngine (WE)  — 无头 REST API 引擎 + 游戏发布平台，面向玩家和创作者
```

**WE 的核心差异化（TH 没有的）：**

| 特性 | 说明 |
|------|------|
| game-package.json | Preset + Worldbook + Regex + 素材一键打包/发布/导入 |
| VN 指令 | `[bg|...]`、`[sprite|...]`、`[bgm|...]`、`[choice|A|B|C]` — 后端解析、前端渲染 |
| 素材库 | Material + `search_material` 工具，AI 按标签检索注入上下文 |
| ScheduledTurn | 变量阈值触发 NPC 自主回合，Cooldown 持久化到 session.variables |
| 创作代理 | creation-agent：AI 对话式修改游戏规则（无需 UI） |
| 游戏发布平台 | 创作层（CW）+ 游玩层（GW）共用 WE 引擎，game-package 是桥梁 |

WE 不打算取代 TH。两者面向不同用户群体：TH 服务"给我一个对齐 ST 的 REST API 的开发者"；WE 服务"我想发布并运营一个 AI 文字游戏"。

---

## 2. 包结构与三层分离

```
backend-v2/internal/
├── core/           # 基础设施（无业务逻辑）
│   ├── db/         # PostgreSQL + GORM，全部 DB 模型定义于此
│   ├── llm/        # LLM 客户端（Chat / ChatStream / SSE）
│   └── tokenizer/  # Token 估算（BPE 兼容启发式）
├── platform/       # 横切关注点（无业务逻辑）
│   ├── auth/       # 账户中间件（X-Account-ID，Phase 4-B 迁移为 JWT）
│   └── provider/   # LLM Profile 注册表（slot 优先级解析）
├── engine/         # 引擎层：游戏会话、Prompt 组装、LLM 调用
│   ├── api/        # HTTP 处理器（game_loop.go、engine_methods.go）
│   ├── macros/     # 宏展开（{{char}} / {{user}} / {{getvar::key}} 等）
│   ├── memory/     # 记忆存取 + 异步整合 Worker
│   ├── parser/     # AI 响应解析（三层回退：XML → 编号列表 → fallback）
│   ├── pipeline/   # Prompt 组装流水线（PromptBlock IR）
│   ├── processor/  # Regex 后处理（user_input / ai_output / all）
│   ├── prompt_ir/  # 流水线上下文类型（ContextData / PromptBlock / ...）
│   ├── scheduled/  # ScheduledTurn 触发规则求值
│   ├── session/    # Session / Floor / Page 状态机
│   ├── tools/      # 工具注册表 + 内置工具 + ResourceToolProvider
│   ├── types/      # 共享消息类型
│   └── variable/   # 五层变量沙箱
├── creation/       # 创作层：模板、角色卡、世界书、素材等 CRUD
│   ├── api/        # HTTP 处理器
│   ├── asset/      # 素材上传与管理
│   ├── card/       # 角色卡 PNG 解析（ST V2/V3）
│   ├── lorebook/   # 世界书管理
│   └── template/   # 游戏模板 CRUD + 游戏包导入导出
└── social/         # 社交层（Phase 5-E 启动，当前为空）
```

**严格的单向依赖规则：**

```
core  ←  platform  ←  engine  ←  social
                   ←  creation
                   ←  integration
```

- `engine` 不能导入 `creation` 或 `social`
- `creation` 不能导入 `engine/api`
- `social` 只能通过 HTTP 调用 engine（不能导入 engine 内部包）

**为什么这样分？**

引擎（engine）是"游戏运行时"，创作（creation）是"资源管理"。如果 engine 导入 creation，就无法独立部署引擎、无法为创作层单独扩容，也无法把引擎打包给第三方使用。保持这个边界让 WE 引擎在未来可以作为独立 Go 模块分发。

---

## 3. 决策：PromptBlock Priority IR

**问题：** Prompt 组装涉及多个来源（System Prompt、世界书词条、记忆、历史对话、预设条目），插入位置和顺序复杂，每个 Pipeline 节点各自决定插入位置，难以统一调整。

**选择：** 每个 Pipeline 节点产出一组带 `Priority` 整数标签的 `PromptBlock`，Runner 最后统一按 Priority 全局排序，输出最终消息列表。

```
Priority 映射约定：
  0–99   = before_template（高优系统提示）
  100–199 = template（角色/游戏系统提示）
  200–299 = worldbook entries（世界书注入）
  300–399 = memories（记忆注入）
  400–499 = history（对话历史）
  500–599 = after_template（预设末尾条目）
  600+    = user turn
```

**理由：**
- 创作者只需理解一个数字（Priority），不需要理解 Pipeline 节点执行顺序
- 任何节点可以产出任意 Priority 的 Block，即 WorldbookNode 可以产出 Priority=50 的"钉住在最前面的词条"，无需特殊处理
- 新增节点只需实现 `Build(ctx) []PromptBlock` 接口，Runner 无需修改
- Token Budget 裁剪可以操作 Block 列表（按 Priority 排序后贪心保留），与组装逻辑完全解耦

**代价：**
- Priority 数字是约定，不是类型系统保证；如果两个节点用了重叠的 Priority 值，顺序依赖 slice 稳定性
- 调试时需要查看全部 Block 的 Priority 排序结果，才能看出"最终送给 LLM 的消息是什么顺序"
- 解决：`GET /sessions/:id/prompt-preview` 干运行端点，返回完整 Block 列表（含 Priority 和来源节点）

**参照：** TH 用 position 字段（`before_system`/`after_world`/`at_depth` 等枚举）描述插入位置，含义更直观但扩展性较弱——加一个新位置需要修改枚举定义。WE 的 Priority 数字更灵活，但需要文档约定来避免歧义。

---

## 4. 决策：Floor / Page 双层结构

**问题：** 对话历史需要支持两种操作：①重新生成（Swipe）——同一楼层的多个 AI 版本；②平行时间线（Fork）——从某个楼层往后的分叉。这是两个正交维度。

**选择：**
- **Floor**：对应一轮对话（user turn + assistant response），按 `seq_num` 排序，有状态机（`generating → committed / failed`）
- **Page**：同一 Floor 的多个 AI 响应版本（Swipe），同一时刻只有一个 `is_active=true` 的 Page
- **Fork**：复制某个 Session 到 `seq_num <= N` 的所有 Floor/Page，产生新 Session（新时间线），原 Session 不变

```
GameSession A                GameSession B (fork at floor 5)
  Floor 1 (committed)          Floor 1 (committed, 复制)
  Floor 2 (committed)          Floor 2 (committed, 复制)
  Floor 3 (committed)          Floor 3 (committed, 复制)
    ├── Page 1 (superseded)      └── Page 1 (active)
    └── Page 2 (active)
  Floor 4 (committed)          Floor 4 (新分叉，不同选择)
  Floor 5 (committed)
```

**理由：**
- Floor 是"时间维度"（故事进展），Page 是"版本维度"（Swipe 选择），分开建模避免数组嵌套
- Fork 产生新 Session 而不是新 Branch，每个 Session 都是完整的自洽时间线，更容易理解和 API 设计
- 前端渲染时，"当前故事线"= 取每个 Floor 的 active Page，逻辑简单

**代价：**
- Fork 会复制 Floor/Page 数据，存储量翻倍；但文字游戏的数据量很小（文本），不是瓶颈
- 如果想在同一个 Session 内管理多个时间线（"存档树"），需要 `branch_id` 字段（M14 路线图）；当前 Fork 是"另建一个 Session"，跨 Session 的关联靠 `fork_parent_id` 字段

---

## 5. 决策：game-package.json 游戏包格式

**问题：** 一个 AI 游戏由多个资源组成（角色、预设条目、世界书词条、Regex 规则、素材文件），如何让创作者一键分享、一键部署？

**选择：** 定义 `game-package.json` 格式，把一个游戏的所有资源序列化为单个 JSON 文件。导出时打包，导入时解包并重新建立关联。

```json
{
  "version": "1.0",
  "template": { ...GameTemplate fields },
  "character": { ...CharacterCard fields },
  "preset_entries": [...],
  "worldbook_entries": [...],
  "regex_rules": [...],
  "materials": [...],
  "llm_profile_hints": [...]
}
```

**理由：**
- 创作者只需要分享一个文件，接收方 `POST /templates/import` 即可复现完整游戏环境
- 与 ST 的角色卡 PNG（只含角色信息）不同，game-package 包含游戏运行所需的全部规则，是 WE 的核心差异化
- 版本字段允许未来格式演进时的迁移处理
- `llm_profile_hints` 是建议字段（不是强制），创作者可以指定推荐的模型规格，平台方可以忽略

**代价：**
- 如果游戏包含大量素材（图片、音频），单个 JSON 文件不适合；当前素材仅存储元数据（标签、描述），文件本体在素材库另存
- 导入时 ID 需要重新生成（不能直接用原 ID，可能冲突），外键关系需要 ID 映射表重建

---

## 6. 决策：宏展开在后端执行

**问题：** ST 宏系统（`{{char}}`、`{{user}}`、`{{getvar::key}}`、`{{time}}`）是在前端浏览器执行还是后端执行？

**选择：** 后端执行。宏在 Pipeline 组装阶段展开，展开后的文本才送给 LLM。

实现在 `internal/engine/macros/expand.go`：

```
MacroContext {CharName, UserName, PersonaName, Variables, Now}
Expand(text string, ctx MacroContext) string
```

Pipeline 中三个节点调用宏展开：
- `node_template.go` — 系统提示展开
- `node_preset.go` — 预设条目展开
- `node_worldbook.go` — 世界书词条内容展开

**理由：**
- WE 是无头引擎，LLM 调用在后端，Prompt 组装也在后端。把宏展开放在前端会导致前后端各维护一套 `{{char}}` 解析逻辑
- `{{getvar::key}}` 需要读取 Session 变量，这是服务端状态，前端无法直接访问（不应该暴露整个变量集合）
- 未来宏系统扩展（`{{setvar::key::val}}`、`{{roll::1d6}}`）在后端更容易实现副作用（写变量）

**代价：**
- 调试时创作者看到的是展开前的原文，送给 LLM 的是展开后的文本；`GET /sessions/:id/prompt-preview` 返回的是展开后的版本，可用于调试
- `{{time}}` 和 `{{date}}` 以服务器时间为准，不是用户本地时区（Phase 5-C 可加时区参数）

**宏展开的未来规划（Phase 5-C）：**

当前 `Expand()` 是单一函数，宏集合写死在代码中。Phase 5-C.1 将重构为注册表模式：

```go
// 目标接口（尚未实现）
var DefaultRegistry = NewRegistry()
DefaultRegistry.Register("char",       func(ctx MacroContext, _ string) string { return ctx.CharName })
DefaultRegistry.Register("getvar",     func(ctx MacroContext, key string) string { ... })
DefaultRegistry.Register("roll",       func(ctx MacroContext, expr string) string { ... })
// 第三方插件可调用 Register() 扩充宏集合
```

这样第三方可以不修改核心代码地注册自定义宏，与 ST 宏扩展生态对齐。

---

## 7. 决策：WorldInfo 在服务端匹配

**问题：** ST 的 WorldInfo（世界书）是在前端扫描对话历史匹配关键词，还是在后端执行？

**选择：** 完全在后端执行。`WorldbookNode.Build()` 扫描当前上下文，匹配关键词，把命中词条作为 PromptBlock 返回。

匹配逻辑（`internal/engine/pipeline/node_worldbook.go`）：
- 主关键词 + 次级关键词
- 逻辑门：AND_ANY / AND_ALL / NOT_ANY / NOT_ALL
- 正则模式 / 全词匹配
- 扫描深度（最近 N 条消息）
- 常驻词条（无需匹配，always active）
- **递归激活**：A 词条命中后，其内容可以触发 B 词条的关键词
- 变量门控：`var:key=value` 硬条件
- 互斥分组（`applyGroupCap`）：同一分组最多保留 N 条
- Token Budget（`applyTokenBudget`）：按 Priority 贪心保留，Constant 词条钉住不剪

**理由：**
- 所有上下文（对话历史、变量、记忆）都在服务端，后端匹配无需往返 HTTP
- 前端不需要实现复杂的世界书匹配逻辑，可以保持薄层渲染
- Token Budget 计算需要知道整体 Token 预算，这由 LLM Profile 管理，也在服务端

**代价：**
- 匹配结果不透明：创作者不知道当前哪些词条被激活了
- 解决：`GET /sessions/:id/floors/:fid/snapshot` 返回 `ActivatedWorldbookIDs`，`GET /sessions/:id/prompt-preview` 的 `debugWorldbook=true` 返回触发详情

---

## 8. 决策：Session Fork 而不是 Branch ID

**问题：** 玩家想在第 20 楼"存档并尝试不同选择"，如何实现平行时间线？

**方案 A（Branch ID）：** Floor 添加 `branch_id` 字段，同一 Session 内维护多个分支，用树形结构表示时间线。

**方案 B（Session Fork，当前选择）：** `POST /sessions/:id/fork?at=20` 复制该 Session 前 20 层 Floor/Page 到新 Session，返回新 Session ID。玩家在新 Session 中继续游玩。

**选择了方案 B（Session Fork），理由：**
- API 简单：Session 是一个完整的、自洽的对象，所有操作都针对 Session ID，无需引入 `branch_id` 参数
- 前端无需理解树形结构，每个 Session 都是线性的 Floor 序列
- "存档点"是一种高频用法（玩家想保存节点），Fork 就是语义最自然的操作
- 跨 Session 关联靠 `fork_parent_id`，UI 可以据此显示"派生自 Session X"

**方案 A 的价值（M14 路线图）：** 当玩家想在同一界面管理"时间线树"（类似游戏存档管理器），需要 Branch ID。但这是 Phase 3 后期功能，当前 Fork 已满足"多时间线"的核心需求。

---

## 9. 决策：One-Shot LLM 纪律

**问题：** 一个回合需要多个分析步骤（风格分析、叙事生成、一致性校验），是多次调用 LLM 还是用 Tool Call Agentic Loop？

**选择：** 非生成性任务（Director 分析、Verifier 校验）使用独立 LLM Slot（廉价小模型一次调用），主叙事生成用主 LLM 槽一次调用。对创作者自定义工具使用 Agentic Loop（最多 5 轮），但限制在主生成阶段。

```
一个回合的 LLM 调用序列：
  1. Director 槽（可选）：上下文分析 → 结果插入主消息首位（一次调用）
  2. 主叙事槽：Prompt 组装 → LLM 调用（含 Agentic Loop，最多 5 轮工具调用）
  3. Verifier 槽（可选）：一致性校验 → 结果写入 PromptSnapshot（一次调用）
```

**理由：**
- 廉价小模型做分析，贵的大模型做叙事，降低每回合成本
- Director 失败时静默跳过（不阻断回合），Verifier 失败时只标记 PromptSnapshot，不阻断
- Agentic Loop 上限 5 轮防止无限工具调用循环消耗 Token

**代价：**
- 每回合可能触发 2-3 次 LLM 调用（Director + 主叙事 + Verifier），延迟比单次高
- Director 和 Verifier 是可选槽，不激活时回退为单次调用

---

## 10. 决策：变量五级沙箱

**问题：** 变量（游戏状态）需要在哪个级别持久化？Page 级？Session 级？

**选择：** 五级读取（Global → Chat → Floor → Branch → Page），写入始终到 Page 层，CommitTurn 时展平到 Chat 层。

```
读取优先级（高 → 低）：
  Page 层（当前回合草稿）
  Branch 层（分支局部状态）
  Floor 层（楼层局部状态）
  Chat 层（会话持久状态）← CommitTurn 时 Page 展平到这里
  Global 层（游戏全局默认值）
```

**理由：**
- 生成阶段（generating）的变量变更不应该立即可见，需要在 CommitTurn 后才"生效"
- 回退（RegenTurn）时 Page 层丢弃，变量自动回到上一次 CommitTurn 后的状态
- Branch 层让 Session Fork 的两条时间线有独立的变量状态

**代价：**
- 五层查找比单层 map 复杂，调试时需要 `GET /sessions/:id/variables` 查看当前展平后的快照
- Branch 层当前仅为 Fork 服务；M14（Session 内分支）实现后 Branch 层才完全用上

---

## 11. 决策：工具系统可扩展注册表

**问题：** LLM 可以调用哪些工具？只有内置工具（get/set variable、search memory），还是允许创作者自定义？

**选择：** `tools.Registry` 注册表模式，支持三类工具：
1. **内置工具**：`get_variable`、`set_variable`、`search_memory`、`search_material`（硬编码注册）
2. **资源工具**（ResourceToolProvider）：14 个 AI 可操作资源工具，读写 worldbook/preset/memory/material
3. **Preset Tool**：创作者在模板中定义的 HTTP 回调工具（`preset:*` / `preset:<name>` 动态加载）

```go
// 注册接口
type Tool interface {
    Definition() LLMToolDefinition
    Execute(ctx ExecuteContext) (any, error)
}

registry.Register(tool)                 // 静态注册
registry.RegisterProvider(provider)     // 动态提供者（按回合加载）
```

**理由：**
- 内置工具满足游戏内状态管理（读写变量、查记忆、查素材）
- Preset Tool 让创作者无需修改后端代码就能给 AI 添加自定义能力（webhook、外部 API 调用）
- 资源工具让 creation-agent（AI 辅助创作）可以通过对话修改游戏资源
- ReplaySafety 等级（`safe` / `confirm_on_replay` / `never_auto_replay`）控制自动重放风险

**代价：**
- Preset Tool 是 HTTP 回调，网络延迟不可控，需要创作者自己保证可用性
- 工具执行记录持久化到 `tool_execution_records` 表，高频工具调用会增加 DB 写入量

---

## 12. 参照物：TavernHeadless 架构对比

WE 在设计时深度参考了 TavernHeadless（TH）的架构。理解两者的差异有助于接手开发者快速判断"这个功能应该怎么做"。

### 12.1 三层分离对比

| 层 | TH | WE |
|----|----|----|
| **Prompt Layer** | PromptGraph DAG compiler + PromptIR assembler | PromptBlock Priority IR（单一路径，无 DAG）|
| **Runtime Layer** | TurnOrchestrator + floor_run_state 精细状态机 + RuntimeRevisionGuard | game_loop.go（单 goroutine per turn，无精细状态机）|
| **State Layer** | 五层变量 + MemoryV2（micro/macro/fact）+ Branch-scoped vars | 五层变量（简化版）+ Memory（单层 fact）|

**WE 当前简化合理的原因：** WE 是单用户游玩场景，无并发 turn；TH 面向多开发者集成，需要严格的并发保护和精细状态追踪。

### 12.2 WE 独有（TH 没有）

| WE 特性 | 说明 |
|---------|------|
| PromptBlock Priority IR | 全局优先级排序，比 TH position 枚举更灵活 |
| game-package.json | 完整游戏打包格式 |
| VN 指令解析 | `[bg|...]`、`[sprite|...]` 等视觉小说指令 |
| 素材库 + search_material | 游戏内文本素材检索工具 |
| ScheduledTurn | 变量阈值触发 NPC 自主回合 |
| creation-agent | AI 对话式修改游戏资源 |

### 12.3 TH 有、WE 尚未实现

| TH 特性 | WE 状态 | 路线图 |
|---------|---------|--------|
| floor_run_state 精细状态机 | 仅 SSE phase 事件 | M17 |
| API Key AES-256-GCM 加密 | 明文存储 | M15 |
| JWT Auth | X-Account-ID 简化 | M16 |
| runtime_job 持久化 | goroutine（进程重启丢失）| M19 |
| 双层记忆（micro/macro）| 单层 fact | M18 |
| Session 内分支（branch_id）| Fork 方案代替 | M14 |

### 12.4 文档哲学借鉴

TH 的文档质量是业内标杆。WE 借鉴其三个原则：

**① 决策记录（ADR 轻量版）**：每项重要设计不只写"是什么"，还要写"为什么"和"代价是什么"（本文的格式来源于此）。

**② PROGRESS.md 里程碑追踪**：每个里程碑明确列出完成内容，新人第一眼就知道系统有哪些能力。`backend-v2/PROGRESS.md` 是 WE 对应的文档。

**③ 代码与文档同步更新**：每次功能完成时，同步更新 PROGRESS.md 和 architecture.md，而不是等"有时间了再补文档"。

---

## 13. 演进路线与待解决问题

### 13.1 已知设计债务

| 债务 | 位置 | 影响 | 计划 |
|------|------|------|------|
| DB 模型定义在 `core/db/`，与 engine/creation 业务逻辑分离不彻底 | `internal/core/db/models.go` | 修改模型需要改 core 包，导致所有包重新编译 | Phase 4 重构时按领域分拆 |
| `engine/api/game_loop.go` 既组装 Prompt 又调用 LLM，单文件职责偏重 | `internal/engine/api/game_loop.go` | 不影响功能，测试覆盖困难 | Phase 4 拆分为 Runner + API 两层 |
| 异步 Memory Worker 用 goroutine，进程重启丢失 | `internal/engine/memory/worker.go` | 低概率丢失整合任务 | M19 runtime_job 表 |
| `X-Account-ID` 认证无加密，可伪造 | `internal/platform/auth/` | 内测可接受，上线前必须修复 | M16 JWT Auth |

### 13.2 扩展点

新增 Pipeline 节点：实现 `Build(ctx ContextData) []PromptBlock` 并在 `RunPipeline()` 中注册即可，其他节点不受影响。

新增工具：实现 `Tool` 接口（`Definition()` + `Execute()`），调用 `registry.Register()` 注册，无需修改 Agentic Loop。

新增宏（Phase 5-C.1 后）：调用 `macros.DefaultRegistry.Register(name, handler)` 注册，无需修改 `Expand()`。

新增 API 路由：实现 Handler 函数，在 `cmd/server/main.go` 挂载到对应命名空间（`/api/v2/play` 或 `/api/v2/create`）。

### 13.3 官方集成包计划（Phase 5-B）

WE 引擎是纯 Go HTTP 服务，完全可以脱离自己的前端运行。Phase 5-B 计划发布官方集成包：

```
packages/
├── @gw/sdk             # TypeScript HTTP + SSE 客户端（镜像 @tavern/sdk 设计）
└── @gw/play-helpers    # 纯函数状态工具（镜像 @tavern/client-helpers 设计）
                        # buildMessageTimeline / reduceGameStream / applyVNDirectives
```

目标：第三方开发者可以用 `@gw/sdk` + `@gw/play-helpers` 在任意前端框架（Vue/React/Svelte）上构建自己的游戏界面，WE 引擎作为无头后端提供 REST/SSE API。
