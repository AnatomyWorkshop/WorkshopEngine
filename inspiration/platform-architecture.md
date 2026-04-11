# Platform 层架构说明

> 版本：2026-04-10（路由迁移完成，Social 层已实现）

---

## 为什么需要 Platform 层

`backend-v2` 有多个业务后端并行运行，共享同一组技术基础：

| 业务后端 | 路径 | 职责 |
|---------|------|------|
| `engine` | `internal/engine/` | 游戏主循环、Session/Floor/Page 生命周期、Prompt Pipeline |
| `creation` | `internal/creation/` | 角色卡、模板、世界书、素材管理（CW 创作工具后端）|
| `social` | `internal/social/` | 点赞/收藏、评论、论坛帖子 |

这些后端不应各自实现认证、日志、限流、LLM Provider 管理。**Platform 层**是它们共用的技术基础：

> **Platform 层不依赖任何业务后端；业务后端可以随意引用 Platform 层。**

---

## 层级关系

```
┌─────────────────────────────────────────────────────────┐
│                    cmd/server  cmd/worker                │  ← 可执行入口
└───────────────────────┬─────────────────────────────────┘
                        │ 引用
        ┌───────────────┼────────────────────┐
        ▼               ▼                    ▼
  internal/engine  internal/creation   internal/social     ← 业务后端
        │               │                    │
        └───────────────┼────────────────────┘
                        │ 引用（单向）
        ┌───────────────┼────────────────────┐
        ▼               ▼                    ▼
  platform/gateway  platform/auth    platform/provider     ← Platform 层
  platform/play
        │               │                    │
        └───────────────┼────────────────────┘
                        │ 引用（单向）
              ┌─────────┼──────────┐
              ▼         ▼          ▼
         core/db    core/llm  core/config                  ← 底层基础设施
```

---

## CW → GW 发行流与路由归属

```
CW（创作工具）                    GW（游戏平台）
internal/creation/               internal/platform/play/   internal/engine/
      │                                   │                       │
      │  创作者发布游戏包                   │  玩家发现游戏           │  玩家游玩
      │  PATCH /api/create/templates/:id   │                       │
      │  → status: draft → published      │                       │
      │                                   │  GET /api/play/games  │
      │                                   │  GET /api/play/games/:slug
      │                                   │                       │
      │                                   │  POST /api/play/sessions（创建会话）
      │                                   │  → 调用 engine.CreateSession()
      │                                   │                       │
      │                                   │                       │  POST /api/play/sessions/:id/turn
      │                                   │                       │  GET  /api/play/sessions/:id/stream
      │                                   │                       │  ...（所有执行层路由）
```

**路由归属原则**：

| 路由 | 归属 | 理由 |
|------|------|------|
| `GET /api/play/games` | `platform/play/` ✅ | 纯数据库查询，不需要 LLM |
| `GET /api/play/games/:slug` | `platform/play/` ✅ | 同上，含 comment_config（A-10） |
| `GET /api/play/games/worldbook/:id` | `platform/play/` ✅ | 只读查询 + 权限检查 |
| `POST /api/play/sessions` | `platform/play/` ✅ | 入口逻辑，调用 SessionCreator 接口 |
| `GET /api/play/sessions` | `platform/play/` ✅ | 存档列表，纯查询 |
| `POST /api/play/sessions/:id/turn` | `engine/api/` | 核心执行，需要 Pipeline |
| `GET /api/play/sessions/:id/stream` | `engine/api/` | 核心执行，SSE 流式 |
| `POST /api/play/sessions/:id/suggest` | `engine/api/` | 全管线 Impersonate |
| `POST /api/play/sessions/:id/fork` | `engine/api/` | 执行层操作 |

---

## 各 Platform 包现状

### `internal/platform/gateway` ✅

HTTP 请求生命周期管理。

| 中间件 | 说明 |
|--------|------|
| `RequestID()` | 唯一请求 ID（`X-Request-ID`） |
| `StructuredLogger()` | JSON 结构化日志 |
| `Recovery()` | Panic 恢复 |
| `CORS(config)` | 可配置 CORS（`CORSConfig.AllowedOrigins`） |

### `internal/platform/auth` ✅

鉴权与账户上下文注入。

| 模式 | 触发条件 | 说明 |
|------|---------|------|
| `jwt` | `AUTH_MODE=jwt` + `AUTH_JWT_SECRET` 非空 | JWT HS256，`sub` = account_id |
| `api_key` | `ADMIN_KEY` 非空 | 单 Key，无账户映射 |
| `multi_key` | `AUTH_API_KEYS` 非空 | 多 Key，每个 Key 映射到 account_id |
| `off` | `AUTH_MODE=off` 或均未配置 | 开发放行 |

Token 签发：`POST /api/auth/token`（admin_key 验证后签发 JWT，在 auth 中间件之外）。
X-Account-ID 读取顺序（非 JWT 模式）：header → query param → key-account 映射 → `"anonymous"`。

### `internal/platform/provider` ✅

LLM Profile 动态解析，5 级优先级（session slot X → global slot X → session * → global * → env）。

支持的 slot：`narrator` / `director` / `verifier` / `memory` / `*`（通配）。

### `internal/platform/play` ✅（2026-04-10 新增）

玩家发现层，从 `engine/api/routes.go` 迁移。

| 文件 | 说明 |
|------|------|
| `handler.go` | Handler + SessionCreator 接口 + publicGameView + 5 个 handler |
| `routes.go` | RegisterPlayRoutes（5 条路由） |

关键设计：`SessionCreator` 接口定义在 `platform/play/`，`GameEngine` 天然满足，无需修改 engine。`platform/play/` 可合法 import `social/comment` 读取 comment_config。

---

## 未来工作

### Phase 4 — 安全升级 ✅

| 工作 | 位置 | 编号 | 状态 |
|------|------|------|------|
| API Key AES-256-GCM 加密 | `core/secrets`（新增）+ `creation/api` + `provider` | P-4A | ✅ 2026-04-10 |
| JWT Auth（`AUTH_MODE=off\|jwt`） | `platform/auth` | P-4B | ✅ 2026-04-10 |

### Phase 5 — 包结构治理

| 工作 | 说明 |
|------|------|
| `publicGameView` 提取到 `core/db` | 当第二个包需要相同逻辑时再做（当前仅 `platform/play/` 使用） |
| `platform/user/` 包 | A-11（常驻角色）进入开发计划时决策是否新建 |
| `GET /play/sessions` 分页一致性 | 统一改用 `util.ParsePage(c)` |
