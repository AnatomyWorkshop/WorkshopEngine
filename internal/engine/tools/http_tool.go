package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dbmodels "mvu-backend/internal/core/db"
)

// contextKey 用于在 context 中传递 floor_id
type contextKey string

const CtxFloorID contextKey = "floor_id"

// HttpCallTool 将 LLM tool_call 转发到创作者注册的 HTTP 端点。
//
// 请求体：{"session_id":"...","floor_id":"...","args":{...}}
// 期望响应：任意 JSON（直接作为 tool message 内容）
type HttpCallTool struct {
	def       dbmodels.PresetTool
	sessionID string
}

func NewHttpCallTool(def dbmodels.PresetTool, sessionID string) *HttpCallTool {
	return &HttpCallTool{def: def, sessionID: sessionID}
}

func (t *HttpCallTool) Name() string              { return t.def.Name }
func (t *HttpCallTool) Description() string       { return t.def.Description }
func (t *HttpCallTool) ReplaySafety() ReplaySafety { return ReplayUncertain }
func (t *HttpCallTool) Parameters() json.RawMessage {
	if len(t.def.Parameters) == 0 {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return json.RawMessage(t.def.Parameters)
}

func (t *HttpCallTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	// 安全校验：只允许 http/https
	ep := t.def.Endpoint
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		return `{"error":"invalid endpoint scheme"}`, nil
	}

	floorID, _ := ctx.Value(CtxFloorID).(string)

	body, _ := json.Marshal(map[string]any{
		"session_id": t.sessionID,
		"floor_id":   floorID,
		"args":       json.RawMessage(params),
	})

	timeout := time.Duration(t.def.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, ep, bytes.NewReader(body))
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}
	defer resp.Body.Close()

	result, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024)) // 最多 64KB
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}
	return string(result), nil
}
