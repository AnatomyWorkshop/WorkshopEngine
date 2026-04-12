# GW 新增 API — 实现计划与进度

> 版本：2026-04-12 v9（Phase 3 前端 + Phase 4 后端全部完成，准备内测对比）
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
| A-10 | `comment_config` 暴露到游戏详情响应 | ✅ |
| A-11 | 常驻角色 `GET/POST/DELETE /api/users/:id/resident_character` | 🔜 延后 |
| A-12 | `publicGameView` 补充 `config.tags`（`author_name` 待 users 表建立）| ✅ 2026-04-11 |
| A-13 | `UIConfig.input_placeholder` seed 数据补充 | ✅ 2026-04-11 |
| A-14 | `GET /api/social/reactions/mine/game/:id` 路径对齐 | ✅ 无需修改 |
| A-15 | 个人游戏库 CRUD（`LibraryEntry`）| ✅ 2026-04-11 |
| A-16 | `SeriesKey` / `RuntimeBinding` 存储 | ✅ 纯前端 localStorage |
| A-17 | `GET /api/play/sessions/:id/variables` | ✅ 已存在（engine/api/routes.go）|
| A-18 | `DELETE /api/play/sessions/:id` + `PATCH` 重命名 | ✅ 已存在（engine/api/routes.go）|
| A-19 | `GET /api/create/templates/:id/export` 游戏包完整导出 | ✅ 已存在（creation/api/import_export.go）|
| A-20 | 玩家私有世界书 CRUD（`PlayerWorldbookOverride`）| ✅ 2026-04-12 |
| A-21 | `slug` 非唯一 + 路由改 UUID | 🔜 Phase 4 技术债 |
| A-22 | 内测表现力：`WorldbookEntry` 补全字段（probability / sticky）| ✅ 2026-04-12 |
| A-23 | 内测表现力：`PATCH /api/play/sessions/:id/memories/:mid` 玩家记忆编辑 | ✅ 已存在 |
| A-24 | 内测表现力：`GET /api/play/sessions/:id/snapshot` 调试面板 | ✅ 已存在 |

---

## 二、前端 Phase 3 可执行性分析

前端 Phase 3 的 7 项任务（P3-1 ~ P3-8），逐项判断后端依赖：

### P3-1：LLM 错误信息显示 ✅ 纯前端

**后端依赖：无。**

SSE 流式错误已经通过 `event: error` 事件下发（`engine/api/routes.go` 的 stream handler）。前端只需在 `useStreamStore` 增加 `streamError` 状态，在消息列表底部渲染错误气泡。**可立即实现，无需后端修改。**

### P3-2：修复世界书显示 ✅ 纯 seed 数据

**后端依赖：无。**

只需在三张游戏的 `.data/games/*/game.json` 里加 `"allow_player_worldbook_view": true`，重新 seed。`GET /api/play/games/worldbook/:id` 已就绪（A-5）。**可立即实现。**

### P3-3：ActionBar 导入逻辑修正 ✅ 后端已就绪

**后端依赖：已全部满足。**

- 创建 session：`POST /api/play/sessions` ✅（A-1）
- 存档列表：`GET /api/play/sessions?game_id=` ✅（A-2）
- 个人库写入：`POST /api/users/:id/library` ✅（A-15）

前端删除 `slugPrefix`/版本 popover 逻辑，改为三态按钮（无 session / 有 session / 新存档）。**可立即实现。**

### P3-4：存档管理（ArchiveDrawer 增强）✅ 后端已就绪

**后端依赖：已全部满足。**

- 删除存档：`DELETE /api/play/sessions/:id` ✅（A-18，engine/api/routes.go line 135）
- 重命名存档：`PATCH /api/play/sessions/:id { title }` ✅（A-18，engine/api/routes.go line 120）
- 新建存档：`POST /api/play/sessions` ✅

**可立即实现，无需后端修改。**

### P3-5：统计抽屉显示 session 变量 ✅ 后端已就绪

**后端依赖：已满足。**

`GET /api/play/sessions/:id/variables` ✅（A-17，engine/api/routes.go line 143）

前端 `StatsDrawer` 调用此接口，展示 `display_vars` 列表中的变量。**可立即实现，无需后端修改。**

### P3-6：游戏包导出（个人库）✅ 后端已就绪

**后端依赖：已满足。**

`GET /api/create/templates/:id/export` ✅（A-19，creation/api/import_export.go line 201）

