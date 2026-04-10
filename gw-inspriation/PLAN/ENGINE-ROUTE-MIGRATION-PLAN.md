# Engine API 路由迁移 — 归档

> 版本：2026-04-10 v3（归档已完成工作 + 后续改进备忘）
> 状态：**已完成 2026-04-10**

---

## 一、已完成工作（归档）

### 背景

`engine/api/routes.go`（原 564 行）混合了玩家发现层（纯 DB 读）和执行层（需要 LLM/Pipeline）两类路由，违反职责单一原则，且导致 A-10（`comment_config`）无法在 engine 层合法 import `social/comment`。

### 迁移结果

**新建 `internal/platform/play/`：**

```
internal/platform/play/
├── handler.go   ← Handler + SessionCreator 接口 + publicGameView + 5 个 handler 方法
└── routes.go    ← RegisterPlayRoutes(rg, h)
```

**迁移的 5 条路由（从 engine/api/routes.go 移出）：**

| 路由 | 说明 |
|------|------|
| `GET /play/games` | 游戏列表，分页/标签/类型/排序 |
| `GET /play/games/:slug` | 游戏详情（slug 或 UUID），**附加 comment_config（A-10）** |
| `GET /play/games/worldbook/:id` | 玩家只读世界书 |
| `GET /play/sessions` | 存档列表，直接 GORM 查询（不再调用 engine.ListSessions）|
| `POST /play/sessions` | 创建会话，调用 SessionCreator 接口 + 原子递增 play_count |

**关键设计决策：**
- `SessionCreator` 接口定义在 `platform/play/`，`GameEngine` 天然满足，无需修改 engine
- `publicGameView` 复制到 `platform/play/handler.go`，同时从 `engine_methods.go` 删除（方案 A）
- `GET /play/sessions` 改为直接 GORM 查询，不再依赖 `engine.ListSessions()`
- `platform/play/` 合法 import `social/comment`，A-10 在 `getGame()` 中实现

**文件变化：**
- `engine/api/routes.go`：564 → 437 行（删除 5 条路由 + 相关 import）
- `engine/api/engine_methods.go`：删除 `publicGameView`（~28 行）
- `cmd/server/main.go`：新增 `playapi` import + `NewHandler` + `RegisterPlayRoutes`

**验证：** `go build ./...` + `go vet ./...` 均通过，无编译错误。

---

## 二、A-10 实现（已完成）

`platform/play/handler.go` 的 `getGame()` 在游戏详情响应中附加：

```json
"comment_config": { "default_mode": "linear" }
```

前端 `CommentCore` 可读取此字段，不再需要硬编码 `linear`。

---

## 三、后续改进备忘

### 3-A：`publicGameView` 提取到 `core/db`（方案 B，低优先级）

**当前状态：** `publicGameView` 定义在 `platform/play/handler.go`，是包级私有函数。

**改进方向：** 提取为 `GameTemplate` 的方法：

```go
// internal/core/db/models_shared.go
func (t GameTemplate) PublicView() map[string]any {
    // 同现有 publicGameView 逻辑
}
```

**收益：** 语义更清晰，`creation/api` 等其他包若需要构造公开视图可直接调用，不需要重复实现。

**触发时机：** 当第二个包需要相同逻辑时再做，目前只有 `platform/play/` 使用，不值得提前抽象。

---

### 3-B：`comment_config` 响应字段扩展（低优先级）

**当前响应：**
```json
"comment_config": { "default_mode": "linear" }
```

**可扩展字段：** `enabled_modes`、`allow_anonymous`、`require_approval`（均在 `GameCommentConfig` 中已有）。

**触发时机：** 前端需要展示评论模式切换 UI 或匿名评论开关时。

---

### 3-C：`GET /play/sessions` 分页一致性（低优先级）

**当前状态：** `listSessions` 直接读 `limit`/`offset` query param，上限 100，默认 20。

**潜在问题：** 与其他接口的 `util.ParsePage`（上限 200，默认 20）行为不一致。

**建议：** 统一改用 `util.ParsePage(c)`，或在 `util.ParsePage` 中支持自定义上限参数。

---

### 3-D：`platform/user/` 包规划（A-11 前置）

路由迁移完成后，`platform/play/` 包已建立，为 A-11（常驻角色）铺路。

A-11 的路由 `GET/POST/DELETE /api/users/:id/resident_character` 可归属：
- **选项 1：** 放入 `platform/play/`（扩展现有包）
- **选项 2：** 新建 `platform/user/`（职责更清晰，但多一个包）

**建议：** 等 A-11 进入开发计划时再决策，不提前创建空包。

---

## 四、与其他文档的对接

| 文档 | 状态 |
|------|------|
| `GW-NEW-APIS-PLAN.md` | A-10 已归档为完成，A-11 待规划 |
| `GW-BACKEND-REFACTOR-PLAN.md` | Task 4（social 实现）已完成；路由迁移独立完成 |
| `P-WE-OVERVIEW.md` | 路由迁移不属于 WE Phase 计划，属于 GW 平台层工程 |
