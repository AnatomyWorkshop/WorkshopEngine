package gateway

import "github.com/gin-gonic/gin"

// CORSConfig CORS 配置。
type CORSConfig struct {
	// AllowedOrigins 允许的来源列表。["*"] 表示允许全部。
	AllowedOrigins []string
}

// CORS 返回 CORS 中间件。
//
// 行为：
//   - 请求来源在 AllowedOrigins 中（或列表含 "*"）→ 回显该来源
//   - 否则回退到列表第一个来源
//   - OPTIONS 预检请求直接返回 204
func CORS(cfg CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := false
		for _, o := range cfg.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}
		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		} else if len(cfg.AllowedOrigins) > 0 {
			c.Header("Access-Control-Allow-Origin", cfg.AllowedOrigins[0])
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Api-Key,X-Account-ID,X-Request-ID")
		c.Header("Vary", "Origin")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
