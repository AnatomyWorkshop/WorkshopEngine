# 创作层设计

> 状态：2026-04-06

---

## 定位

创作层是游戏设计师的工作台，负责从零完成一个游戏的全部配置并打包发布。

```
创作层（PC 软件）  →  internal/creation/   →  /api/v2/create
游玩层（手机 App） →  internal/engine/     →  /api/v2/play
```

两层唯一共享点是 `internal/core/db`（数据库模型），不互相导入。

---

## WE vs TH 能力对比

### 已对齐

| 能力 | TH | WE |
|---|---|---|
| Floor/Page 状态机 | ✅ draft→generating→committed/failed | ✅ 相同 |
| 五级变量沙箱 | ✅ global/chat/branch/floor/page | ✅ 相同 |
| Prompt Pipeline + IR | ✅ native + compat 双路径 | ✅ 单路径（PresetNode/WorldbookNode/MemoryNode/HistoryNode/TemplateNode） |
| 世界书触发 | ✅ primary/secondary keys, scan_depth | ✅ 相同 |
| 滚动摘要压缩 | ✅ LLM 提取 + Memory 整合 | ✅ triggerMemoryConsolidation |
| RegexRule 后处理 | ✅ ai_output/user_input/all | ✅ 相同 |
| LLM Profile + slot 优先级 | ✅ session>global, slot>* | ✅ 相同 |
| Director 槽 | ✅ | ✅ |
| Preset Tool（HTTP 回调） | ✅ preset-defined tools | ✅ HttpCallTool |
| 工具执行审计 | ✅ tool_execution_record | ✅ 相同 |
| SSE 流式输出 | ✅ | ✅ |

### WE 尚未实现（TH 已有）

| 能力 | TH | WE 现状 | 优先级 |
|---|---|---|---|
| Verifier 槽 | ✅ 后置校验+重试 | ❌ | 高（Phase 2） |
| MCP 协议接入 | ✅ stdio + HTTP transport | ❌ | 中（Phase 3） |
| prompt_snapshot | ✅ 每回合冻结 prompt 版本 | ❌ | 中 |
| memory_edge | ✅ 记忆关系图（supports/contradicts） | ❌ | 低 |
| Background Job Runtime | ✅ lease/retry/dead-letter | ❌（goroutine 直接异步） | 低 |
| 角色卡版本控制 | ✅ character_version | ❌ | 低 |
| Resource Tool Provider | ✅ LLM 可读写角色/世界书/预设 | ❌ | 中 |

### WE 独有（TH 没有）

| 能力 | 说明 |
|---|---|
| GameTemplate 打包/解包 | 单文件 JSON 包，含所有关联数据 |
| Material 素材库 | 按标签检索的内容池，search_material 工具 |
| 游戏发布状态 | draft/published，手机端只看 published |
| 创作/游玩层解耦部署 | 可拆分为独立进程 |

---

## 游戏类型：无限定义

WE 的 `GameTemplate.type` 字段当前枚举为 `visual_novel | narrative | simulator`，但这只是 UI 分组提示，**不影响引擎行为**。引擎本身对游戏类型无感知——它只处理 PresetEntry、WorldbookEntry、Tool、Memory 的组合。

任何游戏类型都可以通过配置实现：

| 游戏类型 | 实现方式 |
|---|---|
| 视觉小说 | RegexRule 提取选项，Material 存场景文本，PresetTool 管理场景状态 |
| 文字 RPG | WorldbookEntry 构建世界，变量追踪属性/状态，PresetTool 处理战斗/技能逻辑 |
| 多角色模拟 | 多个 is_system_prompt=true 的 PresetEntry 分别定义角色，Director 槽协调叙事 |
| 知识库问答 | Material 存原始资料，WorldbookEntry 存结构化知识，Memory 追踪对话上下文 |
| 互动剧本 | PresetEntry 定义叙事规则，set_variable 追踪剧情分支，边界归档记录结局 |
| 纯游戏逻辑 | PresetTool 处理所有状态变更，LLM 只负责叙事渲染，甚至可以绕过 LLM |

**渲染不必走 LLM**：PresetTool 可以返回任意 JSON，前端可以直接解析游戏状态并渲染，LLM 只在需要叙事生成时介入。这和 TH 的 TavernHelper 脚本模式一致。

`GameTemplate.type` 应改为自由文本字段，让创作者自定义游戏类型标签。

---

## 创作范式对照

WE 的创作层不是"写角色卡"，而是"设计一个可运行的叙事系统"。

