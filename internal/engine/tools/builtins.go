package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mvu-backend/internal/engine/memory"
	"mvu-backend/internal/engine/variable"
)

// ── get_variable ──────────────────────────────────────────────────────────────

// GetVariableTool 读取沙箱变量（级联：Page→Floor→Branch→Chat→Global）
type GetVariableTool struct{ sb *variable.Sandbox }

func NewGetVariableTool(sb *variable.Sandbox) *GetVariableTool {
	return &GetVariableTool{sb: sb}
}

func (t *GetVariableTool) Name() string         { return "get_variable" }
func (t *GetVariableTool) Description() string  { return "读取游戏变量的当前值" }
func (t *GetVariableTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *GetVariableTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string", "description": "变量名"}
		},
		"required": ["name"]
	}`)
}

func (t *GetVariableTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	val, ok := t.sb.Get(p.Name)
	if !ok {
		return fmt.Sprintf(`{"found":false,"name":%q}`, p.Name), nil
	}
	b, _ := json.Marshal(val)
	return fmt.Sprintf(`{"found":true,"name":%q,"value":%s}`, p.Name, string(b)), nil
}

// ── set_variable ──────────────────────────────────────────────────────────────

// SetVariableTool 写入沙箱变量（写入 Page 层，CommitTurn 后提升至 Chat 层）
type SetVariableTool struct{ sb *variable.Sandbox }

func NewSetVariableTool(sb *variable.Sandbox) *SetVariableTool {
	return &SetVariableTool{sb: sb}
}

func (t *SetVariableTool) Name() string         { return "set_variable" }
func (t *SetVariableTool) Description() string  { return "设置游戏变量的值（写入 Page 沙箱，回合提交后固化）" }
func (t *SetVariableTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *SetVariableTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":  {"type": "string", "description": "变量名"},
			"value": {"description": "变量新值（任意 JSON 类型）"}
		},
		"required": ["name", "value"]
	}`)
}

func (t *SetVariableTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Name  string          `json:"name"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	var val any
	_ = json.Unmarshal(p.Value, &val)
	t.sb.Set(p.Name, val)
	return fmt.Sprintf(`{"ok":true,"name":%q}`, p.Name), nil
}

// ── search_memory ─────────────────────────────────────────────────────────────

// SearchMemoryTool 在会话记忆中按关键词全文搜索（最多返回 5 条）
type SearchMemoryTool struct {
	sessionID string
	memStore  *memory.Store
}

func NewSearchMemoryTool(sessionID string, memStore *memory.Store) *SearchMemoryTool {
	return &SearchMemoryTool{sessionID: sessionID, memStore: memStore}
}

func (t *SearchMemoryTool) Name() string         { return "search_memory" }
func (t *SearchMemoryTool) Description() string  { return "在会话记忆中搜索包含关键词的条目（最多返回 5 条）" }
func (t *SearchMemoryTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *SearchMemoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "关键词或短语"}
		},
		"required": ["query"]
	}`)
}

func (t *SearchMemoryTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}

	mems, err := t.memStore.ListMemories(t.sessionID)
	if err != nil {
		return "", fmt.Errorf("list memories: %w", err)
	}

	query := strings.ToLower(p.Query)
	var hits []string
	for _, m := range mems {
		if strings.Contains(strings.ToLower(m.Content), query) {
			hits = append(hits, m.Content)
			if len(hits) >= 5 {
				break
			}
		}
	}

	if len(hits) == 0 {
		return `{"found":false,"results":[]}`, nil
	}
	b, _ := json.Marshal(hits)
	return fmt.Sprintf(`{"found":true,"results":%s}`, string(b)), nil
}
