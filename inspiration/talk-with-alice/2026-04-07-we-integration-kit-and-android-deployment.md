# WE 官方集成层设计 + GW Android 远期部署架构

> 日期：2026-04-07
> 接续：2026-04-07-th-worldbook-efficiency-and-midterm-prep.md
> 触发问题：WE 应该怎么设计官方集成层？GW 在 Android 上远期部署需要安装什么、服务器部署什么、软件应该怎么设计？

---

## 一、TH 官方集成层的设计思路

TH 的 Official Integration Kit 由两个包组成，分工非常清晰：

```
@tavern/sdk              ← 传输层：HTTP 客户端 + SSE 流解析 + 28+ 资源封装
@tavern/client-helpers   ← 语义层：纯函数，把 API 数据整理成前端好用的形态
```

### @tavern/sdk 的核心设计

- **框架无关**：只依赖标准 `fetch`，通过 `fetchImpl` 选项注入适配器（React Native、Deno 等）
- **动态 Auth**：`getHeaders()` 是一个回调（可以是 async），支持任意鉴权机制
- **类型安全**：类型从 OpenAPI spec 自动生成，不手写 DTO
- **SSE 解析**：`readSseStream()` 处理所有事件类型（`start/chunk/run/tool/summary/error/done`），以回调模式暴露

### @tavern/client-helpers 的核心设计

- **纯函数**：没有 fetch、没有状态，只对已拿到的数据做转换
- **Reducer 模式**：`createInitialRespondStreamState()` + `reduceRespondStream()` — 流事件逐步累积为 UI 状态
- **数据规范化**：`resolveUsage()` 统一不同 LLM 的 token 计数格式；`buildTimelineMessages()` 把 Floor 层级打平为线性消息列表；`flattenVariableSnapshot()` 把嵌套变量树展平为 inspector 行

合在一起的效果：前端（Vue/React/任意）只需 `import { createTavernClient } from '@tavern/sdk'` 和几个 helper，就能做出完整的游玩界面，不用了解后端的 Floor/Page 数据结构细节。

---

## 二、WE 应该怎么设计官方集成层

### 2.1 WE 和 TH 定位的差异对集成层的影响

TH 是 headless API，第三方客户端（包括官方前端）都通过 SDK 接入——这是 SDK 必须存在的原因。

WE 的定位有所不同：
- WE 是 **游戏引擎 + 服务端**，GW 前端是 **第一方消费方**，不是第三方
- WE 的 API 是给 GW 前端用的，同时也需要支持将来的第三方（别的客户端、脚本、自动化）
- WE 还打算支持"所有 AI RP session 形式在手机上游玩"——这意味着前端是主要消费方，而不是 headless 调用者

所以 WE 的集成层需求比 TH 更具体：**它的主要消费者是 GW 前端，次要消费者是 CW 第三方**。

### 2.2 WE Integration Kit 设计

建议两层结构，和 TH 对齐但简化：

```
@gw/api-client       ← 传输层（等价 @tavern/sdk，但不需要做到 28 个资源）
@gw/play-helpers     ← 语义层（等价 @tavern/client-helpers，专注游玩流）
```

放在前端项目里（不需要发布 npm），路径 `frontend/packages/` 或 `frontend/src/api/`。

---

#### `@gw/api-client`（传输层）

**职责**：封装所有 WE backend REST API 调用，类型安全，SSE 流解析。

**关键设计点**：

1. **类型从 swaggo 生成**：Phase 4-D 实现 swaggo 后，用 `openapi-typescript` 生成 `openapi.d.ts`，api-client 基于这个文件的类型，不手写 DTO。每次后端改接口，跑一次生成脚本，TypeScript 类型错误立刻暴露。

2. **SSE 解析**：WE 实现 4-H（Floor Run Phase SSE）后，SSE 事件会有 `phase` + `token` 两种类型。`readGameStream()` 处理：
   ```typescript
   type GameStreamEvent =
     | { event: 'phase'; data: { phase: WEPhase } }
     | { event: 'token'; data: { text: string } }
     | { event: 'done'; data: { floor_id: string; usage: Usage } }
     | { event: 'error'; data: { code: string; message: string } }
   ```

