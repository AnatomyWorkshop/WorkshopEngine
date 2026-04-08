# TavernHeadless 更新分析 + 渲染架构比较 + WE 宏指令规划

> 时间：2026-04-08
> 基础：本地克隆 `D:\ai-game-workshop\plagiarism-and-secret\TavernHeadless`，最新 commit `cf67bee`（2026-04-02），git pull 因网络原因未能拉取更新，以当前本地版本分析。

---

## 一、TavernHeadless 近期更新梳理

### 1.1 最新 commit 序列（2026-03 ~ 2026-04）

| 日期 | commit | 内容 |
|------|--------|------|
| 2026-04-02 | cf67bee | **Secret 存储与 Auth 契约修复**：API Key 加密存储（`secret_config_encrypted` 字段）与 Auth 鉴权契约对齐 |
| 2026-04-02 | e25da3c | **floor_run_state + branch-scoped vars**：新增 `floor_run_state` 表（tracking `running/completed/failed/cancelled`），变量作用域扩展支持 branch 级别 |
| 2026-04-02 | 05bc73b | **Regex Profile runtime/sdk 对齐**：正则后处理与运行时、客户端 SDK 合约同步 |
| 2026-04-01 | 1ff6507 | **Runtime 第一组完成**：Job Runtime 系统第一批完整交付 |
| 2026-04-01 | 0fbc04e | **MCP 工具延迟运行入口**：MCP 工具在用户确认后延迟执行（deferred tool runtime entry） |
| 2026-03-31 | d1f9170 | **后台 Job + Mutation Runtime 对齐**：`runtime_job` 表与 Mutation batch 系统双向对齐 |
| 2026-03-30 | c38f5a0 | **Character/User 并发写入协议**：多账户隔离下角色卡与用户并发写入的竞态修复 |
| 2026-03-30 | 9d474a1 | **Preset/Worldbook 并发写入修复**：预设与世界书并发读写场景的竞态修复 |
| 2026-03-30 | 43eea0f | **LLM Profile + Instance 修复**：LLM Profile 与 Instance 绑定层修复收尾 |
| 2026-03-29 | a2c7432 | **多账户隔离修复**：完成多账户数据隔离修复 |
| 2026-03-28 | 8a63fa8 | **Memory v2 修复**：记忆系统 v2 修复全量交付 |
| 2026-03-27 | f6edcb6 | **Tool-calling runtime 修复**：工具调用运行时全量修复 |
| 2026-03-27 | 2adc9e8 | **变量系统修复与客户端收口**：变量系统修复并收口到客户端 SDK |
| 2026-03-21 | 933e3e6 | **Turn commit 一致性边界**：回合提交的原子性边界完整实现 |
| 2026-03-18 | 5f7ec83 | **Integration Kit SDK 扩展**：`@tavern/sdk` + `@tavern/client-helpers` surface 扩展，文档更新 |

### 1.2 功能里程碑（PROGRESS.md 记录）

当前里程碑为 `M9-M12`，主要方向：

**已完成：**
- Tool Calling Phase 1-5（80 个测试）：内置工具 + 预设自定义工具 + API 路由，完整的 `ToolRegistry`/`ToolExecutor` + 14→23 个内置工具
- MCP 集成（32 个测试）：`McpConnectionManager` + stdio/HTTP 双传输，12 个 API 端点，用户授权 deferred 执行
- ResourceToolProvider 第 1-3 批（42+38+42 个测试）：AI 可直接操作角色卡/世界书/预设/正则的完整工具集（23 个工具）
- 对话导入/导出（53 个测试）：ST JSONL 导入解析 + `.thchat` 原生格式，`serializeSessionToThChat()` + `serializeSessionToStJsonl()`，character 导出（ST Card V2 JSON），worldbook/preset/regex 导出
- Native Pipeline `ConditionNode`/`TransformNode`：分支执行与正则变换节点

**全量测试规模：** core 315 + adapters 142 + shared 32 + api 613 = **1102 个测试**

**当前进行中：**
- `floor_run_state` 精细状态机（`running/completed/failed/cancelled`）
- Branch-scoped variables（分支级变量作用域）
- Secret 存储加密（API Key AES 加密）
- Runtime Job 系统（`runtime_job` 表 + lease/retry/dead_letter）
- Mutation Runtime（变更执行器 + 并发保护）

### 1.3 更新思路与下一步方向

**当前核心主线：稳定性修复 → 生产就绪**