该接口已完整实现，返回 `GameTemplate + PresetEntries + WorldbookEntries + RegexProfiles + Materials + PresetTools`。

**内测阶段前端只需触发浏览器下载此接口的 JSON 响应即可。**

### P3-7：Markdown 渲染 ✅ 纯前端

**后端依赖：无。** `react-markdown` + `remark-gfm` 纯前端实现。

### P3-8：页面美化 ✅ 纯前端

**后端依赖：无。**

---

**Phase 3 结论：所有 7 项任务的后端依赖均已就绪，Phase 3 可以完全在前端侧推进，无需等待任何后端新接口。**

---

## 三、关键设计问题

### Q1：私有世界书需要保留原始游戏包内容吗？

**结论：是的，必须保留，且原始内容永远不应被覆盖。**

原因：
1. **可恢复性**：玩家修改后可以"恢复默认"，这要求原始词条始终存在
2. **多玩家隔离**：同一游戏的不同玩家有各自的修改，原始词条是共享基准
3. **游戏更新**：创作者更新游戏包时，原始词条会变化，玩家的 override 应该叠加在新版本上

**实现方案（方案 B，推荐）：**

新建 `PlayerWorldbookOverride` 表，存储玩家对特定词条的覆盖，不修改原始 `WorldbookEntry`：

```
PlayerWorldbookOverride {
  id          uuid PK
  game_id     string  NOT NULL  -- 关联 GameTemplate（不是 session，跟随游戏副本）
  user_id     string  NOT NULL  -- 关联玩家
  entry_id    string            -- 原始词条 ID（空 = 玩家新增的词条）
  content     text              -- 覆盖内容（NULL = 使用原始内容）
  enabled     bool              -- 覆盖启用状态（NULL = 使用原始状态）
  is_new      bool  default false -- true = 玩家新增词条（entry_id 为空）
  UNIQUE(game_id, user_id, entry_id)
}
```

**引擎合并逻辑（WorldbookNode）：**
```
原始词条列表 + 玩家 override 列表
→ 按 entry_id 合并：override 存在则覆盖 content/enabled
→ 追加 is_new=true 的玩家新增词条
→ 最终词条列表送入 Pipeline
```

**为什么不用 session 级别而用 game+user 级别：**
- 同一游戏的多个存档（多周目）应该共享同一套世界书修改
- 玩家修改的是"对这个游戏的理解"，不是"某次游玩的临时状态"
- 如果用 session 级别，每次新建存档都要重新配置世界书，体验差

### Q2：Text 游戏是否导入即下载？

**结论：不是传统意义的"下载"，而是"注册到个人库 + 在服务器端创建 session"。**

Text 游戏的所有内容（`GameTemplate`、`WorldbookEntry`、`PresetEntry` 等）存储在服务器 DB，不下载到客户端。

**"导入"的实际含义：**
1. 调用 `POST /api/users/:id/library`，在个人库表中写一条 `LibraryEntry`（记录"我关注了这个游戏"）
2. 前端 localStorage 写入游戏基础信息（`gw_library`，用于离线展示）
3. 游玩时调用 `POST /api/play/sessions`，在服务器端创建 session

**与 SillyTavern 的区别：**
- ST 是本地应用，角色卡真的下载到本地文件系统
- GW 是 Web 应用，"导入"只是建立用户与游戏的关联关系，内容始终在服务器

**例外：PNG 导入（`POST /api/create/cards/import`）**

PNG 导入是真正的"上传"——玩家把本地的 ST 角色卡 PNG 上传到服务器，服务器解析并创建新的 `GameTemplate`。这个新 `GameTemplate` 属于该玩家（`author_id = 玩家 ID`），存储在服务器 DB，不在客户端。

**离线场景：**
- 当前不支持离线游玩（需要服务器 LLM 调用）
- localStorage 的 `gw_library` 只用于展示个人库列表，不用于离线游玩
- 未来如果支持本地 LLM（`engine_mode = local_engine`），可以考虑游戏包本地缓存，但这是长期方向

---

## 四、Phase 4 详细设计

### ST 世界书编辑的对应关系

SillyTavern 的世界书编辑流程：
1. `loadWorldInfo(name)` → `POST /api/worldinfo/get` → 返回整个 world info 文件（`{ entries: { [uid]: entry } }`）
2. 用户在 UI 编辑某个词条字段（实时修改内存中的 `data.entries[uid]`）
3. `saveWorldInfo(name, data)` → 防抖 → `_save()` → `POST /api/worldinfo/edit` → 发送**整个文件**

