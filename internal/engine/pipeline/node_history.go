package pipeline

import (
	"mvu-backend/internal/engine/prompt_ir"
)

// HistoryNode 负责将用户最近对话作为历史块塞进 IR
type HistoryNode struct{}

func NewHistoryNode() *HistoryNode {
	return &HistoryNode{}
}

func (n *HistoryNode) Name() string {
	return "HistoryNode"
}

func (n *HistoryNode) Process(ctx *prompt_ir.ContextData) error {
	// (可选：Token Budget 的修剪逻辑就在这里执行，
	// 如果超出预算，就从最老的历史开始裁剪，保证近期对话。)

	// 这里我们将最近的消息组装并推入 IR。优先级排到最低 (比如 100)，即最靠近当前的用户输入。
	priorityStart := 100

	for i, msg := range ctx.RecentMessages {
		ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
			Type:     prompt_ir.BlockHistory,
			Role:     msg.Role,
			Content:  msg.Content,
			Priority: priorityStart + i, // 保持其自然时序
		})
	}
	return nil
}
