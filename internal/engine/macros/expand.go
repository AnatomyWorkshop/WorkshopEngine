// Package macros 提供 ST 兼容宏展开功能。
//
// # 支持的宏集合（最小可用版，覆盖 ~90% 的实际使用场景）
//
//   - {{char}}           → MacroContext.CharName（角色名）
//   - {{user}}           → MacroContext.UserName（玩家名，默认"你"）
//   - {{persona}}        → MacroContext.PersonaName（角色人设名，回退至 CharName）
//   - {{getvar::key}}    → MacroContext.Variables["key"]（字符串化）
//   - {{time}}           → MacroContext.Now 格式化为 15:04:05
//   - {{date}}           → MacroContext.Now 格式化为 2006-01-02
//
// 展开策略：
//   - 宏展开在 Pipeline 组装时（PresetEntryNode / WorldbookNode 输出内容前）调用
//   - 未知宏保留原文（不替换）
//   - 展开结果不再递归展开（防止无限循环）
package macros

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MacroContext 宏展开所需的上下文信息。
//
// 所有字段均为可选：空字符串或 nil 时，对应宏保留原文或使用默认值。
type MacroContext struct {
	// CharName 角色名，对应 {{char}}。
	// 来源：GameTemplate.Config.char_name 或 CharacterCard.Name。
	CharName string

	// UserName 玩家名，对应 {{user}}。
	// 来源：GameTemplate.Config.player_name，默认"你"。
	UserName string

	// PersonaName 角色人设显示名，对应 {{persona}}。
	// 若为空，回退至 CharName。
	PersonaName string

	// Variables 当前会话变量快照（Flatten 后的 map），对应 {{getvar::key}}。
	Variables map[string]any

	// Now 当前时间，对应 {{time}} / {{date}}。
	// 若为零值，使用 time.Now()。
	Now time.Time
}

// reGetvar 匹配 {{getvar::key}} 格式（key 可以是任意非 }} 字符）。
var reGetvar = regexp.MustCompile(`\{\{getvar::([^}]+)\}\}`)

// Expand 展开 text 中的 ST 宏，返回展开后的字符串。
//
// 展开顺序：先处理 {{getvar::key}} 动态宏，再处理固定宏。
// 未识别的 {{xxx}} 占位符保留原文。
func Expand(text string, ctx MacroContext) string {
	if text == "" {
		return text
	}

	// 时间零值兜底
	now := ctx.Now
	if now.IsZero() {
		now = time.Now()
	}

	// PersonaName 回退
	personaName := ctx.PersonaName
	if personaName == "" {
		personaName = ctx.CharName
	}

	// UserName 默认值
	userName := ctx.UserName
	if userName == "" {
		userName = "你"
	}

	result := text

	// 1. {{getvar::key}} — 动态变量宏（先处理，因为 key 本身可能包含其他宏展开结果）
	result = reGetvar.ReplaceAllStringFunc(result, func(match string) string {
		subs := reGetvar.FindStringSubmatch(match)
		if len(subs) < 2 {
			return match
		}
		key := strings.TrimSpace(subs[1])
		if ctx.Variables == nil {
			return match
		}
		val, ok := ctx.Variables[key]
		if !ok || val == nil {
			return match
		}
		return fmt.Sprintf("%v", val)
	})

	// 2. 固定宏替换（按出现频率降序，减少 ReplaceAll 调用次数）
	if ctx.CharName != "" {
		result = strings.ReplaceAll(result, "{{char}}", ctx.CharName)
	}
	result = strings.ReplaceAll(result, "{{user}}", userName)
	result = strings.ReplaceAll(result, "{{persona}}", personaName)
	result = strings.ReplaceAll(result, "{{time}}", now.Format("15:04:05"))
	result = strings.ReplaceAll(result, "{{date}}", now.Format("2006-01-02"))

	return result
}
