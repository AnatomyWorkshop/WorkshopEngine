# GW 新增 API — 实现计划与进度

> 版本：2026-04-09
> 范围：GW 前端开发所需的后端 API 缺口，按优先级排序实现。

---

## 一、总览

| 编号 | API | 状态 | 前端解锁 |
|------|-----|------|---------|
| A-1 | `GET /api/play/games` + `GET /api/play/games/:slug` | ✅ 已完成 | 游戏详情页静态展示 |
| A-2 | `GET /api/play/sessions?game_id=` | ✅ 已完成 | 继续游玩存档选择 |
| A-3 | reaction `target_type` 加 `game` | ✅ 已完成 | 收藏/点赞游戏 |
| A-4 | `PATCH /api/play/sessions/:id` (is_public) + floors 范围查询 | ✅ 已完成 | 游记分享 |
| A-5 | `GET /api/play/games/:id/worldbook-entries` | ✅ 已完成 | 世界书面板 |
| A-6 | `GET/POST/DELETE /api/users/:id/resident_character` | 🔜 延后 | 常驻角色迁移 |

**前端文本游戏全流程所需 API 已全部就绪（A-1 ~ A-5）。A-6 常驻角色是独立功能，不阻塞前端开发。**

---

## 二、已完成

### A-1：公开游戏列表 + 详情

**文件**：`internal/engine/api/routes.go`（暂放，待迁移至 `platform/play/`，见 ENGINE-ROUTE-MIGRATION-PLAN.md）

**GET /api/play/games**：分页、标签过滤（JSONB `?`）、类型过滤、`new/hot` 排序，返回 `{ games, total, limit, offset }`

**GET /api/play/games/:slug**：slug 或 UUID 双路查询，只返回 `status = 'published'`

**publicGameView**（`engine_methods.go`）：提取 `config.ui_config`，过滤私有字段，返回 `id, slug, title, type, short_desc, notes, cover_url, author_id, play_count, like_count, favorite_count, ui_config, created_at`

**GameTemplate 新增字段**（`models_shared.go`）：`ShortDesc`, `Notes`, `PlayCount`, `FavoriteCount`, `LikeCount`

---

### A-2：用户存档列表

`GET /api/play/sessions?game_id=&user_id=&limit=&offset=`：按 `updated_at DESC` 排序，返回完整 `GameSession`（含 `floor_count`）

---

### A-3：Reaction target_type 加 game

- `TargetGame = "game"` 加入 `validTargetTypes`
- `syncCount`：`game + like` → `game_templates.like_count`，`game + favorite` → `game_templates.favorite_count`
- 路由错误消息更新

---

### A-4：Session 公开分享 + 楼层范围查询

**文件**：`internal/core/db/models_engine.go` + `engine/api/engine_methods.go` + `routes.go`

- `GameSession` 新增 `IsPublic bool`（AutoMigrate 自动建列）
- `UpdateSessionReq` 加 `IsPublic *bool`（指针类型，nil = 不修改）
- `PATCH /api/play/sessions/:id { "is_public": true }` — 已有 handler 扩展支持
- `GET /api/play/sessions/:id/floors?from=1&to=20` — 在已有 floors 路由上加可选范围过滤（内存过滤，楼层数通常 < 500）

---

### A-5：世界书玩家只读 API

**文件**：`internal/engine/api/routes.go`

`GET /api/play/games/:id/worldbook-entries`
- 检查 `GameTemplate.Config.allow_player_worldbook_view == true`，否则 403
- 返回：`[{ id, keys, content, comment }]`，不返回 `position/scan_depth/probability` 等 LLM 技术参数
- 按 `priority ASC` 排序（创作者设定的展示顺序）

---

## 三、延后：A-6 常驻角色

不阻塞前端文本游戏全流程。待常驻角色功能进入开发计划时实现。

**实现要点**（备忘）：
- 新建 `UserProfile` 表（`user_id PK, resident_card_id, resident_session_id, resident_display_name`）
- `POST /api/users/:id/resident_character { character_card_id, import_session_id? }`
  - 若 `import_session_id` 存在：迁移 `importance >= 7` 的记忆到常驻 session
- 触发时机：实现 A-6 时顺带建立 `platform/play/` 包，触发路由迁移（见 ENGINE-ROUTE-MIGRATION-PLAN.md）

---

## 四、前端可以开始的工作

A-1 ~ A-5 全部就绪，前端文本游戏全流程所需 API 完整：

| 功能 | API |
|------|-----|
| 游戏列表/详情页 | `GET /api/play/games` + `GET /api/play/games/:slug` |
| 开始游玩 | `POST /api/play/sessions` |
| 继续游玩（存档选择）| `GET /api/play/sessions?game_id=` |
| 游玩回合 | `POST /api/play/sessions/:id/turn` |
| SSE 流式输出 | `GET /api/play/sessions/:id/stream` |
| 重新生成 | `POST /api/play/sessions/:id/regen` |
| 楼层历史 + 分叉 | `GET /api/play/sessions/:id/floors` + `POST .../floors/:fid/branch` |
| 收藏游戏 | `POST /api/social/reactions/game/:id/favorite` |
| 评论列表/发评论 | `GET/POST /api/social/games/:id/comments` |
| 论坛帖子列表 | `GET /api/social/posts?game_tag=` |
| 世界书面板 | `GET /api/play/games/:id/worldbook-entries` |
| 游记分享（公开存档）| `PATCH /api/play/sessions/:id { is_public: true }` |
| 楼层剪辑 | `GET /api/play/sessions/:id/floors?from=&to=` |
| 社交统计 | `GET /api/social/games/:id/stats` |
