package api

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/memory"
	"mvu-backend/internal/engine/parser"
	"mvu-backend/internal/engine/pipeline"
	"mvu-backend/internal/engine/prompt_ir"
	"mvu-backend/internal/engine/session"
	"mvu-backend/internal/engine/variable"
	"mvu-backend/internal/platform/provider"
)

// GenParams 前端或配置层可覆盖的生成参数（全部可选）。
//
// 与 TH 的 LlmBindingGenerationParams 对齐：
//   - nil / 不传 = 不覆盖，使用 profile / env 默认值
//   - 显式传 0 = 合法值（如 temperature=0 表示贪婪解码）
type GenParams struct {
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	ReasoningEffort  *string  `json:"reasoning_effort,omitempty"` // "low"|"medium"|"high"
	Stop             []string `json:"stop,omitempty"`
}

// TurnRequest 前端提交的游戏操作
type TurnRequest struct {
	SessionID string `json:"session_id"`
	UserInput string `json:"user_input"`
	IsRegen   bool   `json:"is_regen"` // 重新生成（Swipe）

	// 可选：用户自定义 AI 配置（覆盖服务器默认 Key）
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model,omitempty"`

	// 可选：本轮生成参数覆盖（最高优先级，覆盖 profile 和 env 配置）
	// 对齐 TH TurnRequest.generation_params 字段
	GenerationParams *GenParams `json:"generation_params,omitempty"`
}

// TurnResponse 一回合结果（MVU 的 Model → View 快照）
type TurnResponse struct {
	FloorID   string               `json:"floor_id"`
	PageID    string               `json:"page_id"`
	Narrative string               `json:"narrative"`
	Options   []string             `json:"options"`
	Variables map[string]any       `json:"variables"` // 合并后的完整变量快照，供前端重绘
	VN        *parser.VNDirectives `json:"vn,omitempty"`
	ParseMode string               `json:"parse_mode"` // 调试：用哪种解析策略
	TokenUsed int                  `json:"token_used"`
}

// GameEngine 完整游戏引擎（依赖注入）
type GameEngine struct {
	db         *gorm.DB
	llmClient  *llm.Client      // env 配置的默认客户端（兜底）
	registry   *provider.Registry // 动态 Provider 注册表（可 nil，退化为 llmClient）
	sessions   *session.Manager
	memStore   *memory.Store

	// 运行时参数（来自配置，避免硬编码）
	memoryTriggerRounds int
	maxTokens           int
	tokenBudget         int
	maxHistoryFloors    int
	memoryMaxTokens     int
	memoryTokenBudget   int
}

// NewGameEngine 构造游戏引擎（所有依赖从外部注入，方便单测）
func NewGameEngine(db *gorm.DB, llmClient *llm.Client, reg *provider.Registry, memoryTriggerRounds, maxTokens, tokenBudget, maxHistoryFloors, memoryMaxTokens, memoryTokenBudget int) *GameEngine {
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	if tokenBudget <= 0 {
		tokenBudget = 8000
	}
	if maxHistoryFloors <= 0 {
		maxHistoryFloors = 20
	}
	if memoryMaxTokens <= 0 {
		memoryMaxTokens = 512
	}
	if memoryTokenBudget <= 0 {
		memoryTokenBudget = 600
	}
	return &GameEngine{
		db:                  db,
		llmClient:           llmClient,
		registry:            reg,
		sessions:            session.NewManager(db),
		memStore:            memory.NewStore(db),
		memoryTriggerRounds: memoryTriggerRounds,
		maxTokens:           maxTokens,
		tokenBudget:         tokenBudget,
		maxHistoryFloors:    maxHistoryFloors,
		memoryMaxTokens:     memoryMaxTokens,
		memoryTokenBudget:   memoryTokenBudget,
	}
}

