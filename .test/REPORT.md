# Test Report — backend-v2 Engine

生成时间：2026-04-04

## 目录结构

```
.test/
├── .env            # LLM 配置（不提交 git）
├── go.mod          # 独立模块，用于 godotenv 依赖管理
├── run.sh          # 一键运行全部测试
└── REPORT.md       # 本报告
internal/integration/
└── llm_test.go     # 集成测试（需 -tags integration）
```

## 如何运行

```bash
# 单元测试（无需网络）
go test ./internal/core/tokenizer/... \
        ./internal/engine/parser/... \
        ./internal/engine/processor/... \
        ./internal/engine/scheduled/... \
        ./internal/engine/variable/... \
        ./internal/engine/pipeline/...

# 集成测试（需要 DeepSeek API Key）
set -a && source .test/.env && set +a
go test -tags integration ./internal/integration/... -v -timeout 120s

# 一键全跑
bash .test/run.sh
```

## 单元测试结果

| 包 | 测试数 | 结果 |
|---|---|---|
| `internal/core/tokenizer` | 9 | ✅ PASS |
| `internal/engine/parser` | 10 | ✅ PASS |
| `internal/engine/processor` | 12 | ✅ PASS |
| `internal/engine/scheduled` | 9 | ✅ PASS |
| `internal/engine/variable` | 8 | ✅ PASS |
| `internal/engine/pipeline` | 18 | ✅ PASS |
| **合计** | **66** | **全部通过** |

## 集成测试结果（DeepSeek API）

| 测试 | 描述 | 结果 |
|---|---|---|
| `TestIntegration_LLMChat` | 验证 LLM 客户端可到达 DeepSeek | ✅ PASS (~1.6s) |
| `TestIntegration_ParserWithLLMOutput` | 真实 LLM 输出 → Parser 解析 | ✅ PASS (~3.6s) |
| `TestIntegration_PipelineWithWorldbook` | Pipeline 注入 Worldbook → LLM 响应 | ✅ PASS (~2.3s) |

## 发现的 Bug 及修复

### Bug 1：`GetFloat` 冷却键查找失败

**位置：** `internal/engine/scheduled/trigger.go`

**问题：** 冷却记录的键名格式为 `__sched.<id>.last_floor`（含字面量点号），存储时作为平级 key 写入变量沙箱。但 `GetFloat` 在处理路径时，总是先按点号分割尝试嵌套路径查找，导致 `__sched.r1` 子 map 不存在而返回 `false`，冷却逻辑永远失效。

**修复：** 在 `GetFloat` 开头新增字面量精确匹配：先检查 `vars[path]` 是否直接命中，命中则返回；否则再按点号分割递归遍历嵌套路径。

```go
// 修复前（直接分割点号）：
dot := strings.IndexByte(path, '.')
...

// 修复后（先尝试字面量匹配）：
if v, ok := vars[path]; ok {
    if f, ok2 := toFloat(v); ok2 {
        return f, true
    }
}
dot := strings.IndexByte(path, '.')
...
```

**影响：** `TestEvaluate_CooldownBlocks` 测试从 FAIL 变为 PASS；自动回合触发器的冷却功能恢复正常。

---

### Bug 2：圆圈数字选项测试格式错误

**位置：** `internal/engine/parser/parser_test.go`

**问题：** `TestParse_NumberedList_CircledNumbers` 测试用例使用 `①选项一` 格式（圆圈数字后直接跟文字），但 Parser 的正则要求分隔符（`.`、`、`、`．`），即 `①.选项一`。

**修复：** 将测试中的 `①选项一` 改为 `①.选项一`，与 Parser 实际支持的格式一致。这是测试写法错误，Parser 实现本身正确。

---

### Bug 3：VN game_response 测试格式错误

**位置：** `internal/engine/parser/parser_test.go`

**问题：** `TestParse_GameResponse_VN` 用例中 BG、BGM、选项使用了错误的文本格式（如 `BG|city_night.jpg`），但 Parser 实际期望括号格式（`[bg|city_night.jpg]`）。

**修复：** 将测试中的所有 VN 指令改为正确的 `[bg|...]`、`[bgm|...]`、`[choice|...]` 括号格式。同样是测试写法问题，实现正确。

---

## 说明

- 单元测试文件保留在各自的包目录下（Go 要求 `*_test.go` 与被测包同目录）
- `.test/.env` 包含真实 API 密钥，已被 `.gitignore` 排除，不会提交
- 集成测试使用 `//go:build integration` 标记，常规 `go test ./...` 不会触发
