# Preset Tool — 用户自定义工具动态加载（Phase 2）

目标：创作者通过 API 注册工具定义（名称、描述、参数 Schema、执行端点），引擎在每回合动态加载并注入 LLM 工具调用链路，跑通"外部工具动态加载"的完整链路。

比 MCP 简单：不需要 stdio/HTTP transport 协议，只需要一个 HTTP 回调 URL。

---

## 设计思路

### 工具执行模型

内置工具（`get_variable`、`set_variable` 等）在引擎进程内执行。  
Preset Tool 在**引擎进程外**执行——引擎收到 LLM 的 tool_call 后，向创作者注册的 `endpoint` 发一个 HTTP POST，把参数转发过去，把响应转回给 LLM。

```
LLM → tool_call{name, args}
  → 引擎查 PresetTool 表，找到 endpoint
  → POST endpoint {session_id, floor_id, args}
  → 创作者服务返回 {result: "..."}
  → 引擎把 result 作为 tool message 追加给 LLM
```

### 数据模型

```
PresetTool
  id          uuid
  game_id     string   // 绑定到哪个游戏模板
  name        string   // 工具名（LLM 调用时用，per-game 唯一）
  description string   // 工具描述（注入 LLM）
  parameters  jsonb    // JSON Schema object（LLM 参数定义）
  endpoint    string   // HTTP POST URL（引擎回调）
  timeout_ms  int      // 超时，默认 5000
  enabled     bool
  created_at  time
```

### 注册方式

创作者调用 `POST /api/v2/create/templates/:id/tools`，传入工具定义。  
引擎在 `PlayTurn` / `StreamTurn` 加载工具时，额外查一次 `preset_tools` 表，把启用的工具包装成 `HttpCallTool` 注册进 `Registry`。

`enabled_tools` 配置里加 `"preset:*"` 表示启用该游戏所有 Preset Tool，也可以单独列 `"preset:tool_name"`。

---

## 执行步骤

### Step 1 — 数据模型（models.go + connect.go）

新增 `PresetTool` 结构体，加入 AutoMigrate。

**文件**：`internal/core/db/models.go`、`internal/core/db/connect.go`

---

### Step 2 — HttpCallTool（tools 包）

新建 `internal/engine/tools/http_tool.go`，实现 `Tool` 接口。

`Execute` 逻辑：
- 构造请求体 `{session_id, floor_id, args}`（`floor_id` 通过 context 传入，用 `context.WithValue`）
- `http.Post` 到 `endpoint`，超时用 `context.WithTimeout`
- 解析响应 `{result: string}` 或直接把响应 body 作为 result
- 失败返回 `{"error": "..."}` 字符串，不上抛（与内置工具一致）

`ReplaySafety()` 返回 `ReplayUncertain`（外部副作用未知）。

---

### Step 3 — 加载逻辑（game_loop.go + engine_methods.go）

在工具注册块（Step 3b）末尾，追加：

```go
// 加载 Preset Tool（外部 HTTP 回调工具）
if _, ok := enabled["preset:*"]; ok || hasPresetPrefix(tmplCfg.EnabledTools) {
    var presetTools []dbmodels.PresetTool
    e.db.Where("game_id = ? AND enabled = true", sess.GameID).Find(&presetTools)
    for _, pt := range presetTools {
        if _, ok := enabled["preset:*"]; ok {
            toolReg.Register(tools.NewHttpCallTool(pt, req.SessionID))
        } else if _, ok := enabled["preset:"+pt.Name]; ok {
            toolReg.Register(tools.NewHttpCallTool(pt, req.SessionID))
        }
    }
}
```

`hasPresetPrefix` 是一个小辅助函数，检查 `EnabledTools` 里是否有 `"preset:"` 前缀的条目。

---

### Step 4 — CRUD API（creation/api/routes.go）

在 creation routes 末尾追加 `/templates/:id/tools` 路由组：

```
GET    /templates/:id/tools          列出工具
POST   /templates/:id/tools          创建工具
PATCH  /templates/:id/tools/:tid     更新工具
DELETE /templates/:id/tools/:tid     删除工具
```

---

### Step 5 — 端到端验证

