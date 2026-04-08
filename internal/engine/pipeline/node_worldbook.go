package pipeline

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"mvu-backend/internal/engine/macros"
	"mvu-backend/internal/engine/prompt_ir"
)

// WorldbookNode 扫描历史消息，匹配并注入世界书词条。
//
// # 触发逻辑
//
//  1. 常驻词条（Constant=true）无条件注入。
//  2. 主关键词（Keys）：任意一条命中即通过。
//     支持 "regex:<pattern>" 前缀（Go regexp，自动添加 (?i)）；
//     WholeWord=true 时要求词边界匹配。
//  3. 次级关键词（SecondaryKeys + SecondaryLogic）：在主关键词通过后额外检查。
//     - and_any : 次级关键词中至少一条命中
//     - and_all : 次级关键词全部命中
//     - not_any : 次级关键词全部未命中
//     - not_all : 次级关键词中至少一条未命中
//  4. ScanDepth > 0 时只扫描最近 ScanDepth 条消息（不包括当前用户输入）。
//  5. 首次扫描完成后，额外进行一次递归扫描：
//     将已激活词条的 Content 拼接为新的扫描文本，再扫描剩余未激活词条一次。
//
// # 注入位置
//
//   - "before_template" : Priority = 10 + priority_offset（在 TemplateNode 1000 之前）
//   - "after_template"  : Priority = 1050 + priority_offset（在 TemplateNode 1000 之后）
//   - "at_depth"        : Priority = -200 - priority_offset（嵌入历史段）
type WorldbookNode struct{}

func NewWorldbookNode() *WorldbookNode {
	return &WorldbookNode{}
}

func (n *WorldbookNode) Name() string { return "WorldbookNode" }

func (n *WorldbookNode) Process(ctx *prompt_ir.ContextData) error {
	entries := ctx.Config.WorldbookEntries
	if len(entries) == 0 {
		return nil
	}

	// ── 构建扫描文本 ─────────────────────────────────────────
	// recentMessages 中最后一条是本轮用户输入，前面是历史
	msgs := ctx.RecentMessages
	scanText := buildScanText(msgs, 0) // 全量扫描，用于 ScanDepth=0 情形

	// ── 第一次扫描 ──────────────────────────────────────────
	activated := make([]prompt_ir.WorldbookEntry, 0, len(entries))
	activatedIDs := make(map[string]bool, len(entries))
	remaining := make([]prompt_ir.WorldbookEntry, 0, len(entries))

	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		if entry.Constant {
			activated = append(activated, entry)
			activatedIDs[entry.ID] = true
			continue
		}

		// 按词条的 ScanDepth 构建各自的扫描窗口
		text := scanText
		if entry.ScanDepth > 0 {
			text = buildScanText(msgs, entry.ScanDepth)
		}

		if n.matches(text, entry, ctx.Variables) {
			activated = append(activated, entry)
			activatedIDs[entry.ID] = true
		} else {
			remaining = append(remaining, entry)
		}
	}

	// ── 递归扫描（1 级） ─────────────────────────────────────
	// 把已激活词条的内容拼起来，再对剩余词条扫描一次
	if len(activated) > 0 && len(remaining) > 0 {
		var recursiveScan strings.Builder
		for _, e := range activated {
			recursiveScan.WriteString(e.Content)
			recursiveScan.WriteString("\n")
		}
		recText := recursiveScan.String()

		for _, entry := range remaining {
			if !entry.Enabled || entry.Constant {
				continue
			}
			if n.matches(recText, entry, ctx.Variables) {
				activated = append(activated, entry)
			}
		}
	}

	// ── 分组裁剪（互斥分组）─────────────────────────────────────
	// 同组词条（Group != ""）按 GroupWeight 降序排列，超出 cap 的词条丢弃。
	// cap 由 GameConfig.WorldbookGroupCap 配置（默认 1，即每组只保留权重最高的词条）。
	activated = applyGroupCap(activated, ctx.Config.WorldbookGroupCap)

	// ── Token 预算裁剪 ──────────────────────────────────────────
	// 若 WorldbookTokenBudget > 0，则在 GroupCap 之后再做一次全局 token 裁剪：
	// Constant=true 的词条 pinned，不计入预算；其余词条按 Priority 升序保留。
	if ctx.Config.WorldbookTokenBudget > 0 {
		activated = applyTokenBudget(activated, ctx.Config.WorldbookTokenBudget)
	}

	macroCtx := macros.MacroContext{
		CharName:    ctx.CharName,
		UserName:    ctx.UserName,
		PersonaName: ctx.PersonaName,
		Variables:   ctx.Variables,
		Now:         time.Now(),
	}

	// ── 组装 PromptBlocks + 记录命中 ID ───────────────────────
	for _, entry := range activated {
		content := macros.Expand(entry.Content, macroCtx)

		if entry.Position == "at_depth" {
			// at_depth 词条：不进入普通 Block 排序，由 Runner 插入历史特定位置
			ctx.AtDepthBlocks = append(ctx.AtDepthBlocks, prompt_ir.AtDepthBlock{
				Content:  content,
				Depth:    entry.Depth,
				Priority: entry.Priority,
				EntryID:  entry.ID,
			})
		} else {
			priority := positionToPriority(entry.Position, entry.Priority)
			ctx.Blocks = append(ctx.Blocks, prompt_ir.PromptBlock{
				Type:     prompt_ir.BlockWorldbook,
				Role:     "system",
				Content:  content,
				Priority: priority,
			})
		}
		// 暴露命中词条 ID，供 PromptSnapshot 持久化使用
		if entry.ID != "" {
			ctx.ActivatedWorldbookIDs = append(ctx.ActivatedWorldbookIDs, entry.ID)
		}
	}

	return nil
}

