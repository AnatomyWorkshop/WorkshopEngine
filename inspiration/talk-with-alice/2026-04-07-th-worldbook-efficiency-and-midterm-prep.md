# TH 诞生逻辑、世界书效率机制与 WE 中期工作准备

> 日期：2026-04-07
> 接续：2026-04-06-complexity-multiplayer-th-author.md
> 触发问题：为什么 TH 面对大世界书响应快且 token 不多？TH 有什么潜力取代酒馆？还有哪些设计没有对比到？

---

## 一、TH/ST 面对大世界书响应快的真正原因

### 1.1 问题的起源：异世界和平测试首回合 147k tokens

上一个测试显示，直接导入的转换包在第一回合消耗了 147k tokens，撑爆了 DeepSeek 131k 的上下文限制。但同一张卡在 ST 里却能正常游玩，响应速度也合理。这不是 ST 的 LLM 上下文更大，而是 ST（以及它所依赖的扩展）做了 **LLM 永远看不到的预处理**：

```
ST 的真实执行路径：

原始世界书词条（EJS 源码）
       ↓
   EJS 渲染器（浏览器端，在 JS-Slash-Runner 里）
       ↓
[stat_data.世界状态.当前章节]  →  "第三章"（已求值）
[stat_data.角色.莉莉亚.好感度]  →  "25"（已求值）
  ↓                                    ↓
只有激活章节的纲要被注入         不需要的章节代码被丢弃
       ↓
LLM 实际看到的 tokens：精简后的静态内容
```

**异世界和平的核心设计假设：**

| ST 扩展 | 它承担了什么 |
|---------|-------------|
| JS-Slash-Runner（MVU Beta）| 维护变量树 `stat_data`；`getvar()` 提取值；EJS 剧情词条里的 `_.includes(getvar('stat_data.世界状态.已经历剧情[0]') || [], "xxx")` 在渲染时被求值，不满足条件的分支整块被剔除，LLM 从未看到 |
| ST-Prompt-Template | EJS 引擎；将 `<%= allCharacterNames.join(', ') %>` 渲染为 `莉莉亚, 露娜玛丽亚, ...`；这个列表原来是 9371 chars，渲染后只有一两行 |

换句话说：**异世界和平这张卡的真正上下文不是 147k tokens，而是 ST 扩展完成渲染后大概 15-30k tokens。** 那张卡的作者把大量内容写在了 EJS 代码里，不是静态文本——EJS 是"运行时被删掉的代码"，而不是"要发给 LLM 的内容"。

我们的 WE 转换脚本没有 EJS 运行时，只能 strip 掉 EJS 代码块但保留所有静态文本，所以把原本"应该被运行时处理掉的"内容全部塞给了 LLM。

---

### 1.2 ST 和 TH 的世界书真实机制

#### scan_depth 不是"扫描条数"，是"扫描窗口"

TH 和 ST 的 `scan_depth` 含义相同：
- `scan_depth = 0`：扫描所有历史消息（全量）
- `scan_depth = 3`：只扫描最近 3 条消息

**这张卡的词条 scan_depth 全部为 0，意味着每个词条都扫描全量历史。**  
一旦某个角色名出现在历史里任何地方，她的角色描述词条就会一直被激活。在第一回合（只有"开始游戏"这一条消息）时，"开始游戏"这几个字不包含任何角色名，所以本来不应该触发任何角色描述词条——问题出在 `first_mes`。

ST 的 `first_mes` 是作为**角色的第一句话**直接显示给用户的，它进入对话历史，而这段开场白包含了大量角色名（莉莉亚、露娜玛丽亚等），于是所有角色描述词条都被触发了。WE 把 `first_mes` 存在 `_meta` 里没有注入，但从 prompt-preview 的输出看，375k chars 的内容说明有其他内容触发了所有词条——最可能是 `initial_variables` 里存的角色树被序列化后注入了系统提示，包含了所有角色名。

#### 真正的 Token 效率来自三个机制

```
1. Group Cap（互斥分组）
   同一组的词条（比如"当前场景角色描述"组）最多激活 N 条
   → 角色 50 个，每次只有 3 个在场，group_cap=3 只注入 3 人的描述

2. Position at_depth（深度注入）
   词条不必注入到系统提示最顶部，可以放在对话历史的特定深度
   → 只影响 LLM 需要时"回顾"的位置，不影响其他 token 预算
   → WE 目前支持 before_template / after_template，但 ST/TH 还有 at_depth

3. Token Budget（硬预算）
   TH 有全局 worldbook_token_budget：所有词条总计不超过 N tokens
   → 超出预算的词条按优先级排序，低优先级直接丢弃
   → WE 的 worldbook_group_cap 目前是按"条数"裁剪，不是按 token 裁剪
```