3. **Auth 回调**：`getHeaders(): Record<string, string>` — 支持 API Key 模式（内测）和 JWT 模式（上线后），不硬编码

4. **资源范围**：只需要覆盖游玩层（session / floor / page / memory / variable）和必要的创作层（template / card / material）。不需要 TH 那样覆盖全部 28 个资源。

```typescript
const client = createGWClient({ baseURL, getHeaders })

// 游玩
client.sessions.create({ game_id, title })
client.sessions.playTurn(sessionId, { content, role: 'user' })
client.sessions.stream(sessionId)           // returns ReadableStream
client.sessions.getState(sessionId)
client.sessions.getMemories(sessionId)

// 创作
client.templates.list()
client.templates.importPackage(file)
```

---

#### `@gw/play-helpers`（语义层）

**职责**：纯函数，把 WE 的 API 数据整理成游玩界面需要的形态。

核心函数：

| 函数 | 输入 | 输出 | 用途 |
|------|------|------|------|
| `buildMessageTimeline(floors)` | `Floor[]` | `TimelineMessage[]` | 把楼层列表打平为线性消息流（含 swipe 状态） |
| `reduceGameStream(state, event)` | `StreamState, GameStreamEvent` | `StreamState` | Reducer：累积 SSE 事件为 UI 状态（当前 phase + 已生成文本） |
| `parseVNDirectives(text)` | `string` | `VNDirective[]` | 解析 `<game_response>` 块中的立绘/背景/BGM 指令 |
| `flattenVariables(snapshot)` | `VariableSnapshot` | `VariableRow[]` | 把嵌套变量树展平，用于变量面板展示 |
| `resolveUsage(usage)` | `UsageObject` | `{ prompt, completion, total }` | 统一不同 LLM 的 token 计数格式 |
| `mapPhaseToLabel(phase)` | `WEPhase` | `string` | "director_running" → "正在分析上下文…" |

Pinia store 可以直接调这些 helper 而不用内嵌逻辑，保持 store 的干净。

---

### 2.3 什么时候做

| 时机 | 动作 |
|------|------|
| **现在（MVP 阶段）** | 不做正式集成层——前端直接在 `src/api/client.js` 里写裸 fetch，SSE 自己解析。接口稳定前过度封装是浪费。 |
| **Phase 4-D（swaggo）完成后** | 用 `openapi-typescript` 生成类型文件，创建 `@gw/api-client` 的 TypeScript 版本 |
| **4-H（Floor Run Phase SSE）完成后** | 写 `reduceGameStream()` helper，让所有界面统一处理 phase 事件 |
| **VN 渲染引擎（Phase 5）前** | 写 `parseVNDirectives()`，把解析逻辑从界面组件里提出来 |

---

## 三、GW 在 Android 上的远期部署架构

### 3.1 什么应该在服务器，什么应该在 Android 设备上

先确认一个前提：**WE 是服务端引擎，不是本地软件**。这一点和 ST（本地 Node.js 应用）根本不同。

```
ST 的部署模型：
  用户设备（Windows/Mac）
  ├── SillyTavern Node.js 进程（含 UI + 引擎）
  └── 浏览器访问 localhost:8000

WE 的部署模型：
  服务器
  ├── WE Go 二进制（引擎 + API）
  └── PostgreSQL
  
  用户设备（Android）
  └── 客户端（浏览器 PWA 或原生 App）→ HTTP/SSE → 服务器
```

这个模型是刻意的：
- LLM API Key 不暴露在设备上（4-A 加密存服务端）
- 多设备无缝续档（换手机继续游玩，存档在服务器）
- 游戏包/角色卡/素材全部在服务端，设备无需存储

---

### 3.2 Android 客户端的三个形态选项

#### 选项 A：PWA（渐进式 Web 应用）

