# 补丁 3-E：Memory 分阶段标签（stage_tags）

> 日期：2026-04-07
> 状态：✅ 完成，编译通过，全部现有测试通过
> 关联计划：implementation-plan.md § 3-E

---

## 背景

多幕叙事（如三幕侦探故事、多线武侠剧情）中，前期调查线索/红鲱鱼条目在后期剧情中持续占用 token 并干扰 LLM 判断。需要让创作者为 Memory 条目打上阶段标签，引擎在每回合注入记忆时按当前 `game_stage` 变量过滤，实现"LLM 只看当前阶段需要的记忆"。

**与 TH 对比：** TH 无此设计，`GetForInjection` 全量注入所有非废弃记忆。此为 WE 原创功能。

---

## 改动文件

| 文件 | 改动 |
|------|------|
| `internal/core/db/models.go` | `Memory` 新增 `StageTags datatypes.JSON`（`default:'[]'`） |
| `internal/engine/memory/store.go` | `GetForInjection` 新增 `currentStage` 参数；新增 `stageMatches` / `stageToTags` helpers；`UpsertFact` 新增 `stageTags []string` 参数；`factEntry` 新增 `Stage string` 字段；`UpdateMemory` 允许更新 `stage_tags` |
| `internal/engine/memory/worker.go` | `GetForInjection` 调用补 `""` 参数（Worker 缓存不过滤） |
| `internal/engine/api/game_loop.go` | 步骤 4 改为从 `chatVars["game_stage"]` 提取阶段，调用 `GetForInjection` 替代 `sess.MemorySummary` |
| `internal/engine/api/engine_methods.go` | streaming PlayTurn 路径、PromptPreview 路径同上改动；`CreateMemoryReq` 新增 `StageTags []string`；`CreateMemory` 写入 StageTags |

无 DB schema 迁移文件（GORM AutoMigrate 自动添加列）。

---

## 核心逻辑

### stage_tags 语义

```
[]                    → 无阶段限制，始终注入（向后兼容）
["act_1"]             → 仅在 game_stage == "act_1" 时注入
["act_1", "act_2"]    → 在第一幕或第二幕均注入
```

### 过滤时机

```
PlayTurn 主链路：
  chatVars["game_stage"] → currentStage
  GetForInjection(sessionID, budget, currentStage)
    → 从 DB 取最多 50 条未废弃记忆
    → 若 currentStage != ""，过滤 stageMatches
    → 衰减排序 + Token 预算裁剪
    → 返回注入文本（直接用于本回合 prompt）

Worker 缓存路径（consolidation 后）：
  GetForInjection(sessionID, budget, "")  ← 不过滤，全量缓存
  UpdateSessionSummaryCache               ← 供 GetState 展示用
```

### LLM 结构化输出中打标

`consolidationJSONInstruction` 的 `facts_add` 数组现在支持可选 `stage` 字段：

```json
{
  "facts_add": [
    {"key": "clue_knife", "content": "凶器是匕首", "stage": "act_1"},
    {"key": "final_culprit", "content": "凶手是管家", "stage": "act_3"}
  ]
}
```

LLM 不输出 `stage` 时，词条 `stage_tags = []`（无阶段限制，默认注入）。

### UpsertFact 签名变更

```go
// 旧
UpsertFact(sessionID, factKey, content string, importance float64, sourceFloor int)

// 新
UpsertFact(sessionID, factKey, content string, importance float64, sourceFloor int, stageTags []string)
```

`stageTags` 传 `nil` 或 `[]string{}` 均等价于"无限制"。

---

## 注入文本缓存的变化

| 路径 | 之前 | 之后 |
|------|------|------|
| PlayTurn (主) | 直读 `sess.MemorySummary`（Worker 异步刷新的缓存） | 每回合实时查询，按当前阶段过滤 |
| PlayTurn (streaming) | 直读 `sess.MemorySummary` | 同上 |
| PromptPreview | 直读 `sess.MemorySummary` | 同上 |
| Worker 缓存刷新 | `GetForInjection(id, budget)` | `GetForInjection(id, budget, "")` 全量不过滤 |
| `GetState` API | 返回 `sess.MemorySummary` | 不变（仍返回全量缓存，用于调试展示） |

**性能影响：** 每回合 PlayTurn 多一次 DB 查询（SELECT + ORDER BY + LIMIT 50，走索引）。对于游戏规模（< 1000 条记忆），延迟可忽略不计。

---

## 启用方式

### 方法一：LLM 自动打标（推荐）

在 `consolidationJSONInstruction` 中补充创作者期望的阶段提示：

```
system_prompt_template 中加入：
"当前剧情阶段：{{game_stage}}"
```

整合模型会在 `facts_add` 中自动填写 `stage`（如果在指令中提示它这么做）。

### 方法二：API 手动打标

```http
POST /api/v2/play/sessions/:id/memories
{"content": "玩家获得了铁锤", "type": "fact", "stage_tags": ["act_2"]}

PATCH /api/v2/play/sessions/:id/memories/:mid
{"stage_tags": ["act_2", "act_3"]}
```

### 方法三：变量驱动（无需改代码）

在游戏模板中：
```
初始变量：game_stage = "act_1"
第二幕触发规则：game_stage = "act_2"（通过 set_variable 工具）
```

引擎自动按当前阶段过滤，无需任何 API 调用。

---

## 下一步

- **3-F**：边界归档 API（`POST /sessions/:id/archive`）—— 结局时固化记忆
- **SQLite 驱动支持**：`gorm.io/driver/sqlite` + `--db` 参数（GW 本地部署前置）
