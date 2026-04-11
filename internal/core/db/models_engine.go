package db

import (
	"time"

	"gorm.io/datatypes"
)

// ──────────────────────────────────────────────────────
// 引擎层私有模型（仅 internal/engine/ 使用）
// 三层消息结构（复刻 TavernHeadless）
// Session → Floor → MessagePage → Message
// ──────────────────────────────────────────────────────

// GameSession 一次完整的游玩会话
type GameSession struct {
	ID                  string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID              string         `gorm:"not null;index"                                json:"game_id"`
	UserID              string         `gorm:"index"                                         json:"user_id"`
	Title               string         `gorm:"default:''"                                    json:"title"`
	Status              string         `gorm:"default:'active'"                              json:"status"`   // active | archived
	IsPublic            bool           `gorm:"default:false"                                 json:"is_public"` // 玩家公开存档（供游记分享）
	Variables           datatypes.JSON `gorm:"type:jsonb;default:'{}'"                       json:"variables"`
	MemorySummary       string         `gorm:"type:text"                                     json:"memory_summary"`
	FloorCount          int            `gorm:"default:0"                                     json:"floor_count"`
	CharacterCardID     string         `gorm:"default:''"                                    json:"character_card_id"`
	CharacterSnapshot   datatypes.JSON `gorm:"type:jsonb;default:'null'"                     json:"character_snapshot"`
	Generating          bool           `gorm:"default:false"                                 json:"generating"`
	GenerationMode      string         `gorm:"default:'reject'"                              json:"generation_mode"`
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

// ──────────────────────────────────────────────────────
// 后台任务运行时（P-4G）
// ──────────────────────────────────────────────────────

// JobStatus 后台任务状态机
type JobStatus string

const (
	JobQueued JobStatus = "queued" // 等待执行
	JobLeased JobStatus = "leased" // 已被 worker 租约
	JobDone   JobStatus = "done"   // 执行成功
	JobFailed JobStatus = "failed" // 执行失败（可重试）
	JobDead   JobStatus = "dead"   // 死信（超过最大重试次数）
)

// RuntimeJob DB 持久化的后台任务
type RuntimeJob struct {
	ID         string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	JobType    string         `gorm:"not null;index"                                 json:"job_type"`    // memory_consolidation / ...
	SessionID  string         `gorm:"not null;index"                                 json:"session_id"`
	Payload    datatypes.JSON `gorm:"type:jsonb;default:'{}'"                        json:"payload"`
	Status     JobStatus      `gorm:"not null;default:'queued';index"                json:"status"`
	LeaseUntil *time.Time     `gorm:"index"                                          json:"lease_until"`
	RetryCount int            `gorm:"default:0"                                      json:"retry_count"`
	MaxRetries int            `gorm:"default:3"                                      json:"max_retries"`
	ErrorLog   string         `gorm:"type:text"                                      json:"error_log"`
	DedupeKey  string         `gorm:"uniqueIndex"                                    json:"dedupe_key"`  // job_type:session_id 去重
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

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
