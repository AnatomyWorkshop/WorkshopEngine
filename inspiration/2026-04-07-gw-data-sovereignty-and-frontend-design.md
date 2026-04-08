# GW 数据主权架构：本地 × 云端双轨设计

> 日期：2026-04-07
> 接续：2026-04-07-gw-local-data-and-api-spec.md
> 触发问题：NSFW/私密游戏数据保存在本地，SFW/分享数据保存在云端，本地和云端共用引擎，可以实现吗？

---

## 一、核心命题

用户的需求可以拆分为三个独立问题：

| 问题 | 答案 |
|------|------|
| 同一引擎能否同时服务本地和云端数据？ | **可以。** WE 无状态，数据层完全抽象在 GORM 之后。 |
| 用户能否按游戏选择"这个游戏只存本地"？ | **可以。** 每个游戏包附带一个 `storage_policy` 字段。 |
| 本地数据能完全不经过服务器吗？ | **可以。** 纯本地模式下，LLM API 请求直出用户设备，服务器从不看到内容。 |

---

## 二、数据分类 × 存储策略

### 2.1 四类数据的归属建议

```
┌─── GW 数据分类 ─────────────────────────────────────────────────────┐
│                                                                     │
│  【A 类：游戏模板（只读配置）】                                         │
│    SFW 公开游戏   → 云端主存，本地缓存（CDN 友好）                      │
│    NSFW/私密游戏  → 本地唯一，不上传                                    │
│    本地制作游戏   → 本地主存，可选上传至 CW 创作平台                     │
│                                                                     │
│  【B 类：存档（Sessions + Floors）】                                   │
│    SFW 游戏存档   → 可选云同步（跨设备继续游玩）                        │
│    NSFW/私密存档  → 本地唯一，用户显式选择才备份                        │
│                                                                     │
│  【C 类：记忆与变量（动态状态）】                                        │
│    随存档走，与 B 类同策略                                              │
│                                                                     │
│  【D 类：账户 & API 配置】                                              │
│    LLM Profile（含 API Key）→ 本地 AES-256-GCM 加密存储，不上云        │
│    用户偏好设置 → 可选云同步                                            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 游戏包 storage_policy 字段

在 `GameTemplate.Config` 中新增（无需 DB 迁移，写入 JSONB）：

```json
{
  "storage_policy": "local_only"
}
```

| 值 | 含义 |
|----|------|
| `"local_only"` | 存档和记忆只写本地 SQLite，不允许云同步 |
| `"cloud_optional"` | 默认云同步，用户可关闭 |
| `"cloud_required"` | 创作者要求云同步（如多人共享剧情状态） |
| 不设置（默认） | 等价于 `"cloud_optional"` |

这个字段由游戏包创作者设置，用户无法覆盖（保护创作者意图）。

---

## 三、双轨架构图

```
用户设备（GW App）
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  ┌─── WE Go 进程（嵌入）───────────────────────────────────┐    │
│  │                                                          │    │
│  │  PlayTurn → Pipeline → LLM API ──────────────────────────┼──→ LLM Provider (DeepSeek/OpenAI/...)
│  │                                                          │    │  （唯一离开设备的请求）
│  │  SQLite gw.db                                           │    │
│  │  ├── local_only 游戏的所有数据                           │    │
│  │  └── cloud_optional 游戏的本地副本（异步同步）            │    │
│  │                                                          │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌─── 同步代理（可选，background）──────────────────────────┐    │
│  │  检测 storage_policy != "local_only"                    │    │
│  │  → POST /cloud/sync/push（增量 floors）                 │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                 │
└────────────────────────────┬────────────────────────────────────┘
                             │  HTTPS（仅 cloud_optional/required 游戏）
                             ↓
                    GW 云端平台（PostgreSQL）
                    ├── 公开游戏模板库
                    ├── 用户云端存档
                    └── 跨设备同步服务
```

**核心保证：** `local_only` 游戏的任何字节永远不离开设备，即使同步代理在运行。

---

## 四、前端设计思路

### 4.1 状态模型（Pinia）

```typescript
// stores/session.ts —— 单一状态树，来自 TH Pinia 架构
interface SessionStore {
  // 当前游戏会话
  sessionId: string | null
  gameId: string | null
  storagePolicy: 'local_only' | 'cloud_optional' | 'cloud_required'

  // 消息时间线（核心渲染数据）
  floors: Floor[]          // 楼层列表（从 GET /floors 加载）
  streamBuffer: string     // SSE 流式累积缓冲

  // 游戏状态
  variables: Record<string, any>
  memories: Memory[]

