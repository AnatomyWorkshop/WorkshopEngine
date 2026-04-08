# Test Report — backend-v2 Engine

更新时间：2026-04-08（Phase 3 全部完成 M11–M14）

## 目录结构

```
backend-v2/
├── .test/
│   ├── .env                    # LLM 配置（不提交 git）
│   ├── go.mod                  # 独立模块（replace mvu-backend => ../）
│   ├── run.sh                  # 一键运行全部测试
│   ├── REPORT.md               # 本报告
│   ├── unit_test.go            # 黑盒单元测试（package mvu_test，无网络依赖）
│   ├── data/                   # 测试数据
│   └── preset_tool_echo/       # Preset Tool HTTP 回调测试服务器
internal/integration/
└── llm_test.go                 # 集成测试（需 -tags integration，需 LLM API Key）
```

## 运行方式

```bash
# 单元测试（无需 API Key）
cd .test && go test ./... -v -count=1

# 全量测试（需 .test/.env 配置 API Key）
bash .test/run.sh
```

## 如何运行

```bash
# 单元测试（无需网络）
cd backend-v2
go test ./testutil/... -v -count=1

# 集成测试（需要 DeepSeek API Key）
set -a && source .test/.env && set +a
go test -tags integration ./internal/integration/... -v -timeout 120s

# 一键全跑
bash .test/run.sh
```

## 单元测试结果（2026-04-06）

| # | 测试组 | 测试数 | 结果 |
|---|---|---|---|
| 1 | Tokenizer | 5 | ✅ PASS |
| 2 | Parser | 7 | ✅ PASS |
| 3 | Variable Sandbox | 4 | ✅ PASS |
| 4 | Scheduled Triggers | 4 | ✅ PASS |
| 5 | Regex Processor | 6 | ✅ PASS |
| 6 | Worldbook 基础匹配 | 11 | ✅ PASS |
| 7 | Worldbook 互斥分组 (3-C) | 4 | ✅ PASS |
| 8 | Worldbook `var:` 门控 (3-D) | 10 | ✅ PASS |
| 9 | Pipeline Runner | 3 | ✅ PASS |
| 10 | LLM Discovery/Test (3-B) | 3 | ✅ PASS |
| **合计** | | **57** | **全部通过** |

```
ok  mvu-backend/testutil  0.770s
```

## 集成测试结果（DeepSeek API）

| 测试 | 描述 | 结果 |
|---|---|---|
| `TestIntegration_LLMChat` | 验证 LLM 客户端可到达 DeepSeek | ✅ PASS (~1.6s) |
| `TestIntegration_ParserWithLLMOutput` | 真实 LLM 输出 → Parser 解析 | ✅ PASS (~3.6s) |
| `TestIntegration_PipelineWithWorldbook` | Pipeline 注入 Worldbook → LLM 响应 | ✅ PASS (~2.3s) |

## Phase 3 新增测试覆盖

### 3-B — LLM 模型发现 + 连通性测试
- `TestLLM_DiscoverModels_InvalidURL_ReturnsError`：无效 URL 返回错误（不 panic）
- `TestLLM_TestConnection_InvalidURL_ReturnsError`：无效 URL 返回错误（不 panic）
- `TestLLM_NewClient_BaseURL_Trim`：BaseURL 末尾斜杠自动裁剪

### 3-C — Worldbook 互斥分组（GroupCap）
- `TestWorldbook_GroupCap_KeepsHighestWeight`：同组只保留权重最高的词条
- `TestWorldbook_GroupCap_Ungrouped_NotAffected`：无组词条不受裁剪影响
- `TestWorldbook_GroupCap_MultipleGroups_Independent`：多组独立裁剪
- `TestWorldbook_GroupCap_Cap2`：cap=2 时保留权重最高的两条

### 3-D — Worldbook `var:` 变量门控
- `TestWorldbook_VarGate_Equals_Hit`：`var:stage=confrontation` 变量匹配激活
- `TestWorldbook_VarGate_Equals_Miss`：变量不匹配不激活
- `TestWorldbook_VarGate_Equals_MissingVar`：变量不存在不激活
- `TestWorldbook_VarGate_NotEquals_Hit/Miss`：`var:stage!=investigation` 不等条件
- `TestWorldbook_VarGate_Exists_Hit/Miss`：`var:boss_defeated` 存在性条件
- `TestWorldbook_VarGate_NoTextRequired`：门控与扫描文本无关
- `TestWorldbook_VarGate_TextMatchDoesNotActivate`：**关键**：文本出现门控词但变量不匹配 → 不激活（证明这是引擎层硬门控，非软匹配）
- `TestWorldbook_VarGate_NumericValue`：整数变量自动转字符串比较（`3` == `"3"`）
- `TestWorldbook_VarGate_MixedWithRegularKey`：门控关键词作为主键，次级关键词组合

## 测试结构变更（2026-04-06）

**重构原因：**
Go 的 `internal/` 包访问限制要求测试文件位于同一 module 内。`.test/` 目录虽有 `replace mvu-backend => ../` 指令，但作为独立 module 仍无法访问 `internal` 包。

**重构方案：**
- 将 `unit_test.go` 从 `.test/` 移动至 `backend-v2/testutil/`（主模块内）
- 测试包名 `mvu_test`（黑盒外部测试风格），不依赖被测包的未导出符号
- `.test/` 保留 `preset_tool_echo`（独立 HTTP 工具）和运行脚本

**原来在各包目录下的测试文件已全部删除**，统一由 `testutil/unit_test.go` 覆盖：
- ~~`internal/engine/parser/parser_test.go`~~
- ~~`internal/engine/pipeline/worldbook_test.go`~~
- ~~`internal/engine/processor/regex_test.go`~~
- ~~`internal/engine/scheduled/trigger_test.go`~~
- ~~`internal/engine/variable/sandbox_test.go`~~
- ~~`internal/core/tokenizer/estimate_test.go`~~

## 历史 Bug 记录（原 REPORT.md）

### Bug 1：`GetFloat` 冷却键查找失败

**位置：** `internal/engine/scheduled/trigger.go`

**问题：** 冷却键 `__sched.r1.last_floor` 中的点号被错误地当作路径分隔符处理，导致冷却逻辑永远失效。

**修复：** `GetFloat` 开头新增字面量精确匹配，命中则直接返回，否则再按点号分割递归查找。

### Bug 2：圆圈数字选项测试格式错误

**位置：** `internal/engine/parser`（原测试文件）

**问题 + 修复：** `①选项一` 格式有误，Parser 要求分隔符（`①.选项一`）。已在 `testutil/unit_test.go` 中修正。

### Bug 3：VN game_response 测试格式错误

**位置：** `internal/engine/parser`（原测试文件）

**问题 + 修复：** VN 指令格式有误（应为 `[bg|...]` 而非 `BG|...`）。已在 `testutil/unit_test.go` 中修正。

---

## 说明

- `testutil/unit_test.go` 是黑盒测试：只用导出 API，不依赖内部实现细节
- `.test/.env` 包含真实 API 密钥，已被 `.gitignore` 排除，不提交
- 集成测试用 `//go:build integration` 标记，`go test ./...` 不触发
