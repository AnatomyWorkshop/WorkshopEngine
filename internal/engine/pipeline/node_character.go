package pipeline

import (
	"time"

	"mvu-backend/internal/engine/macros"
	"mvu-backend/internal/engine/prompt_ir"
)

// CharacterInjectionNode 将角色卡内容（description / personality / scenario）
// 自动注入为系统提示块。
//
// # 触发条件
//
// 仅当 ctx.CharacterDescription 非空时才注入。
// 为空时静默跳过（无角色卡绑定，或角色卡无有效字段）。
//
// # 注入位置
//
// Priority = 9，位于 TemplateNode（Priority=0）之后、WorldbookNode 第一条（Priority=10+）之前。
// 这保证角色卡描述在系统主提示词之后立即出现，接近 LLM 注意力焦点。
//
// # 宏展开
//
// 内容在注入前经过 macros.Expand()，支持 {{char}} / {{user}} / {{persona}} / {{getvar::key}}。
type CharacterInjectionNode struct{}

func NewCharacterInjectionNode() *CharacterInjectionNode {
	return &CharacterInjectionNode{}
}

func (n *CharacterInjectionNode) Name() string { return "CharacterInjectionNode" }

func (n *CharacterInjectionNode) Process(ctx *prompt_ir.ContextData) error {
	if ctx.CharacterDescription == "" {
		return nil
	}

	content := macros.Expand(ctx.CharacterDescription, macros.MacroContext{
		CharName:    ctx.CharName,
		UserName:    ctx.UserName,
		PersonaName: ctx.PersonaName,
		Variables:   ctx.Variables,
		Now:         time.Now(),
	})

	if content == "" {
		return nil
	}

	ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
		Type:     prompt_ir.BlockSystem,
		Role:     "system",
		Content:  content,
		Priority: 9,
	})
	return nil
}