**ST 的关键设计：整文件保存，不是逐条保存。** 每次保存都把整个 world info JSON 发给服务器覆盖。这在本地文件系统上简单可靠，但在多用户 DB 场景下不适用（会覆盖其他用户的修改）。

**GW 的对应设计：逐条 upsert，不是整文件覆盖。** 每次编辑一条词条就发一个 PATCH 请求，服务器只更新那一条记录。这是 DB 场景的正确做法，与 ST 的整文件保存在用户体验上等价（都是"编辑后自动保存"），但实现更安全。

---

### A-20：玩家私有世界书 CRUD（Phase 4）

**核心问题：导入游戏和玩家上传游戏的世界书编辑体验是否可以统一？**

**结论：可以，且必须统一。**

两种情况的本质相同：
- **导入公共库游戏**：`GameTemplate` 在服务器 DB，`author_id ≠ 当前用户`，玩家不能直接修改原始词条
- **玩家上传 PNG 游戏**：`GameTemplate` 在服务器 DB，`author_id = 当前用户`，玩家理论上可以直接修改原始词条

**统一方案：无论哪种情况，玩家编辑世界书都走 `PlayerWorldbookOverride` 路径，不直接修改 `WorldbookEntry`。**

原因：
1. **一致性**：前端只需要一套编辑 UI 和一套 API，不需要根据 `author_id == me` 走不同路径
2. **可恢复性**：即使是自己上传的游戏，也可以"恢复默认"（恢复到上传时的原始状态）
3. **多存档隔离**：同一游戏的不同存档可以有不同的世界书配置（虽然当前设计是 game+user 级别，但未来可以细化到 session 级别）
4. **创作者视角与玩家视角分离**：创作者通过 `creation/` 接口修改原始词条（影响所有玩家），玩家通过 `platform/library/` 接口修改自己的副本

**数据模型：**

```go
// internal/platform/library/worldbook_override.go
type PlayerWorldbookOverride struct {
    ID      string  `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    GameID  string  `gorm:"not null;index"`
    UserID  string  `gorm:"not null;index"`
    EntryID string  `gorm:"index;default:''"` // 空 = 玩家新增词条
    // 可覆盖字段（NULL = 使用原始值）
    Content        *string        `gorm:"type:text"`
    Enabled        *bool
    Keys           datatypes.JSON `gorm:"type:jsonb"` // NULL = 使用原始 keys
    SecondaryKeys  datatypes.JSON `gorm:"type:jsonb"`
    SecondaryLogic *string
    Constant       *bool
    Priority       *int
    // is_new=true 时 EntryID 为空，这是玩家完全新增的词条
    IsNew          bool `gorm:"default:false"`
    // UNIQUE INDEX (game_id, user_id, entry_id) — 原生 SQL 建立（entry_id 空时允许多条 is_new）
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**API 端点（归属 `platform/library/`）：**

```
GET  /api/users/:uid/library/:game_id/worldbook
  → 返回原始词条 + 玩家 override 合并后的完整列表
  → 每条词条附加 { is_overridden: bool, is_new: bool }
  → 前端用 is_overridden 显示"已修改"标记，用"恢复"按钮触发 DELETE

PATCH /api/users/:uid/library/:game_id/worldbook/:entry_id
  body: { content?, enabled?, keys?, secondary_keys?, secondary_logic?,
          constant?, priority? }
  → upsert PlayerWorldbookOverride（只更新传入的字段，其余保持 NULL）
  → 对应 ST 的 saveWorldInfo（防抖后整文件保存）→ GW 改为逐字段 upsert

POST  /api/users/:uid/library/:game_id/worldbook
  body: { content, keys, enabled?, constant?, priority?, ... }
  → 新增玩家私有词条（is_new=true，entry_id 为空）
  → 对应 ST 的 createWorldInfoEntry

DELETE /api/users/:uid/library/:game_id/worldbook/:entry_id
  → entry_id 对应原始词条：删除 override（恢复原始）
  → entry_id 对应 is_new 词条：物理删除
  → 对应 ST 的 deleteWorldInfoEntry

DELETE /api/users/:uid/library/:game_id/worldbook
  → 重置所有 override（恢复全部默认）
```

**引擎集成（WorldbookNode 修改）：**

