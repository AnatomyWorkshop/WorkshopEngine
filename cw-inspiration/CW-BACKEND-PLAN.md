# CW 后端计划：Creation Workshop

> 编写于 2026-04-11，更新于 2026-04-11（补充解包分析、resolveSlot 债务、CW/WE 解耦判断）
> 性质：功能畅想 + 现阶段首要任务（对齐游戏解包/铺开/创作/打包）

---

## 一、CW 是什么

CW（Creation Workshop）是 GW 生态的**内容生产层**。
GW 是消费端（玩家游玩），CW 是生产端（创作者制作游戏包）。

当前 CW 以 `internal/creation/` 包的形式存在于 `backend-v2` 中，与 GW 共享同一个服务和数据库。
这在 MVP 阶段合理，长期需要解耦（见 `gw-inspriation/2026-04-11-architecture-outlook.md` 第八节）。

---

## 二、CW 的功能全景（畅想）

### 2.1 核心创作能力（已有基础）

| 功能 | 当前状态 | 说明 |
|------|----------|------|
| 角色卡 CRUD | ✅ 已有 | `POST /api/create/cards/` |
| PNG 导入（CCv2/CCv3）| ✅ 已有 | `card/parser.go` + `POST /api/create/cards/import` |
| 世界书词条 CRUD | ✅ 已有 | `WorldbookEntry` 表 |
| Preset 配置 CRUD | ✅ 已有 | `PresetEntry` 表 |
| LLM Profile 管理 | ✅ 已有 | `LLMProfile` + `LLMProfileBinding` |
| 素材库 | ✅ 已有 | `Material` 表 + `/api/game-assets/:slug` |
| 游戏模板 CRUD | ✅ 已有 | `GameTemplate` 表 |
| 游戏包 JSON 导出 | ✅ 已有 | `GET /api/create/templates/:id/export` |
| 游戏包 JSON 导入 | ✅ 已有 | `POST /api/create/templates/import` |
| PNG 打包工具 | ✅ 已完成 | `cmd/pack-card`（写入 `gw_game` tEXt chunk）|
| PNG 导入扩展（gw_game）| ✅ 已完成 | `POST /api/create/cards/import` 支持 `gw_game` keyword |

### 2.2 需要新增的核心能力

#### A. 游戏包解包与重制工具

**ST 格式解析扩展**
- 当前 `card/parser.go` 解析 CCv2/CCv3 角色卡
- 扩展：解析 ST 预设文件（`prompt_order` + `prompts` 数组）→ 提取 system prompt 内容
- 输出：`PresetEntry[]`，可直接导入 GW 游戏模板（`POST /api/create/templates/:id/preset/import-st` 已有）

**宏转换工具**
- 将 ST 宏（`{{setvar}}`/`{{random}}`/`{{roll}}`）转换为 GW 等价原语或静态文本
- 不支持的宏（`<<taskjs>>`）直接丢弃并报告

#### B. 游戏模板版本控制

- `GameTemplate` 增加 `version`/`draft`/`published` 状态
- `POST /api/create/templates/:id/publish` — 发布草稿
- `POST /api/create/templates/:id/rollback/:version` — 回滚版本
- `GET /api/create/templates/:id/history` — 版本历史

**为什么需要：** 创作者在调试游戏时会频繁修改世界书/预设，需要能回滚到上一个可用版本。

#### C. AI 辅助创作

- `POST /api/create/assist/worldbook-entry` — 给定游戏背景，AI 生成世界书词条草稿
- `POST /api/create/assist/system-prompt` — 给定角色设定，AI 生成 system prompt 草稿
- `POST /api/create/assist/first-mes` — AI 生成开场白
- `POST /api/create/assist/variables` — 给定游戏类型，AI 建议初始变量结构

**实现方式：** 直接调用 `platform/provider`（LLM 调用层），不走游戏引擎。

#### D. 素材仓库增强

- 素材标签自动提取（上传时 AI 分析图片/文本，自动打标签）
- 素材语义检索（pgvector，Material 量 > 100 时引入）
- 素材包导入/导出（将一组素材打包为 `.zip`，跨实例迁移）

#### E. 预设模板库（Template Repository）