写一个最小 echo server（`backend-v2/.test/preset_tool_echo/main.go`）：

```go
// POST / → 把收到的 args 原样返回
// {"result": "echo: <args json>"}
```

用 curl 走完整链路：
1. 创建游戏模板
2. 注册 Preset Tool（endpoint 指向 echo server）
3. 在模板 config 里加 `"enabled_tools": ["preset:*"]`
4. 创建会话，发一条能触发工具调用的消息
5. 检查 `tool_execution_records` 表，确认有记录

---

## 文件变更清单

| 文件 | 操作 |
|---|---|
| `internal/core/db/models.go` | 新增 `PresetTool` 结构体 |
| `internal/core/db/connect.go` | AutoMigrate 加 `&PresetTool{}` |
| `internal/engine/tools/http_tool.go` | 新建，实现 `HttpCallTool` |
| `internal/engine/api/game_loop.go` | 工具注册块追加 preset 加载 |
| `internal/engine/api/engine_methods.go` | 同上（StreamTurn 同步） |
| `internal/creation/api/routes.go` | 追加 `/templates/:id/tools` CRUD |
| `.test/preset_tool_echo/main.go` | 新建，验证用 echo server |

---

## 关键约束

- **超时**：`HttpCallTool.Execute` 必须有超时（默认 5s），防止外部服务挂起阻塞 Agentic Loop
- **错误隔离**：外部工具失败只返回 error 字符串，不中断整个回合
- **安全**：endpoint 只允许 http/https，不允许 file:// 等协议（在 `NewHttpCallTool` 里校验）
- **幂等性**：`ReplaySafety = Uncertain`，Swipe 重生成时不自动重放外部工具调用

---

## 当前进度

- [x] Step 1：PresetTool 数据模型
- [x] Step 2：HttpCallTool 实现
- [x] Step 3：game_loop + engine_methods 加载逻辑
- [x] Step 4：CRUD API
- [x] Step 5：echo server 已写好（`.test/preset_tool_echo/main.go`，监听 :9090）

## 验证步骤（手动执行）

```bash
# 1. 启动 echo server
cd backend-v2/.test/preset_tool_echo && go run main.go

# 2. 启动 WE 后端
cd backend-v2 && go run ./cmd/server

# 3. 创建游戏模板（或用已有 template_id）
TMPL_ID=<your_template_id>

# 4. 注册 Preset Tool
curl -X POST http://localhost:8080/api/v2/create/templates/$TMPL_ID/tools \
  -H "Content-Type: application/json" \
  -d '{
    "name": "echo_tool",
    "description": "把参数原样返回，用于测试",
    "parameters": {"type":"object","properties":{"message":{"type":"string"}}},
    "endpoint": "http://localhost:9090/",
    "timeout_ms": 3000
  }'

# 5. 在模板 config 里启用工具
curl -X PATCH http://localhost:8080/api/v2/create/templates/$TMPL_ID \
  -H "Content-Type: application/json" \
  -d '{"config": {"enabled_tools": ["preset:*"]}}'

# 6. 创建会话并发一条能触发工具调用的消息
SESS_ID=$(curl -s -X POST http://localhost:8080/api/v2/play/sessions \
  -H "Content-Type: application/json" \
  -d "{\"game_id\":\"$TMPL_ID\"}" | jq -r '.data.session_id')

curl -X POST http://localhost:8080/api/v2/play/sessions/$SESS_ID/turn \
  -H "Content-Type: application/json" \
  -d '{"session_id":"'$SESS_ID'","user_input":"请调用 echo_tool，message 填写 hello"}'

# 7. 检查工具执行记录
curl http://localhost:8080/api/v2/play/sessions/$SESS_ID/tool-executions
```

---

## ST 预设兼容性分析

### ST 预设的核心结构

ST 预设 JSON 包含两层：

**内容层**（`prompts[]`）：每条 prompt 有 identifier、name、role、content、injection_order、enabled、marker 标志。  
**顺序层**（`prompt_order[]`）：按角色上下文（character_id）存储启用状态和排列顺序，`enabled` 在这里而不在 prompts 里。

