# GW 共享基础设施计划 — 哪些需要提取到公共包

> 版本：2026-04-08
> 问题：social/ 启动时，哪些功能和 engine/creation 重叠，需要提前移到 core/ 或 platform/?
> 依据：基于对当前代码的实际扫描，而非假设。

---

## 一、当前包结构的现状评估

先确认哪些已经做对了，不需要动：

```
internal/
├── core/
│   ├── db/          ✅ DB 连接 + 所有引擎模型，engine/creation/social 均可 import
│   ├── llm/         ✅ LLM 客户端，供 engine + provider 使用
│   ├── tokenizer/   ✅ Token 估算，engine 专用，social 不需要
│   └── config/      ✅ 配置加载，main.go 统一调用
│
├── platform/
│   ├── auth/        ✅ 多模式鉴权中间件，engine/creation 已正确使用
│   ├── gateway/     ✅ RequestID / StructuredLogger / Recovery，全层共用
│   └── provider/    ✅ LLM Provider 注册表 + Slot 解析，engine 专用
│
├── engine/          ✅ WE 本体，不对外暴露内部包
├── creation/        ✅ CW 工具层，使用 platform/auth + core/db
├── social/          ⬜ 待建
└── user/            ⚠️  deprecated shim（见下文）
```

**结论：platform/ 层的设计是正确的。**
engine 和 creation 已经通过 `platform/auth.GetAccountID(c)` 读取账户 ID，
通过 `platform/gateway` 中间件处理请求生命周期。
social/ 接入时直接复用，无需重复。

---

## 二、需要提取的三处重叠（✅ 已完成）

---

### 2-A  `slugify()` → `core/util/slug.go` ✅ 2026-04-08

**已完成：**
- 新建 `internal/core/util/slug.go`，导出 `Slugify(name, fallbackPrefix string) string`
- `creation/api/routes.go` 删除本地 `slugify()` 定义，替换为 `util.Slugify(name, "card")` 和 `util.Slugify(name, "game")`
- 同步移除 `unicode`、`fmt` 两个无用 import

**social 直接调用：** `util.Slugify(post.Title, "post")`

---

### 2-B  分页参数解析 → `core/util/pagination.go` ✅ 2026-04-08

**已完成：**
- 新建 `internal/core/util/pagination.go`，导出 `ParsePage(c *gin.Context) (limit, offset int)`
- 默认 20，上限 200，负值校正为 0
- social/api handler 直接调用，engine/api 按需迁移

---

### 2-C  CORS 中间件 → `platform/gateway/cors.go` ✅ 2026-04-08

**已完成：**
- 新建 `internal/platform/gateway/cors.go`，导出 `CORS(cfg CORSConfig) gin.HandlerFunc`
- `cmd/server/main.go` 替换匿名闭包为 `r.Use(gateway.CORS(gateway.CORSConfig{AllowedOrigins: cfg.Server.CORSOrigins}))`

---

## 三、遗留包清理（✅ 已完成）

### 3-A  `internal/user/` — deprecated shim ✅ 2026-04-08

**已完成：** grep 确认无调用方，直接删除整个目录。
所有包均直接 import `mvu-backend/internal/platform/auth`（正确路径）。

---

## 四、social/ 的 DB 模型策略

**问题：** social 的模型（Comment、Post、Reaction 等）应该放在哪里？

**答案：不放进 `core/db/models.go`。**

`core/db/models.go` 当前存放的是引擎模型（GameTemplate、GameSession、Floor 等）。
如果把社区模型也堆进去，这个文件会变成一个无边界的"全局模型仓库"，
违背 social 层与 engine 层独立的原则。

**推荐策略：**
```
internal/social/comment/model.go   ← Comment, ForumReply 模型定义
internal/social/forum/model.go     ← Post 模型定义
internal/social/reaction/model.go  ← Reaction 模型定义
```

每个包在自己的 `model.go` 里调用 `db.AutoMigrate(&Comment{})` 等，
由 `cmd/server/main.go` 在启动时按顺序调用各包的 `Migrate(db)` 函数。

`core/db/connect.go` 只负责建立连接，返回 `*gorm.DB`，
不再包含 AutoMigrate 调用列表（或仅包含引擎核心模型）。

**social 读取引擎模型的方式：** 直接 import `core/db`，读取 `GameTemplate` 等结构体，
用于验证 `game_id` 是否存在。这是允许的——`core/` 是最底层，所有包都可以 import。

---

## 五、platform/provider/ 与 social 的关系

`platform/provider/registry.go` 管理 LLM Provider 和 Slot 解析，当前 engine 使用。

**social 层不需要 LLM。**
评论、点赞、帖子都是纯 CRUD，不调用任何 AI 接口。

**creation 层有 AI 辅助创作功能**（creation-agent），如果它需要调用 LLM，
应通过 `platform/provider` 解析，而不是自己硬编码。目前 creation/api/routes.go
直接 import `core/llm`，这在简单场景下可以接受，
但如果未来创作工具要支持多 slot（Director 预分析 + Narrator 生成），
应切换到 `platform/provider.Registry`。

