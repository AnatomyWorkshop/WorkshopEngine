# 创作层设计思路

> 状态：2026-04-05

---

## 核心分层原则

WE 引擎分两个完全独立的层，对应两个独立的客户端：

```
游玩层（Play）          →  手机 App
  internal/engine/       游戏运行时（Session/Floor/Memory/Tools）
  internal/platform/     LLM Provider / slot 系统

创作层（Creation）      →  PC 软件
  internal/creation/     游戏打包工具（预设/角色卡/正则/素材/模板）
```

**解耦原则**：
- 游玩层只读 `game_templates` 及其关联表（WorldbookEntry、PresetEntry、RegexProfile、Material、PresetTool），不写
- 创作层只写这些表，不涉及 Session/Floor/Memory 等运行时状态
- 两层共享同一个 PostgreSQL 数据库，但可以部署在不同进程甚至不同机器上
- 手机 App 只需要连接游玩层 API（`/api/v2/play`），PC 软件只需要连接创作层 API（`/api/v2/create`）

这个分离**完全可以实现**，因为 `internal/creation` 和 `internal/engine` 已经是独立包，没有互相导入。

---

## 游玩层（手机 App）

手机 App 的职责：

```
游玩  →  POST /play/sessions/:id/turn
流式  →  GET  /play/sessions/:id/stream（SSE）
分享  →  POST /play/sessions/:id/archive（边界归档，待实现）
交流  →  论坛 API（social 层，独立）
```

手机端不需要知道游戏是怎么做的，只需要：
1. 选择一个游戏（`GET /play/sessions` 列出可用游戏）
2. 创建会话并游玩
3. 在结局/精彩时刻归档分享

**token 控制**：手机端每回合 token 固定可控，滚动摘要压缩保证不膨胀。

---

## 创作层（PC 软件）

PC 软件是游戏设计师的工作台，从零开始完成一个游戏的全部配置，然后"打包"发布。

### 当前已有的创作层能力

```
internal/creation/
  api/        HTTP 路由（/api/v2/create）
  card/       角色卡 PNG 解析（CCv2/v3）
  lorebook/   世界书（已有 CRUD）
  template/   游戏模板（已有 CRUD）
  asset/      素材管理（目录已建，待实现）
```

### 创作流程（从零开始）

```
1. 导入角色卡 PNG（可选）
   POST /create/cards/import
   → 解析 tEXt chunk → CharacterCard 入库
   → 自动提取 description/personality/scenario → 写入 PresetEntry（is_system_prompt=true）

2. 配置预设条目
   POST /create/templates/:id/preset-entries
   → 每条 PresetEntry 对应一段 system prompt
   → injection_order 决定排列顺序
   → 可从 ST 预设 JSON 批量导入（待实现：import-st 端点）

3. 编写世界书
   POST /create/templates/:id/lorebook
   → 关键词触发的背景知识
   → constant=true 的条目无条件常驻

4. 配置正则规则
   POST /create/llm-profiles（RegexProfile + RegexRule）
   → user_input / ai_output / all 三个阶段
   → 用于格式化、过滤、替换

5. 上传素材
   POST /create/game-assets（待实现）
   → 文本素材（对话片段、氛围描写、事件）
   → 图片/音频资源（URL 映射）

6. 注册自定义工具
   POST /create/templates/:id/tools
   → HTTP 回调工具（Preset Tool）
   → 创作者自己的服务处理游戏逻辑

7. 配置 LLM Profile
   POST /create/llm-profiles
   POST /create/llm-profiles/:id/activate（绑定到 slot）
   → narrator 槽：主叙述模型
   → director 槽：廉价预分析模型（可选）
   → verifier 槽：后置校验模型（待实现）

8. 打包发布
   PATCH /create/templates/:id  { "status": "published" }
   → 游戏对手机端可见
```

### 待实现的创作层功能

**ST 预设导入**（`POST /create/templates/:id/preset/import-st`）

接收 ST 预设 JSON，转换规则：
- 取 `prompt_order[character_id=100000].order` 的数组下标 × 10 作为 `injection_order`
- 丢弃 8 个 marker 条目（`chatHistory`、`worldInfoBefore` 等，WE 用 Pipeline 节点替代）
- 普通条目批量 Upsert 到 `PresetEntry`
- 可选：把采样参数（temperature、top_p 等）写入 LLMProfile.Params

**素材导入**（`POST /create/game-assets/import`）

支持批量导入文本素材，自动打标签（mood/style/function_tag）。

**游戏打包导出**（`GET /create/templates/:id/export`）

把一个 GameTemplate 及其所有关联数据（PresetEntry、WorldbookEntry、RegexProfile、Material、PresetTool、CharacterCard）序列化为单个 JSON 文件，可以：
- 分享给其他创作者
- 导入到另一个 WE 实例
- 在本地离线游玩

**游戏解包**（`POST /create/templates/import`）

接收导出的 JSON，重建所有关联数据。这是"把游戏带回创作层重制"的关键操作。

---

## 解耦验证

当前 `internal/creation` 和 `internal/engine` 的依赖关系：

```
internal/creation/api/routes.go
  → internal/core/db（共享数据库模型）
  → internal/creation/card（角色卡解析）
  → internal/platform/auth（鉴权）
  ✗ 不导入 internal/engine 任何包

internal/engine/api/game_loop.go
  → internal/core/db（共享数据库模型）
  → internal/engine/pipeline、memory、session、tools 等
  ✗ 不导入 internal/creation 任何包
```

两层唯一的共享点是 `internal/core/db`（数据库模型），这是正确的——它们操作同一份数据，但通过不同的 API 入口。

**部署方案**：

```
方案 A：单进程（当前）
  cmd/server/main.go 同时注册 /play 和 /create 路由
  手机 App 和 PC 软件连同一个服务

方案 B：双进程（未来）
  cmd/play-server/main.go   只注册 /play 路由，部署到轻量云服务器
  cmd/create-server/main.go 只注册 /create 路由，部署到 PC 本地或私有服务器
  共享同一个 PostgreSQL 数据库
```

方案 B 只需要拆分 `main.go` 的路由注册，不需要改动任何业务代码。

---

## Verifier 槽的优先级判断

**先完善创作层，Verifier 槽稍后。**

原因：
- 创作层的打包/解包/ST 导入是手机 App 能跑起来的前提——没有游戏可以发布，游玩层就是空的
- Verifier 是"后置校验 + 重试"，需要定义校验失败的处理策略（重试几次？降级输出？报错？），逻辑比 Director 复杂

Verifier 槽的最小实现方案（备忘）：
```
1. resolveSlot("verifier") 检查是否绑定
2. 主生成完成后，把 ParsedResponse + 原始 llmMsgs 发给 verifier 模型
3. verifier 返回 {"pass": true} 或 {"pass": false, "reason": "..."}
4. 失败时：最多重试 N 次（N 在 GameTemplate.Config 配置），超限则用最后一次结果
5. verifier 失败不阻断回合（静默降级）
```
约 40 行，等创作层基础稳定后再做。
