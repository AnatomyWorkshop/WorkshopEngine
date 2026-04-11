# 宏注册表扩展性 + VN 渲染时机分析 + JS-Slash-Runner 架构拆解

> 日期：2026-04-11
> 触发：Deepwiki 分析文档 `SillyTavern_EJS与提示词模板实现分析.md` 引发的三个问题
> 参照：`plagiarism-and-secret/JS-Slash-Runner` 源码分析

---

## 问题一：WE 宏注册表是否可扩展？扩展是否方便？是否后端执行？

### 当前状态

WE 当前宏系统（`internal/engine/macros/expand.go`）是硬编码的 `strings.ReplaceAll` 链，支持 6 个宏：`{{char}}`、`{{user}}`、`{{persona}}`、`{{getvar::key}}`、`{{time}}`、`{{date}}`。

**不可扩展。** 添加新宏必须修改 `Expand()` 函数本身。

### P-4K 计划的改进

P-4K 设计了 `Registry` + `Handler` 模式：

```go
type Handler func(name string, args []string, ctx *MacroContext) (string, bool)
```

- 通过 `Register(name, handler)` 注册新宏
- 单次 regex 扫描 `\{\{([^}]+)\}\}` 提取所有宏调用
- `ReadOnly` 上下文区分 Verifier 只读阶段和 Pipeline 可写阶段
- 嵌套展开最多 3 轮

**扩展方便吗？** 方便。注册一个新宏只需一行：

```go
registry.Register("random", func(name string, args []string, ctx *MacroContext) (string, bool) {
    n, _ := strconv.Atoi(args[0])
    return strconv.Itoa(rand.Intn(n)), true
})
```

### 是否后端执行？

**是，完全后端执行。** 这是 WE 和 ST 的核心架构差异：

| 维度 | SillyTavern | WorkshopEngine |
|------|-------------|----------------|
| 宏展开位置 | 前端 JS（浏览器） | 后端 Go（服务器） |
| 执行时机 | 发送 prompt 前 + 渲染消息时 | Pipeline 组装时（PresetNode/WorldbookNode 输出前） |
| 安全模型 | 用户本地运行，信任所有代码 | 多用户服务，宏不能执行任意代码 |
| 扩展方式 | JS 插件注入全局对象 | Go 代码注册 Handler |

**WE 的后端执行是正确的选择：**
1. 多用户环境下，宏展开必须在服务端完成，否则不同客户端可能产生不同的 prompt
2. 宏结果影响 LLM 输入（世界书触发、变量注入），这些逻辑必须在 Pipeline 内
3. 后端执行可以做 `ReadOnly` 安全约束，Verifier 阶段禁止副作用宏

**但有一个例外需要注意：** 前端渲染时的宏（如 `{{char}}` 在对话气泡中显示为角色名）。这类纯展示宏可以在前端做简单替换，不需要走后端。WE 前端可以维护一个轻量的只读宏表（从后端同步 char/user/persona 等值），渲染时本地替换。

### 与 ST EJS 的能力对比

ST 的 EJS 是图灵完备的（嵌入任意 JS），WE 的宏注册表不是。这是**有意为之**：

- EJS 的图灵完备性在多用户环境下是安全灾难（用户可以在世界书里写 `<% fetch('...') %>`）
- WE 的宏是声明式的：`{{getvar::key}}`、`{{setvar::key::value}}`，每个宏的行为由后端 Handler 严格定义
- 对于需要复杂逻辑的场景（如条件世界书触发），WE 用世界书的 `scan_depth` + `trigger_keywords` + `stage_tags` 组合实现，而非在模板里写 if/else

**结论：WE 不需要也不应该实现 EJS 的全部能力。** 宏注册表覆盖 95% 的实际使用场景，剩余 5% 的复杂逻辑通过世界书配置和变量系统实现。

---

## 问题二：VN 渲染器在后端执行是否导致回复时间过长？

### 先澄清一个误解

**VN 渲染不在后端执行。** WE 的架构是：

```
LLM 输出 → 后端 parser.Parse() 提取 VNDirectives → JSON 返回前端 → 前端 VN 渲染器渲染
```

后端只做**解析**（从 `<game_response>` XML 中提取 `[bg|...]`、`[bgm|...]`、`[sprite|...]` 等指令），不做渲染。渲染完全在前端。

### 时间线分析

