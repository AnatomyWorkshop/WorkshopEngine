# 向量记忆、风格漂移与客户端数据域

> 日期：2026-04-09
> 话题：① RAG 向量记忆在 AI 角色中的应用；② AI 角色"越用越懂你"的机制；③ 风格漂移问题与创作者/玩家控制权；④ 客户端专属数据域的必要性与实现

---

## 一、RAG 向量记忆：ST / TH / WE 现状

### 1.1 什么是 RAG 向量记忆

RAG（Retrieval-Augmented Generation）在 AI 角色场景里的含义：

```
传统记忆（WE 当前方案）：
  所有记忆条目 → 按 importance × decay 排序 → 取 Top-K → 拼入 Prompt

向量记忆（RAG 方案）：
  所有记忆条目 → 向量化（embedding）→ 存入向量数据库
  每次对话时 → 把当前输入向量化 → 余弦相似度检索 → 取语义最近的 Top-K → 拼入 Prompt
```

核心差异：**语义相关性**替代**时间重要性**作为检索依据。

### 1.2 ST 的现状

ST 本身没有内置向量记忆，但有插件生态：

- **Memory Bank 插件**（社区）：把对话历史向量化存入本地 SQLite + `sqlite-vss` 扩展，每轮检索 Top-5 注入 System Prompt。
- **Chroma / Qdrant 插件**：连接外部向量数据库，适合长期角色（数千条记忆）。
- **Summarize 插件**：不是向量，而是定期用 LLM 压缩历史为摘要，注入 System Prompt 顶部。

ST 的向量记忆是**可选插件**，不是核心功能。大多数用户用的是 ST 内置的"Summary"（摘要）机制，不是 RAG。

### 1.3 TH 的现状

TH 的记忆系统（见 `st-comparison.md §3.4`）：

- 结构化整合输出：`{turn_summary, facts_add, facts_update, facts_deprecate}`
- MemoryEdge：supports / contradicts / updates 关系图
- **没有向量检索**：TH 的记忆检索是基于 `importance` 排序 + 时间衰减，不是语义相似度。

TH 的设计哲学是"结构化事实图"，而不是"向量语义检索"。

### 1.4 WE 的现状

WE 当前记忆系统（`internal/engine/memory/`）：

- 指数半衰期 + MinDecayFactor 动态排序（比 TH 更强的时间衰减）
- 自由文本摘要（不是结构化 JSON facts）
- **没有向量检索**

WE 目前的检索是：`importance × decay_factor` 排序，取 Top-K 注入 Prompt。

### 1.5 向量记忆的实际价值评估

| 场景 | 向量记忆的价值 | 传统排序的价值 |
|------|--------------|--------------|
| 短期游戏（< 50 回合） | 低（记忆少，排序够用） | 高（简单可靠） |
| 长期角色（> 500 回合） | 高（语义检索找到"3个月前提到的事"） | 低（时间衰减会把旧记忆淘汰） |
| 常驻角色（跨游戏积累） | **非常高** | 中（记忆量大时排序失效） |

**结论**：向量记忆对 WE 的**常驻角色**（Resident Character）场景价值最大。普通游戏 session 用当前排序方案足够。

### 1.6 WE 加入向量记忆的方案（中期目标）

**方案：pgvector**（PostgreSQL 扩展，不引入新基础设施）

```sql
-- 在 memories 表加 embedding 列
ALTER TABLE memories ADD COLUMN embedding vector(1536);  -- OpenAI text-embedding-3-small 维度

-- 创建向量索引
CREATE INDEX ON memories USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
```

检索时：

```go
// 把当前用户输入向量化
embedding := llm.Embed(userInput)  // 调用 embedding API

// 语义检索 Top-10
db.Raw(`
    SELECT *, 1 - (embedding <=> ?) AS similarity
    FROM memories
    WHERE session_id = ?
    ORDER BY similarity DESC
    LIMIT 10
`, pgvector.NewVector(embedding), sessionID).Scan(&memories)
```

