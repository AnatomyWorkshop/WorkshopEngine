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
			NewPresetNode(),            // 条目化 Prompt（优先于 TemplateNode，两者可共存）
			NewTemplateNode(),          // SystemPromptTemplate 单字符串兜底
			NewCharacterInjectionNode(), // 角色卡注入（Priority=9，紧跟 Template 之后）
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

	// 4. 插入 at_depth 词条
	//
	// depth=0 表示追加到最底部；depth=N 表示在倒数第 N 条消息之前插入。
	// 插入位置：idx = max(1, len(finalMessages) - depth)
	//   - 下限 1：确保不插入到全局系统消息之前
	//   - 超出范围时（depth 大于历史长度）退化到紧跟系统消息之后（idx=1）
	//
	// 同深度词条按 Priority 升序排列（数值越小越靠近系统消息方向）。
	// 从最大 idx 开始插入，避免前序插入造成下标位移。
	if len(ctx.AtDepthBlocks) > 0 {
		n := len(finalMessages)

		// 计算每条词条的插入位置（基于原始消息数 n）
		type insertion struct {
			idx      int
			priority int
			content  string
		}
		inserts := make([]insertion, 0, len(ctx.AtDepthBlocks))
		for _, b := range ctx.AtDepthBlocks {
			idx := n - b.Depth
			if idx < 1 {
				idx = 1
			}
			if idx > n {
				idx = n
			}
			inserts = append(inserts, insertion{idx: idx, priority: b.Priority, content: b.Content})
		}

		// 按 idx 降序，同 idx 内按 Priority 降序（后插入的排在前面，最终结果是升序）
		sort.SliceStable(inserts, func(i, j int) bool {
			if inserts[i].idx != inserts[j].idx {
				return inserts[i].idx > inserts[j].idx
			}
			return inserts[i].priority > inserts[j].priority
		})

		for _, ins := range inserts {
			msg := map[string]string{"role": "system", "content": ins.content}
			// 在 ins.idx 位置插入
			finalMessages = append(finalMessages, nil)
			copy(finalMessages[ins.idx+1:], finalMessages[ins.idx:])
			finalMessages[ins.idx] = msg
		}
	}

	return finalMessages, nil
}
