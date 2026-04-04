package processor

import (
	"testing"

	"mvu-backend/internal/engine/prompt_ir"
)

func rule(pattern, replacement, applyTo string) prompt_ir.RegexRule {
	return prompt_ir.RegexRule{
		Pattern:     pattern,
		Replacement: replacement,
		ApplyTo:     applyTo,
		Enabled:     true,
	}
}

// ── ApplyToAIOutput ───────────────────────────────────────────────────────────

func TestApplyToAIOutput_BasicReplacement(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`\[系统\]`, "[SYSTEM]", "ai_output")}
	got := ApplyToAIOutput("[系统] 触发了事件", rules)
	if got != "[SYSTEM] 触发了事件" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestApplyToAIOutput_SkipsUserInput(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`hello`, "world", "user_input")}
	got := ApplyToAIOutput("hello there", rules)
	if got != "hello there" {
		t.Fatalf("user_input rule should not apply to ai_output, got %q", got)
	}
}

func TestApplyToAIOutput_AllApplies(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`foo`, "bar", "all")}
	got := ApplyToAIOutput("foo baz", rules)
	if got != "bar baz" {
		t.Fatalf("all rule should apply to ai_output, got %q", got)
	}
}

func TestApplyToAIOutput_DisabledSkipped(t *testing.T) {
	r := rule(`hello`, "world", "ai_output")
	r.Enabled = false
	got := ApplyToAIOutput("hello", []prompt_ir.RegexRule{r})
	if got != "hello" {
		t.Fatalf("disabled rule should not apply, got %q", got)
	}
}

func TestApplyToAIOutput_Multiline(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`(?m)^旁白：`, "", "ai_output")}
	got := ApplyToAIOutput("旁白：夜深了\n旁白：风停了", rules)
	if got != "夜深了\n风停了" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestApplyToAIOutput_CaptureGroup(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`\*([^*]+)\*`, "$1", "ai_output")}
	got := ApplyToAIOutput("她*轻声*说道", rules)
	if got != "她轻声说道" {
		t.Fatalf("capture group replacement failed, got %q", got)
	}
}

func TestApplyToAIOutput_InvalidRegex_NoError(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`[invalid`, "x", "ai_output")}
	got := ApplyToAIOutput("original text", rules)
	if got != "original text" {
		t.Fatalf("invalid regex should be skipped, got %q", got)
	}
}

// ── ApplyToUserInput ──────────────────────────────────────────────────────────

func TestApplyToUserInput_BasicReplacement(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`你好`, "hello", "user_input")}
	got := ApplyToUserInput("你好世界", rules)
	if got != "hello世界" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestApplyToUserInput_SkipsAIOutput(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`foo`, "bar", "ai_output")}
	got := ApplyToUserInput("foo baz", rules)
	if got != "foo baz" {
		t.Fatalf("ai_output rule should not apply to user_input, got %q", got)
	}
}

// ── /pattern/flags syntax ─────────────────────────────────────────────────────

func TestApplyToAIOutput_SlashFlagsInsensitive(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`/HELLO/i`, "HI", "ai_output")}
	got := ApplyToAIOutput("Say hello please", rules)
	if got != "Say HI please" {
		t.Fatalf("case-insensitive flag failed, got %q", got)
	}
}

func TestApplyToAIOutput_SlashPattern_NoFlags(t *testing.T) {
	rules := []prompt_ir.RegexRule{rule(`/world/`, "earth", "ai_output")}
	got := ApplyToAIOutput("hello world", rules)
	if got != "hello earth" {
		t.Fatalf("slash pattern without flags failed, got %q", got)
	}
}

// ── Order / chaining ──────────────────────────────────────────────────────────

func TestApplyToAIOutput_Chained(t *testing.T) {
	rules := []prompt_ir.RegexRule{
		rule(`A`, "B", "ai_output"),
		rule(`B`, "C", "ai_output"),
	}
	got := ApplyToAIOutput("A", rules)
	if got != "C" {
		t.Fatalf("chained rules should apply in order, got %q", got)
	}
}
