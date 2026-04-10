# Engine API 路由迁移计划

> 版本：2026-04-10 v2（详细迁移步骤 + 代码级规格）
> 状态：**计划中，尚未执行**
> 触发条件：现在主动推进，不等 A-11

---

## 一、背景与动机

`engine/api/routes.go`（当前 564 行）混合了两类路由：

| 类型 | 路由 | 问题 |
|------|------|------|
| 玩家发现层 | `GET /play/games`、`GET /play/games/:slug`、`GET /play/sessions`、`POST /play/sessions`、`GET /play/games/worldbook/:id` | 纯 DB 读，不需要 LLM，不属于 engine 职责 |
| 执行层 | `POST /play/sessions/:id/turn`、`GET /play/sessions/:id/stream`、`POST /play/sessions/:id/regen` 等 | 核心执行，需要 Pipeline + LLM，属于 engine |

**迁移的直接收益：**
1. A-10（`comment_config` 暴露）在 `platform/play/` 层可以合法 import `social/comment`，不违反架构原则
2. `engine/api/routes.go` 瘦身，职责单一
3. 为 A-11（常驻角色）和未来 `platform/user/` 包铺路

**API 路径不变，前端无感知。**

---

## 二、迁移后目标架构

```
internal/platform/play/
├── handler.go     ← 游戏查询、存档列表、会话创建、worldbook、comment_config
└── routes.go      ← RegisterPlayRoutes(rg, h)

internal/engine/api/
├── game_loop.go   ← PlayTurn、StreamTurn（不变）
├── engine_methods.go ← CreateSession、Suggest、ForkSession 等（不变）
├── routes.go      ← 只保留执行层路由（turn/stream/regen/fork/floors/memories 等）
└── verifier.go    ← 不变
```

`platform/play/` 的依赖图：
```
platform/play/
  ├── core/db          ← 读 GameTemplate（publicGameView 逻辑移入此包）
  ├── core/util        ← ParsePage
  ├── platform/auth    ← GetAccountID
  ├── social/comment   ← 读 GameCommentConfig（A-10 在此实现）
  └── engine/api       ← 通过 SessionCreator 接口调用 CreateSession
                         （接口定义在 platform/play/，engine 实现它）
```

---

## 三、接口设计

`platform/play/` 通过接口依赖 engine，不直接 import `engine/api` 包（避免循环依赖）：

```go
// internal/platform/play/handler.go

// SessionCreator 由 engine.GameEngine 实现，platform/play 通过此接口调用
type SessionCreator interface {
    CreateSession(ctx context.Context, gameID, userID string) (string, error)
}

type Handler struct {
    db      *gorm.DB
    engine  SessionCreator
    comment *comment.Service  // 用于读取 GameCommentConfig（A-10）
}

func NewHandler(db *gorm.DB, engine SessionCreator, commentSvc *comment.Service) *Handler {
    return &Handler{db: db, engine: engine, comment: commentSvc}
}
```

`GameEngine` 已有 `CreateSession` 方法，天然满足接口，无需修改。

---

## 四、需要迁移的路由（完整列表）

| 路由 | 当前位置 | 目标位置 | 迁移复杂度 |
|------|---------|---------|-----------|
| `GET /play/games` | `engine/api/routes.go:23` | `platform/play/handler.go` | 低（复制 handler，移除 `engine.db` 引用改为 `h.db`）|
| `GET /play/games/:slug` | `engine/api/routes.go:71` | `platform/play/handler.go` | 低 |
| `GET /play/games/worldbook/:id` | `engine/api/routes.go:303` | `platform/play/handler.go` | 低 |
| `GET /play/sessions` | `engine/api/routes.go:253` | `platform/play/handler.go` | 低 |
| `POST /play/sessions` | `engine/api/routes.go:84` | `platform/play/handler.go` | 中（调用 `h.engine.CreateSession()`）|

**同时在 `platform/play/` 新增（A-10）：**