// matches 检查扫描文本是否满足词条的完整触发条件（主关键词 + 次级关键词逻辑门）。
func (n *WorldbookNode) matches(text string, entry prompt_ir.WorldbookEntry, vars map[string]any) bool {
	// 主关键词：任意一条匹配
	primaryOK := false
	for _, key := range entry.Keys {
		if matchKey(text, key, entry.WholeWord, vars) {
			primaryOK = true
			break
		}
	}
	if !primaryOK {
		return false
	}

	// 无次级关键词时直接通过
	if len(entry.SecondaryKeys) == 0 {
		return true
	}

	logic := entry.SecondaryLogic
	if logic == "" {
		logic = "and_any"
	}

	hits := 0
	for _, key := range entry.SecondaryKeys {
		if matchKey(text, key, entry.WholeWord, vars) {
			hits++
		}
	}
	total := len(entry.SecondaryKeys)

	switch logic {
	case "and_any":
		return hits > 0
	case "and_all":
		return hits == total
	case "not_any":
		return hits == 0
	case "not_all":
		return hits < total
	default:
		return hits > 0
	}
}

// buildScanText 从消息列表末尾取 depth 条（0=全部）拼成扫描文本。
func buildScanText(msgs []prompt_ir.Message, depth int) string {
	target := msgs
	if depth > 0 && depth < len(msgs) {
		target = msgs[len(msgs)-depth:]
	}
	var sb strings.Builder
	for _, m := range target {
		sb.WriteString(m.Content)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// matchKey 检查单条关键词是否命中。
//
// 格式支持：
//   - "var:key=value"  — 变量等值条件（vars["key"] == "value"）；引擎层强制门控，与扫描文本无关
//   - "var:key!=value" — 变量不等条件
//   - "var:key"        — 变量存在且非空
//   - "regex:<pattern>" — Go regexp，自动加 (?i)；出错降级为字面量
//   - 普通字符串       — 大小写不敏感子串（wholeWord=true 时加 \b 边界）
func matchKey(text, key string, wholeWord bool, vars map[string]any) bool {
	const varPrefix = "var:"
	if strings.HasPrefix(key, varPrefix) {
		expr := key[len(varPrefix):]
		// var:key!=value
		if idx := strings.Index(expr, "!="); idx > 0 {
			varName, expected := expr[:idx], expr[idx+2:]
			actual := fmt.Sprintf("%v", vars[varName])
			return actual != expected
		}
		// var:key=value
		if idx := strings.Index(expr, "="); idx > 0 {
			varName, expected := expr[:idx], expr[idx+1:]
			actual := fmt.Sprintf("%v", vars[varName])
			return actual == expected
		}
		// var:key — 存在且非空
		val, ok := vars[expr]
		if !ok || val == nil {
			return false
		}
		return fmt.Sprintf("%v", val) != ""
	}

	const regexPrefix = "regex:"
	if strings.HasPrefix(key, regexPrefix) {
		pattern := key[len(regexPrefix):]
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			return strings.Contains(strings.ToLower(text), strings.ToLower(strings.TrimSpace(pattern)))
		}
		return re.MatchString(text)
	}

	literal := strings.TrimSpace(key)
	if wholeWord {
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(literal) + `\b`)
		if err != nil {
			return strings.Contains(strings.ToLower(text), strings.ToLower(literal))
		}
		return re.MatchString(text)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(literal))
}

