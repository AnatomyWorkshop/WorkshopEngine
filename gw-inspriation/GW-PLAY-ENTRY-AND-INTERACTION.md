# GameWorkshop — 游玩入口与互动子系统设计

> 版本：2026-04-08
> 定位：本文属于 GW **平台产品层**文档，描述游玩入口、游戏详情页交互、云端/本地双轨游玩流。
> 与引擎文档的边界：WE（WorkshopEngine）负责"对话回合如何执行"，本文负责"玩家如何进入、如何分享、如何发现"。

---

## 一、GameWorkshop 产品定位

```
CreateWorkshop (CW)    创作者视角：制作游戏包、上传素材、管理世界书
GameWorkshop   (GW)    玩家视角：发现游戏、游玩、分享存档、评论交流
```

两个产品共用同一套 WE 引擎 API，前端分离，账户系统共用。
本文只涉及 GW 玩家侧。

---

## 二、游戏发现流（Game Discovery Flow）

### 2.1 入口层级

```
GW 首页
├── 推荐流（算法 + 人工）
├── 分类浏览（标签 / 类型：轻/中/重前端）
├── 搜索（全文 + 标签）
└── 好友正在玩 / 最近游玩记录
```

每张游戏卡片展示：封面图、标题、标签、玩家数、简介前 60 字。

### 2.2 游戏类型与前端渲染等级

| 类型标识 | 子类型 | 渲染能力 | 目录呈现方式 |
|---------|-------|---------|------------|
| `text` | — | 纯文本聊天，GW 提供全部 UI | **角色卡片**（头像 + 角色名 + 简介）|
| `light` | — | 文本 + 立绘 + 状态栏 + 选项按钮 + 多角色面板 | 游戏封面卡 |
| `rich` | `rich-a`（VN） | 完整视觉小说：背景图层 / 立绘槽 / 对话框 / BGM | 游戏封面卡 |
| `rich` | `rich-b`（独立应用）| 创作者完全自定义前端，GW 提供 iframe 容器 + API | 游戏封面卡 |

前端根据 `game_type`（以及 `rich` 类型中的 `config.rich_subtype`）字段自动选择渲染器。

**text 游戏的目录区独立**：text 游戏不与 light/rich 游戏混排。  
首页/目录分两个区块：
```
── 「与角色对话」（text 游戏）──
  [角色卡 × N]  头像 + 角色名 + 简介 + 标签 + 在线人数
  点击 → 有存档直接进聊天；无存档 → 轻量 Modal（角色简介 + 开始按钮）→ 聊天界面

── 「叙事游戏」（light/rich）──
  [封面卡 × N]  封面图 + 标题 + 玩家数 + 简介
  点击 → 完整游戏详情页
```

---

## 三、游戏详情页（Game Detail Page）

### 3.1 页面结构

```
┌─── 游戏详情页 ──────────────────────────────────────────────────┐
│                                                                  │
│  [封面区]  标题 / 作者 / 标签 / 简介                               │
│  [操作区]  [▶ 开始游玩]  [继续上次存档]  [❤ 收藏]  [分享]           │
│                                                                  │
│  [Tab 栏]  概述 │ 评论 │ 攻略/游记 │ 创作者说明                     │
│                                                                  │
│  [概述 Tab]                                                      │
│    - 游戏类型 / 预计时长 / 内容标签（SFW/NSFW/暗黑/轻松...）        │
│    - 最近更新记录（游戏包 changelog）                               │
│    - 存储策略提示（"本游戏存档仅保存在本地设备"）                     │
│                                                                  │
│  [评论 Tab]  → 见第四章 CommentCore 子系统                         │
│                                                                  │
│  [攻略/游记 Tab]                                                  │
│    - 玩家发布的公开游记（POST 类型 = "guide" / "journal"）          │
│    - 与 social 层 Post 表关联（session_id 可选挂载）               │
│                                                                  │
│  [创作者说明 Tab]                                                  │
│    - 游戏包版本历史                                                │
│    - 玩法说明 / 注意事项（Markdown 渲染）                           │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 数据来源（无引擎耦合）

| 页面区块 | 数据来源 | API |
|---------|---------|-----|
| 封面/标题/简介 | GameTemplate | `GET /api/play/games/:id` |
| 玩家数/收藏数 | social 层统计 | `GET /api/social/games/:id/stats` |
| 评论 | social 层 CommentCore | `GET /api/social/games/:id/comments` |
| 游记/攻略 | social 层 Post | `GET /api/social/posts?game_id=` |
| 存档列表 | engine 层 Session | `GET /api/play/sessions?game_id=&user_id=` |

**关键原则：** 游戏详情页的评论和统计数据完全来自 social 层，不读取 engine 内部的 Floor / Memory / Variable。

---

## 四、游玩入口流程

### 4.1 新游玩

```
点击"开始游玩"
│
├─ 检查 storage_policy
│   ├─ local_only  → 提示"本游戏存档仅本地保存" → 确认
│   └─ cloud_*     → 直接继续
│
├─ 调用 POST /api/play/sessions { game_id, user_id }
│   └─ WE 自动注入 first_mes，返回 session_id
│
└─ 跳转游玩页（Play.vue），传入 session_id
```

### 4.2 继续游玩

```
点击"继续上次存档"
│
├─ GET /api/play/sessions?game_id=&user_id=&limit=5
│   └─ 展示最近 5 个存档（标题 / 最后游玩时间 / 楼层数）
│
└─ 选择存档 → 跳转游玩页，传入已有 session_id
```

### 4.3 分叉存档（从某个节点重来）

```
游玩页 → 楼层历史面板 → 点击某楼"从此处分叉"
│
├─ POST /api/play/sessions/:id/floors/:fid/branch
│   └─ 返回 branch_id
│
└─ 后续 PlayTurn 请求携带 branch_id，走新时间线
   （旧 main 分支存档完整保留，互不影响）
