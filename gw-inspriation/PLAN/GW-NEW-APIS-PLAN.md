# GW 新增 API — 实现计划与进度

> 版本：2026-04-10 v2
> 范围：GW 前端开发所需的后端 API 缺口，按优先级排序实现。

---

## 一、总览

| 编号 | API | 状态 | 前端解锁 |
|------|-----|------|---------|
| A-1 | `GET /api/play/games` + `GET /api/play/games/:slug` | ✅ 已完成 | 游戏列表/详情页 |
| A-2 | `GET /api/play/sessions?game_id=` | ✅ 已完成 | 继续游玩存档选择 |
| A-3 | reaction `target_type` 加 `game` | ✅ 已完成 | 收藏/点赞游戏 |
| A-4 | `PATCH /api/play/sessions/:id` (is_public) + floors 范围查询 | ✅ 已完成 | 游记分享 |
| A-5 | `GET /api/play/games/worldbook/:id` | ✅ 已完成 | 世界书面板 |
| A-6 | `GET /api/social/games/:id/stats` | ✅ 已完成（main.go 聚合端点）| StatsBar 评论数/游玩数 |
| A-7 | `GET /api/play/games/:slug` 支持 UUID 查询 | ✅ 已完成（slug OR id::text）| PlayPage 刷新不丢标题 |
| A-8 | `POST /api/play/sessions/:id/suggest`（AI 帮答）| ✅ 已完成 | ChatInput AI 帮答按钮 |
| A-9 | `GET /api/play/sessions/:id` 返回 `game_id` | ✅ 已完成（Session 结构体含 game_id）| PlayPage 游戏数据加载 |
| A-10 | `comment_config` 暴露到 `publicGameView` | ⬜ 待实现 | 评论区模式切换 |
| A-11 | 常驻角色 `GET/POST/DELETE /api/users/:id/resident_character` | 🔜 延后 | 常驻角色迁移 |

---

## 二、已完成

### A-1：公开游戏列表 + 详情

`GET /api/play/games`：分页、标签过滤（JSONB `?`）、类型过滤、`new/hot` 排序，返回 `{ games, total, limit, offset }`

`GET /api/play/games/:slug`：**slug 或 UUID 双路查询**（`slug = ? OR id::text = ?`），只返回 `status = 'published'`

`publicGameView`（`engine_methods.go:1131`）：提取 `config.ui_config`，返回 `id, slug, title, type, short_desc, notes, cover_url, author_id, play_count, like_count, favorite_count, ui_config, created_at`

---

### A-2：用户存档列表

`GET /api/play/sessions?game_id=&user_id=&limit=&offset=`：按 `updated_at DESC` 排序，返回完整 `GameSession`（含 `floor_count`）

---

### A-3：Reaction target_type 加 game

- `TargetGame = "game"` 加入 `validTargetTypes`
- `syncCount`：`game + like` → `game_templates.like_count`，`game + favorite` → `game_templates.favorite_count`

---

### A-4：Session 公开分享 + 楼层范围查询

- `GameSession` 新增 `IsPublic bool`
- `PATCH /api/play/sessions/:id { "is_public": true }`
- `GET /api/play/sessions/:id/floors?from=1&to=20`

---

### A-5：世界书玩家只读 API

`GET /api/play/games/worldbook/:id`
- 检查 `config.allow_player_worldbook_view == true`，否则 403
- 返回 `[{ id, keys, content, comment }]`，按 `priority ASC`

---

### A-6：游戏社交统计聚合

`GET /api/social/games/:id/stats`（`cmd/server/main.go:125`）

```go
{
  "comment_count": commentSvc.CountByGame(gameID),
  "post_count":    forumSvc.CountByGameTag(gameID),
}
```

前端 `StatsBar` 已可接入，`socialApi.getStats(gameId)` 调用此端点。

---

### A-7：PlayPage 刷新不丢游戏标题

`GET /api/play/games/:slug` 已支持 UUID 查询（`slug = ? OR id::text = ?`），前端 `useGame(session.game_id)` 可直接传 UUID，后端正确返回游戏数据。

---

### A-9：Session 含 game_id

`GET /api/play/sessions/:id` 返回完整 `GameSession` 结构体，含 `game_id` 字段，前端 `useSession(sessionId)` → `useGame(session.game_id)` 链路可用。

---

## 三、待实现

### A-8：AI 帮答（`POST /api/play/sessions/:id/suggest`）✅ 2026-04-10

**实现位置：** `engine_methods.go` `Suggest()` + `routes.go` 路由

