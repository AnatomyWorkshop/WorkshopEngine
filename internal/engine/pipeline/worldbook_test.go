package pipeline

import (
	"strings"
	"testing"

	"mvu-backend/internal/engine/prompt_ir"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeEntry(id, content string, keys []string) prompt_ir.WorldbookEntry {
	return prompt_ir.WorldbookEntry{
		ID:      id,
		Keys:    keys,
		Content: content,
		Enabled: true,
	}
}

func msgs(texts ...string) []prompt_ir.Message {
	out := make([]prompt_ir.Message, len(texts))
	for i, t := range texts {
		out[i] = prompt_ir.Message{Role: "user", Content: t}
	}
	return out
}

func runWorldbook(entries []prompt_ir.WorldbookEntry, messages []prompt_ir.Message) *prompt_ir.ContextData {
	ctx := &prompt_ir.ContextData{
		Config: prompt_ir.GameConfig{
			WorldbookEntries: entries,
		},
		RecentMessages: messages,
	}
	n := NewWorldbookNode()
	_ = n.Process(ctx)
	return ctx
}

// ── Primary key matching ──────────────────────────────────────────────────────

func TestWorldbook_PrimaryKey_Hit(t *testing.T) {
	entries := []prompt_ir.WorldbookEntry{makeEntry("e1", "NPC info", []string{"夜歌"})}
	ctx := runWorldbook(entries, msgs("我遇见了夜歌"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_PrimaryKey_Miss(t *testing.T) {
	entries := []prompt_ir.WorldbookEntry{makeEntry("e1", "NPC info", []string{"夜歌"})}
	ctx := runWorldbook(entries, msgs("今天天气不错"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_CaseInsensitive(t *testing.T) {
	entries := []prompt_ir.WorldbookEntry{makeEntry("e1", "info", []string{"nightsong"})}
	ctx := runWorldbook(entries, msgs("I met NIGHTSONG today"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("expected 1 block (case-insensitive match), got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_Constant_AlwaysInjected(t *testing.T) {
	e := makeEntry("e1", "Always here", []string{"never_matches_keyword_xyz_abc"})
	e.Constant = true
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("unrelated"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("constant entry should always inject, got %d blocks", len(ctx.Blocks))
	}
}

func TestWorldbook_RegexKey(t *testing.T) {
	entries := []prompt_ir.WorldbookEntry{makeEntry("e1", "info", []string{"regex:夜[歌曲唱]"})}
	ctx := runWorldbook(entries, msgs("夜歌在唱歌"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("regex key should match, got %d blocks", len(ctx.Blocks))
	}
}

func TestWorldbook_RegexKey_Miss(t *testing.T) {
	entries := []prompt_ir.WorldbookEntry{makeEntry("e1", "info", []string{"regex:^只出现在行首"})}
	ctx := runWorldbook(entries, msgs("文字 只出现在行首"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("regex key should not match mid-text, got %d blocks", len(ctx.Blocks))
	}
}

func TestWorldbook_WholeWord(t *testing.T) {
	e := makeEntry("e1", "info", []string{"cat"})
	e.WholeWord = true
	// "category" should NOT match "cat" with whole-word
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("I bought a category"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("whole word match should reject partial match, got %d blocks", len(ctx.Blocks))
	}
	// "cat" should match
	ctx = runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("I saw a cat today"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("whole word match should hit exact word, got %d blocks", len(ctx.Blocks))
	}
}

// ── Secondary keys / logic gates ─────────────────────────────────────────────

func makeEntryWithSecondary(id, content string, keys, secKeys []string, logic string) prompt_ir.WorldbookEntry {
	return prompt_ir.WorldbookEntry{
		ID:             id,
		Keys:           keys,
		SecondaryKeys:  secKeys,
		SecondaryLogic: logic,
		Content:        content,
		Enabled:        true,
	}
}

func TestWorldbook_Secondary_AndAny_Hit(t *testing.T) {
	e := makeEntryWithSecondary("e1", "info", []string{"夜歌"}, []string{"信任", "好感"}, "and_any")
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌对我的信任度上升了"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("and_any should trigger on any secondary match, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_Secondary_AndAny_Miss(t *testing.T) {
	e := makeEntryWithSecondary("e1", "info", []string{"夜歌"}, []string{"信任", "好感"}, "and_any")
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌看着远方"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("and_any should not trigger without any secondary match, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_Secondary_AndAll_AllPresent(t *testing.T) {
	e := makeEntryWithSecondary("e1", "info", []string{"夜歌"}, []string{"信任", "好感"}, "and_all")
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌信任我，好感上升了"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("and_all should trigger when all secondary present, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_Secondary_AndAll_OneMissing(t *testing.T) {
	e := makeEntryWithSecondary("e1", "info", []string{"夜歌"}, []string{"信任", "好感"}, "and_all")
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌信任我"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("and_all should not trigger with partial secondary match, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_Secondary_NotAny(t *testing.T) {
	e := makeEntryWithSecondary("e1", "info", []string{"夜歌"}, []string{"愤怒"}, "not_any")
	// No "愤怒" present → should trigger
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌平静地说话"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("not_any should trigger when no secondary match, got %d", len(ctx.Blocks))
	}
	// "愤怒" present → should NOT trigger
	ctx = runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌愤怒地喊道"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("not_any should not trigger when secondary matches, got %d", len(ctx.Blocks))
	}
}

func TestWorldbook_Secondary_NotAll(t *testing.T) {
	e := makeEntryWithSecondary("e1", "info", []string{"夜歌"}, []string{"愤怒", "仇恨"}, "not_all")
	// Only one match → not_all passes (not ALL matched)
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌愤怒地说"))
	if len(ctx.Blocks) != 1 {
		t.Fatalf("not_all should trigger when secondary not fully matched, got %d", len(ctx.Blocks))
	}
	// Both match → not_all fails
	ctx = runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌充满愤怒和仇恨"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("not_all should not trigger when all secondary match, got %d", len(ctx.Blocks))
	}
}

// ── ScanDepth ─────────────────────────────────────────────────────────────────

func TestWorldbook_ScanDepth(t *testing.T) {
	e := makeEntry("e1", "info", []string{"夜歌"})
	e.ScanDepth = 1 // only scan last 1 message

	history := msgs("夜歌出现了", "然后故事继续") // keyword in first, not last
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, history)
	if len(ctx.Blocks) != 0 {
		t.Fatalf("scan_depth=1 should only scan last message, keyword is not there, got %d blocks", len(ctx.Blocks))
	}

	history2 := msgs("无关内容", "夜歌又出现了") // keyword in last message
	ctx2 := runWorldbook([]prompt_ir.WorldbookEntry{e}, history2)
	if len(ctx2.Blocks) != 1 {
		t.Fatalf("scan_depth=1 should find keyword in last message, got %d blocks", len(ctx2.Blocks))
	}
}

// ── Recursive activation ──────────────────────────────────────────────────────

func TestWorldbook_RecursiveActivation(t *testing.T) {
	// Entry A activates on "触发词" and its content contains "递归词"
	// Entry B activates on "递归词" - should be activated by A's content
	entryA := makeEntry("A", "递归词在这里", []string{"触发词"})
	entryB := makeEntry("B", "B的内容", []string{"递归词"})

	ctx := runWorldbook([]prompt_ir.WorldbookEntry{entryA, entryB}, msgs("文章中有触发词"))
	if len(ctx.Blocks) != 2 {
		t.Fatalf("recursive activation should trigger B via A's content, got %d blocks", len(ctx.Blocks))
	}
}

// ── Position / Priority ───────────────────────────────────────────────────────

func TestWorldbook_Position_BeforeTemplate(t *testing.T) {
	e := makeEntry("e1", "info", []string{"夜歌"})
	e.Position = "before_template"
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌"))
	if len(ctx.Blocks) == 0 {
		t.Fatal("expected block")
	}
	if ctx.Blocks[0].Priority >= 1000 {
		t.Fatalf("before_template should have priority < 1000, got %d", ctx.Blocks[0].Priority)
	}
}

func TestWorldbook_Position_AfterTemplate(t *testing.T) {
	e := makeEntry("e1", "info", []string{"夜歌"})
	e.Position = "after_template"
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌"))
	if len(ctx.Blocks) == 0 {
		t.Fatal("expected block")
	}
	if ctx.Blocks[0].Priority <= 1000 {
		t.Fatalf("after_template should have priority > 1000, got %d", ctx.Blocks[0].Priority)
	}
}

// ── Disabled entries ──────────────────────────────────────────────────────────

func TestWorldbook_DisabledEntry(t *testing.T) {
	e := makeEntry("e1", "info", []string{"夜歌"})
	e.Enabled = false
	ctx := runWorldbook([]prompt_ir.WorldbookEntry{e}, msgs("夜歌在这里"))
	if len(ctx.Blocks) != 0 {
		t.Fatalf("disabled entry should not inject, got %d blocks", len(ctx.Blocks))
	}
}

// ── Pipeline Runner ───────────────────────────────────────────────────────────

func TestRunner_BasicExecution(t *testing.T) {
	ctx := &prompt_ir.ContextData{
		Mode: prompt_ir.ModeNative,
		Config: prompt_ir.GameConfig{
			SystemPromptTemplate: "你是一个游戏主持人。",
		},
		RecentMessages: []prompt_ir.Message{
			{Role: "user", Content: "你好"},
		},
		TokenBudget: 4000,
	}
	runner := NewRunner()
	msgs, err := runner.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	// Should have system message with template
	hasSystem := false
	for _, m := range msgs {
		if m["role"] == "system" && strings.Contains(m["content"], "游戏主持人") {
			hasSystem = true
		}
	}
	if !hasSystem {
		t.Fatalf("expected system message with template content, got: %v", msgs)
	}
	// Should have user message at end
	last := msgs[len(msgs)-1]
	if last["role"] != "user" || last["content"] != "你好" {
		t.Fatalf("last message should be user input, got: %v", last)
	}
}

func TestRunner_WorldbookInjectedBeforeHistory(t *testing.T) {
	ctx := &prompt_ir.ContextData{
		Mode: prompt_ir.ModeNative,
		Config: prompt_ir.GameConfig{
			SystemPromptTemplate: "系统提示词",
			WorldbookEntries: []prompt_ir.WorldbookEntry{
				makeEntry("e1", "夜歌是一个歌手", []string{"夜歌"}),
			},
		},
		RecentMessages: []prompt_ir.Message{
			{Role: "user", Content: "夜歌今天在哪里？"},
		},
		TokenBudget: 4000,
	}
	runner := NewRunner()
	result, err := runner.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// Find system message content
	var systemContent string
	for _, m := range result {
		if m["role"] == "system" {
			systemContent += m["content"]
		}
	}
	if !strings.Contains(systemContent, "夜歌是一个歌手") {
		t.Fatalf("worldbook entry should be in system message, system=%q", systemContent)
	}
}
