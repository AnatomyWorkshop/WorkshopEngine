# 数据库数据字典（backend-v2）

本文档记录 `backend-v2` 当前 PostgreSQL schema 的字段含义、枚举约束与索引约定。

## 迁移与版本

- ORM: GORM (gorm.io/gorm)
- 数据库: PostgreSQL
- 迁移方式: `db.AutoMigrate()`（启动时自动执行）
- 迁移入口: `internal/core/db/connect.go`

## `game_sessions`

一次完整的游玩会话。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 会话 ID |
| `game_id` | `text` | `NOT NULL`, index | 关联游戏模板 ID |
| `user_id` | `text` | index | 关联用户 ID |
| `title` | `text` | default `''` | 会话标题（可选） |
| `status` | `text` | default `'active'` | 会话状态 |
| `variables` | `jsonb` | default `'{}'` | Chat 级持久变量（五级沙箱的 chat 层） |
| `memory_summary` | `text` | | 最新摘要快照（由异步 Worker 写入） |
| `floor_count` | `integer` | default `0` | 已完成回合数，触发摘要的阈值依据 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `status`: `active | archived`

索引：
- 普通索引 `idx_game_sessions_game_id(game_id)`
- 普通索引 `idx_game_sessions_user_id(user_id)`

## `floors`

一个游戏回合（提交后不可改——核心设计原则）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 楼层 ID |
| `session_id` | `text` | `NOT NULL`, index | 所属会话 ID |
| `seq` | `integer` | `NOT NULL` | 楼层时序，决定对话顺序 |
| `status` | `text` | `NOT NULL`, default `'draft'` | 楼层状态 |
| `created_at` | `timestamptz` | | 创建时间 |

枚举约束：
- `status`: `draft | generating | committed | failed`

索引：
- 普通索引 `idx_floors_session_id(session_id)`

## `message_pages`

一个楼层内的一次生成尝试（即酒馆的 Swipe）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 消息页 ID |
| `floor_id` | `text` | `NOT NULL`, index | 所属楼层 ID |
| `is_active` | `boolean` | default `true` | 同一楼层只有一个 active |
| `messages` | `jsonb` | default `'[]'` | `[]Message{Role, Content}` |
| `page_vars` | `jsonb` | default `'{}'` | Page 沙箱变量（重试时直接丢弃） |
| `token_used` | `integer` | | 本页消耗 token 数 |
| `created_at` | `timestamptz` | | 创建时间 |

索引：
- 普通索引 `idx_message_pages_floor_id(floor_id)`

## `memories`

一条记忆条目。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 记忆 ID |
| `session_id` | `text` | `NOT NULL`, index | 所属会话 ID |
| `content` | `text` | `NOT NULL` | 记忆内容 |
| `type` | `text` | `NOT NULL`, default `'summary'` | 记忆类型 |
| `importance` | `float8` | default `1.0` | 衰减排序权重（越高越优先注入） |
| `source_floor` | `integer` | | 来自第几楼层（可溯源） |
| `deprecated` | `boolean` | default `false` | 过时标记（Lint 时置为 true） |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `type`: `fact | summary | open_loop`

索引：
- 普通索引 `idx_memories_session_id(session_id)`

## `character_cards`

角色卡（从 PNG 导入后结构化存储）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 角色卡 ID |
| `slug` | `text` | `NOT NULL`, uniqueIndex | 唯一标识符（URL 友好） |
| `name` | `text` | `NOT NULL` | 角色名称 |
| `spec` | `text` | default `'chara_card_v2'` | 规范版本 |
| `data` | `jsonb` | | 完整角色卡 JSON（CCv2/v3） |
| `avatar_url` | `text` | | 头像 URL |
| `tags` | `jsonb` | default `'[]'` | 标签数组 |
| `is_public` | `boolean` | default `true` | 是否公开 |
| `author_id` | `text` | index | 作者 ID |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `spec`: `chara_card_v2 | chara_card_v3`

索引：
- 唯一索引 `idx_character_cards_slug(slug)`
- 普通索引 `idx_character_cards_author_id(author_id)`

