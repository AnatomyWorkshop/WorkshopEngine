# GW 后端架构审查与工作计划

> 版本：2026-04-08
> 来源：从 GW-SHARED-INFRA-PLAN.md 提取并扩展为完整执行计划。
> 范围：API 路径清理 + core/ 层重整 + social/ 实现路径。

---

## 一、当前架构问题清单

### 问题 1：`/api/v2` 是历史包袱，应当删除

**现状：** 所有路由挂在 `/api/v2/...` 下。
`v2` 来源于文件夹名 `backend-v2`（第二次重写），不是 API 版本号的语义。

**影响：** 给前端和外部消费者传递了"存在 v1"或"将来有 v3"的错误信息。
这是目前唯一的后端，另一个旧后端最终将删除，不存在版本迁移场景。

**目标：** 路由改为 `/api/play/...`、`/api/create/...`、`/api/social/...`，去掉版本前缀。

---

### 问题 2：`core/tokenizer` 不属于 core/

**现状：** `internal/core/tokenizer/estimate.go` 放在 core 层。
**实际调用方：** 仅 engine 内部三个文件：
- `internal/engine/api/engine_methods.go`
- `internal/engine/api/verifier.go`
- `internal/engine/memory/store.go`

**没有任何 platform、creation、social 包使用它。**
Token 估算是 LLM Prompt 管理的概念，属于引擎内部逻辑，不是基础设施。

**目标：** 移动到 `internal/engine/tokenizer/`，更新 3 处 import。

---

### 问题 3：`core/db/models.go` 混入了 engine 私有模型

**现状：** 一个文件里定义了 28 个结构体，其中：

| 分类 | 模型 | 说明 |
|------|------|------|
| **真正共享**（social/creation 需要读） | `GameTemplate`, `Material` | 验证 game_id、搜索素材 |
| **创作工具层**（creation 专用） | `CharacterCard`, `WorldbookEntry`, `PresetEntry`, `LLMProfile`, `LLMProfileBinding`, `RegexProfile`, `RegexRule`, `PresetTool` | CW 工具管理 |
| **引擎内部**（engine 专用，外层不应感知） | `GameSession`, `Floor`, `SessionBranch`, `MessagePage`, `Memory`, `MemoryEdge`, `PromptSnapshot`, `ToolExecutionRecord` | WE 运行时 |

**影响：** social/ 只需要读 `GameTemplate`，但必须 import 包含 `Floor`、`Memory`、
`WorldbookEntry` 等引擎细节的整个文件——语义污染，违背"social 不感知 engine"原则。

**目标：** 按职责拆分为三个文件（同在 `core/db/` 目录内，不改包名，不影响已有 import）：
```
internal/core/db/
├── models_shared.go    ← GameTemplate, Material（跨层共享）
├── models_engine.go    ← GameSession, Floor, Memory...（engine 私有）
└── models_creation.go  ← CharacterCard, WorldbookEntry, LLMProfile...（creation 专用）
```
同包，**不改任何 import**，只是文件拆分。注释边界清晰。

---

### 问题 4：`core/` 边界定义不清晰

**当前 core/ 实际内容：**

| 包 | 实际用途 | 归属判断 |
|----|---------|---------|
| `core/db` | DB 连接 + 全部模型 | ✅ 连接是共享基础设施，模型需拆分（见问题3）|
| `core/llm` | OpenAI 兼容 HTTP 客户端 | ✅ 共享，platform/provider 依赖它 |
| `core/config` | 环境变量配置加载 | ✅ 共享，main.go 使用 |
| `core/util` | Slugify + ParsePage | ✅ 共享，刚建立 |
| `core/tokenizer` | Token 数量估算 | ❌ engine 专用，应移出（见问题2）|

**`core/` 的正确定义：** 零业务逻辑的基础设施——DB 连接、HTTP 客户端、配置、工具函数。
不是"引擎的公共层"，而是"整个服务的基础层"。

---

## 二、解耦完整性评估

```
当前依赖图（箭头 = import）：

cmd/server/main.go
  ├── core/db       ← ✅
  ├── core/config   ← ✅
  ├── platform/auth ← ✅
  ├── platform/gateway ← ✅
  ├── platform/provider ← ✅
  ├── engine/api    ← ✅
  ├── creation/api  ← ✅
  └── creation/asset ← ✅

platform/provider
  ├── core/db       ← ✅（读 LLMProfile）
  └── core/llm      ← ✅

engine/api
  ├── core/db       ← ✅
  ├── engine/tokenizer ← ✅（已从 core/tokenizer 迁移）
  ├── platform/auth ← ✅
  └── platform/provider ← ✅

creation/api
  ├── core/db       ← ✅
  ├── core/util     ← ✅
  ├── core/llm      ← ⚠️  直接用，未走 platform/provider（可接受，简单场景）
  └── platform/auth ← ✅

social/（待建）
  ├── core/db       ← ✅（只读 GameTemplate）
  ├── core/util     ← ✅
  └── platform/auth ← ✅
  ✗ 绝不 import engine/ 任何子包
```

