// Package mvu_test — black-box unit tests for backend-v2.
// All tests run without network access.
// Run: cd .test && go test ./... -v -count=1
package mvu_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/parser"
	"mvu-backend/internal/engine/pipeline"
	"mvu-backend/internal/engine/processor"
	"mvu-backend/internal/engine/prompt_ir"
	"mvu-backend/internal/engine/scheduled"
	"mvu-backend/internal/engine/tokenizer"
	"mvu-backend/internal/engine/variable"
	"mvu-backend/internal/social/forum"
)

// ═══════════════════════════════════════════════════════════════════════════
// helpers
// ═══════════════════════════════════════════════════════════════════════════

func wb(id, content string, keys []string) prompt_ir.WorldbookEntry {
	return prompt_ir.WorldbookEntry{ID: id, Keys: keys, Content: content, Enabled: true}
}

func wbConst(id, content string) prompt_ir.WorldbookEntry {
	e := wb(id, content, nil)
	e.Constant = true
	return e
}

func wbVar(id, content, varExpr string) prompt_ir.WorldbookEntry {
	return wb(id, content, []string{"var:" + varExpr})
}

func wbGroup(id, content, group string, weight float64) prompt_ir.WorldbookEntry {
	e := wb(id, content, []string{"触发词"})
	e.Group = group
	e.GroupWeight = weight
	return e
}

func runWB(entries []prompt_ir.WorldbookEntry, vars map[string]any, texts ...string) *prompt_ir.ContextData {
	msgs := make([]prompt_ir.Message, len(texts))
	for i, t := range texts {
		msgs[i] = prompt_ir.Message{Role: "user", Content: t}
	}
	ctx := &prompt_ir.ContextData{
		Config:         prompt_ir.GameConfig{WorldbookEntries: entries},
		Variables:      vars,
		RecentMessages: msgs,
	}
	_ = pipeline.NewWorldbookNode().Process(ctx)
	return ctx
}

func blocksLen(ctx *prompt_ir.ContextData) int { return len(ctx.Blocks) }

// ═══════════════════════════════════════════════════════════════════════════
// 1. TOKENIZER
// ═══════════════════════════════════════════════════════════════════════════

func TestTokenizer_Empty(t *testing.T) {
	if tokenizer.Estimate("") != 0 {
		t.Fatal("empty → 0")
	}
}

func TestTokenizer_ASCIIRange(t *testing.T) {
	n := tokenizer.Estimate("hello world")
	if n < 1 || n > 8 {
		t.Fatalf("unexpected estimate %d", n)
	}
}

func TestTokenizer_CJK(t *testing.T) {
	n := tokenizer.Estimate("夜歌")
	if n < 1 || n > 4 {
		t.Fatalf("unexpected estimate %d for CJK", n)
	}
}

func TestTokenizer_LongerIsMore(t *testing.T) {
	short := tokenizer.Estimate("hi")
	long := tokenizer.Estimate(strings.Repeat("hello world ", 20))
	if long <= short {
		t.Fatalf("long (%d) should > short (%d)", long, short)
	}
}

