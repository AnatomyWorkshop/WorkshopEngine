# GameWorkshop — 游戏评论区（comment/ 包）设计思路

> 版本：2026-04-08
> 定位：本文聚焦 `internal/social/comment/` 包——即游戏详情页的评论区子系统（ResourceBindCommentCore）。
> 范围：评论形态设计、五个子模块后端映射、工程疑问与意见。
> 不涉及：社区论坛帖子（见 GW-SOCIAL-PACKAGE-DESIGN.md）、游戏包格式、游玩交互。
>
> **说明：** 游戏评论区（comment/）与社区论坛（forum/）是两个独立包，本文只描述前者。
> 两者的区别与包结构总览见 `GW-SOCIAL-PACKAGE-DESIGN.md`。

---

## 一、核心定位与解耦铁规

互动子系统（ResourceBindCommentCore）的唯一绑定键是 **`game_id`**（GameTemplate 的主键）。

```
GameTemplate (game_id)
    │
    └── 评论 / 点赞 / 圆桌 / 游记
        （social 层，独立库表，不读 engine 内部任何表）
```

**解耦铁规（不可妥协）：**
- `internal/social/` 包不得 import `internal/engine/` 任何子包
- 评论表不存储 Floor 内容、Memory 内容、Variable 值
- 唯一允许的弱引用：`session_id`（可选，仅用于游记挂载，不做 JOIN 查询）
- 点赞数、评论数不回写 GameSession 或 GameTemplate

这与 B 站的设计一致：视频评论系统完全独立于视频转码/播放系统，只通过 BVID 关联。

---

## 二、模块放置决策

### 2.1 MVP 阶段：monolith 内的独立包

当前阶段不拆微服务。`internal/social/` 作为主服务内的独立包，与 engine/creation 平级：

```
internal/
├── core/           ← DB、LLM 客户端（无业务依赖）
├── engine/         ← WE 游戏运行时
├── creation/       ← CW 创作工具
└── social/         ← GW 互动层（本文范围）
    ├── comment/    ← 评论核心（BasicComment + LinearFeed）
    ├── reaction/   ← 点赞/收藏/投币
    ├── roundtable/ ← 话题圆桌（Phase 2）
    └── api/        ← HTTP handler（薄层）
```

独立库表：social 层的表不与 engine 表做外键约束，`game_id` 只是字符串索引，不是 DB 级 FK。

### 2.2 演进路径：独立服务集群

当评论量级达到需要独立扩容时（参考规格：单游戏日评论 > 10 万条），按以下路径拆分：

```
阶段 1（当前）：monolith，internal/social/ 包
阶段 2：social 服务独立部署，主服务通过 HTTP 调用
阶段 3：comment / reaction / roundtable 各自独立，按流量独立扩容
```

MVP 阶段不做过度设计，但包边界要清晰，为未来拆分留好接缝。

---

## 三、五个子模块的后端映射

### 3.1 BasicCommentMode — 嵌套盖楼

**形态：** 树形 Thread，支持主楼 + 楼中楼回复。

**核心表：**
```sql
comments (
    id          UUID PRIMARY KEY,
    game_id     TEXT NOT NULL,          -- 绑定游戏
    author_id   TEXT NOT NULL,
    parent_id   UUID REFERENCES comments(id),  -- NULL = 主楼
    root_id     UUID,                   -- 主楼 ID（加速楼中楼查询）
    content     TEXT NOT NULL,
    thread_type TEXT DEFAULT 'nested',  -- nested | linear
    status      TEXT DEFAULT 'visible', -- visible | hidden | deleted
    created_at  TIMESTAMPTZ,
    updated_at  TIMESTAMPTZ
)
```

`parent_id` 为 NULL 时是主楼，非 NULL 时是回复。`root_id` 冗余存储主楼 ID，避免递归查询。

**API：**
```
POST   /api/social/games/:id/comments          发主楼
POST   /api/social/comments/:id/replies        回复某楼
GET    /api/social/games/:id/comments          主楼列表（分页）
GET    /api/social/comments/:id/replies        某楼的回复列表
```

