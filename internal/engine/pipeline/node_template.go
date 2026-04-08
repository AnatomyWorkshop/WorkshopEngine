package pipeline

import (
	"encoding/json"
	"strings"
	"time"

	"mvu-backend/internal/engine/macros"
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

	// 2. 简易变量宏替换（{{variable_key}} 形式，来自 sandbox）
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

	// 3. ST 宏展开（{{char}} / {{user}} / {{persona}} / {{getvar::key}} / {{time}} / {{date}}）
	resolvedPrompt = macros.Expand(resolvedPrompt, macros.MacroContext{
		CharName:    ctx.CharName,
		UserName:    ctx.UserName,
		PersonaName: ctx.PersonaName,
		Variables:   ctx.Variables,
		Now:         time.Now(),
	})

	// 4. 将解析后的内容作为最重要的系统级 Prompt 压入上下文，优先级设为极高 (0)
	ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
		Type:     prompt_ir.BlockSystem,
		Role:     "system",
		Content:  resolvedPrompt,
		Priority: 0,
	})

	return nil
}