func TestTokenizer_Messages_Overhead(t *testing.T) {
	msgs := []map[string]string{{"role": "user", "content": "hi"}}
	got := tokenizer.EstimateMessages(msgs)
	base := tokenizer.Estimate("hi")
	if got != base+4 {
		t.Fatalf("want %d got %d", base+4, got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 2. PARSER
// ═══════════════════════════════════════════════════════════════════════════

func TestParser_XML_Basic(t *testing.T) {
	r := parser.Parse(`<Narrative>夜歌沉默地看着你。</Narrative>
<Options><option>等待</option><option>离开</option></Options>`)
	if r.ParseMode != "xml_narrative" {
		t.Fatalf("mode %q", r.ParseMode)
	}
	if r.Narrative != "夜歌沉默地看着你。" {
		t.Fatalf("narrative %q", r.Narrative)
	}
	if len(r.Options) != 2 {
		t.Fatalf("options %d", len(r.Options))
	}
}

func TestParser_XML_Summary(t *testing.T) {
	r := parser.Parse(`<Narrative>故事。</Narrative><Summary>紧张关系。</Summary>`)
	if r.Summary != "紧张关系。" {
		t.Fatalf("summary %q", r.Summary)
	}
}

func TestParser_XML_StatePatch(t *testing.T) {
	r := parser.Parse(`<Narrative>x</Narrative><UpdateState>{"hp":80}</UpdateState>`)
	if r.StatePatch == nil || r.StatePatch["hp"] == nil {
		t.Fatal("expected hp in state_patch")
	}
}

func TestParser_NumberedList(t *testing.T) {
	r := parser.Parse("叙事\n1. 选A\n2. 选B\n3. 选C")
	if r.ParseMode != "numbered_list" {
		t.Fatalf("mode %q", r.ParseMode)
	}
	if len(r.Options) != 3 {
		t.Fatalf("options %d", len(r.Options))
	}
}

func TestParser_CircledNumbers(t *testing.T) {
	r := parser.Parse("叙事\n①.选项一\n②.选项二\n③.选项三")
	if r.ParseMode != "numbered_list" {
		t.Fatalf("mode %q", r.ParseMode)
	}
	if len(r.Options) != 3 {
		t.Fatalf("options %d", len(r.Options))
	}
}

func TestParser_Fallback_PlainText(t *testing.T) {
	raw := "这段文字没有结构。"
	r := parser.Parse(raw)
	if r.ParseMode != "fallback" {
		t.Fatalf("mode %q", r.ParseMode)
	}
	if r.Narrative != raw {
		t.Fatal("narrative should equal raw")
	}
}

func TestParser_VN_GameResponse(t *testing.T) {
	r := parser.Parse(`<game_response>
[bg|city_night.jpg]
[bgm|rain]
旁白||夜色深沉。
夜歌|sad.png|……你来了。
[choice|回应|沉默|离开]
</game_response>`)
	if r.ParseMode != "xml_game_response" {
		t.Fatalf("mode %q", r.ParseMode)
	}
	if r.VN == nil {
		t.Fatal("expected VN")
	}
	if r.VN.BG != "city_night.jpg" {
		t.Fatalf("BG %q", r.VN.BG)
	}
	if r.VN.BGM != "rain" {
		t.Fatalf("BGM %q", r.VN.BGM)
	}
	if len(r.Options) != 3 {
		t.Fatalf("choices %d", len(r.Options))
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 3. VARIABLE SANDBOX
// ═══════════════════════════════════════════════════════════════════════════

func TestSandbox_SetGet(t *testing.T) {
	sb := variable.NewSandbox(nil, nil, nil, nil, nil)
	sb.Set("hp", 100)
	v, ok := sb.Get("hp")
	if !ok || v != 100 {
		t.Fatalf("got %v ok=%v", v, ok)
	}
}

func TestSandbox_PageOverridesChat(t *testing.T) {
	sb := variable.NewSandbox(nil, map[string]any{"x": "chat"}, nil, nil, nil)
	sb.Set("x", "page")
	v, _ := sb.Get("x")
	if v != "page" {
		t.Fatalf("page should win, got %v", v)
	}
}

func TestSandbox_ScopePrecedence(t *testing.T) {
	sb := variable.NewSandbox(
		map[string]any{"k": "global"},
		map[string]any{"k": "chat"},
		map[string]any{"k": "branch"},
		map[string]any{"k": "floor"},
		map[string]any{"k": "page"},
	)
	v, _ := sb.Get("k")
	if v != "page" {
		t.Fatalf("page should win, got %v", v)
	}
}

func TestSandbox_Flatten_Copy(t *testing.T) {
	sb := variable.NewSandbox(nil, map[string]any{"k": "orig"}, nil, nil, nil)
	flat := sb.Flatten()
	flat["k"] = "mutated"
	v, _ := sb.Get("k")
	if fmt.Sprintf("%v", v) == "mutated" {
		t.Fatal("Flatten should return a copy")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 4. SCHEDULED TRIGGERS
// ═══════════════════════════════════════════════════════════════════════════

func TestScheduled_Triggers(t *testing.T) {
	rules := []scheduled.TriggerRule{{
		ID: "r1", ConditionVar: "tension", Threshold: 70, Probability: 1.0,
		UserInput: "[hostile]",
	}}
	r := scheduled.Evaluate(rules, map[string]any{"tension": float64(80)}, 10, 0.1)
	if r == nil || r.ID != "r1" {
		t.Fatalf("expected trigger got %v", r)
	}
}

func TestScheduled_BelowThreshold(t *testing.T) {
	rules := []scheduled.TriggerRule{{ID: "r1", ConditionVar: "tension", Threshold: 70, Probability: 1.0}}
	r := scheduled.Evaluate(rules, map[string]any{"tension": float64(60)}, 10, 0.1)
	if r != nil {
		t.Fatal("should not trigger below threshold")
	}
}

func TestScheduled_CooldownBlocks(t *testing.T) {
	vars := map[string]any{
		"tension":                    float64(80),
		scheduled.CooldownKey("r1"): float64(8),
	}
	rules := []scheduled.TriggerRule{{
		ID: "r1", ConditionVar: "tension", Threshold: 70,
		Probability: 1.0, CooldownFloors: 5,
	}}
	if scheduled.Evaluate(rules, vars, 10, 0.1) != nil {
		t.Fatal("cooldown should block at floor 10 (diff=2)")
	}
	if scheduled.Evaluate(rules, vars, 14, 0.1) == nil {
		t.Fatal("should trigger at floor 14 (diff=6)")
	}
}

func TestScheduled_NestedVar(t *testing.T) {
	vars := map[string]any{"emotion": map[string]any{"trust": float64(90)}}
	rules := []scheduled.TriggerRule{{
		ID: "r1", ConditionVar: "emotion.trust", Threshold: 80, Probability: 1.0,
	}}
	if scheduled.Evaluate(rules, vars, 5, 0.1) == nil {
		t.Fatal("nested var should work")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 5. REGEX PROCESSOR
// ═══════════════════════════════════════════════════════════════════════════

func TestRegex_AIOutput_Basic(t *testing.T) {
	rules := []prompt_ir.RegexRule{{Pattern: `\[系统\]`, Replacement: "[SYS]", ApplyTo: "ai_output", Enabled: true}}
	got := processor.ApplyToAIOutput("[系统]触发", rules)
	if got != "[SYS]触发" {
		t.Fatalf("got %q", got)
	}
}

func TestRegex_UserInput_NotAppliedToAI(t *testing.T) {
	rules := []prompt_ir.RegexRule{{Pattern: `foo`, Replacement: "bar", ApplyTo: "user_input", Enabled: true}}
	got := processor.ApplyToAIOutput("foo", rules)
	if got != "foo" {
		t.Fatalf("user_input rule should not apply to ai, got %q", got)
	}
}

func TestRegex_All_AppliesBoth(t *testing.T) {
	rules := []prompt_ir.RegexRule{{Pattern: `x`, Replacement: "y", ApplyTo: "all", Enabled: true}}
	if processor.ApplyToAIOutput("x", rules) != "y" {
		t.Fatal("all rule should apply to ai_output")
	}
	if processor.ApplyToUserInput("x", rules) != "y" {
		t.Fatal("all rule should apply to user_input")
	}
}

func TestRegex_CaptureGroup(t *testing.T) {
	rules := []prompt_ir.RegexRule{{Pattern: `\*([^*]+)\*`, Replacement: "$1", ApplyTo: "ai_output", Enabled: true}}
	got := processor.ApplyToAIOutput("她*轻声*说", rules)
	if got != "她轻声说" {
		t.Fatalf("capture group failed: %q", got)
	}
}

func TestRegex_Chained(t *testing.T) {
	rules := []prompt_ir.RegexRule{
		{Pattern: "A", Replacement: "B", ApplyTo: "ai_output", Enabled: true},
		{Pattern: "B", Replacement: "C", ApplyTo: "ai_output", Enabled: true},
	}
	got := processor.ApplyToAIOutput("A", rules)
	if got != "C" {
		t.Fatalf("chained failed: %q", got)
	}
}

func TestRegex_SlashFlags(t *testing.T) {
	rules := []prompt_ir.RegexRule{{Pattern: `/HELLO/i`, Replacement: "HI", ApplyTo: "ai_output", Enabled: true}}
	got := processor.ApplyToAIOutput("say hello", rules)
	if got != "say HI" {
		t.Fatalf("slash flag failed: %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 6. WORLDBOOK — basic matching (migrated from package-internal tests)
// ═══════════════════════════════════════════════════════════════════════════

func TestWorldbook_KeyHit(t *testing.T) {
	ctx := runWB([]prompt_ir.WorldbookEntry{wb("e1", "info", []string{"夜歌"})}, nil, "我遇见了夜歌")
	if blocksLen(ctx) != 1 {
		t.Fatalf("want 1 got %d", blocksLen(ctx))
	}
}

func TestWorldbook_KeyMiss(t *testing.T) {
	ctx := runWB([]prompt_ir.WorldbookEntry{wb("e1", "info", []string{"夜歌"})}, nil, "今天天气不错")
	if blocksLen(ctx) != 0 {
		t.Fatalf("want 0 got %d", blocksLen(ctx))
	}
}

func TestWorldbook_Constant(t *testing.T) {
	ctx := runWB([]prompt_ir.WorldbookEntry{wbConst("e1", "always")}, nil, "unrelated")
	if blocksLen(ctx) != 1 {
		t.Fatal("constant should always inject")
	}
}

func TestWorldbook_CaseInsensitive(t *testing.T) {
	ctx := runWB([]prompt_ir.WorldbookEntry{wb("e1", "x", []string{"alice"})}, nil, "I met ALICE")
	if blocksLen(ctx) != 1 {
		t.Fatal("case-insensitive failed")
	}
}

func TestWorldbook_Regex(t *testing.T) {
	ctx := runWB([]prompt_ir.WorldbookEntry{wb("e1", "x", []string{"regex:夜[歌曲]"})}, nil, "夜曲响起")
	if blocksLen(ctx) != 1 {
		t.Fatal("regex should match")
	}
}

func TestWorldbook_WholeWord_Reject(t *testing.T) {
	e := wb("e1", "x", []string{"cat"})
	e.WholeWord = true
	ctx := runWB([]prompt_ir.WorldbookEntry{e}, nil, "category here")
	if blocksLen(ctx) != 0 {
		t.Fatal("whole-word should reject partial match")
	}
}

func TestWorldbook_WholeWord_Accept(t *testing.T) {
	e := wb("e1", "x", []string{"cat"})
	e.WholeWord = true
	ctx := runWB([]prompt_ir.WorldbookEntry{e}, nil, "a cat sat")
	if blocksLen(ctx) != 1 {
		t.Fatal("whole-word should match exact word")
	}
}

func TestWorldbook_SecondaryAndAny(t *testing.T) {
	e := wb("e1", "x", []string{"夜歌"})
	e.SecondaryKeys = []string{"信任", "好感"}
	e.SecondaryLogic = "and_any"
	e.Enabled = true
	ctx := runWB([]prompt_ir.WorldbookEntry{e}, nil, "夜歌的信任度上升")
	if blocksLen(ctx) != 1 {
		t.Fatal("and_any should hit on any secondary")
	}
}

func TestWorldbook_SecondaryNotAny(t *testing.T) {
	e := wb("e1", "x", []string{"夜歌"})
	e.SecondaryKeys = []string{"愤怒"}
	e.SecondaryLogic = "not_any"
	e.Enabled = true
	ctx := runWB([]prompt_ir.WorldbookEntry{e}, nil, "夜歌平静说话")
	if blocksLen(ctx) != 1 {
		t.Fatal("not_any should pass when secondary absent")
	}
}

func TestWorldbook_ScanDepth(t *testing.T) {
	e := wb("e1", "x", []string{"夜歌"})
	e.ScanDepth = 1
	e.Enabled = true
	// keyword in first message only — should miss with depth=1
	ctx := runWB([]prompt_ir.WorldbookEntry{e}, nil, "夜歌出现了", "然后故事继续")
	if blocksLen(ctx) != 0 {
		t.Fatal("scan_depth=1 should miss keyword in old message")
	}
	// keyword in last message — should hit
	ctx = runWB([]prompt_ir.WorldbookEntry{e}, nil, "无关内容", "夜歌又来了")
	if blocksLen(ctx) != 1 {
		t.Fatal("scan_depth=1 should find keyword in latest message")
	}
}

func TestWorldbook_RecursiveActivation(t *testing.T) {
	// A activates on "触发词", content contains "递归词"
	// B activates on "递归词" → should be triggered by A's content
	entA := wb("A", "递归词在这里", []string{"触发词"})
	entB := wb("B", "B内容", []string{"递归词"})
	ctx := runWB([]prompt_ir.WorldbookEntry{entA, entB}, nil, "文章有触发词")
	if blocksLen(ctx) != 2 {
		t.Fatalf("recursive activation should give 2 blocks, got %d", blocksLen(ctx))
	}
}

func TestWorldbook_Disabled(t *testing.T) {
	e := wb("e1", "x", []string{"夜歌"})
	e.Enabled = false
	ctx := runWB([]prompt_ir.WorldbookEntry{e}, nil, "夜歌来了")
	if blocksLen(ctx) != 0 {
		t.Fatal("disabled entry should not inject")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 7. WORLDBOOK — GROUP CAP (3-C)
// ═══════════════════════════════════════════════════════════════════════════

func TestWorldbook_GroupCap_KeepsHighestWeight(t *testing.T) {
	// Three entries in same group, only highest-weight should survive
	entries := []prompt_ir.WorldbookEntry{
		wbGroup("low", "low weight", "scene", 1.0),
		wbGroup("high", "high weight", "scene", 10.0),
		wbGroup("mid", "mid weight", "scene", 5.0),
	}
	ctx := runWB(entries, nil, "触发词")
	if blocksLen(ctx) != 1 {
		t.Fatalf("group cap=1: want 1 block, got %d", blocksLen(ctx))
	}
	if !strings.Contains(ctx.Blocks[0].Content, "high weight") {
		t.Fatalf("expected highest-weight entry, got: %q", ctx.Blocks[0].Content)
	}
}

func TestWorldbook_GroupCap_Ungrouped_NotAffected(t *testing.T) {
	// Ungrouped entries should all pass through even when group cap=1
	entries := []prompt_ir.WorldbookEntry{
		wb("u1", "ungrouped1", []string{"触发词"}),
		wb("u2", "ungrouped2", []string{"触发词"}),
		wb("u3", "ungrouped3", []string{"触发词"}),
	}
	ctx := runWB(entries, nil, "触发词")
	if blocksLen(ctx) != 3 {
		t.Fatalf("ungrouped entries all should inject, got %d", blocksLen(ctx))
	}
}

func TestWorldbook_GroupCap_MultipleGroups_Independent(t *testing.T) {
	// Two different groups, each should only keep 1 entry (their respective best)
	entries := []prompt_ir.WorldbookEntry{
		wbGroup("a-low", "group A low", "A", 1.0),
		wbGroup("a-high", "group A high", "A", 9.0),
		wbGroup("b-low", "group B low", "B", 1.0),
		wbGroup("b-high", "group B high", "B", 9.0),
	}
	ctx := runWB(entries, nil, "触发词")
	if blocksLen(ctx) != 2 {
		t.Fatalf("2 groups × 1 cap = 2 blocks, got %d", blocksLen(ctx))
	}
	var contents []string
	for _, b := range ctx.Blocks {
		contents = append(contents, b.Content)
	}
	for _, c := range contents {
		if strings.Contains(c, "low") {
			t.Fatalf("low-weight entries should be filtered, blocks: %v", contents)
		}
	}
}

func TestWorldbook_GroupCap_Cap2(t *testing.T) {
	// When cap=2, top-2 by weight are kept
	entries := []prompt_ir.WorldbookEntry{
		wbGroup("w1", "weight-1", "g", 1.0),
		wbGroup("w5", "weight-5", "g", 5.0),
		wbGroup("w9", "weight-9", "g", 9.0),
	}
	ctx := &prompt_ir.ContextData{
		Config: prompt_ir.GameConfig{
			WorldbookEntries:  entries,
			WorldbookGroupCap: 2,
		},
		RecentMessages: []prompt_ir.Message{{Role: "user", Content: "触发词"}},
	}
	_ = pipeline.NewWorldbookNode().Process(ctx)
	if blocksLen(ctx) != 2 {
		t.Fatalf("cap=2 should keep top-2, got %d", blocksLen(ctx))
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 8. WORLDBOOK — VAR: GATE (3-D)
// ═══════════════════════════════════════════════════════════════════════════

func TestWorldbook_VarGate_Equals_Hit(t *testing.T) {
	entry := wbVar("e1", "confrontation info", "stage=confrontation")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"stage": "confrontation"})
	if blocksLen(ctx) != 1 {
		t.Fatal("var= should activate when variable matches")
	}
}

func TestWorldbook_VarGate_Equals_Miss(t *testing.T) {
	entry := wbVar("e1", "confrontation info", "stage=confrontation")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"stage": "investigation"})
	if blocksLen(ctx) != 0 {
		t.Fatal("var= should NOT activate when variable differs")
	}
}

func TestWorldbook_VarGate_Equals_MissingVar(t *testing.T) {
	entry := wbVar("e1", "info", "stage=confrontation")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{})
	if blocksLen(ctx) != 0 {
		t.Fatal("missing variable should not activate")
	}
}

func TestWorldbook_VarGate_NotEquals_Hit(t *testing.T) {
	// var:stage!=investigation → activates in any stage EXCEPT investigation
	entry := wbVar("e1", "non-investigation info", "stage!=investigation")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"stage": "confrontation"})
	if blocksLen(ctx) != 1 {
		t.Fatal("var!= should activate when variable differs")
	}
}

func TestWorldbook_VarGate_NotEquals_Miss(t *testing.T) {
	entry := wbVar("e1", "info", "stage!=investigation")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"stage": "investigation"})
	if blocksLen(ctx) != 0 {
		t.Fatal("var!= should NOT activate when variable matches")
	}
}

func TestWorldbook_VarGate_Exists_Hit(t *testing.T) {
	// var:boss_defeated — activates when variable is set and non-empty
	entry := wbVar("e1", "boss defeated content", "boss_defeated")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"boss_defeated": "true"})
	if blocksLen(ctx) != 1 {
		t.Fatal("var exists should activate when set")
	}
}

func TestWorldbook_VarGate_Exists_Miss(t *testing.T) {
	entry := wbVar("e1", "info", "boss_defeated")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{})
	if blocksLen(ctx) != 0 {
		t.Fatal("var exists should NOT activate when missing")
	}
}

func TestWorldbook_VarGate_NoTextRequired(t *testing.T) {
	// var: gate should NOT depend on conversation text at all
	entry := wbVar("e1", "secret info", "stage=reveal")
	// No conversation text, but variable matches
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"stage": "reveal"})
	if blocksLen(ctx) != 1 {
		t.Fatal("var gate should not require text match")
	}
}

func TestWorldbook_VarGate_TextMatchDoesNotActivate(t *testing.T) {
	// Even if text contains "confrontation", var gate requires the VARIABLE to match
	entry := wbVar("e1", "secret", "stage=confrontation")
	ctx := runWB(
		[]prompt_ir.WorldbookEntry{entry},
		map[string]any{"stage": "investigation"}, // variable is WRONG
		"we are now in confrontation!", // text mentions the word, irrelevant
	)
	if blocksLen(ctx) != 0 {
		t.Fatal("var gate ignores text; wrong variable should block activation")
	}
}

func TestWorldbook_VarGate_NumericValue(t *testing.T) {
	// Variables are stored as any; fmt.Sprintf("%v", ...) converts numeric to string
	entry := wbVar("e1", "info", "chapter=3")
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"chapter": 3})
	if blocksLen(ctx) != 1 {
		t.Fatalf("numeric variable %v should match string '3'", 3)
	}
}

