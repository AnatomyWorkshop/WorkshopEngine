package scheduled

import (
	"testing"
)

// ── GetFloat ─────────────────────────────────────────────────────────────────

func TestGetFloat_TopLevel(t *testing.T) {
	vars := map[string]any{"tension": float64(80)}
	v, ok := GetFloat(vars, "tension")
	if !ok || v != 80 {
		t.Fatalf("want 80 got %v ok=%v", v, ok)
	}
}

func TestGetFloat_Nested(t *testing.T) {
	vars := map[string]any{
		"emotion": map[string]any{"tension": float64(55)},
	}
	v, ok := GetFloat(vars, "emotion.tension")
	if !ok || v != 55 {
		t.Fatalf("want 55 got %v", v)
	}
}

func TestGetFloat_DeepNested(t *testing.T) {
	vars := map[string]any{
		"npc": map[string]any{
			"夜歌": map[string]any{"trust": float64(30)},
		},
	}
	v, ok := GetFloat(vars, "npc.夜歌.trust")
	if !ok || v != 30 {
		t.Fatalf("want 30 got %v", v)
	}
}

func TestGetFloat_Missing(t *testing.T) {
	vars := map[string]any{}
	_, ok := GetFloat(vars, "missing")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestGetFloat_MissingNested(t *testing.T) {
	vars := map[string]any{"emotion": map[string]any{}}
	_, ok := GetFloat(vars, "emotion.tension")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestGetFloat_WrongType(t *testing.T) {
	vars := map[string]any{"tension": "high"} // string, not number
	_, ok := GetFloat(vars, "tension")
	if ok {
		t.Fatal("expected not found for string value")
	}
}

// ── CooldownKey ───────────────────────────────────────────────────────────────

func TestCooldownKey(t *testing.T) {
	got := CooldownKey("hostile_npc")
	want := "__sched.hostile_npc.last_floor"
	if got != want {
		t.Fatalf("want %q got %q", want, got)
	}
}

// ── TriggerRule.PickInput ────────────────────────────────────────────────────

func TestPickInput_UserInput(t *testing.T) {
	r := TriggerRule{UserInput: "[SYSTEM: 触发]"}
	got := r.PickInput()
	if got != "[SYSTEM: 触发]" {
		t.Fatalf("unexpected %q", got)
	}
}

func TestPickInput_EventPool_AlwaysFromPool(t *testing.T) {
	pool := []string{"事件A", "事件B", "事件C"}
	r := TriggerRule{UserInput: "not this", EventPool: pool}

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		got := r.PickInput()
		seen[got] = true
		found := false
		for _, p := range pool {
			if got == p {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("PickInput returned %q which is not in event pool", got)
		}
	}
	// With 100 draws from 3 items, expect all 3 to appear (probabilistically)
	if len(seen) < 2 {
		t.Logf("warning: only %d distinct values seen in 100 draws", len(seen))
	}
}

// ── Evaluate ─────────────────────────────────────────────────────────────────

func makeVars(tension float64) map[string]any {
	return map[string]any{"emotion": map[string]any{"tension": tension}}
}

func TestEvaluate_Triggers(t *testing.T) {
	rules := []TriggerRule{
		{
			ID:             "hostile",
			ConditionVar:   "emotion.tension",
			Threshold:      70,
			Probability:    1.0,
			CooldownFloors: 0,
			UserInput:      "[NPC: hostile]",
		},
	}
	r := Evaluate(rules, makeVars(80), 10, 0.5)
	if r == nil {
		t.Fatal("expected rule to trigger")
	}
	if r.ID != "hostile" {
		t.Fatalf("unexpected rule id %q", r.ID)
	}
}

func TestEvaluate_BelowThreshold(t *testing.T) {
	rules := []TriggerRule{
		{ID: "r1", ConditionVar: "emotion.tension", Threshold: 70, Probability: 1.0},
	}
	r := Evaluate(rules, makeVars(60), 10, 0.5)
	if r != nil {
		t.Fatal("expected no trigger below threshold")
	}
}

func TestEvaluate_AtThreshold(t *testing.T) {
	rules := []TriggerRule{
		{ID: "r1", ConditionVar: "emotion.tension", Threshold: 70, Probability: 1.0},
	}
	r := Evaluate(rules, makeVars(70), 10, 0.5)
	if r == nil {
		t.Fatal("expected trigger at exact threshold (>=)")
	}
}

func TestEvaluate_ProbabilityBlocks(t *testing.T) {
	rules := []TriggerRule{
		{ID: "r1", ConditionVar: "emotion.tension", Threshold: 70, Probability: 0.5},
	}
	// rng=0.5 is NOT < 0.5, so should NOT trigger
	r := Evaluate(rules, makeVars(80), 10, 0.5)
	if r != nil {
		t.Fatal("expected no trigger when rng >= probability")
	}
	// rng=0.49 IS < 0.5, should trigger
	r = Evaluate(rules, makeVars(80), 10, 0.49)
	if r == nil {
		t.Fatal("expected trigger when rng < probability")
	}
}

func TestEvaluate_ZeroProbabilityMeansAlways(t *testing.T) {
	rules := []TriggerRule{
		{ID: "r1", ConditionVar: "emotion.tension", Threshold: 70, Probability: 0},
	}
	r := Evaluate(rules, makeVars(80), 10, 0.99)
	if r == nil {
		t.Fatal("probability=0 should mean always trigger (treated as 1.0)")
	}
}

func TestEvaluate_CooldownBlocks(t *testing.T) {
	vars := makeVars(80)
	vars[CooldownKey("r1")] = float64(8) // last triggered at floor 8
	rules := []TriggerRule{
		{ID: "r1", ConditionVar: "emotion.tension", Threshold: 70, Probability: 1.0, CooldownFloors: 5},
	}
	// currentFloor=10, last=8, diff=2 < cooldown=5 → blocked
	r := Evaluate(rules, vars, 10, 0.1)
	if r != nil {
		t.Fatal("expected cooldown to block trigger")
	}
	// currentFloor=14, last=8, diff=6 >= cooldown=5 → passes
	r = Evaluate(rules, vars, 14, 0.1)
	if r == nil {
		t.Fatal("expected trigger after cooldown expires")
	}
}

func TestEvaluate_UnknownModeSkipped(t *testing.T) {
	rules := []TriggerRule{
		{ID: "r1", Mode: "time_based", ConditionVar: "emotion.tension", Threshold: 10, Probability: 1.0},
	}
	r := Evaluate(rules, makeVars(80), 10, 0.1)
	if r != nil {
		t.Fatal("unknown mode should be skipped")
	}
}

func TestEvaluate_FirstMatchWins(t *testing.T) {
	rules := []TriggerRule{
		{ID: "first", ConditionVar: "emotion.tension", Threshold: 50, Probability: 1.0},
		{ID: "second", ConditionVar: "emotion.tension", Threshold: 50, Probability: 1.0},
	}
	r := Evaluate(rules, makeVars(80), 10, 0.1)
	if r == nil || r.ID != "first" {
		t.Fatalf("expected first rule to win, got %v", r)
	}
}

func TestEvaluate_MissingVar(t *testing.T) {
	rules := []TriggerRule{
		{ID: "r1", ConditionVar: "nonexistent", Threshold: 10, Probability: 1.0},
	}
	r := Evaluate(rules, map[string]any{}, 10, 0.1)
	if r != nil {
		t.Fatal("missing variable should not trigger")
	}
}
