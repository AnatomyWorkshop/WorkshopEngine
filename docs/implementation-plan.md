# backend-v2 完整实现计划

> 原则：深度解耦、One-Shot LLM、每个节点对外有明确接口文档
> 状态更新：2026-04-04

---

## 模块总览

```
backend-v2/
├── cmd/
│   ├── server/main.go          # HTTP 服务入口
│   └── worker/main.go          # 异步任务处理器（记忆摘要/角色卡解析）
├── internal/
│   ├── core/                   # 基础设施层（全局共享，不含业务逻辑）
│   │   ├── config/             # 配置加载（.env / yaml）
│   │   ├── db/                 # 数据库连接与迁移（GORM + PostgreSQL）
│   │   ├── llm/                # LLM 客户端（OpenAI 兼容接口 + SSE 流式）
│   │   └── queue/              # 轻量任务队列（内存 channel，异步 worker）
│   │
│   ├── engine/                 # 模块 1：游戏引擎层（复刻 TH 核心）
│   │   ├── prompt_ir/          # [已完成] Prompt 中间表示与节点接口
│   │   ├── pipeline/           # [已完成] 流水线节点实现
│   │   ├── variable/           # [已完成] 五级变量沙箱
│   │   ├── types/              # [已完成] 三层消息结构
│   │   ├── parser/             # [待实现] AI 响应结构化解析器
│   │   ├── memory/             # [待实现] 记忆系统（存储+检索+异步整合）
│   │   ├── session/            # [待实现] 会话/楼层/消息页生命周期管理
│   │   └── api/                # [待实现] HTTP 游玩接口（/api/v2/play）
│   │
│   ├── creation/               # 模块 2：创作工具层
│   │   ├── card/               # [待实现] 角色卡解析（PNG tEXt → CCv2/v3 JSON）
│   │   ├── lorebook/           # [待实现] 世界书 CRUD 与触发规则管理
│   │   ├── template/           # [待实现] 游戏模板 CRUD
│   │   ├── asset/              # [待实现] 素材管理（上传/CDN映射）
│   │   └── api/                # [待实现] HTTP 创作接口（/api/v2/create）
│   │
│   ├── social/                 # 模块 3：社区层（复用现有 backend/ 逻辑）
│   └── user/                   # 模块 4：用户鉴权（JWT，复用现有逻辑）
```

---

## 各节点详细计划

### 1. `internal/core/config` ✅ 待实现
**职责**：加载 .env / yaml 配置，暴露强类型结构体  
**对外接口**：
```go
type Config struct {
    DB      DBConfig
    LLM     LLMConfig    // BaseURL, APIKey, Model, TimeoutSec
    Server  ServerConfig // Port, CORSOrigins
    Worker  WorkerConfig // MemorySummaryIntervalRounds
}
func Load() (*Config, error)
```

---

### 2. `internal/core/db` ✅ 待实现
**职责**：数据库连接（PostgreSQL/SQLite 均支持）、GORM 迁移  
**关键模型**（复刻 TH 的三层消息结构）：
```go
// Game Session
GameSession { ID, GameID, UserID, Variables JSONB, MemorySummary text, CreatedAt }
// Floor（回合，提交后不可改）
Floor       { ID, SessionID, Seq int, Status enum, CreatedAt }
// MessagePage（每一次生成尝试，是 Swipe 的实体）
MessagePage { ID, FloorID, IsActive bool, Messages JSONB, PageVars JSONB, TokenUsage int }
// Memory（异步摘要）
Memory      { ID, SessionID, Content text, Type enum, Importance float, CreatedAt }
// CharacterCard
CharacterCard { ID, Slug, Name, Spec, Data JSONB, AvatarURL, Tags }
// WorldbookEntry
WorldbookEntry { ID, GameID, Keys []string, Content text, Constant bool, Priority int, Enabled bool }
```

---

### 3. `internal/core/llm` ✅ 待实现
**职责**：OpenAI 兼容 HTTP 客户端，支持非流式（一次返回）和 SSE 流式  
**对外接口**：
```go
type Client interface {
    Chat(ctx context.Context, messages []Message, opts Options) (string, Usage, error)
    ChatStream(ctx context.Context, messages []Message, opts Options) (<-chan string, error)
}
type Options struct { Model, Temperature, MaxTokens, Stop []string }
type Usage  struct { PromptTokens, CompletionTokens, TotalTokens int }
```
**设计要点**：
- 默认非流式（One-Shot），SSE 版本供前端实时打字动画
- 支持 Timeout 和自动重试（指数退避，rate limit 场景）

---

