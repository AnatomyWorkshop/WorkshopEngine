# WebGal 状态模块哲学 vs WE/TH + 分阶段记忆控制

> 日期：2026-04-06
> 素材：WebGal 团队《状态模块 PRD》+ WE 引擎代码分析

---

## 一、WebGal 的状态设计哲学

WebGal PRD 描述的是一套用于 TRPG/跑团场景的**确定性状态机**，和 TH/WE 的 LLM 驱动思路在根本上不同。值得完整理解这个思路，因为它代表了"游戏引擎传统做法"在 AI 叙事领域的延伸。

### 核心架构：三层分离

```
底层变量 (KV)         状态系统 (Status)         显示层
  hp: 100       ×    毒素debuff (-20%)    →    显示值: 80
  atk: 50       ×    buff (+30%)         →    显示值: 65
  mp: 30        ×    (无效果)            →    显示值: 30
```

**Layer 1 — KV 变量系统**：纯数据，只有 Set / Add / Sub 三种操作。类似 Redis，但操作是可重算的（不是 CRDT 也不是 event sourcing，是快照 + 重算）。

**Layer 2 — 状态系统（Status/Buff/Debuff）**：临时叠加在 KV 上的修正层。状态有生命周期（"下一回合"触发 decay 和 settlement）。状态不存值，只存"对哪个变量施加什么系数/加减"。

**Layer 3 — 显示值**：前端渲染时动态计算 `baseValue × statusModifiers`，不存入 DB。

**技能（Skill）= 原子消息组**：一个技能被拆分为多条原子消息，每条消息触发一个状态变更。"召唤冰霜之刃"= [对目标施加 atk_down 状态] + [对目标施加 def_down 状态] + [造成 20 点伤害]。

### 可逆性设计：快照 + 重算

WebGal 明确拒绝了"操作可逆"方案（思路一），选择了**快照 + 重算**（思路二）：

```
[快照 @turn_10: {hp:80, mp:20, ...}]
   ↓ 消息序列
   消息A: hp -5
   消息B: mp +10 (被删除/修改)
   消息C: hp -3
   ↓ 重算
[当前状态: {hp:72, mp:30, ...}]
```

当历史消息被修改，从最近的快照开始对消息队列重新演算。只要消息队列不超过几千条，速度足够快。

**这个选择为什么重要：** 它使系统状态是**确定性**的——相同的消息序列必然产生相同的结果，与 LLM 的不确定输出无关。

---

## 二、为什么 TH 和 WE 与 WebGal"完全不相容"

### 根本哲学差异

| 维度 | WebGal 状态模块 | TH/WE |
|---|---|---|
| **状态由谁计算** | 确定性算法（Set/Add/Sub + Snapshot） | LLM 理解并生成变量更新 |
| **结果是否可复现** | 是（相同消息序列 → 相同状态） | 否（LLM 输出有随机性） |
| **技能/规则定义** | 原子消息映射（精确，可编辑） | 自然语言 Prompt（灵活，不精确） |
| **Buff/Debuff 生命周期** | 显式回合计数器驱动 | LLM 记住/遗忘（不可靠） |
| **状态一致性保证** | 快照 + 重算（强保证） | 无（依赖 LLM 上下文记忆） |
| **适用场景** | PvP 战斗 / COC 跑团 / 严格规则系统 | 开放叙事 / 角色扮演 / 故事推进 |

### 一个具体例子说明差异

**场景**：玩家在战斗中中了毒，毒素持续 3 回合，每回合扣 10 血。

**WebGal 做法：**
```
施毒消息 → 创建状态 {type: poison, value: -10, duration: 3}
第1回合结束 → 状态系统: hp -= 10, duration → 2
第2回合结束 → hp -= 10, duration → 1
第3回合结束 → hp -= 10, 状态过期删除
```
精确，机械，可复现。玩家回退第2回合消息 → 重算，精确还原。

**WE/TH 做法：**
```
SystemPrompt: "角色中了毒，毒素持续3回合，每回合扣10血"
变量: {hp: 90, poison_turns_remaining: 3}
每回合：LLM 读到变量 → 在叙述中扣血 → Director 槽更新变量
```
灵活，叙事连贯，但：
- LLM 可能在叙述中忘记扣血（上下文偏移时）
- 回合计数依赖 LLM 正确递减变量（有时会出错）
- 无法"回退第2回合"

**结论：** 两种做法不是优劣，是**适用场景不同**。WebGal 适合需要规则精确性的 TRPG；WE/TH 适合以叙事为中心、规则服务于故事的场景。**不建议在 WE 里强行引入 WebGal 的状态系统**——它会带来大量工程复杂度，但 WE 的目标用户（写角色扮演游戏的创作者）并不需要这种精确性。

