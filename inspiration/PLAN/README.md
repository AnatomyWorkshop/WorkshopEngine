# WorkshopEngine — PLAN 目录说明

> 版本：2026-04-08

本目录包含 WorkshopEngine（WE）的所有计划与参照文档。

---

## 文档列表

### [P-WE-OVERVIEW.md](./P-WE-OVERVIEW.md)

**整体开发计划**（源自 `implementation-plan.md` + `PROGRESS.md` 合并整理）

编号规则：`P-<阶段><序号>`，例：`P-1A`（Phase 1，第 A 项）

| 阶段 | 内容 | 状态 |
|------|------|------|
| Phase 1 | 核心基础设施（M1–M10 已完成，含 M11 CharacterInjectionNode）| ✅ |
| Phase 2 | 工具 + 多槽 LLM + 创作层 | ✅（并入 Phase 1）|
| Phase 3 | 引擎能力补全（M11–M13 全部完成）| ✅ |
| Phase 4 | 安全 + 平台工程（全部待做）| 📋 |
| Phase 5 | 集成包 + 社区层 + 架构治理（全部待做）| 📋 |

**用途：** 接手新开发者时的第一读物；规划下一个迭代时的任务来源。

---

### [P-WE-PROGRESS.md](./P-WE-PROGRESS.md)

**里程碑进度记录**（源自 `PROGRESS.md` 迁移，2026-04-08）

系统定位 + 包结构总览 + 完整 API 端点速查表 + M1–M13 每个里程碑的目标与完成内容。

**用途：** 快速了解当前实现状态；查询某个功能属于哪个里程碑；查阅可用 API 端点。

---

### [P-TH-CODE-MAP.md](./P-TH-CODE-MAP.md)

**TavernHeadless → WorkshopEngine 代码位置对照**

逐一列出 TH 每个核心功能的源码路径，并注明 WE 的对应实现位置。

| 章节 | 内容 |
|------|------|
| 一 | 代码库结构对比（包层次）|
| 二 | 消息层级（Session / Floor / Page）|
| 三 | Prompt Pipeline（提示词组装）|
| 四 | 变量系统 |
| 五 | 记忆系统 |
| 六 | LLM 调度 |
| 七 | 工具调用 |
| 八 | 回合编排（Turn Orchestration）|
| 九 | Runtime Substrate（运行时底层）|
| 十 | SillyTavern 兼容（适配器）|
| 十一 | DB Schema 迁移对照 |
| 十二 | 官方集成包（Integration Kit）|
| 十三 | WE 独有功能（TH 无对应）|

**用途：** 借鉴 TH 某个功能时，直接定位源文件；避免盲目猜测路径。

---

## 快速导航

```
正在做什么？  → P-WE-OVERVIEW.md（当前阶段 Phase 4 起）
历史里程碑？  → P-WE-PROGRESS.md（M1–M13 完成情况 + API 端点速查）
借鉴 TH 实现？ → P-TH-CODE-MAP.md（按功能模块查找 TH 文件）
架构决策？    → ../../docs/architecture.md（设计原则 + ADR 记录）
```

---

## TH 代码库位置

```
D:\ai-game-workshop\plagiarism-and-secret\TavernHeadless\
  packages/
    core/src/            ← 引擎核心（无框架依赖）
    adapters-sillytavern/src/  ← ST 兼容适配器
    official-integration-kit/  ← @tavern/sdk + @tavern/client-helpers
    shared/src/          ← 自动生成的 OpenAPI 类型
  apps/
    api/src/             ← Fastify HTTP 层 + DB（Drizzle）
    web/src/             ← 创作工作台前端
```