ST/TH 在大世界书里响应合理的核心原因：**卡片作者预先设计了合理的 group 分组和 scan_depth，加上 EJS 渲染提前消化了动态内容，LLM 最终看到的是已经裁剪好的静态提示词。**

异世界和平的问题不在 WE 也不在 ST，在于这张卡是为一个非常特定的���展生态写的，移植时必须复现那个扩展生态的处理逻辑。

---

## 二、TH 取代酒馆的潜力与逻辑

### 2.1 TH 诞生的背景

ST（SillyTavern）的架构本质上是一个**前端重型的单体应用**：

```
ST 的真实架构：

浏览器 / Electron
├── UI（聊天界面）
├── Extension 运行时（JS-Slash-Runner、ST-Prompt-Template 等）
├── 正则/变量处理（浏览器端 JavaScript）
└── HTTP 请求代理 → Node.js Express 后端
    ├── 文件系统（角色卡 PNG、世界书 JSON）
    └── API Key 转发 → 第三方 LLM API
```

ST 的后端非常薄——几乎所有的智能都在浏览器里运行，包括 EJS 渲染、变量处理、正则执行。这带来了几个问题：

1. **无法服务器部署**：需要浏览器会话，不能作为无状态 API
2. **扩展冲突**：十几个扩展在同一个 JS 运行时里互相污染
3. **状态不可预期**：浏览器刷新、扩展更新都可能破坏游戏状态
4. **多用户不可能**：每个用户需要自己的 ST 实例

**TH 的诞生逻辑**：把 ST 的前端智能迁移到服务器，用一个干净的 REST API 复刻 ST 的全部行为，让"运行 ST 卡"变成一个无依赖的 API 调用。

```
TH 的架构：

客户端（任意）
└── POST /sessions/:id/turn  →  TH Fastify 服务
    ├── EJS 渲染（服务端）
    ├── 世界书扫描（服务端）
    ├── 变量处理（服务端）
    ├── 记忆整合（后台 Job）
    └── LLM API 调用
```

### 2.2 TH 能用更少 token 实现更丰富功能的核心设计

**更少 token：Token Budget 管理**

TH 有明确的 token 预算分配机制，每个注入位置（Worldbook / Memory / History / Character）都有预算上限，超出预算的内容按优先级裁剪。这是 ST 没有的硬约束，WE 目前也只有 GroupCap（按条数，不按 token）。

**更丰富功能：Director + Verifier 双槽**

TH 在一次回合里最多运行 3 个 LLM：
- **Director**（廉价，pre-turn）：分析上下文，决定"本回合应该激活哪些世界书词条"
- **Narrator**（主力，生成叙事）
- **Verifier**（廉价，post-turn）：校验输出一致性

Director 的价值：用一个小模型（Gemini Flash / GPT-4o mini）分析当前场景，告诉主力模型应该关注什么——这等价于"动态世界书激活"，不需要 EJS，靠语义理解代替关键词匹配。

**实现路径（WE 已有基础）：**
- WE 已有 Director 和 Verifier goroutine
- 差距在于：TH 的 Director 输出可以**影响世界书词条的激活**（Director 说"当前场景是战斗"，引擎补充激活战斗相关词条）
- WE 的 Director 目前只是把分析结果注入 Prompt 顶部，不影响 WorldbookEntry 选择

### 2.3 TH 取代酒馆的可能性

| 维度 | ST 的局限 | TH 的方案 |
|------|-----------|-----------|
| 部署 | 只能单机浏览器 | 任意服务器，API 化 |
| 多用户 | 每人装独立 ST | 多账户隔离，共享实例 |
| 扩展稳定性 | 扩展互相污染，易崩 | 扩展逻辑编译进服务端，稳定 |
| 卡片兼容 | ST V2/V3 格式原生支持 | TH adapter 层 1:1 复刻 ST 行为 |
| 移动端 | 无官方手机版 | 任意 HTTP 客户端即可游玩 |
| MCP 工具集成 | 依赖 JS 扩展，格式多样 | 标准 MCP 协议，接入任意工具 |

