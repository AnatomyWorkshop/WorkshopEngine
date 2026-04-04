package api

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/parser"
	"mvu-backend/internal/engine/pipeline"
	"mvu-backend/internal/engine/prompt_ir"
	"mvu-backend/internal/engine/session"
	"mvu-backend/internal/engine/variable"
)

// CreateSession 创建新的游玩会话
func (e *GameEngine) CreateSession(ctx context.Context, gameID, userID string) (string, error) {
	sess := dbmodels.GameSession{
		GameID:    gameID,
		UserID:    userID,
		Variables: []byte("{}"),
	}
	if err := e.db.Create(&sess).Error; err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return sess.ID, nil
}

// StateResponse 游戏快照（变量 + 最近历史）
type StateResponse struct {
	SessionID     string         `json:"session_id"`
	Variables     map[string]any `json:"variables"`
	MemorySummary string         `json:"memory_summary"`
	FloorCount    int            `json:"floor_count"`
	RecentHistory []map[string]string `json:"recent_history"`
}

// GetState 返回当前游戏状态快照
func (e *GameEngine) GetState(ctx context.Context, sessionID string) (*StateResponse, error) {
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	var vars map[string]any
	_ = json.Unmarshal(sess.Variables, &vars)

	history, _ := e.sessions.GetHistory(sessionID, e.maxHistoryFloors)

	return &StateResponse{
		SessionID:     sessionID,
		Variables:     vars,
		MemorySummary: sess.MemorySummary,
		FloorCount:    sess.FloorCount,
		RecentHistory: history,
	}, nil
}

