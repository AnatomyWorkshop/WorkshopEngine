# GameWorkshop — Social 层包结构设计

> 版本：2026-04-08
> 定位：`internal/social/` 内部包结构的设计思路。
> 核心问题：社区论坛（首页帖子）与游戏评论区（详情页）如何拆包？

---

## 一、两个系统的本质区别

GW 的互动内容在产品上分属两个不同的上下文：

```
首页 / 社区发现页               游戏详情页
──────────────────────         ──────────────────────
论坛帖子（可打游戏标签）          游戏评论区（绑定单游戏）
  - 类比：小黑盒「游戏帖子」         - 类比：小黑盒「游戏评测/评分」
  - 类比：B站动态/专栏               - 类比：B站视频评论区
  - 作者自发创作                     - 玩家针对游戏发表
  - 可同时挂多个游戏标签             - 只属于一个游戏
  - 平台层内容发现                   - 游戏层反馈聚合
```

**关键差异：绑定模型不同。**

| 维度 | 游戏评论区 (comment) | 社区论坛 (forum) |
|------|---------------------|-----------------|
| 游戏关联 | `game_id` 强绑定（必填，索引） | `game_tags[]` 弱关联（可选，可多个） |
| 内容形态 | 短评、打分、即时反馈 | 长帖、攻略、游记、泛议题 |
| 存在前提 | 必须有对应游戏 | 可以独立存在，游戏标签只是过滤维度 |
| 发现路径 | 进入游戏详情页才看到 | 首页信息流、分类浏览可直接发现 |
| 权限归属 | 游戏设计者可配置评论区形态 | 平台统一规则，作者自主 |
| 嵌套层级 | 评论 → 回复（2层或线性）| 主帖 → 盖楼回复（多层 Thread） |

---

## 二、为什么不合并为「游戏互动页」包

"游戏互动页"是前端**视图的聚合**：详情页把评论区 + 本游戏的论坛帖子列表放在一起展示。
这是 API 层的查询聚合，不是包的职责边界。

如果合并，会产生：
- 论坛帖子的权限逻辑（作者自主）和评论区的权限逻辑（游戏设计者可控）混在一个包里
- `game_tags[]` 弱关联的查询逻辑和 `game_id` 强绑定的查询逻辑混在一起
- 首页论坛帖子没有 `game_id`，强行填写会破坏数据模型语义

**结论：按数据绑定关系拆包，视图聚合在 API handler 层完成。**

---

## 三、推荐包结构

```
internal/social/
│
├── comment/           ← 游戏评论区（强绑定 game_id）
│   ├── model.go       ← Comment 表（含 thread_type: nested|linear）
│   ├── service.go     ← 发评论、回复、查树形列表
│   └── api/           ← HTTP handler
│
├── forum/             ← 社区论坛/帖子（弱关联 game_tags[]）
│   ├── model.go       ← Post 表（title, content, game_tags, type）
│   ├── reply.go       ← ForumReply 表（盖楼，parent_id 树形）
│   ├── service.go     ← 发帖、盖楼、标签搜索
│   └── api/           ← HTTP handler
│
├── reaction/          ← 公共互动能力（点赞/收藏，多态）
│   ├── model.go       ← Reaction 表（target_type + target_id 多态）
│   └── service.go     ← 点赞、取消、计数
│
├── roundtable/        ← 话题圆桌（Phase 2）
│   └── ...
│
└── profile/           ← 用户标签/兴趣（Phase 2，TagBased 排序前置）
    └── ...
```

**严格依赖规则：**
- `comment/`、`forum/`、`roundtable/` 互不依赖
- 三者都可以引用 `reaction/`（公共能力）
- 没有一个包引用 `internal/engine/` 或 `internal/creation/`

---

## 四、数据模型对比

### comment/ — 游戏评论区

```go
type Comment struct {
    ID         string    `gorm:"primaryKey"`
    GameID     string    `gorm:"not null;index"`   // 强绑定，必填
    AuthorID   string    `gorm:"not null"`
    ParentID   *string   // NULL = 主楼
    RootID     *string   // 加速楼中楼查询
    ThreadType string    `gorm:"default:'linear'"` // nested | linear
    Content    string
    Status     string    `gorm:"default:'visible'"`
    CreatedAt  time.Time
}
```

### forum/ — 社区论坛