### 如果真的需要精确数值系统怎么办？

WE 里有一个折中方案：**把规则计算委托给 Director 槽**。

```
Director Prompt（创作者配置）:
  "根据当前变量和玩家行动，精确计算战斗结果：
   攻击 = atk * (1 + buff) - def * (1 - debuff)
   将计算结果以 JSON 返回：{damage: N, new_hp: M, status_changes: [...]}"

主回合 LLM（Narrator）:
  "收到 Director 的计算结果，用叙事方式呈现战斗结果"
```

Director 做数值计算，Narrator 做叙事渲染，两者分工。这不如 WebGal 的确定性算法可靠，但对大多数叙事游戏来说已经足够，且开发成本极低（只需在模板 Config 里写 Director Prompt）。

---

## 三、WE 能否在游戏过程中按阶段控制记忆的可见性？

### 问题

> 游戏发展到某一作者设定的阶段，可能只需要特定的逻辑和记忆，避免 LLM 阅读其它逻辑，TH 对这种情况有相关设计吗？WE 应该怎么做？

这个需求叫**分阶段上下文隔离（Stage-Scoped Context）**，在推理小说、多幕剧结构的 VN、和多章节 RPG 里非常有价值：

```
第一幕（调查阶段）：LLM 只知道线索A/B/C，不知道凶手是谁
第二幕（对峙阶段）：LLM 需要知道凶手，但应该"忘掉"早期红鲱鱼条目
第三幕（收尾阶段）：LLM 需要结局相关的记忆，早期调查细节不再重要
```

目的是：**省 token + 防止 LLM 提前剧透 + 保持叙事焦点**。

### TH 有没有这个设计？

**没有专门的阶段隔离机制。**

TH 的 WorldbookEntry 靠关键词触发，本质上是被动的——LLM 谈到某个词条才激活。Memory 是全量注入的（按 importance + time 排序取 budget 内的条目），没有阶段过滤。

TH 可以间接实现：创作者把"阶段"存为变量（`current_act: "investigation"`），然后在 WorldbookEntry 的 Content 里用条件格式，但这依赖 LLM 自己判断是否应用该条目的内容，不是引擎层面的硬过滤。

### WE 当前状态

**Worldbook 部分**：`resolveMacros` 已经支持 `{{var}}` 宏替换内容，但激活条件（Keys）不支持变量匹配——只能匹配扫描文本中的关键词，不能写"仅当 stage==act2 时激活"。

**Memory 部分**：`GetForInjection` 全量取所有 `deprecated=false` 的 Memory，无阶段过滤。

### WE 应该怎么做：三层递进方案

---

#### 方案 A（零改动）：用 Worldbook Group + 关键词语义隐藏

把阶段写进会话变量，然后在 SystemPrompt 里注入阶段标识，让 Worldbook 条目根据扫描文本自动激活/抑制：

```
SystemPrompt（每回合注入）: 
  "当前游戏阶段：{{game_stage}}
   （investigation = 调查阶段，confrontation = 对峙阶段）"

WorldbookEntry（对峙阶段专属条目）:
  Keys: ["confrontation", "对峙阶段"]   ← 匹配 SystemPrompt 中注入的阶段词
  Content: "真相：凶手是X，动机是Y..."
```

当 `game_stage = "investigation"` 时，SystemPrompt 里不含 "confrontation"，该词条不激活。  
当 `game_stage = "confrontation"` 时，词条自动激活。

**优点**：零后端改动，现在就能用。  
**缺点**：不是真正的硬隔离——如果对话内容中出现"confrontation"这个词，条目意外激活。

---

#### 方案 B（少量改动）：matchKey 支持 `var:` 语法

扩展 `node_worldbook.go` 的 `matchKey` 函数，识别 `var:key_name=value` 格式的条件键：

```go
// 新增：变量条件匹配
// 格式: "var:stage=confrontation"
const varPrefix = "var:"
if strings.HasPrefix(key, varPrefix) {
    expr := key[len(varPrefix):]  // "stage=confrontation"
    parts := strings.SplitN(expr, "=", 2)
    if len(parts) == 2 {
        varKey, expected := parts[0], parts[1]
        actual := fmt.Sprintf("%v", ctx.Variables[varKey])
        return actual == expected
    }
    return false
}
```

WorldbookEntry 配置：
```
Keys: ["var:game_stage=confrontation"]
Content: "真相：凶手是X..."
```