## `game_templates`

游戏模板（创作者打包的完整游戏配置）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 模板 ID |
| `slug` | `text` | `NOT NULL`, uniqueIndex | 唯一标识符 |
| `title` | `text` | `NOT NULL` | 模板标题 |
| `type` | `text` | `NOT NULL`, default `'visual_novel'` | 游戏类型 |
| `description` | `text` | | 描述 |
| `system_prompt_template` | `text` | | 系统提示模板（支持 `{{宏}}` 变量展开） |
| `config` | `jsonb` | default `'{}'` | 运行时配置（见下方 Config 字段说明） |
| `cover_url` | `text` | | 封面 URL |
| `status` | `text` | default `'draft'` | 发布状态 |
| `author_id` | `text` | index | 作者 ID |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `type`: `visual_novel | narrative | simulator`
- `status`: `draft | published`

索引：
- 唯一索引 `idx_game_templates_slug(slug)`
- 普通索引 `idx_game_templates_author_id(author_id)`

`config` JSONB 字段结构：

| 键 | 类型 | 说明 |
| -- | ---- | ---- |
| `memory_label` | `string` | 记忆注入标签前缀 |
| `fallback_options` | `[]string` | parser fallback 默认选项 |
| `enabled_tools` | `[]string` | 启用的工具名列表（`preset:*` 启用所有自定义工具） |
| `scheduled_turns` | `[]TriggerRule` | 自动回合触发规则 |
| `director_prompt` | `string` | Director 槽的分析指令（空则使用默认指令） |

## `worldbook_entries`

世界书词条（独立于游戏模板，支持多对多）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 词条 ID |
| `game_id` | `text` | `NOT NULL`, index | 关联游戏 ID |
| `keys` | `jsonb` | default `'[]'` | 主关键词数组（任意一条匹配即触发） |
| `secondary_keys` | `jsonb` | default `'[]'` | 次级关键词数组 |
| `secondary_logic` | `text` | default `'and_any'` | 次级关键词逻辑 |
| `content` | `text` | `NOT NULL` | 词条内容 |
| `constant` | `boolean` | default `false` | 无条件常驻（不需要关键词触发） |
| `priority` | `integer` | default `0` | 优先级偏移（影响 PromptBlock.Priority） |
| `scan_depth` | `integer` | default `0` | 扫描最近 N 条消息（0 = 全部） |
| `position` | `text` | default `'before_template'` | 注入位置 |
| `whole_word` | `boolean` | default `false` | 全词匹配 |
| `enabled` | `boolean` | default `true` | 是否启用 |
| `comment` | `text` | | 备注 |
| `created_at` | `timestamptz` | | 创建时间 |

枚举约束：
- `secondary_logic`: `and_any | and_all | not_any | not_all`
- `position`: `before_template | after_template | at_depth`

索引：
- 普通索引 `idx_worldbook_entries_game_id(game_id)`

## `preset_entries`

条目化 Prompt 组装（复刻 TH 的 preset-entries 系统）。

`injection_order` 直接映射为 `PromptBlock.Priority`，数值越小越靠前。建议范围：

| 范围 | 位置 |
| ---- | ---- |
| 1–9 | 最顶部（高于世界书） |
| 10–509 | 与世界书并列 |
| 510–989 | 记忆/世界书下方 |
| 990–1009 | 主角色人设槽 |
| 1010+ | 底部附加指令 |

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 条目 ID |
| `game_id` | `text` | `NOT NULL`, index | 关联游戏 ID |
| `identifier` | `text` | `NOT NULL` | 人类可读唯一标识（per-game 唯一） |
| `name` | `text` | `NOT NULL` | 显示名 |
| `role` | `text` | `NOT NULL`, default `'system'` | 消息角色 |
| `content` | `text` | | 模板文本（支持 `{{宏}}` 替换） |
| `injection_position` | `text` | `NOT NULL`, default `'system'` | UI 分组提示（不影响排序） |
| `injection_order` | `integer` | `NOT NULL`, default `1000` | 注入顺序（→ PromptBlock.Priority） |
| `enabled` | `boolean` | default `true` | 是否启用 |
| `is_system_prompt` | `boolean` | default `false` | 标记为"主系统提示槽"（角色卡导入时写入此处） |
| `comment` | `text` | | 备注 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `role`: `system | user | assistant`
- `injection_position`: `top | system | bottom`

