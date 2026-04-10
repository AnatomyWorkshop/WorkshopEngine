# GW 新增 API — 实现计划与进度

> 版本：2026-04-10 v3（归档已完成工作，保留待实现项）
> 范围：GW 前端开发所需的后端 API 缺口。

---

## 一、总览

| 编号 | API | 状态 |
|------|-----|------|
| A-1 | `GET /api/play/games` + `GET /api/play/games/:slug` | ✅ |
| A-2 | `GET /api/play/sessions?game_id=` | ✅ |
| A-3 | reaction `target_type` 加 `game` | ✅ |
| A-4 | `PATCH /api/play/sessions/:id` (is_public) + floors 范围查询 | ✅ |
| A-5 | `GET /api/play/games/worldbook/:id` | ✅ |
| A-6 | `GET /api/social/games/:id/stats` | ✅ |
| A-7 | `GET /api/play/games/:slug` 支持 UUID 查询 | ✅ |
| A-8 | `POST /api/play/sessions/:id/suggest`（AI 帮答）| ✅ |
| A-9 | `GET /api/play/sessions/:id` 返回 `game_id` | ✅ |
| A-10 | `comment_config` 暴露到游戏详情响应 | ✅ 路由迁移后已实现 |
| A-11 | 常驻角色 `GET/POST/DELETE /api/users/:id/resident_character` | 🔜 延后 |

**A-1 ~ A-10 全部完成，前端文本游戏全流程所需 API 就绪。**

---

## 二、已完成（归档）

### A-1：公开游戏列表 + 详情

- `GET /api/play/games`：分页、标签过滤、类型过滤、`new/hot` 排序
- `GET /api/play/games/:slug`：slug 或 UUID 双路查询（`slug = ? OR id::text = ?`）
- `publicGameView`（`engine_methods.go:1174`）：提取 `ui_config`，返回公开字段

### A-2：用户存档列表

`GET /api/play/sessions?game_id=&user_id=&limit=&offset=`，按 `updated_at DESC`

### A-3：Reaction target_type 加 game

`syncCount`：`game + like` → `like_count`，`game + favorite` → `favorite_count`

### A-4：Session 公开分享 + 楼层范围查询

- `PATCH /api/play/sessions/:id { "is_public": true }`
- `GET /api/play/sessions/:id/floors?from=1&to=20`

### A-5：世界书玩家只读 API

`GET /api/play/games/worldbook/:id`，检查 `allow_player_worldbook_view`，返回 `[{ id, keys, content, comment }]`

### A-6：游戏社交统计聚合

`GET /api/social/games/:id/stats`（`cmd/server/main.go`），返回 `{ comment_count, post_count }`

### A-7：PlayPage 刷新不丢游戏标题

`GET /api/play/games/:slug` 支持 UUID 查询，前端 `useGame(session.game_id)` 链路可用

### A-8：AI 帮答

`POST /api/play/sessions/:id/suggest`（`engine_methods.go` `Suggest()`）
- Impersonate 模式，读取最近对话历史，注入玩家视角指令
- 调用 narrator slot LLM，`MaxTokens=200`，不写入 Floor

### A-9：Session 含 game_id

`GET /api/play/sessions/:id` 返回完整 `GameSession`，含 `game_id` 字段

---

## 三、待实现

### A-10：`comment_config` 暴露到游戏详情响应 ✅ 2026-04-10

**实现位置：** `platform/play/handler.go` `getGame()`

路由迁移完成后，`platform/play/` 可合法 import `social/comment`，在游戏详情响应中附加：

```json
"comment_config": { "default_mode": "linear" }
```

---

### A-11：常驻角色

不阻塞前端文本游戏全流程。待常驻角色功能进入开发计划时实现。

**实现要点（备忘）：**
- 新建 `UserProfile` 表（`user_id PK, resident_card_id, resident_session_id, resident_display_name`）
- `POST /api/users/:id/resident_character { character_card_id, import_session_id? }`
  - 若 `import_session_id` 存在：迁移 `importance >= 7` 的记忆到常驻 session
- 路由归属：`platform/play/` 或独立 `platform/user/` 包
- 触发时机：路由迁移完成后，`platform/play/` 包已建立，顺带实现

---

## 四、前端可用 API 清单（完整）

| 功能 | API |
|------|-----|
| 游戏列表 | `GET /api/play/games` |
| 游戏详情（slug 或 UUID）| `GET /api/play/games/:slug` |
| 创建 session | `POST /api/play/sessions` |
| 获取 session（含 game_id）| `GET /api/play/sessions/:id` |
| 存档列表 | `GET /api/play/sessions?game_id=` |
| 楼层历史 | `GET /api/play/sessions/:id/floors` |
| 楼层范围（游记剪辑）| `GET /api/play/sessions/:id/floors?from=&to=` |
| SSE 流式对话 | `GET /api/play/sessions/:id/stream?input=` |
| 重生成 | `POST /api/play/sessions/:id/regen` |
| AI 帮答 | `POST /api/play/sessions/:id/suggest` |
| 分叉存档 | `POST /api/play/sessions/:id/floors/:fid/branch` |
| Swipe 页列表 | `GET /api/play/sessions/:id/floors/:fid/pages` |
| Swipe 选页 | `PATCH /api/play/sessions/:id/floors/:fid/pages/:pid/activate` |
| 公开存档 | `PATCH /api/play/sessions/:id { is_public: true }` |
| 删除 session | `DELETE /api/play/sessions/:id` |
| 点赞/收藏游戏 | `POST /api/social/reactions/game/:id/like\|favorite` |
| 取消点赞/收藏 | `DELETE /api/social/reactions/game/:id/like\|favorite` |
| 查自己的 reaction | `GET /api/social/reactions/mine/:target_type/:target_id` |
| 评论列表 | `GET /api/social/games/:id/comments` |
| 发评论 | `POST /api/social/games/:id/comments` |
| 评论回复 | `POST /api/social/comments/:id/replies` |
| 评论点赞 | `POST /api/social/comments/:id/vote` |
| 论坛帖子列表 | `GET /api/social/posts?game_tag=` |
| 发帖 | `POST /api/social/posts` |
| 社交统计 | `GET /api/social/games/:id/stats` |
| 世界书面板 | `GET /api/play/games/worldbook/:id` |