```
Android Chrome
└── GW Vue 3 前端（PWA）
    ├── Service Worker（离线缓存对话历史）
    ├── Web Push（NPC 自主回合通知）
    └── SSE 流式输出（原生支持）
```

**优点：**
- 零 App Store 审核（AI 内容在国内/海外应用商店都是雷区）
- 发布新版本只需部署前端，用户无需更新
- 与桌面端共用同一套代码
- 可"安装到主屏幕"，体验接近原生应用

**缺点：**
- iOS Safari 的 PWA 支持有历史问题（Web Push 直到 iOS 16.4 才支持）
- 无法访问 Android 文件系统（用户导入角色卡 PNG 需要通过上传接口）
- SSE 在切到后台后可能被系统挂起（安卓激进的后台管理）

**适合 GW 的理由：** AI 游戏内容规避应用商店风险是核心需求；GW 的主要内容（文字 + 图片渲染）不需要原生 API。

#### 选项 B：Capacitor 混合应用

```
Android APK（Capacitor）
└── WebView（运行 GW Vue 3 前端）
    ├── 原生插件：文件选择器（导入角色卡）、本地通知、相机
    └── HTTP + SSE → WE 服务器
```

**优点：**
- 可以发布到 Google Play（提升可发现性）
- 可以访问原生 API（本地通知、文件选择）
- SSE 更稳定（Capacitor 的 WebView 不会被系统挂起）
- 前端代码与 PWA 几乎完全共用

**缺点：**
- 需要维护 Android 构建流程
- 应用商店审核（AI 相关内容需要特别处理）
- Capacitor 插件兼容性问题

**适合什么场景：** 如果 GW 决定上 Google Play，优先选这个而不是直接写原生 App。

#### 选项 C：原生 Android App（Kotlin/Flutter）

开销最大，收益有限——GW 的核心体验是文字+图片，不需要 3D 渲染、蓝牙等硬件能力。除非有明确的性能瓶颈，否则不值得。**暂时不考虑。**

---

### 3.3 远期推荐架构

```
┌─────────────────────────────────────────┐
│              GW 云端服务器              │
│                                         │
│  ┌─────────────────┐  ┌──────────────┐  │
│  │  WE Go 二进制   │  │  PostgreSQL  │  │
│  │  (引擎 + API)   │  │  (存档/记忆) │  │
│  └────────┬────────┘  └──────────────┘  │
│           │ REST + SSE                  │
│  ┌────────┴────────┐                    │
│  │  GW 静态前端    │  ← Vue 3 Build     │
│  │  (CDN 分发)     │                    │
│  └─────────────────┘                    │
└────────────────────┬────────────────────┘
                     │ HTTPS
          ┌──────────┴──────────┐
          │                     │
   ┌──────┴──────┐       ┌──────┴──────┐
   │  Android    │       │   Desktop   │
   │  PWA/App    │       │   Browser   │
   │  Vue 3 UI   │       │   Vue 3 UI  │
   └─────────────┘       └─────────────┘
```

**关键设计决策：**

1. **前端静态文件从 CDN 分发**（Vercel / Cloudflare Pages）：Android 客户端加载的是同一份 JS bundle，更新时无需用户手动操作

2. **WE 后端是唯一的有状态服务**：所有存档、记忆、变量都在 PostgreSQL，设备是无状态的终端

3. **LLM 调用在服务端**：API Key 从不出现在设备上（4-A 加密），GW 平台可以内置公共 Key（带限流），用户也可以绑定自己的 Key

4. **SSE 连接管理**：Android 端需要处理网络切换（WiFi → 4G）导致的 SSE 断开；客户端实现自动重连 + 从最后一个 floor_id 续传

---

### 3.4 需要安装哪些东西

**服务器（云端部署，运维只需处理一次）：**

| 组件 | 用途 | 规格建议 |
|------|------|---------|
| WE Go 二进制 | 游戏引擎 + REST API | 2C4G 入门，LLM 调用是网络 I/O，CPU 瓶颈不大 |
| PostgreSQL | 存档/记忆/模板持久化 | 托管服务（Supabase / PlanetScale） |
| 反向代理（Nginx / Caddy） | HTTPS 终止 + 静态文件 | Caddy 自动 ACME 证书更方便 |
| 可选：Redis | SSE 广播（多副本场景） | 单副本时不需要 |