索引：
- 普通索引 `idx_preset_entries_game_id(game_id)`

## `llm_profiles`

用户自定义 LLM 配置（per-account key vault）。

`params` JSONB 存储该 Profile 默认的采样参数，与 TH `llm-profiles` 的 `generation_params` 字段对齐。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 配置 ID |
| `account_id` | `text` | `NOT NULL`, index | 关联账户 ID |
| `name` | `text` | `NOT NULL` | 配置名称 |
| `provider` | `text` | default `'openai-compatible'` | LLM 提供商 |
| `model_id` | `text` | `NOT NULL` | 模型 ID |
| `base_url` | `text` | | 自定义 API 地址（覆盖默认） |
| `api_key` | `text` | `NOT NULL` | API Key（明文，生产应加密） |
| `params` | `jsonb` | default `'{}'` | 采样参数（temperature, top_p, max_tokens 等） |
| `status` | `text` | default `'active'` | 状态 |
| `is_global` | `boolean` | default `false` | 是否为全局活跃配置 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `provider`: `openai | anthropic | openai-compatible`
- `status`: `active | disabled`

索引：
- 普通索引 `idx_llm_profiles_account_id(account_id)`

## `llm_profile_bindings`

将 LLMProfile 绑定到 global 或特定 session 的 instance slot。

优先级解析规则：`session(slot) > global(slot) > session(*) > global(*) > env 兜底`

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 绑定 ID |
| `account_id` | `text` | `NOT NULL`, index | 关联账户 ID |
| `profile_id` | `text` | `NOT NULL`, index | 关联 LLMProfile ID |
| `scope` | `text` | `NOT NULL`, default `'global'` | 绑定作用域 |
| `scope_id` | `text` | `NOT NULL`, default `'global'` | 作用域 ID（`global` 或 session UUID） |
| `slot` | `text` | `NOT NULL`, default `'*'` | 实例槽位 |
| `params` | `jsonb` | default `'{}'` | 在 Profile Params 之上额外覆盖 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `scope`: `global | session`
- `slot`: `* | narrator | director | verifier | memory`

索引：
- 普通索引 `idx_llm_profile_bindings_account_id(account_id)`
- 普通索引 `idx_llm_profile_bindings_profile_id(profile_id)`

## `regex_profiles`

一组可复用的正则规则集（绑定到游戏模板）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 规则集 ID |
| `game_id` | `text` | `NOT NULL`, index | 关联游戏 ID |
| `name` | `text` | `NOT NULL` | 规则集名称 |
| `enabled` | `boolean` | default `true` | 是否启用 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

索引：
- 普通索引 `idx_regex_profiles_game_id(game_id)`

## `regex_rules`

单条正则替换规则（隶属于 RegexProfile）。

`pattern` 支持两种格式：
- 普通字符串：`hello world`（整体作为 Go regexp 模式）
- `/pattern/flags` 格式：`/hello/i`（flags 支持 `i`=忽略大小写, `m`=多行, `s`=点匹配换行）

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 规则 ID |
| `profile_id` | `text` | `NOT NULL`, index | 关联 RegexProfile ID |
| `name` | `text` | | 规则名称（可选标注） |
| `pattern` | `text` | `NOT NULL` | 正则表达式 |
| `replacement` | `text` | | 替换字符串（支持 `$1` 等捕获组引用） |
| `apply_to` | `text` | default `'ai_output'` | 应用阶段 |
| `order` | `integer` | default `0` | 执行顺序（小→先执行） |
| `enabled` | `boolean` | default `true` | 是否启用 |
| `created_at` | `timestamptz` | | 创建时间 |

枚举约束：
- `apply_to`: `ai_output | user_input | all`

