# GW 本地数据架构 + 前端对接 API 规范

> 日期：2026-04-07
> 接续：2026-04-07-we-integration-kit-and-android-deployment.md § 四
> 触发问题：ST/TH 的 worldbook token 思路是什么？个人存档/数据如何放到手机里？GW 软件需要 WE 的哪些功能？前端对接细节。

---

## 一、ST / TH 的 Token 管理思路对比（与 WE 3-D.1 的关系）

### 1.1 三个引擎的层级不同

理解核心差异：这三者实际上在**不同层面**解决"token 不够用"的问题。

```
ST 的上下文分配（整体视角）：
┌─────────────────────────────────────────────────────┐
│  totalContextTokens = maxTokens - reservedForReply  │
├─────────────────────────────────────────────────────┤
│  systemPrompt     (pinned, 不可裁剪)                 │
│  worldInfo        (有独立预算 worldInfoBudget)       │
│  chatHistory      (动态裁剪，最旧的消息先剔除)        │
└─────────────────────────────────────────────────────┘

ST 的 worldInfoBudget：
  = totalContextTokens × world_info_budget_ratio（默认 0.5，即 50%）
  世界书词条在这个预算里按优先级竞争；超出的词条不注入
```

**ST 的关键设计**：全局 token 预算按"角色扮演"和"世界书"两大块切分，通过 `world_info_budget_ratio` 让用户控制比例。历史记录的裁剪和世界书的裁剪是分离的两套逻辑。

---

```
TH 的上下文分配（PromptIR 视角）：

IRSection:  systemPrompt   → pinned=true,  prunable=false
IRSection:  worldbookBefore → pinned=true, prunable=false
IRSection:  memorySummary  → pinned=true,  prunable=false
IRSection:  chatHistory    → pinned=false, messages 各自 prunable=true
IRSection:  worldbookAfter → pinned=true,  prunable=false

TokenBudget.prune() 扫描所有 section：
  fixedTokens = 所有 pinned section + prunable=false messages 的 token 总和
  availableForHistory = totalBudget - fixedTokens
  → 只有 chatHistory 里的消息参与裁剪
  → 裁剪策略：priority 升序 + 旧消息优先淘汰
```

**TH 的关键设计**：worldbook 词条一旦被选中就进入 pinned section，**不再参与全局裁剪**。词条选择阶段（keyword 匹配 + GroupCap）就已经决定哪些词条进入 prompt；进入 prompt 的词条被视为"必须保留"，代价是挤压历史消息。

---

```
WE 的上下文分配（当前实现）：

applyGroupCap  → 同组词条互斥裁剪（3-C）
applyTokenBudget → 总 token 上限裁剪（3-D.1，新增）
组装 PromptBlocks（worldbook 作为 system role 注入）
历史消息：按 maxHistoryFloors 条数限制，不动态裁剪
```

**WE 3-D.1 和 TH 的关系**：WE 的 `applyTokenBudget` 对应的是 TH 的**词条选择阶段的隐式约束**——TH 通过词条选择 + GroupCap 确保注入量不超标；WE 通过显式的 token 预算裁剪做同样的事，但更直接可控。

**WE 目前还没有的**：TH 那样的**全局 prompt 级别的 token 预算**（历史消息动态裁剪）。WE 的历史消息靠 `maxHistoryFloors` 按条数控制，不是按 token 控制。这是未来的 Phase 4 工作。

### 1.2 实际效果对比

| 场景 | ST | TH | WE (3-D.1后) |
|------|----|----|--------------|
| 世界书词条太多 | worldInfoBudget 裁剪 | GroupCap + pinned 保护 | GroupCap + TokenBudget 裁剪 ✅ |
| 历史记录太长 | 按 token 裁剪最旧消息 | TokenBudget.prune() 裁剪 chatHistory | 按条数裁剪（`maxHistoryFloors`） |
| 单条词条超大 | 词条本身被 budget 截断 | 词条 pinned 后不裁剪 | 词条 token > budget 时整条跳过 ✅ |
| Context 总超限 | 综合裁剪 | 动态裁剪历史 | 历史超过 maxHistoryFloors 被截断 |