**Android 设备（用户侧）：**

| 组件 | 说明 |
|------|------|
| Chrome 或任意现代浏览器 | 访问 GW PWA |
| "安装到主屏幕"（PWA） | 一键完成，无需 App Store |
| 无其他依赖 | 不需要 Node.js、不需要 LLM 运行时、不需要本地存储 |

**对比 ST 的用户安装负担：**

```
ST 用户需要：
  1. 安装 Node.js
  2. git clone SillyTavern
  3. 安装 JS-Slash-Runner 扩展
  4. 安装 ST-Prompt-Template 扩展
  5. 配置 API Key
  6. 启动本地服务
  7. 手机访问局域网 IP（如果想手机玩）

GW 用户需要：
  1. 打开浏览器，输入网址
  2. （可选）安装到主屏幕
```

这是 WE 服务端优先设计最核心的用户体验优势。

---

### 3.5 离线和推送通知

**离线场景：**
- 对话历史：Service Worker 缓存最近 N 个 session 的楼层数据（只读），离线时可以翻历史
- 新回合：必须联网（LLM 调用是服务端的），离线时界面显示"等待网络恢复"
- 素材（立绘/BGM）：CDN 缓存（浏览器自动 cache），加载过的素材离线可展示

**推送通知（NPC 自主回合 / 异步多人）：**
- ScheduledTurn 触发后，服务端需要通知玩家"你的游戏有新进展"
- 技术路径：Web Push API（PWA）或 FCM（Capacitor App）
- WE 需要新增 `push_subscription` 表（存储设备的 push endpoint），Phase 5 的一部分
- GW 论坛的消息通知也走同一套推送基础设施

---

### 3.6 软件设计的核心原则

1. **服务端是唯一真相来源**：设备坏了、换手机、清除浏览器缓存，存档不丢

2. **渐进式 Web**：先 PWA，后 Capacitor（如果需要应用商店），不要一开始就写原生

3. **断线续传**：客户端的 SSE 订阅必须处理重连；重连后从 `GET /sessions/:id/state` 恢复 UI 状态，不依赖内存

4. **无感知更新**：前端静态文件走 CDN + Service Worker，用户下次打开 PWA 自动拿到新版本；后端 Go 二进制独立部署，不影响客户端

5. **LLM Key 在服务端**：普通玩家不需要懂 API Key 是什么；高级用户（自带 Key）可选绑定，用 4-A 的加密方案存储

---

---

## 四、重新理解部署模型：本地优先，云端可选

> **补充说明**：上面第三节描述的是纯云端模型。但 GW 的目标用户场景要求更灵活——用户应该能把游戏"安装"到本地，离线运行，不依赖云服务器，LLM 也可以是自己的。这催生了两个截然不同的软件形态。

### 4.1 两个产品：CW 和 GW

| | **CW（创作工作站）** | **GW（游戏坊）** |
|---|---|---|
| 目标用户 | 游戏设计师、开发者 | 普通玩家 |
| 核心功能 | 世界书编辑、角色卡导入、游戏包打包、提示词测试、WE 引擎完整访问 | 游戏安装、游玩、存档管理、发现社区游戏 |
| 后端 | WE Go 二进制（本地运行）| WE Go 二进制（本地运行 or 连云端） |
| 数据库 | SQLite（本地文件）| SQLite（本地文件）|
| 网络依赖 | 仅 LLM API 调用 | 仅 LLM API 调用（+ 可选云同步）|
| 类比 | ST 的本地创作环境 + WE 引擎 | 像 Steam/itch.io 的游戏客户端 |

**关键认知转变**：WE 不是"服务端引擎"，而是一个**可以本地运行的轻量引擎**。它的服务端能力（多用户、云存档、社区分享）是可选的叠加，不是必须的前提。

---