// StreamTurn 流式推进一回合，返回 token 和 error channel（供 SSE 路由消费）
// 与 PlayTurn 对齐：加载 preset/worldbook/tmplCfg，完整 Pipeline + slot 解析。
func (e *GameEngine) StreamTurn(ctx context.Context, req TurnRequest) (<-chan string, <-chan error) {
	tokenCh := make(chan string, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(tokenCh)
		defer close(errCh)

		// 1. 加载会话 + 模板
		var sess dbmodels.GameSession
		if err := e.db.First(&sess, "id = ?", req.SessionID).Error; err != nil {
			errCh <- fmt.Errorf("session not found: %w", err)
			return
		}
		var template dbmodels.GameTemplate
		if err := e.db.First(&template, "id = ?", sess.GameID).Error; err != nil {
			errCh <- fmt.Errorf("template not found: %w", err)
			return
		}

		// 2. 加载世界书 + Preset Entry（对齐 PlayTurn）
		var wbEntries []dbmodels.WorldbookEntry
		e.db.Where("game_id = ? AND enabled = true", sess.GameID).Find(&wbEntries)
		var presetEntries []dbmodels.PresetEntry
		e.db.Where("game_id = ? AND enabled = true", sess.GameID).
			Order("injection_order ASC").Find(&presetEntries)

		// 3. 解析模板 Config JSONB
		var tmplCfg struct {
			MemoryLabel     string   `json:"memory_label"`
			FallbackOptions []string `json:"fallback_options"`
		}
		_ = json.Unmarshal(template.Config, &tmplCfg)

		// 4. 变量沙箱
		var chatVars map[string]any
		_ = json.Unmarshal(sess.Variables, &chatVars)
		sb := variable.NewSandbox(nil, chatVars, nil, nil, nil)

		// 5. 转换 WorldbookEntry / PresetEntry → IR
		var wbIR []prompt_ir.WorldbookEntry
		for _, entry := range wbEntries {
			var keys []string
			_ = json.Unmarshal(entry.Keys, &keys)
			wbIR = append(wbIR, prompt_ir.WorldbookEntry{
				ID: entry.ID, Keys: keys, Content: entry.Content,
				Constant: entry.Constant, Priority: entry.Priority, Enabled: entry.Enabled,
			})
		}
		var presetIR []prompt_ir.PresetEntry
		for _, pe := range presetEntries {
			presetIR = append(presetIR, prompt_ir.PresetEntry{
				Identifier: pe.Identifier, Name: pe.Name, Role: pe.Role,
				Content: pe.Content, InjectionPosition: pe.InjectionPosition,
				InjectionOrder: pe.InjectionOrder, Enabled: pe.Enabled,
				IsSystemPrompt: pe.IsSystemPrompt,
			})
		}

		// 6. 历史 + 当前输入
		history, _ := e.sessions.GetHistory(req.SessionID, e.maxHistoryFloors)
		var recentMsgs []prompt_ir.Message
		for _, m := range history {
			recentMsgs = append(recentMsgs, prompt_ir.Message{Role: m["role"], Content: m["content"]})
		}
		recentMsgs = append(recentMsgs, prompt_ir.Message{Role: "user", Content: req.UserInput})

		// 7. 运行 Prompt Pipeline
		pipelineCtx := &prompt_ir.ContextData{
			Mode: prompt_ir.ModeNative,
			Config: prompt_ir.GameConfig{
				SystemPromptTemplate: template.SystemPromptTemplate,
				WorldbookEntries:     wbIR,
				PresetEntries:        presetIR,
				MemorySummary:        sess.MemorySummary,
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
			errCh <- err
			return
		}

		// 8. 处理楼层/页面（Regen 复用当前楼层，否则创建新楼层）
		var floorID, pageID string
		if req.IsRegen {
			var floor dbmodels.Floor
			e.db.Where("session_id = ? AND status IN ?", req.SessionID,
				[]string{string(dbmodels.FloorGenerating), string(dbmodels.FloorFailed)}).
				Order("seq DESC").First(&floor)
			if floor.ID == "" {
				errCh <- fmt.Errorf("no active floor to regen")
				return
			}
			floorID = floor.ID
			pageID, err = e.sessions.RegenTurn(floorID, req.UserInput)
		} else {
			floorID, pageID, err = e.sessions.StartTurn(req.SessionID, req.UserInput)
		}
		if err != nil {
			errCh <- fmt.Errorf("session turn: %w", err)
			return
		}

		// 9. 解析 LLM Profile + 覆盖参数（对齐 PlayTurn 优先级链）
		client, llmOpts := e.resolveSlot(req.SessionID, sess.UserID, "narrator")
		if req.Model != "" {
			llmOpts.Model = req.Model
		}
		if p := req.GenerationParams; p != nil {
			applyGenParams(&llmOpts, p)
		}
		if req.APIKey != "" {
			baseURL := req.BaseURL
			if baseURL == "" {
				baseURL = e.llmClient.BaseURL()
			}
			client = llm.NewClient(baseURL, req.APIKey, llmOpts.Model, 60, 2)
		}

		var llmMsgs []llm.Message
		for _, m := range finalMessages {
			llmMsgs = append(llmMsgs, llm.Message{Role: m["role"], Content: m["content"]})
		}

		// 10. 流式 LLM 调用
		streamCh, streamErrCh := client.ChatStream(ctx, llmMsgs, llmOpts)

		// 11. 转发 token，流结束后提交状态
		var fullContent string
		for {
			select {
			case token, ok := <-streamCh:
				if !ok {
					// 流结束：解析 + 提交
					parsed := parser.Parse(fullContent)
					if len(parsed.Options) == 0 && len(tmplCfg.FallbackOptions) > 0 {
						parsed.Options = tmplCfg.FallbackOptions
					}
					for k, v := range parsed.StatePatch {
						sb.Set(k, v)
					}
					_ = e.sessions.CommitTurn(pageID, fullContent, sb.Flatten())
					if parsed.Summary != "" {
						go func() {
							_ = e.memStore.SaveFromParser(req.SessionID, parsed.Summary, 0)
						}()
					}
					return
				}
				fullContent += token
				select {
				case tokenCh <- token:
				case <-ctx.Done():
					_ = e.sessions.FailTurn(floorID, "context cancelled")
					return
				}
			case err := <-streamErrCh:
				if err != nil {
					_ = e.sessions.FailTurn(floorID, err.Error())
					errCh <- err
				}
				return
			case <-ctx.Done():
				_ = e.sessions.FailTurn(floorID, "context cancelled")
				return
			}
		}
	}()

	return tokenCh, errCh
}

// ── Session CRUD ──────────────────────────────────────────────────────────────

// UpdateSessionReq PATCH /sessions/:id 请求体
type UpdateSessionReq struct {
	Title  string `json:"title"`
	Status string `json:"status"` // active | archived
}

// UpdateSession 更新会话标题或状态
func (e *GameEngine) UpdateSession(ctx context.Context, sessionID string, req UpdateSessionReq) (*dbmodels.GameSession, error) {
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	updates := map[string]any{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Status == "active" || req.Status == "archived" {
		updates["status"] = req.Status
	}
	if len(updates) == 0 {
		return &sess, nil
	}
	if err := e.db.Model(&sess).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("update session: %w", err)
	}
	return &sess, nil
}

// DeleteSession 删除会话及所有关联的楼层、页面和记忆
func (e *GameEngine) DeleteSession(ctx context.Context, sessionID string) error {
	// 1. 删除记忆
	e.db.Where("session_id = ?", sessionID).Delete(&dbmodels.Memory{})

	// 2. 找出所有楼层，删除楼层下的页面
	var floors []dbmodels.Floor
	e.db.Where("session_id = ?", sessionID).Find(&floors)
	for _, f := range floors {
		e.db.Where("floor_id = ?", f.ID).Delete(&dbmodels.MessagePage{})
	}
	e.db.Where("session_id = ?", sessionID).Delete(&dbmodels.Floor{})

	// 3. 删除会话
	if err := e.db.Where("id = ?", sessionID).Delete(&dbmodels.GameSession{}).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// PatchVariables 合并更新会话的 Chat 级变量
func (e *GameEngine) PatchVariables(ctx context.Context, sessionID string, patch map[string]any) (map[string]any, error) {
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	var vars map[string]any
	_ = json.Unmarshal(sess.Variables, &vars)
	if vars == nil {
		vars = map[string]any{}
	}
	for k, v := range patch {
		vars[k] = v
	}
	newVars, _ := json.Marshal(vars)
	if err := e.db.Model(&sess).Update("variables", newVars).Error; err != nil {
		return nil, fmt.Errorf("update variables: %w", err)
	}
	return vars, nil
}

// ── Session 列表 ───────────────────────────────────────────────────────────────

// ListSessionsReq GET /play/sessions 的查询参数
type ListSessionsReq struct {
	GameID string
	UserID string
	Limit  int
	Offset int
}

// ListSessions 列出会话（可按 game_id 或 user_id 过滤，支持分页）
func (e *GameEngine) ListSessions(_ context.Context, req ListSessionsReq) ([]dbmodels.GameSession, error) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}
	query := e.db.Order("updated_at DESC").Limit(req.Limit).Offset(req.Offset)
	if req.GameID != "" {
		query = query.Where("game_id = ?", req.GameID)
	}
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	var sessions []dbmodels.GameSession
	return sessions, query.Find(&sessions).Error
}