### 4. `internal/engine/parser` ✅ 待实现（最关键节点之一）
**职责**：从 LLM 原始输出中提取结构化内容。TH 叫 "接收后处理"。  
**输入**：AI 的原始文本  
**输出**：
```go
type ParsedResponse struct {
    Narrative  string          // 给玩家展示的叙事
    Options    []string        // 选项按钮
    StatePatch map[string]any  // 变量更新（写入 Page 沙箱）
    Summary    string          // 记忆摘要（交给异步 worker 落库）
    BGM        string          // 可选：背景音乐指令
    BG         string          // 可选：背景图指令
    RawXML     string          // 调试用的原始输出
}
```
**解析策略**（三层回退，不依赖 LLM 完美跟随）：
1. 优先解析 `<game_response>` 或 `<Narrative>/<Options>/<UpdateState>/<Summary>` XML 标签
2. 降级：检测编号列表 `1. 选项A` `2. 选项B`
3. 兜底：固定选项 `["继续", "环顾四周"]`

---

### 5. `internal/engine/session` ✅ 待实现
**职责**：管理 Floor 状态机 + MessagePage 的生命周期  
**对外接口**：
```go
// 开始一次回合（创建 Floor draft + Page v1）
func StartTurn(sessionID string, userInput string) (floorID, pageID string, err error)
// 生成成功后锁定
func CommitTurn(pageID string, response ParsedResponse, newVars map[string]any) error
// 玩家不满意，重新生成（在同一 Floor 创建新 Page，旧 Page 标记 inactive）
func RegenTurn(floorID string) (newPageID string, err error)
// 生成失败（保留现场供重试）
func FailTurn(pageID string, reason string) error
```

---

### 6. `internal/engine/memory` ✅ 待实现
**职责**：记忆摘要的写入、检索、异步整合  
**组成**：
- `Store`：CRUD GameSession.MemorySummary 和 memories 表
- `Injector`：MemoryNode 调用，按 Token 预算读取最近 N 条，拼成注入文本
- `Worker`（运行在 `cmd/worker`）：每隔 K 回合触发一次摘要整合，调用廉价模型生成新摘要追加入库
**对外接口（节点侧）**：
```go
func (m *MemoryStore) GetSummaryForInjection(sessionID string, tokenBudget int) (string, error)
func (m *MemoryStore) SaveFromParser(sessionID string, summary string) error
```

---

### 7. `internal/creation/card` ✅ 待实现
**职责**：解析 SillyTavern 角色卡 PNG（tEXt chunk → base64 → CCv2/v3 JSON）  
**对外接口**：
```go
func ParsePNG(r io.Reader) (*CharacterCardData, error)  // 同时支持 ccv2 和 ccv3
type CharacterCardData struct {
    Name, Description, Personality, Scenario, FirstMes string
    CharacterBook *Lorebook
    Tags          []string
    Extensions    map[string]any
}
```
**复用现有**：`backend/handlers/card_import.go` 已有雏形，搬到新结构中正规化。

---

### 8. `internal/creation/lorebook` ✅ 待实现
**职责**：世界书条目的 CRUD，以及对接 Pipeline 的 WorldbookNode  
**设计**：世界书与游戏模板是多对多关系（一个世界书可以挂多个游戏）  
**对外接口**：
```go
func ListEntries(gameID string) ([]WorldbookEntry, error)
func UpsertEntry(entry WorldbookEntry) error
func DeleteEntry(id string) error
func GetTriggeredEntries(gameID string, recentText string) ([]WorldbookEntry, error)
```

---

### 9. `internal/engine/api` + `internal/creation/api` ✅ 待实现
**职责**：HTTP 路由，使用 Gin（与现有 backend 保持一致）  
**游玩接口（/api/v2/play）**：
```
POST /api/v2/play/sessions              创建游玩会话
POST /api/v2/play/sessions/:id/turn     提交操作（选项点击 / 自由输入）
POST /api/v2/play/sessions/:id/regen    重新生成当前回合
GET  /api/v2/play/sessions/:id/state   获取当前游戏状态（变量快照 + 历史）
GET  /api/v2/play/sessions/:id/stream  SSE 流式输出（打字动画）
```
**创作接口（/api/v2/create）**：
```
POST /api/v2/create/cards/import        导入角色卡 PNG
GET  /api/v2/create/cards               列出角色卡
GET  /api/v2/create/cards/:id           角色卡详情
POST /api/v2/create/templates           创建游戏模板
GET  /api/v2/create/templates/:id/lorebook  获取世界书
POST /api/v2/create/templates/:id/lorebook  更新世界书条目
POST /api/v2/create/assets/:slug/upload 素材上传
```

---

## 实现顺序（优先级）

| 顺序 | 模块 | 原因 |
|------|------|------|
| 1 | `core/config` + `core/db` | 其他所有模块的基础 |
| 2 | `core/llm` | engine 和 worker 都需要它 |
| 3 | `engine/parser` | 决定游戏能否正确理解 AI 输出 |
| 4 | `engine/session` | Floor/Page 状态机，决定重试逻辑 |
| 5 | `engine/memory` | 长对话场景的核心解法 |
| 6 | `engine/api` | 把 1-5 串成可调用的 HTTP 接口 |
| 7 | `creation/card` | 导入角色卡 |
| 8 | `creation/lorebook` | 世界书在线编辑 |
| 9 | `creation/api` | 创作工具 HTTP 接口 |
| 10 | `cmd/worker` | 异步记忆摘要任务 |
