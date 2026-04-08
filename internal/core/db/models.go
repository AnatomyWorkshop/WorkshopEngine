package db

import (
	"time"

	"gorm.io/datatypes"
)

// ──────────────────────────────────────────────────────
// 三层消息结构（复刻 TavernHeadless）
// Session → Floor → MessagePage → Message
// ──────────────────────────────────────────────────────

// GameSession 一次完整的游玩会话
type GameSession struct {
	ID                  string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID              string         `gorm:"not null;index"                                json:"game_id"`
	UserID              string         `gorm:"index"                                         json:"user_id"`
	Title               string         `gorm:"default:''"                                    json:"title"`               // 会话标题（可选）
	Status              string         `gorm:"default:'active'"                              json:"status"`              // active | archived
	Variables           datatypes.JSON `gorm:"type:jsonb;default:'{}'"                       json:"variables"`           // Chat 级持久变量
	MemorySummary       string         `gorm:"type:text"                                     json:"memory_summary"`      // 最新摘要快照（由 Worker 异步写入）
	FloorCount          int            `gorm:"default:0"                                     json:"floor_count"`         // 已完成回合数，触发摘要的阈值依据
	// 角色卡注入管线（M11）
	CharacterCardID     string         `gorm:"default:''"                                    json:"character_card_id"`   // 关联角色卡 ID（空 = 无角色卡绑定）
	CharacterSnapshot   datatypes.JSON `gorm:"type:jsonb;default:'null'"                     json:"character_snapshot"`  // pin 策略时的角色卡快照（session 创建时冻结）
	// 并发生成保护（M13）
	Generating          bool           `gorm:"default:false"                                 json:"generating"`          // true = 正在生成，防并发
	GenerationMode      string         `gorm:"default:'reject'"                              json:"generation_mode"`     // reject（默认，直接 409）| queue（排队，P-3K 扩展）
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// FloorStatus 楼层状态机（提交后不可改 —— 核心设计原则）
type FloorStatus string

const (
	FloorDraft      FloorStatus = "draft"      // 刚创建，等待生成
	FloorGenerating FloorStatus = "generating" // 正在调用 LLM
	FloorCommitted  FloorStatus = "committed"  // 已提交，内容锁定
	FloorFailed     FloorStatus = "failed"     // 生成失败，保留现场
)

// Floor 一个游戏回合
type Floor struct {
	ID        string      `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID string      `gorm:"not null;index"                                json:"session_id"`
	Seq       int         `gorm:"not null"                                      json:"seq"`       // 楼层时序（session 内全局唯一，跨分支共享计数器）
	BranchID  string      `gorm:"not null;default:'main';index"                 json:"branch_id"` // 所属分支（P-3G）
	Status    FloorStatus `gorm:"not null;default:'draft'"                      json:"status"`
	CreatedAt time.Time   `json:"created_at"`
}

// SessionBranch Session 内时间线分支元数据（P-3G）
//
// 每条记录代表从某个楼层序号分叉出的新分支。
// 历史重建规则：branch "foo" 的完整历史 = 主分支楼层（seq ≤ OriginSeq）+ "foo" 分支所有楼层。
// "main" 分支是隐式主干，不在此表存储。
type SessionBranch struct {
	ID           string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID    string    `gorm:"not null;index"                                 json:"session_id"`
	BranchID     string    `gorm:"not null"                                       json:"branch_id"`      // 分支标识符（用户可读，如 "branch-abc123"）
	ParentBranch string    `gorm:"not null;default:'main'"                        json:"parent_branch"`  // 父分支 ID（当前仅支持从 main 分叉）
	OriginSeq    int       `gorm:"not null"                                       json:"origin_seq"`     // 分叉点楼层序号（含该楼层）
	CreatedAt    time.Time `json:"created_at"`
}

// MessagePage 一个楼层内的一次生成尝试（即酒馆的 Swipe）
type MessagePage struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	FloorID   string         `gorm:"not null;index"                                json:"floor_id"`
	IsActive  bool           `gorm:"default:true"                                  json:"is_active"` // 同一楼层只有一个 active
	Messages  datatypes.JSON `gorm:"type:jsonb;default:'[]'"                       json:"messages"`  // []Message{Role, Content}
	PageVars  datatypes.JSON `gorm:"type:jsonb;default:'{}'"                       json:"page_vars"` // Page 沙箱变量（重试时直接丢弃）
	TokenUsed int            `json:"token_used"`
	CreatedAt time.Time      `json:"created_at"`
}

// ──────────────────────────────────────────────────────
// 记忆系统
// ──────────────────────────────────────────────────────

// MemoryType 记忆类型（复刻 TH）
type MemoryType string

const (
	MemoryFact     MemoryType = "fact"      // 明确事实（好感度 = 50，物品已拿走）
	MemorySummary  MemoryType = "summary"   // 剧情摘要
	MemoryOpenLoop MemoryType = "open_loop" // 待解决的悬念
)

// Memory 一条记忆条目
type Memory struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID   string         `gorm:"not null;index"                                json:"session_id"`
	FactKey     string         `gorm:"index;default:''"                              json:"fact_key"`    // 结构化事实的稳定键（type=fact 时有效，空=自由文本摘要）
	Content     string         `gorm:"type:text;not null"                            json:"content"`
	Type        MemoryType     `gorm:"not null;default:'summary'"                    json:"type"`
	Importance  float64        `gorm:"default:1.0"                                   json:"importance"`  // 衰减排序权重
	SourceFloor int            `json:"source_floor"`                                                  // 来自第几楼层（可溯源）
	Deprecated  bool           `gorm:"default:false"                                 json:"deprecated"` // 过时标记
	// StageTags 阶段标签（空数组 = 无阶段限制，始终注入；非空 = 仅在 game_stage 变量匹配时注入）。
	// 用于多幕叙事：第一幕的调查线索不应在第三幕结局时占用 token。
	StageTags   datatypes.JSON `gorm:"type:jsonb;default:'[]'"                       json:"stage_tags"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// MemoryRelation 记忆边的关系类型
type MemoryRelation string

const (
	// MemoryRelationUpdates 新事实更新/取代旧事实（fact_key 冲突时写入）
	MemoryRelationUpdates MemoryRelation = "updates"
	// MemoryRelationContradicts 两条事实相互矛盾（由 Memory Lint 或 LLM 标记）
	MemoryRelationContradicts MemoryRelation = "contradicts"
	// MemoryRelationSupports 一条事实支持/强化另一条事实
	MemoryRelationSupports MemoryRelation = "supports"
	// MemoryRelationResolves 摘要或事实解决了一个悬念（open_loop）
	MemoryRelationResolves MemoryRelation = "resolves"
)

// MemoryEdge 记忆条目之间的有向关系边
//
// 设计说明：
//   - 不参与 Prompt 注入，仅用于溯源、审计和 Memory Lint
//   - from_id → to_id 表示"from 对 to 施加 relation"（如 新事实 updates 旧事实）
//   - CASCADE 删除：memory 条目被物理删除时关联 edge 自动清除
//   - 对应 TH memory_edge 表，简化为 WE 所需的 4 种 relation（去掉 TH 双层压缩专用的 derived_from / compacts）
type MemoryEdge struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID string         `gorm:"not null;index"                                json:"session_id"`
	FromID    string         `gorm:"not null;index"                                json:"from_id"` // 施加关系的一方
	ToID      string         `gorm:"not null;index"                                json:"to_id"`   // 被施加关系的一方
	Relation  MemoryRelation `gorm:"not null"                                      json:"relation"`
	CreatedAt time.Time      `json:"created_at"`
}

// ──────────────────────────────────────────────────────
// 创作层实体
// ──────────────────────────────────────────────────────

// CharacterCard 角色卡（从 PNG 导入后结构化存储）
type CharacterCard struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Slug      string         `gorm:"uniqueIndex;not null"                          json:"slug"`
	Name      string         `gorm:"not null"                                      json:"name"`
	Spec      string         `gorm:"default:'chara_card_v2'"                       json:"spec"` // chara_card_v2 | chara_card_v3
	Data      datatypes.JSON `gorm:"type:jsonb"                                    json:"data"` // 完整角色卡 JSON
	AvatarURL string         `json:"avatar_url"`
	Tags      datatypes.JSON `gorm:"type:jsonb;default:'[]'"                       json:"tags"`
	IsPublic  bool           `gorm:"default:true"                                  json:"is_public"`
	AuthorID  string         `gorm:"index"                                         json:"author_id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// GameTemplate 游戏模板
type GameTemplate struct {
	ID                   string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Slug                 string         `gorm:"uniqueIndex;not null"                          json:"slug"`
	Title                string         `gorm:"not null"                                      json:"title"`
	Type                 string         `gorm:"not null;default:'text'"                       json:"type"` // text（纯文字）| light（轻前端）| rich（重前端）| 创作者自由描述
	Description          string         `json:"description"`
	SystemPromptTemplate string         `gorm:"type:text"                                     json:"system_prompt_template"` // 支持 {{宏}} 变量展开
	Config               datatypes.JSON `gorm:"type:jsonb;default:'{}'"                       json:"config"`                // 初始变量、资产配置等
	CoverURL             string         `json:"cover_url"`
	Status               string         `gorm:"default:'draft'"                               json:"status"` // draft | published
	AuthorID             string         `gorm:"index"                                         json:"author_id"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// WorldbookEntry 世界书词条（独立于游戏模板，支持多对多）
type WorldbookEntry struct {
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID         string         `gorm:"not null;index"                                json:"game_id"`
	Keys           datatypes.JSON `gorm:"type:jsonb;default:'[]'"                       json:"keys"`            // []string 主关键词（至少一条匹配即触发）
	SecondaryKeys  datatypes.JSON `gorm:"type:jsonb;default:'[]'"                       json:"secondary_keys"`  // []string 次级关键词
	SecondaryLogic string         `gorm:"default:'and_any'"                             json:"secondary_logic"` // and_any | and_all | not_any | not_all
	Content        string         `gorm:"type:text;not null"                            json:"content"`
	Constant       bool           `gorm:"default:false"                                 json:"constant"`      // 无条件常驻
	Priority       int            `gorm:"default:0"                                     json:"priority"`      // 优先级偏移
	ScanDepth      int            `gorm:"default:0"                                     json:"scan_depth"`    // 扫描最近 N 条消息（0 = 全部）
	Position       string         `gorm:"default:'before_template'"                     json:"position"`      // before_template | after_template | at_depth
	Depth          int            `gorm:"default:0"                                     json:"depth"`         // at_depth 时距底部的距离（0=最底，1=倒数第一，依此类推）
	WholeWord      bool           `gorm:"default:false"                                 json:"whole_word"`    // 全词匹配
	Enabled        bool           `gorm:"default:true"                                  json:"enabled"`
	Group          string         `gorm:"default:''"                                    json:"group"`         // 互斥分组名（空 = 不参与分组裁剪）
	GroupWeight    float64        `gorm:"default:0"                                     json:"group_weight"`  // 同组内优先级（降序，最高权重的词条被保留）
	Comment        string         `json:"comment"`
	CreatedAt      time.Time      `json:"created_at"`
}

// PresetEntry 条目化 Prompt 组装（复刻 TH 的 preset-entries 系统）。
//
// 取代单一 GameTemplate.SystemPromptTemplate：创作者将 System Prompt 拆分为
// 多个有名称、有顺序的条目，引擎在每回合按 InjectionOrder 排序后合并注入。
//
// # InjectionOrder（注入顺序）
//
// 直接映射到 Pipeline PromptBlock.Priority；数值越小越靠前。
// 建议范围参考（与现有节点保持一致）：
//   - 1–9   : 最顶部（高于世界书的 10+ 范围）
//   - 10–509: 与世界书并列
//   - 510–989: 记忆/世界书下方
//   - 990–1009: 主角色人设槽（与 SystemPromptTemplate 同级）
//   - 1010+  : 底部附加指令
//
// # InjectionPosition（注入位置）
//
// 字符串标签，供前端 UI 分组展示，不影响后端排序逻辑：
//   - "top"    : 顶部（InjectionOrder 建议 0–9）
//   - "system" : 主系统槽（建议 990–1009）
//   - "bottom" : 底部（建议 1010+）
type PresetEntry struct {
	ID                string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID            string    `gorm:"not null;index"                                  json:"game_id"`
	Identifier        string    `gorm:"not null"                                        json:"identifier"`         // 人类可读唯一标识（per-game 唯一），如 "main_persona"
	Name              string    `gorm:"not null"                                        json:"name"`               // 显示名，如 "主角色描述"
	Role              string    `gorm:"not null;default:'system'"                       json:"role"`               // system | user | assistant
	Content           string    `gorm:"type:text"                                       json:"content"`            // 模板文本，支持 {{宏}} 替换
	InjectionPosition string    `gorm:"not null;default:'system'"                       json:"injection_position"` // top | system | bottom（UI 分组提示）
	InjectionOrder    int       `gorm:"not null;default:1000"                           json:"injection_order"`    // 直接作为 PromptBlock.Priority
	Enabled           bool      `gorm:"default:true"                                    json:"enabled"`
	IsSystemPrompt    bool      `gorm:"default:false"                                   json:"is_system_prompt"` // 标记为"主系统提示槽"（角色卡导入时写入此处）
	Comment           string    `json:"comment"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────────────
// LLM 配置层（复刻 TH 的 llm_profiles）
// 用户可以保存多个 LLM Provider 配置，并指定哪个为当前活跃配置
// ──────────────────────────────────────────────────────

// LLMProfile 用户自定义 LLM 配置（per-account key vault）
//
// Params 存储该 Profile 默认的采样参数（JSON）：
//
//	{"temperature":0.8,"top_p":0.95,"max_tokens":2048,...}
//
// 与 TH llm-profiles 的 generation_params 字段完全对齐。
// Binding 上的 Params 具有更高优先级，可逐字段覆盖。
type LLMProfile struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	AccountID string         `gorm:"not null;index"                                  json:"account_id"` // 关联账户/用户
	Name      string         `gorm:"not null"                                        json:"name"`       // 配置名称（如 "我的 GPT-4"）
	Provider  string         `gorm:"default:'openai-compatible'"                     json:"provider"`   // openai | anthropic | openai-compatible
	ModelID   string         `gorm:"not null"                                        json:"model_id"`   // 模型 ID（如 "gpt-4o-mini"）
	BaseURL   string         `json:"base_url"`                                                          // 可选，覆盖默认 API 地址
	APIKey    string         `gorm:"not null"                                        json:"-"`          // 明文 Key（生产应加密，此处简化）
	Params    datatypes.JSON `gorm:"type:jsonb;default:'{}'"                         json:"params"`     // 采样参数覆盖（见 GenParams）
	Status    string         `gorm:"default:'active'"                                json:"status"`     // active | disabled
	IsGlobal  bool           `gorm:"default:false"                                   json:"is_global"`  // 是否为全局活跃配置
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// LLMProfileBinding 将 LLMProfile 绑定到 global 或特定 session 的 instance slot。
//
// Params 允许在同一个 Profile 的不同 slot 上使用不同的采样参数
//（例如 narrator 用高温，memory 用低温）。
type LLMProfileBinding struct {
	ID        string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	AccountID string         `gorm:"not null;index"                                  json:"account_id"`
	ProfileID string         `gorm:"not null;index"                                  json:"profile_id"` // 关联 LLMProfile
	Scope     string         `gorm:"not null;default:'global'"                       json:"scope"`      // global | session
	ScopeID   string         `gorm:"not null;default:'global'"                       json:"scope_id"`   // "global" 或 session UUID
	Slot      string         `gorm:"not null;default:'*'"                            json:"slot"`       // * | narrator | memory
	Params    datatypes.JSON `gorm:"type:jsonb;default:'{}'"                         json:"params"`     // 在 Profile Params 之上额外覆盖
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ──────────────────────────────────────────────────────
// Regex 后处理系统（复刻 TH adapters-sillytavern regex-engine）
// RegexProfile 可绑定到游戏；RegexRule 是独立的正则替换规则
// ──────────────────────────────────────────────────────

// RegexProfile 一组可复用的正则规则集（绑定到游戏模板）
type RegexProfile struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID    string    `gorm:"not null;index"                                  json:"game_id"` // 关联游戏（未来可设 null 表示全局）
	Name      string    `gorm:"not null"                                        json:"name"`
	Enabled   bool      `gorm:"default:true"                                    json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RegexRule 单条正则替换规则（隶属于 RegexProfile）。
//
// Pattern 支持两种格式：
//   - 普通字符串：`hello world`（整体作为 Go regexp 模式）
//   - /pattern/flags 格式：`/hello/i`（flags 支持 i=忽略大小写, m=多行, s=点匹配换行）
//
// ApplyTo 控制规则作用阶段：
//   - "ai_output" : 作用于 LLM 返回文本（ParsedResponse.Narrative）
//   - "user_input": 作用于用户输入（TurnRequest.UserInput）
//   - "all"       : 两者均应用
type RegexRule struct {
	ID          string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	ProfileID   string    `gorm:"not null;index"                                  json:"profile_id"`   // 关联 RegexProfile
	Name        string    `json:"name"`                                                                // 可选标注
	Pattern     string    `gorm:"not null"                                        json:"pattern"`      // 正则表达式
	Replacement string    `json:"replacement"`                                                         // 替换字符串（支持 $1 等捕获组引用）
	ApplyTo     string    `gorm:"default:'ai_output'"                             json:"apply_to"`     // ai_output | user_input | all
	Order       int       `gorm:"default:0"                                       json:"order"`        // 执行顺序（小→先执行）
	Enabled     bool      `gorm:"default:true"                                    json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// PresetTool 创作者自定义工具（HTTP 回调执行，动态注入 Agentic Loop）。
//
// 引擎收到 LLM tool_call 后，向 Endpoint POST {session_id, floor_id, args}，
// 期望响应 {"result": "..."} 或任意 JSON（直接作为 tool message 内容）。
type PresetTool struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID      string         `gorm:"not null;index"                                  json:"game_id"`
	Name        string         `gorm:"not null"                                        json:"name"`        // LLM 调用时的工具名（per-game 唯一）
	Description string         `gorm:"type:text"                                       json:"description"` // 注入 LLM 的工具描述
	Parameters  datatypes.JSON `gorm:"type:jsonb;default:'{}'"                         json:"parameters"`  // JSON Schema object
	Endpoint    string         `gorm:"not null"                                        json:"endpoint"`    // HTTP POST URL
	TimeoutMs   int            `gorm:"default:5000"                                    json:"timeout_ms"`
	Enabled     bool           `gorm:"default:true"                                    json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// PromptSnapshot 记录某个 committed floor 实际生成时使用的 Prompt 资源版本和诊断信息。
//
// 复刻 TH 的 prompt_snapshot 表：
//   - 每个已提交楼层都有一条快照，冻结本次生成的 preset/worldbook/regex 状态
//   - ActivatedWorldbookIDs：本回合触发的世界书词条 ID 列表（JSON 数组）
//   - PresetHits：生效的 Preset Entry 数量
//   - EstTokens：粗估 Prompt Token 总数（BPE 兼容的启发式估算）
//   - VerifyPassed：Verifier 槽校验结果（null=未配置 verifier，true/false=校验通过/拒绝）
//   - VerifyNote：Verifier 给出的简短说明（通过时为空）
//
// 对应 GET /sessions/:id/floors/:fid/snapshot。
type PromptSnapshot struct {
	ID                    string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID             string         `gorm:"not null;index"                                 json:"session_id"`
	FloorID               string         `gorm:"not null;uniqueIndex"                           json:"floor_id"` // 1 floor : 1 snapshot
	ActivatedWorldbookIDs datatypes.JSON `gorm:"type:jsonb;default:'[]'"                        json:"activated_worldbook_ids"` // []string
	PresetHits            int            `json:"preset_hits"`      // 生效的 PresetEntry 数量
	WorldbookHits         int            `json:"worldbook_hits"`   // 命中的世界书词条数量
	EstTokens             int            `json:"est_tokens"`       // 粗估 token 总数
	VerifyPassed          *bool          `json:"verify_passed"`    // null=未运行，true=通过，false=拒绝
	VerifyNote            string         `json:"verify_note"`      // Verifier 说明（通过时空串）
	CreatedAt             time.Time      `json:"created_at"`
}

// Material 素材库条目（游戏级内容池）。
//
// ToolExecutionRecord 记录 Agentic Loop 中每次工具调用的入参、出参和耗时。
// 用于审计、调试和 replay 决策（结合 ReplaySafety 等级）。
type ToolExecutionRecord struct {
	ID         string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID  string    `gorm:"not null;index"                                 json:"session_id"`
	FloorID    string    `gorm:"not null;index"                                 json:"floor_id"`
	PageID     string    `gorm:"not null;index"                                 json:"page_id"`
	ToolName   string    `gorm:"not null"                                       json:"tool_name"`
	Params     string    `gorm:"type:text"                                      json:"params"`      // JSON 字符串
	Result     string    `gorm:"type:text"                                      json:"result"`      // JSON 字符串
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// 设计初衷：游戏设计师预先准备大量文本内容（发帖体、对话片段、氛围描写等），
// 引擎通过 search_material 工具按标签/情绪/风格检索匹配条目，
// 注入当前回合 LLM 上下文，实现"素材库驱动的 Tier3 NPC 内容生成"。
//
// 标签检索使用 PostgreSQL JSONB `?|` 操作符（数组包含任意一项即匹配）。
type Material struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID      string         `gorm:"not null;index"                                  json:"game_id"`      // 关联 GameTemplate
	Type        string         `gorm:"not null;default:'text'"                         json:"type"`         // post|dialogue|description|event|atmosphere
	Content     string         `gorm:"type:text;not null"                              json:"content"`      // 素材正文
	Tags        datatypes.JSON `gorm:"type:jsonb;default:'[]'"                         json:"tags"`         // []string 通用标签
	WorldTags   datatypes.JSON `gorm:"type:jsonb;default:'[]'"                         json:"world_tags"`   // []string 世界专属标签
	Mood        string         `json:"mood"`                                                                // happy|sad|tense|melancholy|neutral...
	Style       string         `json:"style"`                                                               // lyrical|aggressive|neutral|humorous...
	FunctionTag string         `json:"function_tag"`                                                        // atmosphere|plot_hook|dialogue|lore...
	UsedCount   int            `gorm:"default:0"                                       json:"used_count"`   // 被检索引用次数
	Enabled     bool           `gorm:"default:true"                                    json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