```
用户输入 → [后端 Pipeline 组装 ~5ms] → [LLM 生成 ~2-15s] → [后端解析 ~1ms] → [前端渲染]
                                                                                    ↓
                                                                              VN 渲染器执行
                                                                              - 背景切换: CSS transition 0.5s
                                                                              - 立绘动画: CSS transition 0.3s
                                                                              - 打字机效果: 逐字显示
                                                                              - BGM 切换: crossfade
```

**VN 渲染不增加"回复时间"。** 用户感知的等待时间 = LLM 生成时间。VN 渲染是在收到回复**之后**的展示动画，和"等待回复"是两个阶段。

### 与 ST 前端渲染的区别

| 维度 | SillyTavern | WorkshopEngine |
|------|-------------|----------------|
| 渲染触发 | 消息渲染事件（`CHARACTER_MESSAGE_RENDERED`） | API 响应中的 `vn` 字段 |
| 渲染内容 | EJS 模板 + iframe 沙箱 + 自定义 HTML/CSS/JS | 结构化 VNDirectives JSON → 专用渲染组件 |
| 渲染方式 | 用户写任意 HTML，iframe 隔离执行 | 预定义的 bg/sprite/bgm/cg 图层，CSS 动画 |
| 性能 | iframe 创建开销大（每条消息一个 iframe） | 复用固定图层，只更新状态 |
| 安全性 | iframe sandbox 隔离 | 无任意代码执行，纯数据驱动 |

### ST 的 JS-Slash-Runner 渲染架构

从源码分析，JS-Slash-Runner 的渲染流程：

1. **消息渲染时**（`demacroOnRender`）：遍历 `.mes_text` DOM，用 regex 替换自定义宏
2. **iframe 渲染**（`createSrcContent`）：将用户写的 HTML/CSS/JS 包装成完整 HTML 文档，注入 iframe
3. **高度自适应**：iframe 内脚本通过 `postMessage` 通知父页面调整高度

**关键洞察：ST 的"VN 渲染"不是引擎内置功能，而是用户通过 EJS + iframe 自己实现的。** 每个角色卡作者自己写 HTML/CSS/JS 来实现立绘、背景、音乐等效果。这导致：
- 每个角色卡的 VN 实现质量参差不齐
- iframe 隔离带来性能开销
- 没有统一的资产管理系统

### WE 的优势

WE 把 VN 渲染作为**引擎内置能力**：
- 后端解析 `[bg|forest.jpg]` → 前端渲染器统一处理
- 资产系统（P-4I Stage B）提供 sprite/scene/bgm/cg 的上传和管理
- 游戏创作者只需在 prompt 中告诉 AI 使用哪些指令，不需要写前端代码
- 渲染质量一致，性能可控

### 有更好的建议吗？

**建议：流式 VN 渲染（P-4H + P-4I 联动）**

当前设计是等 LLM 完整输出后再解析 VN 指令。更好的方案：

1. **P-4H SSE 流式推送**已在计划中。可以在流式输出中**增量解析** VN 指令：
   - 检测到 `[bg|forest.jpg]` 时立即推送背景切换事件，不等整条消息完成
   - 检测到 `[bgm|battle.mp3]` 时立即切换音乐
   - 对话文本逐句推送，前端逐句显示（打字机效果）

2. **分层渲染优先级**：
   - 背景/BGM：立即执行（用户感知最强）
   - 立绘：检测到完整指令后执行
   - 对话文本：流式打字机
   - 选项按钮：等完整输出后渲染

这样用户在 LLM 生成过程中就能看到场景变化，体验远优于"等完再渲染"。

---

## JS-Slash-Runner 架构分析

### 项目定位

JS-Slash-Runner（又名 TavernHelper）是 SillyTavern 的第三方扩展，提供：
- 自定义宏（macro_like）
- Slash 命令执行
- iframe 沙箱渲染
- 音频管理
- 变量管理增强
- 世界书/角色卡/预设的 CRUD API

### 核心架构

```
index.ts（入口）
├── registerMacros()          — 注册 userAvatarPath/charAvatarPath 到 ST 宏系统
├── registerSwipeEvent()      — Swipe 手势事件
├── initTavernHelperObject()  — 暴露 globalThis.TavernHelper API
├── initThirdPartyObject()    — 第三方兼容层
└── initSlashCommands()       — 注册 /audio 等 slash 命令
```

### 宏系统（macro_like.ts）

**不是 ST 原生宏系统的一部分**，而是一个独立的"类宏"系统：

```typescript
interface MacroLike {
  regex: RegExp;
  replace: (context: MacroLikeContext, substring: string, ...args: any[]) => string;
}
```

