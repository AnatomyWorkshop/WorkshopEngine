package pipeline

import (
	"regexp"
	"strings"

	"mvu-backend/internal/engine/prompt_ir"
)

// WorldbookNode 负责扫描最近的历史，匹配并注入世界书规则
type WorldbookNode struct{}

func NewWorldbookNode() *WorldbookNode {
	return &WorldbookNode{}
}

func (n *WorldbookNode) Name() string {
	return "WorldbookNode"
}

func (n *WorldbookNode) Process(ctx *prompt_ir.ContextData) error {
	if len(ctx.Config.WorldbookEntries) == 0 {
		return nil
	}

	// 1. 构建扫描文本 (把最近的历史拼起来)
	var scanTextBuilder strings.Builder
	for _, msg := range ctx.RecentMessages {
		scanTextBuilder.WriteString(msg.Content)
		scanTextBuilder.WriteString("\n")
	}
	scanText := scanTextBuilder.String()

	// 2. 判定哪些词条被触发
	var activatedEntries []prompt_ir.WorldbookEntry

	for _, entry := range ctx.Config.WorldbookEntries {
		if !entry.Enabled {
			continue
		}

		// 如果是常驻词条，无条件触发
		if entry.Constant {
			activatedEntries = append(activatedEntries, entry)
			continue
		}

		// 关键词匹配（支持 "regex:<pattern>" 前缀，否则大小写不敏感子串匹配）
		triggered := false
		for _, key := range entry.Keys {
			if matchWorldbookKey(scanText, key) {
				triggered = true
				break
			}
		}

		if triggered {
			activatedEntries = append(activatedEntries, entry)
		}
	}

	// 3. 将触发的词条组装成 PromptBlocks
	for _, entry := range activatedEntries {
		resolvedContent := n.resolveMacros(entry.Content, ctx.Variables)

		ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
			Type:     prompt_ir.BlockWorldbook,
			Role:     "system",
			Content:  resolvedContent,
			Priority: 10 + entry.Priority, // 基础权重10，再加上创作者设定的优先级偏移
		})
	}

	return nil
}

// matchWorldbookKey 判断 scanText 是否匹配一条关键词。
//
// 支持两种格式：
//   - 普通字符串：大小写不敏感子串匹配（对齐 TH 默认行为）
//   - "regex:<pattern>"：Go regexp，大小写不敏感（(?i) 自动添加）
func matchWorldbookKey(scanText, key string) bool {
	const regexPrefix = "regex:"
	if strings.HasPrefix(key, regexPrefix) {
		pattern := key[len(regexPrefix):]
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			// 非法正则降级为字面量匹配
			return strings.Contains(strings.ToLower(scanText), strings.ToLower(pattern))
		}
		return re.MatchString(scanText)
	}
	return strings.Contains(strings.ToLower(scanText), strings.ToLower(strings.TrimSpace(key)))
}

func (n *WorldbookNode) resolveMacros(text string, vars map[string]any) string {
	res := text
	for k, v := range vars {
		if strVal, ok := v.(string); ok {
			res = strings.ReplaceAll(res, "{{"+k+"}}", strVal)
		}
	}
	return res
}
