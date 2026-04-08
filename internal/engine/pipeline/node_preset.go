package pipeline

import (
	"time"

	"mvu-backend/internal/engine/macros"
	"mvu-backend/internal/engine/prompt_ir"
)

// PresetNode 处理条目化 Prompt 组装（复刻 TH 的 preset-entries）。
//
// 设计：
//   - 每条 PresetEntry 映射为一个 PromptBlock，Priority = InjectionOrder
//   - role=system 条目会被 Runner 合并进统一的系统消息（顺序由 Priority 决定）
//   - role=user / role=assistant 条目作为独立消息插入到历史对话中
//   - 当 GameConfig.PresetEntries 为空时，此节点为无操作（TemplateNode 作为兜底）
//
// # InjectionOrder 与现有节点的优先级区间
//
//	0–9    : 最顶部，高于世界书（WorldbookNode: 10+）
//	10–509 : 与世界书并列
//	510–989: 世界书之后、人设之前
//	990–1009: 主角色人设槽（与 TemplateNode 的 Priority=1000 同级）
//	1010+  : 底部附加指令
type PresetNode struct{}

func NewPresetNode() *PresetNode {
	return &PresetNode{}
}

func (n *PresetNode) Name() string {
	return "PresetNode"
}

func (n *PresetNode) Process(ctx *prompt_ir.ContextData) error {
	if len(ctx.Config.PresetEntries) == 0 {
		return nil
	}

	macroCtx := macros.MacroContext{
		CharName:    ctx.CharName,
		UserName:    ctx.UserName,
		PersonaName: ctx.PersonaName,
		Variables:   ctx.Variables,
		Now:         time.Now(),
	}

	for _, entry := range ctx.Config.PresetEntries {
		if !entry.Enabled {
			continue
		}

		content := macros.Expand(entry.Content, macroCtx)
		if content == "" {
			continue
		}

		role := entry.Role
		if role == "" {
			role = "system"
		}

		ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
			Type:     prompt_ir.BlockPreset,
			Role:     role,
			Content:  content,
			Priority: entry.InjectionOrder,
		})
	}

	return nil
}