| 路由 | 说明 |
|------|------|
| `GET /play/games/:slug` 响应中附加 `comment_config` | 调用 `h.comment.GetConfig(game.ID)` |

**保留在 `engine/api/routes.go`（不迁移）：**

| 路由 | 理由 |
|------|------|
| `POST /play/sessions/:id/turn` | 核心执行，需要 Pipeline |
| `GET /play/sessions/:id/stream` | SSE 流式，需要 Pipeline |
| `POST /play/sessions/:id/regen` | 执行层 |
| `POST /play/sessions/:id/suggest` | 执行层（调用 LLM）|
| `POST /play/sessions/:id/fork` | 执行层 |
| `GET /play/sessions/:id/floors` | 引擎内部数据 |
| `GET /play/sessions/:id/floors/:fid/pages` | 引擎内部数据 |
| `PATCH /play/sessions/:id/floors/:fid/pages/:pid/activate` | 引擎内部操作 |
| `GET /play/sessions/:id/state` | 引擎内部数据 |
| `GET /play/sessions/:id/variables` | 引擎内部数据 |
| `PATCH /play/sessions/:id/variables` | 引擎内部操作 |
| `PATCH /play/sessions/:id` | 更新 session 标题/状态/is_public |
| `DELETE /play/sessions/:id` | 删除 session |
| `GET /play/sessions/:id/memories` | 引擎内部数据 |
| `POST /play/sessions/:id/memories` | 引擎内部操作 |
| `PATCH /play/sessions/:id/memories/:mid` | 引擎内部操作 |
| `DELETE /play/sessions/:id/memories/:mid` | 引擎内部操作 |
| `POST /play/sessions/:id/memories/consolidate` | 引擎调试 |
| `GET /play/sessions/:id/memory-edges` | 引擎内部数据 |
| `GET /play/sessions/:id/memories/:mid/edges` | 引擎内部数据 |
| `POST /play/sessions/:id/memory-edges` | 引擎调试 |
| `PATCH /play/sessions/:id/memory-edges/:eid` | 引擎调试 |
| `DELETE /play/sessions/:id/memory-edges/:eid` | 引擎调试 |
| `GET /play/sessions/:id/branches` | 引擎内部数据 |
| `POST /play/sessions/:id/floors/:fid/branch` | 引擎内部操作 |
| `DELETE /play/sessions/:id/branches/:bid` | 引擎内部操作 |
| `GET /play/sessions/:id/prompt-preview` | 引擎调试 |
| `GET /play/sessions/:id/floors/:fid/snapshot` | 引擎调试 |
| `GET /play/sessions/:id/tool-executions` | 引擎调试 |

---

## 五、`publicGameView` 的处置

当前 `publicGameView` 是 `engine/api/engine_methods.go` 中的包级私有函数（`func publicGameView(t dbmodels.GameTemplate) map[string]any`）。

迁移后它需要在 `platform/play/` 中使用，有两个选项：

**方案 A（推荐）：** 在 `platform/play/handler.go` 中重新定义同名函数（内容完全相同）。迁移完成后删除 `engine_methods.go` 中的旧版本。

**方案 B：** 提取到 `core/db` 包作为 `GameTemplate` 的方法（`func (t GameTemplate) PublicView() map[string]any`）。更优雅，但改动范围更大。

**当前选择方案 A**，保持改动最小。方案 B 可在后续重构时做。

---

## 六、A-10 在迁移后的实现

`platform/play/handler.go` 的游戏详情 handler：

```go
func (h *Handler) getGame(c *gin.Context) {
    slug := c.Param("slug")
    var tmpl dbmodels.GameTemplate
    err := h.db.Where("status = 'published' AND (slug = ? OR id::text = ?)", slug, slug).
        First(&tmpl).Error
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
        return
    }

    view := publicGameView(tmpl)

    // A-10：附加 comment_config（platform/play 可合法 import social/comment）
    cfg := h.comment.GetConfig(tmpl.ID)
    view["comment_config"] = map[string]any{
        "default_mode": cfg.DefaultMode,
    }

    c.JSON(http.StatusOK, gin.H{"code": 0, "data": view})
}
```