---

## 二、个人数据在 GW 手机端的架构

### 2.1 数据是什么

GW 用户的"个人数据"分四类：

```
┌─── GW 个人数据 ────────────────────────────────────────┐
│                                                        │
│  【游戏库】游戏模板（已安装的游戏）                         │
│    game_templates + worldbook_entries + preset_entries │
│    + materials + regex_profiles                        │
│                                                        │
│  【存档】游戏进度                                         │
│    sessions + floors + pages                           │
│    + session.variables（当前游戏状态）                   │
│    + memories（LLM 整合的记忆条目）                      │
│                                                        │
│  【设置】账户 & API 配置                                  │
│    llm_profiles（API Key，AES-256 加密）                │
│    user 配置（昵称、游玩偏好）                            │
│                                                        │
│  【素材】本地资源（可选）                                  │
│    materials 表的 url 字段（指向 CDN 或本地文件）         │
│                                                        │
└────────────────────────────────────────────────────────┘
```

### 2.2 本地存储方案（SQLite 单文件）

GW 本地部署时，所有数据存在一个 SQLite 文件里：

```
Android 应用私有存储：
/data/data/com.gw.app/files/
└── gw.db          ← WE 引擎的 SQLite 数据库（全部数据）
└── gw.db-shm      ← SQLite WAL 模式辅助文件
└── gw.db-wal      ← SQLite WAL 日志

可选：
└── backups/
    └── gw-2026-04-07.db.gz  ← 用户手动备份
```

`gw.db` 是 GW 的**完整存档**。备份 = 复制这一个文件。用户换设备 = 把这个文件导入新设备的 GW 里。

### 2.3 数据库 Schema（手机端最小子集）

WE 的数据库迁移是代码自动生成的（GORM AutoMigrate）。手机端本地 SQLite 和云端 PostgreSQL 用的是同一套 schema，区别只在：

| 字段 | PostgreSQL 类型 | SQLite 等效 |
|------|----------------|-------------|
| `id` | `uuid DEFAULT gen_random_uuid()` | `TEXT DEFAULT (lower(hex(randomblob(4))) || '-' || ...)` |
| `config` | `jsonb` | `TEXT`（JSON 字符串，SQLite 原生支持 JSON 函数） |
| `created_at` | `timestamptz` | `DATETIME` |

GORM 的 `datatypes.JSON` 在 SQLite 下自动降级为 TEXT，业务代码零感知。

### 2.4 数据的生命周期

```
安装游戏：
  POST /api/v2/create/templates/import  ← 上传 .game-package.json
  → 写入 game_templates + worldbook_entries + ... 表

开始游玩：
  POST /api/v2/play/sessions            ← 创建 session
  → 写入 sessions 表（关联 game_id）

每一回合：
  POST /api/v2/play/sessions/:id/turn   ← 发送输入
  GET  /api/v2/play/sessions/:id/stream ← SSE 接收输出
  → 写入 floors + pages 表
  → 异步：Memory Worker 整合记忆 → 写入 memories 表
  → session.variables 更新（来自 <UpdateState> 解析）

存档快照：
  GET  /api/v2/play/sessions/:id/state        ← 当前状态（变量 + 最新楼层）
  GET  /api/v2/play/sessions/:id/variables    ← 变量树

卸载游戏（删除模板）：
  DELETE /api/v2/create/templates/:id
  → 级联删除 worldbook_entries, preset_entries, materials...
  → 注意：sessions（存档）不自动删除，保留历史

导出存档（4-E 实现后）：
  GET /api/v2/play/sessions/:id/export  ← 导出 .thchat 格式
```

---

## 三、GW 软件需要 WE 的哪些功能（前端对接规范）

以下是 GW 前端（手机 App）需要对接的完整 API 列表，按功能模块分组。

### 3.1 启动 & 健康检查

```
GET /health
→ { status: "ok", version: "...", db: "sqlite|postgres" }

前端逻辑：App 启动时轮询此接口，确认本地 WE 进程已就绪
超时处理：5s 无响应 → 显示"引擎启动中..."，继续重试
```

