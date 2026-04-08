# 2026-04-07 异世界和平游戏可玩性修复

## 背景

目标：通过 WE 引擎完整驱动「异世界和平 v0.21」游戏。

---

## 修复列表

### 1. SSE 流式 `done` 事件缺失（race condition）

**位置**：`internal/engine/api/routes.go`

**根因**：`StreamTurn` goroutine 中 defer 顺序为 LIFO：
```
defer close(tokenCh)  // 最后关闭
defer close(metaCh)   // 倒数第二
defer close(errCh)    // 最先关闭
```
`errCh` 先于 `tokenCh` 关闭。`c.Stream()` 的 select 同时监听 `tokenCh` 和 `errCh`，
Go runtime 随机选择已就绪的 channel。当 `errCh` 被 close 时（ok=false, err=nil），
select 可能选中 errCh case，以 `err=nil` 返回 `false`，绕过 meta/done 发送逻辑。

**修复**：对 `errCh` case 改为感知 `ok` 状态：
```go
case err, ok := <-errCh:
    if !ok {
        return true  // 关闭但无错误，继续等 tokenCh
    }
    ...
```

### 2. SSE 流式调用超时（http.Client.Timeout 影响流体读取）

**位置**：`internal/core/llm/client.go`

**根因**：`http.Client{Timeout: 60s}` 对整个响应体读取计时。流式生成超过 60s 会被中断。

**修复**：为 `Client` 添加 `streamHTTPClient`（Timeout=0），流式调用（`ChatStream`）
使用该客户端，依赖 request context 取消而非全局超时。

### 3. 世界书 249KB 全量注入导致 GLM-4-plus prompt 超长

**位置**：`GameTemplate.Config.worldbook_token_budget`

**根因**：游戏模板 Config 为 null，`WorldbookTokenBudget=0` 触发无限制注入，
26 个 `Constant=true` 词条 + 被递归激活的词条共 249KB 被注入。

**修复**：PATCH 游戏模板 Config 设置 `worldbook_token_budget: 3000`。
`node_worldbook.go` 的 `applyTokenBudget()` 将非常驻词条裁剪至 3000 tokens 以内。
实际注入从 249KB → 41KB。

### 4. first_mes 未注入新会话

**位置**：`internal/engine/api/engine_methods.go`（`CreateSession`）

**根因**：游戏模板 Config 为 null，`extractFirstMes()` 返回空字符串，
CreateSession 不写 floor 0。

**修复**：PATCH 游戏模板 Config 补入 `first_mes`（4531 字符的开场 `<game_response>` 剧情）。
`CreateSession` 正确读取并写入 seq=0 的已提交楼层。

### 5. 前端历史加载错位（first_mes 与普通楼层配对逻辑）

**位置**：`test-plan/from-traeCN/src/views/GamePlay.vue`

**根因**：`recent_history` 以 assistant 消息（first_mes）开头，
但前端按 user/assistant 严格两两配对，导致 first_mes 被当作用户消息显示，
后续所有消息均错位。

**修复**：检测历史首条消息 role，若为 assistant 则单独渲染为无用户输入楼层，
再从 index 1 开始正常配对；用户消息为空时隐藏用户气泡。

### 6. 消除硬编码（去魔法数字）

**位置**：多处

| 原硬编码 | 修复方式 |
|---|---|
| `llm.NewClient(..., 60, 2)` in engine_methods/game_loop | 改用 `e.llmTimeoutSec`, `e.llmMaxRetries` |
| `const maxToolIter = 5` in engine_methods/game_loop | 改用 `e.maxToolIter` |
| `extractClientTimeouts()` 返回静态 `60, 2` | 通过 `Client.TimeoutSec()` / `Client.MaxRetries()` getter 读取 |
| `NewGameEngine()` 参数不含超时/重试/maxToolIter | 扩展参数签名 |
| `config.go` 无 `MaxToolIter` 字段 | 添加 `LLM_MAX_TOOL_ITER`（默认 5） |
| `Client` struct 无公开 getter | 添加 `TimeoutSec()`, `MaxRetries()` |

---

## .env 当前配置（截至本次）

