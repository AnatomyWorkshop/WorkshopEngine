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
	ID             string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID         string         `gorm:"not null;index"                                json:"game_id"`
	UserID         string         `gorm:"index"                                         json:"user_id"`
	Title          string         `gorm:"default:''"                                    json:"title"`           // 会话标题（可选）
	Status         string         `gorm:"default:'active'"                              json:"status"`          // active | archived
	Variables      datatypes.JSON `gorm:"type:jsonb;default:'{}'"                       json:"variables"`       // Chat 级持久变量
	MemorySummary  string         `gorm:"type:text"                                     json:"memory_summary"`  // 最新摘要快照（由 Worker 异步写入）
	FloorCount     int            `gorm:"default:0"                                     json:"floor_count"`     // 已完成回合数，触发摘要的阈值依据
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
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
	Seq       int         `gorm:"not null"                                      json:"seq"`    // 楼层时序，决定对话顺序
	Status    FloorStatus `gorm:"not null;default:'draft'"                      json:"status"`
	CreatedAt time.Time   `json:"created_at"`
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
	ID         string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	SessionID  string         `gorm:"not null;index"                                json:"session_id"`
	Content    string         `gorm:"type:text;not null"                            json:"content"`
	Type       MemoryType     `gorm:"not null;default:'summary'"                    json:"type"`
	Importance float64        `gorm:"default:1.0"                                   json:"importance"` // 衰减排序权重
	SourceFloor int           `json:"source_floor"` // 来自第几楼层（可溯源）
	Deprecated bool           `gorm:"default:false"                                 json:"deprecated"` // 过时标记
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
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
	Type                 string         `gorm:"not null;default:'visual_novel'"               json:"type"` // visual_novel | narrative | simulator
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
	ID         string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID     string         `gorm:"not null;index"                                json:"game_id"` // 关联游戏（未来可改为多对多）
	Keys       datatypes.JSON `gorm:"type:jsonb;default:'[]'"                       json:"keys"`    // []string 触发关键词
	Content    string         `gorm:"type:text;not null"                            json:"content"`
	Constant   bool           `gorm:"default:false"                                 json:"constant"` // 无条件常驻
	Priority   int            `gorm:"default:0"                                     json:"priority"` // 优先级偏移
	Enabled    bool           `gorm:"default:true"                                  json:"enabled"`
	Comment    string         `json:"comment"` // 创作者注释
	CreatedAt  time.Time      `json:"created_at"`
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