### 3.2 游戏库（已安装游戏）

```
# 列出所有已发布/草稿游戏（用于首页游戏列表）
GET /api/v2/play/games
→ [{ id, slug, title, type, description, cover_url, status }]
参数：?status=published（只看已发布）

# 游戏详情（含完整 Config，用于创作者查看）
GET /api/v2/create/templates/:id
→ { ...template, config: {...} }

# 导入游戏包（用户从文件/URL 安装游戏）
POST /api/v2/create/templates/import
Body: game-package.json 内容
→ { id, slug, title, ... }

# 导出游戏包（用户备份/分享游戏）
GET /api/v2/create/templates/:id/export
→ 下载 .game-package.json 文件

# 删除游戏（卸载）
DELETE /api/v2/create/templates/:id
→ 204 No Content
```

### 3.3 存档管理（Session）

```
# 列出存档（首页"继续游戏"列表）
GET /api/v2/play/sessions
参数：?game_id=&user_id=&limit=20&offset=0
→ [{ id, title, game_id, status, updated_at, ... }]

# 创建新存档（开始游戏）
POST /api/v2/play/sessions
Body: { "game_id": "<uuid>", "title": "我的冒险" }
→ { id, title, game_id, status: "active", ... }

# 获取存档状态（恢复游戏界面时使用）
GET /api/v2/play/sessions/:id/state
→ {
    session: { id, title, status, ... },
    variables: { ... },       ← 当前游戏变量树
    latest_floor: { ... }     ← 最新楼层内容
  }

# 更新存档标题/状态
PATCH /api/v2/play/sessions/:id
Body: { "title": "新标题", "status": "archived" }

# 删除存档
DELETE /api/v2/play/sessions/:id

# 分叉（另存为新时间线）
POST /api/v2/play/sessions/:id/fork
Body: { "title": "分支时间线" }
→ { id: "<new_session_id>", ... }
```

### 3.4 游玩核心（回合 + 流式输出）

这是 GW 最核心的接口，需要特别处理 SSE。

```
# 发起一回合（发送用户输入）
POST /api/v2/play/sessions/:id/turn
Body: { "content": "我决定去神殿", "role": "user" }
→ 202 Accepted { floor_id, page_id }
  （生成是异步的，结果通过 SSE 流获取）

# 重新生成最后一条回复（Swipe）
POST /api/v2/play/sessions/:id/regen
Body: {} （无需 content）
→ 202 Accepted { floor_id, page_id }

# SSE 流式输出（游玩界面的核心）
GET /api/v2/play/sessions/:id/stream
→ text/event-stream

SSE 事件格式（4-H 实现后）：
  data: {"event":"phase","data":{"phase":"preparing"}}
  data: {"event":"phase","data":{"phase":"director_running"}}
  data: {"event":"phase","data":{"phase":"generating"}}
  data: {"event":"token","data":{"text":"莉"}}
  data: {"event":"token","data":{"text":"莉亚"}}
  data: {"event":"phase","data":{"phase":"verifying"}}
  data: {"event":"done","data":{"floor_id":"...","usage":{"prompt_tokens":1234,"completion_tokens":567}}}
  data: {"event":"error","data":{"code":"context_exceeded","message":"..."}}

当前实现（4-H 前）：
  data: {"text":"莉"}              ← 只有 token 事件
  data: {"done":true}
```

**前端 SSE 处理要点：**

```javascript
// 标准 SSE 接入
const es = new EventSource(`/api/v2/play/sessions/${id}/stream`, {
  headers: { 'X-Api-Key': apiKey }
})

// 文本流累积
let text = ''
es.onmessage = (e) => {
  const msg = JSON.parse(e.data)
  if (msg.event === 'token') text += msg.data.text
  if (msg.event === 'phase') updatePhaseUI(msg.data.phase)
  if (msg.event === 'done') {
    finalizeDisplay(text)
    es.close()
    // 完成后 parseVNDirectives(text) 解析 VN 指令
  }
  if (msg.event === 'error') showError(msg.data)
}

// 断线重连：SSE 原生支持 retry，但建议手动实现
es.onerror = () => {
  es.close()
  setTimeout(() => reconnect(), 2000)
}
```