**结论：** `platform/provider` 目前只属于 engine，creation 按需扩展，social 不涉及。

---

## 六、social/ 启动时完整的 import 依赖图

```
internal/social/comment/
    ├── import "mvu-backend/internal/core/db"       ← DB 连接 + GameTemplate 模型
    ├── import "mvu-backend/internal/core/util"     ← Slugify, ParsePage（待建）
    ├── import "mvu-backend/internal/platform/auth" ← GetAccountID
    └── [不 import internal/engine/ 任何子包]

internal/social/forum/
    ├── import "mvu-backend/internal/core/db"
    ├── import "mvu-backend/internal/core/util"
    ├── import "mvu-backend/internal/platform/auth"
    └── [不 import internal/engine/ 任何子包]

internal/social/reaction/
    ├── import "mvu-backend/internal/core/db"
    ├── import "mvu-backend/internal/platform/auth"
    └── [不 import internal/engine/ 任何子包]
```

`cmd/server/main.go` 在 `RegisterGameRoutes` 和 `RegisterCreationRoutes` 之后：
```go
socialapi.RegisterSocialRoutes(v2, gormDB)
```

---

## 七、执行优先级（✅ P1/P2 已完成，以下为新增阶段）

| 优先级 | 任务 | 状态 |
|--------|------|------|
| P1 | 删除 `internal/user/` shim | ✅ 已完成 |
| P2 | 建 `core/util/slug.go` + `pagination.go` | ✅ 已完成 |
| P2 | 将 CORS 移入 `platform/gateway/cors.go` | ✅ 已完成 |
| P3 | 实现 `internal/social/comment/` 包（见第八章）| ⬜ 下一步 |
| P3 | 实现 `internal/social/forum/` 包（见第八章）| ⬜ 下一步 |
| P3 | 实现 `internal/social/reaction/` 包（见第八章）| ⬜ 下一步 |
| P4 | creation AI 接入 `platform/provider` | 按需 |

---

## 八、social/ 完整实现计划（基于参考库提取）

