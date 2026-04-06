package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/core/tokenizer"
	"mvu-backend/internal/engine/parser"
	"mvu-backend/internal/engine/pipeline"
	"mvu-backend/internal/engine/processor"
	"mvu-backend/internal/engine/prompt_ir"
	"mvu-backend/internal/engine/scheduled"
	"mvu-backend/internal/engine/session"
	"mvu-backend/internal/engine/tools"
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

// StreamMeta StreamTurn 流式结束后返回的回合元数据（通过 metaCh 发送一次）。
// 前端通过 SSE "meta" 事件接收，结构与 TurnResponse 对齐。
type StreamMeta struct {
	FloorID        string               `json:"floor_id"`
	PageID         string               `json:"page_id"`
	Variables      map[string]any       `json:"variables"`
	Options        []string             `json:"options"`
	VN             *parser.VNDirectives `json:"vn,omitempty"`
	ParseMode      string               `json:"parse_mode"`
	TokenUsed      int                  `json:"token_used"`
	ScheduledInput string               `json:"scheduled_input,omitempty"`
}

// StreamTurn 流式推进一回合。与 PlayTurn 完全对齐（regex / tools / agentic loop / ScheduledTurn）。
//
// 返回三个 channel：
//   - tokenCh:  逐 token 推送（关闭表示最终 LLM 流结束）
//   - metaCh:   流结束后推送一条 StreamMeta（变量快照 / options / scheduled_input 等）
//   - errCh:    出错时推送并退出
//
// Agentic 工具循环：
//   - 若游戏模板启用了工具，先以**非流式**方式完成所有工具调用轮次（最多 5 轮）
//   - 所有工具调用解决后，对最终回复**流式**输出给前端
func (e *GameEngine) StreamTurn(ctx context.Context, req TurnRequest) (<-chan string, <-chan StreamMeta, <-chan error) {
	tokenCh := make(chan string, 64)
	metaCh := make(chan StreamMeta, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(tokenCh)
		defer close(metaCh)
		defer close(errCh)

		// ── 1. 加载会话 + 模板 ────────────────────────────────────
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

		// ── 2. 加载世界书 / Preset / Regex（与 PlayTurn 完全一致）───
		var wbEntries []dbmodels.WorldbookEntry
		e.db.Where("game_id = ? AND enabled = true", sess.GameID).Find(&wbEntries)
		var presetEntries []dbmodels.PresetEntry
		e.db.Where("game_id = ? AND enabled = true", sess.GameID).
			Order("injection_order ASC").Find(&presetEntries)
		var dbRegexRules []dbmodels.RegexRule
		e.db.Joins("JOIN regex_profiles ON regex_rules.profile_id = regex_profiles.id").
			Where("regex_profiles.game_id = ? AND regex_profiles.enabled = true AND regex_rules.enabled = true", sess.GameID).
			Order("regex_rules.order ASC").Find(&dbRegexRules)

		// ── 3. 解析模板 Config（与 PlayTurn 对齐，含 ScheduledTurns）─
		var tmplCfg struct {
			MemoryLabel       string                  `json:"memory_label"`
			FallbackOptions   []string                `json:"fallback_options"`
			EnabledTools      []string                `json:"enabled_tools"`
			ScheduledTurns    []scheduled.TriggerRule `json:"scheduled_turns"`
			DirectorPrompt    string                  `json:"director_prompt"`
			VerifierPrompt    string                  `json:"verifier_prompt"` // 可选，Verifier 槽校验指令
			WorldbookGroupCap int                     `json:"worldbook_group_cap"`
		}
		_ = json.Unmarshal(template.Config, &tmplCfg)

		// ── 4. 变量沙箱 ───────────────────────────────────────────
		var chatVars map[string]any
		_ = json.Unmarshal(sess.Variables, &chatVars)
		sb := variable.NewSandbox(nil, chatVars, nil, nil, nil)

		// ── 4b. 工具注册（与 PlayTurn 完全一致）─────────────────────
		toolReg := tools.NewRegistry()
		if len(tmplCfg.EnabledTools) > 0 {
			enabled := make(map[string]struct{}, len(tmplCfg.EnabledTools))
			for _, name := range tmplCfg.EnabledTools {
				enabled[name] = struct{}{}
			}
			if _, ok := enabled["get_variable"]; ok {
				toolReg.Register(tools.NewGetVariableTool(sb))
			}
			if _, ok := enabled["set_variable"]; ok {
				toolReg.Register(tools.NewSetVariableTool(sb))
			}
			if _, ok := enabled["search_memory"]; ok {
				toolReg.Register(tools.NewSearchMemoryTool(req.SessionID, e.memStore))
			}
			if _, ok := enabled["search_material"]; ok {
				toolReg.Register(tools.NewSearchMaterialTool(e.db, sess.GameID, req.SessionID))
			}
			if _, ok := enabled["resource:*"]; ok {
				for _, t := range tools.NewResourceToolProvider(e.db, sess.GameID, req.SessionID, e.memStore) {
					toolReg.Register(t)
				}
			} else {
				for _, t := range tools.NewResourceToolProvider(e.db, sess.GameID, req.SessionID, e.memStore) {
					if _, ok := enabled[t.Name()]; ok {
						toolReg.Register(t)
					}
				}
			}
			// preset:* 或 preset:<name> — 加载创作者自定义 HTTP 回调工具
			var presetTools []dbmodels.PresetTool
			e.db.Where("game_id = ? AND enabled = true", sess.GameID).Find(&presetTools)
			for _, pt := range presetTools {
				_, allOk := enabled["preset:*"]
				_, nameOk := enabled["preset:"+pt.Name]
				if allOk || nameOk {
					toolReg.Register(tools.NewHttpCallTool(pt, req.SessionID))
				}
			}
		}

		// ── 5. 转换 WorldbookEntry / PresetEntry / Regex → IR ─────
		var wbIR []prompt_ir.WorldbookEntry
		for _, entry := range wbEntries {
			var keys, secondaryKeys []string
			_ = json.Unmarshal(entry.Keys, &keys)
			_ = json.Unmarshal(entry.SecondaryKeys, &secondaryKeys)
			wbIR = append(wbIR, prompt_ir.WorldbookEntry{
				ID: entry.ID, Keys: keys, SecondaryKeys: secondaryKeys,
				SecondaryLogic: entry.SecondaryLogic, Content: entry.Content,
				Constant: entry.Constant, Priority: entry.Priority,
				ScanDepth: entry.ScanDepth, Position: entry.Position,
				WholeWord: entry.WholeWord, Enabled: entry.Enabled,
				Group: entry.Group, GroupWeight: entry.GroupWeight,
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
		var regexRules []prompt_ir.RegexRule
		for _, r := range dbRegexRules {
			regexRules = append(regexRules, prompt_ir.RegexRule{
				Pattern: r.Pattern, Replacement: r.Replacement,
				ApplyTo: r.ApplyTo, Enabled: r.Enabled,
			})
		}

		// ── 6. 历史 + 用户输入（先经 regex 预处理）────────────────
		history, _ := e.sessions.GetHistory(req.SessionID, e.maxHistoryFloors)
		userInput := processor.ApplyToUserInput(req.UserInput, regexRules)
		var recentMsgs []prompt_ir.Message
		for _, m := range history {
			recentMsgs = append(recentMsgs, prompt_ir.Message{Role: m["role"], Content: m["content"]})
		}
		recentMsgs = append(recentMsgs, prompt_ir.Message{Role: "user", Content: userInput})

		// ── 7. 运行 Prompt Pipeline ────────────────────────────────
		pipelineCtx := &prompt_ir.ContextData{
			Mode: prompt_ir.ModeNative,
			Config: prompt_ir.GameConfig{
				SystemPromptTemplate: template.SystemPromptTemplate,
				WorldbookEntries:     wbIR,
				PresetEntries:        presetIR,
				MemorySummary:        sess.MemorySummary,
				MemoryLabel:          tmplCfg.MemoryLabel,
				FallbackOptions:      tmplCfg.FallbackOptions,
				RegexRules:           regexRules,
				WorldbookGroupCap:    tmplCfg.WorldbookGroupCap,
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

		// ── 8. 处理楼层/页面 ──────────────────────────────────────
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

		// ── 9. 解析 LLM Profile + 参数覆盖（与 PlayTurn 优先级链一致）─
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

		// ── Director 槽（可选，廉价模型预分析）────────────────────────
		if dirClient, dirOpts, ok := func() (*llm.Client, llm.Options, bool) {
			if e.registry == nil {
				return nil, llm.Options{}, false
			}
			c, o, ok := e.registry.ResolveForSlot(e.db, sess.UserID, req.SessionID, "director")
			return c, o, ok
		}(); ok {
			dirPrompt := tmplCfg.DirectorPrompt
			if dirPrompt == "" {
				dirPrompt = "分析当前对话上下文，用一段简短的中文指导下一步叙事方向，不超过100字。"
			}
			dirMsgs := append(llmMsgs, llm.Message{Role: "user", Content: dirPrompt})
			if dirResp, err := dirClient.Chat(ctx, dirMsgs, dirOpts); err == nil && dirResp.Content != "" {
				llmMsgs = append([]llm.Message{{Role: "system", Content: "[Director] " + dirResp.Content}}, llmMsgs...)
			}
		}

		// 注入工具定义
		if defs := toolReg.ToLLMDefinitions(); len(defs) > 0 {
			llmOpts.Tools = defs
		}

		// ── 10a. Agentic 工具循环（非流式，最多 5 轮）────────────────
		// 先以非流式方式完成所有工具调用，再对最终回复做流式推送。
		const maxToolIter = 5
		var toolResp *llm.Response
		for iter := 0; iter < maxToolIter; iter++ {
			toolResp, err = client.Chat(ctx, llmMsgs, llmOpts)
			if err != nil {
				_ = e.sessions.FailTurn(floorID, err.Error())
				errCh <- fmt.Errorf("llm tool round: %w", err)
				return
			}
			if len(toolResp.ToolCalls) == 0 {
				break // 无更多工具调用
			}
			llmMsgs = append(llmMsgs, llm.Message{
				Role:      "assistant",
				Content:   toolResp.Content,
				ToolCalls: toolResp.ToolCalls,
			})
			for _, tc := range toolResp.ToolCalls {
				toolCtx := context.WithValue(ctx, tools.CtxFloorID, floorID)
				result := toolReg.ExecuteAndRecord(toolCtx, tc.Function.Name, json.RawMessage(tc.Function.Arguments),
					tools.ToolRecord{SessionID: req.SessionID, FloorID: floorID, PageID: pageID}, e.db)
				llmMsgs = append(llmMsgs, llm.Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
				})
			}
		}
		// 工具轮次完成后移除工具定义，防止最终流式调用再次触发工具
		llmOpts.Tools = nil

		// ── 10b. 流式输出最终 LLM 回复 ───────────────────────────
		streamCh, usageCh, streamErrCh := client.ChatStream(ctx, llmMsgs, llmOpts)

		var fullContent string
		// 工具调用轮次已消耗的 token（若有）
		var tokenUsed int
		if toolResp != nil {
			tokenUsed = toolResp.Usage.TotalTokens
		}
		streamDone := false
		for !streamDone {
			select {
			case token, ok := <-streamCh:
				if !ok {
					streamDone = true
					break
				}
				fullContent += token
				select {
				case tokenCh <- token:
				case <-ctx.Done():
					_ = e.sessions.FailTurn(floorID, "context cancelled")
					return
				}
			case err = <-streamErrCh:
				if err != nil {
					_ = e.sessions.FailTurn(floorID, err.Error())
					errCh <- err
					return
				}
				streamDone = true
			case <-ctx.Done():
				_ = e.sessions.FailTurn(floorID, "context cancelled")
				return
			}
		}
		// 读取流式 usage（provider 在最后一帧返回；若为 0 则保留工具轮次的值）
		if u, ok := <-usageCh; ok && u.TotalTokens > 0 {
			tokenUsed = u.TotalTokens
		}

		// ── 11. 解析 AI 响应 + regex 后处理 ──────────────────────
		parsed := parser.Parse(fullContent)
		if len(regexRules) > 0 {
			parsed.Narrative = processor.ApplyToAIOutput(parsed.Narrative, regexRules)
		}
		if len(parsed.Options) == 0 && len(tmplCfg.FallbackOptions) > 0 {
			parsed.Options = tmplCfg.FallbackOptions
		}

		// ── 11b. Verifier 槽（可选，廉价模型一致性校验）──────────────
		verResult, _ := e.runVerifier(ctx, req.SessionID, sess.UserID, parsed.Narrative, tmplCfg.VerifierPrompt)

		// ── 12. 更新变量沙箱 ──────────────────────────────────────
		for k, v := range parsed.StatePatch {
			sb.Set(k, v)
		}

		// ── 13. CommitTurn ────────────────────────────────────────
		if err = e.sessions.CommitTurn(pageID, fullContent, sb.Flatten()); err != nil {
			errCh <- fmt.Errorf("commit turn: %w", err)
			return
		}

		// ── 14. 同步递增楼层计数 ───────────────────────────────────
		count, _ := e.sessions.IncrFloorCount(req.SessionID)

		// ── 15. 异步任务（记忆整合 + PromptSnapshot）────────────────
		go func() {
			if parsed.Summary != "" {
				_ = e.memStore.SaveFromParser(req.SessionID, parsed.Summary, 0)
			}
			if e.memoryTriggerRounds > 0 && count%e.memoryTriggerRounds == 0 {
				e.triggerMemoryConsolidation(req.SessionID, history, count)
			}
			e.savePromptSnapshot(SnapshotInput{
				SessionID:             req.SessionID,
				FloorID:               floorID,
				ActivatedWorldbookIDs: pipelineCtx.ActivatedWorldbookIDs,
				FinalMessages:         finalMessages,
				PipelineCtx:           pipelineCtx,
				VerifyResult:          verResult,
			})
		}()

		// ── 16. ScheduledTurn 触发检查 ────────────────────────────
		var scheduledInput string
		if len(tmplCfg.ScheduledTurns) > 0 {
			if rule := scheduled.Evaluate(tmplCfg.ScheduledTurns, sb.Flatten(), count, rand.Float64()); rule != nil {
				scheduledInput = rule.PickInput()
				_ = e.sessions.PatchSessionVariables(req.SessionID, map[string]any{
					scheduled.CooldownKey(rule.ID): float64(count),
				})
			}
		}

		// ── 17. 推送元数据 ────────────────────────────────────────
		metaCh <- StreamMeta{
			FloorID:        floorID,
			PageID:         pageID,
			Variables:      sb.Flatten(),
			Options:        parsed.Options,
			VN:             parsed.VN,
			ParseMode:      parsed.ParseMode,
			TokenUsed:      tokenUsed,
			ScheduledInput: scheduledInput,
		}
	}()

	return tokenCh, metaCh, errCh
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

// ── Session Fork ───────────────────────────────────────────────────────────────

// ForkSessionReq POST /sessions/:id/fork 请求体
type ForkSessionReq struct {
	// 复制到哪个楼层序号（含）。
	// nil = 复制全部已提交楼层；0 = 不复制任何楼层（空历史分叉，相当于同模板新会话）。
	FromFloorSeq *int   `json:"from_floor_seq"`
	UserID       string `json:"user_id"` // 可选：覆盖新 session 的用户 ID
}

// ForkSession 从源会话指定楼层分叉出新会话（平行时间线 / 存档点）。
//
// 新会话继承源会话的 game_id 和 MemorySummary 缓存，
// 并复制 [1..from_floor_seq] 的 Floor/Page 历史与变量快照；
// 从 from_floor_seq+1 楼开始走新方向，不影响源会话。
func (e *GameEngine) ForkSession(_ context.Context, sourceID string, req ForkSessionReq) (string, error) {
	// 1. 加载源 Session
	var src dbmodels.GameSession
	if err := e.db.First(&src, "id = ?", sourceID).Error; err != nil {
		return "", fmt.Errorf("source session not found: %w", err)
	}

	// 2. 查询要复制的楼层（只复制已提交的）
	q := e.db.Where("session_id = ? AND status = ?", sourceID, dbmodels.FloorCommitted).
		Order("seq ASC")
	if req.FromFloorSeq != nil {
		q = q.Where("seq <= ?", *req.FromFloorSeq)
	}
	var floors []dbmodels.Floor
	if err := q.Find(&floors).Error; err != nil {
		return "", fmt.Errorf("list source floors: %w", err)
	}

	// 3. 确定新 Session 的初始变量
	// CommitTurn 将 sb.Flatten()（全量变量）写入 PageVars，
	// 所以最后一个复制楼层的激活页 PageVars 就是该楼层提交后的完整变量快照。
	initVars := []byte("{}")
	if len(floors) > 0 {
		var lastPage dbmodels.MessagePage
		if err := e.db.Where("floor_id = ? AND is_active = true", floors[len(floors)-1].ID).
			First(&lastPage).Error; err == nil {
			initVars = lastPage.PageVars
		}
	}

	// 4. 确定基础字段
	userID := src.UserID
	if req.UserID != "" {
		userID = req.UserID
	}
	title := src.Title
	if title != "" {
		title += " (fork)"
	} else {
		title = "Fork"
	}

	// 5. 事务内创建新 Session + 复制 Floor/Page
	var newSessID string
	err := e.db.Transaction(func(tx *gorm.DB) error {
		newSess := dbmodels.GameSession{
			GameID:        src.GameID,
			UserID:        userID,
			Title:         title,
			MemorySummary: src.MemorySummary,
			Variables:     initVars,
			FloorCount:    len(floors),
		}
		if err := tx.Create(&newSess).Error; err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		newSessID = newSess.ID

		for _, floor := range floors {
			// 只复制激活页（is_active = true）
			var srcPage dbmodels.MessagePage
			if err := tx.Where("floor_id = ? AND is_active = true", floor.ID).
				First(&srcPage).Error; err != nil {
				continue // 无激活页，跳过本楼
			}

			newFloor := dbmodels.Floor{
				SessionID: newSessID,
				Seq:       floor.Seq,
				Status:    dbmodels.FloorCommitted,
			}
			if err := tx.Create(&newFloor).Error; err != nil {
				return fmt.Errorf("create floor seq=%d: %w", floor.Seq, err)
			}

			newPage := dbmodels.MessagePage{
				FloorID:   newFloor.ID,
				IsActive:  true,
				Messages:  srcPage.Messages,
				PageVars:  srcPage.PageVars,
				TokenUsed: srcPage.TokenUsed,
			}
			if err := tx.Create(&newPage).Error; err != nil {
				return fmt.Errorf("create page for floor seq=%d: %w", floor.Seq, err)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return newSessID, nil
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
		MemoryLabel       string   `json:"memory_label"`
		FallbackOptions   []string `json:"fallback_options"`
		WorldbookGroupCap int      `json:"worldbook_group_cap"`
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
			Group: entry.Group, GroupWeight: entry.GroupWeight,
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
			WorldbookGroupCap:    tmplCfg.WorldbookGroupCap,
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

	// 估算 token 数（使用启发式 tokenizer，BPE 兼容）
	estTokens := tokenizer.EstimateMessages(finalMessages)

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
		EstTokens:     estTokens,
		BlockCount:    len(pCtx.Blocks),
		PresetHits:    presetHits,
		WorldbookHits: wbHits,
		MemoryUsed:    memUsed,
	}, nil
}