### 3.5 楼层 & Swipe（历史管理）

```
# 楼层列表（聊天历史滚动加载）
GET /api/v2/play/sessions/:id/floors
参数：?limit=20&offset=0
→ [{ id, seq, user_message, active_page: { content }, status }]

# 某楼层的所有候选页（Swipe 选择）
GET /api/v2/play/sessions/:id/floors/:fid/pages
→ [{ id, content, is_active }]

# 激活某页（Swipe 选择第 N 个候选）
PATCH /api/v2/play/sessions/:id/floors/:fid/pages/:pid/activate
→ 200 OK { id, is_active: true }

# Prompt 快照（调试用：查看某楼层的完整 prompt）
GET /api/v2/play/sessions/:id/floors/:fid/snapshot
→ { worldbook_hits: [...], preset_hits: [...], est_tokens, verify_passed }
```

### 3.6 变量管理（游戏状态）

```
# 读取当前变量树
GET /api/v2/play/sessions/:id/variables
→ {
    "世界状态": {
      "当前地点": "新法尼亚王国 - 神殿",
      "当前时间段": "午后",
      "当前互动角色": ["莉莉亚", "露娜玛丽亚"]
    },
    "玩家状态": { ... },
    "角色": { ... }
  }

# 手动修改变量（创作者调试模式 / 存档编辑）
PATCH /api/v2/play/sessions/:id/variables
Body: { "世界状态.当前地点": "王都 - 王宫", "角色.莉莉亚.好感度": 15 }
→ 200 OK { merged: true }
```

### 3.7 记忆（Memory）

```
# 查看记忆列表（调试 / 知识卡片展示）
GET /api/v2/play/sessions/:id/memories
参数：?limit=20&offset=0
→ [{ id, fact_key, content, importance, stage_tags, created_at }]

# 手动立即触发记忆整合（通常自动在后台运行）
POST /api/v2/play/sessions/:id/memories/consolidate
→ 200 OK { consolidated: true, new_facts: [...] }
```

### 3.8 LLM 配置（API Key 管理）

```
# 列出已配置的 LLM（首次启动时为空）
GET /api/v2/create/llm-profiles
→ [{ id, name, base_url, model, api_key_mask, slot }]

# 新建 LLM 配置
POST /api/v2/create/llm-profiles
Body: {
  "name": "我的 DeepSeek",
  "base_url": "https://api.deepseek.com/v1",
  "model": "deepseek-chat",
  "api_key": "sk-xxxxxxxx"   ← 写入时加密，读取返回 api_key_mask
}

# 绑定到 slot（narrator/director/memory/verifier）
POST /api/v2/create/llm-profiles/:id/activate
Body: { "slot": "narrator", "game_id": null }  ← null = 全局绑定

# 连通性测试（填写配置后验证是否可用）
POST /api/v2/create/llm-profiles/models/test
Body: { "base_url": "...", "api_key": "...", "model": "..." }
→ { latency_ms: 342, response_text: "..." }

# 发现可用模型列表（填 base_url + key 后自动获取）
POST /api/v2/create/llm-profiles/models/discover
Body: { "base_url": "...", "api_key": "..." }
→ [{ id: "deepseek-chat", label: "DeepSeek Chat" }]
```

### 3.9 素材（立绘/背景图/BGM）

```
# 列出素材（VN 渲染时查找立绘文件名对应的 URL）
GET /api/v2/create/materials
参数：?game_id=&function_tag=sprite&limit=100
→ [{ id, type, content, tags, function_tag, url }]

# 搜索素材（用于 search_material 工具）
GET /api/v2/create/materials?q=莉莉亚&function_tag=sprite
→ [{ content: "莉莉亚_happy", url: "https://cdn.../莉莉亚_happy.png" }]
```

### 3.10 角色卡（Character Card）