- 内置预设模板（文字冒险/视觉小说/养成模拟/角色扮演）
- `GET /api/create/template-presets` — 列出内置模板
- `POST /api/create/templates/from-preset/:preset_id` — 从模板创建游戏

**为什么需要：** 降低创作门槛，新手不需要从零配置 `PresetEntry`/`WorldbookEntry`。

#### F. 轻社交 / 创作交流（长期）

- 游戏包公开分享（创作者发布到公共库，附带创作说明）
- 创作者主页（展示已发布游戏、素材包）
- 游戏包 Fork（基于他人游戏包创建自己的版本）
- 创作讨论区（针对游戏包的技术讨论，区别于 GW 的玩家评论）

**与 GW 社交的区别：** GW 社交是玩家视角（游记/评论），CW 社交是创作者视角（技术讨论/素材共享）。

---

## 三、打包核心过程与 CW 复刻

### 3.1 打包流程（已实现）

```
game.json（GameTemplate 导出格式）+ cover.png
  ↓ cmd/pack-card
  ↓ base64(game.json) → tEXt chunk（keyword = "gw_game"）
  ↓ 插入 PNG IEND chunk 之前
  → output.png（可分发）
```

**`cmd/pack-card` 的核心逻辑**（`cmd/pack-card/main.go`）：
1. 读取 `game.json` → base64 编码
2. 读取封面 PNG → 找到 IEND chunk 位置
3. 构造 `tEXt` chunk（keyword=`gw_game`，value=base64 JSON，附 CRC32）
4. 在 IEND 之前插入，写出

**将来复刻进 CW 的方式：**

CLI 工具（`cmd/pack-card`）是给创作者本地使用的。CW 后端服务化后，打包逻辑会以 API 形式暴露：

```
POST /api/create/templates/:id/pack-png
  Body: { "cover_url": "..." }  ← 或 multipart 上传封面
  Response: PNG 文件流（Content-Type: image/png）
```

核心 PNG chunk 写入逻辑（`injectTextChunk` + `makeChunk` + `findIEND`）会从 `cmd/pack-card/main.go` 提取到 `internal/creation/card/packer.go`，供 CLI 和 API 共用。

**现阶段不做 API 化**，原因：
- 打包是低频操作（创作者手动触发），CLI 足够
- API 化需要处理封面图存储（当前素材库只有 URL，不存二进制）
- 等 CW 有前端 UI 时再做 API，避免过早抽象

### 3.2 解包流程（已实现）

```
output.png（gw_game keyword）
  ↓ POST /api/create/cards/import（multipart）
  ↓ card.ParseGWGamePNG → 提取 base64 → JSON
  ↓ importGamePackage（事务）→ 写入 DB
  → { type: "gw_game", template_id, slug, title }
```

ST 角色卡（`chara`/`ccv3` keyword）走原有路径，返回 `CharacterCard`。

---

## 四、现阶段首要任务：Victoria 卡完整流程

### 4.1 原始素材位置

```
.data/public/Brain-like/text/
  Victoria          ← 原始 PNG 卡（实为文本文件，内含 CCv3 JSON）
  Victoria.png      ← 封面图
```

**注意**：`Victoria` 文件是 Unicode 文本（UTF-8 with CRLF），不是二进制 PNG。
这意味着它是**直接的 JSON 文本**，不是 PNG 格式的角色卡，无需 PNG 解包，直接读取 JSON 即可。

### 4.2 解包后的目标目录

解包文件放到 `.data/games/victoria/`：

```
.data/games/victoria/
  raw.json          ← 原始 CCv3 JSON（直接从 Victoria 文件复制）
  game.json         ← 整理后的 GW 游戏包格式（手动编辑）
  victoria-gw.png   ← 打包后的可分发 PNG（cmd/pack-card 输出）
```

封面图从 `.data/public/Brain-like/text/Victoria.png` 复制过来。

### 4.3 game.json 格式（GW 游戏包 Schema）

`game.json` 是 `GET /api/create/templates/:id/export` 的输出格式，也是 `POST /api/create/templates/import` 的输入格式：

