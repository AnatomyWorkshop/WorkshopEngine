# 第八条：嵌套变量 / JSONPatch 问题的深度分析与方案

> 日期：2026-04-11
> 触发：CW-BACKEND-PLAN.md 第八节 — Victoria 卡变量系统 Gap 分析

---

## 一、问题的本质

Victoria 原卡使用 **嵌套 JSON + JSONPatch 风格操作**（`delta`/`insert`/`remove`/`move`）来管理游戏状态：

```json
{ "op": "delta", "path": "/玩家状态/资产/金币", "value": -5 }
```

GW 引擎当前的 `<UpdateState>` 解析是**简单 JSON merge patch**（`map[string]any` 覆盖写入），不支持嵌套路径和增量操作。

这是一个**真实的表达力 Gap**，但不是紧急问题。

---

## 二、TH 是怎么做的？

**TH 完全不用嵌套变量。**

TH 的变量系统是**扁平 key-value 存储**，有 scope 层级（page → floor → branch → chat → global），但每个变量都是独立的 `key: value` 对：

```typescript
// TH 的 set_variable 工具
{ key: "金币", value: "95" }  // 字符串，不是数字
{ key: "温莎声望", value: "10" }
```

TH 没有 `delta` 操作，没有嵌套路径，没有 JSONPatch。LLM 要修改金币，必须先 `get_variable("金币")` 读出当前值，计算后再 `set_variable("金币", newValue)` 写回。

**TH 的变量设计原则：简单、扁平、可审计。** 每次变量变化都是一次完整的 upsert，有完整的事件日志。

---

## 三、嵌套变量的问题

Victoria 原卡的嵌套结构（`/玩家状态/资产/金币`）在实践中有几个问题：

### 3.1 LLM 可靠性问题

嵌套路径越深，LLM 出错概率越高。`/玩家状态/资产/金币` 这种路径要求 LLM 精确记住整个变量树的结构，一旦路径写错（如 `/玩家/资产/金币`），操作静默失败。

扁平结构（`金币: 95`）的 LLM 出错率远低于嵌套路径。

### 3.2 delta 操作的竞态问题

`delta` 操作（`/金币 += -5`）在后端是**读-改-写**三步，不是原子操作。如果同一 session 有并发请求（虽然 GW 有 generating 锁，但 Agentic Loop 内多次工具调用可能触发），delta 操作会产生竞态。

扁平 + 工具调用（先 get 再 set）把竞态问题暴露给 LLM，反而更安全——LLM 会在同一次 tool loop 内串行执行。

### 3.3 未来嵌套的可能性

**会增加嵌套吗？** 短期不会，中期可能有有限支持。

真正需要嵌套的场景：
- 物品栏（`物品栏/蒸汽手枪/数量`）
- 势力声望（`势力声望/温莎`）

这些场景的更好解法是**用前缀模拟嵌套**：
```
物品栏.蒸汽手枪.数量 = 1
势力声望.温莎 = 10
```

前缀扁平化保留了嵌套的语义，同时避免了 JSONPatch 的复杂性。前端展示时按前缀分组即可。

---

## 四、更好的解决方案

### 方案对比

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| **A（当前推荐）** | 扁平化变量 + `<UpdateState>` merge patch | 立即可用，LLM 可靠性高 | 需要重写 Victoria 的变量结构 |
| **B（中期）** | 后端实现 JSONPatch delta/insert/remove | 完整复现原卡 | 实现复杂，LLM 出错率高 |
| **C（推荐长期）** | 扁平前缀 + `delta` 工具 | 兼顾表达力和可靠性 | 需要新增工具 |

### 推荐方案 C 的具体设计

在现有 `set_variable`/`get_variable` 基础上，新增一个 `delta_variable` 工具：

```go
// 新增工具：数值增减（原子操作，后端保证）
{
  name: "delta_variable",
  description: "对数值变量做增减操作（原子）。key 支持点分隔前缀如 '势力声望.温莎'",
  parameters: {
    key: string,   // 支持 "金币" 或 "势力声望.温莎"
    delta: number  // 正数增加，负数减少
  }
}
```

后端实现：
```go
// 读取当前值 → 加 delta → 写回（在 session 锁内执行，无竞态）
func (m *Manager) DeltaVariable(sessionID, key string, delta float64) error
```

这样 LLM 不需要先 get 再 set，一次工具调用完成增减，且后端保证原子性。

**物品栏的处理**：用 `set_variable("物品栏.蒸汽手枪", `{"描述":"...","数量":1}`)` 存 JSON 字符串，前端解析展示。不需要嵌套路径操作。

---

## 五、现阶段需要做的工作

### 5.1 立即可做（不阻塞游玩）

