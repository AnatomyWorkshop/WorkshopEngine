# Karpathy LLM Wiki 模式 — WE 引擎对照分析与设计方向

原文：https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f

---

## Karpathy 的核心主张

RAG 是"每次查询重新推导"，没有积累。LLM Wiki 的不同之处在于：

> LLM 增量维护一个持久化 Markdown wiki，每次摄入新源时主动整合、更新交叉引用、标记矛盾。知识被编译一次，持续保鲜，而不是每次查询时重新合成。

三层架构：**Raw sources（不可变）→ Wiki（LLM 维护）→ Schema（配置 LLM 行为）**

三类操作：**Ingest（摄入）/ Query（查询，答案可归档）/ Lint（健康检查）**

---

## ST / TH 的实际做法

ST 的 Memory 扩展（`extensions/memory/index.js`）代表了主流 AI 聊天工具的标准思路：

**滚动摘要压缩（Rolling Summary Compression）**

1. 每隔 N 条消息触发一次摘要（`promptInterval`）
2. 取"上一条摘要 + 此后新消息"作为输入，调用廉价模型生成新摘要
3. 摘要以 `mes.extra.memory` 字段附挂在某条历史消息上（不新建消息，不污染对话流）
4. 每次构建 prompt 时，通过 `setExtensionPrompt` 将最新摘要注入到指定位置（`position` + `depth`）
5. 摘要之前的原始消息**不删除**，只是不再送入 LLM 上下文窗口

**关键设计原则**：
- 摘要是**压缩**，不是归档；原始消息保留在本地，可回溯
- 摘要挂在消息的 `extra` 字段，对话流本身不变
- token 预算严格控制：`PROMPT_SIZE - PADDING`，超出就停止追加消息
- 触发是**被动的**（每 N 轮），不是主动的（不分析内容重要性）

WE 的 `triggerMemoryConsolidation` 完全复刻了这个模式。

---

## WE 的设计约束

WE 引擎的目标是**手机端轻量游玩**，与论坛、agent 一起在移动端互动。这决定了：

- **每回合 token 必须可控**：不能因为记忆系统膨胀而让每次请求变贵
- **游戏过程仍然用摘要压缩**：ST/TH 的滚动摘要是正确的，继续沿用
- **归档发生在边界时刻**：结局、输出、分享决定——这些是玩家主动选择的节点，不是每回合自动触发

---

## Karpathy 模式的借鉴点：边界归档

Karpathy 最有价值的洞察不是"每次都更新 wiki"，而是：

> **好的答案/发现不应该消失在对话历史里，应该归档回知识库。**

对应到 WE 的游戏语境：

| 游戏事件 | 对应 Karpathy 操作 | WE 应做的事 |
|---|---|---|
| 玩家选择结局 | Ingest（重要源） | 将本局摘要 + 关键变量快照归档为一条持久 Memory（`type=summary`, `importance` 高） |
| 玩家导出/分享存档 | Query 结果归档 | 生成一份结构化的"游戏记录"，可供 agent 读取、论坛展示 |
| 玩家手动标记某段剧情 | 用户主动 Ingest | 将该楼层内容提升为高重要性 Memory 条目 |
| 定期 Lint | 健康检查 | 标记互相矛盾的 fact 条目（`Deprecated=true`），清理过时变量快照 |

**游戏过程中**：继续用滚动摘要压缩，token 轻量，不改变现有机制。  
**边界时刻**：触发一次性的结构化归档，写入持久 Memory 或导出为可读文档。

---

## MVM 渲染器的角色

MVM（Model-View-Message）的核心思想是**唯一信息源，人机双渲染**：

- 同一份数据，LLM 可以读（结构化 JSON / Markdown），人也可以读（渲染后的 UI）
- 归档后的内容可以**自由剪辑**：删掉某段、合并两段、调整顺序
- 可以**降级渲染**：完整 VN 场景 → 纯文本摘要 → 单行标题，同一份数据适配不同展示场景（手机通知、论坛帖子、agent 上下文）

这意味着归档格式应该是**结构化 Markdown + 可选 frontmatter**，而不是纯文本 blob：

```markdown
---
type: session_summary
game_id: xxx
floor_count: 42
ended_at: 2026-04-05
key_vars: {affection: 85, route: "true_end"}
---

## 剧情摘要
爱丽丝最终选择了...

## 关键决策
- 第12回合：选择了"告诉她真相"
- 第31回合：放弃了逃跑机会

## 开放悬念
- 父亲的身份仍未揭晓
```

这份文档 LLM 可以直接读（作为下一局的世界书条目或 agent 上下文），人可以在论坛展示，也可以降级为一行"爱丽丝线 True End，好感度 85"。

---

## WE 现有实现对照

| 功能 | 现状 | 与 Karpathy/MVM 的关系 |
|---|---|---|
| 滚动摘要压缩 | ✅ `triggerMemoryConsolidation` | 正确，继续沿用 |
| 摘要注入 prompt | ✅ `MemorySummary` 字段 + Pipeline | 正确 |
| 边界归档 | ❌ 无 | 需要新增：结局/分享时触发 |
| 结构化 Memory 格式 | ❌ 纯文本 | 可选改进：frontmatter + 分节 Markdown |
| Memory Lint | ❌ 无 | 低优先级，可用廉价模型定期跑 |
| 跨会话知识库 | ❌ session 级 | 长期方向，归档后的 Memory 可提升为 game 级 |
| 降级渲染 | ❌ 无 | 前端工作，归档格式确定后实现 |