### 3.2 LinearFeedComment — 线性信息流

**形态：** 纯时序气泡流，无嵌套，适合泛聊。

复用同一张 `comments` 表，`thread_type = 'linear'`，`parent_id` 永远为 NULL。
查询时按 `created_at DESC` 分页，不做树形组装。

**API 与 BasicComment 共用同一套路由**，由 `thread_type` 区分渲染形态。
前端根据游戏的 `comment_config.default_mode` 决定用哪种渲染方式。

### 3.3 TopicRoundtableModule — 话题圆桌（Phase 2）

**形态：** 独立隔离的小会话域，有准入门槛。

**核心表：**
```sql
roundtables (
    id           UUID PRIMARY KEY,
    game_id      TEXT NOT NULL,
    creator_id   TEXT NOT NULL,
    title        TEXT,
    access_type  TEXT,  -- open | password | tag_match | apply
    access_config JSONB, -- {"password": "xxx"} 或 {"required_tags": ["speedrun"]}
    status       TEXT DEFAULT 'active'
)

roundtable_members (
    roundtable_id UUID,
    user_id       TEXT,
    joined_at     TIMESTAMPTZ,
    PRIMARY KEY (roundtable_id, user_id)
)
```

圆桌内的评论仍写入 `comments` 表，增加 `roundtable_id` 字段关联。

**游戏设计者控制层：**
```sql
game_comment_config (
    game_id              TEXT PRIMARY KEY,
    enabled_modes        TEXT[],  -- ["nested","linear","roundtable"]
    default_mode         TEXT,
    roundtable_enabled   BOOLEAN DEFAULT true,
    roundtable_priority  INT DEFAULT 0  -- 展示权重
)
```

游戏设计者通过 `PATCH /api/create/games/:id/comment-config` 管理本游戏的评论形态配置。

### 3.4 CustomCommentEngine — 自定义评论引擎（Phase 3，重度游戏专属）

**形态：** 创作者自定义渲染逻辑 + 排序算法。

**工程意见（见第五章）：** 这个子模块风险最高，MVP 阶段不实现。
占位设计：`game_comment_config` 表预留 `custom_renderer_url TEXT` 字段，指向创作者自托管的渲染配置 JSON。
平台侧不执行任何创作者代码，只透传配置给前端渲染。

### 3.5 Sort&AntiFilterModule — 排序与反茧房

**排序维度（MVP 实现）：**
- `hot`：`(likes * 2 + replies) / (age_hours + 2)^1.5`（Wilson score 简化版）
- `new`：`created_at DESC`

**"最像"排序（Phase 2，依赖用户标签系统）：**
- 需要 `user_tags` 表（用户自选兴趣标签）
- 查询时按 `author_tags ∩ viewer_tags` 的交集大小排序
- 纯标签匹配，无向量计算，低成本

**Mandatory Diversity Injection（反茧房，Phase 2）：**
- 策略：每 N 条相似内容后强制插入 1 条跨标签内容
- 实现：查询时分两段：主查询（相似标签）+ 补充查询（排除已有标签），在应用层合并
- N 值可配置，默认 5

---

## 四、UniversalInteractionCapability — 公共互动能力

点赞/收藏/投币/关注复用同一套多态表：

```sql
reactions (
    id          UUID PRIMARY KEY,
    target_type TEXT NOT NULL,  -- comment | post | game
    target_id   TEXT NOT NULL,
    author_id   TEXT NOT NULL,
    type        TEXT NOT NULL,  -- like | favorite | coin
    created_at  TIMESTAMPTZ,
    UNIQUE (target_type, target_id, author_id, type)
)
```

统计数（likes_count 等）用 Redis 计数器缓存，异步写回 DB。MVP 阶段直接 COUNT 查询，无需 Redis。

---

## 五、工程疑问与意见

### 5.1 JWT 认证前置依赖

当前系统用 `X-Account-ID` 无签名 header，任何人可伪造。
评论发布、点赞、圆桌准入都需要可信身份。