> 参考来源：Artalk（MIT, Go）+ Gitea reaction.go（MIT, Go）+ Flarum migrations（MIT, PHP）
> 克隆位置：`D:\ai-game-workshop\plagiarism-and-secret\Social-Reference\`

---

### 8-A  `internal/social/comment/` — 游戏评论区

**数据模型（综合 Artalk entity/comment.go 设计）：**

```go
// internal/social/comment/model.go
type Comment struct {
    ID          string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    GameID      string    `gorm:"not null;index"`          // 强绑定游戏
    AuthorID    string    `gorm:"not null;index"`
    Content     string    `gorm:"not null"`
    Rid         string    `gorm:"index"`                   // 直接父节点 ID（Artalk: Rid）
    RootID      string    `gorm:"index"`                   // 根节点 ID，加速树形查询（Artalk: RootID）
    ThreadType  string    `gorm:"default:'linear'"`        // linear | nested
    IsCollapsed bool      `gorm:"default:false"`
    IsPinned    bool      `gorm:"default:false"`
    IsPending   bool      `gorm:"default:false"`           // 待审核
    VoteUp      int       `gorm:"default:0"`               // 反规范化计数（Artalk 方案）
    Status      string    `gorm:"default:'visible'"`       // visible | hidden | deleted
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

**关键设计决策（来自 Artalk）：**
- `Rid` = 直接父节点（楼中楼的上层），`RootID` = 主楼 ID
- 树形重建：`WHERE root_id IN (主楼IDs) AND rid != ''` 一次查全子节点
- 线性模式：`rid` 始终为空，只用 `created_at DESC` 排序
- `VoteUp` 反规范化到 Comment 行，避免每次 COUNT(reactions)

**排序枚举（来自 Artalk sort.go）：**
```go
const (
    SortDateDesc = "date_desc"   // created_at DESC（默认）
    SortDateAsc  = "date_asc"    // created_at ASC
    SortVote     = "vote"        // vote_up DESC, created_at DESC
)
```

**API：**
```
POST   /api/social/games/:id/comments          发主楼
GET    /api/social/games/:id/comments          列表（?sort=date_desc|vote&thread_type=）
POST   /api/social/comments/:id/replies        楼中楼回复
GET    /api/social/comments/:id/replies        子评论列表
PATCH  /api/social/comments/:id               编辑（仅作者，5 分钟内）
DELETE /api/social/comments/:id               删除（作者或游戏设计者）
POST   /api/social/comments/:id/vote          点赞（+1 VoteUp，写 reactions 表）
```

---

### 8-B  `internal/social/forum/` — 社区论坛帖子

**数据模型（综合 Flarum discussions + posts 迁移）：**

```go
// internal/social/forum/model.go
type Post struct {
    ID            string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    AuthorID      string    `gorm:"not null;index"`
    Title         string    `gorm:"not null"`
    Slug          string    `gorm:"uniqueIndex"`          // URL slug，来自 util.Slugify
    Content       string    `gorm:"not null"`
    GameTags      []string  `gorm:"type:text[]"`          // 弱关联游戏，可空
    PostType      string    `gorm:"default:'discussion'"` // discussion|guide|journal|fanart
    Status        string    `gorm:"default:'published'"`  // published|draft|archived
    // 反规范化缓存（来自 Flarum discussions 表设计）
    RepliesCount  int       `gorm:"default:0"`            // 避免 COUNT JOIN
    LastReplyAt   *time.Time                              // 快速排序热帖
    LastReplyUser string                                  // 显示最后回复者
    // 全文搜索向量（PostgreSQL tsvector）
    SearchVector  string    `gorm:"type:tsvector;-:migration"` // 由触发器维护
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type ForumReply struct {
    ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    PostID    string    `gorm:"not null;index"`
    AuthorID  string    `gorm:"not null"`
    ParentID  string    `gorm:"index"`                    // 楼中楼，可空
    Number    int       `gorm:"not null"`                  // 楼层序号（来自 Flarum posts.number）
    Content   string    `gorm:"not null"`
    VoteUp    int       `gorm:"default:0"`
    Status    string    `gorm:"default:'visible'"`
    CreatedAt time.Time
}
```

**关键设计决策（来自 Flarum）：**
- `RepliesCount` + `LastReplyAt` 反规范化到 Post，热帖排序无需 JOIN
- `ForumReply.Number` 楼层序号方便引用（"3楼"）
- `SearchVector` 由 PostgreSQL 触发器自动维护，`tsvector` 全文索引

**迁移时补充的 SQL（非 GORM AutoMigrate）：**
```sql
-- 全文搜索触发器（在 AutoMigrate 后手动执行一次）
CREATE INDEX IF NOT EXISTS posts_search_idx ON posts USING GIN(search_vector);
CREATE OR REPLACE FUNCTION update_post_search() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('simple', COALESCE(NEW.title,'') || ' ' || COALESCE(NEW.content,''));
  RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER post_search_trigger BEFORE INSERT OR UPDATE ON posts
  FOR EACH ROW EXECUTE FUNCTION update_post_search();
```

**API：**
```
GET    /api/social/posts                    列表（?game_tag=&type=&sort=hot|new&q=）
POST   /api/social/posts                    发帖
GET    /api/social/posts/:id                详情
PATCH  /api/social/posts/:id                编辑（仅作者）
DELETE /api/social/posts/:id                删除
GET    /api/social/posts/:id/replies        盖楼列表（分页）
POST   /api/social/posts/:id/replies        盖楼
```

---

### 8-C  `internal/social/reaction/` — 公共互动能力

**数据模型（综合 Gitea reaction.go + Artalk vote.go）：**

Gitea 方案（IssueID/CommentID 分离）适合强类型系统；
Artalk 方案（TargetID uint + VoteType enum）更简洁但类型不安全。

**我们采用中间方案（多态字符串 ID + 类型枚举）：**

```go
// internal/social/reaction/model.go
type Reaction struct {
    ID         string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    TargetType string    `gorm:"not null;index"`  // comment | forum_post | forum_reply
    TargetID   string    `gorm:"not null;index"`  // 对应表的 UUID
    AuthorID   string    `gorm:"not null;index"`
    Type       string    `gorm:"not null"`        // like | dislike（暂不做 coin）
    CreatedAt  time.Time
}
// UNIQUE (target_type, target_id, author_id, type)
// 来自 Gitea reaction.go UNIQUE(s) 约束设计
```

**两个错误类型（来自 Gitea）：**
```go
type ErrReactionAlreadyExist struct{ Type string }  // 409
type ErrReactionForbidden    struct{ Type string }  // 403
```

**计数更新策略：**
- 创建/删除 reaction 时同步更新 Comment.VoteUp 或 Post 的 likes 缓存字段
- 保持反规范化计数同步，避免每次 COUNT(reactions)（来自 Artalk/Flarum 共同实践）

**API：**
```
POST   /api/social/reactions            { target_type, target_id, type: "like" }
DELETE /api/social/reactions            { target_type, target_id, type: "like" }
GET    /api/social/reactions/counts     ?targets=comment:id1,forum_post:id2（批量查询）
```

---

### 8-D  `game_comment_config` — 游戏评论区配置

```go
// 由游戏设计者通过 create API 管理
type GameCommentConfig struct {
    GameID          string   `gorm:"primaryKey"`
    EnabledModes    []string `gorm:"type:text[]"`  // ["linear","nested"]
    DefaultMode     string   `gorm:"default:'linear'"`
    AllowAnonymous  bool     `gorm:"default:false"`
    RequireApproval bool     `gorm:"default:false"` // IsPending 流程开关
}
```

**创作者 API：**
```
GET    /api/create/games/:id/comment-config
PATCH  /api/create/games/:id/comment-config
```

---

### 8-E  AutoMigrate 注册方式

social 层各包提供独立 `Migrate(db)` 函数，main.go 按序调用：

```go
// cmd/server/main.go 新增（在 db.Connect 之后）
comment.Migrate(gormDB)
forum.Migrate(gormDB)
reaction.Migrate(gormDB)
// 然后手动执行一次全文搜索触发器 SQL（仅首次部署）
```

core/db/connect.go 的 AutoMigrate 只保留引擎核心模型，不堆入 social 模型。
