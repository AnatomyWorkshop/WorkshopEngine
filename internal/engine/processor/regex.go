package processor

import (
	"regexp"
	"strings"

	"mvu-backend/internal/engine/prompt_ir"
)

// ApplyToAIOutput 对 AI 输出文本应用 apply_to=ai_output 或 all 的规则（按 Order 顺序）。
func ApplyToAIOutput(text string, rules []prompt_ir.RegexRule) string {
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.ApplyTo != "ai_output" && r.ApplyTo != "all" {
			continue
		}
		text = applyRule(text, r)
	}
	return text
}

// ApplyToUserInput 对用户输入文本应用 apply_to=user_input 或 all 的规则（按 Order 顺序）。
func ApplyToUserInput(text string, rules []prompt_ir.RegexRule) string {
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.ApplyTo != "user_input" && r.ApplyTo != "all" {
			continue
		}
		text = applyRule(text, r)
	}
	return text
}

// applyRule 将单条规则应用到文本，返回结果。非法正则则原样返回。
func applyRule(text string, r prompt_ir.RegexRule) string {
	pattern, flags := parsePatternFlags(r.Pattern)
	if flags != "" {
		// 将 flags 转为 Go regexp 内联标志，仅支持 i / m / s
		var flagStr strings.Builder
		for _, f := range flags {
			switch f {
			case 'i', 'm', 's':
				flagStr.WriteRune(f)
			}
		}
		if flagStr.Len() > 0 {
			pattern = "(?" + flagStr.String() + ")" + pattern
		}
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text // 非法正则，跳过
	}
	return re.ReplaceAllString(text, r.Replacement)
}

// parsePatternFlags 从 /pattern/flags 格式中解析出 pattern 和 flags。
// 若不是该格式则原样返回 pattern，flags 为空。
func parsePatternFlags(raw string) (pattern, flags string) {
	if len(raw) > 1 && raw[0] == '/' {
		// 从末尾找最后一个 /（跳过第一个字符）
		lastSlash := strings.LastIndex(raw[1:], "/")
		if lastSlash >= 0 {
			lastSlash++ // 修正偏移量
			pattern = raw[1:lastSlash]
			flags = raw[lastSlash+1:]
			return
		}
	}
	return raw, ""
}