**意见：** social 层启动前必须先完成 P-4B（JWT Auth）。否则评论系统的权限控制形同虚设。
可以先做 social 层的数据模型和只读 API，写入 API 等 P-4B 完成后再开放。

### 5.2 "最像"排序依赖用户标签系统

用户标签（兴趣标签、游玩风格标签）目前没有对应的数据模型。
`Sort&AntiFilterModule` 的 TagBased 排序需要先建立用户标签体系。

**意见：** Phase 2 实现"最像"排序时，同步设计 `user_profile` 表（含 `tags TEXT[]` 字段）。
MVP 阶段只做 hot/new 两种排序，不阻塞上线。

### 5.3 local_only 游戏的评论策略

`storage_policy = local_only` 的游戏，存档只在本地，但游戏模板（`game_id`）是云端注册的。

**结论：** 评论绑定的是 `game_id`（GameTemplate），不是 session。local_only 游戏完全可以有云端评论区，两者不冲突。
需要在游戏详情页说明："本游戏存档仅本地保存，但评论区为公开云端数据"。

### 5.4 CustomCommentEngine 的沙箱风险

规格文档说"支持重前端游戏作者自研布局、交互逻辑、排序算法"。

**意见：** 如果平台执行创作者提交的排序算法代码，这是严重的安全风险（任意代码执行）。
建议限制为：创作者只能提交**配置 JSON**（指定字段权重、排序规则枚举值），平台侧用固定代码执行。
真正的"自定义渲染"只在前端层面（创作者提供自己的前端页面），后端不执行任何创作者代码。

### 5.5 圆桌准入的执行位置

"圆桌创建者可配置准入门槛"——这个验证逻辑应该在后端执行，不能依赖前端。

**设计：** `POST /api/social/roundtables/:id/join` 接口在后端验证 `access_config`：
- `password`：对比哈希
- `tag_match`：查 `user_profile.tags` 是否满足 `required_tags`
- `apply`：创建申请记录，等待创建者审批

游戏设计者的"最终决策权"通过 `game_comment_config` 表实现，不是运行时权限覆盖。

### 5.6 投币机制的经济系统依赖

"投币"需要平台虚拟货币系统（用户余额、扣款、防刷）。

**意见：** MVP 阶段去掉投币，只做点赞和收藏。投币是独立的经济子系统，不应该和评论模块耦合。
`reactions` 表预留 `type = 'coin'` 枚举值，但不实现扣款逻辑，等经济系统就绪后再接入。

---

## 六、MVP 范围界定

| 子模块 | MVP | Phase 2 | Phase 3 |
|--------|-----|---------|---------|
| BasicCommentMode（嵌套盖楼）| ✅ | — | — |
| LinearFeedComment（线性信息流）| ✅ | — | — |
| TopicRoundtableModule（圆桌）| ⬜ | ✅ | — |
| CustomCommentEngine（自定义引擎）| ⬜ | ⬜ | ✅ |
| 排序：hot / new | ✅ | — | — |
| 排序：最像（TagBased）| ⬜ | ✅ | — |
| Mandatory Diversity Injection | ⬜ | ✅ | — |
| 点赞 / 收藏 | ✅ | — | — |
| 投币 | ⬜ | ⬜ | ✅ |
| game_comment_config 配置表 | ✅ | — | — |

MVP 的 social 层启动条件：P-4B（JWT Auth）完成后。

---

## 七、与 P-5E 的对应关系

本文是 `P-5E Social 层启动` 的互动子系统细化设计。
P-5E 中定义的 `Post`（游记）和 `Comment` 是 social 层的两个并列模块：

```
internal/social/
├── post/       ← 游记/攻略（P-5E 已定义数据模型）
├── comment/    ← 本文描述的评论子系统
├── reaction/   ← 点赞/收藏（本文）
└── roundtable/ ← 圆桌（Phase 2）
```

游记（Post）和评论（Comment）共用 `reactions` 表，`target_type` 区分。
