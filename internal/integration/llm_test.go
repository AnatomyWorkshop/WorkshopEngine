//go:build integration

// Package integration contains end-to-end tests that call the real LLM API.
// Run with: go test -tags integration ./internal/integration/... -v
// Requires .test/.env (loaded via LLM_* env vars or set manually).
package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/parser"
	"mvu-backend/internal/engine/pipeline"
	"mvu-backend/internal/engine/prompt_ir"
)

func newClient(t *testing.T) *llm.Client {
	t.Helper()
	baseURL := os.Getenv("LLM_BASE_URL")
	apiKey := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL")
	if apiKey == "" {
		t.Skip("LLM_API_KEY not set")
	}
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	if model == "" {
		model = "deepseek-chat"
	}
	return llm.NewClient(baseURL, apiKey, model, 30, 1)
}

// TestIntegration_LLMChat verifies the LLM client can reach DeepSeek.
func TestIntegration_LLMChat(t *testing.T) {
	client := newClient(t)
	resp, err := client.Chat(context.Background(), []llm.Message{
		{Role: "user", Content: "Reply with exactly: OK"},
	}, llm.Options{MaxTokens: 10})
	if err != nil {
		t.Fatalf("LLM call failed: %v", err)
	}
	if resp.Content == "" {
		t.Fatal("empty response from LLM")
	}
	t.Logf("LLM response: %q", resp.Content)
}

// TestIntegration_ParserWithLLMOutput feeds real LLM output through the parser.
func TestIntegration_ParserWithLLMOutput(t *testing.T) {
	client := newClient(t)

	resp, err := client.Chat(context.Background(), []llm.Message{
		{Role: "system", Content: "你是一个文字游戏主持人。请用以下XML格式回复：\n<Narrative>（叙事内容）</Narrative>\n<Options>\n<option>选项一</option>\n<option>选项二</option>\n</Options>"},
		{Role: "user", Content: "开始游戏"},
	}, llm.Options{MaxTokens: 200})
	if err != nil {
		t.Fatalf("LLM call failed: %v", err)
	}
	t.Logf("Raw LLM output:\n%s", resp.Content)

	result := parser.Parse(resp.Content)
	t.Logf("ParseMode=%s Narrative=%q Options=%v", result.ParseMode, result.Narrative, result.Options)

	if result.Narrative == "" {
		t.Error("parser produced empty narrative from LLM output")
	}
}

// TestIntegration_PipelineWithWorldbook verifies worldbook injection + LLM response.
func TestIntegration_PipelineWithWorldbook(t *testing.T) {
	ctx := &prompt_ir.ContextData{
		Mode: prompt_ir.ModeNative,
		Config: prompt_ir.GameConfig{
			SystemPromptTemplate: "你是一个游戏主持人。",
			WorldbookEntries: []prompt_ir.WorldbookEntry{
				{ID: "npc1", Keys: []string{"夜歌"}, Content: "夜歌是一位神秘的歌手。", Enabled: true},
			},
		},
		RecentMessages: []prompt_ir.Message{
			{Role: "user", Content: "夜歌在哪里？"},
		},
		TokenBudget: 4000,
	}

	runner := pipeline.NewRunner()
	msgs, err := runner.Execute(ctx)
	if err != nil {
		t.Fatalf("pipeline failed: %v", err)
	}

	var systemContent string
	for _, m := range msgs {
		if m["role"] == "system" {
			systemContent += m["content"]
		}
	}
	if !strings.Contains(systemContent, "夜歌是一位神秘的歌手") {
		t.Errorf("worldbook not injected, system=%q", systemContent)
	}

	client := newClient(t)
	llmMsgs := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		llmMsgs[i] = llm.Message{Role: m["role"], Content: m["content"]}
	}
	resp, err := client.Chat(context.Background(), llmMsgs, llm.Options{MaxTokens: 300})
	if err != nil {
		t.Fatalf("LLM call failed: %v", err)
	}
	t.Logf("LLM response: %q", resp.Content)
	if resp.Content == "" {
		t.Error("empty LLM response")
	}
}