**成本**：每次对话多一次 embedding API 调用（约 $0.00002 / 1K tokens，极低）。  
**依赖**：PostgreSQL 需安装 `pgvector` 扩展（`CREATE EXTENSION vector`）。

**实现优先级**：常驻角色 Phase 2 时引入，普通游戏 session 暂不需要。

---

## 二、AI 角色"越用越懂你"的机制

### 2.1 这个现象的本质

"越用越懂你"不是模型本身在学习（LLM 权重不会因为你的对话而更新），而是**上下文积累**：

```
第 1 次对话：AI 不知道你喜欢什么
第 100 次对话：AI 的 Prompt 里注入了 100 条关于你的记忆
              → AI 的回复"看起来"更懂你
```

本质是**记忆系统的积累效果**，不是模型学习。

### 2.2 ST 的实现方式

ST 的"越用越懂你"来自几个机制叠加：

1. **Author's Note**：玩家手动写入的关于自己的描述，每次都注入 Prompt。
2. **Persona**（用户画像）：ST 有 `{{user}}` 宏，玩家可以定义自己的角色描述。
3. **记忆插件**：把对话中提到的玩家偏好自动提取为记忆条目。
4. **世界书触发**：玩家名字/特征作为触发词，激活相关世界书条目。

### 2.3 TH 的实现方式

TH 的结构化记忆整合：

```json
{
  "turn_summary": "玩家提到自己喜欢猫，不喜欢早起",
  "facts_add": [
    { "key": "player.preference.pet", "value": "喜欢猫" },
    { "key": "player.preference.morning", "value": "不喜欢早起" }
  ]
}
```

这些 facts 在后续对话中被检索注入，AI 就"记住"了玩家的偏好。

### 2.4 WE 目前的能力

WE 的记忆系统可以做到类似效果，但需要创作者在 Preset 里配置记忆提取 Prompt：

```
[Memory Worker Prompt]
从以下对话中提取关于玩家的重要信息，以 JSON 格式输出：
{ "summary": "...", "player_facts": [{ "key": "...", "value": "..." }] }
```

WE 的 Memory Worker 会定期（每 N 回合）运行这个 Prompt，把提取的信息存入 memories 表。

**差距**：WE 目前是自由文本摘要，没有 TH 那样的结构化 `fact_key` 系统。中期目标是实现结构化整合（见 `st-comparison.md §五 Tier 1`）。

### 2.5 玩家画像向量化（你的想法）

你提到：**让玩家自己描述形象，把回复风格和玩家画像描述进向量数据库并可视化**。

这是一个很有意思的设计，分解一下：

**玩家画像（Player Persona）**：
- 玩家在 GW 个人设置里填写："我是一个喜欢悬疑故事的读者，偏好简洁的文风，不喜欢过于煽情的描写"
- 这段描述向量化后存入 `user_profiles.persona_embedding`
- 游戏开始时，把玩家画像注入 Prompt（类似 ST 的 Persona 功能）

**回复风格向量化**：
- 每次 AI 回复后，提取风格特征（句子长度、情感倾向、词汇复杂度）
- 存入 `session_style_history` 表
- 可视化：折线图展示"AI 回复风格随时间的变化趋势"

**可行性**：技术上完全可行，但这是 GW 的**高级功能**，不是 MVP 必需。

---

## 三、风格漂移问题与控制权设计

### 3.1 什么是风格漂移

AI 角色在长期使用中，回复风格会逐渐向玩家的输入风格靠拢：

```
玩家习惯用短句、口语化表达
→ AI 的回复也变得越来越短、越来越口语化
→ 角色的"个性"被稀释，变成了玩家的镜像
```

这是 LLM 的 in-context learning 特性导致的：模型会从上下文中学习"这个对话的风格是什么"，然后模仿。

### 3.2 ST 的现状

ST 没有专门的风格漂移控制机制。用户的解决方案：

- 在 System Prompt 里反复强调角色的说话风格（"Alice 总是用长句，喜欢引用诗歌"）
- 使用 Author's Note 在每次对话前提醒 AI
- 定期手动清理对话历史（减少玩家风格的"污染"）

这些都是手动的、不系统的解决方案。