TH 的真正价值：**把 ST 生态的全部卡片迁移到服务器，不再依赖用户自装扩展**。如果 TH 对 JS-Slash-Runner 的行为做了完整服务端复刻（包括 EJS 渲染和 `getvar()`），那么任何 ST 卡可以无修改地在 TH 上运行——这是 WE 明确不做的方向，但也是 TH 最重要的差异化。

---

## 三、一个有能力的开发者会问的问题

### 3.1 架构层面

**Q：TH 的 scan_depth=0（全量扫描）在大型对话后会不会变慢？**

回答：是潜在瓶颈。TH 的扫描是纯字符串匹配（正则 + 关键词），O(entries × history_length)。实践中 ST/TH 用两个缓解手段：
1. `scan_depth > 0` 截断历史深度
2. 词条激活后，下一回合如果历史未变，复用上一次的激活结果（TH 有 `promptSnapshotCache`，但不跨回合持久化）

WE 的情况：同样是 O(n×m) 扫描，无缓存。优化路径：按词条的 hash 和历史段 hash 做 LRU 缓存，但目前没必要——大多数游戏词条数不超过 200。

**Q：TH 的 Background Job Runtime 在进程重启后如何恢复？**

TH 把 Job 状态写入 `runtime_job` SQLite 表（状态：queued → leased → done / failed / dead）。进程重启时，对所有 `leased` 状态超过 `lease_timeout` 的 Job 执行 "lease expiry"：重置为 queued 重新处理。
WE 的 goroutine 方案：进程重启会丢失未完成的内存整合任务。这是 WE 的一个已知弱点，在 Phase 4 的 Background Job Runtime 里修复。

**Q：GenerationCoordinator 的 reject vs queue 语义区别是什么？**

这是 TH 里还没完全研究到的一个机制。
- **reject**（拒绝模式）：同一个 session 的第二个并发生成请求直接返回 409 Conflict。前端负责重试。
- **queue**（排队模式）：第二个请求进队列等待，第一个完成后自动开始第二个。

reject 更简单，适合"玩家不会同时点两次"的单人游戏。queue 适合"快速点击多次"的竞速场景或观众投票推进的多人游戏。WE 目前既无 reject 也无 queue，两个并发 PlayTurn 会产生数据竞争（两个 goroutine 同时写同一个 session 的 Floor）。

**Q：TH 的 `at_depth` 注入位置如何与传统 position 兼容？**

`at_depth=N` 把词条注入在对话历史的第 N 个来回之前（从最新开始倒数）。这意味着词条夹在历史消息之间，而不是放在 System 里。这对某些叙事逻辑很重要：
- 全局规则放 `before_template`（System 顶）
- 当前场景描述放 `at_depth=1`（就在上一条消息前）
- 背景故事放 `before_template` 但低优先级

WE 目前只支持 `before_template` / `after_template`，不支持 `at_depth`。这是 WE 和 TH 在注入位置上的最大差距，对于"让当前场景描述新鲜地出现在对话末端附近"很重要。

### 3.2 设计层面

**Q：TH 的 lorebook_overrides 机制是什么？**

ST 支持每个角色卡携带独立的世界书（character book），并且可以设置这个世界书是否"覆盖"全局世界书的某些词条（同名词条以角色卡版本为准）。TH 通过 `lorebook_overrides` 表把这个语义带到了服务端。

WE 没有这个概念：WE 的世界书全部属于 `game_id`，角色卡的世界书在导入时被合并进去，不区分"来源"。如果需要精确复刻某些 ST 卡的行为，这个差距会显现。

**Q：Branch Governance（分支治理规则）是什么？**

TH M13 的 Session 内分支（`floor.branch_id`）实现后，需要管理分支的生命周期：
- 哪些分支可以合并？（`merge` 操作）
- 哪些分支在 N 天不活动后自动归档？（`prune` 策略）
- 分支之间的父子关系如何呈现？（`branch_graph`）

这套规则合称 Branch Governance。WE 的 3-G 计划了分支功能，但没有治理规则——届时需要简化版的 prune 策略防止分支数无限增长。

**Q：双层摘要（compact + extended）的意义？**

TH 的 Memory V2 计划了两类摘要：
- **compact**：极短（2-3 句），每回合强制注入，不受 token 预算影响。"目前在魔法学院，进行中的是入学任务。"
- **extended**：详细（20-50 句），按重要性注入，受 token 预算控制。

这解决了"最重要的叙事状态永不丢失，但细节随着历史增长自动裁剪"的问题。WE 目前是单层（无 compact/extended 区分），长期对话后全部记忆会因为 token 预算挤掉一些重要状态。这是 WE Phase 4 或 5 的工作。

