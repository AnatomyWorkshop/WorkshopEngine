// Package util 提供跨层共用的小工具函数。
package util

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// Slugify 将任意名称转换为 URL-safe slug。
//
// 规则：
//   - ASCII 字母转小写保留，数字保留
//   - 空格 / 连字符 / 下划线 → 连字符
//   - 其余字符（含中文）跳过
//   - 结果为空时生成 "item-<uuid8>" 作为兜底
//
// 兜底前缀通过 fallbackPrefix 参数自定义（如 "card", "game", "post"）。
func Slugify(name, fallbackPrefix string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(unicode.ToLower(r))
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		prefix := fallbackPrefix
		if prefix == "" {
			prefix = "item"
		}
		return fmt.Sprintf("%s-%s", prefix, uuid.New().String()[:8])
	}
	return b.String()
}
