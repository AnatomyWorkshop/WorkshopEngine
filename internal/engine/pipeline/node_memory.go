package pipeline

import (
	"mvu-backend/internal/engine/prompt_ir"
)

// MemoryNode 负责将后台生成的长期记忆摘要注入到上下文中
type MemoryNode struct{}

func NewMemoryNode() *MemoryNode {
	return &MemoryNode{}
}

func (n *MemoryNode) Name() string {
	return "MemoryNode"
}

// defaultMemoryLabel 内置标签（当 GameConfig.MemoryLabel 为空时使用）。
// 使用语言无关的格式；游戏可通过 GameTemplate.Config.memory_label 覆盖为任意语言。
const defaultMemoryLabel = "[Memory Summary]\n"

func (n *MemoryNode) Process(ctx *prompt_ir.ContextData) error {
	if ctx.Config.MemorySummary == "" {
		return nil
	}

	label := ctx.Config.MemoryLabel
	if label == "" {
		label = defaultMemoryLabel
	}

	// 将长期记忆作为强上下文注入，
	// 优先级通常设定为紧贴在近期历史记录之前（权重设定为如 50），
	// 让 LLM 能"承上启下"。
	ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
		Type:     prompt_ir.BlockMemory,
		Role:     "system",
		Content:  label + ctx.Config.MemorySummary,
		Priority: 50,
	})

	return nil
}