  // UI 状态
  isGenerating: boolean
  phase: 'idle' | 'preparing' | 'generating' | 'verifying' | 'done'
  error: string | null
}
```

### 4.2 消息时间线构建（对齐 TH buildMessageTimeline）

TH 的 `@tavern/client-helpers` 的核心函数是 `buildMessageTimeline()`，将 floors + pages 平铺为线性时间线，供 UI 渲染。WE 前端应实现等价函数：

```typescript
// @gw/play-helpers/timeline.ts
export function buildMessageTimeline(floors: Floor[]): TimelineItem[] {
  return floors.flatMap(floor => {
    const activePage = floor.pages?.find(p => p.is_active) ?? floor.active_page
    return [
      { type: 'user',      content: activePage?.user_message ?? '',  floorId: floor.id, seq: floor.seq },
      { type: 'assistant', content: activePage?.content ?? '',        floorId: floor.id, seq: floor.seq,
        pages: floor.pages,  // Swipe 候选页
        isStreaming: floor.status === 'generating' },
    ]
  })
}
```

### 4.3 SSE 流式处理（对齐 TH FloorRunPublicPhase）

```typescript
// @gw/play-helpers/stream.ts
export type StreamEvent =
  | { event: 'phase';  data: { phase: 'preparing' | 'generating' | 'verifying' | 'done' } }
  | { event: 'token';  data: { text: string } }
  | { event: 'done';   data: { floor_id: string; usage: TokenUsage } }
  | { event: 'error';  data: { code: string; message: string } }

// 当前 WE 实现（4-H 前）只有 token + done 事件
// 前端应使用同一个 reducer，4-H 后透明升级支持 phase

export function reduceGameStream(
  state: { text: string; phase: string },
  event: StreamEvent
): { text: string; phase: string } {
  switch (event.event) {
    case 'token': return { ...state, text: state.text + event.data.text }
    case 'phase': return { ...state, phase: event.data.phase }
    case 'done':  return { ...state, phase: 'done' }
    default:      return state
  }
}
```

### 4.4 VN 指令解析（视觉小说层）

```typescript
// @gw/play-helpers/vn.ts
// WE 解析器已在后端处理，前端直接消费 TurnResponse.vn 字段
export interface VNDirectives {
  background?: string    // 背景图素材名
  sprites?: SpriteCmd[]  // 立绘指令列表
  bgm?: string           // 背景音乐
  sfx?: string[]         // 音效列表
  shake?: boolean        // 震动特效
}

// 前端渲染逻辑
function applyVNDirectives(vn: VNDirectives, stage: VNStage): void {
  if (vn.background) stage.setBackground(resolveMaterialUrl(vn.background))
  vn.sprites?.forEach(cmd => stage.updateSprite(cmd.name, cmd.expression, cmd.position))
  if (vn.bgm) stage.playBGM(resolveMaterialUrl(vn.bgm))
}
```

### 4.5 存储策略感知的 UI

```typescript
// 前端根据 storage_policy 显示不同 UI 提示
function getPrivacyBadge(policy: string): string {
  switch (policy) {
    case 'local_only': return '🔒 仅本地'        // 绿色，强调隐私
    case 'cloud_required': return '☁️ 云端同步'   // 蓝色，跨设备
    default: return ''                            // cloud_optional 不展示
  }
}

// 在游戏库列表和游戏详情页显示隐私徽章
// 在设置页提供"关闭 cloud_optional 同步"的全局开关
```

---

## 五、同步架构（cloud_optional 游戏）

### 5.1 增量同步协议

```
本地提交一个 Floor 后：
  if (session.storagePolicy !== 'local_only' && isOnline) {
    POST /cloud/sync/push {
      session_id,
      floors: [{ id, seq, messages, page_vars, status }]
    }
  }

换设备登录：
  GET /cloud/sync/pull?user_id=&since_floor_seq=0
  → 拉取所有云端 floors → 写入本地 SQLite
  → 本地 WE 重建 session.variables（从最新 floor 的 page_vars）
```

### 5.2 冲突处理规则

同一 session 在两台设备同时推进是边缘案例，简单规则：

```
冲突判定：两端 max(floor.seq) 不同 且 本地有未同步 floor
解决策略：
  1. 以 floor.seq 为准（数值大 = 时间线更新）
  2. 冲突的 floor 在本地创建为"分支时间线"（POST /sessions/:id/fork）
  3. 用户手动选择保留哪条时间线
```

### 5.3 local_only 的硬隔离

```go
// 在 sync worker 中检查 storage_policy，直接跳过 local_only
func shouldSync(template GameTemplate) bool {
  var cfg struct{ StoragePolicy string `json:"storage_policy"` }
  json.Unmarshal(template.Config, &cfg)
  return cfg.StoragePolicy != "local_only"
}
```

---

## 六、TH 前端设计参考

TH（TavernHeadless）前端是 Vue3 + Pinia + TailwindCSS 的 SPA，以下是对 GW 有参考价值的设计决策：

### 6.1 消息渲染层级（TH 的 CardMessageRenderer）

TH 将消息渲染分为三层：

```
原始文本（raw）
  ↓ MarkdownRenderer（marked.js + DOMPurify）
  ↓ VNDirectiveRenderer（[[背景:森林]] 等标记）
  ↓ 最终 HTML
