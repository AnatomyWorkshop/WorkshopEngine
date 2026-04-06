# 两个问题：LLM Wiki 游戏化 + Galgame 预测选项

> 日期：2026-04-06

---

## Q1：LLM Wiki 在 GW 里的游戏化知识库体验

### 问题还原

> 在 GW 里找到一个游戏，内容是 LLM Wiki，点进去是我的知识库，存档在我这里，我提供和交流的知识不仅可以在 LLM Wiki 里渲染成我看到的，还能记忆对吗？
> 如果我想阅读世界书，可以在 GW 里解包 LLM Wiki，换其它预设、模板等然后独自属于我的 LLM Wiki 吗？
> 假如 LLM Wiki 有可选择的渲染方式，但我就喜欢看 md，我应该传回 CW 里解包还是可以在 GW 里解包修改去掉多余功能，还是 LLM Wiki 可以做开发者模式，GW 里可以随时点进去修改？

### 回答

#### 知识可以同时渲染 + 记忆 ✅

对。这是 WE 引擎已经支持的能力：

- 每次 `PlayTurn`（你发言 = 一次摄入或提问）之后，LLM 输出的内容在前端渲染给你看
- 同时 `triggerMemoryConsolidation` 在后台把这轮对话整合成结构化 Memory（`facts_add/update`），写入你的 Session
- 下次你开启这个知识库会话，你之前学过的东西已经在 Memory 里，LLM 会带着这些记忆跟你交流

这正是 karpathy 分析里说的"Ingest + Query 合并在同一次交互"——你说话既是输入也是触发更新。

#### 存档归属 ✅

Session 是按 `session_id` 隔离的，存档（Floor/Memory/Variable）全部挂在你的 session 下，而不是公共游戏状态。所以：

- 同一个 LLM Wiki 游戏，你和另一个用户的知识库是完全独立的
- 你的存档只能你自己访问（在当前无 JWT Auth 的情况下靠 session_id 隔离，Auth 上线后靠账号隔离）

#### 解包 + 定制：三条路径

你想把公共的 LLM Wiki "变成自己的"，有三条路径，复杂度和控制权依次递增：

---

**路径 1：Session 内变量覆盖（无需解包，最轻量）**

如果 LLM Wiki 的预设设计了渲染模式变量（如 `render_mode: narrative|minimal|markdown`），你可以直接通过 API `PATCH /sessions/:id/variables` 修改这个变量，后续所有输出都会按 markdown 渲染。

这适合："我只想改渲染风格，其他都不想动。"

缺点：只有创作者在游戏设计里暴露了这个变量，你才能改。

---

**路径 2：游戏包解包 → 在 CW（创作工坊）里修改 → 重新打包**

操作流程：
```
GW 里找到 LLM Wiki
  → GET /api/v2/create/templates/:id/export  下载 game-package.json
  → 在 CW 里导入 POST /api/v2/create/templates/import
  → 修改模板：删掉不需要的预设条目、改系统 Prompt、固定渲染模式
  → 发布为你自己的私有版本
  → 在 GW 里用这个新版本开始新会话
```

这适合："我想深度定制，把 LLM Wiki 的叙事渲染模块全删掉，只保留 Markdown 输出的知识库模式。"

缺点：创建了一个全新游戏副本，原来那个会话的存档不能无缝迁移（需要手动把旧 Memory 导入新 Session，或者用 `ForkSession` 过渡）。

---

**路径 3：GW 内置开发者模式（待规划功能）**

在 GW 里直接点进"开发者视图"，可以编辑当前游戏会话绑定的模板配置：

- 修改 PresetEntry（开关某个系统 Prompt 块）
- 切换 LLMProfile（换模型或 API Key）
- 修改模板 Config（改渲染模式、关掉 VN Directive 解析）
- 不退出游戏、不新建会话，修改立即生效

这最接近你说的"随时点进去修改"。

**当前状态**：后端 API 已经支持这一切（PATCH /templates、PATCH /preset-entries 都有），只是 GW 前端没有开发者界面。  
**要做什么**：GW 增加一个"实验性/开发者"侧边栏，嵌入创作层的精简 UI。

这是三条路径里最理想的体验，但开发成本最高（需要前端创作 UI）。

---

#### 推荐方案

| 你的需求 | 推荐路径 | 工作量 |
|---|---|---|
| 只想改渲染风格 | 路径 1：Session 变量覆盖 | 零开发，API 调用即可 |
| 想深度精简 LLM Wiki | 路径 2：解包 → CW 修改 → 重打包 | 中等，手动操作 |
| 想在 GW 里随时改 | 路径 3：GW 开发者模式 | 较高，需要前端工作 |

对于你"喜欢看 md"的具体需求：最轻量的做法是**在 LLM Wiki 的 SystemPrompt 里固定输出格式为 Markdown**（这是创作者配置层面的决定），然后在 GW 的 ChatMessage 组件里做 Markdown 渲染（`marked.js` 几行代码）。不需要解包，不需要开发者模式。

---

## Q2：Galgame 零延迟选项 — 预测预渲染

### 问题还原

> GW 里有一个精美的 Galgame，每次非选择的游玩阶段，设计三种选择和保留用户输入。预测的三种选择在上一个阶段已经全部渲染好，如果用户不主动输入，三选一可以直接玩，并且选择后预测下一个选择和提前渲染，这样如果用户从不主动输入，游戏就会像制成一样不需要响应时间？

### 回答

这完全可以实现，而且 WE 引擎已经具备所有底层能力，缺的只是触发逻辑和前端联动。这个模式我们叫它 **Preflight Rendering（预飞渲染）**。

#### 核心思路