TH 的更新轨迹从快速功能堆砌（M2-M9 高密度交付）转向系统稳定化：

1. **并发安全**：多账户隔离、并发写入竞态、Generation Guard（reject/queue 两模式）
2. **Secret 安全**：API Key 加密存储（`secret_config_encrypted`）
3. **Job 持久化**：`runtime_job` 表保证进程重启后任务不丢失
4. **SDK 收口**：`@tavern/sdk` 统一客户端面，前端通过 SDK 而非直连 API
5. **Floor Run State 精细化**：`floor_run_state` 表记录回合执行状态，支持 retry/cancel

**下一步推断：**
- Character Lab（角色实验室）、Memory Explorer（记忆可视化）——PROGRESS.md P2 路线
- Web 前端继续解耦收口（wfd-05 ~ wfd-06），目标是 Narrative Workspace 而非管理后台
- 暂无 VN 渲染/游玩层计划——TH 的前端 `apps/web` 是**创作工作台**，不是玩家游玩界面

---

## 二、SillyTavern 是否依赖 WebGL？

**结论：ST 不依赖 WebGL，是纯 HTML/CSS/JS 渲染。**

检查结果：
- ST `public/scripts/` 主体逻辑全部是 DOM 操作
- 搜索 `pixi\|webgl\|PixiJS` 仅在 `node_modules`（caniuse-lite 特性检测数据）和 webpack bundle 中出现，不在主逻辑文件中
- 角色立绘通过 `<img>` 标签 + CSS 绝对定位渲染，背景通过 `background-image` CSS 属性
- 表情（expressions）扩展通过 LLM 推断情绪 → 切换对应表情图片（`imageSrc` 赋值），是纯 DOM/CSS 实现
- **Live2D** 是可选扩展，不是核心渲染依赖；WebGPU/WebGL 同理（仅特定 TTS 语音合成插件用到 WASM + WebGL）

**ST 渲染堆栈：**
```
背景层：CSS background-image (body, #bg1)
立绘层：<img id="expression-image"> + CSS position:absolute
对话气泡：<div class="mes"> + jQuery DOM 操作
动效：CSS animation/transition（抖动、淡入淡出）
BGM/SFX：<audio> HTMLMediaElement API
```

**结论意义：** ST 的渲染可以在任何 Web 浏览器（包括手机 Chrome）中完整运行，无特殊 GPU 要求。

---

## 三、打包两个软件的渲染方案比较

### 3.1 "两个软件"的可能形态

| 方案 | 宿主环境 | 渲染能力 | 分发方式 |
|------|---------|---------|---------|
| **Electron 桌面 App** | Chromium（内嵌）| 全栈 Web + 本地文件系统 | 安装包（EXE/DMG） |
| **Tauri 桌面 App** | 系统 WebView（Edge/WebKit） | Web + Rust 原生能力 | 安装包（更小） |
| **纯 Web 应用** | 浏览器 | 受同源策略限制 | URL 访问/PWA |
| **Mobile App（Capacitor/Flutter）** | WebView + 原生壳 | Web + 原生 API | App Store |

### 3.2 渲染速度：Web VN vs 原生 App

**Web VN 渲染不比桌面慢，关键是资产加载策略。**

| 因素 | 桌面 App（Electron）| Web App（浏览器）| 差距 |
|------|---------------------|-----------------|------|
| 渲染帧率 | Chromium，60fps | 相同 Chromium，60fps | **无差距** |
| CSS 动画 | GPU 加速 | GPU 加速 | **无差距** |
| 图片加载 | 本地磁盘 ~1ms | HTTP 缓存/CDN | 首次加载慢，缓存后等同 |
| 音频播放 | 本地文件 | `<audio>` 流式加载 | 首次切换 BGM 有延迟 |
| JS 执行 | V8（内嵌）| V8（浏览器）| **无差距** |

**核心矛盾不在渲染速度，而在资产文件的来源：**
- 桌面 App：读取本地 `public/` 目录，路径直接可用，零网络延迟
- Web App：必须通过 HTTP/HTTPS 请求 CDN 或后端静态文件服务器，需要预加载策略

**推荐方案：** 如果要同时打包两个软件：
1. **Web 优先**（当前方向）：后端 `/api/v2/play/games/:id/assets` + 静态文件 CDN + 浏览器缓存，移动端和桌面浏览器均可游玩
2. **可选 Electron 包装**：把同一个 Web 前端用 Electron 打包，后端作为本地进程随 Electron 启动（Electron + Go 子进程），静态文件读取本地磁盘