```

### 4.4 云端 / 本地双轨

| 存储策略 | 游玩时数据流向 | 跨设备体验 |
|---------|-------------|----------|
| `local_only` | 存档只写本地 SQLite，LLM 请求直出设备 | 无法跨设备（设计如此） |
| `cloud_optional`（默认）| 存档写云端 PostgreSQL + 本地缓存 | 任意设备无缝继续 |
| `cloud_required` | 强制云端，本地无持久化 | 多人共享状态场景 |

---

## 五、游玩页互动设计（Play.vue）

### 5.1 核心区域布局（text / light / rich-A）

```
┌─── 游玩页 ──────────────────────────────────────────────────────┐
│                                                                  │
│  [场景区]  背景图层 / 立绘槽 / CG 覆盖层                            │
│            （rich-A 显示；text/light 折叠）                       │
│                                                                  │
│  [立绘区]  角色立绘图（light/rich-A）                              │
│            多角色指示器（群聊类 light 游戏：当前对话角色切换）         │
│                                                                  │
│  [消息流]  对话气泡流（assistant / user / system 三类样式）          │
│            打字机效果，流式 token 逐字显示                           │
│                                                                  │
│  [状态栏]  （light/rich-A）🏞 场景名 │ ❤ 好感度 │ 变量快照展开按钮   │
│                                                                  │
│  [选项区]  AI 返回 choice 时渲染按钮，纯文本输入时渲染输入框           │
│                                                                  │
│  [工具栏]  ≡ 菜单 │ 重新生成 │ 世界书 │ 变量 │ 记忆 │ 历史 │ 分享   │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 5.1b Rich-B 游玩页（完全不同）

```
┌─── 游玩页（rich-B）──────────────────────────────────────────────┐
│                                                                  │
│  [IframeGame]                                                    │
│    <iframe sandbox="allow-scripts"                               │
│            src={game.rich_b_url}                                 │
│            style="width:100%; height:100%; border:none;" />      │
│                                                                  │
│  [GW 外层 UI：仅保留]                                             │
│    TopBar（游戏名 + 评论按钮 + 分享按钮）                           │
│    ResidentCharacterWidget（右下角浮窗，与游戏无关）               │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

Rich-B 的游戏 UI 完全在 iframe 内，GW 通过 postMessage 传递 sessionId + apiToken，  
游戏内的 AI 请求由 iframe 内代码直接调用 WE REST API。

### 5.2 Swipe（多版本回答）

- 触发：点击"重新生成"或滑动消息气泡
- 后端：`POST /sessions/:id/regen`
- 前端：同一楼层展示多个 Page，左右滑动切换，选中后调用 `PATCH .../pages/:pid/activate`
- UX 规范：当前 Page 右上角显示"1/3"页码指示器

### 5.3 分支时间线面板

- 触发：工具栏"历史"图标
- 展示：`GET /sessions/:id/floors` 的所有楼层（含分支）
- 每条楼层：显示用户输入摘要 + AI 回复前 30 字 + 所属分支名
- 操作：点击某楼"从此处分叉" → 创建新分支，继续游玩

### 5.4 世界书面板（WorldbookPanel）

- 触发：工具栏"📖 世界书"图标
- 展示：右侧抽屉，两个 Tab
  - **游戏世界书**：`GET /api/play/games/:id/worldbook-entries`，玩家只读；  
    仅当 `game.config.allow_player_worldbook_view = true` 时显示该 Tab（否则整个图标灰显）
  - **我的批注**：`GET /api/play/sessions/:id/worldbook-entries`，玩家的个人条目，可增删改
- 字段展示：keyword（触发词）+ content（内容），不暴露 position/order 等技术参数

### 5.5 游记分享入口

- 触发：工具栏"分享"图标 或 游玩结束后弹出
- 流程：选取楼层范围 → 预览 MVM 渲染 → 发布为 social 层 Post
- 后端：`POST /api/social/posts { session_id, floor_range, type: "journal" }`
- 公开/私密：游玩页分享默认草稿，用户确认后发布

---

## 六、与引擎的边界（防止耦合蔓延）

| 功能 | GW 平台层做 | WE 引擎层做 | 禁止越界的例子 |
|------|-----------|-----------|-------------|
| 游戏发现/推荐 | ✅ | ✗ | 引擎不维护游戏热度 |
| 评论/点赞 | ✅ social 层 | ✗ | 点赞数不写入 GameSession |
| 存档列表 UI | ✅ | ✗ | |
| 游玩回合执行 | ✗ | ✅ engine 层 | 评论不读 Floor 内容 |
| 世界书触发 | ✗ | ✅ engine 层 | 点赞不触发世界书 |
| 变量修改 | ✗（仅展示）| ✅ engine 层 | 评论区不能 PATCH /variables |
| 分支创建 | GW 提供 UI | ✅ engine 提供 API | |

**一句话总结：** GW 平台层只消费 engine 的 REST API，永远不直接访问 engine 的内部 DB 表。