### 4.2 WE Go 二进制的本地部署可行性

WE 的技术栈天然适合本地部署：

```
WE 在本地运行需要什么？

WE Go 二进制（单文件，ARM64 约 20-30MB）
    └── SQLite 文件（存档 + 记忆 + 游戏包，初始几 KB）
    
外部依赖：
    └── LLM API 调用（HTTPS 请求，可用用户自己的 key 或反向代理）
    
不需要：
    ✗ PostgreSQL 服务进程
    ✗ Nginx / 反向代理
    ✗ Docker
    ✗ Node.js
    ✗ 任何其他进程
```

**数据库层的切换**：WE 目前使用 PostgreSQL，但 GORM 支持 SQLite 只需换驱动（`gorm.io/driver/sqlite`），业务代码零改动。工程方案：

```go
// 通过启动参数选择数据库驱动
// WE 二进制支持 --db=sqlite:./gw.db 或 --db=postgres://...
// 两个模式共用同一套 Model 定义
```

本地模式用 `--db=sqlite`，云端模式用 `--db=postgres`。这是需要在 Phase 4 之前做的一个基础工作（加 SQLite 驱动 + 启动参数解析，约 20 行）。

---

### 4.3 Android 本地部署方案（修订）

#### 选项 D：嵌入式本地后端（推荐，本地优先场景）

```
GW Android APK（Capacitor 打包）
├── WebView
│   └── Vue 3 前端（随 APK 打包的静态文件）
├── Go 后端服务（ARM64 二进制，作为后台 Service 启动）
│   ├── WE 引擎（本地 HTTP API，监听 localhost:8080）
│   └── SQLite 文件（应用私有存储，无需权限）
└── 对外：LLM API 调用（用户 Key / 平台 Key / 本地 Ollama）
```

**工作原理：**
1. App 启动时，Capacitor 插件（或 Android Service）启动 Go 二进制作为后台进程，监听 `localhost:8080`
2. WebView 向 `http://localhost:8080` 发送 REST/SSE 请求（App 内通信，无需网络权限）
3. 只有 LLM API 调用需要出网
4. App 关闭时，停止 Go 进程，SQLite 文件保留

**LLM 接入的三种模式：**

| 模式 | 实现 | 用户门槛 |
|------|------|---------|
| 平台 Key（GW 提供）| 连接 GW 云端，有限免费额度 + 付费 | 零门槛，开箱即用 |
| 用户自己的 API Key | 在 App 内输入，本地 AES-256 加密存储（4-A 方案） | 需要有 API Key |
| 自建反向代理 | 用户填入自定义 Base URL，完全绕过 GW 平台 | 面向高级用户 |
| 本地 LLM（Ollama）| Base URL 指向局域网内的 Ollama 实例 | 需要额外设备跑模型 |

**"完全本地"场景：**
```
Android GW App
└── WE Go（localhost:8080）
    └── SQLite 文件
    └── LLM: http://192.168.1.100:11434（局域网内的 Ollama）
    
→ 零云端依赖，零流量费用，离线可用
```

**Go 交叉编译到 Android：**
Go 原生支持 `GOOS=android GOARCH=arm64` 交叉编译，输出标准 ELF 二进制。Capacitor 可以通过 Android `ProcessBuilder` 启动它，或者用 `gomobile bind` 将 WE 打包为 `.aar` 库（更干净，但需要适配 gomobile 的接口约束）。最简单的起步方案：先用 `ProcessBuilder` 启动裸二进制，后续再迁移到 `.aar`。

---

### 4.4 三种部署模式的完整对照

