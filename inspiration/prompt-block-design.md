# PromptBlock 设计 与 MVM 多媒体渲染展望

> 编写于 2026-04-04
> 本文档分两部分：
> 1. **PromptBlock IR** — 已实现，描述设计意图与扩展路径
> 2. **MVM 多媒体渲染** — 设计展望，尚未实现

---

## 一、PromptBlock：Prompt 的中间表示

### 1.1 为什么需要 IR

把"发给 LLM 的消息列表"直接组装是最简单的做法，很多框架就是这样做的。但随着内容来源增多（系统提示、记忆摘要、世界书、历史对话……），直接组装有两个问题：

1. **顺序难以控制**：哪段文字应该在最前面？哪段应该紧贴历史？规则是动态的。
2. **组装逻辑与来源逻辑耦合**：加一个新节点需要动已有节点的代码。

PromptBlock 是解决方案：每个节点只生产 Block，不关心排序；Runner 统一按 Priority 排序再合并为最终消息列表。

```
Pipeline 节点 ──生产──▶  []PromptBlock  ──排序──▶  []llm.Message
```

### 1.2 当前 PromptBlock 结构

```go
type PromptBlock struct {
    Type     BlockType  // system | preset | worldbook | memory | history | user
    Role     string     // "system" | "user" | "assistant"
    Content  string
    Priority int        // 越小越靠上（越靠近 System Prompt 顶部）
}
```

Priority 的直觉：

```
TemplateNode       Priority=1000   （兜底 System Prompt）
PresetEntry A      Priority=200    （由 injection_order 直接决定）
PresetEntry B      Priority=500
WorldbookEntry     Priority=10~510 （由 priority 字段决定）
MemoryNode         Priority=400
HistoryNode[最旧]  Priority=-N
HistoryNode[最新]  Priority=-1
```

Runner 按 Priority 升序排列，相同 Priority 按节点添加顺序。History 用负值保证永远在末尾。

### 1.3 Runner 的合并规则

排序后的 PromptBlock 按如下规则合并为 `[]llm.Message`：

- `Role == "system"` 的连续 Block 合并为单条 system message（LLM API 通常只允许一条 system）
- `Role == "user" / "assistant"` 的 Block 独立成条，按顺序排列
- History Block 的 Role 来自原始历史记录（user/assistant 交替）

### 1.4 扩展路径

PromptBlock 预留了 `Type` 字段。未来新增内容类型只需：
1. 定义新的 `BlockType` 常量
2. 写一个新的 Pipeline 节点返回对应类型的 Block
3. 若 Runner 合并逻辑需要特殊处理，在 `runner.go` 增加分支

不影响任何已有节点。

**预留扩展类型（未实现）：**

| Type | 用途 |
|------|------|
| `BlockTool` | 工具调用结果注入（Tools/MCP 层用） |
| `BlockDirector` | Director 角色槽的系统提示（多角色槽用） |
| `BlockVerifier` | Verifier 校验指令（多角色槽用） |
| `BlockCondition` | 条件渲染块（变量条件选项用） |

---

## 二、MVM：内容与渲染的分离

> 详细 API 设计见 [`mvm-rendering.md`](mvm-rendering.md)，本文从 PromptBlock 角度解释分层关系。

### 2.1 从 PromptBlock 到 MVM MSG

PromptBlock 是 **生产侧** 的 IR：它描述"LLM 需要看什么"。  
MVM MSG 是 **消费侧** 的 IR：它描述"前端需要渲染什么"。

```
PromptBlock[]  ──runner──▶  llm.Message[]  ──LLM──▶  raw_text
                                                           │
                                                      parser.Parse()
                                                           │
                                                      ParsedResponse   ← 这就是 MVM MSG
                                                     (narrative/options/state_patch/summary/vn)
```

`ParsedResponse` 已经是一个简化的 MVM MSG：它结构化了 AI 的输出，但只有 narrative 和 vn 两种 MODEL。

### 2.2 MVM 三层

```
MODEL  —— 内容的 Schema（这段内容是什么结构）
MSG    —— 内容的实例（LLM 或编辑器实际产出的数据）
VIEW   —— 渲染规则（在什么客户端、以什么形式展示 MSG）
```

