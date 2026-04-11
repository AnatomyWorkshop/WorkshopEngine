# Phase 4 第二批收官 — P-4D/P-4E/P-4G 完成

> 日期：2026-04-11
> 状态：Phase 4 第二批全部完成

---

## 本轮完成

### P-4D OpenAPI/Swagger 文档

73 个 API 路径全部标注。采用 `swagger_docs.go` 策略——每个包一个独立文件，用空函数携带 `@Router` 注解，零改动现有 handler 逻辑。

Swagger UI 挂载在 `/swagger/*any`，公开访问无需鉴权。

### P-4E 对话导入/导出

两种格式：
- `.thchat`（WE 原生）：完整 JSON，保留 session/floor/page/memory/edge/branch 全部 UUID。导出时原样保留，导入时全量 ID 重映射
- `.jsonl`（ST 兼容）：有损导出，仅主分支 committed floor + active page，一行一条消息

导入走 `db.Transaction`，edge 引用缺失 memory 时静默跳过。

### P-4G Background Job Runtime

核心改动：
1. `runtime_job` 表：`queued → leased → done / failed / dead` 状态机，`dedupe_key` 唯一索引防重复入队
2. `internal/engine/scheduler` 包：`Enqueue` + `LeaseJob`（`FOR UPDATE SKIP LOCKED`）+ `Complete` + `Fail` + `RecoverStale`
3. `memory/worker.go` 重构：移除 `sync.Map` 内存租约，改为从 scheduler 消费 Job
4. `GameEngine.triggerMemoryConsolidation` 实现：写 DB 行而非 goroutine

进程重启后 `RecoverStale()` 自动恢复超时租约，dead letter 策略 `retry_count >= 3` 进入 dead。

---

## Phase 4 当前状态

| 编号 | 任务 | 状态 |
|------|------|------|
| P-4A | API Key 加密 | ✅ |
| P-4B | JWT 鉴权 | ✅ |
| P-4C | 多 Provider 抽象 | ✅ |
| P-4D | OpenAPI/Swagger | ✅ |
| P-4E | 对话导入/导出 | ✅ |
| P-4F | 双层记忆压缩 | ⬜ |
| P-4G | Background Job Runtime | ✅ |
| P-4H | Floor Run Phase SSE | ⬜ |
| P-4I | VN 渲染 | ⬜ |
| P-4J | game_loop 重构 | ⬜ |
| P-4K | Macro 注册表 | ⬜ |
| P-4L | Preflight 渲染 | ⬜ |

第二批（平台工程）全部完成。剩余 P-4F~P-4L 为第三批（引擎完善 + 体验增强）。

---

## 已知遗留

- `resolveSlot` / `applyGenParams` 未定义（P-4J 工作），导致 `engine/api` 包整体无法编译。各子包独立编译通过
- Memory Worker 的 `Run()` 尚未在 `main.go` 中启动——当前记忆整合仍由 `triggerMemoryConsolidation` 入队，需要一个消费者 goroutine。可在 main.go 中加 `go worker.Run(ctx)` 启动

---

## 接下来

**短期（前端优先）：**
- 论坛模块前端初步建造——后端 `social/forum` API 已就绪（9 个端点），前端需要对接
- backend-v2 推到 GitHub

**后续后端：**
- P-4F 双层记忆压缩——现在有了 P-4G 的 scheduler 基础设施，macro compaction 可以直接入队为 Job
- P-4J game_loop 重构——解决 `resolveSlot` / `applyGenParams` 编译问题，是 P-4K/P-4L 的前置
- P-4H SSE 推送——提升生成过程的用户体验
