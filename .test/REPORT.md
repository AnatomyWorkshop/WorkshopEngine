# Test Report — backend-v2

更新时间：2026-04-08（社交层完成，测试全部迁移至 .test/）

## 结构

```
backend-v2/
└── .test/
    ├── go.mod                   # module mvu-backend/test（共享 internal 访问权）
    ├── go.sum
    ├── REPORT.md
    ├── run.sh                   # 一键运行
    ├── unit_test.go             # 单元测试（无网络 / 无 DB）
    ├── integration_llm_test.go  # LLM 集成测试（需 LLM_API_KEY）
    ├── integration_social_test.go # Social DB 集成测试（需 TEST_DSN）
    ├── data/
    └── preset_tool_echo/
```

注：原散落在各包目录下的 `*_test.go` 文件已全部删除，统一在此管理。

---

## 运行

```bash
cd backend-v2/.test

# 单元测试（无需任何外部依赖）
go test -v -count=1

# LLM 集成测试
LLM_API_KEY=sk-... go test -tags integration -v -run LLM

# Social DB 集成测试
TEST_DSN="host=localhost user=postgres password=postgres dbname=gw_test sslmode=disable" \
  go test -tags integration -v -run Social
```

---

## 单元测试覆盖（unit_test.go）

| # | 测试组 | 测试数 | 描述 |
|---|---|---|---|
| 1 | Tokenizer | 8 | Estimate / EstimateMessages，含 CJK 精确计数、边界 clamp |
| 2 | Parser | 7 | XML / 编号列表 / 圆圈数字 / VN game_response / fallback |
| 3 | Variable Sandbox | 4 | 作用域优先级、Flatten 副本隔离 |
| 4 | Scheduled Triggers | 4 | 阈值 / 冷却 / 嵌套变量路径 |
| 5 | Regex Processor | 6 | ai_output / user_input / all / capture group / chained / flags |
| 6 | Worldbook 基础 | 11 | 命中 / 未命中 / 常量 / 大小写 / 正则 / whole_word / secondary / scan_depth / 递归 / disabled |
| 7 | Worldbook GroupCap | 4 | 互斥分组权重裁剪 |
| 8 | Worldbook VarGate | 11 | var:= / var:!= / var 存在性 / 无文本触发 / 数值比较 / 组合逻辑 |
| 9 | Pipeline Runner | 3 | system prompt / worldbook 注入 / user message 最后 |
| 10 | LLM Client | 3 | DiscoverModels / TestConnection 无效 URL / BaseURL 裁剪 |
| 11 | Tokenizer 补充 | 3 | PureCJK 计数 / SingleChar clamp / Messages(nil) |
| 12 | Forum HotScore | 4 | 零分 / 活跃度对比 / 时间衰减 / 公式验证 |
| 13 | Forum RenderContent | 2 | Markdown 渲染 / XSS 净化 |
| **合计** | | **70** | **全部通过** |

---

## 集成测试（LLM）

| 测试 | 说明 | 依赖 |
|---|---|---|
| `TestIntegration_LLMChat` | 真实 LLM 调用，验证响应非空 | LLM_API_KEY |
| `TestIntegration_ParserWithLLMOutput` | LLM 输出 → Parser 解析 | LLM_API_KEY |
| `TestIntegration_PipelineWithWorldbook` | Pipeline + Worldbook 注入 → LLM | LLM_API_KEY |

---

## 集成测试（Social DB）

| 测试 | 说明 | 依赖 |
|---|---|---|
| `TestSocial_Reaction_AddRemove` | 点赞 / 重复 / 取消 / 重复取消 | TEST_DSN |
| `TestSocial_Reaction_CountBatch` | 批量计数 + 零值 | TEST_DSN |
| `TestSocial_Reaction_CheckMine` | 个人状态查询 | TEST_DSN |
| `TestSocial_Comment_CreateAndTree` | 主楼 → 回复 → 嵌套树 → CountByGame | TEST_DSN |
| `TestSocial_Comment_Edit_And_Delete` | 5分钟编辑 + 权限 + 软删除 | TEST_DSN |
| `TestSocial_Forum_CreateAndList` | 发帖 + game_tag 过滤 + 计数 | TEST_DSN |
| `TestSocial_Forum_Reply` | 楼层序号 + 楼中楼 + RepliesCount | TEST_DSN |

---

## 模块设计说明

`.test/go.mod` 使用 `module mvu-backend/test`（而非 `mvu-backend-test`），
这样模块路径以 `mvu-backend/` 为前缀，满足 Go 的 `internal` 包访问规则，
可直接 import `mvu-backend/internal/...` 中的任意包。

`replace mvu-backend => ../` 指向父目录的主模块，`go mod tidy` 自动同步所有依赖版本。