```go
type Post struct {
    ID        string    `gorm:"primaryKey"`
    AuthorID  string    `gorm:"not null"`
    Title     string
    Content   string
    GameTags  []string  `gorm:"type:text[]"`   // 可空，可多值，弱关联
    PostType  string    // guide | journal | discussion | fanart
    Status    string    `gorm:"default:'published'"`
    CreatedAt time.Time
    UpdatedAt time.Time
}

type ForumReply struct {
    ID       string   `gorm:"primaryKey"`
    PostID   string   `gorm:"not null;index"` // 归属帖子
    AuthorID string   `gorm:"not null"`
    ParentID *string  // 盖楼：指向上一楼的回复
    Content  string
    CreatedAt time.Time
}
```

**区别：**
- `Comment.GameID` 是必填强绑定 → 游戏不存在则评论区不存在
- `Post.GameTags` 是可空数组 → 帖子可以和任意数量游戏关联，也可以完全不关联游戏

---

## 五、API 命名空间设计

```
/api/social/

# 游戏评论区（comment/ 包）
GET    /games/:id/comments              游戏评论列表（主楼分页）
POST   /games/:id/comments              发主楼评论
GET    /comments/:id/replies            某楼的回复列表
POST   /comments/:id/replies            盖楼/回复

# 社区论坛（forum/ 包）
GET    /posts                           帖子列表（?game_tag=&type=&sort=）
POST   /posts                           发帖
GET    /posts/:id                       帖子详情
PATCH  /posts/:id                       编辑（仅作者）
DELETE /posts/:id                       删除
GET    /posts/:id/replies               盖楼列表
POST   /posts/:id/replies               盖楼

# 公共互动（reaction/ 包，多态）
POST   /reactions                       点赞/收藏 {target_type, target_id, type}
DELETE /reactions                       取消
GET    /reactions/counts                批量查计数

# 游戏详情页的聚合（API 层组合，不是新包）
GET    /games/:id/stats                 评论数 + 点赞数 + 帖子数聚合
```

**游戏详情页的「评论区 + 相关帖子」是 API handler 层的两次查询聚合，不需要新建包。**

---

## 六、游戏详情页如何聚合两个系统

游戏详情页的「互动 Tab」展示两类内容，由前端分 Tab 请求：

```
游戏详情页 /games/:id
│
├── [评论 Tab]
│   └── GET /social/games/:id/comments     → comment/ 包返回
│
├── [攻略/游记 Tab]
│   └── GET /social/posts?game_tag=:id     → forum/ 包返回（按标签过滤）
│
└── [统计]
    └── GET /social/games/:id/stats        → 聚合两者计数
```

后端 `/social/games/:id/stats` 的实现：
```go
// social/api/stats.go — 唯一需要同时引用 comment/ 和 forum/ 的地方
func GetGameStats(c *gin.Context) {
    gameID := c.Param("id")
    commentCount := commentSvc.CountByGame(gameID)
    postCount    := forumSvc.CountByGameTag(gameID)
    // ... 返回聚合
}
```

这是唯一一处跨包聚合，其他所有业务逻辑在各自包内独立处理。

---

## 七、盖楼形态的归属

"首页论坛的盖楼形式" 属于 `forum/ForumReply`（帖子下的多层回复）。
"游戏评论区的楼中楼" 属于 `comment/Comment`（`parent_id` 树形）。

两种"盖楼"在产品上看起来类似，但数据绑定语义完全不同，放在同一个包会造成模型污染。
`forum/ForumReply` 的存在不依赖任何 `game_id`，可以在纯平台社区帖下盖楼；
`comment/Comment` 的存在必须有对应的 `game_id`。

---

## 八、MVP 启动顺序

```
Step 1（P-4B JWT 完成后）：
  ├── reaction/        点赞/收藏（最简单，先打通能力基础）
  ├── comment/         游戏评论区（BasicComment + Linear，无圆桌）
  └── forum/           社区帖子（发帖 + 盖楼 + 按 game_tag 过滤）

Step 2：
  ├── 排序：hot / new
  └── game_comment_config 游戏评论区配置（游戏设计者选择 nested/linear）

Step 3（Phase 2）：
  ├── roundtable/      话题圆桌
  ├── profile/         用户标签（为 TagBased 排序准备）
  └── Sort&AntiFilter  最像排序 + 反茧房注入
```