### 3.3 你的"风格收束"滑块设计

你提出的方案：

```
风格收束度（Style Convergence）滑块：
  最大（酒馆默认）← ─────────────────── → 最小（创作者保护）
  AI 自由适应玩家风格              AI 严格保持角色风格
```

- 创作者设定默认值（保护角色个性）
- 玩家可以在允许范围内调整（个性化体验）

**这个思路非常好**，而且 WE 已经有实现基础。

### 3.4 WE 的实现方案

**方案 A：Preset Entry 注入（最简单，立即可做）**

在 GameTemplate 的 Preset 里加一个"风格锚定"条目：

```
[injection_order: 1, position: system_top]
你是 Alice，一个对镜子有执念的少女。
你的说话风格：长句，喜欢用比喻，偶尔引用诗歌，不使用网络用语。
无论对话如何发展，你必须保持这种说话风格。
```

这是最简单的方案，但效果有限（LLM 仍然会被上下文影响）。

**方案 B：Verifier 槽（WE 已有，中期可用）**

WE 已有 Verifier 槽（`verifier_prompt` in Config）。可以配置：

```
[Verifier Prompt]
检查以下 AI 回复是否符合角色的说话风格：
- 是否使用了长句？
- 是否避免了网络用语？
- 是否保持了角色的情感基调？
如果不符合，输出 { "pass": false, "reason": "..." }，触发重新生成。
```

这是**自动风格校验**，不需要玩家手动干预。

**方案 C：风格收束度参数（你的滑块设计，中期实现）**

在 `GameTemplate.Config` 里加 `style_convergence` 参数：

```json
{
  "style_convergence": {
    "default": 0.3,          // 创作者设定默认值（0=完全保持角色，1=完全适应玩家）
    "player_min": 0.1,       // 玩家可调整的最小值
    "player_max": 0.7        // 玩家可调整的最大值
  }
}
```

前端：游玩页设置里显示滑块，玩家可以在 `[player_min, player_max]` 范围内调整。

后端实现：`style_convergence` 值影响 Preset 里"风格锚定"条目的注入强度：

```
style_convergence = 0.1 → 注入强力风格锚定（"你必须严格保持以下风格..."）
style_convergence = 0.7 → 注入弱风格提示（"你倾向于以下风格，但可以根据对话自然调整..."）
```

具体实现：在 PromptBlock 组装时，根据 `style_convergence` 值动态选择不同的 Preset Entry（创作者预先写好两个版本，WE 根据参数选择注入哪个）。

**方案 D：创作者素材库向量数据库（你的想法）**

你提到：**在 Material 内允许创作者自己的向量数据库**。

这实际上是 WE 已有的 `search_material` 工具的扩展：

```
当前：search_material 按标签/情绪/风格检索素材文本
扩展：search_material 支持语义检索（向量相似度）
     → 创作者上传"角色风格示例对话"作为素材
     → 每次对话时，检索最相似的风格示例注入 Prompt
     → AI 参考这些示例保持风格一致性
```

这是**风格示例注入**（Few-shot style anchoring），比单纯的文字描述更有效。

**实现**：在 `materials` 表加 `embedding vector(1536)` 列，`search_material` 工具支持 `mode: "semantic"` 参数。

### 3.5 渐进式风格飘移（新设想）

你提出的扩展：**滑块随故事发展自动向玩家风格飘移，玩家随时可以拒绝**。

**核心思路**：

```
早期（前 20 回合）：style_convergence 锚定在创作者设定的默认值
                   → AI 严格保持角色原始风格，玩家感受到"这个角色有自己的个性"

中期（20-80 回合）：style_convergence 开始缓慢向玩家风格漂移
                   → 每 N 回合，后端分析玩家输入风格（句长/情感/词汇），
                     将 style_convergence 向玩家风格方向微调 Δ
                   → 玩家感受到"角色开始懂我了"

后期（80+ 回合）：style_convergence 稳定在玩家风格附近
                  → 角色已经"学会"了玩家的表达习惯
```

**玩家控制权**：