```json
{
  "version": "1.0",
  "template": {
    "slug": "victoria",
    "title": "维多利亚",
    "type": "text",
    "short_desc": "蒸汽朋克工业都会生存",
    "description": "...",
    "system_prompt_template": "...",
    "config": {
      "first_mes": "...",
      "initial_variables": { "区域": "维多利亚城", "资金": 1000, "声望": 0 },
      "enabled_tools": ["set_variable", "get_variable"],
      "display_vars": ["区域", "资金", "声望"]
    }
  },
  "preset_entries": [...],
  "worldbook_entries": [...],
  "regex_profiles": [],
  "regex_rules": [],
  "materials": [],
  "preset_tools": []
}
```

**关键字段说明：**
- `config.initial_variables`：session 创建时写入 `GameSession.Variables`（引擎已支持）
- `config.enabled_tools`：引擎 Pipeline 中激活的工具（`set_variable`/`get_variable` 已实现）
- `config.display_vars`：前端 `TextSessionTopBar` 统计抽屉展示的变量名（Phase 3 前端工作）
- `worldbook_entries[].group`：互斥分组（引擎已支持，区域词条用此字段）
- `worldbook_entries[].position`：`before_template`/`after_template`/`at_depth`（引擎已支持）

### 4.4 打包命令

```bash
cd backend-v2
go run ./cmd/pack-card/ \
  -game ../.data/games/victoria/game.json \
  -cover ../.data/public/Brain-like/text/Victoria.png \
  -out ../.data/games/victoria/victoria-gw.png
```

---

## 五、resolveSlot / applyGenParams 技术债务

### ✅ 已完整修复（2026-04-11 确认）

`go build ./internal/engine/api/...` 编译通过，无错误。

**resolveSlot**（`game_loop.go:157`）：
```go
func (e *GameEngine) resolveSlot(sessionID, userID, slot string) (llm.Provider, llm.Options) {
    if e.registry != nil {
        if client, opts, ok := e.registry.ResolveForSlot(e.db, userID, sessionID, slot); ok {
            return client, opts
        }
    }
    return e.llmClient, llm.Options{MaxTokens: e.maxTokens}
}
```

**applyGenParams**（`game_loop.go:168`）：engine/api 包内有独立实现，操作本地 `GenParams` 类型，与 `platform/provider` 包的同名函数不冲突（各自服务不同类型）。

`cmd/worker` 仍有编译错误（`BatchSize`/`LeaseTTL` 字段变更 + `NewWorker` 签名变更），但这是 worker 包的独立问题，不影响主服务器和 creation 包。

---

## 六、CW 与 WE 引擎的解耦

### 6.1 现阶段：不需要解耦，共享服务合理

CW（`internal/creation/`）和 WE（`internal/engine/`）当前共享同一个 Go 服务和数据库。
**现阶段这是正确的选择**，原因：

1. **数据强耦合**：CW 写入 `GameTemplate`/`WorldbookEntry`/`PresetEntry`，WE 读取这些表。分离服务需要跨服务数据同步，复杂度远超收益。
2. **用户群体尚未分化**：内测阶段创作者和玩家可能是同一批人，没有必要分离认证和权限。
3. **流量规模不需要独立扩缩**：内测阶段两者流量都很小。

### 6.2 解耦的前置条件（中期工作）

解耦需要先修复以下技术债务（见 `gw-inspriation/2026-04-11-architecture-outlook.md` 第八节）：

| 债务 | 位置 | 说明 |
|------|------|------|
| `CharacterCard.GameID` 语义混乱 | `creation/api/routes.go:68` | 暂用 `CharacterCard.ID` 作为 `GameID`，语义不清 |
| `LLMProfile` 在 `models_creation.go` 但被 `platform/provider` import | `models_creation.go` | 依赖方向反了 |
| `creation/api` 直接用 `core/llm` 绕过 Provider 注册表 | `creation/api/routes.go` | 多 Provider 场景下会有问题 |

### 6.3 解耦的边界（长期目标）

```
CW 写入：GameTemplate / WorldbookEntry / PresetEntry / CharacterCard / Material
WE 只读：GameTemplate / WorldbookEntry / PresetEntry（通过 game_id 引用）
WE 私有：GameSession / Floor / Memory / MemoryEdge / PromptSnapshot / Variables
```