**结论：** 架构基本正确，核心依赖方向清晰。两个待修复点：
1. `core/tokenizer` → `engine/tokenizer`
2. `core/db/models.go` 单文件拆分为三文件

---

## 三、执行计划

### Task 1：`/api/v2` → `/api/` ✅ 2026-04-08

**影响范围：** `cmd/server/main.go` 的 router group 定义。
注释里的 API 路径字符串（不影响运行，但要更新）。

```go
// 改前
v2 := r.Group("/api/v2")
// 改后
apiGroup := r.Group("/api")
```

**前端同步修改：** `frontend-v2/` 的 base URL 配置。
**工作量：** 10 分钟，1 个文件改 1 行，注释批量更新。

---

### Task 2：`core/tokenizer` → `engine/tokenizer` ✅ 2026-04-08

**步骤：**
1. 新建 `internal/engine/tokenizer/` 目录，移动 `estimate.go`
2. 修改包声明 `package tokenizer`（不变）
3. 更新 3 处 import：`engine_methods.go`、`verifier.go`、`memory/store.go`
4. 删除 `internal/core/tokenizer/` 目录
5. `go build ./...` 验证

**工作量：** 15 分钟。

---

### Task 3：`core/db/models.go` 拆分为三文件 ✅ 2026-04-08

**步骤（同包拆分，零 import 变更）：**

1. 新建 `internal/core/db/models_shared.go`
   - 移入：`GameTemplate`、`Material`
   - 文件顶注释：`// 跨层共享模型：social/ creation/ engine/ 均可读取`

2. 新建 `internal/core/db/models_engine.go`
   - 移入：`GameSession`、`Floor`、`SessionBranch`、`MessagePage`、`Memory`、`MemoryEdge`、`MemoryRelation`、`MemoryType`、`FloorStatus`、`PromptSnapshot`、`ToolExecutionRecord`
   - 文件顶注释：`// Engine 内部运行时模型：仅 engine/ 使用，social/ creation/ 不应依赖`

3. 新建 `internal/core/db/models_creation.go`
   - 移入：`CharacterCard`、`WorldbookEntry`、`PresetEntry`、`LLMProfile`、`LLMProfileBinding`、`RegexProfile`、`RegexRule`、`PresetTool`
   - 文件顶注释：`// 创作工具层模型：creation/ 专用，engine/ 通过 session 引用`

4. 删除原 `models.go`（内容已全部分发）

5. `go build ./...` 验证（同包拆分，不改任何 import，应直接通过）

**工作量：** 30 分钟，纯文件重组。

---

### Task 4：实现 `internal/social/` MVP ✅ 2026-04-08

