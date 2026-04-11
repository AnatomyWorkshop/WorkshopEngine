// Package auth 提供 HTTP 鉴权中间件与账户上下文注入，供所有业务后端共用。
//
// 支持三种鉴权模式（通过环境变量配置）：
//   - ModeOff      — 放行所有请求（开发/内网环境）
//   - ModeAPIKey   — 校验单个静态 Admin Key（单用户/单租户模式）
//   - ModeMultiKey — 多个 Key，每个 Key 映射到一个 account_id（多租户模式）
//
// 账户 ID 来源优先级：
//  1. Key-Account 映射（ModeMultiKey 自动注入）
//  2. X-Account-ID header
//  3. account_id query param
//  4. "anonymous"（AllowAnonymous=true 时）
package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Mode 鉴权模式
type Mode string

const (
	ModeOff      Mode = "off"       // 不校验任何凭证
	ModeAPIKey   Mode = "api_key"   // 单个 Admin Key
	ModeMultiKey Mode = "multi_key" // 多 Key，每 Key 对应一个 account_id
	ModeJWT      Mode = "jwt"       // JWT Bearer token（HS256）
)

const (
	// ContextKeyAccountID gin.Context 中存储账户 ID 的 key
	ContextKeyAccountID = "account_id"
	// DefaultAccountID 未提供账户 ID 时的匿名 ID
	DefaultAccountID = "anonymous"
)

// Config 鉴权配置
type Config struct {
	Mode Mode

	// ModeAPIKey：静态 Admin Key（X-Api-Key 或 Authorization: Bearer <key>）
	// 通过 ADMIN_KEY 环境变量设置
	AdminKey string

	// ModeMultiKey：允许的 Key 列表，通过 AUTH_API_KEYS 设置（逗号分隔）
	APIKeys []string
	// ModeMultiKey：Key → account_id 映射，通过 AUTH_KEY_ACCOUNT_MAP 设置
	// 格式：key1:acc1,key2:acc2
	KeyAccountMap map[string]string

	// ModeJWT：HS256 签名密钥，通过 AUTH_JWT_SECRET 设置
	JWTSecret string

	// AllowAnonymous 为 true 时，未提供账户 ID 的请求仍然放行，
	// account_id 设为 DefaultAccountID（"anonymous"）
	AllowAnonymous bool
}

// NewConfigFromEnv 从已加载的环境参数构建 Config（在 config.Load() 之后调用）。
//
// 参数来自 cmd/server/main.go，避免直接调用 os.Getenv（保持单一配置入口）：
//
//	auth.NewConfigFromEnv(os.Getenv("ADMIN_KEY"), os.Getenv("AUTH_API_KEYS"),
//	    os.Getenv("AUTH_KEY_ACCOUNT_MAP"), os.Getenv("ALLOW_ANONYMOUS") != "false")
func NewConfigFromEnv(adminKey, apiKeysCSV, keyAccountMapCSV string, allowAnon bool, authMode, jwtSecret string) Config {
	cfg := Config{AllowAnonymous: allowAnon, AdminKey: adminKey, JWTSecret: jwtSecret}

	// 显式 AUTH_MODE 优先
	if authMode == "jwt" && jwtSecret != "" {
		cfg.Mode = ModeJWT
		return cfg
	}
	if authMode == "off" {
		cfg.Mode = ModeOff
		return cfg
	}

	// 自动检测（向后兼容）
	if apiKeysCSV != "" {
		keys := splitTrimmed(apiKeysCSV, ",")
		cfg.Mode = ModeMultiKey
		cfg.APIKeys = keys
		cfg.KeyAccountMap = parseKeyAccountMap(keyAccountMapCSV)
	} else if adminKey != "" {
		cfg.Mode = ModeAPIKey
	} else {
		cfg.Mode = ModeOff
	}
	return cfg
}

// Middleware 返回 Gin 鉴权中间件。
//
// 行为（按模式）：
//   - ModeOff      — 直接 Next，仅读取 X-Account-ID
//   - ModeAPIKey   — 校验单 Key，再读取 X-Account-ID
//   - ModeMultiKey — 校验 Key 并自动注入对应 account_id
func Middleware(cfg Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch cfg.Mode {
		case ModeAPIKey:
			if !matchKey(c, cfg.AdminKey) {
				abort(c, "invalid or missing api key")
				return
			}
		case ModeMultiKey:
			key := extractKey(c)
			matched := false
			for _, k := range cfg.APIKeys {
				if k == key {
					matched = true
					// 从映射中自动注入 account_id（优先于 header）
					if accID, ok := cfg.KeyAccountMap[key]; ok {
						c.Set(ContextKeyAccountID, accID)
					}
					break
				}
			}
			if !matched {
				abort(c, "invalid or missing api key")
				return
			}
		case ModeJWT:
			token := extractKey(c)
			if token == "" {
				abort(c, "missing bearer token")
				return
			}
			accountID, err := ParseToken(token, cfg.JWTSecret)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"code":  "invalid_token",
					"error": err.Error(),
				})
				return
			}
			c.Set(ContextKeyAccountID, accountID)
		}

		// 若 account_id 尚未由 key 映射注入，则从 header / query 读取
		if _, exists := c.Get(ContextKeyAccountID); !exists {
			accountID := readAccountID(c)
			if accountID == "" {
				if !cfg.AllowAnonymous {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"code":  "missing_account_id",
						"error": "X-Account-ID header is required",
					})
					return
				}
				accountID = DefaultAccountID
			}
			c.Set(ContextKeyAccountID, accountID)
		}

		c.Next()
	}
}

// GetAccountID 从 gin.Context 中安全读取账户 ID。
// 若未设置（中间件未挂载），回退到 header → "anonymous"。
func GetAccountID(c *gin.Context) string {
	if v, exists := c.Get(ContextKeyAccountID); exists {
		if id, ok := v.(string); ok && id != "" {
			return id
		}
	}
	if id := readAccountID(c); id != "" {
		return id
	}
	return DefaultAccountID
}

// ── 内部工具 ──────────────────────────────────────────────

func abort(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":  "unauthorized",
		"error": msg,
	})
}

// extractKey 从请求中提取 API Key（X-Api-Key 或 Authorization: Bearer）
func extractKey(c *gin.Context) string {
	if k := c.GetHeader("X-Api-Key"); k != "" {
		return k
	}
	auth := c.GetHeader("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// matchKey 验证请求中的 Key 是否与 expected 匹配
func matchKey(c *gin.Context, expected string) bool {
	return extractKey(c) == expected
}

// readAccountID 从 header 或 query 中读取 account_id
func readAccountID(c *gin.Context) string {
	if id := strings.TrimSpace(c.GetHeader("X-Account-ID")); id != "" {
		return id
	}
	if id := strings.TrimSpace(c.Query("account_id")); id != "" {
		return id
	}
	return ""
}

// parseKeyAccountMap 解析 "key1:acc1,key2:acc2" 格式
func parseKeyAccountMap(csv string) map[string]string {
	m := map[string]string{}
	for _, pair := range splitTrimmed(csv, ",") {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func splitTrimmed(s, sep string) []string {
	if s == "" {
		return nil
	}
	raw := strings.Split(s, sep)
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