```env
LLM_BASE_URL=https://open.bigmodel.cn/api/paas/v4
LLM_API_KEY=ba18fd2263f0482691709c9aa3f7a122.8iU4mBAHa7V8EkJ1
LLM_MODEL=glm-4-plus
LLM_MAX_TOKENS=2048
LLM_TOKEN_BUDGET=2000
LLM_MAX_HISTORY_FLOORS=4
DATABASE_URL=host=localhost user=postgres password=130633 dbname=game_workshop sslmode=disable
PORT=8080
CORS_ORIGINS=http://localhost:5174,http://localhost:5173,http://localhost:3000
ALLOW_ANONYMOUS=true
```

新增可选 env：`LLM_MAX_TOOL_ITER`（默认 5）

---

## 游戏模板状态

- ID: `d1c7d763-bf32-4a5b-b83f-79e8e8605cc6`
- Slug: `isekai-peace-v021-imported-20260407183937`
- Config 已包含：`first_mes`（4531 chars）、`worldbook_token_budget: 3000`、`worldbook_group_cap: 20`
- 前端：`http://localhost:3000`
- 后端：`http://localhost:8080`

---

## 已知遗留问题

- Izumi 0401.json 的 `[main]` 系统提示词含 ST 宏（`{{getvar::...}}`、`{{lastUserMessage}}` 等），
  与 WE 不兼容，尚未翻译接入。Regex 规则（14 条已启用）已导入。
- `LLM_TOKEN_BUDGET=2000` 是 pipeline 全局预算（非 worldbook 专项），
  目前设置偏小，可视情况调大。

---

## 补充分析：视觉渲染缺失的根因与 TH 对比（2026-04-08）

### 问题现象

游戏回合文字流转正常（SSE 流、done/meta 事件均正确）。
meta 事件携带完整结构化 VN 指令：
- `vn.bgm`：BGM 文件名（如 `祈祷`）
- `vn.bg`：背景文件名（如 `贵族宅邸·华丽卧室`）
- `vn.cg`：CG 文件名（如 `赤熊三明治`）
- `vn.lines[]`：解析后的对话行（speaker、sprite、text）
- `vn.options[]`：玩家选项

但前端仅显示纯文本（旁白斜体 + 对话配对），没有任何图像或音频渲染。

### 根因拆解

#### 1. 前端是文本 VN 渲染器（GamePlay.vue）

`GamePlay.vue` 的 `parseVN()` 函数解析了 `game_response` 格式，
但所有视觉指令只用 emoji 文字表示：
- `[bg|X]` → 显示为 `🏞 X`（文字标签）
- `[bgm|X]` → 显示为 `🎵 X`（文字标签）
- `[cg|X]` → 显示为 `📷 X`（文字标签）
没有任何 `<img>` 标签或 CSS 背景渲染。

#### 2. 素材文件不存在

数据库中有 50 条 `Material` 记录，但：
- `url` 字段全部为空（只有描述文字）
- 实际的精灵图 PNG / 背景图 JPG / BGM MP3 **从未上传**到 WE 后端
- 这些文件存在于原始 ST 游戏包的本地磁盘目录中，未迁移

游戏的资产清单存储在世界书常驻词条中：
- `sprite_list`：78 个立绘名（如 `莉莉亚常服`、`露娜玛丽亚常服`）
- `scene_list`：40+ 背景名（如 `贵族宅邸·华丽卧室`、`神殿·洒满阳光的大厅`）
- `cg_list`：8 个 CG 名（如 `赤熊三明治`、`白色水晶`）
- `bgm_list`：若干 BGM 曲目名

这些只是提示词文本，不是可查询的资产目录。

#### 3. 无资产 URL 解析 API

目前没有 API 能将 `莉莉亚常服` 映射为 `/uploads/isekai/sprites/莉莉亚常服.png`。
前端无法知道任何文件名对应的实际 URL。

---

### TH / ST 的对比实现

**SillyTavern（桌面端）**
- 角色卡 `data.extensions.tav.world` 字段指向本地目录
- 立绘文件存放在 `ST/data/default-user/worlds/<卡名>/sprites/` 下
- 前端（浏览器）直接用相对路径加载 `<img>` 标签
- BGM 用 `<audio>` + Web Audio API 播放本地文件
- 无"名字→URL"映射问题：文件名即 URL（本地服务器路径）

