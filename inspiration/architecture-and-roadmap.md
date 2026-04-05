# 游戏工坊 V2 模块化后端架构设计与复刻计划

> **核心原则**：
> 1. **深度模块解耦**：将单体后端拆分为界限清晰的独立上下文（引擎层、创作层、社区层）。
> 2. **Token 成本极简主义 (One-Shot LLM)**：吸收 TavernHeadless(TH) 优秀的运行时抽象，但**坚决砍掉昂贵的多阶段生成**。每次用户交互必须且只能触发 **1 次** LLM 生成。任何 Director 校验、Memory 整理必须是异步的或被整合到 Prompt 规则中。

---

## 目录结构设计 (`backend-v2/`)

```text
backend-v2/
├── cmd/
│   ├── server/             # 主服务入口
│   └── worker/             # 异步任务处理器 (比如记忆摘要生成)
├── internal/
│   ├── engine/             # 模块 1: 核心游戏引擎后端 (TH 深度复刻)
│   │   ├── pipeline/       # 提示词流水线 (按步骤组装 Prompt)
│   │   ├── prompt_ir/      # 中间表示层 (Prompt Intermediate Representation)
│   │   ├── variable/       # 沙箱变量管理器 (Page -> Floor -> Session -> Global)
│   │   ├── memory/         # 记忆与上下文管理
│   │   └── api/            # 面向前端的游戏游玩接口 (/api/v2/play)
│   │
│   ├── creation/           # 模块 2: 模板创作后端
│   │   ├── template/       # 游戏模板 CRUD
│   │   ├── card/           # 角色卡解析与导入 (PNG / CCv3)
│   │   ├── lorebook/       # 世界书在线可视化编辑
│   │   ├── asset/          # CDN 静态素材管理
│   │   └── api/            # 面向创作者的接口 (/api/v2/create)
│   │
│   ├── social/             # 模块 3: 社区论坛后端
│   │   ├── post/           # 游玩记录分享
│   │   ├── comment/        # 评论系统
│   │   └── rank/           # 排行榜
│   │
│   ├── user/               # 模块 4: 用户与鉴权
│   └── core/               # 核心层: DB, Config, LLM Client 封装
```

---

## 引擎层 (Engine) 对 TavernHeadless 的复刻与魔改

TH 的设计非常超前，但在高频次游玩的平台里，**成本和延迟是致命伤**。我们保留它的架构骨架，但对其调用流进行“贫民化 / 效率化”改造。

### 1. 变量沙箱机制复刻 (无损平移)
**TH思路**：`Page -> Floor -> Branch -> Chat -> Global` 级联查询，写操作锁定在 Page。
**V2魔改**：完全照搬。
- **机制**：玩家点击选项触发 `Page`，生成时更新 `Page` 变量。如果不满意（重新生成），直接丢弃 `Page`。玩家进行下一步时，将 `Page` 变量 Commit 到 `Floor` 并提升为 `Chat`。
- **成本**：这是纯内存和 DB 状态机计算，**0 token 成本**。

### 2. 提示词流水线 (Prompt Pipeline & IR)
**TH思路**：Template -> Condition -> Worldbook -> Transform -> Memory -> TokenBudget -> Messages。
**V2魔改**：精简执行图，预编译静态部分。
- 只有在 `Worldbook`（关键词触发）和 `Memory`（检索上下文）时进行动态组装。
- 输出为 `Prompt IR`，最后扁平化为传给 OpenAI/Zhipu 兼容接口的格式。

### 3. 多实例调度 -> “One-Shot” LLM 调用限制
**TH思路**：Director(剧情指引) -> Memory检索 -> Narrator(生成) -> Verifier(格式校验) -> Memory异步更新。
**V2魔改 (核心性能保障)**：
- **同步链路只能有 1 个模型（Narrator）**。用户的每一次点击，只允许请求 1 次 LLM。剧情指引 (Director) 必须写死在 System Prompt 中；格式输出必须强依赖模型的 `<XML>` 跟随能力。
- **异步处理 (Memory)**：当一局游戏达到 10 轮交互后，触发一个挂在 `cmd/worker/` 下的异步 Job，用极其廉价的模型（如 `gpt-4o-mini` 或 `glm-4-flash`）去后台提取这 10 轮的摘要写进数据库。下一次用户请求时，直接读数据库的摘要词条（算在 Pipeline 的 Memory 节点里），不产生同步耗时。

### 4. 角色卡与世界书解析隔离
- 将针对 SillyTavern 复杂的 PNG 和 CCV3 格式提取，全部放到 `creation/card` 模块中。
- `engine` 层在游戏时，只读取已经结构化入库的 JSON，不处理任何二进制图片解析逻辑，大幅提升接口响应速度。

---

## 后续开发计划
1. **基础设施**：在 `backend-v2/core` 初始化路由、配置、数据库链接库 (GORM) 和 LLM 基础客户端。
2. **第一战役 (Prompt Pipeline)**：在 `internal/engine/pipeline` 实现基于节点的 Prompt IR 引擎，确保纯 Go 代码不依赖任何外部接口即可组装出高质量的上下文。
3. **第二战役 (变量沙箱)**：实现 Page -> Floor -> Chat 的提升逻辑。
4. **前端重构对齐**：配合新分离的接口，重新搭建解耦的前端架构。