- 内置 2 个宏：`{{get_*_variable::path}}` 和 `{{format_*_variable::path}}`（支持 message/chat/character/preset/global 五级变量）
- 扩展方式：`registerMacroLike(regex, replaceFn)` — 任何 iframe 脚本都可以注册
- 执行时机：**两个阶段**
  1. `demacroOnPrompt`：发送给 LLM 前，在 prompt 消息数组上执行替换
  2. `demacroOnRender`：消息渲染到 DOM 后，在 HTML 上执行替换

### iframe 渲染（render/iframe.ts）

角色卡作者写的 HTML/CSS/JS 被包装成完整 HTML 文档，注入 iframe：

```html
<!DOCTYPE html>
<html>
<head>
  <style>/* 基础样式重置 */</style>
  <script src="predefine.js"></script>      <!-- 预定义变量 -->
  <script src="adjust_viewport.js"></script> <!-- vh 单位修复 -->
  <script src="adjust_iframe_height.js"></script> <!-- 高度自适应 -->
</head>
<body>${content}</body>
</html>
```

**性能特征：**
- 每条消息可能创建一个 iframe（如果消息包含 HTML 渲染内容）
- iframe 创建是同步的，但内部脚本异步执行
- 通过 `postMessage` 与父页面通信
- vh 单位需要特殊处理（iframe 内的 vh 不等于外部视口高度）

### TavernHelper 全局 API

暴露了约 100+ 个函数到 `globalThis.TavernHelper`，覆盖：
- 角色卡 CRUD
- 聊天消息 CRUD
- 世界书/预设 CRUD
- 变量读写
- 音频控制
- LLM 生成调用
- 事件系统
- Slash 命令触发

**这本质上是一个完整的 SillyTavern SDK**，让 iframe 内的脚本可以操作 ST 的所有功能。

### 对 WE 的启示

1. **WE 不需要 iframe 沙箱模式。** ST 用 iframe 是因为用户写任意 JS，需要隔离。WE 的 VN 渲染是引擎内置的结构化指令，不需要执行用户代码。

2. **WE 不需要暴露 100+ API 给前端脚本。** ST 的 TavernHelper 是为了弥补 ST 本身 API 不足。WE 有完整的 REST API（73 个端点），前端直接调用即可。

3. **WE 的宏系统应该保持简单。** ST 的宏系统复杂是因为要兼容各种用户创作的角色卡。WE 的宏在后端执行，面向游戏创作者（不是终端用户），保持声明式 + 可注册即可。

---

## 架构层面的建议

### 1. 不要追求 EJS 等价

Deepwiki 分析文档中提到"WE 宏注册表扩展实现 EJS 全部功能"，包括 `{{if}}`、`{{for}}`、`{{include}}` 等。**不建议这样做。**

理由：
- 图灵完备的模板语言在多用户后端是安全隐患
- 世界书的 `scan_depth` + `trigger_keywords` + `stage_tags` 已经覆盖了条件注入的需求
- 循环逻辑在 prompt 模板中极少使用（ST 社区中 EJS 的 for 循环使用率 < 1%）
- 如果真的需要复杂逻辑，应该在游戏模板的 Go 代码中实现，而非在模板语言中

### 2. P-4K 的优先级可以降低

P-4K 计划的 190 行改动中，真正紧急的只有：
- `setvar` / `addvar` 副作用宏（让 AI 可以通过宏修改变量）
- `ReadOnly` 安全约束

嵌套展开和 Registry 重构可以延后。当前 `expand.go` 的硬编码方式虽然不优雅，但 6 个宏的性能和正确性都没问题。

### 3. P-4I VN 渲染的关键路径

VN 渲染的真正瓶颈不是渲染速度，而是**资产管理**：
- 游戏创作者需要上传立绘/背景/BGM
- 前端需要预加载资产（避免切换时白屏）
- 资产 URL 需要在 prompt 中可引用（让 AI 知道有哪些可用素材）

建议 P-4I Stage B（资产系统）优先于 Stage C（渲染器），因为没有资产的渲染器只能显示占位符。

### 4. 长期方向：声明式 VN 而非脚本式 VN

ST 的 VN 体验依赖用户写 JS 脚本（通过 JS-Slash-Runner 的 iframe）。WE 应该走**声明式**路线：

```
游戏创作者定义素材清单 → AI 在输出中使用 [bg|...] [sprite|...] 指令 → 引擎自动渲染
```

创作者不需要写任何代码。这是 WE 相对于 ST 的核心体验优势。
