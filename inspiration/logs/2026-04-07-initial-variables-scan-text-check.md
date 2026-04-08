# 补丁 3-D.2：Initial Variables 不注入扫描文本（验证通过）

> 日期：2026-04-07
> 状态：✅ 无需改动，验证通过
> 关联计划：implementation-plan.md § 3-D.2

---

## 背景

3-D.1 完成后需确认：`initial_variables`（游戏模板的初始变量）和 `first_mes`（角色卡首条消息）
是否会被注入到 Worldbook 的扫描文本中，导致词条被意外触发。

## 结论

**当前实现已经干净，无需任何改动。**

---

## 逐项验证

### 1. buildScanText 只读消息历史

`node_worldbook.go` 的 `buildScanText(msgs []prompt_ir.Message, depth int)` 仅处理
`ctx.RecentMessages`，而 `RecentMessages` 来源链：

```
game_loop.go: PlayTurn
  → session.GetHistory(sessionID, maxHistoryFloors)
    → SELECT message_pages WHERE floor.status=committed AND is_active=true
    → 返回 []map[string]string（只含 role + content）
  → 拼入当前回合 user_input
  → 写入 pipelineCtx.RecentMessages
```

变量（`ctx.Variables`）只用于 `resolveMacros`（宏展开）和 `matchKey`（`var:` 前缀关键词门控），
**不写入 buildScanText 的输入**。

### 2. initial_variables 不存在于 Go 代码

在整个 `backend-v2/internal/` 目录中搜索 `initial_var` / `InitialVar`：**零匹配**。

该字段只存在于规划文档（implementation-plan.md）。后续若实现，应在：
- `CreateSession` 时读取 `GameTemplate.Config.initial_variables`
- 写入 `GameSession.Variables`（作为初始变量），而**不是**作为 Floor / Message 注入

只要不把 initial_variables 的内容写成 `message_pages.messages`，就不会进入 buildScanText。

### 3. first_mes 未在游玩管线中使用

`grep "first_mes" internal/` 只命中 `internal/creation/card/parser.go`，
这是解析 ST 角色卡 PNG 的创作工具，不参与 PlayTurn 流程。

当前 `CreateSession`（`engine_methods.go:24`）只创建一条空 session，
**不会**自动把 `first_mes` 写成 Floor 0。

若未来实现 first_mes 自动注入，需要将其写成一个特殊标记的 Floor（`seq=0`）并设
`is_first_mes=true`，然后在 `buildScanText` 中跳过该 Floor，确保它不参与关键词扫描。

---

## 相关路径备忘

| 路径 | 作用 |
|------|------|
| `node_worldbook.go: buildScanText()` | 扫描文本构建，只读 msgs 参数 |
| `session/manager.go: GetHistory()` | 消息历史查询，只返回已提交楼层 |
| `engine_methods.go:24 CreateSession()` | Session 创建，仅存空变量 `{}` |
| `creation/card/parser.go: FirstMes` | 角色卡 PNG 解析，创作工具，不参与游玩 |

---

## 下一步

3-D.2 完成（无改动），可直接进入：

- **3-E**：Memory stage_tags（`memories` 表新增 `stage_tags` 字段，~50 行）
- 或 **SQLite 驱动支持**：`gorm.io/driver/sqlite` + `--db` 参数（GW 本地化部署前置依赖）