// ── Floors / Pages ─────────────────────────────────────────────────────────────

// ListFloors 返回会话的楼层列表（含当前激活页摘要）
func (e *GameEngine) ListFloors(_ context.Context, sessionID string) ([]session.FloorWithPage, error) {
	return e.sessions.ListFloors(sessionID)
}

// ListPages 返回单个楼层的所有 Swipe 页
func (e *GameEngine) ListPages(_ context.Context, floorID string) ([]dbmodels.MessagePage, error) {
	return e.sessions.ListPages(floorID)
}

// SetActivePage 切换楼层的激活页（Swipe 选择）
func (e *GameEngine) SetActivePage(_ context.Context, floorID, pageID string) error {
	return e.sessions.SetActivePage(floorID, pageID)
}

// ── Memory CRUD ────────────────────────────────────────────────────────────────

// ListMemories 列出会话的所有记忆条目
func (e *GameEngine) ListMemories(_ context.Context, sessionID string) ([]dbmodels.Memory, error) {
	return e.memStore.ListMemories(sessionID)
}

// CreateMemoryReq POST /memories 请求体
type CreateMemoryReq struct {
	Content    string  `json:"content"    binding:"required"`
	Type       string  `json:"type"`       // fact | summary | open_loop，默认 fact
	Importance float64 `json:"importance"` // 0–1，默认 0.9
}

