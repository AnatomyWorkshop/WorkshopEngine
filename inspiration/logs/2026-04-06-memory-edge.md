# Memory Edge（记忆关系图）— 实验日志

> 日期：2026-04-06
> 状态：已合并到主线
> 关联日志：`2026-04-06-structured-memory.md`

---

## 背景与动机

结构化 Memory 整合完成后（fact_key 系统），自然的下一步是记录事实之间的有向关系，
使未来的 Memory Lint 能够发现矛盾、追踪变更历史。

TH M21 的 `applyConsolidation` 在处理同 key 冲突时写入 `updates` 边；
M（macro compaction）写入 `derived_from` / `compacts` 边；事实解决悬念时写入 `resolves` 边。

WE 没有双层摘要压缩（macro/micro），去掉了 `derived_from` 和 `compacts`，
保留 4 种对 WE 有意义的 relation。

---

## 与 TH 的对比

| 特性 | TH | WE | 说明 |
|------|----|----|------|
| memory_edge 表 | ✅ | ✅ | 对等 |
| 6 种 relation | updates/contradicts/supports/resolves/derived_from/compacts | updates/contradicts/supports/resolves | WE 去掉双层压缩专用的 2 种 |
| account 隔离 | account_id 外键 | session_id 隔离 | WE 无多账户表，用 session_id 替代 |
| 整合时自动写 updates 边 | ✅ 同 key 冲突 → winner updates loser | ✅ facts_update → UpsertFact 废弃旧行 + 写 updates 边 | 对等 |
| resolves 边（悬念解决） | ✅ 自动（LLM 输出解析） | ❌ 需手动通过 API 创建 | WE 暂未实现 LLM 自动标记 open_loop 解决 |
| contradicts 边 | ✅ Lint 扫描后写入 | ❌ 暂无 Lint，只能手动 POST | WE 暂无 Memory Lint |
| findEdges 双向查询 | ✅ | ✅ `ListEdges(sessionID, memID)` | 对等 |
| Edge CRUD API | ✅ 5 个端点（GET/POST/PATCH/DELETE） | ✅ 5 个端点（GET×2 / POST / PATCH / DELETE） | 对等 |
| 参与 Prompt 注入 | ❌ 纯溯源 | ❌ 纯溯源 | 一致 |

---

## 改动内容

### 数据模型（`internal/core/db/models.go`）

新增 `MemoryRelation` 类型（4 个常量）和 `MemoryEdge` 结构体：
- `id` uuid 主键
- `session_id` 索引
- `from_id / to_id` 各自独立索引（GORM 不支持 CASCADE，物理删除 Memory 时需手动清理 edge）
- `relation` 枚举字符串
- `created_at`

**注意**：GORM AutoMigrate 不设置 FK CASCADE。Memory 被物理删除时 edge 不会自动删除，
靠维护脚本或上层逻辑清理孤儿边（当前低优先级，物理删除 Memory 本身就是管理员操作）。

### Store 层（`internal/engine/memory/store.go`）

**`UpsertFact` 签名变更**（重要）：
```
旧：UpsertFact(...) error            // in-place UPDATE 同一行
新：UpsertFact(...) (newID, oldID string, err error)  // 废弃旧行 + 新建行
```
旧行 deprecated=true，新行为独立行。这样 `(newID, oldID)` 可以写入 `updates` 边。

**`DeprecateFactsByKey` 签名变更**：
```
旧：DeprecateFactsByKey(...) error
新：DeprecateFactsByKey(...) ([]Memory, error)  // 返回被废弃的行，供写 edge 用（暂未用）
```

新增方法：
- `SaveEdge(sessionID, fromID, toID, relation)` — 写入一条边
- `ListEdges(sessionID, memoryID)` — 双向查询某条记忆的所有边
- `ListEdgesBySession(sessionID, relation, limit, offset)` — 会话级分页列表
- `DeleteEdge(sessionID, edgeID)` — 物理删除

**`applyStructuredResult` 更新**：
- `facts_deprecate`：不写 edge（纯标记，LLM 明确废弃，无需溯源到哪个新事实）
- `facts_add`：不写 edge（全新 key，无旧行）
- `facts_update`：`UpsertFact` 返回 `(newID, oldID)` 后写入 `updates` 边

### API 路由（`internal/engine/api/routes.go`）

新增 4 个端点：

```
GET    /api/v2/play/sessions/:id/memory-edges               会话所有边（?relation=&limit=&offset=）
GET    /api/v2/play/sessions/:id/memories/:mid/edges        某条记忆的双向边
POST   /api/v2/play/sessions/:id/memory-edges               手动创建边（调试 / Lint 标记用）
PATCH  /api/v2/play/sessions/:id/memory-edges/:eid          修改 relation 类型（标错时调试用）
DELETE /api/v2/play/sessions/:id/memory-edges/:eid          删除边
```

**关于 PATCH**：最初以"防止溯源被破坏"为由去掉，但 Memory Edge 是可变工作数据而非不可变审计日志。调试时标错 relation 需要删除+重建两步，加上 PATCH 更合理。

**关于双层压缩（derived_from/compacts）**：不加。这两种 relation 只在 macro/micro 双层摘要架构下有意义。WE 目前只有单层 `type=summary`，加入是死代码。等 Memory Worker 引入双层架构后再补。

---

## 可能需要调整的内容

### UpsertFact 废弃旧行的副作用
原先 `facts_update` 做 in-place UPDATE（同一行 ID，history 中只有一行）。
现在废弃旧行 + 新建行，GetForInjection 的候选池里不再有旧行（deprecated=true 被过滤），
注入行为与之前一致，但 DB 会多出更多 deprecated 行，需要维护任务定期清理。

**当前维护策略**：`PurgeDeprecatedMemoriesGlobal` 只清理 `type=summary` 的行。
`type=fact` 的旧行永久保留（供 edge 溯源）。
若 fact 行积累过多，可在维护策略中加入 `type=fact AND deprecated=true AND age > N days` 的 purge。

### resolves / contradicts 边的自动写入
当前只有 `updates` 边在整合时自动写入。
`resolves`（open_loop 被解决）和 `contradicts`（矛盾标记）需要手动 POST 或未来 Memory Lint 实现。
这是有意推迟的设计——Lint 是 Phase 3 后续工作，不在本次范围内。

### 孤儿边清理
Memory 物理删除后 edge 不会自动级联删除（GORM AutoMigrate 不设 FK CASCADE）。
目前影响：孤儿边查询会返回，但 from_id / to_id 对应的 Memory 已不存在，调用方需要容错。
后续可在 DeleteMemory(hard=true) 时同步删除关联 edge，或加 DB 级 FK。

---

## 测试要点

1. 触发记忆整合，确认 `facts_update` 后 DB `memory_edges` 有一条 `relation=updates` 的行
2. `GET /sessions/:id/memories/:mid/edges` 返回包含上述边
3. `GET /sessions/:id/memory-edges?relation=updates` 过滤正确
4. 手动 `POST /sessions/:id/memory-edges`（`relation=contradicts`）后 `DELETE` 该边，确认删除成功
5. 重复 `facts_update` 同一 key 多次，确认每次都有新的 `updates` 边链（新行 → 上一轮新行（已废弃））
