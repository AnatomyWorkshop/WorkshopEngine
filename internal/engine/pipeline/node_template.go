package pipeline

import (
	"encoding/json"
	"strings"

	"mvu-backend/internal/engine/prompt_ir"
)

// TemplateNode 处理基础系统提示词，并执行变量宏替换
type TemplateNode struct{}

func NewTemplateNode() *TemplateNode {
	return &TemplateNode{}
}

func (n *TemplateNode) Name() string {
	return "TemplateNode"
}

func (n *TemplateNode) Process(ctx *prompt_ir.ContextData) error {
	if ctx.Config.SystemPromptTemplate == "" {
		return nil
	}

	// 1. 获取全局变量快照
	flattenedVars := ctx.Variables

	// 2. 简易宏替换引擎
	// 比如: "你的名字是 {{char}}，当前血量 {{hp}}。"
	resolvedPrompt := ctx.Config.SystemPromptTemplate
	for k, v := range flattenedVars {
		strVal := ""
		switch val := v.(type) {
		case string:
			strVal = val
		case float64, int, bool:
			b, _ := json.Marshal(val)
			strVal = string(b)
		}
		macro := "{{" + k + "}}"
		resolvedPrompt = strings.ReplaceAll(resolvedPrompt, macro, strVal)
	}

	// 3. 将解析后的内容作为最重要的系统级 Prompt 压入上下文，优先级设为极高 (0)
	ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
		Type:     prompt_ir.BlockSystem,
		Role:     "system",
		Content:  resolvedPrompt,
		Priority: 0,
	})

	return nil
}