// CreateMemory 手动创建记忆条目（创作者/调试用）
func (e *GameEngine) CreateMemory(_ context.Context, sessionID string, req CreateMemoryReq) (*dbmodels.Memory, error) {
	memType := dbmodels.MemoryType(req.Type)
	if memType == "" {
		memType = dbmodels.MemoryFact
	}
	importance := req.Importance
	if importance <= 0 {
		importance = 0.9
	}
	mem := dbmodels.Memory{
		SessionID:  sessionID,
		Content:    req.Content,
		Type:       memType,
		Importance: importance,
	}
	if err := e.db.Create(&mem).Error; err != nil {
		return nil, fmt.Errorf("create memory: %w", err)
	}
	return &mem, nil
}

// UpdateMemory 部分更新记忆字段
func (e *GameEngine) UpdateMemory(_ context.Context, sessionID, memID string, updates map[string]any) (*dbmodels.Memory, error) {
	return e.memStore.UpdateMemory(memID, sessionID, updates)
}

// DeleteMemory 软删除（默认）或物理删除记忆条目
func (e *GameEngine) DeleteMemory(_ context.Context, sessionID, memID string, hard bool) error {
	return e.memStore.DeleteMemory(memID, sessionID, hard)
}

// ConsolidateNow 立即对指定会话执行记忆整合（同步，供调试 / 手动触发）
func (e *GameEngine) ConsolidateNow(ctx context.Context, sessionID string) error {
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return fmt.Errorf("session not found: %w", err)
	}
	history, err := e.sessions.GetHistory(sessionID, e.maxHistoryFloors)
	if err != nil {
		return err
	}
	prompt, err := e.memStore.BuildConsolidationPrompt(sessionID, history)
	if err != nil {
		return err
	}
	resp, err := e.llmClient.Chat(ctx, []llm.Message{{Role: "user", Content: prompt}},
		llm.Options{MaxTokens: e.memoryMaxTokens})
	if err != nil {
		return fmt.Errorf("llm consolidation: %w", err)
	}
	if err := e.memStore.ParseConsolidationResult(sessionID, resp.Content, sess.FloorCount); err != nil {
		return err
	}
	summary, _ := e.memStore.GetForInjection(sessionID, e.memoryTokenBudget)
	return e.memStore.UpdateSessionSummaryCache(sessionID, summary)
}

// ── Prompt Dry-Run ─────────────────────────────────────────────────────────────

// PromptPreviewResponse prompt-preview 的返回结构
type PromptPreviewResponse struct {
	Messages      []map[string]string `json:"messages"`       // 组装后的消息列表
	EstTokens     int                 `json:"est_tokens"`     // 粗估总 token 数
	BlockCount    int                 `json:"block_count"`    // PromptBlock 数量
	PresetHits    int                 `json:"preset_hits"`    // 触发的 Preset Entry 数
	WorldbookHits int                 `json:"worldbook_hits"` // 触发的世界书词条数
	MemoryUsed    bool                `json:"memory_used"`    // 是否注入了记忆摘要
}