**TavernHeadless（无头 API）**
- 同样假设资产文件部署在服务器某个固定静态目录下
- `GET /api/v2/assets/{gameId}/{type}/{name}` 返回文件流或重定向
- 前端用 `<img src="/api/v2/assets/...">` 直接引用
- 静态文件通过专用 CDN/nginx 或者内置 Static 路由提供

**WE 需要做的事（完整 VN 渲染能力）**

分两层：后端资产管理 + 前端 VN 渲染引擎。

---

### 修复路径：分阶段实现

#### Stage A（短期，2-4天）：内容可视化 + 占位渲染

目标：不需要真实素材文件，在当前文字模式基础上增加**结构感**。

1. **前端改造（GamePlay.vue → VN Text Mode）**
   - 对话框底部固定，显示当前场景名 + BGM 名（文字标签）
   - 人物立绘显示彩色圆形头像占位符（用立绘名首字）
   - 选项按钮改为底部固定的分组按钮
   - 流式输出时的打字机效果（逐字显示而非一次性追加）

2. **后端：资产目录 API**
   - `GET /api/v2/play/games/:id/assets` 返回 `{sprites: {}, scenes: {}, bgms: {}, cgs: {}}`
   - 目前所有 URL 为空，前端知道"此名无对应文件"即可降级显示

#### Stage B（中期，后端资产系统）：真实素材上传与服务

目标：支持游戏包附带真实素材文件，前端渲染实际图像。

1. **Material 模型增强**
   - 新增 `filename` 字段（资产文件名，如 `莉莉亚常服.png`）
   - 新增 `asset_type` 字段（`sprite` / `scene` / `bgm` / `cg`）
   - `url` 字段在文件上传后填充（本地存储路径或 CDN URL）

2. **批量上传 API**
   - `POST /api/v2/assets/:game_id/upload-pack`：接收 zip 包，解压后批量写入 Material
   - 或：`POST /api/v2/assets/:game_id/files`：逐文件上传

3. **资产 URL 解析 API**
   - `GET /api/v2/play/games/:id/assets`：返回完整的 `{sprites: {"莉莉亚常服": "/uploads/...png"}, ...}`

#### Stage C（中期，前端 VN 引擎）：完整视觉小说渲染

目标：与 ST 游玩体验对等。

1. **背景层**：CSS `background-image` + 淡入淡出过渡，`[bg|X]` 触发切换
2. **立绘层**：2-4 个绝对定位的 `<img>` 元素，支持 shake/jump 动画（CSS `@keyframes`）
3. **CG 层**：全屏叠加，点击或 `[hide_cg]` 隐藏
4. **BGM 层**：`<audio>` 循环播放，切换时淡出当前曲目再淡入新曲目
5. **对话框**：固定底部，打字机效果，支持流式 token 追加
6. **选项层**：`[choice|...]` 触发后覆盖输入框，点选即发送

数据来源：
- **VN 指令**来自 SSE `meta` 事件的 `vn` 字段（结构化，已有）
- **素材 URL** 来自 Stage B 的 `GET /games/:id/assets` API 缓存

---

### 现状总结

| 能力 | ST | TH | WE 现状 |
|---|---|---|---|
| VN 文本输出 | ✅ | ✅ | ✅ 正常 |
| 结构化 VN 指令（meta 事件）| ✅ | ✅ | ✅ 已有 |
| 素材文件服务 | ✅ 本地文件 | ✅ 静态路由 | ❌ 无 URL |
| 名字→URL 映射 API | ✅ 隐式 | ✅ REST | ❌ 缺失 |
| 背景图渲染 | ✅ CSS | ✅ 前端 | ❌ 文字标签 |
| 立绘渲染 | ✅ 定位 img | ✅ 前端 | ❌ 文字标签 |
| BGM 播放 | ✅ Audio | ✅ 前端 | ❌ 文字标签 |
| CG 渲染 | ✅ 全屏覆盖 | ✅ 前端 | ❌ 文字标签 |
| 打字机效果 | ✅ | ✅ | ❌（追加渲染） |
| 选项按钮 | ✅ 底部覆盖 | ✅ 前端 | ⚠️ 顶部列表 |
