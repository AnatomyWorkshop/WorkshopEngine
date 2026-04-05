package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"mvu-backend/internal/core/llm"
)

// ReplaySafety 工具重放安全等级（对齐 TH replay-safety.ts）。
//
//	Safe            — 幂等操作，可自动重放（读取类工具）
//	ConfirmOnReplay — 重放前需用户确认（轻写操作）
//	NeverAutoReplay — 禁止自动重放（不可逆写操作）
//	Uncertain       — 不确定（外部副作用，如网络调用）
type ReplaySafety string

const (
	ReplaySafe            ReplaySafety = "safe"
	ReplayConfirmOnReplay ReplaySafety = "confirm_on_replay"
	ReplayNeverAutoReplay ReplaySafety = "never_auto_replay"
	ReplayUncertain       ReplaySafety = "uncertain"
)

// Tool 游戏工具接口。每个工具绑定所需依赖（沙箱、记忆库等），Execute 不需要传入 session 上下文。
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage // JSON Schema object，定义参数结构
	ReplaySafety() ReplaySafety  // 重放安全等级
	Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// Registry 工具注册表。按名称索引，供 Agentic Loop 查找和执行。
type Registry struct {
	tools map[string]Tool
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册一个工具（覆盖同名已有工具）。
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Len 返回已注册工具数量。
func (r *Registry) Len() int { return len(r.tools) }

// Execute 执行指定工具，返回结果字符串。工具不存在或执行出错均以字符串形式返回描述，不上抛 error。
func (r *Registry) Execute(ctx context.Context, name string, params json.RawMessage) string {
	t, ok := r.tools[name]
	if !ok {
		return fmt.Sprintf(`{"error":"unknown tool %q"}`, name)
	}
	result, err := t.Execute(ctx, params)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return result
}

// ToLLMDefinitions 将注册表内全部工具转换为 OpenAI function calling 格式。
func (r *Registry) ToLLMDefinitions() []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, llm.ToolDefinition{
			Type: "function",
			Function: llm.ToolFunctionDef{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

// ReplaySafetyOf 返回指定工具的重放安全等级（工具不存在时返回 Uncertain）。
func (r *Registry) ReplaySafetyOf(name string) ReplaySafety {
	if t, ok := r.tools[name]; ok {
		return t.ReplaySafety()
	}
	return ReplayUncertain
}

// ToolRecord 工具执行记录（写入 DB 的最小字段集）。
type ToolRecord struct {
	SessionID string
	FloorID   string
	PageID    string
}

// ExecuteAndRecord 执行工具并异步持久化执行记录。
// db 为 nil 时退化为普通 Execute（不记录）。
func (r *Registry) ExecuteAndRecord(ctx context.Context, name string, params json.RawMessage, rec ToolRecord, db *gorm.DB) string {
	start := time.Now()
	result := r.Execute(ctx, name, params)
	if db == nil {
		return result
	}
	durationMs := time.Since(start).Milliseconds()
	go func() {
		db.Exec(
			`INSERT INTO tool_execution_records (session_id, floor_id, page_id, tool_name, params, result, duration_ms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			rec.SessionID, rec.FloorID, rec.PageID, name, string(params), result, durationMs, time.Now(),
		)
	}()
	return result
}