// PromptPreview 组装 prompt 但不调用 LLM（dry-run，供创作者调试用）
func (e *GameEngine) PromptPreview(ctx context.Context, sessionID, userInput string) (*PromptPreviewResponse, error) {
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	var tmpl dbmodels.GameTemplate
	if err := e.db.First(&tmpl, "id = ?", sess.GameID).Error; err != nil {
		return nil, fmt.Errorf("game template not found: %w", err)
	}

	// 加载世界书 + Preset Entry
	var wbEntries []dbmodels.WorldbookEntry
	e.db.Where("game_id = ? AND enabled = true", sess.GameID).Find(&wbEntries)
	var presetEntries []dbmodels.PresetEntry
	e.db.Where("game_id = ? AND enabled = true", sess.GameID).
		Order("injection_order ASC").Find(&presetEntries)

	var tmplCfg struct {
		MemoryLabel     string   `json:"memory_label"`
		FallbackOptions []string `json:"fallback_options"`
	}
	_ = json.Unmarshal(tmpl.Config, &tmplCfg)

	// 变量沙箱
	var chatVars map[string]any
	_ = json.Unmarshal(sess.Variables, &chatVars)
	sb := variable.NewSandbox(nil, chatVars, nil, nil, nil)

	// 历史 + 当前输入
	history, _ := e.sessions.GetHistory(sessionID, e.maxHistoryFloors)
	var recentMsgs []prompt_ir.Message
	for _, m := range history {
		recentMsgs = append(recentMsgs, prompt_ir.Message{Role: m["role"], Content: m["content"]})
	}
	if userInput != "" {
		recentMsgs = append(recentMsgs, prompt_ir.Message{Role: "user", Content: userInput})
	}

	// 转换 DB 行 → IR 类型
	var wbIR []prompt_ir.WorldbookEntry
	for _, entry := range wbEntries {
		var keys []string
		_ = json.Unmarshal(entry.Keys, &keys)
		wbIR = append(wbIR, prompt_ir.WorldbookEntry{
			ID: entry.ID, Keys: keys, Content: entry.Content,
			Constant: entry.Constant, Priority: entry.Priority, Enabled: entry.Enabled,
		})
	}
	var presetIR []prompt_ir.PresetEntry
	for _, pe := range presetEntries {
		presetIR = append(presetIR, prompt_ir.PresetEntry{
			Identifier: pe.Identifier, Name: pe.Name, Role: pe.Role,
			Content: pe.Content, InjectionPosition: pe.InjectionPosition,
			InjectionOrder: pe.InjectionOrder, Enabled: pe.Enabled,
			IsSystemPrompt: pe.IsSystemPrompt,
		})
	}

	pCtx := &prompt_ir.ContextData{
		Mode: prompt_ir.ModeNative,
		Config: prompt_ir.GameConfig{
			SystemPromptTemplate: tmpl.SystemPromptTemplate,
			WorldbookEntries:     wbIR,
			PresetEntries:        presetIR,
			MemorySummary:        sess.MemorySummary,
			MemoryLabel:          tmplCfg.MemoryLabel,
			FallbackOptions:      tmplCfg.FallbackOptions,
		},
		Variables:      sb.Flatten(),
		RecentMessages: recentMsgs,
		TokenBudget:    e.tokenBudget,
	}

	runner := pipeline.NewRunner()
	finalMessages, err := runner.Execute(pCtx)
	if err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}

	// 粗估 token 数：UTF-8 字符数 / 2 (中英混合经验值)
	var totalChars int
	for _, m := range finalMessages {
		totalChars += utf8.RuneCountInString(m["content"])
	}

	// 统计各类 block 数量
	presetHits, wbHits, memUsed := 0, 0, false
	for _, b := range pCtx.Blocks {
		switch b.Type {
		case prompt_ir.BlockPreset:
			presetHits++
		case prompt_ir.BlockWorldbook:
			wbHits++
		case prompt_ir.BlockMemory:
			memUsed = true
		}
	}

	return &PromptPreviewResponse{
		Messages:      finalMessages,
		EstTokens:     totalChars / 2,
		BlockCount:    len(pCtx.Blocks),
		PresetHits:    presetHits,
		WorldbookHits: wbHits,
		MemoryUsed:    memUsed,
	}, nil
}