// positionToPriority 将 WorldbookEntry.Position 映射为 PromptBlock.Priority 数值。
//
// Priority 越小越靠前（靠近 System Prompt 顶部）。当前节点参考值：
//
//	TemplateNode   1000
//	WorldbookNode  10~510 (before_template 默认)
//	MemoryNode     400
//	HistoryNode    0~-N
func positionToPriority(position string, offset int) int {
	switch position {
	case "after_template":
		return 1050 + offset
	case "at_depth":
		return -200 - offset
	default: // "before_template" 及任何未知值
		return 10 + offset
	}
}

// applyGroupCap 对同组词条做互斥裁剪：
// 每个非空 Group 内，按 GroupWeight 降序排列，只保留前 cap 条。
// cap <= 0 时视为 1（默认每组最多保留一条）。
// Group 为空的词条不受影响，全部保留。
func applyGroupCap(entries []prompt_ir.WorldbookEntry, cap int) []prompt_ir.WorldbookEntry {
	if cap <= 0 {
		cap = 1
	}

	// 按组收集
	groups := map[string][]prompt_ir.WorldbookEntry{}
	var ungrouped []prompt_ir.WorldbookEntry
	for _, e := range entries {
		if e.Group == "" {
			ungrouped = append(ungrouped, e)
		} else {
			groups[e.Group] = append(groups[e.Group], e)
		}
	}

	result := ungrouped
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			return group[i].GroupWeight > group[j].GroupWeight
		})
		if len(group) > cap {
			group = group[:cap]
		}
		result = append(result, group...)
	}
	return result
}

// estimateTokens 估算文本的 token 数量。
// 使用字符数（rune）/ 4 取上整，与 TH SimpleTokenCounter 策略一致。
// 对于 CJK 字符，每字约 1-1.5 token，此估算偏保守，可接受。
func estimateTokens(text string) int {
	n := len([]rune(text))
	return (n + 3) / 4
}

// applyTokenBudget 按 token 总预算裁剪词条列表。
//
// 策略（对齐 TH token-budget.ts）：
//  1. Constant=true 的词条视为 pinned，始终保留，不占用预算。
//  2. 其余词条按 Priority 升序排序（数值越小 = 越重要，优先保留）。
//  3. 从小到大累加 token 数，直到超出预算，超出部分丢弃。
//
// budget <= 0 时不裁剪（调用方应提前判断，此处做防御处理）。
func applyTokenBudget(entries []prompt_ir.WorldbookEntry, budget int) []prompt_ir.WorldbookEntry {
	if budget <= 0 {
		return entries
	}

	// 分离 pinned（常驻）和 prunable（可裁剪）
	var pinned []prompt_ir.WorldbookEntry
	var prunable []prompt_ir.WorldbookEntry
	for _, e := range entries {
		if e.Constant {
			pinned = append(pinned, e)
		} else {
			prunable = append(prunable, e)
		}
	}

	// prunable 按 Priority 升序（数值小 = 重要性高，先保留）
	sort.Slice(prunable, func(i, j int) bool {
		return prunable[i].Priority < prunable[j].Priority
	})

	// 贪心保留：累加 token，超出预算后停止
	used := 0
	kept := prunable[:0:0] // 复用底层数组但长度为 0
	for _, e := range prunable {
		cost := estimateTokens(e.Content)
		if used+cost > budget {
			continue // 丢弃：超出预算
		}
		used += cost
		kept = append(kept, e)
	}

	// 合并：pinned 在前（无条件），kept 在后
	result := make([]prompt_ir.WorldbookEntry, 0, len(pinned)+len(kept))
	result = append(result, pinned...)
	result = append(result, kept...)
	return result
}
