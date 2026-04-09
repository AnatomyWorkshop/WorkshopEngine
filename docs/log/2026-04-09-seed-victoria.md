# 2026-04-09 — seed 脚本 + Victoria 游戏包

## 背景

数据库结构尚未稳定（计划大改），当前阶段需要一种临时的游戏数据导入方式，用于内测前的端到端验证。

## 决策

**不在后端写正式的 seed 机制**，而是写一个临时的 `cmd/seed/main.go` 脚本：

- 读取 `.data/games/*/game.json`（项目根目录下的内测数据包）
- 直接写入 PostgreSQL（通过 GORM，复用现有 DB 模型）
- 幂等：已存在的 slug 跳过；`--force` 参数强制重新导入
- 不引入新的 API 端点，不修改现有 DB 模型

**为什么是临时的**：
- DB 结构（GameTemplate / WorldbookEntry / PresetEntry）在常驻角色 Phase 2 时会大改
- 正式的游戏导入应该通过 CW（创作者工作台）完成，CW 尚未实现
- seed 脚本只服务于内测期间的数据准备，之后可以删除或重写

## 文件

- `backend-v2/cmd/seed/main.go` — seed 脚本
- `.data/games/victoria/game.json` — Victoria 游戏包（第一个内测游戏）

## Victoria 游戏包说明

来源：`.data/public/Brain-like/text/Victoria.png`（ST chara_card_v3 格式，PNG tEXt chunk 内嵌 base64 JSON）

提取内容：
- `first_mes`：原始开场白（965字，五大区域选择引导）
- `worldbook_entries`：44 条（从 51 条中过滤掉 7 条 MVU/变量脚本条目）
  - 12 条 constant（区域规则、世界总纲等常驻注入）
  - 32 条 keyword-triggered（事件、NPC、势力详情）
- `system_prompt`：重新编写（原卡 system_prompt 字段为空，逻辑在 ST 预设里）

过滤掉的条目类型（ST 专属，WE 不需要）：
- MVU 前端格式指令（`<StatusPlaceHolderImpl/>` 注入）
- `[mvu_update]` 变量输出格式（ST 的 JS 变量渲染）
- `[initvar]` 变量初始化（ST 的 STscript 变量系统）
- 变量列表 schema（Zod schema，JS 专属）

## 预设文件分析

`.data/public/Brain-like/preset/` 下有两个预设：
- `Izumi 0401.json`：泉此方通用写作预设（203 条 prompt），不适合直接用于 Victoria
- `【小猫之神】3.10.json`：小猫之神角色预设，与 Victoria 无关

**结论**：两个预设均为通用写作辅助，不包含 Victoria 专属内容。Victoria 的叙事风格已通过 `system_prompt` 重新编写，不需要引入外部预设。

## 已知缺口（内测前需要在 WE 补充）

1. `Config.initial_variables`：CreateSession 时自动写入初始变量（如玩家选择的区域、初始资产）
   - 当前绕过方案：system_prompt 中描述初始状态，不用变量宏
   - 优先级：P1

2. `regen` 支持对最后一条 committed 楼层重新生成
   - 当前只支持 `status=generating/failed` 的楼层
   - 优先级：P1

3. 静态文件服务（封面图）
   - 当前绕过方案：封面图放在 `GameWorkshop/public/covers/`，前端直接访问
   - 优先级：P2
