package variable

import (
	"testing"
)

// ── Get / Set ─────────────────────────────────────────────────────────────────

func TestSandbox_GetSet_PageScope(t *testing.T) {
	sb := NewSandbox(nil, nil, nil, nil, nil)
	sb.Set("tension", 80)
	v, ok := sb.Get("tension")
	if !ok {
		t.Fatal("expected to find tension")
	}
	if v != 80 {
		t.Fatalf("expected 80 got %v", v)
	}
}

func TestSandbox_ChatOverriddenByPage(t *testing.T) {
	chatVars := map[string]any{"tension": float64(30)}
	sb := NewSandbox(nil, chatVars, nil, nil, nil)
	// Page-level set should shadow chat
	sb.Set("tension", float64(90))
	v, _ := sb.Get("tension")
	if v != float64(90) {
		t.Fatalf("expected page value 90, got %v", v)
	}
}

func TestSandbox_FallsBackToChat(t *testing.T) {
	chatVars := map[string]any{"trust": float64(50)}
	sb := NewSandbox(nil, chatVars, nil, nil, nil)
	v, ok := sb.Get("trust")
	if !ok {
		t.Fatal("expected to find trust in chat scope")
	}
	if v != float64(50) {
		t.Fatalf("expected 50, got %v", v)
	}
}

func TestSandbox_FallsBackToGlobal(t *testing.T) {
	globalVars := map[string]any{"world_seed": "alpha"}
	sb := NewSandbox(globalVars, nil, nil, nil, nil)
	v, ok := sb.Get("world_seed")
	if !ok {
		t.Fatal("expected to find world_seed in global scope")
	}
	if v != "alpha" {
		t.Fatalf("expected alpha, got %v", v)
	}
}

func TestSandbox_Missing(t *testing.T) {
	sb := NewSandbox(nil, nil, nil, nil, nil)
	_, ok := sb.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

// ── Flatten ───────────────────────────────────────────────────────────────────

func TestSandbox_Flatten_MergesAllScopes(t *testing.T) {
	global := map[string]any{"a": "global_a"}
	chat := map[string]any{"b": "chat_b"}
	sb := NewSandbox(global, chat, nil, nil, nil)
	flat := sb.Flatten()
	if flat["a"] != "global_a" {
		t.Fatalf("missing global var: %v", flat)
	}
	if flat["b"] != "chat_b" {
		t.Fatalf("missing chat var: %v", flat)
	}
}

func TestSandbox_Flatten_PageWinsOverChat(t *testing.T) {
	chat := map[string]any{"x": "from_chat"}
	sb := NewSandbox(nil, chat, nil, nil, nil)
	sb.Set("x", "from_page")
	flat := sb.Flatten()
	if flat["x"] != "from_page" {
		t.Fatalf("page scope should win, got %v", flat["x"])
	}
}

func TestSandbox_Flatten_DoesNotMutateInput(t *testing.T) {
	chat := map[string]any{"k": "v1"}
	sb := NewSandbox(nil, chat, nil, nil, nil)
	flat := sb.Flatten()
	flat["k"] = "v2"
	// Original Sandbox should be unaffected
	v, _ := sb.Get("k")
	if v == "v2" {
		t.Fatal("Flatten should return a copy, not a reference")
	}
}

// ── Scope precedence: Page > Floor > Branch > Chat > Global ──────────────────

func TestSandbox_ScopePrecedence(t *testing.T) {
	global := map[string]any{"k": "global"}
	chat := map[string]any{"k": "chat"}
	branch := map[string]any{"k": "branch"}
	floor := map[string]any{"k": "floor"}
	page := map[string]any{"k": "page"}
	sb := NewSandbox(global, chat, branch, floor, page)
	v, _ := sb.Get("k")
	if v != "page" {
		t.Fatalf("page scope should have highest priority, got %v", v)
	}
}