```
[当前回合完成]
      │
      ├─► 正常渲染给用户看（已有）
      │
      └─► 同时触发"预飞 Job"：
            用同一个 Prompt 上下文，
            多调用一次廉价模型（Director 槽），
            让它预测接下来最可能的 3 个选项，
            把结果缓存在 Session 变量里
                  │
                  └─► 用户在读剧情时，选项已经就绪
                  └─► 用户点选项 → PlayTurn 携带预选选项
                  └─► 同时触发下一轮预飞 Job（循环）
```

用户永不需要等待，只要阅读速度 > LLM 生成速度（几乎总是成立）。

---

#### 技术实现路径

**Step 1 — 预飞 Job（后端）**

在 `PlayTurn` 完成后，异步触发一个 goroutine：

```go
// 主回合完成后
go engine.preflight(ctx, sessionID, floorSeq)

func (e *GameEngine) preflight(ctx context.Context, sessionID string, floorSeq int) {
    // 1. 用 Director 槽的廉价模型（已有）
    // 2. 系统 Prompt 末尾加一条：
    //    "基于当前剧情，预测玩家最可能做出的3个选择，以JSON数组返回：
    //     [{\"label\":\"...\",\"subtext\":\"...\"}]"
    // 3. 把结果写入 Session 变量：predicted_choices
    e.varStore.SetSessionVar(sessionID, "predicted_choices", result)
}
```

这不阻塞主流程，Director 槽就是为轻量预分析存在的。

**Step 2 — 前端读取预测选项**

`GET /sessions/:id/variables` 里直接读 `predicted_choices`，前端展示在输入框上方：

```
┌────────────────────────────────┐
│ [A] 告诉她真相                  │  ← predicted_choices[0]
│ [B] 选择沉默                    │  ← predicted_choices[1]
│ [C] 转移话题                    │  ← predicted_choices[2]
├────────────────────────────────┤
│ 或者，输入你自己的选择...  [发送]│  ← 保留自由输入
└────────────────────────────────┘
```

**Step 3 — 用户选择后的流程**

用户点 [A]：
1. 前端发 `POST /sessions/:id/turn` 携带 `user_input: "告诉她真相"`
2. 同时前端把选项 UI 换成"加载中"（感知上是瞬间的，因为服务器已经收到输入）
3. 主回合 LLM 正常执行
4. 主回合完成后，再次触发预飞 Job 缓存下一轮选项

如果用户在预飞完成前已经点击选项（极罕见：用户速读），降级为普通等待——体验和现在完全一样。

---

#### 更进一步：真正的"制成游戏感"

如果想要完全的零等待体验（用户点选项后连主回合也是瞬间的），需要更激进的方案：

**完整预生成（Eager Execution）**

预飞 Job 不只预测选项，而是对每个选项**完整跑一次主回合**（用同样的 Prompt + 不同的 user_input），把三条分支的完整响应都缓存起来。

```
用户看剧情时，后台已经生成了三条完整的"如果用户选A/B/C"的 LLM 响应。
用户点 [A]，直接从缓存取出分支 A 的结果，无需等待任何 LLM 调用。
```

**代价**：每回合调用 LLM 次数从 1 变成 1+3=4。成本 4x，但如果用便宜的模型（如 DeepSeek-V3），每回合成本仍然可控。

**前提**：游戏设计者确认"这个游戏用预生成模式"，开关在 `GameTemplate.Config.preflight_mode: lazy|eager`。

- `lazy`（默认）：只预测选项文本，主回合仍需等待（延迟降至 < 0.3s，用户无感知）
- `eager`：预生成完整响应，点击后真正零等待（成本 4x）

---

#### 和 WE 现有架构的契合度

| 需要什么 | WE 已有 | 需要新增 |
|---|---|---|
| 廉价模型做预测 | ✅ Director 槽 | 只需换一个 system prompt |
| 异步后台任务 | ✅ goroutine + 内存 lease | 预飞 Job 直接用这套 |
| 缓存预测结果 | ✅ Session 变量系统 | `predicted_choices` 变量 |
| 前端展示预测选项 | ❌ 需要前端新增 UI | ChatInput 上方的选项行 |
| 用户点选项触发回合 | ✅ PlayTurn 已有 | 无需改动后端接口 |
| eager 预生成缓存 | ❌ 需要新增缓存字段 | `predicted_pages: [{choice, content}]` 存在 Session 变量或单独字段 |

**最小可用版本**（lazy 模式）后端改动：~80 行 Go（一个异步预飞函数 + 变量写入）。  
**完整版本**（eager 模式）后端改动：~150 行，加上分支结果缓存逻辑。  
**前端改动**：ChatInput 组件上方增加选项行，读取 `predicted_choices` 变量展示。

---

#### 这和"制成游戏"感的差异

完全制成的 Galgame（如 Clannad、命运石之门）是完全预写好的，每个分支都是固定文本。

WE 的 Preflight 模式创造的是：

> **感官上像制成游戏（无等待），内容上是实时生成（无限分支）**

用户永远可以离开选项框自由输入，这意味着任何时刻都能走出"制成轨道"进入完全开放的生成模式——这是传统 Galgame 做不到的。这两种体验的切换是无缝的。

---

## 两个问题的关联

LLM Wiki（Q1）和 Galgame 预测（Q2）其实共享同一个底层设计：

**"LLM 的输出不只是给当前用户看的，同时也在更新某种状态。"**

- LLM Wiki：每次对话 = Ingest，更新知识库 Memory
- Galgame：每次回合完成 = 触发 Preflight，更新预测选项缓存

两者都是"主流程完成后异步触发副作用"，WE 的 goroutine + 变量系统天然支持这种模式。
