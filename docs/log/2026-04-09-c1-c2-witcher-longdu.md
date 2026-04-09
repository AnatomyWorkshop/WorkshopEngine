# 2026-04-09 — C1/C2 WE 补丁 + 猎魔人世界 + 让我听听你的心

## 背景

内测前需要完成两个 WE 功能缺口（C1/C2），并将两张新卡改编为 WE 游戏包。

## C1：regen 支持 committed 楼层（Swipe 语义）

**问题**：`regen` 路径只查找 `status=generating/failed` 的楼层，玩家在正常对话后点击重生成会报 `no active floor to regen`。

**修改**：
- `backend-v2/internal/engine/api/game_loop.go`（PlayTurn regen 路径）
- `backend-v2/internal/engine/api/engine_methods.go`（StreamTurn regen 路径）

**逻辑**：先找 `generating/failed` 楼层（优先级高），若无则找最后一条 `committed` 楼层。这与 ST 的 Swipe 语义一致：对已提交的 AI 回复重新生成一个新页面。

## C2：Config.initial_variables 支持

**问题**：`CreateSession` 时 `sess.Variables` 硬编码为 `{}`，游戏包无法声明初始变量（如好奇度、区域等）。

**修改**：`backend-v2/internal/engine/api/engine_methods.go`（CreateSession）

**逻辑**：解析 `Config.initial_variables`（`map[string]any`），若非空则序列化为 JSON 写入 `sess.Variables`。

**用法**（game.json）：
```json
{
  "config": {
    "initial_variables": {
      "好奇度": 5,
      "感谢度": 0,
      "自我攻略度": 0
    }
  }
}
```

## 新游戏包

### 猎魔人世界（witcher-world）

来源：`.data/public/Brain-like/text/9af3e96e2e99b3d3.png`（ST chara_card_v3）

- 156 条世界书词条（全部关键词触发，1 条 constant 叙事哲学）
- 原卡无 system_prompt / first_mes（纯世界书参考卡）
- 重新编写 system_prompt（猎魔人叙事规则）和 first_mes（凯尔莫罕要塞开场）
- 目标：`.data/games/witcher-world/game.json`

### 让我听听你的心（longdu）

来源：`.data/public/Brain-like/text/Image_1775674380514_530.png`（ST chara_card_v3）

- 12 条 constant 世界书词条，保留 10 条（过滤 2 条禁用的 HTML 状态栏）
- description 字段包含完整系统逻辑（因果织网者协议），直接用作 system_prompt
- 清理：移除 `<thinking>` 内部 CoT 块，修正 `{{uesr}}` typo
- 添加 WE 输出格式指令（`<Options>` 标签）
- initial_variables：`{好奇度: 5, 感谢度: 0, 自我攻略度: 0, 因果等级: 1, 暴走百分比: 0}`
- 目标：`.data/games/longdu/game.json`

## 关于是否延后内测

不需要延后。Victoria 不使用变量，C1/C2 对 Victoria 的端到端验证不是阻塞项。
两个补丁已完成，现在三张卡（Victoria / 猎魔人世界 / 让我听听你的心）均可通过 seed 脚本导入。