**已实现的 MODEL：**

| MODEL | 对应代码 | 状态 |
|-------|---------|------|
| NarrativeBlock | `ParsedResponse.Narrative + Options` | ✅ |
| VNScene | `ParsedResponse.VN (VNDirectives)` | ✅ |

**待实现的 MODEL：**

| MODEL | 说明 |
|-------|------|
| AudioEvent | bgm/sfx/voice 控制 |
| StateNotice | 物品获得、HP 变化等状态通知 |
| ClipFrame | 楼层快照，用于剪辑分享 |
| ToolCall | 工具调用事件（Tools 层产出） |

### 2.3 VIEW 降级链

同一 MSG 在不同客户端应用不同渲染规则：

```
vn-full → narrative → minimal → pure-text
```

这个降级在 **前端** 完成，引擎只需在响应中携带 `parse_mode` 字段告诉前端当前 MSG 的类型，前端根据自身能力选择最高可用的 VIEW。

引擎侧不需要知道 VIEW 是什么——这是 MVM 分层的核心价值：引擎和渲染客户端之间只共享 MSG Schema，互不耦合。

### 2.4 扩展 PromptBlock → MVM 的时机

当前 PromptBlock → LLM → parser → MVM MSG 的链路已经运作。

引入新 MODEL（如 AudioEvent）的工作量：

1. **引擎侧**：在 `parser.go` 的 XML 解析中增加 `<audio>` 标签处理，产出 `AudioEvent` 结构
2. **引擎侧**：在 `ParsedResponse` 中增加 `Audio *AudioEvent` 字段
3. **引擎侧**：在 `TurnResponse` 中携带 `Audio` 字段
4. **前端侧**：在 `vn-full` VIEW 中实现播放逻辑

不需要修改 PromptBlock、Pipeline 节点或 Runner。

---

## 三、存档应不应该做到引擎层？

**结论：不需要单独的"存档系统"。**

### 3.1 引擎已经是一个隐式存档系统

每一次 `CommitTurn` 都持久化了：
- 该 Floor 的用户输入和 AI 回复（`MessagePage.Messages`）
- 该 Floor 的变量快照（`MessagePage.PageVars`）
- Session 级的变量（`GameSession.Variables`）
- 记忆摘要缓存（`GameSession.MemorySummary`）

这比传统游戏的手动存档更细粒度——**每一回合都是一个完整存档点**。

### 3.2 "存档"需求可以用现有原语满足

| 用户需求 | 实现方式 | 引擎是否已支持 |
|---------|---------|--------------|
| 保存进度 | 一直在自动保存 | ✅ 无需额外操作 |
| 回到某回合重玩 | 从指定 Floor 开始 Fork Session | 设计已有，待实现 |
| 命名存档（"第一章末"） | 给 Session 打 title | ✅ `PATCH /sessions/:id` |
| 存档分享给朋友 | Session 状态序列化为 JSON | 可作为轻量工具函数 |
| 多存档槽 | 同一 game_id 下多个 Session | ✅ 天然支持 |
| 读档（从某 Session 继续） | 直接用那个 session_id 发 turn | ✅ 无需额外操作 |

### 3.3 真正需要实现的是 Fork

**Session Fork** 才是目前缺失的存档相关能力：

```
POST /play/sessions/:id/fork
Body: { from_floor_seq: 5 }
→ 创建新 Session，复制 Floor 0..5 的历史和变量，然后可以从第 6 回合走新方向
```

这比"存档/读档"更强——它允许从任意回合分叉出平行时间线，同时保留原始会话。

Fork 的引擎实现：
1. 创建新 `GameSession`（相同 `game_id`，新 `id`）
2. 复制 `floors[0..N]` 的 `MessagePage`（只复制 `is_active = true` 的页）
3. 复制目标 Floor 的 `PageVars` 到新 Session 的 `Variables`
4. 复制原 Session 的 `MemorySummary` 作为初始摘要

Session Fork 复杂度适中，是下一个有价值实现的引擎原语。