```

**WE 对应方案：**
- `type=text` 游戏：只做 markdown 渲染（marked.js）
- `type=light` 游戏：markdown + VN 指令渲染
- `type=rich` 游戏：前端完全自定义（游戏包携带自己的渲染器 JS）

### 6.2 Swipe 交互（TH 的 CharCardMessageSwipe）

TH 的 Swipe 是在 Floor 内切换 MessagePage 的核心 UX：

```
← → 按钮（或手势）
  → PATCH /floors/:fid/pages/:pid/activate
  → Pinia 更新 activePageId
  → 重新渲染该楼层的消息内容
  → 重新应用 VN 指令（背景/立绘可能随 swipe 变化）
```

**注意：** Swipe 切换后，后续 context window 使用新的激活页内容，这会影响世界书扫描结果。

### 6.3 PromptPreview 调试面板（TH 的 Prompt Inspector）

TH 提供 Debug 模式下的 Prompt 检查器，GW 也应实现：

```
GET /api/v2/play/sessions/:id/floors/:fid/snapshot
→ ActivatedWorldbookIDs + PresetHits + EstTokens + VerifyPassed

前端展示：
  - 命中世界书词条列表（高亮关键词）
  - Preset Entry 注入顺序
  - Token 估算饼图（system/worldbook/memory/history/user 各占比）
  - Verifier 校验结果（通过/拒绝及原因）
```

### 6.4 变量面板（TH 的 Variable Inspector）

```
GET /api/v2/play/sessions/:id/variables
→ 嵌套变量树 JSON

前端展示：
  - 可折叠的树形视图（按 group 分组）
  - 直接编辑值（PATCH /sessions/:id/variables）
  - 变化追踪（对比上一回合的 diff）
  - game_stage 快速切换（多幕游戏调试用）
```

### 6.5 记忆面板（TH 无，WE 原创）

```
GET /api/v2/play/sessions/:id/memories
→ 分阶段展示记忆列表

前端展示：
  - 按 stage_tags 分组展示（第一幕 / 第二幕 / 无限制）
  - 当前阶段的记忆高亮显示
  - 重要度（importance）进度条
  - 手动添加/编辑/废弃记忆
  - 触发立即整合按钮（POST .../memories/consolidate）
```

---

## 七、前端路由结构建议

```
/                         → 首页（游戏库 + 近期存档）
/games                    → 游戏商店（cloud_optional/required）
/games/:id                → 游戏详情页
/play/:sessionId          → 游玩界面（核心）
  /play/:sessionId/debug  → 调试面板（变量 + 记忆 + Prompt Inspector）
/settings                 → 设置（LLM Profile + 同步策略）
/settings/llm             → LLM 配置向导
/library                  → 本地游戏库（local_only 游戏）
/import                   → 游戏包导入（.game-package.json）
```

---

## 八、本地 × 云端共用引擎的技术可行性

### 8.1 WE 引擎本身是无状态服务

WE 的所有状态都在数据库里，引擎进程本身无内存状态（除 LRU 缓存等辅助结构）。因此：

```
本地模式：WE 连接 SQLite gw.db
云端模式：WE 连接 PostgreSQL（相同的 GORM schema，无代码改动）
混合模式：本地 WE 连接本地 SQLite + 异步同步到云端 PostgreSQL
```

### 8.2 SQLite 驱动支持（待实现，下一步工作）

```go
// cmd/server/main.go 中添加 --db 参数
flag.StringVar(&dbDSN, "db", os.Getenv("DATABASE_URL"), 
  "Database DSN: postgres://... or sqlite:./gw.db")

// 驱动选择
switch {
case strings.HasPrefix(dbDSN, "sqlite:"):
  db, err = gorm.Open(sqlite.Open(strings.TrimPrefix(dbDSN, "sqlite:")), cfg)
default:
  db, err = gorm.Open(postgres.Open(dbDSN), cfg)
}
```

加入 `gorm.io/driver/sqlite` 后，整个 WE 可以零改动运行在本地 SQLite 模式下。

### 8.3 storage_policy 的引擎层感知

引擎不需要感知 `storage_policy`——这是前端/同步层的概念。引擎只管"往当前连接的数据库里读写"。数据的物理去向（本地 or 云端）由**上层决策**：

```
前端 ← 决定哪些操作请求本地 WE（http://localhost:3721）
       决定哪些数据同步到云端 WE（https://api.gw.app）
       决定何时触发 sync push
```

这个分层让引擎保持纯净，不污染任何存储偏好逻辑。

---

## 九、用户体验设计原则

1. **隐私默认**：游戏包不设 `storage_policy` 时，默认为 `cloud_optional`，但**首次游玩时明确询问**用户是否开启同步，而不是静默上传
2. **透明展示**：游戏库列表中清晰标注哪些游戏是"仅本地"，避免用户误以为云端有备份
3. **离线优先**：在本地 WE 可用的情况下，所有读写操作首先走本地，云端同步为后台任务，失败不影响游玩
4. **导出自由**：用户随时可以 `GET /admin/backup` 导出完整 `gw.db.gz`，不被平台锁定
5. **local_only 游戏的保护提示**：安装 `local_only` 游戏时提示"该游戏仅存储在本地，换设备时需手动导出存档"