索引：
- 普通索引 `idx_regex_rules_profile_id(profile_id)`

## `materials`

素材库条目（游戏级内容池）。

游戏设计师预先准备大量文本内容，引擎通过 `search_material` 工具按标签/情绪/风格检索匹配条目，注入当前回合 LLM 上下文。标签检索使用 PostgreSQL JSONB `?|` 操作符。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 素材 ID |
| `game_id` | `text` | `NOT NULL`, index | 关联游戏 ID |
| `type` | `text` | `NOT NULL`, default `'text'` | 素材类型 |
| `content` | `text` | `NOT NULL` | 素材正文 |
| `tags` | `jsonb` | default `'[]'` | 通用标签数组 |
| `world_tags` | `jsonb` | default `'[]'` | 世界专属标签数组 |
| `mood` | `text` | | 情绪标签 |
| `style` | `text` | | 风格标签 |
| `function_tag` | `text` | | 功能标签 |
| `used_count` | `integer` | default `0` | 被检索引用次数 |
| `enabled` | `boolean` | default `true` | 是否启用 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

枚举约束：
- `type`: `post | dialogue | description | event | atmosphere`
- `mood`: `happy | sad | tense | melancholy | neutral`
- `style`: `lyrical | aggressive | neutral | humorous`
- `function_tag`: `atmosphere | plot_hook | dialogue | lore`

索引：
- 普通索引 `idx_materials_game_id(game_id)`

## `preset_tools`

创作者自定义工具（HTTP 回调执行，动态注入 Agentic Loop）。

引擎收到 LLM `tool_call` 后，向 `endpoint` POST `{session_id, floor_id, args}`，期望响应任意 JSON（直接作为 tool message 内容）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 工具 ID |
| `game_id` | `text` | `NOT NULL`, index | 关联游戏 ID |
| `name` | `text` | `NOT NULL` | 工具名（LLM 调用时用，per-game 唯一） |
| `description` | `text` | | 工具描述（注入 LLM） |
| `parameters` | `jsonb` | default `'{}'` | JSON Schema object（LLM 参数定义） |
| `endpoint` | `text` | `NOT NULL` | HTTP POST URL（只允许 http/https） |
| `timeout_ms` | `integer` | default `5000` | 超时时间（毫秒） |
| `enabled` | `boolean` | default `true` | 是否启用 |
| `created_at` | `timestamptz` | | 创建时间 |
| `updated_at` | `timestamptz` | | 更新时间 |

索引：
- 普通索引 `idx_preset_tools_game_id(game_id)`

## `tool_execution_records`

记录 Agentic Loop 中每次工具调用的入参、出参和耗时。用于审计、调试和 replay 决策（结合 `ReplaySafety` 等级）。

| 列名 | 类型 | 约束/默认值 | 说明 |
| ---- | ---- | ----------- | ---- |
| `id` | `uuid` | PK, default `gen_random_uuid()` | 记录 ID |
| `session_id` | `text` | `NOT NULL`, index | 所属会话 ID |
| `floor_id` | `text` | `NOT NULL`, index | 所属楼层 ID |
| `page_id` | `text` | `NOT NULL`, index | 所属消息页 ID |
| `tool_name` | `text` | `NOT NULL` | 工具名称 |
| `params` | `text` | | 调用参数（JSON 字符串） |
| `result` | `text` | | 返回结果（JSON 字符串） |
| `duration_ms` | `int8` | | 执行耗时（毫秒） |
| `created_at` | `timestamptz` | | 创建时间 |

索引：
- 普通索引 `idx_tool_execution_records_session_id(session_id)`
- 普通索引 `idx_tool_execution_records_floor_id(floor_id)`
- 普通索引 `idx_tool_execution_records_page_id(page_id)`

---

## 列表接口约定

所有列表接口统一支持：
- 分页：`limit`（默认 50）、`offset`（默认 0）
- 过滤：各实体特有过滤字段（`game_id`、`session_id`、`floor_id` 等）

统一返回：
- `code`: `0` 表示成功
- `data`: 当前页数据（数组或单对象）
