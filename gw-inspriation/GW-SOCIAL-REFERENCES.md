# Social 层开源参考项目

> 版本：2026-04-08
> 用途：设计 `internal/social/` 时的参考对象，可直接拷贝数据模型、API 结构或排序算法。
> 原则：参考设计思路，不直接嵌入 GPL 许可代码；MIT/Apache 许可的工具库可直接引入。

---

## 一、核心参考：评论 + 论坛系统

### 1-A  Artalk（★ 首选，Go 实现）

**仓库：** https://github.com/ArtalkJS/Artalk
**许可：** MIT
**语言：** Go + TypeScript
**为什么：** 这是最直接的参考——Go 写的自托管评论系统，支持嵌套评论、多站点、邮件通知、
管理后台。数据模型、GORM 用法、分页接口设计和我们的技术栈完全一致。

**可直接参考的部分：**
- `server/handler/comment/` — 评论 CRUD、嵌套查询（parent_id 树形重建）
- `server/dao/comment.go` — GORM 评论模型定义（字段名、索引设计）
- `server/handler/vote/` — 点赞/踩实现（多态 target_type + target_id）
- 分页 cursor 设计（Artalk 用 offset，但有 cursor 分支可参考）
- 敏感词过滤接入（支持自定义词库 + 外部服务）

**值得拷贝研究的文件：**
```
server/model/entity/comment.go   ← Comment 结构体定义
server/model/entity/vote.go      ← Vote（点赞）多态模型
server/handler/comment/list.go   ← 评论列表接口，含树形重建逻辑
server/handler/comment/create.go ← 发评论，权限校验流程
```

---

### 1-B  Flarum（论坛数据模型参考，PHP 但设计清晰）

**仓库：** https://github.com/flarum/framework
**许可：** MIT
**语言：** PHP + JavaScript
**为什么：** Flarum 的数据模型设计极其清晰，Discussion（主帖）/ Post（回复）/ Tag 三层结构
与我们的 forum/Post + forum/ForumReply + game_tags 完全对应。
虽然是 PHP，但数据模型 migration 文件可以直接翻译成 Go struct。

**可直接参考的部分：**
- `framework/core/migrations/` — Discussion、Post、Tag 表的字段设计
- Tag 与 Discussion 的多对多关联（对应我们 Post.GameTags[]）
- 话题置顶 / 精华 / 锁定状态字段设计
- 帖子"最后回复时间"缓存字段（`last_posted_at`）避免每次 COUNT JOIN

**值得翻译研究的文件：**
```
framework/core/migrations/2018_09_15_000000_create_discussions_table.php
framework/core/migrations/2018_09_15_000000_create_posts_table.php
framework/core/migrations/2018_09_15_000000_create_tags_table.php
```

---

### 1-C  Discourse（排序算法 + 反茧房）

**仓库：** https://github.com/discourse/discourse
**许可：** GPL-2.0（代码不可直接复制，但算法思路公开）
**语言：** Ruby on Rails
**为什么：** Discourse 的帖子排序算法（热度公式）和"相关话题"推荐是业界标杆。
我们的 Sort&AntiFilterModule 可以直接参考其热度计算公式。

**参考的算法（不复制代码，只参考公式）：**

热度分：
```
score = (likes * 2 + replies * 3 + views * 0.2)
      / (age_hours + 2) ^ 1.8
```

"你可能感兴趣"推荐：按标签交集 + 时效性加权，与我们的 TagBased 排序思路一致。

**具体参考文件（只读逻辑）：**
```
app/models/topic_hot_score.rb   ← 热度分计算
app/services/topic_query.rb     ← 多条件查询链式构建（参考接口设计）
```

---

## 二、点赞 / 互动能力

### 2-A  go-social-graph（轻量关注关系）

**仓库：** https://github.com/alash3al/go-chat（类似思路的 Go 示例）
**更好的参考：** Artalk 的 Vote 实现（见 1-A）