- 前端滑块实时显示当前 `style_convergence` 值（包括自动飘移后的值）
- 玩家可以随时手动拖动滑块，覆盖自动飘移值
- 玩家可以点击"锁定"按钮，冻结当前值，阻止后续自动飘移
- 创作者可以在 `GameTemplate.Config` 里设置 `drift_enabled: false`，完全禁用自动飘移

**后端实现思路**：

```json
{
  "style_convergence": {
    "default": 0.3,
    "player_min": 0.1,
    "player_max": 0.8,
    "drift_enabled": true,
    "drift_rate": 0.01,        // 每 10 回合最多飘移 0.01
    "drift_start_turn": 20,    // 第 20 回合开始飘移
    "drift_locked": false      // 玩家锁定后为 true
  }
}
```

每次 Memory Worker 运行时（每 N 回合），分析最近 K 条玩家输入的风格特征，计算"玩家风格向量"，与当前 `style_convergence` 比较，决定是否微调。

**叙事意义**：这不只是技术参数，而是一个叙事弧——"角色在认识你"。早期的陌生感、中期的磨合、后期的默契，都通过风格变化自然呈现，而不需要显式的剧情说明。玩家可以拒绝这种"被理解"（锁定滑块），这本身也是一种叙事选择。

### 3.6 推荐实现路径

```
Phase 1（立即）：方案 A — Preset Entry 风格锚定（创作者手动配置，无需引擎改动）
Phase 2（中期）：方案 B — Verifier 风格校验（WE 已有 Verifier 槽，配置即可）
Phase 3（长期）：方案 C — 风格收束度滑块（需要前端 UI + Config 参数 + Preset 动态选择）
Phase 4（长期）：方案 D — 素材库向量检索（需要 pgvector + embedding API）
```

**你的滑块设计是正确方向**，Phase 3 实现。Phase 1 和 Phase 2 是过渡方案，创作者现在就可以用。

---

## 四、客户端专属数据域

### 4.1 什么是客户端专属数据域

**客户端专属数据域**（Client-Side Data Domain / Local Data Sovereignty）：

> 某些数据只存在于玩家的本地设备，不上传到服务器，服务器无法访问。

这是 GW 的 `local_only` 存储策略的核心概念（见 `GW-PLAY-ENTRY-AND-INTERACTION.md §四`）。

具体包括：
- 玩家的对话历史（`local_only` 游戏）
- 玩家的个人 API Key（不应上传到服务器）
- 玩家的私人批注（不想让平台看到）
- 玩家的设备偏好设置

### 4.2 是否有必要做

**有必要，原因：**

1. **隐私敏感内容**：很多 AI 角色游戏涉及私密对话（情感陪伴、NSFW 内容），玩家不希望这些数据存在服务器上。
2. **API Key 安全**：玩家自带 API Key 时，Key 不应经过 GW 服务器（中间人风险）。
3. **合规要求**：GDPR 等法规要求用户对自己的数据有控制权，"不上传"是最彻底的控制。
4. **竞争差异化**：ST 的核心卖点之一就是"本地运行，数据不离开你的设备"。GW 提供类似选项可以吸引这部分用户。

### 4.3 怎么做

**三层架构**：

```
Layer 1：云端存储（默认）
  - 存档存在 GW 服务器 PostgreSQL
  - 跨设备同步，社交功能完整
  - 玩家 API Key 通过 GW 服务器中转（GW 代理 LLM 请求）

Layer 2：混合存储（local_optional）
  - 存档存在本地 IndexedDB（浏览器）或 SQLite（桌面客户端）
  - 同时上传到云端（可选，玩家控制）
  - 玩家 API Key 存在本地，LLM 请求直接从浏览器发出（不经过 GW 服务器）

Layer 3：纯本地（local_only）
  - 存档只在本地，不上传
  - LLM 请求直接从客户端发出
  - GW 服务器只提供游戏包下载，不参与游玩过程
```

### 4.4 在哪做

**前端（GW React 应用）**：