```
# 导入 ST 格式角色卡 PNG（拖放导入）
POST /api/v2/create/cards/import
Content-Type: multipart/form-data
File: 角色卡.png
→ { id, name, description, ... }

# 列出已导入的角色卡
GET /api/v2/create/cards
→ [{ id, name, cover_url, ... }]
```

---

## 四、数据移植：换设备/云同步方案

### 4.1 最小方案（文件级备份）

```
手机 A → 导出 → gw.db.gz
         ↓
手机 B → 导入 → gw.db.gz → 覆盖本地 SQLite
```

WE 引擎需要提供：
```
# 导出完整数据库备份（Phase 4 工作）
GET /api/v2/admin/backup
→ 下载 gw.db.gz（GZIP 压缩的 SQLite 文件）

# 导入备份（恢复）
POST /api/v2/admin/restore
Body: gw.db.gz 文件
→ 202 Accepted（需要重启 WE 进程）
```

### 4.2 游戏包级迁移（已实现）

只迁移游戏（不迁移存档）：

```
旧设备：GET /api/v2/create/templates/:id/export → game-package.json
新设备：POST /api/v2/create/templates/import ← game-package.json
```

### 4.3 存档级迁移（4-E 实现后）

```
旧设备：GET /api/v2/play/sessions/:id/export → session.thchat
新设备：POST /api/v2/play/sessions/import ← session.thchat
        （需要先安装对应的游戏模板）
```

### 4.4 云同步（GW 平台服务，Phase 5）

```
本地 GW App
  ├── 自动上传：每次 CommitTurn 后，增量同步新增的 Floor/Page 到云端
  ├── 拉取：换设备登录后，从云端拉取所有 Session + Floor 到本地 SQLite
  └── 冲突处理：以最新 floor.seq 为准（同一时间线不会产生冲突）

云端 GW 平台 API：
  POST /cloud/sync/push  ← 上传增量 floors
  GET  /cloud/sync/pull  ← 拉取新 floors（?since_floor_seq=N）
```

---

## 五、前端初始化流程（GW App 启动）

```
App 启动
  ↓
1. 启动本地 WE Go 进程（Android Service 或 Capacitor Plugin）
  ↓
2. 轮询 GET /health，直到 status=ok（最多 10s）
  ↓
3. GET /api/v2/create/llm-profiles
   → 空列表？→ 引导用户配置 API Key（首次启动向导）
   → 有配置？→ 继续
  ↓
4. GET /api/v2/play/games
   → 空列表？→ 引导用户安装第一个游戏（游戏商店页）
   → 有游戏？→ 进入首页
  ↓
5. 首页：已安装游戏列表 + 近期存档
   GET /api/v2/play/games
   GET /api/v2/play/sessions?limit=5（最近游玩）
```

---

## 六、Phase 排期（前端对接优先级）

| 功能 | 接口 | 当前状态 | 前端需要等待 |
|------|------|---------|------------|
| 游戏库 + 存档 CRUD | §3.2–3.3 | ✅ 已实现 | 可以开始对接 |
| 回合 + SSE | §3.4 | ✅ 已实现（无 phase 事件） | 可以开始对接，4-H 后加 phase 渲染 |
| 楼层/Swipe | §3.5 | ✅ 已实现 | 可以开始对接 |
| 变量 | §3.6 | ✅ 已实现 | 可以开始对接 |
| 记忆列表 | §3.7 | ✅ 已实现 | 可以开始对接 |
| LLM 配置 | §3.8 | ✅ 已实现（API Key 未加密）| 可以开始对接，4-A 后加密 |
| 素材 | §3.9 | ✅ 已实现 | 可以开始对接 |
| 角色卡导入 | §3.10 | ✅ 已实现 | 可以开始对接 |
| Phase SSE 事件 | §3.4 | 待实现（4-H） | 先用 token-only 流 |
| 存档导入/导出 | §4.2–4.3 | 游戏包已实现，session 待实现（4-E）| 先用文件备份 |
| 数据库备份 | §4.1 | 待实现 | 先手动复制 SQLite 文件 |
| SQLite 模式 | —— | 待实现（需加驱动 + `--db` 参数）| 当前只支持 PostgreSQL |