| WE 概念 | ST 生态 | LLM Wiki | 多角色模拟 | 本质 |
|---|---|---|---|---|
| `GameTemplate` | 预设 + 角色卡组合 | 知识库配置 | 世界设定文件 | 游戏运行时配置 |
| `PresetEntry` | Prompt 条目 | Schema 定义 | 角色行为规则 | 分段注入的 system prompt |
| `WorldbookEntry` | 世界书词条 | Wiki 词条 | 角色/地点/事件档案 | 关键词触发的背景知识 |
| `Material` | 无直接对应 | 原始资料库 | 台词库/场景库 | LLM 按需检索的内容池 |
| `RegexRule` | 正则脚本 | 输出格式化 | 对话格式控制 | 输入/输出后处理 |
| `PresetTool` | TavernHelper 脚本 | 外部知识接口 | 状态机回调 | HTTP 回调工具 |
| `Memory` | 摘要压缩 | 增量 Wiki 更新 | 角色状态追踪 | 滚动压缩的对话记忆 |
| `Floor` | 楼层/回合 | 推理步骤 | 场景帧 | 不可改的已提交回合 |

角色卡（PNG 导入）是兼容现有生态的入口，不是创作的必须起点。

---

## 创作流程（从零开始）

```
1. 创建游戏模板
   POST /create/templates
   → 定义游戏类型（自由文本）
   → 配置 enabled_tools、director_prompt 等运行时参数

2. 设计叙事规则（PresetEntry）
   POST /create/templates/:id/preset-entries
   → 1–9：全局规则（最高优先级）
   → 10–509：叙事风格、格式要求、行为约束
   → 510–989：动态内容槽（变量驱动切换）
   → 990–1009：主角色人设（is_system_prompt=true）
   → 1010+：底部附加指令

3. 构建知识库（WorldbookEntry）
   POST /create/templates/:id/lorebook
   → constant=true：无条件常驻的世界观/规则
   → 关键词触发：人物档案、地点描述、事件背景

4. 准备内容池（Material）
   POST /create/templates/:id/materials
   → 台词片段、氛围描写、场景文本
   → 打标签（mood/style/function_tag）

5. 配置输出处理（RegexRule）
   POST /create/templates/:id/regex-profiles
   → ai_output：格式化输出（提取选项、清理标签）
   → user_input：预处理玩家输入

6. 注册游戏逻辑（PresetTool）
   POST /create/templates/:id/tools
   → HTTP 回调处理状态机逻辑
   → 可完全绕过 LLM 处理纯游戏逻辑

7. 绑定模型
   POST /create/llm-profiles/:id/activate
   → narrator 槽：主叙述模型
   → director 槽：廉价预分析（可选）

8. 打包发布
   PATCH /create/templates/:id { "status": "published" }
   GET   /create/templates/:id/export  → 单文件 JSON 包
```

---

## 创作 Agent（AI 辅助创作）

创作 Agent 是一个特殊配置的 GameTemplate，用于 AI 辅助完成创作流程。Agent 通过 PresetTool 调用 Creation API，不需要理解引擎内部实现。

**Agent 工具（注册为 PresetTool）**：

| 工具名 | 调用的 API | 用途 |
|---|---|---|
| `create_preset_entry` | `POST /create/templates/:id/preset-entries` | 写入叙事规则段落 |
| `update_preset_entry` | `PATCH /create/templates/:id/preset-entries/:eid` | 修改已有段落 |
| `add_worldbook_entry` | `POST /create/templates/:id/lorebook` | 写入知识库词条 |
| `add_material` | `POST /create/templates/:id/materials` | 写入内容池 |

**Prompt 结构**：

```
injection_order 10  → 创作原则（常驻）
injection_order 20  → 当前任务模板（用 set_variable 切换）
injection_order 30  → 输出格式要求（工具调用格式）
injection_order 990 → 游戏草稿状态（is_system_prompt=true）
```

---

## 接下来的工作

### 创作层（当前优先）

**1. GameTemplate.type 改为自由文本**
去掉枚举约束，让创作者自定义游戏类型标签。数据库层只需要去掉 CHECK 约束。

**2. Resource Tool Provider**
让 LLM 在生成过程中可以读写创作资源（WorldbookEntry、PresetEntry、Material）。
TH 已有完整实现可借鉴。这是创作 Agent 能自主操作数据库的关键。

**3. 创作 Agent 标准模板**
提供一套开箱即用的创作助手 GameTemplate（纯数据配置）：
- PresetEntry 组（创作原则、任务切换、格式要求）
- WorldbookEntry 组（叙事结构模板、人物设计规范）
- `enabled_tools: ["set_variable", "get_variable", "resource:*"]`

**4. 角色卡 PATCH 端点**
```
PATCH /create/cards/:slug
```

### 游玩层（稍后）

- **Verifier 槽**：约 40 行
- **边界归档 API**：`POST /sessions/:id/archive`
