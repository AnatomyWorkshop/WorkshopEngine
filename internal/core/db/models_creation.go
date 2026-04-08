package db

import (
	"time"

	"gorm.io/datatypes"
)

// ──────────────────────────────────────────────────────
// 创作层实体（仅 internal/creation/ 使用）
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
