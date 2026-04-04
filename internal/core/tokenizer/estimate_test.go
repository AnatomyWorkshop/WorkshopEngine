package tokenizer

import (
	"testing"
)

func TestEstimate_Empty(t *testing.T) {
	if Estimate("") != 0 {
		t.Fatal("empty string should return 0")
	}
}

func TestEstimate_ASCIIOnly(t *testing.T) {
	// "hello world" = 11 ASCII chars → ~2-3 tokens
	got := Estimate("hello world")
	if got < 1 || got > 5 {
		t.Fatalf("unexpected token estimate for ASCII: %d", got)
	}
}

func TestEstimate_CJKOnly(t *testing.T) {
	// "夜歌" = 2 CJK chars → ~1-2 tokens
	got := Estimate("夜歌")
	if got < 1 || got > 4 {
		t.Fatalf("unexpected token estimate for CJK: %d", got)
	}
}

func TestEstimate_MinOne(t *testing.T) {
	// A single CJK char should return at least 1
	got := Estimate("夜")
	if got < 1 {
		t.Fatalf("expected at least 1, got %d", got)
	}
}

func TestEstimate_Mixed(t *testing.T) {
	// Mixed text should return a reasonable positive value
	text := "Hello 夜歌, how are you? 今天天气很好。"
	got := Estimate(text)
	if got <= 0 {
		t.Fatalf("expected positive estimate, got %d", got)
	}
}

func TestEstimate_LongText(t *testing.T) {
	// Ensure longer text produces proportionally larger estimates
	short := Estimate("hello world")
	long := Estimate("hello world hello world hello world hello world hello world")
	if long <= short {
		t.Fatalf("longer text should produce larger estimate: short=%d long=%d", short, long)
	}
}

func TestEstimateMessages_Empty(t *testing.T) {
	got := EstimateMessages(nil)
	if got != 0 {
		t.Fatalf("expected 0 for nil messages, got %d", got)
	}
}

func TestEstimateMessages_AddsStructureOverhead(t *testing.T) {
	msgs := []map[string]string{
		{"role": "user", "content": "hi"},
	}
	// content estimate + 4 overhead
	got := EstimateMessages(msgs)
	contentOnly := Estimate("hi")
	if got != contentOnly+4 {
		t.Fatalf("expected %d (content=%d + overhead=4), got %d", contentOnly+4, contentOnly, got)
	}
}

func TestEstimateMessages_Multiple(t *testing.T) {
	msgs := []map[string]string{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "world"},
	}
	got := EstimateMessages(msgs)
	if got <= 0 {
		t.Fatalf("expected positive estimate, got %d", got)
	}
}