按 `GW-SHARED-INFRA-PLAN.md` 第八章的完整设计实现。
参考库：`D:\ai-game-workshop\plagiarism-and-secret\Social-Reference\`（Artalk / Gitea / Flarum）

**前置条件：** P-4B（JWT Auth）完成后，写入 API 开放。只读 API 可先上线。

---

#### 4-A  `internal/social/reaction/` — 公共互动能力 ✅ 2026-04-08

**已完成文件：**
- `internal/social/reaction/model.go`：`Reaction` 结构体 + `TargetType`/`Type` 枚举 + `Migrate(db)` + 校验函数
- `internal/social/reaction/service.go`：`Add` / `Remove` / `CountBatch` / `CheckMine`，`syncCount` 原子更新反规范化字段
- `internal/social/reaction/api/routes.go`：4 个端点（见下）
- `cmd/server/main.go`：注入 `reaction.Migrate` + `RegisterReactionRoutes`

**API 端点：**
```
POST   /api/social/reactions/:target_type/:target_id/:type   点赞/收藏（需登录）
DELETE /api/social/reactions/:target_type/:target_id/:type   取消（需登录）
GET    /api/social/reactions/counts?targets=comment:id,...   批量查计数（公开）
GET    /api/social/reactions/mine/:target_type/:target_id    查自己状态（需登录）
```

**target_type 合法值：** `comment` | `forum_post` | `forum_reply`
**type 合法值：** `like` | `favorite`

**关键设计决策（来自 Gitea + Artalk）：**
- UNIQUE INDEX `(target_type, target_id, author_id, type)` 通过原生 SQL 建立（GORM AutoMigrate 不支持多列唯一索引）
- `syncCount`：`like` 操作同步更新目标表的 `vote_up` 字段（`GREATEST(0, vote_up ± 1)`），避免每次 COUNT(reactions)
- `syncCount` 失败静默处理（计数可从 reactions 表重建），不影响主流程
- MVP 去掉 dislike，`favorite` 不加 `vote_up`（用于收藏夹逻辑，独立于点赞热度）
- 匿名用户只读（counts/mine 返回空），写入返回 401

---

#### 4-B  `internal/social/comment/` — 游戏评论区 ✅ 2026-04-08

**已完成文件：**
- `internal/social/comment/model.go`：`Comment` 结构体 + `GameCommentConfig` + `Migrate`
- `internal/social/comment/service.go`：`Create` / `Reply` / `ListByGame`（含嵌套树重组）/ `ListReplies` / `Edit` / `Delete` / `Vote` / `Unvote` / `GetConfig` / `UpsertConfig`
- `internal/social/comment/api/routes.go`：9 个端点（见下）
- `cmd/server/main.go`：注入 `comment.Migrate` + `RegisterCommentRoutes`（共享 reactionSvc 实例）

**API 端点：**
```
POST   /api/social/games/:id/comments           发主楼（需登录）
GET    /api/social/games/:id/comments           主楼列表（?sort=date_desc|date_asc|vote&thread_type=linear|nested）
POST   /api/social/comments/:id/replies         回复（需登录，body: game_id+content）
GET    /api/social/comments/:id/replies         子评论列表（公开，分页）
PATCH  /api/social/comments/:id                编辑（仅作者，5 分钟窗口）
DELETE /api/social/comments/:id                软删除（作者或游戏设计者）
POST   /api/social/comments/:id/vote           点赞（需登录）
DELETE /api/social/comments/:id/vote           取消点赞（需登录）
GET    /api/create/games/:id/comment-config    查配置
PATCH  /api/create/games/:id/comment-config    更新配置（需登录）
```

**关键设计决策：**
- `GameCommentConfig` 放在 `comment/` 包内（非 core/db），因为只有 comment/ 和 creation/ 需要
- 树形重建：主楼查询后，一次 `WHERE root_id IN (...)` 批量取子节点，内存分组（Artalk 双索引方案）
- 线性模式（`thread_type=linear`）跳过树形重建，直接返回主楼列表
- 主楼 `RootID = 自身 ID`（写入后二次 UPDATE 设置）
- 软删除保留 status="deleted" 但不清空 content，保持树形结构完整
- 编辑 5 分钟窗口（`EditDeadline = 5 * time.Minute`）
- `Vote` 直接调用 `reactionSvc.Add`，`vote_up` 由 `syncCount` 自动维护

---

#### 4-C  `internal/social/forum/` — 社区论坛帖子 ✅ 2026-04-08

**已完成文件：**
- `internal/social/forum/model.go`：`Post` 结构体（含 `ContentRaw` 保留原始 Markdown、`SearchVector tsvector;-:migration`）+ `ForumReply` + `Migrate(db)` + `MigrateSQL()` 返回全文搜索触发器 DDL
- `internal/social/forum/service.go`：完整业务逻辑（见下）
- `internal/social/forum/api/routes.go`：9 个端点（见下）
- `internal/social/forum/service_test.go`：6 个纯单元测试（HotScore × 4 + renderContent × 2）
- `cmd/server/main.go`：注入 `forum.Migrate` + `RegisterForumRoutes`（共享 `reactionSvc`）

**API 端点（9 个）：**
```
GET    /api/social/posts                    帖子列表（?game_tag=&type=&sort=hot|new&q=）
POST   /api/social/posts                    发帖（需登录）
GET    /api/social/posts/:id                帖子详情（id 或 slug）
PATCH  /api/social/posts/:id                编辑（仅作者）
DELETE /api/social/posts/:id                软删除（仅作者，archived 状态）
GET    /api/social/posts/:id/replies        盖楼列表（分页）
POST   /api/social/posts/:id/replies        盖楼（需登录）
POST   /api/social/posts/:id/vote           点赞（需登录）
DELETE /api/social/posts/:id/vote           取消点赞（需登录）
```

**设计要点：**
- Content 管线：Goldmark（MD→HTML）→ Bluemonday（`UGCPolicy` 净化 XSS）→ 存库；`ContentRaw` 保留原始 Markdown 供编辑回显
- 热度公式：`HotScore(replies, votes int, createdAt) float64` = `(replies*2 + votes) / pow(ageHours+2, 1.5)` — Hacker News 变体，已导出供单元测试
- 楼层序号：`SELECT ... FOR UPDATE` 悲观锁保证 `Number = RepliesCount + 1` 在并发下正确
- `search_vector tsvector` 列用 `gorm:"-:migration"` 跳过 AutoMigrate，由 PostgreSQL 触发器维护；`MigrateSQL()` 返回触发器 DDL，**需在首次部署后手动执行一次**
- gameTag 过滤：`WHERE ? = ANY(game_tags)` PostgreSQL 数组操作符
- MVP 搜索 fallback：全文搜索不可用时降级为 `ILIKE`

**参考来源：** Flarum 的 RepliesCount/LastReplyAt 反规范化字段设计 + Gitea 的 goldmark/bluemonday 管线

---

#### 4-D  `GameCommentConfig` 归属决策 ✅ 2026-04-08

**原计划：** 放在 `internal/core/db/models_social.go`
**实际决策：** 放在 `internal/social/comment/model.go`（`package comment`）

**原因：** 只有 `comment/` 和 `creation/` 两个包需要这个结构体。放进 `core/db/` 会让 core 层感知 social 业务逻辑，违背 core 是"零业务逻辑基础设施"的原则。两个包直接 import `comment/` 即可，不需要绕道 core。

**结果：** 跳过创建 `models_social.go`，`core/db/` 保持清洁。

---

#### 4-E  聚合路由注册 ✅ 2026-04-08

**原计划：** 创建 `internal/social/api/routes.go` 统一注册三个子包路由。
**实际决策：** 直接在 `cmd/server/main.go` 注册，不创建 `social/api/` 聚合包。

**原因：** 三个社交包各自的 `api/routes.go` 已经足够清晰，main.go 是服务唯一组装点，直接在此注册避免了一个额外的间接层，代码更易追踪。

---

#### 4-F  `cmd/server/main.go` 注册 ✅ 2026-04-08

**已完成：**
```go
// Social 层迁移
reaction.Migrate(gormDB)
comment.Migrate(gormDB)
forum.Migrate(gormDB)