---

## 优先级

1. **边界归档 API**（中期）：新增 `POST /sessions/:id/archive`，在结局/分享时调用，生成结构化 Markdown 摘要，写入高重要性 Memory 条目，可选导出为文件
2. **归档格式标准化**（配合上条）：Memory 内容支持 frontmatter，`type=archive` 时按结构解析
3. **Memory Lint**（低优先级）：复用 `memoryTriggerRounds` 机制，每 2N 轮用廉价模型扫描矛盾条目
4. **跨会话知识库**（长期）：game 级 Memory 表，归档内容可提升为全局世界书条目

---

## WE 引擎能否实现 Karpathy 设想的知识库功能？

### 场景描述

创作者选择"知识库"方向：玩家在平台上的各类学术交流作为 Memory 持续摄入，时刻更新存档，可以自由剪辑分享。

### 直接用 WE 引擎实现

WE 的现有原语已经覆盖了这个场景的核心需求，只需要换一套 GameTemplate 配置：

**对应关系：**

| Karpathy 概念 | WE 原语 | 说明 |
|---|---|---|
| Raw sources（不可变原始文档）| `Material` 表 | 创作者预置的学术资料、论文摘要等 |
| Wiki（LLM 维护的结构化知识）| `Memory` 表 + `WorldbookEntry` | Memory 存增量摘要，WorldbookEntry 存稳定的实体/概念页 |
| Schema（配置 LLM 行为）| `GameTemplate.SystemPromptTemplate` + `PresetEntry` | 告诉 LLM 如何摄入新内容、如何更新知识 |
| Ingest（摄入新源）| 用户发一条消息 = 一次 PlayTurn | 每回合就是一次摄入，LLM 读取并整合 |
| Query（查询）| 同上，PlayTurn 的另一种用法 | 用户提问，LLM 检索 Memory + WorldbookEntry 回答 |
| 答案归档 | 边界归档 API（待实现）| 重要发现写回 Memory，可选提升为 WorldbookEntry |
| Lint | Memory Lint（待实现）| 定期扫描矛盾/过时条目 |
| 剪辑分享 | MVM 降级渲染 + 边界归档 | 归档为结构化 Markdown，前端自由剪辑 |

**具体配置方式：**

```
GameTemplate {
  SystemPromptTemplate: "你是一个知识库维护助手。
    当用户分享新内容时，提取关键知识点并整合进已有知识。
    当用户提问时，检索相关记忆回答，并将重要结论标记为 [ARCHIVE]。
    {{memory_summary}}"

  Config: {
    enabled_tools: ["search_memory", "search_material"],
    memory_label: "知识库摘要",
    fallback_options: ["继续探讨", "换个话题", "整理笔记"]
  }
}
```

用户每次发言（分享论文、提问、讨论）= 一次 PlayTurn。  
`triggerMemoryConsolidation` 每 N 轮自动压缩摘要。  
边界归档 API 在用户主动"保存笔记"时触发，生成结构化 Markdown。

**论坛交流作为 Memory 的实现路径：**

论坛帖子/回复不需要经过 PlayTurn，可以直接调用 `POST /sessions/:id/memories` 手动写入 Memory 条目（该 API 已实现）。这样论坛的学术讨论可以异步摄入，不阻塞游玩流程。

### 从引擎拆出独立知识库模块

如果不需要游戏外壳，只要知识库功能，WE 引擎可以拆出以下最小子集：

```
需要的包：
  internal/core/db          → Memory 表、GameSession 表
  internal/core/llm          → LLM 客户端
  internal/engine/memory     → Memory Store（摄入、检索、整合）
  internal/engine/session    → Session 管理（简化版，只需 GetHistory）

不需要的包：
  internal/engine/parser     → VN/选项解析（知识库不需要）
  internal/engine/pipeline   → Prompt 组装（可简化为直接拼接）
  internal/engine/tools      → 工具调用（可选保留 search_memory）
  internal/engine/scheduled  → 定时触发（可选）
  internal/creation          → 创作层（知识库不需要游戏模板）
```

最小知识库服务只需要：
1. `Memory.Store` — 存取记忆条目
2. `llm.Client` — 调用 LLM
3. 一个简单的 HTTP handler：接收文本 → 调 LLM 整合 → 写入 Memory → 返回摘要

约 200 行 Go 代码，不依赖 Pipeline/Parser/Session 的复杂逻辑。

### 两种路径的选择

| 路径 | 适合场景 | 工作量 |
|---|---|---|
| 直接用 WE 引擎，换 GameTemplate 配置 | 知识库需要游戏化交互（选项、VN、角色扮演）、论坛集成、手机端 | 几乎零开发，配置即可 |
| 从引擎拆出独立知识库模块 | 纯知识库，不需要游戏外壳，要嵌入其他系统 | ~200 行，1-2 天 |

**推荐**：先用 WE 引擎直接跑，GameTemplate 配置成知识库模式，验证效果。如果后续需要独立部署或嵌入其他系统，再拆出最小子集。WE 的 Memory 系统和 LLM 客户端已经是干净的独立包，拆出成本很低。
