package prompt_ir

// PresetEntry Prompt 流水线中的一个条目（来自 DB PresetEntry，去掉 DB 字段）
type PresetEntry struct {
	Identifier        string
	Name              string
	Role              string // system | user | assistant
	Content           string
	InjectionPosition string // top | system | bottom（UI 分组，不影响排序）
	InjectionOrder    int    // 直接作为 PromptBlock.Priority
	Enabled           bool
	IsSystemPrompt    bool
}

// WorldbookEntry 代表一条世界书/Lorebook规则
type WorldbookEntry struct {
	ID             string   `json:"id"`
	Keys           []string `json:"keys"`            // 主关键词（任意一条匹配即触发）
	SecondaryKeys  []string `json:"secondary_keys"`  // 次级关键词（与 SecondaryLogic 配合）
	SecondaryLogic string   `json:"secondary_logic"` // and_any | and_all | not_any | not_all（默认 and_any）
	Content        string   `json:"content"`
	Constant       bool     `json:"constant"`
	Priority       int      `json:"priority"`
	ScanDepth      int      `json:"scan_depth"` // 0 = 全部消息；N = 只扫最近 N 条
	Position       string   `json:"position"`   // before_template | after_template | at_depth
	Depth          int      `json:"depth"`      // at_depth 时距底部的距离（0=最底，1=倒数第一，依此类推）
	WholeWord      bool     `json:"whole_word"`
	Enabled        bool     `json:"enabled"`
	Group          string   `json:"group"`        // 互斥分组名（空 = 不参与分组裁剪）
	GroupWeight    float64  `json:"group_weight"` // 同组内优先级（降序，最高权重的词条被保留）
}

// AtDepthBlock 代表一条需要嵌入对话历史特定深度的世界书词条。
// 由 WorldbookNode 路由到 ContextData.AtDepthBlocks，
// Runner 在完成普通 Block 组装后将其插入最终消息列表的对应位置。
type AtDepthBlock struct {
	Content  string // 展开后的词条内容（已完成宏替换）
	Depth    int    // 距底部的消息数（0=最底，1=倒数第一条前，依此类推）
	Priority int    // 同深度时按 Priority 升序排序（数值越小越靠前）
	EntryID  string // 来源词条 ID（用于 PromptSnapshot）
}

// RegexRule Prompt 后处理正则规则（来自 DB RegexRule，去掉 DB 字段）
type RegexRule struct {
	Pattern     string // 正则表达式（支持 /pattern/flags 格式）
	Replacement string // 替换字符串（支持 $1 捕获组）
	ApplyTo     string // ai_output | user_input | all
	Enabled     bool
}

// GameConfig 是执行一回合所需的静态模板配置 (由外层从 DB 加载)
type GameConfig struct {
	SystemPromptTemplate string           // 系统提示词模板（单字符串兜底，支持 {{宏}}）
	WorldbookEntries     []WorldbookEntry // 该游戏挂载的所有世界书词条
	MemorySummary        string           // 之前异步生成的长期记忆摘要
	PresetEntries        []PresetEntry    // 条目化 Prompt 组装（优先于 SystemPromptTemplate）
	RegexRules           []RegexRule      // 后处理正则规则（ai_output / user_input / all）

	// MemoryLabel 注入记忆摘要时的标签前缀（默认 "[Memory Summary]\n"）。
	// 可通过 GameTemplate.Config.memory_label 按游戏覆盖。
	MemoryLabel string

	// FallbackOptions parser fallback 时的默认选项（默认为空，由游戏模板配置）。
	// 对应 GameTemplate.Config.fallback_options。
	FallbackOptions []string

	// WorldbookGroupCap 同组词条最多保留数量（默认 1）。
	// 同组词条激活后按 GroupWeight 降序排列，超出 cap 的词条被丢弃，不参与注入。
	// 对应 GameTemplate.Config.worldbook_group_cap。0 表示使用默认值 1。
	WorldbookGroupCap int

	// WorldbookTokenBudget 世界书注入的总 token 上限（默认 0 = 不限制）。
	// 激活且通过 GroupCap 裁剪后，若所有非常驻词条的累计 token 超出预算，
	// 则按 Priority 升序（数值越小越重要）逐条保留，超出预算的词条丢弃。
	// Constant=true 的词条始终注入，不占用此预算。
	// 对应 GameTemplate.Config.worldbook_token_budget。
	WorldbookTokenBudget int
}

// ContextData 是贯穿整个流水线的上下文载体。
type ContextData struct {
	Mode           PipelineMode
	Config         GameConfig            // 静态游戏配置
	Variables      map[string]any        // 来自 Variable Sandbox Flatten 的动态变量
	RecentMessages []Message             // 最近的 N 条历史记录 (用于世界书触发判断)
	Blocks         []PromptBlock         // 输出的 IR 块
	TokenBudget    int                   // Token 上限预留

	// ST 宏展开所需字段（由 game_loop 构建，各 Node 通过 macros.Expand 使用）
	// 对应 {{char}} / {{user}} / {{persona}} 宏
	CharName    string // {{char}} → 角色名（来自 template.Config.char_name 或角色卡名）
	UserName    string // {{user}} → 玩家名（来自 template.Config.player_name，默认"你"）
	PersonaName string // {{persona}} → 人设显示名（空时回退至 CharName）

	// CharacterDescription 角色卡自动注入内容（M11 CharacterInjectionNode 使用）。
	// 由 game_loop 从 GameSession.CharacterSnapshot 或 CharacterCard DB 记录构建，
	// 空字符串表示无角色卡绑定或已被 CharacterInjectionNode 跳过。
	CharacterDescription string

	// 流水线执行后填充，供调用方读取（不参与 Prompt 组装）
	ActivatedWorldbookIDs []string     // 本回合命中的世界书词条 ID 列表（用于 PromptSnapshot）
	AtDepthBlocks         []AtDepthBlock // position=at_depth 的词条，由 Runner 插入历史特定位置
}

// Message 代表一条用于上下文匹配和最后生成的历史消息
type Message struct {
	Role    string
	Content string
}

// PipelineMode 运行模式
type PipelineMode string

const (
	ModeCompatStrict PipelineMode = "compat_strict"
	ModeNative       PipelineMode = "native"
)

// BlockType Prompt 组成块的类别
type BlockType string

const (
	BlockSystem    BlockType = "system"
	BlockPreset    BlockType = "preset"    // 条目化 Prompt 组装（PresetEntry）
	BlockWorldbook BlockType = "worldbook"
	BlockMemory    BlockType = "memory"
	BlockHistory   BlockType = "history"
	BlockUser      BlockType = "user"
)

// PromptBlock 提示词中间表示
type PromptBlock struct {
	Type     BlockType
	Role     string // "system", "user", "assistant"
	Content  string
	Priority int // 排序权重：数值越小，越靠上（越靠近最顶部的 System 角色）
}

// PipelineNode 流水线节点接口
type PipelineNode interface {
	Name() string
	Process(ctx *ContextData) error
}