`WorldbookNode` 当前直接查 `WorldbookEntry WHERE game_id = ?`。
需要增加合并步骤，通过 `BuildContext.UserID` 查 override：

```go
// 合并逻辑（伪代码）
func mergeWorldbook(entries []WorldbookEntry, overrides []PlayerWorldbookOverride) []WorldbookEntry {
    overrideMap := map[string]PlayerWorldbookOverride{}
    for _, o := range overrides {
        overrideMap[o.EntryID] = o
    }
    result := make([]WorldbookEntry, 0, len(entries))
    for _, e := range entries {
        if o, ok := overrideMap[e.ID]; ok {
            // 用 override 覆盖非 NULL 字段
            if o.Content != nil { e.Content = *o.Content }
            if o.Enabled != nil { e.Enabled = *o.Enabled }
            // ... 其他字段
        }
        result = append(result, e)
    }
    // 追加玩家新增词条（is_new=true）
    for _, o := range overrides {
        if o.IsNew { result = append(result, overrideToEntry(o)) }
    }
    return result
}
```

**前置条件：**
- JWT 认证（`user_id` 可识别）
- `platform/library/` 包已建立（A-15 ✅）

**解耦原则：**
- `PlayerWorldbookOverride` 归属 `platform/library/`，不 import `engine/`
- `WorldbookNode` 通过 `BuildContext.UserID` 查 override 表（`core/db` 层，合法）
- `creation/` 的世界书接口操作原始 `WorldbookEntry`，不受影响

---

### A-21：`slug` 非唯一 + 路由改 UUID（Phase 4 技术债）

**背景：** 当前 `GameTemplate.Slug` 有 `uniqueIndex`，不允许不同作者用同名游戏。

**影响范围：**
- `internal/core/db/models_shared.go`：去掉 `uniqueIndex`，改为普通 `index`
- `platform/play/handler.go`：`getGame()` slug 不唯一后改用 UUID 主路径
- 前端路由：`/games/:slug` → `/games/:id`（UUID）

**推荐方案：** 路由改为 `/games/:id`（UUID），slug 只用于显示。`getGame()` 保留 `slug = ? OR id::text = ?` 兼容查询，内测后逐步迁移。

**内测阶段：** 保持 slug 唯一（只有一个作者），记录为技术债，Phase 4 处理。

---

### A-22：`WorldbookEntry` 补全字段（Phase 4，内测表现力）

**背景：** ST 的 `LorebookEntry` 有若干 GW 当前未实现的字段，影响内测时的表现力。

**ST 有但 GW 缺失的字段：**

| ST 字段 | 含义 | GW 优先级 |
|---------|------|---------|
| `probability` (0-100) | 词条触发概率（100=必触发）| 中 — 随机性叙事需要 |
| `sticky` (int) | 触发后保持激活 N 轮 | 中 — 避免重要词条因扫描深度不足而消失 |
| `cooldown` (int) | 触发后冷却 N 轮再次触发 | 低 — 防止同一词条反复注入 |
| `delay` (int) | 延迟 N 轮后才开始触发 | 低 — 用于剧情推进 |
| `vectorized` (bool) | 向量化检索（替代关键词匹配）| 低 — 依赖 pgvector，长期方向 |
| `exclude_recursion` (bool) | 不参与递归扫描 | 低 — 递归扫描本身是高级功能 |

**内测前必须补充的字段：**
- `probability`：影响叙事随机性，创作者常用
- `sticky`：影响重要词条的持续性，没有这个字段会导致关键设定在长对话中消失

**实现位置：**
- `internal/core/db/models_creation.go`：`WorldbookEntry` 增加字段
- `internal/engine/pipeline/node_worldbook.go`：`WorldbookNode` 实现 probability 随机过滤 + sticky 计数
- `internal/core/db/models_engine.go`：`GameSession` 或 `Floor` 增加 `sticky_entries` 字段（记录当前激活的 sticky 词条及剩余轮数）

---

## 五、内测表现力清单

**目标：内测时达到 SillyTavern 的基础表现力。**

以下是对照 ST 功能的完整清单，标注当前状态和优先级：

### 5.1 核心游玩流程

