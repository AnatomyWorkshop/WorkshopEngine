# Platform 层架构说明

> 版本：2026-04-04

---

## 为什么需要 Platform 层

`backend-v2` 最终会有多个业务后端并行运行，它们有不同的职责，但共享同一组技术基础：

| 业务后端 | 路径 | 职责 |
|---------|------|------|
| `engine` | `internal/engine/` | 游戏主循环、Session/Floor/Page 生命周期 |
| `creation` | `internal/creation/` | 角色卡、模板、世界书、素材管理 |
| `social`（规划中）| `internal/social/` | 论坛、点赞、分享 |
| `admin`（规划中）| `internal/admin/` | 后台管理 |

这些后端不应各自实现认证、日志、限流、LLM Provider 管理。**Platform 层**是它们共用的技术基础，规则是：

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
  internal/engine  internal/creation   internal/social …   ← 业务后端
        │               │                    │
        └───────────────┼────────────────────┘
                        │ 引用（单向）
        ┌───────────────┼────────────────────┐
        ▼               ▼                    ▼
  platform/gateway  platform/auth    platform/provider     ← Platform 层
        │               │                    │
        └───────────────┼────────────────────┘
                        │ 引用（单向）
              ┌─────────┼──────────┐
              ▼         ▼          ▼
         core/db    core/llm  core/config                  ← 底层基础设施
```

---

## 各 Platform 包职责

### `internal/platform/gateway`

**职责**：HTTP 请求生命周期管理，适用于所有 Gin 路由组。

| 中间件 | 说明 |
|--------|------|
| `RequestID()` | 为每个请求注入唯一 ID（`X-Request-ID` header），传播到日志和响应 |
| `StructuredLogger()` | JSON 结构化日志（method/path/status/duration/request_id/account_id） |
| `Recovery()` | Panic 恢复（记录 stack trace，返回 500） |

**使用方式**（在 `cmd/server/main.go`）：
```go
r := gin.New()
r.Use(gateway.Recovery())
r.Use(gateway.RequestID())
r.Use(gateway.StructuredLogger())
```

---

### `internal/platform/auth`

**职责**：鉴权与账户上下文注入，替代旧的 `internal/user` 包。

**核心类型**：
```go
type Config struct {
    Mode           Mode   // off | api_key | multi_key
    AdminKey       string // Mode=api_key 时校验
    APIKeys        []string  // Mode=multi_key 时允许的 key 列表
    KeyAccountMap  map[string]string // key → account_id 映射
    AllowAnonymous bool
}
type Mode string
const (
    ModeOff      Mode = "off"
    ModeAPIKey   Mode = "api_key"
    ModeMultiKey Mode = "multi_key"
)
```

**解析优先级**：
1. `ADMIN_KEY` → 单 key，无账户映射（开发/单用户模式）
2. `AUTH_API_KEYS` + `AUTH_KEY_ACCOUNT_MAP` → 多 key，每个 key 对应一个 account_id（多租户）
3. 不配置 → `ModeOff`（开发放行）

**X-Account-ID 读取顺序**：
1. `X-Account-ID` header
2. `account_id` query param
3. key-account 映射（mode=multi_key 时自动注入）
4. `"anonymous"`（AllowAnonymous=true 时）

---

### `internal/platform/provider`

**职责**：多 Provider LLM 注册表 + 按账户/会话/插槽解析活跃配置。

**核心思路**（对齐 TH `OrchestrationFactory`）：

```
Registry（静态，来自环境变量）
  └─ Provider "default"  → env-configured LLM client
  └─ Provider "memory"   → cheap model for consolidation

ResolveForSlot(db, accountID, sessionID, slot)（动态，查 DB）
  优先级：
  1. session-scope + slot 精确匹配
  2. global-scope  + slot 精确匹配
  3. session-scope + slot = "*"
  4. global-scope  + slot = "*"
  5. Registry.Default()（env 配置兜底）
```

**支持的 slot 名**（对齐 TH）：
- `*` — 通配，所有场合
- `narrator` — 主叙事生成
- `memory` — 记忆摘要整合（通常用廉价模型）

**ResolveForSlot 返回** `(client *llm.Client, opts llm.Options, ok bool)`：
- `ok=false` 时调用方应使用 env 默认客户端
- `opts` 中已合并 profile params + binding params

**使用方式**（在 `GameEngine`）：
```go
// 创建时注入
engine := api.NewGameEngine(db, defaultClient, registry, ...)