---

## 七、`cmd/server/main.go` 变更

```go
// 新增 import
import (
    playapi "mvu-backend/internal/platform/play"
)

// 注册路由（在 gameapi.RegisterGameRoutes 之后）
playHandler := playapi.NewHandler(gormDB, engine, commentSvc)
playapi.RegisterPlayRoutes(api, playHandler)

// gameapi.RegisterGameRoutes 保留，但其中已迁移的路由被删除
gameapi.RegisterGameRoutes(api, engine)
```

---

## 八、迁移步骤（执行时按序）

```
Step 1：新建 internal/platform/play/ 目录
        创建 handler.go（Handler 结构体 + SessionCreator 接口 + publicGameView）
        创建 routes.go（RegisterPlayRoutes）

Step 2：把 5 条路由的 handler 逻辑从 engine/api/routes.go 复制到 platform/play/handler.go
        - GET /play/games
        - GET /play/games/:slug（同时附加 comment_config，实现 A-10）
        - GET /play/games/worldbook/:id
        - GET /play/sessions
        - POST /play/sessions

Step 3：在 cmd/server/main.go 注册 platform/play/ 路由

Step 4：从 engine/api/routes.go 删除已迁移的 5 条路由

Step 5：从 engine/api/engine_methods.go 删除 publicGameView 函数

Step 6：go build ./... 验证编译
        go vet ./... 验证静态分析

Step 7：手动测试关键路径
        - GET /api/play/games → 返回游戏列表
        - GET /api/play/games/:slug → 返回游戏详情（含 comment_config）
        - POST /api/play/sessions → 创建 session，返回 session_id
        - GET /api/play/sessions/:id/stream → SSE 流式正常（engine 路由未受影响）
```

---

## 九、风险与注意事项

| 风险 | 说明 | 缓解 |
|------|------|------|
| `publicGameView` 重复定义 | 两处代码短暂并存 | Step 5 立即删除旧版本，不留死代码 |
| `engine/api/routes.go` 中 `engine.db` 引用 | 迁移后 engine 路由不再需要直接查 GameTemplate | 检查剩余路由是否还有 `engine.db.Model(&GameTemplate{})` 调用 |
| `POST /play/sessions` 的 `play_count` 递增 | 当前在 `engine/api/routes.go:99` 做 `play_count + 1` | 迁移到 `platform/play/` 时保留此逻辑 |
| `session.FloorWithPage` 类型跨包引用 | `GET /play/sessions` 返回 `[]GameSession`，不涉及此类型，无问题 | — |

---

## 十、迁移完成后的文件状态

```
internal/platform/play/
├── handler.go     ← 新建（~120 行）
└── routes.go      ← 新建（~20 行）

internal/engine/api/
├── game_loop.go   ← 不变
├── engine_methods.go ← 删除 publicGameView（~25 行减少）
├── routes.go      ← 删除 5 条路由（~80 行减少，从 564 → ~484 行）
└── verifier.go    ← 不变
```

---

## 十一、与其他文档的对接

| 文档 | 关联 |
|------|------|
| `GW-NEW-APIS-PLAN.md` | A-10 在迁移后实现，A-11 在迁移后规划 |
| `GW-BACKEND-REFACTOR-PLAN.md` | Task 4（social 实现）已完成；路由迁移是独立后续步骤 |
| `GW-SHARED-INFRA-PLAN.md` | `platform/play/` 的依赖图与第六章一致 |
| `platform-architecture.md` | 路由归属表（第三节）与本文第四节一致 |
| `P-WE-OVERVIEW.md` | 路由迁移不属于 WE Phase 计划，属于 GW 平台层工程 |