| 功能 | ST 对应 | GW 状态 | 内测优先级 |
|------|---------|---------|---------|
| SSE 流式输出 | `streamingProcessor` | ✅ 已实现 | — |
| 重新生成（Regen）| `Generate` 重发 | ✅ 已实现 | — |
| Swipe（多页切换）| `swipe_right/left` | ✅ 已实现 | — |
| AI 帮答（Impersonate）| `mes_impersonate` | ✅ 已实现（A-8）| — |
| 存档列表 + 切换 | `chat_metadata` | ✅ 已实现 | — |
| 存档重命名/删除 | `renameChatFile` | ✅ 已实现（A-18）| — |
| 分叉存档（Branch）| `createBranch` | ✅ 已实现 | — |

### 5.2 世界书系统

| 功能 | ST 对应 | GW 状态 | 内测优先级 |
|------|---------|---------|---------|
| 关键词触发 | `world_info.js` scan | ✅ 已实现 | — |
| 常驻词条（Constant）| `constant: true` | ✅ 已实现 | — |
| 次级关键词逻辑 | `selectiveLogic` | ✅ 已实现 | — |
| 扫描深度 | `scan_depth` | ✅ 已实现 | — |
| 注入位置（before/after/at_depth）| `position` | ✅ 已实现 | — |
| 互斥分组 | `group` + `group_weight` | ✅ 已实现 | — |
| 递归扫描 | `recursive` | ✅ 已实现（WorldbookNode 二次扫描）| — |
| 触发概率 | `probability` | ✅ 已实现（A-22，2026-04-12）| — |
| Sticky（持续激活）| `sticky` | ✅ 已实现（A-22，2026-04-12）| — |
| 玩家编辑世界书 | ST 本地编辑 | ✅ 后端已实现（A-20，2026-04-12）| 🔜 前端 WorldbookDrawer 待接入 |
| Cooldown / Delay | `cooldown`/`delay` | ❌ 缺失 | 🟡 中 |
| 向量化检索 | `vectorized` | ❌ 缺失（长期）| 🟢 低 |

### 5.3 记忆系统

| 功能 | ST 对应 | GW 状态 | 内测优先级 |
|------|---------|---------|---------|
| 滚动摘要压缩 | `memory` extension | ✅ 已实现 | — |
| 结构化事实（fact）| — | ✅ 已实现（超越 ST）| — |
| 记忆关系图（edge）| — | ✅ 已实现（超越 ST）| — |
| 阶段标签（stage_tags）| — | ✅ 已实现（超越 ST）| — |
| 玩家查看记忆 | — | ❌ 缺失 | 🟡 中（调试面板）|
| 玩家编辑记忆 | — | ✅ 已存在（A-23，engine/api）| 🟡 中（前端未接入）|
| 重大节点记忆 | — | ❌ 缺失（中期）| 🟢 低 |

### 5.4 变量系统

| 功能 | ST 对应 | GW 状态 | 内测优先级 |
|------|---------|---------|---------|
| 变量读写（set/get）| `setvar`/`getvar` 宏 | ✅ 已实现（工具调用）| — |
| 变量快照查看 | — | ✅ 已实现（A-17）| — |
| 变量展示面板 | — | ✅ 已实现（P3-5，2026-04-12）| — |
| 条件选项（if var > N）| `{{if}}` 宏 | ❌ 缺失（中期）| 🟢 低 |

### 5.5 Prompt 调试

| 功能 | ST 对应 | GW 状态 | 内测优先级 |
|------|---------|---------|---------|
| Prompt 快照查看 | `token_counter` | ✅ 已实现（A-24，`/sessions/:id/floors/:fid/snapshot`）| 🟡 中（前端未接入）|
| 世界书命中记录 | `activated_entries` | ✅ 已实现（`PromptSnapshot.ActivatedWorldbookIDs`）| 🟡 中（前端未接入）|
| Token 用量显示 | `token_counter` | ✅ 已实现（`PromptSnapshot.EstTokens`）| 🟡 中（前端未接入）|

### 5.6 内测前必须完成的工作（优先级排序）

**🔴 必须（阻塞内测体验）：**

1. **A-20 玩家世界书编辑** ✅ 后端完成（2026-04-12）
   - ✅ 后端：`PlayerWorldbookOverride` 表 + `platform/library/` 5 个端点
   - ✅ 引擎：`WorldbookNode` 合并逻辑（`worldbook_helpers.go`）
   - 🔜 前端：`WorldbookDrawer` 从只读变为可编辑（待 ST 对比后实现）

