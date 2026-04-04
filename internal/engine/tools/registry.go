package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"mvu-backend/internal/core/llm"
)

// Tool 游戏工具接口。每个工具绑定所需依赖（沙箱、记忆库等），Execute 不需要传入 session 上下文。
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage // JSON Schema object，定义参数结构
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
