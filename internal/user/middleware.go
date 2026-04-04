// Package user 是向后兼容薄层，重新导出 internal/platform/auth 的公开符号。
//
// Deprecated: 直接导入 mvu-backend/internal/platform/auth。
package user

import "mvu-backend/internal/platform/auth"

// 重新导出常量
const (
	ContextKeyAccountID = auth.ContextKeyAccountID
	DefaultAccountID    = auth.DefaultAccountID
)

// 重新导出类型
type AuthConfig = auth.Config

// 重新导出函数
var (
	Middleware    = auth.Middleware
	GetAccountID  = auth.GetAccountID
)