这是**引擎层面的硬条件**，LLM 看不到也控制不了。`game_stage` 变量由创作者在关键回合通过工具调用或 Verifier 更新，触发阶段切换。

**改动量**：`node_worldbook.go` 的 `matchKey` 函数增加约 15 行，无 schema 变更。  
**优点**：硬过滤，精确，无误触发。

---

#### 方案 C（中等改动）：Memory 的阶段标签过滤

这是最完整的解法，针对 Memory 注入（WorldbookEntry 已有方案 B）。

**DB 层**：`Memory` 新增 `StageTags []string`（JSONB）
```sql
ALTER TABLE memories ADD COLUMN stage_tags JSONB DEFAULT '[]';
```

**注入层**：`GetForInjection` 增加阶段过滤参数
```go
func (s *Store) GetForInjection(sessionID string, tokenBudget int, currentStage string) (string, error) {
    // 只取 stage_tags 为空（无阶段限制）OR 包含 currentStage 的 Memory
    err := s.db.Where(
        "session_id = ? AND deprecated = false AND (stage_tags = '[]' OR stage_tags @> ?)",
        sessionID,
        fmt.Sprintf(`["%s"]`, currentStage),
    ).Order("importance DESC, created_at DESC").Find(&mems)
}
```

**创作者工作流**：
- 创作者在 CW 里为特定 Memory 条目标注 `stage_tags: ["confrontation"]`
- 或者：`applyStructuredResult` 在写入 Memory 时，从 LLM 输出的 `facts_add` 中读取 `stage` 字段自动打标签
- 引擎根据当前 `game_stage` 变量过滤注入的 Memory

**改动量**：Memory 模型 + 注入函数 + API 约 60 行。

---

#### 推荐路径

| 阶段 | 方案 | 适用场景 |
|---|---|---|
| 现在就用 | A（变量注入 + 关键词匹配） | 简单分幕，词条数量少 |
| 3 天内可做 | B（`var:` 语法扩展） | 精确词条阶段门控，零 schema 变更 |
| 中期规划 | B + C（词条 + Memory 双层过滤） | 多幕复杂叙事，需要硬隔离 |

---

## 四、一个完整场景：推理小说三幕式分阶段隔离

```
游戏变量: { game_stage: "investigation" }

=== 第一幕：调查 ===

Worldbook（始终激活）:
  ✅ 案发地点描述
  ✅ 嫌疑人A外貌/动机
  ✅ 嫌疑人B外貌/动机
  ❌ Keys: ["var:game_stage=confrontation"] → 真相 (不激活)
  ❌ Keys: ["var:game_stage=resolution"]   → 结局台词 (不激活)

Memory 注入:
  ✅ stage_tags: [] — 所有公共记忆（玩家选择了谁做嫌疑人等）
  ❌ stage_tags: ["confrontation"] — 还没生成，不注入

=== 玩家调查完毕，创作者设计的触发条件达成 ===
  Director 检测到 "found_all_clues = true"
  → 更新 game_stage = "confrontation"

=== 第二幕：对峙 ===

Worldbook（阶段切换后）:
  ✅ Keys: ["var:game_stage=confrontation"] → 真相激活！LLM 现在知道凶手
  ❌ 红鲱鱼条目用 Group: "red_herring", GroupWeight: 1
     → 被另一个同组权重更高的条目替代，或直接不激活

Memory 注入:
  ✅ stage_tags: []   — 所有公共记忆
  ✅ stage_tags: ["confrontation"] — 对峙阶段特定记忆（玩家的重要推断等）
  过滤掉: stage_tags: ["investigation_only"] — 早期调查细节不再注入

=== 第三幕：结局 ===
同理，game_stage → "resolution"，注入结局相关词条和记忆
```

这个设计：
- LLM 无法"提前知道"凶手（阶段门控是引擎强制的，不是 Prompt 软约束）
- 节省 token：每幕只注入当前阶段需要的上下文
- 不同玩家的 Memory 各自独立：玩家A调查时做的推断不会污染玩家B的视角

---

## 五、小结

**WebGal 状态模块**：确定性状态机，适合精确规则游戏（TRPG、战斗系统）。KV + 快照重算 + 状态层三层分离，与 LLM 无关，是传统游戏引擎思路在多人跑团场景的自然延伸。WE/TH 不应该照搬，但数值系统需求可以通过 Director 槽折中实现。

**分阶段记忆控制**：TH 无此设计。WE 通过三步递进（关键词注入 → `var:` 条件语法 → Memory 阶段标签）可以完整支持。**方案 B（`var:` 语法）是最值得先做的**，只需 15 行代码改动，效果远超方案 A，而不需要 schema 变更。
