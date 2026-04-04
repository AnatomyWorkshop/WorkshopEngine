// Package tokenizer 提供轻量级 token 数量估算工具。
//
// 不依赖外部库，基于字符类型启发式：
//   - ASCII（英文/数字/标点）：约 4 字符 / token（BPE 合并英文词根）
//   - CJK 及其他非 ASCII：约 1.5 字符 / token（CJK 双字词常被合并为 1 token）
//
// 精度：误差通常在 ±15% 以内，足以用于 prompt 预算裁剪。
// 若需精确计数，应以 API 响应中的 usage.prompt_tokens 为准。
package tokenizer

// Estimate 估算文本的近似 token 数。
// 返回值保证 ≥ 1（对非空字符串）。
func Estimate(text string) int {
	if text == "" {
		return 0
	}

	var ascii, other int
	for _, r := range text {
		if r < 0x80 {
			ascii++
		} else {
			other++
		}
	}

	// ASCII：每 4 字符约 1 token
	// CJK/其他：每 1.5 字符约 1 token（即 other * 2 / 3）
	tokens := ascii/4 + (other*2)/3
	if tokens == 0 {
		return 1 // 最少计 1 token（避免空估算）
	}
	return tokens
}

// EstimateMessages 估算多条消息（role+content）的总 token 数。
// 每条消息额外计 4 token 的结构开销（OpenAI 格式惯例）。
func EstimateMessages(messages []map[string]string) int {
	total := 0
	for _, m := range messages {
		total += Estimate(m["content"]) + 4
	}
	return total
}