2. **A-22 probability + sticky 字段** ✅ 完成（2026-04-12）
   - ✅ 后端：`WorldbookEntry` 加 `probability`/`sticky`，`GameSession` 加 `sticky_entries`
   - ✅ 引擎：`applyStickyAndProbability()` 实现随机过滤 + sticky 续期/过期
   - 🔜 前端：创作工具编辑 UI 增加这两个字段（CW 范畴，内测后）

3. **P3-5 变量展示面板** ✅ 完成（2026-04-12）
   - ✅ 前端：`StatsDrawer` 接入 `GET /sessions/:id/variables`，5 秒轮询刷新

**🟡 应该（影响内测质量）：**

4. **Prompt 调试面板**（前端工作，待做）
   - 前端：接入 `GET /sessions/:id/floors/:fid/snapshot`，展示 token 用量 + 世界书命中

5. **记忆查看面板**（前端工作，待做）
   - 前端：接入 `GET /sessions/:id/memories`，展示当前记忆列表

6. **P3-1 LLM 错误显示** ✅ 完成（2026-04-12）
   - ✅ 前端：`useStreamStore` 加 `streamError`，`TextSessionPage` 显示红色错误气泡 + 重试/关闭

**🟢 可以延后（内测后再做）：**

7. A-21 slug 非唯一（技术债）
8. Cooldown / Delay 字段
9. 条件选项（`{{if}}` 宏）
10. 重大节点记忆
11. WorldbookDrawer 可编辑 UI（前端，A-20 后端已就绪）

---

## 六、已完成（归档）

### A-1 ~ A-16（见上方总览）

### A-17：`GET /api/play/sessions/:id/variables`（已存在）

`engine/api/routes.go` line 143，返回 `session.Variables` JSONB 快照。前端 `StatsDrawer` 已接入（P3-5，2026-04-12）。

### A-18：`DELETE /api/play/sessions/:id` + `PATCH` 重命名（已存在）

`engine/api/routes.go` line 120（PATCH）和 line 135（DELETE）。前端 `ArchiveDrawer` 已接入（P3-4，2026-04-12）。

### A-19：`GET /api/create/templates/:id/export`（已存在）

`creation/api/import_export.go` line 201，返回完整游戏包 JSON（Template + PresetEntries + WorldbookEntries + RegexProfiles + Materials + PresetTools）。前端 `MyLibraryPage` 已接入"导出"按钮（P3-6，2026-04-12）。

### A-20：玩家私有世界书 CRUD（✅ 2026-04-12）

**实现文件：**
- `internal/platform/library/worldbook_override.go`：`PlayerWorldbookOverride` 模型 + `MigrateWorldbookOverride` + `RegisterWorldbookOverrideRoutes`（5 个端点）
- `internal/engine/api/worldbook_helpers.go`：`applyWorldbookOverrides()`（直接查表，不 import platform/library，保持解耦）
- `cmd/server/main.go`：接入 Migrate + 路由注册

**关键设计决策：**
- 覆盖跟随 game+user 级别（不是 session 级别），多存档共享同一套修改
- 引擎通过 `db.Table("player_worldbook_overrides").Scan()` 直接查表，避免循环 import
- `is_new=true` 词条的 UNIQUE INDEX 不包含 entry_id（允许多条新增词条）
- `mergeWorldbook()` 在 API 层合并用于前端展示，`applyWorldbookOverrides()` 在引擎层合并用于 Prompt 组装

**API 端点：**
```
GET    /api/users/:uid/library/:game_id/worldbook       — 合并后完整列表（含 is_overridden/is_new 标记）
PATCH  /api/users/:uid/library/:game_id/worldbook/:eid  — upsert 覆盖（只更新传入字段）
POST   /api/users/:uid/library/:game_id/worldbook       — 新增玩家私有词条
DELETE /api/users/:uid/library/:game_id/worldbook/:eid  — 恢复原始 / 删除 is_new 词条
DELETE /api/users/:uid/library/:game_id/worldbook       — 重置所有覆盖
```

### A-22：`WorldbookEntry` 补全字段（✅ 2026-04-12）

**实现文件：**
- `internal/core/db/models_creation.go`：`WorldbookEntry` 加 `Probability int`（默认 100）、`Sticky int`（默认 0）
- `internal/core/db/models_engine.go`：`GameSession` 加 `StickyEntries datatypes.JSON`（map[entry_id]remaining_turns）
- `internal/engine/prompt_ir/pipeline.go`：`WorldbookEntry` IR 同步加两字段
- `internal/engine/api/worldbook_helpers.go`：`applyStickyAndProbability()` 实现逻辑

