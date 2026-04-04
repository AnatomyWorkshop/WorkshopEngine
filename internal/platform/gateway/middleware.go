// Package gateway 提供 HTTP 请求生命周期管理中间件，供所有业务后端共用。
//
// 中间件列表：
//   - RequestID  — 注入唯一请求 ID（X-Request-ID）
//   - StructuredLogger — JSON 结构化请求日志
//   - Recovery   — Panic 捕获并返回 500
//
// 使用方式：
//
//	r := gin.New()
//	r.Use(gateway.Recovery())
//	r.Use(gateway.RequestID())
//	r.Use(gateway.StructuredLogger())
package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// KeyRequestID gin.Context 中存储请求 ID 的 key
	KeyRequestID = "request_id"
	// HeaderRequestID HTTP 请求/响应头名
	HeaderRequestID = "X-Request-ID"
)

// RequestID 中间件：读取或生成唯一请求 ID，写入 gin.Context 和响应 header。
//
// 优先使用客户端传入的 X-Request-ID（便于前端全链路追踪），
// 否则生成 UUID v4。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(HeaderRequestID)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set(KeyRequestID, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// GetRequestID 从 gin.Context 中安全读取请求 ID。
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(KeyRequestID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// logEntry 结构化日志条目（JSON 输出）
type logEntry struct {
	Time      string `json:"time"`
	Level     string `json:"level"`
	RequestID string `json:"request_id,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	ClientIP  string `json:"client_ip"`
	Error     string `json:"error,omitempty"`
}

// StructuredLogger 中间件：以 JSON 格式记录每个请求的关键信息。
//
// 输出字段：time / level / request_id / account_id / method / path /
// status / latency_ms / client_ip / error（非 2xx 时）。
func StructuredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		entry := logEntry{
			Time:      start.UTC().Format(time.RFC3339),
			RequestID: GetRequestID(c),
			Method:    c.Request.Method,
			Path:      c.Request.URL.Path,
			Status:    c.Writer.Status(),
			LatencyMs: time.Since(start).Milliseconds(),
			ClientIP:  c.ClientIP(),
		}
		// 从 Context 读取 account_id（由 auth 中间件注入）
		if v, ok := c.Get("account_id"); ok {
			if id, ok := v.(string); ok {
				entry.AccountID = id
			}
		}
		// 状态码 >= 400 时附带首个 error 信息
		if entry.Status >= 400 {
			entry.Level = "warn"
			if errs := c.Errors; len(errs) > 0 {
				entry.Error = errs[0].Error()
			}
		} else {
			entry.Level = "info"
		}

		data, _ := json.Marshal(entry)
		log.Println(string(data))
	}
}

// Recovery 中间件：捕获 handler panic，记录堆栈，返回 500。
// 替代 gin.Recovery()，保留结构化日志格式。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Printf(`{"level":"error","event":"panic","request_id":%q,"error":%q,"stack":%q}`,
					GetRequestID(c), fmt.Sprintf("%v", r), string(stack))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error":      "internal server error",
					"request_id": GetRequestID(c),
				})
			}
		}()
		c.Next()
	}
}
