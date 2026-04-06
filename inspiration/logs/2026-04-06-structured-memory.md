# 结构化 Memory 整合 — 实验日志

> 日期：2026-04-06
> 状态：已合并到主线
> 目的：对齐 TH Memory V2，让 Memory Worker 输出结构化 JSON，而非仅追加自由文本

---

## 背景

WE 的 Memory Worker 原先只做"追加"：每 N 回合调用廉价模型生成 `<Summary>` + `事实：xxx` 行前缀，
无法更新已有事实，也无法废弃矛盾事实。导致长局游戏中事实并存、互相矛盾。

TH Memory V2 的做法：LLM 输出结构化 JSON `{turn_summary, facts_add, facts_update, facts_deprecate}`，
每条事实带有稳定 `fact_key`（英文蛇形命名），可以精确 upsert 和废弃。

---

## 改动内容

### 1. 数据模型（`internal/core/db/models.go`）

- `Memory` 新增 `FactKey string` 字段，带普通索引
- 空值 = 自由文本摘要 / 旧格式回退事实（无 key）
- 非空值 = 结构化事实，`UpsertFact` 按 `session_id + fact_key + deprecated=false` 定位唯一行

### 2. Store 层（`internal/engine/memory/store.go`）

新增方法：
- `UpsertFact(sessionID, factKey, content, importance, sourceFloor)` — 查找现有未废弃行更新，否则新建
- `DeprecateFactsByKey(sessionID, keys[])` — 按 key 批量软删除

改写方法：
- `BuildConsolidationPrompt` — 把现有 facts（含 `[key]`）展示给 LLM，要求输出结构化 JSON
- `ParseConsolidationResult` — 优先 JSON 解析，走 add/update/deprecate 三路；JSON 失败时回退旧格式
- `GetForInjection` — 注入文本中 `type=fact && fact_key != ""` 的条目带 `[key]` 前缀

清理：
- 移除 `StoreConfig.ConsolidationInstruction`（旧格式回退路径不再用此字段）
- 移除 `defaultConsolidationInstruction` 常量

### 3. Worker 层（`internal/engine/memory/worker.go`）

**无需修改** — 调用签名 `BuildConsolidationPrompt` / `ParseConsolidationResult` 完全兼容。

---

## 设计决策

### 为何 FactKey 不做唯一索引？

同一 session 下同一 key 历史上可能存在多行：废弃旧行 + 新建新行。
`UpsertFact` 用 `deprecated=false` 条件查找，保证业务唯一性；DB 只需普通 index 加速查询。

### 回退路径的作用

已上线游戏可能积累了旧格式（`<Summary>` + `事实：` 行前缀）的记忆，
以及部分 LLM 在 JSON 模式下可能输出带前缀说明文字的非标准 JSON。
双重回退保证迁移无感。

### [key] 前缀暴露给 Narrator

`GetForInjection` 加了 `[key]` 前缀后，Narrator LLM 能在生成内容时看到事实键名，
理论上可以输出更精准的引用（如 `{{player_affinity}} 已达到 70`）。
**这是试验性功能**，如果测试后发现 LLM 无法正确使用或产生噪音，直接删除 `formatMemoryLine` 中的键前缀即可。

---

## 可能需要删掉的内容

### [key] 前缀注入（低风险）
- 文件：`internal/engine/memory/store.go`，函数 `formatMemoryLine`
- 若 Narrator 混淆键名和内容，直接把 `formatMemoryLine` 改回返回 `m.Content` 即可，1 行改动

### JSON 整合整体功能（若 LLM 无法稳定输出结构化 JSON）
- 可在 `ParseConsolidationResult` 中把 JSON 解析分支全部注释，只保留旧格式回退路径
- 短期风险：key 系统失效，`UpsertFact` / `DeprecateFactsByKey` 不会被调用

---

## 测试要点（尚未写自动化测试）

1. 调用 `POST /sessions/:id/memories/consolidate` 手动触发整合
2. 检查 DB `memories` 表，确认有 `fact_key` 非空的行
3. 再次触发整合，确认 `UpsertFact` 更新了已有行而非新建重复行
4. 在 consolidation LLM 输出 `facts_deprecate: ["key1"]` 时，确认对应行 `deprecated=true`
5. 检查 `GET /sessions/:id/memories` 返回的内容，`[key]` 前缀是否在 `memory_summary` 缓存中出现

---

## 对比 TH PROGRESS.md（M21）

| TH M21 特性 | WE 状态 |
|-------------|---------|
| `MemoryStore.applyConsolidation()` 自动冲突消解（同 key 旧记录 deprecated + updates 边） | ✅ `UpsertFact` 废弃旧行，但无 memory_edge（updates 边） |
| `MemoryInjectionOptions.decay`（半衰期衰减排序） | ✅ 已有（HalfLifeDays / MinDecayFactor） |
| `MemoryMaintenanceService` + 定时任务（deprecate summary / purge deprecated） | ✅ `Worker.runMaintenance`，独立 goroutine |
| JSON 结构化整合输出（facts_add / facts_update / facts_deprecate） | ✅ 刚完成 |
| memory_edge（supports / contradicts / updates 关系图） | ❌ Phase 3 中期目标 |
| 记忆维护 CLI（dry-run / batch / policy flags） | ❌ 未计划，当前 API 触发 |