```
模式 A：纯本地（GW App 嵌入 WE）
┌─────────────────────────────┐
│  Android/Desktop            │
│  ├── WE Go（localhost）      │
│  ├── SQLite 文件             │
│  └── Vue 3 WebView           │
└─────────────────────────────┘
              ↓ 只有这一条出网
         LLM API（用户 Key）
         
适合：离线游玩、本地收藏、不信任云端的用户


模式 B：本地 + 云同步（主流用户）
┌─────────────────────────────┐      ┌──────────────────┐
│  Android GW App             │ ←──→ │  GW 云端（可选）  │
│  ├── WE Go（localhost）      │      │  存档同步         │
│  ├── SQLite 文件             │      │  社区游戏发现     │
│  └── Vue 3 WebView           │      │  平台 Key 池      │
└─────────────────────────────┘      └──────────────────┘
              ↓
         LLM API（平台 Key 或用户 Key）
         
适合：想要多设备同步 + 社区功能，但本地保留存档


模式 C：纯云端（开发者/测试环境）
客户端浏览器 ←→ WE Go（服务器）+ PostgreSQL
（对应原来第三节描述的模式，适合部署 GW 平台服务）
```

---

### 4.5 CW 软件的设计

CW 是 WE 对创作者的打包形态，目标是让游戏设计师不需要懂 Go / CLI 就能使用 WE 的全部能力。

```
CW 桌面应用（Windows / Mac / Linux）
├── WE Go 二进制（随 CW 打包）
├── SQLite（或可选连接远程 PostgreSQL）
└── 创作前端（Vue 3，比 GW 前端多出创作工具面板）
    ├── 世界书编辑器
    ├── 角色卡导入（PNG 拖放）
    ├── 游戏包打包 / 解包
    ├── Prompt Preview（dry-run 查看完整 prompt）
    ├── 变量沙箱调试
    └── 游戏测试（内置 GW 游玩界面）
```

**打包方案：Tauri**
- Tauri 用 Rust 做壳，WebView 跑 Vue 3 前端，可以将 WE Go 二进制作为 sidecar 进程启动
- 输出 `.exe` / `.dmg` / `.AppImage`，安装包约 20-30MB（比 Electron 小 10x）
- Tauri 的 `sidecar` 功能专门设计用来管理外部可执行文件的生命周期

**GW 和 CW 共用同一个 WE 引擎**，区别只在前端界面：CW 多了创作工具，GW 只有游玩界面。游戏包（`.game-package.json`）是两者之间的交换格式——在 CW 里制作，在 GW 里游玩。

---

### 4.6 软件设计核心原则（修订）

原来第 3.6 节的原则是云端优先的。修订后：

1. **本地优先，云端可选**：WE 引擎默认跑在用户设备上，云端是锦上添花（存档同步、社区发现），不是前提

2. **LLM 是唯一的网络依赖**：游戏逻辑、存档、记忆全部本地，只有 LLM API 调用出网；高级用户可以用自己的 Key 或本地 Ollama，完全切断云端依赖

3. **游戏包作为分发格式**：游戏不是"在云端服务器上的"，而是可以下载、安装、离线运行的——类似 itch.io 的独立游戏，而不是网页游戏

4. **两套前端，一套引擎**：CW（创作）和 GW（游玩）共用 WE Go 二进制，前端是薄层，不含业务逻辑

5. **SQLite for local，PostgreSQL for cloud**：引擎通过 `--db` 参数在两种模式间切换，业务代码不感知；云端 GW 平台用 PostgreSQL 保证并发；本地 CW/GW 用 SQLite 保证零依赖

6. **断线续传和重连**：即使在本地模式下，LLM API 调用可能失败；客户端 SSE 实现自动重连，从 `GET /sessions/:id/state` 恢复 UI，不依赖内存状态

---

## 附：一句话总结（修订版）

**集成层**：先用裸 fetch，Phase 4-D swaggo 完成后 `openapi-typescript` 生成类型，封装 `@gw/api-client` + `@gw/play-helpers` 两层。主消费方是 GW/CW 自己的前端。

**部署模型**：WE 是一个可以本地运行的轻量引擎（Go 二进制 + SQLite，无其他依赖），不是必须部署在服务器上的。GW 是本地优先的游玩客户端，CW 是本地优先的创作工具，两者都嵌入 WE 引擎，只有 LLM 调用需要出网。云端 GW 平台是可选的叠加层，用于存档同步和社区发现，不是游玩的必要条件。