CW 不感知 `GameSession` 及其子结构，WE 不感知 `CharacterCard` 的创作细节。
两者通过 `GameTemplate.ID`（`game_id`）解耦：CW 生产游戏模板，WE 消费游戏模板。

**结论：现阶段不解耦，等用户规模和功能复杂度达到需要独立部署时再做。**

## 八、Victoria 卡完整性分析与 SillyTavern 表现力对齐

### 8.1 原卡的变量系统（MVU 格式）

Victoria 原卡是为 **MVU（Model-View-Update）系统**设计的，变量更新使用 **JSON Patch (RFC 6902) 扩展格式**：

```xml
<UpdateVariable>
<Analysis>（英文分析，≤80词）</Analysis>
<JSONPatch>
[
  { "op": "replace", "path": "/当前位置", "value": "东区枢纽" },
  { "op": "delta",   "path": "/金币",     "value": -5 },
  { "op": "insert",  "path": "/物品栏/蒸汽手枪", "value": {"描述":"...","数量":1} },
  { "op": "remove",  "path": "/状态/饥饿" }
]
</JSONPatch>
</UpdateVariable>
```

支持的操作：`replace`/`delta`（数值增减）/`insert`（新增键或数组追加）/`remove`/`move`

初始变量结构（来自 `[initvar]` 词条）：
```yaml
世界状态: { 存活天数: 1, 当前位置: 未知 }
玩家状态:
  资产: { 金币: 0, 银币: 0, 铜币: 0 }
  债务: {}
  势力声望: { 温莎: 0, 罗斯柴尔德: 0, 克拉伦斯: 0, 莫里亚蒂: 0, 瓦特: 0, 市政厅: 0 }
  物品栏: {}
  状态: {}  # 动态添加，如 { 饥饿: {描述:..., 严重程度: 轻微} }
```

### 8.2 GW 引擎当前的变量更新机制

引擎 `parser.go` 解析 `<UpdateState>{JSON}</UpdateState>` 标签，是**简单 JSON merge patch**（`extractStatePatch`）：
- 只支持 key-value 覆盖写入
- 不支持 `delta`（数值增减）
- 不支持 `insert`/`remove`/`move`
- 不支持嵌套路径（`/资产/金币`）

`set_variable`/`get_variable` 工具调用是另一条路径，LLM 通过 function calling 主动调用，但：
- 每次只能设置一个变量
- 不支持 delta 操作
- 对于 Victoria 这种复杂嵌套变量结构，需要多次工具调用

### 8.3 Gap 分析：GW 能否完整复现 Victoria 的表现力？

| 功能 | ST 原卡 | GW 当前 | Gap |
|------|---------|---------|-----|
| 变量初始化 | `[initvar]` 词条 | `config.initial_variables` ✅ | 无 |
| 变量读取 | `{{getvar::key}}` 宏 | `get_variable` 工具 ✅ | 无（路径不同但等价）|
| 变量写入（简单替换）| `<UpdateVariable>` JSONPatch replace | `set_variable` 工具 ✅ | 无 |
| 变量写入（数值增减）| JSONPatch `delta` op | ❌ 不支持 | **需要后端扩展** |
| 变量写入（嵌套路径）| JSONPatch `/资产/金币` | ❌ 不支持 | **需要后端扩展** |
| 变量写入（动态新增键）| JSONPatch `insert` | ❌ 不支持 | **需要后端扩展** |
| 变量写入（删除键）| JSONPatch `remove` | ❌ 不支持 | **需要后端扩展** |
| 状态展示 | `{{format_message_variable}}` ST宏 | `display_vars` + 前端 StatusBar（Phase 3）| 前端待做 |
| 角色创建流程 | `[角色创建协议]` 词条（已保留）| 世界书词条触发 ✅ | 无 |
| 世界书词条触发 | key 匹配 | ✅ 已实现 | 无 |
| 区域互斥分组 | `group` 字段 | ✅ 已实现 | 无 |
| 深度插入 | `at_depth` | ✅ 已实现 | 无 |

### 8.4 是否需要摘取预设打包游戏？

**Victoria 原卡没有独立的 ST 预设文件**（Ave Mujica 那种 `prompt_order` + `prompts` 格式）。
它的所有"预设"逻辑都内嵌在世界书词条里（`[mvu_update]`/`[initvar]`/`[角色创建协议]`）。

