# Engine API 路由迁移计划

> 版本：2026-04-09
> 背景：`internal/engine/api/routes.go` 当前混合了两类路由——"游玩执行层"（engine 职责）和"玩家发现/入口层"（platform 职责）。本文描述需要迁移的内容、目标位置和迁移时机。

---

## 一、问题

`engine/api/routes.go` 目前包含：

```
/api/play/games              ← 游戏列表查询，纯 DB 读，不需要 LLM
/api/play/games/:slug        ← 游戏详情查询，纯 DB 读，不需要 LLM
/api/play/sessions           ← 存档列表查询，纯 DB 读，不需要 LLM
/api/play/sessions (POST)    ← 创建会话，调用 engine.CreateSession()
/api/play/sessions/:id/turn  ← 游玩回合，核心执行，需要 Pipeline + LLM
/api/play/sessions/:id/stream ← SSE 流式，核心执行
... 其余执行层路由
```

前三条（游戏查询、存档列表）和 engine 的核心职责无关，放在 `engine/api/` 是职责越界。

---

## 二、目标架构

```
internal/platform/play/       ← 新建，玩家发现 + 入口层
├── handler.go                ← 游戏查询、存档列表、会话创建入口
└── routes.go                 ← 注册路由，依赖 engine.SessionCreator 接口

internal/engine/api/          ← 保留，纯执行层
└── routes.go                 ← 只保留 turn/stream/fork/floors/memories 等执行路由
```

`platform/play/` 通过接口依赖 engine，不直接 import `engine/api` 包：

```go
// platform/play/handler.go
type SessionCreator interface {
    CreateSession(ctx context.Context, gameID, userID string) (string, error)
}
```

`cmd/server/main.go` 把 `engine`（实现了 `SessionCreator`）注入给 `platform/play/`。

---

## 三、需要迁移的路由

| 路由 | 当前位置 | 目标位置 | 迁移复杂度 |
|------|---------|---------|-----------|
| `GET /api/play/games` | `engine/api/routes.go` | `platform/play/handler.go` | 低（纯 DB 查询，复制即可）|
| `GET /api/play/games/:slug` | `engine/api/routes.go` | `platform/play/handler.go` | 低 |
| `GET /api/play/games/:id/worldbook-entries` | `engine/api/routes.go`（A-5 待实现）| `platform/play/handler.go` | 低 |
| `GET /api/play/sessions` | `engine/api/routes.go` | `platform/play/handler.go` | 低 |
| `POST /api/play/sessions` | `engine/api/routes.go` | `platform/play/handler.go` | 中（需注入 SessionCreator 接口）|

**不迁移**（保留在 `engine/api/`）：

| 路由 | 理由 |
|------|------|
| `POST /api/play/sessions/:id/turn` | 核心执行，需要 Pipeline |
| `GET /api/play/sessions/:id/stream` | SSE 流式，需要 Pipeline |
| `POST /api/play/sessions/:id/regen` | 执行层 |
| `POST /api/play/sessions/:id/fork` | 执行层 |
| `GET /api/play/sessions/:id/floors` | 引擎内部数据 |
| `GET /api/play/sessions/:id/memories` | 引擎内部数据 |
| `GET /api/play/sessions/:id/state` | 引擎内部数据 |
| `PATCH /api/play/sessions/:id/variables` | 引擎内部操作 |
| `GET /api/play/sessions/:id/prompt-preview` | 引擎调试 |
| `GET /api/play/sessions/:id/memory-edges` | 引擎内部数据 |
| `GET /api/play/sessions/:id/branches` | 引擎内部数据 |

---

## 四、什么时候迁移

**现在不迁移。** 理由：

1. **API 路径不变**：迁移只是代码位置变化，不影响任何已有或计划中的 API 路径，前端无感知。
2. **当前优先级更高的事**：A-4（session 公开）、A-5（世界书只读）、A-6（常驻角色）都还没实现，前端开发等待这些 API。
3. **迁移触发条件**：当 `platform/play/` 包因为其他原因需要建立时（例如 A-6 的 `UserProfile` 管理路由天然属于 platform 层），顺带把游戏查询路由一起迁移过去。

**触发迁移的时机**：

```
条件 A：开始实现 A-6（常驻角色）时
        → A-6 的 /api/users/:id/resident_character 属于 platform 层
        → 此时建立 platform/play/ 包，顺带迁移游戏查询路由

条件 B：engine/api/routes.go 超过 600 行，维护困难时
        → 强制拆分

条件 C：需要对游戏查询路由加独立中间件（如缓存、限流）时
        → 迁移到独立包更容易加中间件
```

**当前状态**：`engine/api/routes.go` 约 460 行，未触发条件 B。预计在实现 A-6 时（Week 3）触发条件 A，届时一并迁移。

---

## 五、迁移步骤（届时执行）

1. 新建 `internal/platform/play/` 包
2. 定义 `SessionCreator` 接口（只含 `CreateSession`）
3. 把游戏查询 handler 函数从 `engine/api/routes.go` 移动到 `platform/play/handler.go`
4. `publicGameView` 辅助函数随之移动（或提取到 `core/db` 包作为 `GameTemplate` 的方法）
5. `cmd/server/main.go` 注册 `platform/play/` 路由，传入 `engine`（实现 `SessionCreator`）
6. 从 `engine/api/routes.go` 删除已迁移的路由
7. `go build ./...` 验证

整个迁移不改变任何 API 路径，不需要前端配合。
