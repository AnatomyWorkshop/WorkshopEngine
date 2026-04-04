# MVM 渲染协议设计备忘

> 编写于 2026-04-04，作为工坊论坛多媒体渲染、游戏剪辑分享和互动游玩的设计思路记录。
> 本文档是**设计意图**，不是已实现功能的说明。

---

## 一、什么是 MVM

MVM (Model-View-Msg) 是一种**内容渲染架构模式**，将内容分成三层：

```
MODEL  ── 类型 Schema（这段内容是什么结构）
MSG    ── 数据实例（LLM 或编辑器产生的具体内容）
VIEW   ── 渲染规则（在哪个平台、以什么方式呈现 MSG）
```

与 JSON 的关系：JSON 是序列化格式（低层），MVM 是内容组织模式（高层）。
与我们现有架构的关系：`parser.go` 的 `ParsedResponse` 已经是简化版 MVM MSG；
`VNDirectives` 是 VN 场景的 MODEL；`ParseMode` 决定用哪条 VIEW 渲染链。

---

## 二、平台内 MODEL 定义

以下是我们平台需要支持的内容类型（MODEL）：

### 2.1 叙事块 NarrativeBlock（当前已实现）

```
{
  narrative: string        — 给玩家阅读的叙事文本
  options:   string[]      — 选项按钮（空=自由输入）
  state_patch: map[any]    — 变量更新（写入沙箱）
  summary:   string        — 异步写入记忆摘要
  parse_mode: string       — 调试：使用了哪条解析路径
}
```

### 2.2 视觉小说场景 VNScene（当前已实现）

```
{
  bg:      string          — 背景图文件名
  bgm:     string          — 背景音乐文件名
  cg:      string          — CG 图
  sprites: SpriteAction[]  — 立绘动作（show/shake/jump）
  lines:   DialogueLine[]  — 对话行（speaker|sprite|text）
  choices: string[]        — 选项（来自 [choice|...] 标签）
}
```

### 2.3 音效事件 AudioEvent（待实现）

```
{
  type: "bgm" | "sfx" | "voice"
  file: string
  loop: bool
  volume: float
  fade_in: int  // ms
}
```

### 2.4 状态通知 StateNotice（待实现）

```
{
  type:    "item_gain" | "hp_change" | "quest_update" | ...
  payload: map[any]
  display: string  // 给玩家展示的描述文本
}
```

### 2.5 剪辑帧 ClipFrame（待实现）

```
{
  floor_id:   string
  page_id:    string
  seq:        int
  model_type: "vn_scene" | "narrative_block" | ...
  data:       MSG           — 该楼层的解析结果
  thumbnail:  string        — 首帧图（用于论坛卡片预览）
  created_at: time
}
```

---

## 三、VIEW 渲染规则链

同一段 MSG 在不同场景应用不同 VIEW：

```
vn-full      ─→  narrative  ─→  minimal  ─→  pure-text
（完整VN渲染）   （文字+按钮）   （仅文字）    （无格式纯文本）
```

### 3.1 按场景选择 VIEW

| 场景 | VIEW 规则 | 说明 |
|------|-----------|------|
| 游戏主界面 | `vn-full` 或 `narrative` | 由游戏 type 决定 |
| 论坛嵌入卡片 | `minimal` | 只展示叙事文本 + 缩略图，不播放音效 |
| 剪辑分享页 | `narrative` 或 `minimal` | 支持回放但不交互 |
| 纯文本导出 | `pure-text` | 去除所有格式标记 |
| 访问者预览 | `minimal` + "开始游玩" 按钮 | 可 fork 会话进入交互 |

### 3.2 降级渲染规则

当客户端不支持更高级 VIEW 时，自动降级：

```
if client.supports("vn-full"):
    render VNScene
elif client.supports("narrative"):
    render VNScene.lines → joined narrative text + VNScene.choices
elif client.supports("minimal"):
    render narrative text only, no choices
else:
    pure text
```

`parser.go` 的 `ParseMode` 字段已经在做这件事的 LLM 侧版本，
VIEW 降级是它的前端镜像。

---

## 四、剪辑分享（Clip）设计

### 4.1 什么是剪辑

剪辑 = 一个会话中若干连续楼层的快照序列，携带每楼层的 MSG 数据。

```
Clip {
  id:          string
  session_id:  string
  game_id:     string
  title:       string
  frames:      ClipFrame[]  — 按 seq 排序的楼层快照
  cover:       string        — 封面图（第一帧或手动选择）
  created_by:  string
  created_at:  time
}
```