两者共享**同一套 Vue 前端代码**，资产加载方式通过环境变量切换（`VITE_ASSET_BASE` 指向 CDN 或 `file://`）。

---

## 四、当前在 Web 上渲染 VN 缺少的前置步骤

当前状态：后端正确输出结构化 VN 数据（`meta.vn.bg/bgm/lines/options`），前端 Stage A 已显示文字。

### 4.1 缺失环节清单

**后端（必须先做）：**

| 缺失项 | 说明 | 优先级 |
|--------|------|--------|
| **`Material` 表 `filename`/`asset_type` 字段** | 现有 Material 只有 `name` 和 `content`，没有文件路径 | P0 |
| **文件上传 API** | `POST /api/v2/assets/:game_id/upload`（单文件）+ `POST /api/v2/assets/:game_id/upload-pack`（zip 包）| P0 |
| **资产名字→URL 解析 API** | `GET /api/v2/play/games/:id/assets?type=sprite` → `{name: "izumi_normal", url: "https://..."}` | P0 |
| **静态文件服务** | Go 后端 serve `/assets/:game_id/:type/:filename`，或指向 MinIO/CDN | P0 |

**资产文件（必须上传）：**

| 资产类型 | 来源 | 数量（异世界和平）|
|--------|------|------|
| 立绘 PNG | ST 游戏包 `characters/` 目录 | ~78 张 |
| 背景 JPG | ST 游戏包 `backgrounds/` 目录 | ~40 张 |
| BGM MP3/OGG | ST 游戏包 `bgm/` 目录 | 8 首 |
| CG PNG | ST 游戏包 `cg/` 目录 | 8 张 |

**前端（Stage B/C）：**

| 缺失项 | 说明 |
|--------|------|
| **资产缓存层** | 初始化时拉取 `/assets` API，缓存 `Map<string, string>`（名字→URL）|
| **背景层** | CSS `background-image` 切换，淡入淡出 transition |
| **立绘层** | 绝对定位 `<img>`，左/中/右槽位，淡入淡出 |
| **CG 覆盖层** | 全屏 `<div>` + 点击关闭 |
| **BGM 层** | `<audio>` + crossfade（当前 BGM 音量渐出，新 BGM 渐入）|

### 4.2 阻塞依赖关系

```
资产文件（zip 包）
    → 文件上传 API（后端 4-I B）
        → 资产名字→URL 解析 API（后端 4-I B）
            → 前端 Stage B 资产缓存层
                → 前端 Stage C 背景/立绘/BGM 渲染器
```

Stage A（当前已实现）对上述链路无依赖，可立即游玩（纯文字模式）。

---

## 五、TH 的宏指令完整度 vs WE 的规划

### 5.1 SillyTavern 宏指令体系（完整）

ST 的宏系统已演化为**带 AST 的完整 DSL**（MacroEngine 2.0）：

```
Lexer → Parser（Chevrotain CST）→ CstWalker → 求值
```

**宏类型分类（7 大组）：**

| 组 | 宏示例 | 作用 |
|----|--------|------|
| **core** | `{{char}}`, `{{user}}`, `{{persona}}` | 基础名字替换 |
| **env** | `{{model}}`, `{{api}}`, `{{nai_prefix}}` | 运行时环境变量 |
| **state** | `{{lastMessageId}}`, `{{charPromptLastId}}` | 对话状态 |
| **chat** | `{{original}}`, `{{exampleSeparator}}` | 对话内容 |
| **time** | `{{time}}`, `{{date}}`, `{{idle_duration}}` | 时间相关 |
| **variables** | `{{getvar::key}}`, `{{setvar::key::value}}` | ST 本地变量 |
| **instruct** | `{{[system]}}`, `{{[user]}}` | Instruct 模式格式化 |

**关键能力：**
- 宏可嵌套：`{{getvar::{{char}}_stage}}`（变量名动态）
- 扩展宏注册：插件可注册自定义宏处理器
- 宏求值懒触发：只在 prompt 组装时展开，不在存储时求值

### 5.2 TavernHeadless 宏指令状态

**结论：TH 宏系统不完整，委托给调用方。**

TH 的 `adapters-sillytavern/src/compat-assembler.ts` 声明了 `substituteParams?: (text: string) => string` 函数接口，即**宏展开由调用者提供**，不在 TH 核心层实现。