func TestWorldbook_VarGate_MixedWithRegularKey(t *testing.T) {
	// Entry with var: as primary key — if variable gate blocks, entry never activates
	// even if secondary keys would pass
	entry := prompt_ir.WorldbookEntry{
		ID:             "e1",
		Keys:           []string{"var:gate=open"},
		SecondaryKeys:  []string{"trigger"},
		SecondaryLogic: "and_any",
		Content:        "gated content",
		Enabled:        true,
	}
	// gate is closed: even with trigger word in text, entry should not activate
	ctx := runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"gate": "closed"}, "trigger present")
	if blocksLen(ctx) != 0 {
		t.Fatal("closed var gate should block even when secondary key matches")
	}
	// gate is open: entry activates
	ctx = runWB([]prompt_ir.WorldbookEntry{entry}, map[string]any{"gate": "open"}, "trigger present")
	if blocksLen(ctx) != 1 {
		t.Fatal("open var gate + secondary match should activate")
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 9. PIPELINE RUNNER
// ═══════════════════════════════════════════════════════════════════════════

func TestRunner_SystemPromptPresent(t *testing.T) {
	ctx := &prompt_ir.ContextData{
		Mode:           prompt_ir.ModeNative,
		Config:         prompt_ir.GameConfig{SystemPromptTemplate: "你是叙事者。"},
		RecentMessages: []prompt_ir.Message{{Role: "user", Content: "你好"}},
		TokenBudget:    4000,
	}
	msgs, err := pipeline.NewRunner().Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, m := range msgs {
		if m["role"] == "system" && strings.Contains(m["content"], "叙事者") {
			found = true
		}
	}
	if !found {
		t.Fatalf("system prompt missing from pipeline output: %v", msgs)
	}
}

func TestRunner_WorldbookInSystemMessages(t *testing.T) {
	ctx := &prompt_ir.ContextData{
		Mode: prompt_ir.ModeNative,
		Config: prompt_ir.GameConfig{
			WorldbookEntries: []prompt_ir.WorldbookEntry{wb("e1", "夜歌是歌手", []string{"夜歌"})},
		},
		RecentMessages: []prompt_ir.Message{{Role: "user", Content: "夜歌在哪里"}},
		TokenBudget:    4000,
	}
	msgs, err := pipeline.NewRunner().Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sysContent string
	for _, m := range msgs {
		if m["role"] == "system" {
			sysContent += m["content"]
		}
	}
	if !strings.Contains(sysContent, "夜歌是歌手") {
		t.Fatalf("worldbook not in system messages: %q", sysContent)
	}
}

func TestRunner_UserMessageLast(t *testing.T) {
	ctx := &prompt_ir.ContextData{
		Mode:           prompt_ir.ModeNative,
		Config:         prompt_ir.GameConfig{SystemPromptTemplate: "sys"},
		RecentMessages: []prompt_ir.Message{{Role: "user", Content: "最后一句话"}},
		TokenBudget:    4000,
	}
	msgs, err := pipeline.NewRunner().Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := msgs[len(msgs)-1]
	if last["role"] != "user" || last["content"] != "最后一句话" {
		t.Fatalf("last message should be user input, got: %v", last)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 10. LLM CLIENT — DiscoverModels / TestConnection (3-B) (unit, no network)
// ═══════════════════════════════════════════════════════════════════════════

func TestLLM_DiscoverModels_InvalidURL_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2)
	_ = cancel
	defer cancel()
	_, err := llm.DiscoverModels(ctx, "http://127.0.0.1:19999", "fake-key")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestLLM_TestConnection_InvalidURL_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2)
	_ = cancel
	defer cancel()
	_, err := llm.TestConnection(ctx, "http://127.0.0.1:19999", "fake-key", "gpt-3.5-turbo")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestLLM_NewClient_BaseURL_Trim(t *testing.T) {
	c := llm.NewClient("https://api.example.com/v1/", "key", "model", 30, 0)
	if c.BaseURL() != "https://api.example.com/v1" {
		t.Fatalf("trailing slash not trimmed: %q", c.BaseURL())
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 11. TOKENIZER — extra formula coverage
// ═══════════════════════════════════════════════════════════════════════════

func TestTokenizer_PureCJK_Count(t *testing.T) {
	// 6 CJK chars → 6*2/3 = 4 tokens
	if got := tokenizer.Estimate("你好世界游戏"); got != 4 {
		t.Errorf("Estimate(6 CJK) = %d, want 4", got)
	}
}

func TestTokenizer_SingleCJK_Clamped(t *testing.T) {
	// 1 CJK char: other=1, raw tokens = 0 → clamped to 1
	if got := tokenizer.Estimate("中"); got != 1 {
		t.Errorf("Estimate(\"中\") = %d, want 1", got)
	}
}

func TestTokenizer_Messages_Empty(t *testing.T) {
	if got := tokenizer.EstimateMessages(nil); got != 0 {
		t.Errorf("EstimateMessages(nil) = %d, want 0", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// 12. FORUM — HotScore + RenderContent
// ═══════════════════════════════════════════════════════════════════════════

func TestForum_HotScore_Zero(t *testing.T) {
	score := forum.HotScore(0, 0, time.Now().Add(-time.Second))
	if score != 0 {
		t.Errorf("HotScore(0,0,new) = %f, want 0", score)
	}
}

func TestForum_HotScore_MoreRepliesWins(t *testing.T) {
	base := time.Now().Add(-1 * time.Hour)
	if forum.HotScore(10, 5, base) <= forum.HotScore(2, 1, base) {
		t.Error("higher activity post should have higher hot score")
	}
}

func TestForum_HotScore_OlderDecays(t *testing.T) {
	newPost := forum.HotScore(5, 5, time.Now().Add(-1*time.Hour))
	oldPost := forum.HotScore(5, 5, time.Now().Add(-72*time.Hour))
	if newPost <= oldPost {
		t.Error("newer post should have higher hot score than older post with same activity")
	}
}

func TestForum_HotScore_Formula(t *testing.T) {
	// score = (replies*2 + votes) / pow(ageHours+2, 1.5)
	// ageHours ≈ 2 → score = (3*2+2) / pow(4,1.5) = 8/8 = 1.0
	got := forum.HotScore(3, 2, time.Now().Add(-2*time.Hour))
	want := 8.0 / math.Pow(4, 1.5)
	if math.Abs(got-want) > 0.1 {
		t.Errorf("HotScore = %f, want ≈ %f", got, want)
	}
}

func TestForum_RenderContent_Bold(t *testing.T) {
	svc := forum.New(nil, nil)
	html, err := svc.RenderContent("**bold**")
	if err != nil {
		t.Fatalf("RenderContent: %v", err)
	}
	if !strings.Contains(html, "bold") {
		t.Errorf("rendered HTML missing 'bold': %q", html)
	}
}

func TestForum_RenderContent_XSS(t *testing.T) {
	svc := forum.New(nil, nil)
	html, err := svc.RenderContent("<script>alert('xss')</script> hello")
	if err != nil {
		t.Fatalf("RenderContent: %v", err)
	}
	if strings.Contains(html, "<script>") {
		t.Errorf("XSS not sanitized: %q", html)
	}
}