**Victoria 卡 preset_entries 格式指令（方案 A）**

在 `game.json` 的 `preset_entries` 加一条格式指令，告诉 LLM 用扁平变量 + `<UpdateState>` 格式：

```json
{
  "preset_entries": [
    {
      "role": "system",
      "content": "变量更新格式：<UpdateState>{\"金币\": 95, \"温莎声望\": 10}</UpdateState>\n只写需要变化的键，不写的键保持不变。数值直接写新值（不是增量）。",
      "enabled": true,
      "order": 0
    }
  ]
}
```

同时将 `initial_variables` 扁平化：
```json
{
  "initial_variables": {
    "存活天数": 1,
    "当前位置": "未知",
    "金币": 0, "银币": 0, "铜币": 0,
    "温莎声望": 0, "罗斯柴尔德声望": 0, "克拉伦斯声望": 0,
    "莫里亚蒂声望": 0, "瓦特声望": 0, "市政厅声望": 0
  }
}
```

**工作量**：编辑 `game.json`，约 30 分钟。

### 5.2 中期（P-4K 之后，约 2 周）

**新增 `delta_variable` 工具**

在 `internal/engine/tools/` 新增工具定义和执行逻辑：

```go
// internal/engine/tools/builtin.go（新增）
{
  Name: "delta_variable",
  Description: "对数值变量做增减（原子）。支持点分隔前缀如 '势力声望.温莎'",
  Parameters: schema{key: string, delta: number},
}
```

执行逻辑：
```go
case "delta_variable":
    key := args["key"].(string)
    delta := args["delta"].(float64)
    return e.sessions.DeltaVariable(req.SessionID, key, delta)
```

`session.Manager.DeltaVariable`：
```go
func (m *Manager) DeltaVariable(sessionID, key string, delta float64) error {
    // 在 DB 事务内：读当前值 → 加 delta → 写回
    // 用 JSONB 的 || 操作符原子更新
}
```

**工作量**：约 60 行，1-2 小时。

### 5.3 不做（明确排除）

**完整 JSONPatch（RFC 6902）实现**

- `insert`/`remove`/`move` 操作在 LLM 输出中可靠性极低
- 嵌套路径（`/玩家状态/资产/金币`）要求 LLM 精确记住变量树结构
- TH 也没有实现，说明这不是必要能力
- 用扁平前缀 + `delta_variable` 工具可以覆盖 95% 的使用场景

---

## 六、对其他问题的回应

### 6.1 `cmd/worker` 编译错误

**已修复（2026-04-11）**。`BatchSize`/`LeaseTTL` 字段已从 `WorkerConfig` 移除（P-4G 重构），`NewWorker` 签名增加了 `*scheduler.Scheduler` 参数。`cmd/worker/main.go` 已同步更新，`go build ./...` 全部通过。

### 6.2 `LLMProfile` 归属问题

`LLMProfile` 定义在 `models_creation.go`，但被 `platform/provider` 包 import，依赖方向反了。

**现阶段不修复**，原因：
- 这是包结构问题，不是功能问题
- 修复需要将 `LLMProfile`/`LLMProfileBinding` 移到 `models_shared.go` 或新建 `models_platform.go`
- 会触发大量 import 路径变更
- 等 CW/WE 解耦前置条件修复时（中期）一并处理

**临时标记**：在 `models_creation.go` 顶部加注释 `// TODO(P-CW-decouple): LLMProfile 应移至 models_platform.go`。

### 6.3 `creation/api` 直接用 `core/llm` 绕过 Provider 注册表

`creation/api/routes.go` 中 AI 辅助创作接口直接 `llm.NewClient(...)` 而非走 `provider.Registry`。

**现阶段可接受**，原因：
- AI 辅助创作是 P3 优先级功能，尚未实现
- 等实现时直接用 `registry.Default().Client()` 即可，一行改动
- 不影响当前游玩功能

### 6.4 `CharacterCard.GameID` 语义混乱

`creation/api/routes.go:68` 暂用 `CharacterCard.ID` 作为 `GameID`，语义不清。

**现阶段不修复**，等 CW/WE 解耦时明确 `CharacterCard` 与 `GameTemplate` 的关系后再处理。

---

## 七、工作优先级汇总

| 任务 | 优先级 | 工作量 | 说明 |
|------|--------|--------|------|
| Victoria `game.json` 扁平化 + 格式指令 | **P0** | 30 分钟 | 使卡立即可游玩 |
| `delta_variable` 工具 | P2 | 1-2 小时 | P-4K 之后做，不阻塞游玩 |
| `LLMProfile` 移包 | 中期 | 半天 | CW/WE 解耦前置 |
| 完整 JSONPatch | 不做 | — | 用扁平前缀替代 |
