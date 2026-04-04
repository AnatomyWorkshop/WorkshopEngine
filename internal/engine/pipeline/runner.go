package pipeline

import (
	"sort"

	"mvu-backend/internal/engine/prompt_ir"
)

// Runner 流水线执行器
type Runner struct {
	Nodes []prompt_ir.PipelineNode
}

// NewRunner 创建一个默认的执行链
func NewRunner() *Runner {
	return &Runner{
		Nodes: []prompt_ir.PipelineNode{
			NewPresetNode(),   // 条目化 Prompt（优先于 TemplateNode，两者可共存）
			NewTemplateNode(), // SystemPromptTemplate 单字符串兜底
			NewWorldbookNode(),
			NewMemoryNode(),
			NewHistoryNode(),
		},
	}
}

func (r *Runner) AddNode(n prompt_ir.PipelineNode) {
	r.Nodes = append(r.Nodes, n)
}

// Execute 依次运行节点，并返回最终扁平化后的消息切片
func (r *Runner) Execute(ctx *prompt_ir.ContextData) ([]map[string]string, error) {
	// 1. 顺序执行流水线
	for _, node := range r.Nodes {
		if err := node.Process(ctx); err != nil {
			return nil, err
		}
	}

	// 2. 根据 Priority 排序 Blocks
	sort.SliceStable(ctx.Blocks, func(i, j int) bool {
		return ctx.Blocks[i].Priority < ctx.Blocks[j].Priority
	})

	// 3. 组装为标准的 LLM Messages []{role, content}
	var finalMessages []map[string]string

	// 在组装时，我们可以选择把 System / Worldbook / Memory 统一合并到第一条 System 中
	var combinedSystemContent string

	for _, block := range ctx.Blocks {
		if block.Role == "system" {
			if combinedSystemContent != "" {
				combinedSystemContent += "\n\n"
			}
			combinedSystemContent += block.Content
		} else {
			// 如果 System 信息攒着，遇到非 system 时将其先 push 进去
			if combinedSystemContent != "" {
				finalMessages = append(finalMessages, map[string]string{
					"role":    "system",
					"content": combinedSystemContent,
				})
				combinedSystemContent = ""
			}
			finalMessages = append(finalMessages, map[string]string{
				"role":    block.Role,
				"content": block.Content,
			})
		}
	}

	// 如果最后还是只有 System（边界情况），收尾
	if combinedSystemContent != "" {
		finalMessages = append(finalMessages, map[string]string{
			"role":    "system",
			"content": combinedSystemContent,
		})
	}

	return finalMessages, nil
}
