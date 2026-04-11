package api

// verifier.go — Verifier 槽 + PromptSnapshot 持久化
//
// 复刻 TavernHeadless 的 Verifier instance slot：
//   - 在 Narrator 生成内容之后、CommitTurn 之前运行
//   - 使用绑定到 "verifier" slot 的 LLM Profile（通常是廉价模型）
//   - 输出 JSON { "passed": bool, "note": "..." }
//   - Verifier 失败 → 本回合仍提交，但 PromptSnapshot.VerifyPassed = false
//     （不重试；创作者可配置 max_verify_retries 后再考虑支持）
//
// PromptSnapshot 持久化：
//   - CommitTurn 成功后，在独立 goroutine 中异步写入 prompt_snapshot 表
//   - 包含：命中的世界书词条 IDs、preset_hits、est_tokens、verifier 结果

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/tokenizer"
	"mvu-backend/internal/engine/prompt_ir"
)

// VerifierResult Verifier 槽返回的校验结果
type VerifierResult struct {
	Passed bool   // 是否通过校验
	Note   string // 说明（通过时可为空）
}

// verifierDefaultPrompt 默认的 Verifier 指令（可被 GameTemplate.Config.verifier_prompt 覆盖）
const verifierDefaultPrompt = `你是内容一致性校验员。请检查以下 AI 角色扮演回复是否符合角色设定：
1. 角色的说话方式、性格、立场是否保持一致？
2. 是否存在明显的 OOC（Out of Character）破坏叙事的内容？
3. 内容是否通顺、没有明显截断或乱码？

请用以下 JSON 格式回答（只输出 JSON，不要有其他内容）：
{"passed": true/false, "note": "一句话说明，通过时留空"}

被检查的 AI 回复如下：
---
%s
---`

// runVerifier 运行 Verifier 槽校验（可选，不影响主流程）。
//
// 若当前会话没有绑定 "verifier" slot 的 LLM Profile，直接返回 (nil, nil)。
// 校验失败时只记录日志，不返回 error——Verifier 不阻断回合提交。
func (e *GameEngine) runVerifier(
	ctx context.Context,
	sessionID, userID string,
	narrative string,
	customPrompt string, // 来自 GameTemplate.Config.verifier_prompt，空串则用默认值
) (*VerifierResult, error) {
	// 1. 解析 verifier slot（无绑定则跳过）
	verClient, verOpts, ok := func() (llm.Provider, llm.Options, bool) {
		if e.registry == nil {
			return nil, llm.Options{}, false
		}
		c, o, ok := e.registry.ResolveForSlot(e.db, userID, sessionID, "verifier")
		return c, o, ok
	}()
	if !ok {
		return nil, nil // 未配置 verifier，静默跳过
	}

	// 2. 构建 Verifier Prompt
	prompt := customPrompt
	if prompt == "" {
		prompt = fmt.Sprintf(verifierDefaultPrompt, narrative)
	} else {
		prompt = fmt.Sprintf("%s\n\n---\n%s\n---", prompt, narrative)
	}

	// 3. 调用 LLM（强制低温，专注判断）
	verOpts.MaxTokens = 128
	temp := 0.0
	verOpts.Temperature = &temp

	resp, err := verClient.Chat(ctx, []llm.Message{
		{Role: "user", Content: prompt},
	}, verOpts)
	if err != nil {
		// Verifier 调用失败不阻断主流程，视为通过
		log.Printf("[verifier] session=%s llm error: %v", sessionID, err)
		return &VerifierResult{Passed: true, Note: "verifier unavailable"}, nil
	}

	// 4. 解析 JSON 输出
	var result struct {
		Passed bool   `json:"passed"`
		Note   string `json:"note"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		// 解析失败视为通过（宽容模式）
		log.Printf("[verifier] session=%s parse error: %v raw: %s", sessionID, err, resp.Content)
		return &VerifierResult{Passed: true, Note: "parse error"}, nil
	}

	if !result.Passed {
		log.Printf("[verifier] session=%s REJECTED: %s", sessionID, result.Note)
	}

	return &VerifierResult{Passed: result.Passed, Note: result.Note}, nil
}

// SnapshotInput 写入 PromptSnapshot 所需的输入数据
type SnapshotInput struct {
	SessionID             string
	FloorID               string
	ActivatedWorldbookIDs []string
	FinalMessages         []map[string]string
	PipelineCtx           *prompt_ir.ContextData // 用于统计 block 类型
	VerifyResult          *VerifierResult         // nil = 未运行
}

// savePromptSnapshot 异步写入 PromptSnapshot（在 goroutine 中调用，不阻塞主流程）。
func (e *GameEngine) savePromptSnapshot(input SnapshotInput) {
	// 统计 preset_hits 和 worldbook_hits
	presetHits, wbHits := 0, 0
	if input.PipelineCtx != nil {
		for _, b := range input.PipelineCtx.Blocks {
			switch b.Type {
			case prompt_ir.BlockPreset:
				presetHits++
			case prompt_ir.BlockWorldbook:
				wbHits++
			}
		}
	}

	// 粗估 token 总数
	estTokens := tokenizer.EstimateMessages(input.FinalMessages)

	// 序列化命中词条 IDs
	wbIDsJSON, _ := json.Marshal(input.ActivatedWorldbookIDs)

	snap := dbmodels.PromptSnapshot{
		SessionID:             input.SessionID,
		FloorID:               input.FloorID,
		ActivatedWorldbookIDs: wbIDsJSON,
		PresetHits:            presetHits,
		WorldbookHits:         wbHits,
		EstTokens:             estTokens,
		CreatedAt:             time.Now(),
	}

	if input.VerifyResult != nil {
		passed := input.VerifyResult.Passed
		snap.VerifyPassed = &passed
		snap.VerifyNote = input.VerifyResult.Note
	}

	if err := e.db.Create(&snap).Error; err != nil {
		log.Printf("[snapshot] save error floor=%s: %v", input.FloorID, err)
	}
}
