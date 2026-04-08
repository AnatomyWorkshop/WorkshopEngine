package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"mvu-backend/internal/core/util"
	"mvu-backend/internal/platform/auth"
	"mvu-backend/internal/social/comment"
	"mvu-backend/internal/social/reaction"
)

// RegisterCommentRoutes 挂载游戏评论区接口
//
// POST   /social/games/:id/comments              发主楼（需登录）
// GET    /social/games/:id/comments              主楼列表（公开）
// POST   /social/comments/:id/replies            回复（需登录）
// GET    /social/comments/:id/replies            子评论列表（公开）
// PATCH  /social/comments/:id                   编辑（仅作者，5 分钟内）
// DELETE /social/comments/:id                   软删除（作者或游戏设计者）
// POST   /social/comments/:id/vote              点赞（需登录）
// DELETE /social/comments/:id/vote              取消点赞（需登录）
//
// 游戏评论区配置（创作者端）：
// GET    /create/games/:id/comment-config
// PATCH  /create/games/:id/comment-config
func RegisterCommentRoutes(rg *gin.RouterGroup, svc *comment.Service) {
	// ── 游戏维度路由 ─────────────────────────────────────────────────
	games := rg.Group("/social/games")

	// GET /social/games/:id/comments
	games.GET("/:id/comments", func(c *gin.Context) {
		gameID := c.Param("id")
		sort := c.DefaultQuery("sort", comment.SortDateDesc)
		threadType := c.DefaultQuery("thread_type", "linear")
		limit, offset := util.ParsePage(c)

		list, total, err := svc.ListByGame(gameID, sort, threadType, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{"total": total, "items": list},
		})
	})

	// POST /social/games/:id/comments
	games.POST("/:id/comments", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			Content    string `json:"content"     binding:"required"`
			ThreadType string `json:"thread_type"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cm, err := svc.Create(c.Param("id"), accountID, req.Content, req.ThreadType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cm})
	})

	// ── 评论维度路由 ─────────────────────────────────────────────────
	comments := rg.Group("/social/comments")

	// GET /social/comments/:id/replies
	comments.GET("/:id/replies", func(c *gin.Context) {
		limit, offset := util.ParsePage(c)
		replies, total, err := svc.ListReplies(c.Param("id"), limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{"total": total, "items": replies},
		})
	})

	// POST /social/comments/:id/replies
	comments.POST("/:id/replies", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			GameID  string `json:"game_id"  binding:"required"`
			Content string `json:"content"  binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cm, err := svc.Reply(req.GameID, accountID, req.Content, c.Param("id"))
		if errors.Is(err, comment.ErrCommentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "parent comment not found"})
			return
		}
		if errors.Is(err, comment.ErrGameMismatch) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "game_id mismatch with parent comment"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cm})
	})

	// PATCH /social/comments/:id
	comments.PATCH("/:id", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			Content string `json:"content" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		cm, err := svc.Edit(c.Param("id"), accountID, req.Content)
		switch {
		case errors.Is(err, comment.ErrCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		case errors.Is(err, comment.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author can edit"})
		case err != nil:
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": cm})
		}
	})

	// DELETE /social/comments/:id
	// Body（可选）: { "game_author_id": "..." } 供游戏设计者删除他人评论
	comments.DELETE("/:id", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			GameAuthorID string `json:"game_author_id"`
		}
		_ = c.ShouldBindJSON(&req) // 可选，失败不报错

		err := svc.Delete(c.Param("id"), accountID, req.GameAuthorID)
		switch {
		case errors.Is(err, comment.ErrCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		case errors.Is(err, comment.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": true}})
		}
	})

	// POST /social/comments/:id/vote
	comments.POST("/:id/vote", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		err := svc.Vote(c.Param("id"), accountID)
		switch {
		case errors.Is(err, comment.ErrCommentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		case errors.Is(err, reaction.ErrAlreadyReacted):
			c.JSON(http.StatusConflict, gin.H{"error": "already voted"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
		}
	})

	// DELETE /social/comments/:id/vote
	comments.DELETE("/:id/vote", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		err := svc.Unvote(c.Param("id"), accountID)
		switch {
		case errors.Is(err, reaction.ErrNotReacted):
			c.JSON(http.StatusNotFound, gin.H{"error": "vote not found"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
		}
	})

	// ── 创作者配置端点 ────────────────────────────────────────────────
	create := rg.Group("/create/games")

	// GET /create/games/:id/comment-config
	create.GET("/:id/comment-config", func(c *gin.Context) {
		cfg := svc.GetConfig(c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cfg})
	})

	// PATCH /create/games/:id/comment-config
	create.PATCH("/:id/comment-config", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req comment.GameCommentConfig
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.GameID = c.Param("id")
		if err := svc.UpsertConfig(req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": req})
	})
}