### 3.3 运营层面

**Q：TH 如何处理 API Key 泄露的风险？**

TH M18 实现了 `LLM Profile Vault`：
- Master Key 来自环境变量（`LLM_VAULT_MASTER_KEY`），不存入 DB
- API Key 用 AES-256-GCM + PBKDF2 加密存储
- 读取接口只返回 `sk-ab**...1234`（掩码）
- Master Key 轮换需要重新加密所有 Profile

WE 目前明文存 API Key，公网部署前必须参考 TH M18。

**Q：TH 的 OpenAPI 文档和 TypeScript SDK 是必要的吗？**

对于 SaaS/公开 API 是必要的。TH 的 `@tavern/sdk` 让前端开发者不用写 `fetch` 样板代码。WE 在 Phase 4 用 `swaggo` 自动从注释生成文档，SDK 等 API 接口稳定后用 openapi-generator 生成。

---

## 四、TH 设想中还没有对比和阅读到的部分

通过已有的 TH 分析（st-comparison.md、implementation-plan.md），以下 TH 设计点**还没有被深入研究**：

### 4.1 Mutation Runtime

TH 不仅有 Background Job Runtime（异步任务），还有 **Mutation Runtime**（同步状态变更语义）。

Mutation Runtime 的核心思想：每次状态变更（变量写入、记忆更新、楼层 commit）不直接写 DB，而是先写入 `mutation_queue`，然后顺序执行。每条变更记录：操作类型、目标、值、触发原因。

这带来的能力：
- `confirm_on_replay`：重放时遇到非安全变更自动暂停，让用户确认
- 完整的变更审计日志（谁在什么 Floor 改了什么变量）
- 容易实现"撤销"（删除 mutation_queue 里的记录，重放前面的状态）

**WE 明确不做 Mutation Runtime**（见 implementation-plan.md"明确不做"节）。理由是复杂度过高，且变量 Sandbox 的 CommitTurn 模式（整回合一次性提交）已经提供了足够的原子性。

但 Mutation Runtime 的审计日志思路值得借鉴——至少把每个 Floor 的变量变更记录在 PromptSnapshot 里，方便调试时追溯"第几回合改了这个变量"。

### 4.2 Memory Scope Compaction

TH 的记忆压缩分两个层次：
1. **Memory Consolidation**（WE 已有）：定期把近期 Floor 摘要归纳为 Memory 条目
2. **Memory Scope Compaction**（WE 没有）：当 Memory 条目总数超过阈值，把旧的 Memory 进一步"压缩"为更高层的摘要

想象一个长期游玩的故事：
```
Floors 1-50  → Memory 摘要（10 条，已归纳）
Floors 51-100 → Memory 摘要（10 条，已归纳）
这 20 条 Memory → Memory Scope Compact → 2 条高层摘要
```

这解决了"长时间游玩后 Memory 本身也变得太多"的问题。WE 的衰减排序（指数半衰期）是另一种解法——老 Memory 自然衰减到不注入，但不物理删除。TH 的 Compact 会真正删除/合并旧 Memory，节省 DB 空间和检索时间。

### 4.3 Character Injection 与 Persona Blending

TH 有明确的角色注入机制：把角色卡的 `description`、`personality`、`scenario` 和用户的 `persona` 按照 ST 的注入格式（`{{char}} is...`）拼接注入 System Prompt。

WE 目前的角色注入：角色卡被解析为 `CharacterCard`，但 `system_prompt_template` 是 `GameTemplate` 里的一个字段，角色卡内容需要手动写入 `WorldbookEntry` 或 `PresetEntry`，没有自动的"角色卡内容 → System Prompt"管道。

**对于 WE 的 rich 类型游戏这没问题**（设计者手动组织内容），但对于希望兼容 ST V3 卡的场景，这个管道缺失会让导入后的卡片不能自动注入角色描述。

### 4.4 Floor Run State（已知但未深入）

TH 的 `floor_run_state` 表跟踪每个 Floor 的生成管线状态（10+ 个阶段），支持：
- 前端展示"当前在做什么"（"正在整合记忆…"）
- 断点续传（进程重启后从上次的阶段继续）
- 超时诊断（哪个阶段卡住了）

WE 的 PromptSnapshot 记录了事后结果，但没有实时状态追踪。对于长时间运行的 Director/Memory 阶段，前端目前只能显示"生成中…"而无法告知具体阶段。

