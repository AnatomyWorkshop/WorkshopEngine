# WorkshopEngine

一个以 **LLM 对话为核心的无头交互运行时**（Headless AI Interaction Runtime）。

WorkshopEngine 是 **GameWorkshop（GW）** 和 **CreationWorkshop（CW）** 两个产品的共同后端。

---

## 两个产品

### GameWorkshop（GW）— 玩家平台
玩家在这里发现和游玩 AI 互动游戏，参与社区讨论，分享游玩故事。

- AI 文字游戏 / 视觉小说（SSE 流式输出）
- 社区论坛（帖子 / 盖楼 / 按游戏聚合）
- 评论区（嵌套回复树）
- 点赞 / 收藏 / 存档分享

前端：React + TypeScript（规划中，见 `GameWorkshop/inspiration/`）

### CreationWorkshop（CW）— 创作工具
游戏设计者在这里构建和发布 AI 游戏。

- 角色卡 / 世界书 / 预设条目编辑
- LLM Profile 配置（多模型多 slot 调度）
- 正则规则编辑器
- 素材管理（图片 / 音频上传）

---

## 引擎能力

WorkshopEngine 不绑定任何前端框架，也不限定使用场景。它提供一套通用的 **会话管理 → Prompt 编排 → LLM 调用 → 状态持久化** 管道，驱动任何需要"与 AI 来回推进"的应用：互动叙事游戏、对话式工具、角色扮演……都是同一条主链路的不同配置。

### 会话结构（Session / Floor / Page）

```
Session（一次游戏/交互）
  └── Floor（一个回合，提交后不可改）
        └── Page（同一回合的多个生成结果，可 Swipe 切换）
```

- 每个 Floor 对应一条用户输入 + 一条 AI 回复
- 同一 Floor 支持多个 Page（重新生成/Swipe），切换激活 Page 时变量快照自动提升
- **Session Fork**：从任意 FloorSeq 复制全段历史，创建平行时间线

### 变量沙箱（五层）

```
Page → Floor → Branch → Chat → Global
```

AI 回复中的 `<UpdateState>` 写入 Page 层；Floor 提交后提升至 Chat；创作者可在任意层写入全局变量。所有层级在 Prompt 组装前合并展开，用于宏替换 `{{var}}`。

### Prompt Pipeline（可编排节点）

| 节点 | 职责 | 优先级 |
|------|------|--------|
| TemplateNode | System Prompt 模板 + `{{var}}` 宏替换 | 1000（兜底） |
| PresetNode | 条目化 Prompt（injection_order 可排序） | 由 injection_order 直接决定 |
| WorldbookNode | 关键词 / 正则 / 次级逻辑门触发注入 | 10–510 |
| MemoryNode | 时间衰减记忆摘要 | 400 |
| HistoryNode | 近期历史消息 | 0 ~ -N |

### 世界书（Worldbook）— 完整对齐 SillyTavern

| 功能 | 说明 |
|------|------|
| 主关键词触发 | 子串匹配，大小写不敏感 |
| 正则关键词 | `regex:pattern` 前缀，支持 `/pattern/flags` 语法 |
| 全词匹配 | `whole_word: true` |
| 次级关键词 + 逻辑门 | `AND_ANY / AND_ALL / NOT_ANY / NOT_ALL` |
| 扫描深度 | `scan_depth`：只扫最近 N 条消息 |
| 注入位置 | `before_template / after_template`（对应优先级） |
| 常驻词条 | `constant: true`，不论关键词是否命中，永远注入 |
| 递归激活 | 已激活词条内容触发二次扫描，激活更多词条 |
| 变量门控 | `var:stage=confrontation` 变量值条件激活 |
| 互斥分组 | `group` + `group_weight`，同组只保留权重最高的词条 |

### 记忆系统

- 每轮 AI 回复的 `<Summary>` 字段**异步**写入 Memory 表
- **时间衰减排序**：指数半衰期（可配 `HalfLifeDays` + `MinDecayFactor`），旧记忆自然降权
- **维护 Worker**：按回合数触发整合（廉价模型重新摘要）、自动废弃/清理过期记忆
- Memory 类型：`summary`（叙事摘要）+ `fact`（关键事实）

### LLM 响应解析（三层回退）

```
<Narrative>/<game_response> XML  →  编号列表（1. / ①.）  →  纯文本降级
```

解析结果统一为 `ParsedResponse`：

| 字段 | 来源 |
|------|------|
| `narrative` | `<Narrative>` 标签内容，或纯文本 |
| `options` | `<Options><option>` 或编号列表 |
| `state_patch` | `<UpdateState>` JSON 内联 patch |
| `summary` | `<Summary>` 标签 |
| `vn` | `<game_response>` 内的 VN 指令（见下） |

### 视觉小说指令集（VN Directives）

```
[bg|city_night.jpg]          背景图
[bgm|rain_ambient]           背景音乐
旁白||夜色深沉...             旁白行（无立绘）
夜歌|nightsong_sad.png|……    角色台词行（角色名 + 立绘 + 台词）
[choice|回应她|保持沉默]     选项
```

引擎解析为 `VNDirectives{BG, BGM, Lines[], Options[]}`，前端按帧渲染。