// 服务实例（共享 reactionSvc）
reactionSvc := reaction.New(gormDB)
forumSvc    := forum.New(gormDB, reactionSvc)
commentSvc  := comment.New(gormDB, reactionSvc)

// 路由注册
reactionapi.RegisterReactionRoutes(api, reactionSvc)
commentapi.RegisterCommentRoutes(api, commentSvc)
forumapi.RegisterForumRoutes(api, forumSvc)

// 聚合 stats 端点（唯一跨包调用点）
api.GET("/social/games/:id/stats", func(c *gin.Context) { ... })
```

---

#### 4-G  执行顺序（已完成）✅ 2026-04-08

```
Step 1: reaction/ model + service + api  ✅
Step 2: comment/ model + service + api   ✅
Step 3: forum/ model + service + api     ✅
Step 4: GameCommentConfig 放在 comment/ 包内（跳过 models_social.go）✅
Step 5: 直接在 main.go 注册（跳过 social/api/routes.go 聚合）✅
Step 6: go get bluemonday + goldmark（sensitive 留作 TODO）✅
Step 7: 全文搜索触发器 SQL — 见 forum.MigrateSQL()，需部署后手动执行一次 ⚠️
```

---

### Task 5：引入必要的外部依赖 ✅ 2026-04-08

```bash
go get github.com/microcosm-cc/bluemonday   # HTML 净化（防 XSS）✅ v1.0.27
go get github.com/yuin/goldmark              # Markdown 渲染（论坛帖子）✅ v1.8.2
# go get github.com/importcjj/sensitive     # 敏感词 DFA 过滤 — MVP 暂缓，留作 TODO
```

---

## 四、执行顺序与依赖关系

```
Task 2（tokenizer 移动）      — 独立，随时可做
Task 3（models 拆分）         — 独立，随时可做
Task 1（路由路径）             — 独立，最好在 Task 4 之前做

Task 4（social 实现）         — 依赖 Task 1 完成（路径统一）
Task 5（外部依赖）             — 在 Task 4 开发时同步引入
```

**建议执行顺序：** Task 3 → Task 2 → Task 1 → Task 4 + Task 5

---

## 五、不做的事（明确排除）

| 事项 | 原因 |
|------|------|
| 重命名 `backend-v2` 目录 | 只是开发工作区名称，不影响运行时 |
| 把 engine 模型移出 `core/db/` | 同包三文件拆分已解决语义问题，import 零变动 |
| creation/api 接入 platform/provider | 当前直接用 core/llm 可接受，多 slot 需求出现时再改 |
| 给 social/ 加缓存层（Redis）| 写 Reaction 时同步更新 VoteUp 计数即可，当前规模无需 Redis |