### 4.2 剪辑生成流程

```
1. 用户在游戏界面选择"分享这段剧情"（选择起始/结束楼层）
2. 后端 POST /play/sessions/:id/clips { from_floor, to_floor }
3. 后端读取 floors[from..to] 的 active page messages
4. 每个 floor 的 assistant 消息通过 parser.Parse() 重新结构化为 ClipFrame
5. Clip 写入 DB，返回 clip_id
6. 前端论坛发帖时引用 clip_id，渲染为嵌入卡片（minimal VIEW）
```

### 4.3 剪辑播放

剪辑播放 = 按 seq 顺序逐帧渲染 ClipFrame，VIEW 为 `narrative`（或 `vn-full`）。
用户点击"开始游玩"按钮：

```
1. POST /play/sessions  { game_id, fork_from_clip_id }
2. 后端创建新 Session，将 Clip 的前 N 帧历史注入为已提交的 Floor
3. 返回新 session_id，前端进入正常游玩流程
```

---

## 五、论坛多媒体渲染

工坊论坛帖子支持嵌入游戏内容：

### 5.1 内容块类型（对应 forum post body）

```
PostBlock = 
  | TextBlock    { content: string }            — 普通富文本
  | ClipBlock    { clip_id, view_mode }          — 嵌入剪辑
  | ImageBlock   { url, caption }                — 图片
  | CardBlock    { character_slug }              — 角色卡预览
  | GameBlock    { game_id, preview_turn_count } — 游戏预览（前N回合）
```

### 5.2 渲染优先级

论坛帖子渲染时，根据客户端能力选择 VIEW：
- 桌面全功能客户端：ClipBlock → `narrative` VIEW with playback controls
- 移动端：ClipBlock → `minimal` VIEW, 图片懒加载
- RSS/爬虫：ClipBlock → `pure-text`，ImageBlock → alt 文本

---

## 六、互动游玩与分支

### 6.1 选项驱动的 Turn 触发

`NarrativeBlock.options[]` 在 VIEW 中渲染为按钮，每个按钮点击触发：

```
POST /play/sessions/:id/turn { user_input: option_text }
```

前端不需要知道这是"选了一个选项"还是"手动输入了文字"，
后端接口统一。

### 6.2 变量条件选项（待实现）

在 PresetEntry 或 parser 结果中支持条件选项：

```xml
<Options>
  <option if="hp > 0">继续战斗</option>
  <option if="hp <= 0">求饶</option>
  <option always>逃跑</option>
</Options>
```

变量条件求值在 `parser.go` 中完成，依赖 `variable.Sandbox.Flatten()` 注入。

### 6.3 会话分叉（Fork）

从某个楼层开始新的分支游玩（"如果当时选了另一个选项"）：

```
POST /play/sessions/:id/fork { from_floor_seq: int }
→ 创建新 Session，复制 floors[0..N] 的历史，然后可以从第 N+1 楼开始新走向
```

---

## 七、与 TH 的对比

TH 没有专门的"MVM 协议"——TH 的协议是 OpenAPI schema + JSON，
通过 LLM profile 的 `post_processing_rules` 做内容转换。

我们的 MVM 路径更接近自研游戏引擎的做法：
- LLM 输出 → parser 分层 → MSG（ParsedResponse）→ VIEW（前端按 ParseMode 选渲染路径）
- 这比 TH 的"把所有输出当作 string 再后处理"更早分流，减少前端的解析负担

**待与 from-mvm 协议对接的地方：**
- `from-mvm` 的 `===MODEL===` / `===MSG===` 段对应我们的 GameConfig.PresetEntries + TurnResponse
- `.mvm` 文件的 typed section 可以作为 GameTemplate 的可选 schema 层
- 将来如果游戏设计师需要严格 schema 约束，可以引入 `game_schema.mvm` 作为 GameTemplate.Config 的扩展

---

## 八、实现路线图

| 功能 | 优先级 | 依赖 |
|------|--------|------|
| Clip 模型 + 生成 API | 中 | Floor/Page 已完成 |
| 论坛 ClipBlock 嵌入渲染 | 中 | Clip API |
| 变量条件选项 | 中 | variable.Sandbox |
| 会话分叉 (Fork) | 低 | Session 复制逻辑 |
| `.mvm` schema 层 | 低 | 需要 from-mvm 协议稳定 |