// PlayTurn 内：
client, opts, ok := engine.registry.ResolveForSlot(db, accountID, sessionID, "narrator")
if !ok {
    client, opts = engine.llmClient, engine.defaultOpts
}
// 再叠加 req.GenerationParams
applyGenParams(&opts, req.GenerationParams)
llmResp, _ = client.Chat(ctx, messages, opts)
```

---

### `internal/engine/memory/worker.go`

**职责**：记忆整合 Worker 的完整生命周期（替代 `cmd/worker/main.go` 中的内联逻辑）。

**设计原则**：
- `Worker` 是一个可独立运行的服务对象，通过 `Run(ctx)` 启动
- `cmd/worker/main.go` 只负责配置加载和 `Worker.Run()` 调用（瘦 main）
- **In-memory lease**：处理中的 session ID 放入 `sync.Map`，防止同批次重复处理
- **批次并发**：最多 `MaxConcurrent` 个 goroutine 同时调用 LLM（防止 rate limit）
- **Graceful shutdown**：context 取消后等待所有 in-flight goroutine 完成

**配置字段**（来自 `config.WorkerConfig`）：

| 字段 | 环境变量 | 默认值 | 说明 |
|------|---------|--------|------|
| `TriggerRounds` | `MEMORY_TRIGGER_ROUNDS` | 10 | 每 N 回合触发整合 |
| `MaxTokens` | `MEMORY_MAX_TOKENS` | 512 | 摘要 LLM 输出上限 |
| `TokenBudget` | `MEMORY_TOKEN_BUDGET` | 600 | 摘要注入 token 预算 |
| `BatchSize` | `MEMORY_WORKER_BATCH_SIZE` | 20 | 每次扫描最多处理几个 session |
| `MaxConcurrent` | `MEMORY_WORKER_MAX_CONCURRENT` | 4 | 最大并发 LLM 调用数 |
| `PollInterval` | `WORKER_POLL_INTERVAL_SEC` | 30 | 扫描间隔秒数 |
| `LeaseTTL` | `MEMORY_WORKER_LEASE_TTL_SEC` | 120 | 会话处理租约有效期（防双处理） |

**Worker 生命周期**：
```
main() → Worker.Run(ctx)
           ├─ 立即执行一次 processBatch
           ├─ ticker.C → processBatch（每 PollInterval）
           └─ ctx.Done() → 等待 in-flight goroutine → 退出
```

---

## 迁移说明

| 旧位置 | 新位置 | 操作 |
|--------|--------|------|
| `internal/user/middleware.go` | `internal/platform/auth/middleware.go` | 移动并增强（支持 multi_key 模式）|
| `cmd/worker/main.go`（内联 loop） | `internal/engine/memory/worker.go` | 提取为 Worker 结构体 |
| `game_loop.go` `resolveSlot()` | `internal/platform/provider/registry.go` | 提取并优化（单 SQL 查询）|
| CORS 硬编码 `cmd/server/main.go` | `internal/platform/gateway/` | 纳入网关中间件体系 |

旧的 `internal/user/` 保留为向后兼容的薄层（仅重新导出 `platform/auth` 的类型）。

---

## 近期待实现功能

| 优先级 | 功能 | 位置 |
|--------|------|------|
| ⚡ 本次 | Provider Registry + slot 解析 | `platform/provider` |
| ⚡ 本次 | Worker 结构体重写 | `engine/memory/worker.go` |
| ⚡ 本次 | Gateway 中间件（RequestID + 结构化日志）| `platform/gateway` |
| ⚡ 本次 | Auth 增强（multi_key 模式）| `platform/auth` |
| 📋 中期 | Entry-based Preset 系统 | `engine/preset` |
| 📋 中期 | Memory 衰减 + 维护策略 | `engine/memory` |
| 📋 中期 | Prompt Dry-Run 端点 | `engine/api` |
| 📋 中期 | API Key 加密存储 | `platform/auth` + `core/db` |
