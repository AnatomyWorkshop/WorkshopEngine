package play

import "github.com/gin-gonic/gin"

// RegisterPlayRoutes 注册玩家发现层路由（/api/play/...）
// 注意：/play/games/worldbook/:id 必须在 /play/games/:slug 之前注册，
// 否则 Gin 会将 "worldbook" 匹配为 :slug 参数。
func RegisterPlayRoutes(rg *gin.RouterGroup, h *Handler) {
	play := rg.Group("/play")

	play.GET("/games", h.listGames)
	play.GET("/games/worldbook/:id", h.getWorldbook)
	play.GET("/games/:slug", h.getGame)
	play.GET("/sessions", h.listSessions)
	play.POST("/sessions", h.createSession)
}
