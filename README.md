# WorkshopEngine

一个以 **LLM 对话为核心的无头交互运行时**（Headless AI Interaction Runtime）。

WorkshopEngine 不绑定任何前端框架，也不限定使用场景。它提供一套通用的 **会话管理 → Prompt 编排 → LLM 调用 → 状态持久化** 管道，驱动任何需要"与 AI 来回推进"的应用：互动叙事游戏、对话式工具、Vibe Coding 助手、项目进度可视化……都是同一条主链路的不同配置。

---

## 核心能力

### 会话结构

```
Session（一次游戏/交互）
  └── Floor（一个回合）
        └── Page（同一回合的多个生成结果，可 Swipe 切换）
```

- 每个 Floor 包含一条用户输入和一条 AI 回复
- 同一 Floor 支持多个 Page（重新生成/Swipe），玩家可以选择最满意的版本
- 切换激活 Page 时，该 Page 的变量快照自动提升到 Session 级别

### 变量沙箱（五层）

```
Page → Floor → Branch → Chat → Global
```

AI 回复中的 `<state_patch>` 写入 Page 沙箱；Floor 提交后提升至 Chat；创作者可在任意层写入全局变量。所有层级的变量在调用 LLM 前合并展开，用于 Prompt 宏替换。

### Prompt Pipeline（可编排）

将"最终要发给 LLM 的消息列表"拆解为独立的 `PromptBlock`，通过流水线节点按优先级装配：

| 节点 | 类型 | 优先级 |
|------|------|--------|
| PresetNode | 条目化 Prompt（可排序） | 由 `injection_order` 直接决定 |
| TemplateNode | 单字符串 System Prompt | 1000（兜底） |
| WorldbookNode | 关键词/正则触发注入 | 10–510 |
| MemoryNode | 长期记忆摘要注入 | 400 |
| HistoryNode | 近期历史消息 | 0 ~ -N |

优先级数值越小越靠前（越靠近 System Prompt 顶部）。节点可单独开关，便于调试。

### 记忆系统

- **实时摘要**：每轮 AI 回复的 `<summary>` 字段异步写入 Memory 表
- **时间衰减**：指数半衰期排序，旧记忆自然降权（可配置 `HalfLifeDays`）
- **整合 Worker**：每 N 回合用廉价模型重新整合记忆，更新 Session 摘要缓存
- **维护策略**：独立定时器，自动废弃/物理删除过期记忆

### LLM 调用

- **One-Shot**（主链路）和 **SSE 流式** 两条路径
- 支持任何 OpenAI 兼容 API（GLM、本地模型、OpenRouter 等）
- 前端可携带自己的 API Key（BYO Key 模式）
- 采样参数（temperature/topP/topK/penalties/reasoning_effort）全部可配置，支持 per-request 覆盖

### LLM 响应解析

三层回退，无需强迫 AI 格式化输出：

```
XML 严格模式 → JSON 模式 → 纯文本降级
```

解析结果结构化为 `narrative / options / state_patch / summary / vn_directives`，供前端按场景选择渲染方式。

### 创作工具

- 世界书（Worldbook）：关键词/正则触发的背景知识注入
- Preset Entry：条目化 Prompt，可排序、可开关、可携带独立角色
- 角色卡导入：兼容 SillyTavern `chara_card_v2/v3` PNG 格式
- 素材管理：图片/音频上传（MIME 白名单 + 10MB 限制）
- Prompt Dry-Run：组装完整 Prompt 但不调用 LLM，供创作者调试

---

## 架构

```
cmd/
  server/main.go        HTTP 服务入口
  worker/main.go        异步记忆维护 Worker

internal/
  core/                 基础设施（config / db / llm）
  engine/               交互运行时核心
    api/                HTTP 路由（/play/*）
    pipeline/           Prompt 流水线节点
    prompt_ir/          PromptBlock 中间表示
    variable/           五层变量沙箱
    memory/             记忆存储 + Worker
    session/            Floor/Page 生命周期
    parser/             LLM 响应解析
  creation/             创作工具（/create/*）
  platform/             Provider 注册表 / 鉴权
```

---

## 快速启动

```bash
# 环境变量（最少配置）
export LLM_API_KEY=your_key
export DATABASE_URL=host=localhost user=postgres password=postgres dbname=workshop sslmode=disable

# 启动主服务
go run ./cmd/server

# 启动记忆 Worker（可选，独立进程）
go run ./cmd/worker
```

完整环境变量列表见 [`docs/engine-audit.md`](docs/engine-audit.md)。

---

## 主要 API

```
POST   /api/v2/play/sessions              创建会话
POST   /api/v2/play/sessions/:id/turn     推进一回合（One-Shot）
GET    /api/v2/play/sessions/:id/stream   推进一回合（SSE 流式）
GET    /api/v2/play/sessions/:id/floors   楼层历史
GET    /api/v2/play/sessions/:id/memories 记忆列表
GET    /api/v2/play/sessions/:id/prompt-preview  Prompt dry-run

POST   /api/v2/create/templates           创建游戏模板
POST   /api/v2/create/templates/:id/preset-entries  Preset Entry 管理
POST   /api/v2/create/worldbook-entries   世界书词条
```

---

## 待实现

| 功能 | 说明 |
|------|------|
| Tools / Function Calling | 让 LLM 在对话中调用工具（读文件、执行代码、查数据库……） |
| MCP 协议接入 | 标准化工具调用接口，复用社区 MCP 服务 |
| 多 LLM 角色槽 | `director`（剧情控制）、`verifier`（输出校验）与 `narrator` 并行 |
| 多 Provider 注册表 | Anthropic / Google / xAI 原生适配（非 OpenAI 兼容路径） |
| Session Fork | 从指定楼层分叉出新会话（"如果当时选了另一个选项"） |
| 多媒体渲染层（MVM） | 游戏内容与前端渲染解耦，支持剪辑分享、论坛嵌入 |

设计思路见 [`docs/mvm-rendering.md`](docs/mvm-rendering.md) 和 [`docs/prompt-block-design.md`](docs/prompt-block-design.md)。

---

## 许可

见 [LICENSE](LICENSE)。