**probability 逻辑：**
- Constant=true 词条跳过概率检查（必触发）
- 其余词条：`prob >= 100` 必触发，否则 `rand.Intn(100) < prob` 决定是否保留
- 默认值 100（兼容旧数据，行为不变）

**sticky 逻辑：**
- 词条触发后写入 `session.sticky_entries[entry_id] = sticky`（剩余轮数）
- 下一轮：`remaining > 0` 强制激活，`remaining - 1`；归零时自然过期
- sticky 状态在每回合结束后持久化到 `game_sessions.sticky_entries`

**接入点：** `game_loop.go`（PlayTurn）、`engine_methods.go`（StreamTurn + Suggest + PromptPreview）四处统一处理。

### A-23：`PATCH /api/play/sessions/:id/memories/:mid`（已存在）

`engine/api/routes.go` line 245，玩家/创作者可编辑记忆条目的 content/importance/type。前端记忆面板接入后可用。

### A-24：`GET /api/play/sessions/:id/floors/:fid/snapshot`（已存在）

`engine/api/routes.go`，返回 `PromptSnapshot`（ActivatedWorldbookIDs + EstTokens + VerifyPassed）。前端调试面板接入后可用。

### Phase 3 前端（✅ 2026-04-12）

| 任务 | 实现 |
|------|------|
| P3-1 LLM 错误显示 | `stores/stream.ts` 加 `streamError`/`setError`/`clearError` + `humanizeError()`；`TextSessionPage` 显示红色错误气泡（含重试/关闭） |
| P3-3 ActionBar 修正 | 删除 `slugPrefix`/版本 popover/`allGamesData`；三态：无存档→"开始游玩"，有存档→"继续"+"新存档" |
| P3-4 ArchiveDrawer 增强 | inline 重命名（Enter 确认/Escape 取消）、confirm 删除（删当前存档跳回详情页）、"＋ 新存档"按钮 |
| P3-5 StatsDrawer 变量面板 | 接入 `GET /sessions/:id/variables`，5 秒轮询，过滤 `_` 前缀内部变量 |
| P3-6 个人库导出 | `MyLibraryPage` 每项加"导出"按钮，触发 `GET /api/create/templates/:id/export` 浏览器下载 |
| P3-7 Markdown 渲染 | `MessageBubble` 已用 `react-markdown` + `remark-gfm`（Phase 2 已完成）|
| P3-2 世界书 seed 修复 | 待 seed 数据补充 `allow_player_worldbook_view: true` |

---

## 七、前端可用 API 清单（完整，含 Phase 3 新增）

| 功能 | API |
|------|-----|
| 游戏列表 | `GET /api/play/games` |
| 游戏详情（slug 或 UUID）| `GET /api/play/games/:slug` |
| 世界书只读 | `GET /api/play/games/worldbook/:id` |
| 创建 session | `POST /api/play/sessions` |
| 获取 session（含 game_id）| `GET /api/play/sessions/:id` |
| 存档列表 | `GET /api/play/sessions?game_id=` |
| 重命名存档 | `PATCH /api/play/sessions/:id { title }` |
| 删除存档 | `DELETE /api/play/sessions/:id` |
| Session 变量快照 | `GET /api/play/sessions/:id/variables` |
| 楼层历史 | `GET /api/play/sessions/:id/floors` |
| 楼层范围（游记剪辑）| `GET /api/play/sessions/:id/floors?from=&to=` |
| SSE 流式对话 | `GET /api/play/sessions/:id/stream?input=` |
| 重生成 | `POST /api/play/sessions/:id/regen` |
| AI 帮答 | `POST /api/play/sessions/:id/suggest` |
| 分叉存档 | `POST /api/play/sessions/:id/floors/:fid/branch` |
| Swipe 页列表 | `GET /api/play/sessions/:id/floors/:fid/pages` |
| Swipe 选页 | `PATCH /api/play/sessions/:id/floors/:fid/pages/:pid/activate` |
| 公开存档 | `PATCH /api/play/sessions/:id { is_public: true }` |
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
| 个人游戏库列表 | `GET /api/users/:id/library` |
| 导入游戏到个人库 | `POST /api/users/:id/library` |
| 从个人库移除 | `DELETE /api/users/:id/library/:entry_id` |
| PNG 导入角色卡/游戏包 | `POST /api/create/cards/import` |
| 游戏包完整导出 | `GET /api/create/templates/:id/export` |