// PlayTurn 运行一次游戏回合（核心主链路：One-Shot LLM）
func (e *GameEngine) PlayTurn(ctx context.Context, req TurnRequest) (*TurnResponse, error) {
	// ── 1. 加载游戏会话与静态配置 ──────────────────────────────
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", req.SessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	var template dbmodels.GameTemplate
	if err := e.db.First(&template, "id = ?", sess.GameID).Error; err != nil {
		return nil, fmt.Errorf("game template not found: %w", err)
	}

	// 读取世界书词条
	var wbEntries []dbmodels.WorldbookEntry
	e.db.Where("game_id = ? AND enabled = true", sess.GameID).Find(&wbEntries)

	// 读取 Preset Entry 条目
	var presetEntries []dbmodels.PresetEntry
	e.db.Where("game_id = ? AND enabled = true", sess.GameID).
		Order("injection_order ASC").Find(&presetEntries)

	// 解析模板 Config JSONB（memory_label / fallback_options 等可选字段）
	var tmplCfg struct {
		MemoryLabel     string   `json:"memory_label"`     // 记忆注入标签前缀
		FallbackOptions []string `json:"fallback_options"` // parser fallback 默认选项
	}
	_ = json.Unmarshal(template.Config, &tmplCfg)

	// ── 2. 处理楼层/页面状态 ──────────────────────────────────
	var floorID, pageID string
	var err error

	if req.IsRegen {
		var floor dbmodels.Floor
		e.db.Where("session_id = ? AND status IN ?", req.SessionID,
			[]string{string(dbmodels.FloorGenerating), string(dbmodels.FloorFailed)}).
			Order("seq DESC").First(&floor)
		if floor.ID == "" {
			return nil, fmt.Errorf("no active floor to regen")
		}
		floorID = floor.ID
		pageID, err = e.sessions.RegenTurn(floorID, req.UserInput)
	} else {
		floorID, pageID, err = e.sessions.StartTurn(req.SessionID, req.UserInput)
	}
	if err != nil {
		return nil, fmt.Errorf("session turn: %w", err)
	}

	// ── 3. 构建变量沙箱 ────────────────────────────────────────
	var chatVars map[string]any
	_ = json.Unmarshal(sess.Variables, &chatVars)
	sb := variable.NewSandbox(nil, chatVars, nil, nil, nil)

	// ── 4. 准备记忆摘要（来自异步 Worker 的缓存） ──────────────
	memorySummary := sess.MemorySummary

	// ── 5. 加载历史消息 ────────────────────────────────────────
	history, _ := e.sessions.GetHistory(req.SessionID, e.maxHistoryFloors)

	// ── 6. 将世界书条目转换为 Pipeline 所需格式 ────────────────
	var wbIR []prompt_ir.WorldbookEntry
	for _, entry := range wbEntries {
		var keys []string
		_ = json.Unmarshal(entry.Keys, &keys)
		wbIR = append(wbIR, prompt_ir.WorldbookEntry{
			ID:       entry.ID,
			Keys:     keys,
			Content:  entry.Content,
			Constant: entry.Constant,
			Priority: entry.Priority,
			Enabled:  entry.Enabled,
		})
	}

	// ── 7a. 将 Preset Entry 转换为 Pipeline IR ─────────────────
	var presetIR []prompt_ir.PresetEntry
	for _, pe := range presetEntries {
		presetIR = append(presetIR, prompt_ir.PresetEntry{
			Identifier:        pe.Identifier,
			Name:              pe.Name,
			Role:              pe.Role,
			Content:           pe.Content,
			InjectionPosition: pe.InjectionPosition,
			InjectionOrder:    pe.InjectionOrder,
			Enabled:           pe.Enabled,
			IsSystemPrompt:    pe.IsSystemPrompt,
		})
	}

	// ── 7. 构建历史消息列表（含本次用户输入） ─────────────────
	var recentMsgs []prompt_ir.Message
	for _, m := range history {
		recentMsgs = append(recentMsgs, prompt_ir.Message{
			Role: m["role"], Content: m["content"],
		})
	}
	recentMsgs = append(recentMsgs, prompt_ir.Message{
		Role: "user", Content: req.UserInput,
	})

	// ── 8. 运行 Prompt Pipeline ────────────────────────────────
	pipelineCtx := &prompt_ir.ContextData{
		Mode: prompt_ir.ModeNative,
		Config: prompt_ir.GameConfig{
			SystemPromptTemplate: template.SystemPromptTemplate,
			WorldbookEntries:     wbIR,
			PresetEntries:        presetIR,
			MemorySummary:        memorySummary,
			MemoryLabel:          tmplCfg.MemoryLabel,
			FallbackOptions:      tmplCfg.FallbackOptions,
		},
		Variables:      sb.Flatten(),
		RecentMessages: recentMsgs,
		TokenBudget:    e.tokenBudget,
	}

	runner := pipeline.NewRunner()
	finalMessages, err := runner.Execute(pipelineCtx)
	if err != nil {
		_ = e.sessions.FailTurn(floorID, err.Error())
		return nil, fmt.Errorf("pipeline: %w", err)
	}

	// ── 9. 解析 LLM Profile（优先级：req → session → global → env）────
	client, llmOpts := e.resolveSlot(req.SessionID, sess.UserID, "narrator")

	// 本轮 model/api_key/base_url 覆盖（前端自带 Key 模式）
	if req.Model != "" {
		llmOpts.Model = req.Model
	}

	// 本轮 GenerationParams 覆盖（最高优先级）
	if p := req.GenerationParams; p != nil {
		applyGenParams(&llmOpts, p)
	}

	// 将 pipeline 输出转为 llm.Message 切片
	var llmMsgs []llm.Message
	for _, m := range finalMessages {
		llmMsgs = append(llmMsgs, llm.Message{Role: m["role"], Content: m["content"]})
	}

	// 如果前端提供了自定义 key / baseURL，临时构建一个新客户端
	if req.APIKey != "" {
		baseURL := req.BaseURL
		if baseURL == "" {
			baseURL = e.llmClient.BaseURL()
		}
		client = llm.NewClient(baseURL, req.APIKey, llmOpts.Model, 60, 2)
	}

	// ── 10. One-Shot LLM 调用（主链路唯一的 LLM 请求）────────
	llmResp, err := client.Chat(ctx, llmMsgs, llmOpts)
	if err != nil {
		_ = e.sessions.FailTurn(floorID, err.Error())
		return nil, fmt.Errorf("llm: %w", err)
	}

	// ── 11. 解析 AI 响应（三层回退） ──────────────────────────
	parsed := parser.Parse(llmResp.Content)

	// 如果选项为空（通常是 fallback 模式），使用模板配置的兜底选项
	if len(parsed.Options) == 0 && len(tmplCfg.FallbackOptions) > 0 {
		parsed.Options = tmplCfg.FallbackOptions
	}

	// ── 12. 更新 Page 沙箱变量 ────────────────────────────────
	for k, v := range parsed.StatePatch {
		sb.Set(k, v)
	}

	// ── 13. 提交楼层（锁定 + Page 变量提升至 Chat） ────────────
	if err := e.sessions.CommitTurn(pageID, llmResp.Content, sb.Flatten()); err != nil {
		return nil, fmt.Errorf("commit turn: %w", err)
	}

	// ── 14. 异步任务（不阻塞响应） ─────────────────────────────
	go func() {
		if parsed.Summary != "" {
			_ = e.memStore.SaveFromParser(req.SessionID, parsed.Summary, 0)
		}
		count, _ := e.sessions.IncrFloorCount(req.SessionID)
		if e.memoryTriggerRounds > 0 && count%e.memoryTriggerRounds == 0 {
			e.triggerMemoryConsolidation(req.SessionID, history, count)
		}
	}()

	return &TurnResponse{
		FloorID:   floorID,
		PageID:    pageID,
		Narrative: parsed.Narrative,
		Options:   parsed.Options,
		Variables: sb.Flatten(),
		VN:        parsed.VN,
		ParseMode: parsed.ParseMode,
		TokenUsed: llmResp.Usage.TotalTokens,
	}, nil
}

