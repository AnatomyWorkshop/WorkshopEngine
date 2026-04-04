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
	ID       string   `json:"id"`
	Keys     []string `json:"keys"`     // 触发关键词，如果为空且 Constant=true 则常驻
	Content  string   `json:"content"`  // 注入的文本内容
	Constant bool     `json:"constant"` // 是否无视关键词常驻
	Priority int      `json:"priority"` // 优先级，影响在 PromptBlock 中的排序
	Enabled  bool     `json:"enabled"`
}

// GameConfig 是执行一回合所需的静态模板配置 (由外层从 DB 加载)
type GameConfig struct {
	SystemPromptTemplate string           // 系统提示词模板（单字符串兜底，支持 {{宏}}）
	WorldbookEntries     []WorldbookEntry // 该游戏挂载的所有世界书词条
	MemorySummary        string           // 之前异步生成的长期记忆摘要
	PresetEntries        []PresetEntry    // 条目化 Prompt 组装（优先于 SystemPromptTemplate）

	// MemoryLabel 注入记忆摘要时的标签前缀（默认 "[Memory Summary]\n"）。
	// 可通过 GameTemplate.Config.memory_label 按游戏覆盖。
	MemoryLabel string

	// FallbackOptions parser fallback 时的默认选项（默认为空，由游戏模板配置）。
	// 对应 GameTemplate.Config.fallback_options。
	FallbackOptions []string
}

// ContextData 是贯穿整个流水线的上下文载体。
type ContextData struct {
	Mode           PipelineMode
	Config         GameConfig            // 静态游戏配置
	Variables      map[string]any        // 来自 Variable Sandbox Flatten 的动态变量
	RecentMessages []Message             // 最近的 N 条历史记录 (用于世界书触发判断)
	Blocks         []PromptBlock         // 输出的 IR 块
	TokenBudget    int                   // Token 上限预留
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
