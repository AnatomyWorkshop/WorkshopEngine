package parser

import (
	"strings"
	"testing"
)

// ── XML Narrative format ──────────────────────────────────────────────────────

func TestParse_XMLNarrative_Basic(t *testing.T) {
	raw := `<Narrative>夜歌沉默地看着你，眼神复杂。</Narrative>
<Options>
<option>继续等待</option>
<option>转身离开</option>
</Options>`
	r := Parse(raw)
	if r.ParseMode != "xml_narrative" {
		t.Fatalf("expected xml_narrative, got %q", r.ParseMode)
	}
	if r.Narrative != "夜歌沉默地看着你，眼神复杂。" {
		t.Fatalf("unexpected narrative: %q", r.Narrative)
	}
	if len(r.Options) != 2 {
		t.Fatalf("expected 2 options, got %d: %v", len(r.Options), r.Options)
	}
	if r.Options[0] != "继续等待" || r.Options[1] != "转身离开" {
		t.Fatalf("unexpected options: %v", r.Options)
	}
}

func TestParse_XMLNarrative_WithSummaryAndStatePatch(t *testing.T) {
	raw := `<Narrative>故事继续。</Narrative>
<Summary>玩家与夜歌关系趋于紧张。</Summary>
<UpdateState>{"tension": 75}</UpdateState>`
	r := Parse(raw)
	if r.ParseMode != "xml_narrative" {
		t.Fatalf("expected xml_narrative, got %q", r.ParseMode)
	}
	if r.Summary != "玩家与夜歌关系趋于紧张。" {
		t.Fatalf("unexpected summary: %q", r.Summary)
	}
	if r.StatePatch["tension"] == nil {
		t.Fatal("expected tension in state_patch")
	}
}

func TestParse_XMLNarrative_ChineseTag(t *testing.T) {
	raw := `<叙事>这是一个中文叙事标签。</叙事>`
	r := Parse(raw)
	if r.ParseMode != "xml_narrative" {
		t.Fatalf("expected xml_narrative, got %q", r.ParseMode)
	}
	if !strings.Contains(r.Narrative, "中文叙事标签") {
		t.Fatalf("unexpected narrative: %q", r.Narrative)
	}
}

// ── Numbered list fallback ────────────────────────────────────────────────────

func TestParse_NumberedList(t *testing.T) {
	raw := `玩家面临选择：

1. 说出真相
2. 继续隐瞒
3. 转移话题`
	r := Parse(raw)
	if r.ParseMode != "numbered_list" {
		t.Fatalf("expected numbered_list, got %q", r.ParseMode)
	}
	if len(r.Options) != 3 {
		t.Fatalf("expected 3 options, got %d: %v", len(r.Options), r.Options)
	}
	if !strings.Contains(r.Narrative, "玩家面临选择") {
		t.Fatalf("narrative should contain preamble, got %q", r.Narrative)
	}
}

func TestParse_NumberedList_CircledNumbers(t *testing.T) {
	raw := `叙事文本
①.选项一
②.选项二
③.选项三`
	r := Parse(raw)
	if r.ParseMode != "numbered_list" {
		t.Fatalf("expected numbered_list, got %q", r.ParseMode)
	}
	if len(r.Options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(r.Options))
	}
}

// ── Fallback ─────────────────────────────────────────────────────────────────

func TestParse_Fallback_PlainText(t *testing.T) {
	raw := "这段文字没有任何结构标记，直接作为叙事内容。"
	r := Parse(raw)
	if r.ParseMode != "fallback" {
		t.Fatalf("expected fallback, got %q", r.ParseMode)
	}
	if r.Narrative != raw {
		t.Fatalf("expected narrative = raw text")
	}
	if len(r.Options) != 0 {
		t.Fatalf("expected no options in fallback")
	}
}

func TestParse_Fallback_SingleNumberedItem(t *testing.T) {
	// Only 1 numbered item → not enough for numbered_list (needs ≥2)
	raw := `一段叙事
1. 只有一个选项`
	r := Parse(raw)
	if r.ParseMode == "numbered_list" {
		t.Fatal("single numbered item should not trigger numbered_list")
	}
}

// ── StatePatch ────────────────────────────────────────────────────────────────

func TestParse_StatePatch_Nested(t *testing.T) {
	raw := `<Narrative>测试</Narrative>
<UpdateState>{"emotion": {"tension": 80, "trust": 30}}</UpdateState>`
	r := Parse(raw)
	if r.StatePatch == nil {
		t.Fatal("expected state_patch")
	}
	emotion, ok := r.StatePatch["emotion"].(map[string]any)
	if !ok {
		t.Fatalf("expected emotion to be a map, got %T", r.StatePatch["emotion"])
	}
	if emotion["tension"] != float64(80) {
		t.Fatalf("expected tension 80, got %v", emotion["tension"])
	}
}

func TestParse_StatePatch_Invalid_NoError(t *testing.T) {
	// Invalid JSON in UpdateState should not panic, just empty patch
	raw := `<Narrative>叙事</Narrative>
<UpdateState>not json</UpdateState>`
	r := Parse(raw)
	if r.Narrative != "叙事" {
		t.Fatalf("narrative should still parse, got %q", r.Narrative)
	}
}

// ── game_response VN format ───────────────────────────────────────────────────

func TestParse_GameResponse_VN(t *testing.T) {
	raw := `<game_response>
[bg|city_night.jpg]
[bgm|rain_ambient]
旁白||夜色深沉，城市的灯火在雨中晕开。
夜歌|nightsong_sad.png|……你终于来了。
[choice|回应她|保持沉默|直接离开]
</game_response>`
	r := Parse(raw)
	if r.ParseMode != "xml_game_response" {
		t.Fatalf("expected xml_game_response, got %q", r.ParseMode)
	}
	if r.VN == nil {
		t.Fatal("expected VN directives")
	}
	if r.VN.BG != "city_night.jpg" {
		t.Fatalf("unexpected BG: %q", r.VN.BG)
	}
	if r.VN.BGM != "rain_ambient" {
		t.Fatalf("unexpected BGM: %q", r.VN.BGM)
	}
	if len(r.VN.Lines) != 2 {
		t.Fatalf("expected 2 dialogue lines, got %d", len(r.VN.Lines))
	}
	if len(r.Options) != 3 {
		t.Fatalf("expected 3 choices, got %d: %v", len(r.Options), r.Options)
	}
	// 旁白 should accumulate to Narrative
	if !strings.Contains(r.Narrative, "夜色深沉") {
		t.Fatalf("expected narration in Narrative, got %q", r.Narrative)
	}
}