**实现方式：**
1. 读取最近 N 楼对话历史（同 PlayTurn context 窗口）
2. 注入 Impersonate 指令（玩家视角，1-2 句，第一人称）
3. 调用 narrator slot LLM，`MaxTokens=200`
4. 返回 `{ suggestion: string }`，不写入 Floor，不触发记忆整合

**前端接入：** ChatInput 的 `✨ AI 帮答` 按钮解除 disabled，调用此端点后将 suggestion 填入 textarea，玩家可编辑后发送。

---

### A-10：`comment_config` 暴露到 `publicGameView`

**前端现状：** `CommentCore` 硬编码 `linear` 模式，等此字段就绪后读取 `game.ui_config?.comment_style`。

**实现思路：**

在 `publicGameView`（`engine_methods.go:1131`）中从 `GameCommentConfig` 表读取配置并附加到返回值：

```go
// 查询 game_comment_config
var cfg comment.GameCommentConfig
db.Where("game_id = ?", t.ID).First(&cfg)
// 附加到返回
"comment_config": map[string]any{
    "default_mode": cfg.DefaultMode,  // "linear" | "nested"
}
```

**注意：** `publicGameView` 当前在 `engine/api/` 包内，需要 import `social/comment` 包，会引入跨层依赖。

**推荐方案：** 在 `main.go` 的 `/social/games/:id/stats` 端点中一并返回 `comment_config`，或等路由迁移（ENGINE-ROUTE-MIGRATION-PLAN.md）完成后在 `platform/play/` 层处理。

**MVP 处理：** 暂时硬编码 `linear`，不阻塞前端。

---

## 四、前端当前可用的完整 API 清单

| 功能 | API | 状态 |
|------|-----|------|
| 游戏列表 | `GET /api/play/games` | ✅ |
| 游戏详情（slug 或 UUID）| `GET /api/play/games/:slug` | ✅ |
| 创建 session | `POST /api/play/sessions` | ✅ |
| 获取 session（含 game_id）| `GET /api/play/sessions/:id` | ✅ |
| 存档列表 | `GET /api/play/sessions?game_id=` | ✅ |
| 楼层历史 | `GET /api/play/sessions/:id/floors` | ✅ |
| 楼层范围（游记剪辑）| `GET /api/play/sessions/:id/floors?from=&to=` | ✅ |
| SSE 流式对话 | `GET /api/play/sessions/:id/stream?input=` | ✅ |
| 重生成 | `POST /api/play/sessions/:id/regen` | ✅ |
| 分叉存档 | `POST /api/play/sessions/:id/floors/:fid/branch` | ✅ |
| Swipe 页列表 | `GET /api/play/sessions/:id/floors/:fid/pages` | ✅ |
| Swipe 选页 | `PATCH /api/play/sessions/:id/floors/:fid/pages/:pid/activate` | ✅ |
| 公开存档 | `PATCH /api/play/sessions/:id { is_public: true }` | ✅ |
| 删除 session | `DELETE /api/play/sessions/:id` | ✅ |
| 点赞/收藏游戏 | `POST /api/social/reactions/game/:id/like|favorite` | ✅ |
| 取消点赞/收藏 | `DELETE /api/social/reactions/game/:id/like|favorite` | ✅ |
| 查自己的 reaction | `GET /api/social/reactions/mine/:target_type/:target_id` | ✅ |
| 评论列表 | `GET /api/social/games/:id/comments` | ✅ |
| 发评论 | `POST /api/social/games/:id/comments` | ✅ |
| 评论回复 | `POST /api/social/comments/:id/replies` | ✅ |
| 评论点赞 | `POST /api/social/comments/:id/vote` | ✅ |
| 论坛帖子列表 | `GET /api/social/posts?game_tag=` | ✅ |
| 发帖 | `POST /api/social/posts` | ✅ |
| 社交统计 | `GET /api/social/games/:id/stats` | ✅ |
| 世界书面板 | `GET /api/play/games/worldbook/:id` | ✅ |
| AI 帮答 | `POST /api/play/sessions/:id/suggest` | ⬜ 待实现 |

---

## 五、延后：A-11 常驻角色

不阻塞前端文本游戏全流程。待常驻角色功能进入开发计划时实现。

**实现要点**（备忘）：
- 新建 `UserProfile` 表（`user_id PK, resident_card_id, resident_session_id, resident_display_name`）
- `POST /api/users/:id/resident_character { character_card_id, import_session_id? }`
  - 若 `import_session_id` 存在：迁移 `importance >= 7` 的记忆到常驻 session
- 触发时机：实现 A-11 时顺带建立 `platform/play/` 包，触发路由迁移（见 ENGINE-ROUTE-MIGRATION-PLAN.md）