这是一个**调试体验**功能，对开发期的价值很大，对玩家体验也有帮助（明确知道 AI 在做什么）。

### 4.5 Chat Transfer Job 的完整格式

TH 有一套 `ChatTransferJob` 用于把 ST 格式的 `.jsonl` 对话历史导入为 TH 的 Floor/Page 结构，以及反向导出。这涉及到：
- ST 的消息格式（`role: user/assistant`，`name`，`extra` 字段）
- ST 的世界书触发记录（ST 在消息里附加 `worldInfoAfter`/`worldInfoBefore`）
- ST 的 swipe 历史（每个楼层的多个备选回答）

WE 没有 ST 格式导入（只有角色卡 PNG 导入和游戏包导入），如果要做 ST → WE 对话历史迁移，需要实现一个类似 ChatTransferJob 的解析器。

---

## 五、中期工作准备

### 5.1 当前已完成的 Phase 3 工作

| 编号 | 功能 | 状态 |
|------|------|------|
| 3-A | Memory Edge 关系图 | ✅ 完成 |
| 3-B | LLM 模型发现 + 连通性测试 | ✅ 完成 |
| 3-C | Worldbook 互斥分组（Group + GroupWeight + GroupCap）| ✅ 完成 |
| 3-D | Worldbook 变量门控（`var:key=value`）| ✅ 完成 |

### 5.2 Phase 3 剩余工作（按优先级）

**3-E：Memory 分阶段标签（stage_tags）**

改动量最小的一项（~50 行），价值高：长叙事游戏的"调查阶段记忆"不应该带入"高潮阶段"。
核心改动：`Memory` 表新增 `stage_tags JSONB`，`GetForInjection` 按 `ctx.Variables["game_stage"]` 过滤。

**3-F：边界归档 API**

`POST /sessions/:id/archive` — 用 Memory 槽模型生成结构化摘要，写入高重要性 Memory，更新 `session.status = "archived"`。供 GW 论坛分享游记用。

**3-G：Session 内分支（branch_id）**

最大改动量，涉及 Floor 读写全链路。暂排最后，先完成 3-E 和 3-F。

### 5.3 直接影响 WE 游戏体验的两个补丁（应插队到 3-E 之前）

这两个不在原计划里，但来自异世界和平测试暴露的问题：

**补丁 A：worldbook_token_budget（按 token 裁剪）**

现有 `worldbook_group_cap` 按条数裁剪。需要增加按 token 总量裁剪的能力：
`GameTemplate.Config.worldbook_token_budget`（默认 8000）— 世界书注入总 token 上限，超出后按优先级裁剪，保留高优先级词条。

改动位置：`node_worldbook.go` 的 `applyGroupCap` 后新增 `applyTokenBudget`。
约 30 行，无 DB 变更（Config 字段加一项即可）。

**补丁 B：initial_variables 不注入为 Prompt 内容**

测试发现 `initial_variables` 里的 `角色` 树（包含所有角色名）似乎被序列化为文本注入了提示词，触发了所有角色描述词条。需要确认：变量沙箱的初始化只写入变量存储，不应该出现在 `buildScanText` 里。

检查位置：`node_worldbook.go` 的 `buildScanText` 函数，以及 `ctx.Variables` 的数据来源。

### 5.4 对接下来工作的总结建议

| 优先级 | 工作 | 理由 |
|--------|------|------|
| ★★★ | 补丁 A：worldbook_token_budget | 当前无法安全运行大世界书游戏 |
| ★★★ | 补丁 B：确认变量不注入扫描文本 | 同上，影响所有使用初始变量的游戏 |
| ★★☆ | 3-E：Memory 分阶段标签 | 小改动，高价值，长叙事必需 |
| ★★☆ | 3-F：边界归档 API | GW 论坛集成的基础 |
| ★☆☆ | 3-G：Session 内分支 | 大改动，暂缓 |
| ★☆☆ | Director → 世界书激活控制 | 让 Director 输出影响词条选择，用语义替代关键词匹配 |
| ★☆☆ | at_depth 注入位置 | 大世界书场景需要，目前用 before/after 勉强替代 |

---

## 附：TH 架构的一句话总结

TH 的核心思想是：**酒馆的扩展生态太好了，值得被完整复刻到服务器。** 它的每一个设计决策都在问"如果 ST 的这个扩展是服务端原生的，它该怎么设计？"——而 WE 问的是另一个问题："如果从零开始为游戏创作设计一个引擎，它该怎么设计？" 两个问题都是对的，答案必然不同。
