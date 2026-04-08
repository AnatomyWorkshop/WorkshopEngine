# 补丁 3-D.1：Worldbook Token Budget

> 日期：2026-04-07
> 状态：✅ 完成，编译通过，全部现有测试通过
> 关联计划：implementation-plan.md § 3-D.1

---

## 背景

异世界和平移植测试中，第一回合消耗 147k+ tokens，超过 DeepSeek 的 131k 上下文限制。根因是：22 条角色描述词条（每条 5000–8900 chars）全部触发，原有的 `worldbook_group_cap`（按条数裁剪）对总 token 量没有约束。

## TH 参考实现

`packages/core/src/prompt/token-budget.ts` — `TokenBudget.prune()`：

- `pinned=true` 的 section 不参与裁剪（对应 WE 的 `Constant=true`）
- `prunable` 消息按 priority 升序（数值大 = 优先淘汰）排序，同 priority 按 globalIndex 降序（新消息保留）
- 从排序后的候选列表贪心保留，超出预算的丢弃
- token 估算：`SimpleTokenCounter` — `ceil(text.length / 4)`

WE 的实现完全对齐此策略，但 Priority 语义与 TH 相反（WE：数值越小越重要 → 优先保留），已在实现中调整排序方向。

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/engine/prompt_ir/pipeline.go` | `GameConfig` 新增 `WorldbookTokenBudget int` 字段，含完整注释 |
| `internal/engine/pipeline/node_worldbook.go` | 新增 `estimateTokens()` + `applyTokenBudget()`，在 `applyGroupCap` 之后调用 |
| `internal/engine/api/game_loop.go` | `tmplCfg` 结构体新增 `WorldbookTokenBudget`，`GameConfig` 构造时透传 |
| `internal/engine/api/engine_methods.go` | 同上，两处 `tmplCfg`（PlayTurn 路径 + PromptPreview 路径）均已补全 |

无 DB schema 变更。新字段写入 `GameTemplate.Config` JSONB，字段名 `worldbook_token_budget`。

## 核心逻辑（node_worldbook.go）

```go
// estimateTokens：rune 数 / 4 上整，与 TH SimpleTokenCounter 一致
func estimateTokens(text string) int {
    n := len([]rune(text))
    return (n + 3) / 4
}

// applyTokenBudget：
// 1. Constant=true → pinned，不计入预算，始终保留
// 2. 其余按 Priority 升序（数值小 = 重要，优先保留）
// 3. 贪心累加，超出 budget 的词条跳过（丢弃，不是截断）
func applyTokenBudget(entries []prompt_ir.WorldbookEntry, budget int) []prompt_ir.WorldbookEntry
```

## 启用方式

在 `GameTemplate.Config` 中设置：

```json
{
  "worldbook_token_budget": 8000
}
```

`0` 或字段不存在时不裁剪（向后兼容）。

建议配置：
- 小型游戏（< 30 条词条）：不设置或 `0`
- 中型游戏（30–100 条）：`8000`
- 大型游戏（100+ 条，如异世界和平）：`6000`–`10000`

## 与 GroupCap 的执行顺序

```
激活词条
  → applyGroupCap（按组互斥裁剪）
  → applyTokenBudget（全局 token 裁剪）
  → 组装 PromptBlocks
```

GroupCap 先行——GroupCap 负责"同组只保留最相关的"，TokenBudget 负责"整体不超过上限"。两者独立工作，可以只配一个。

## 回滚说明

改动完全向后兼容，`WorldbookTokenBudget=0`（默认值）时 `applyTokenBudget` 不执行，行为与补丁前完全一致。

## 下一步

补丁 3-D.2：确认 `initial_variables` 不注入扫描文本（检查 `buildScanText` + `first_mes` 注入路径）。