```typescript
// 本地存储层（IndexedDB via idb-keyval 或 Dexie.js）
const localDB = new Dexie('gw-local');
localDB.version(1).stores({
  sessions: 'id, game_id, updated_at',
  floors: 'id, session_id, floor_seq',
  settings: 'key'
});

// API Key 存储（不上传）
await localDB.settings.put({ key: 'api_key', value: encryptedKey });

// local_only 游戏的 LLM 请求直接从浏览器发出
if (game.storage_policy === 'local_only') {
  // 直接调用 OpenAI/Claude API，不经过 GW 服务器
  const response = await fetch('https://api.openai.com/v1/chat/completions', {
    headers: { 'Authorization': `Bearer ${localApiKey}` },
    body: JSON.stringify(prompt)
  });
}
```

**后端（WE）**：

`local_only` 游戏的游玩请求不经过 WE 服务器。WE 只需要：
1. 提供游戏包下载（`GET /api/play/games/:id/package`）
2. 提供 Prompt 组装服务（可选：`POST /api/play/prompt-build`，只组装 Prompt，不调用 LLM）

**桌面客户端（长期）**：

如果 GW 做桌面客户端（Electron / Tauri），可以在本地运行完整的 WE 引擎（Go 二进制），实现真正的本地运行。这是 ST 的模式。

### 4.5 玩家 API Key 的处理

这是客户端数据域最重要的部分：

```
方案 A（当前 WE 方案）：
  玩家 API Key → 上传到 GW 服务器 → GW 代理 LLM 请求
  优点：简单，服务器端可以做限流/监控
  缺点：GW 服务器能看到玩家的 API Key（安全风险）

方案 B（客户端直连）：
  玩家 API Key → 存在本地（加密）→ 浏览器直接调用 LLM API
  优点：GW 服务器看不到 Key，隐私更好
  缺点：CORS 问题（部分 LLM API 不允许浏览器直接调用）；无法在服务器端做 Prompt 组装

方案 C（混合）：
  玩家 API Key → 存在本地 → 发送给 GW 服务器时用 HTTPS 加密传输 → GW 用完即丢（不持久化）
  优点：可以做服务器端 Prompt 组装，Key 不持久化在服务器
  缺点：传输过程中 GW 服务器仍然能看到 Key
```

**WE 当前实现**：方案 A（`LLMProfile` 存储 API Key，见 `models_creation.go`）。  
**TH 的方案**：`secret_config_encrypted`（AES-256-GCM 加密存储，见 `st-comparison.md §3.9`）。  
**推荐**：短期用方案 A（已实现），中期参考 TH 实现加密存储（方案 A 的安全增强版）。

### 4.6 MVP 阶段的最小实现

不需要完整的客户端数据域，只需要：

1. **`local_only` 标志**：`GameTemplate.Config` 里加 `storage_policy: "local_only"`，前端检测到后提示玩家"此游戏存档不上传"。
2. **本地 API Key 存储**：玩家设置页面允许填写自己的 API Key，存在 `localStorage`（加密），不上传。
3. **前端直连 LLM**（仅 `local_only` 游戏）：绕过 WE 服务器，直接调用 LLM API。

这三点可以在前端独立实现，不需要后端改动。

---

## 五、总结与优先级

| 功能 | 价值 | 复杂度 | 优先级 |
|------|------|--------|--------|
| 向量记忆（pgvector）| 高（常驻角色场景） | 中 | Phase 2（常驻角色时引入）|
| 结构化记忆整合（JSON facts）| 高 | 中 | 中期（参考 TH 实现）|
| 风格锚定 Preset Entry | 中 | 低 | **立即可做**（创作者配置）|
| Verifier 风格校验 | 高 | 低 | **立即可做**（WE 已有 Verifier 槽）|
| 风格收束度滑块 | 高 | 中 | Phase 3 |
| 渐进式风格飘移（自动 drift） | 高 | 中 | Phase 3（与滑块同期）|
| 素材库向量检索 | 中 | 中 | Phase 4 |
| 客户端数据域（local_only）| 高（隐私卖点）| 中 | Phase 2 |
| 玩家 API Key 加密存储 | 高（安全）| 低 | 中期（参考 TH）|
| 玩家画像向量化 | 中 | 高 | Phase 4 |