对于我们的 `reaction/` 包（点赞/收藏），最简单的设计是多态表：
```sql
reactions (target_type TEXT, target_id TEXT, author_id TEXT, type TEXT)
UNIQUE (target_type, target_id, author_id, type)
```
这个模式来自 GitHub 的 Reaction API 设计，也被 Gitea 采用：

**仓库：** https://github.com/go-gitea/gitea
**许可：** MIT
**参考文件：**
```
models/issues/reaction.go       ← 多态 Reaction 模型（issue/comment 共用）
routers/api/v1/repos/issue_reaction.go ← 点赞接口
```
Gitea 的 Reaction 表结构几乎可以原样使用。

---

## 三、内容审核

### 3-A  go-dfa（DFA 敏感词过滤）

**仓库：** https://github.com/antlinker/go-dirtyfilter
**许可：** MIT
**用途：** 评论/帖子发布时的敏感词过滤，DFA 算法，O(n) 时间复杂度。

备选（更活跃）：
**仓库：** https://github.com/importcjj/sensitive
**许可：** MIT
**用法：**
```go
filter := sensitive.New()
filter.LoadWordDict("dict/sensitive_words.txt")
ok, _ := filter.Validate("评论内容")
```
可以直接作为 `go get` 依赖引入，无需自己实现。

---

## 四、全文搜索

### 4-A  PostgreSQL 原生全文搜索（推荐，零依赖）

不需要额外引入 Elasticsearch 或 MeiliSearch，PostgreSQL 的 `tsvector` + `tsquery` 完全够用：

```sql
-- 论坛帖子搜索索引
ALTER TABLE posts ADD COLUMN search_vector tsvector;
CREATE INDEX posts_search_idx ON posts USING GIN(search_vector);

-- 触发器自动更新
UPDATE posts SET search_vector = to_tsvector('simple', title || ' ' || content);
```

GORM 查询：
```go
db.Where("search_vector @@ plainto_tsquery('simple', ?)", keyword).Find(&posts)
```

**参考实现：** Artalk 的搜索功能（`server/handler/comment/list.go` 的关键词过滤）

---

## 五、圆桌 / 私密频道（Phase 2 参考）

### 5-A  Matrix（协议参考，不是依赖）

**仓库：** https://github.com/matrix-org/dendrite（Go 实现的 Matrix 服务器）
**许可：** Apache-2.0
**为什么看：** 圆桌的"准入门槛 + 成员管理 + 私密讨论域"在设计上和 Matrix Room 概念完全吻合。
Dendrite 的 Room State 管理方式（join_rule / member event）可以给圆桌权限设计提供思路。

**不需要依赖 Matrix SDK**，只是参考其权限模型的设计思路。

---

## 六、可直接 go get 的工具库

| 库 | 用途 | 许可 |
|----|------|------|
| `github.com/importcjj/sensitive` | 敏感词 DFA 过滤 | MIT |
| `github.com/google/uuid` | UUID 生成（已引入）| BSD-3 |
| `github.com/microcosm-cc/bluemonday` | HTML 内容净化（防 XSS）| BSD-2 |
| `github.com/yuin/goldmark` | Markdown 渲染（论坛帖子）| MIT |

**`bluemonday` 是 social 层必须引入的：** 论坛帖子如果支持 Markdown，渲染后的 HTML
必须经过 bluemonday 净化，否则用户可以注入任意脚本。

---

## 七、拷贝计划

按照以下顺序阅读和参考，进入 social/ 实现时对照使用：

```
Step 1  阅读 Artalk server/model/entity/comment.go
        → 确定 Comment 表字段设计

Step 2  阅读 Artalk server/handler/comment/list.go
        → 确定树形评论重建算法（parent_id → children map）

Step 3  阅读 Gitea models/issues/reaction.go
        → 确定 Reaction 多态表设计

Step 4  翻译 Flarum Discussion + Post migration
        → 确定 forum/Post + ForumReply 字段

Step 5  参考 Discourse 热度公式
        → 实现 Sort&AntiFilterModule 的 hot 排序

Step 6  引入 bluemonday + sensitive
        → 帖子发布管线：Markdown 渲染 → HTML 净化 → 敏感词过滤
```