当前 `game.json` 的 `preset_entries: []` 是正确的——不需要摘取预设。
世界书词条已经包含了所有格式指令（44 条，去掉了 7 条 ST 专用词条）。

**但有一个问题**：原卡的 `[mvu_update]` 词条（已被排除）要求 LLM 输出 `<UpdateVariable><JSONPatch>` 格式，而 GW 引擎解析的是 `<UpdateState>` 格式。

**两种解决方案：**

**方案 A（推荐，短期）**：在 `game.json` 的 `preset_entries` 里加一条格式指令，告诉 LLM 用 GW 的 `<UpdateState>` 格式输出变量更新，并限制为简单 key-value（不用嵌套路径）。变量结构也相应简化（扁平化）。

**方案 B（完整，中期）**：后端引擎扩展 `<UpdateVariable><JSONPatch>` 解析，支持 `delta`/`insert`/`remove` 操作，完整复现原卡的表现力。

### 8.5 现阶段可以开始整理吗？

**可以，但需要做一个决策**：用简化变量结构（方案 A）还是等后端扩展（方案 B）。

**推荐方案 A 先跑通**：
1. 将 `initial_variables` 扁平化（去掉嵌套，改为 `金币: 0`/`温莎声望: 0` 等）
2. 在 `preset_entries` 加一条格式指令词条，指导 LLM 用 `<UpdateState>{"金币": -5}</UpdateState>` 格式
3. 打包后可以立即游玩，验证世界书词条触发、角色创建流程等核心功能
4. 后端扩展 JSONPatch 后，再升级到完整变量结构

**方案 B 的后端工作量**：
- `parser.go` 扩展：识别 `<UpdateVariable><JSONPatch>` 标签
- 实现 `delta`/`insert`/`remove`/`move` 操作（作用于嵌套 JSON 路径）
- `PatchSessionVariables` 支持 RFC 6902 风格操作
- 这是中等工作量，不阻塞当前游玩验证

---

| 任务 | 优先级 | 状态 | 说明 |
|------|--------|------|------|
| `cmd/pack-card` CLI 工具 | P0 | ✅ 已完成 | `cmd/pack-card/main.go` |
| `POST /api/create/cards/import` 扩展 `gw_game` | P0 | ✅ 已完成 | `importGamePackage` 共享函数 |
| `resolveSlot`/`applyGenParams` 修复 | P0 | ✅ 已完成 | `engine/api` 编译通过 |
| Victoria 卡解包 → `raw_card.json` | P0 | ✅ 已完成 | 从 Victoria.png 提取 CCv3 JSON |
| Victoria 卡整理 → `game.json`（嵌套变量版）| P0 | ✅ 已完成 | 44 条世界书词条，含 initial_variables |
| Victoria 卡打包 → `victoria-gw.png` | P0 | ✅ 已完成 | `cmd/pack-card` 执行，1.2MB |
| Victoria 卡 preset_entries 格式指令（方案 A）| **P1** | 🔜 待做 | 加一条 `<UpdateState>` 格式指令，使卡可立即游玩 |
| `cmd/worker` 编译错误修复 | P1 | 🔜 待做 | `BatchSize`/`LeaseTTL`/`NewWorker` 签名变更 |
| 后端 JSONPatch 扩展（方案 B）| P2 | 🔜 待做 | `delta`/`insert`/`remove` 操作，完整复现原卡 |
| 游戏包 JSON Schema 文档 | P2 | 🔜 待做 | 见第四节 |
| ST 预设解析扩展（Ave Mujica 等）| P2 | 待做 | 需要先有目标格式 |
| `cmd/pack-card` → `internal/creation/card/packer.go` 提取 | P3 | 待做 | CW API 化前置 |
| AI 辅助创作接口 | P3 | 待做 | 降低创作门槛 |
| 游戏模板版本控制 | P3 | 待做 | 创作者调试需求 |
| CW/WE 解耦前置条件修复 | 中期 | 待做 | 见第六节 |
| CW 独立服务（`cmd/cw-server`）| 长期 | 待做 | 需要用户规模支撑 |
| 轻社交功能 | 长期 | 待做 | 依赖 Phase 3 登录 |