特殊的 **marker 条目**（无 content）：`chatHistory`、`worldInfoBefore`、`worldInfoAfter`、`charDescription`、`charPersonality`、`scenario`、`dialogueExamples`、`personaDescription`——这些是位置锚点，告诉引擎在哪里插入历史/世界书/角色描述。

TH 的 `assemblePrompt()` 把 marker 解析为插槽，在对应位置注入世界书、历史、角色卡字段。

### WE 的对应关系

| ST 字段 | WE PresetEntry | 状态 |
|---|---|---|
| identifier / name / role / content | 完全对应 | ✅ |
| injection_order | injection_order（→ PromptBlock.Priority）| ✅ |
| enabled（来自 prompt_order）| enabled | ✅ |
| marker | 不需要 | WE 用 Pipeline 节点替代 |
| injection_depth | 未实现 | 可后续加字段 |

WE **不需要 marker 系统**：
- 历史消息 → `HistoryNode`（负 Priority，固定末尾）
- 世界书位置 → `WorldbookEntry.position` 字段
- 角色描述 → `SystemPromptTemplate` 宏展开

### ST 预设导入路径

`POST /api/v2/create/templates/:id/preset/import-st`，接收 ST 预设 JSON：

1. 取 `prompt_order[character_id=100000].order` 建立 `identifier → {enabled, seq}` 映射
2. 过滤掉 marker 条目（8 个）
3. 用 `order.seq × 10` 作为 `injection_order`（ST 该预设所有条目 injection_order 均为 100，无区分度）
4. 批量 Upsert `PresetEntry`

这是创作模块的导入工具，不影响引擎运行时。

---

## WE 的游戏分层设计思路

### 酒馆的思路

ST/TH 的设计是**工具箱模式**：预设、正则、世界书、角色卡全部可以随时更换，用户自己组合。角色卡是必选的核心，其余都是可插拔的增强层。这适合高级用户，但对普通玩家门槛很高。

### WE 的分层

WE 把这个工具箱**在创作阶段打包**，玩家拿到的是一个完整的游戏，而不是一堆零件：

```
创作模块（Creation）
  ├── 游戏设计者工作区
  │   ├── 选择/编写预设（PresetEntry）
  │   ├── 配置正则规则（RegexProfile）
  │   ├── 编写世界书（WorldbookEntry）
  │   ├── 导入角色卡（CharacterCard）
  │   ├── 配置 LLM Profile 和工具
  │   └── 打包 → GameTemplate（含所有配置）
  │
  └── 导入 ST 预设/角色卡 → 解耦重制 → 打包为新游戏

游玩模块（Play）
  ├── 玩家只看到游戏，不接触配置层
  ├── 自由游玩（Session/Floor/Page）
  ├── 剪辑分享（边界归档 → MVM 降级渲染）
  └── 把游戏带回创作模块 → 解耦重制 → 再发布
```

**关键原则**：
- 游戏设计者在创作模块完成所有配置，打包进 `GameTemplate`
- 玩家的工作是游玩、剪辑、分享——不需要接触预设/正则/世界书
- 玩家可以把游戏"带回"创作模块，解耦各层（换预设、换角色卡、改世界书），重制后再发布或本地游玩
- ST 预设/角色卡是创作模块的**导入源**，不是游玩时的运行时配置

### 创作引擎 MCP 的定位

后续创作引擎 MCP 是**创作模块的 AI 协作层**，不是游玩引擎的扩展：

```
MCP Tool: import_st_preset      → 导入 ST 预设到 GameTemplate
MCP Tool: import_character_card → 导入角色卡 PNG
MCP Tool: edit_preset_entry     → 修改某条 PresetEntry 内容
MCP Tool: configure_worldbook   → 管理世界书词条
MCP Tool: package_game          → 把当前配置打包为可发布的 GameTemplate
MCP Tool: unpack_game           → 把 GameTemplate 解耦为可编辑的各层配置
```

`unpack_game` 是"把游戏带回创作模块"的关键操作——把 GameTemplate 的所有关联数据（PresetEntry、WorldbookEntry、RegexProfile、CharacterCard）展开为可独立编辑的状态，修改后再 `package_game` 打包。

这与现有 CRUD API 一一对应，MCP 层只是让 AI 代理可以协助创作者完成这些操作。