### Agentic Tool Loop

- 引擎内置，最多 5 轮工具调用迭代
- **Tool 重放安全分级**：`safe / confirm_on_replay / never_auto_replay / uncertain`

**内置工具**

| 工具 | 功能 |
|------|------|
| `get_variable` / `set_variable` | 读写变量沙箱（支持点分路径） |
| `search_memory` | 按相关度检索记忆摘要 |
| `create/update_worldbook_entry` | 动态管理世界书词条 |
| `get/create/update_preset_entry` | 动态管理 Preset 条目 |
| `create/update_memory` | 主动写入 / 修改记忆 |
| `search_material` / `read_material` | 素材库检索（JSONB tag 匹配，按使用频次分发） |

### 社交层（Social）

| 包 | 功能 |
|---|---|
| `social/reaction` | 点赞 / 收藏（polymorphic，支持 comment / forum_post / forum_reply） |
| `social/comment` | 游戏评论区（Artalk 双索引树，嵌套 / 线性两种模式） |
| `social/forum` | 社区论坛（发帖 / 盖楼 / 热度排序 / Markdown 渲染 / XSS 净化） |

---

## 目录结构

```
backend-v2/
├── cmd/
│   ├── server/main.go          HTTP 服务入口（:8080）
│   └── worker/main.go          异步记忆维护 Worker
├── internal/
│   ├── core/                   基础设施（DB / LLM Client / Config / Util）
│   ├── platform/               横切（Auth / Gateway 中间件 / Provider 注册表）
│   ├── engine/                 AI 游戏引擎（Pipeline / Memory / Parser / Tools）
│   ├── creation/               创作工具 API（模板 / 世界书 / 素材库）
│   └── social/                 社交层（Reaction / Comment / Forum）
├── .test/                      所有测试（独立 module mvu-backend/test）
│   ├── unit_test.go            单元测试（70 个，无网络 / 无 DB）
│   ├── integration_llm_test.go LLM 集成测试（-tags integration）
│   └── integration_social_test.go Social DB 集成测试（-tags integration）
└── gw-inspriation/             设计文档 / 参考资料 / 工作日志
```

---

## 快速启动

**依赖：** Go 1.22+，PostgreSQL 14+

```bash
cp .env.example .env   # 填写以下关键字段

# .env 关键字段
# DB_DSN=host=localhost user=postgres password=postgres dbname=workshop sslmode=disable
# LLM_BASE_URL=https://api.deepseek.com
# LLM_API_KEY=sk-...
# LLM_MODEL=deepseek-chat
# ADMIN_KEY=your-admin-key
# CORS_ORIGINS=http://localhost:5173

go run ./cmd/server     # :8080，表结构由 GORM AutoMigrate 自动创建
```

---

## API 路由

```
# 游玩
POST   /api/play/sessions                         创建会话
POST   /api/play/sessions/:id/turn                推进回合（One-Shot）
GET    /api/play/sessions/:id/stream              推进回合（SSE 流式）
POST   /api/play/sessions/:id/fork                分叉平行时间线
GET    /api/play/sessions/:id/floors              楼层历史
PATCH  /api/play/floors/:fid/pages/:pid/activate  切换激活 Page

# 创作
POST   /api/create/templates                      游戏模板 CRUD
POST   /api/create/templates/:id/preset-entries   Preset Entry 管理
POST   /api/create/worldbook-entries              世界书词条 CRUD
POST   /api/create/regex-profiles                 Regex Profile 管理
POST   /api/create/templates/:id/materials        素材库 CRUD

# 社交
POST   /api/social/reactions/:target_type/:target_id/:type   点赞/收藏
GET    /api/social/reactions/counts               批量查计数
POST   /api/social/games/:id/comments             发评论
GET    /api/social/games/:id/comments             评论列表（线性 / 嵌套）
GET    /api/social/posts                          论坛帖子列表
POST   /api/social/posts                          发帖
POST   /api/social/posts/:id/replies              盖楼
GET    /api/social/games/:id/stats                游戏社交聚合统计
```

---

## 运行测试

```bash
cd .test

# 单元测试（无任何外部依赖，70 个）
go test -v -count=1

# LLM 集成测试
LLM_API_KEY=sk-... go test -tags integration -v -run LLM

# Social DB 集成测试
TEST_DSN="host=localhost user=postgres password=postgres dbname=gw_test sslmode=disable" \
  go test -tags integration -v -run Social
```

---

## 路线图

| Phase | 状态 | 内容 |
|-------|------|------|
| Phase 1 | ✅ | 引擎核心（Session / Pipeline / Memory / Tools / Parser） |
| Phase 2 | ✅ | 世界书增强（GroupCap / VarGate / 递归激活） |
| Phase 3 | ✅ | LLM 发现 / 测试 + 社交层（Reaction / Comment / Forum） |
| Phase 4 | ⏳ | MCP 工具接入 / 用户自定义工具 / JWT 鉴权 |
| Phase 5 | ⏳ | Director 槽 / Verifier 槽 / 多 Provider |

---

## License

见 [LICENSE](../LICENSE)（BUSL-1.1）。