原因：TH 是纯后端 API 服务，`{{char}}` 等宏中的名字/环境信息需要由 API 调用者（前端/SDK 客户端）在发起请求时传入。TH API schema 的 `templateVariables` 字段设计用于此目的，但完整的宏系统（嵌套求值、状态宏、时间宏等）尚未在 TH 后端实现。

**已实现：** `{{char}}` / `{{user}}` 基础替换（通过 `config.ts` 里 `substituteParams`）
**未实现：** 嵌套求值、`{{getvar}}` / `{{setvar}}`、时间宏、`{{original}}`、自定义扩展宏

### 5.3 WE 当前宏指令状态

WE 目前**完全没有宏展开**，存在以下问题：

1. **角色卡导入的 `[Main]` 提示词**包含大量 ST 宏：`{{char}}` / `{{user}}` / `{{persona}}` / `{{getvar::...}}` 等，导入后原文注入，LLM 直接看到字面 `{{char}}`
2. **WorldbookEntry.content** 中的宏同理
3. **PresetEntry.content** 中的宏同理

**影响：** 对于包含 ST 宏的角色卡（异世界和平 Izumi 的 `[main]` 提示词），当前注入内容中 `{{char}}` 未被替换为"泉"，LLM 会直接看到 `{{char}}`，可能混淆或忽略。

### 5.4 WE 宏系统规划

**最小可用版（立即做）：** 基础替换，在 Pipeline 组装时展开

```go
// 新增 internal/engine/macros/expand.go
type MacroContext struct {
    CharName   string   // {{char}}
    UserName   string   // {{user}}
    PersonaName string  // {{persona}}
    Variables  map[string]any // {{getvar::key}}
    Now        time.Time      // {{time}}, {{date}}
}

func Expand(text string, ctx MacroContext) string
```

注入点：在 `PresetEntryNode`、`WorldbookNode`、`CharacterInjectionNode` 输出内容前调用 `Expand()`。

**完整版（Phase 4-J 候选）：**
- `{{getvar::key}}` / `{{setvar::key::value}}` 对接 `ctx.Variables`
- `{{time}}` / `{{date}}` / `{{lastMessage}}`
- 嵌套宏求值（`{{getvar::{{char}}_stage}}`）

**与 ST 的差距：** WE 不需要 ST 的 instruct 格式宏（`{{[system]}}`），也不需要 NAI/API 环境宏；重点是 `{{char}}/{{user}}/{{persona}}` 基础名字宏 + `{{getvar}}/{{setvar}}` 变量宏，这覆盖了 90% 的实际使用场景。

---

## 六、WE 中期方向总结

| 问题 | 答案 |
|------|------|
| ST 是否用 WebGL？ | 否，纯 HTML/CSS/JS |
| TH 是否有 VN 渲染层？ | 否，TH 定位是创作工作台（Narrative Workspace），无玩家游玩 UI |
| 打包两软件最合适的渲染方案？ | Web 优先 + 可选 Electron 包装（同一 Vue 前端，资产路径通过 env 切换） |
| Web VN 渲染速度劣于桌面？ | 仅资产首次加载有延迟；帧率/动画无差距；用预加载策略填平 |
| VN 渲染缺少哪些前置步骤？ | 后端：Material 文件字段 + 上传 API + 资产 URL API；前端：资产缓存 + 背景/立绘/BGM 层（Stage B/C） |
| TH 宏指令完整度？ | 基础宏有，但委托调用方实现；嵌套/状态/变量宏未在后端实现 |
| WE 宏指令现状？ | 完全无宏展开；需新增 `macros.Expand()` 在 Pipeline 组装时展开 |
| WE 下一步优先级？ | **Stage A 已完成** → 资产上传 API（4-I B）→ 宏展开（新增 4-J）→ Stage B/C 渲染器 |

---

## 七、对 implementation-plan.md 的影响

以下内容需要补充/修改到 `implementation-plan.md`：

1. **新增 3-I.1（宏展开）**：`{{char}}/{{user}}/{{getvar}}` 基础展开，Pipeline 组装前展开，约 50-80 行 Go，无 DB 变更，影响所有 PresetEntry/Worldbook 内容
2. **4-I B（资产系统）更新**：明确打包方案 = Web 优先 + 可选 Electron，资产 API 设计与 CDN/本地磁盘双路径兼容
3. **TH 进展同步**：`floor_run_state` 精细状态机（TH 已实现，WE 可参考）；`deferred tool runtime`（MCP 工具用户授权后延迟执行，WE 暂不需要）；Secret 加密（WE 4-A 方向对齐）