// resolveSlot 解析指定 slot 的 LLM 客户端和采样参数。
//
// 优先级（委托给 provider.Registry.ResolveForSlot）：
//  1. session slot X → 2. global slot X → 3. session * → 4. global * → 5. env 兜底
func (e *GameEngine) resolveSlot(sessionID, accountID, slot string) (*llm.Client, llm.Options) {
	if e.registry != nil {
		if client, opts, ok := e.registry.ResolveForSlot(e.db, accountID, sessionID, slot); ok {
			return client, opts
		}
	}
	return e.llmClient, llm.Options{MaxTokens: e.maxTokens}
}

// applyGenParams 将 GenParams 中非 nil 的字段写入 llm.Options（就地修改）
func applyGenParams(opts *llm.Options, p *GenParams) {
	if p == nil {
		return
	}
	if p.MaxTokens != nil {
		opts.MaxTokens = *p.MaxTokens
	}
	if p.Temperature != nil {
		opts.Temperature = p.Temperature
	}
	if p.TopP != nil {
		opts.TopP = p.TopP
	}
	if p.TopK != nil {
		opts.TopK = p.TopK
	}
	if p.FrequencyPenalty != nil {
		opts.FrequencyPenalty = p.FrequencyPenalty
	}
	if p.PresencePenalty != nil {
		opts.PresencePenalty = p.PresencePenalty
	}
	if p.ReasoningEffort != nil {
		opts.ReasoningEffort = *p.ReasoningEffort
	}
	if len(p.Stop) > 0 {
		opts.Stop = p.Stop
	}
}

// triggerMemoryConsolidation 触发记忆摘要整合（异步，廉价模型）
func (e *GameEngine) triggerMemoryConsolidation(sessionID string, recentHistory []map[string]string, floorCount int) {
	ctx := context.Background()
	prompt, err := e.memStore.BuildConsolidationPrompt(sessionID, recentHistory)
	if err != nil {
		return
	}
	resp, err := e.llmClient.Chat(ctx, []llm.Message{
		{Role: "user", Content: prompt},
	}, llm.Options{MaxTokens: e.memoryMaxTokens})
	if err != nil {
		return
	}
	_ = e.memStore.ParseConsolidationResult(sessionID, resp.Content, floorCount)

	summary, _ := e.memStore.GetForInjection(sessionID, e.memoryTokenBudget)
	_ = e.memStore.UpdateSessionSummaryCache(sessionID, summary)
}
