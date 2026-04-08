package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"mvu-backend/internal/core/util"
	"mvu-backend/internal/platform/auth"
	"mvu-backend/internal/social/forum"
	"mvu-backend/internal/social/reaction"
)

// RegisterForumRoutes 挂载社区论坛接口
//
// GET    /social/posts                    帖子列表（?game_tag=&type=&sort=hot|new&q=）
// POST   /social/posts                    发帖（需登录）
// GET    /social/posts/:id                帖子详情（id 或 slug）
// PATCH  /social/posts/:id                编辑（仅作者）
// DELETE /social/posts/:id                删除（仅作者，软删除）
// GET    /social/posts/:id/replies        盖楼列表（分页）
// POST   /social/posts/:id/replies        盖楼（需登录）
// POST   /social/posts/:id/vote           点赞（需登录）
// DELETE /social/posts/:id/vote           取消点赞（需登录）
func RegisterForumRoutes(rg *gin.RouterGroup, svc *forum.Service) {
	g := rg.Group("/social/posts")

	// GET /social/posts
	g.GET("", func(c *gin.Context) {
		limit, offset := util.ParsePage(c)
		result, err := svc.ListPosts(
			c.Query("game_tag"),
			c.Query("type"),
			c.DefaultQuery("sort", "new"),
			c.Query("q"),
			limit, offset,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
	})

	// POST /social/posts
	g.POST("", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			Title    string   `json:"title"    binding:"required"`
			Content  string   `json:"content"  binding:"required"`
			GameTags []string `json:"game_tags"`
			PostType string   `json:"post_type"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		post, err := svc.CreatePost(accountID, req.Title, req.Content, req.GameTags, req.PostType)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": post})
	})

	// GET /social/posts/:id
	g.GET("/:id", func(c *gin.Context) {
		post, err := svc.GetPost(c.Param("id"))
		if errors.Is(err, forum.ErrPostNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": post})
	})

	// PATCH /social/posts/:id
	g.PATCH("/:id", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			Title    string   `json:"title"    binding:"required"`
			Content  string   `json:"content"  binding:"required"`
			GameTags []string `json:"game_tags"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		post, err := svc.EditPost(c.Param("id"), accountID, req.Title, req.Content, req.GameTags)
		switch {
		case errors.Is(err, forum.ErrPostNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		case errors.Is(err, forum.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author can edit"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": post})
		}
	})

	// DELETE /social/posts/:id
	g.DELETE("/:id", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		err := svc.DeletePost(c.Param("id"), accountID)
		switch {
		case errors.Is(err, forum.ErrPostNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		case errors.Is(err, forum.ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "only the author can delete"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": true}})
		}
	})

	// GET /social/posts/:id/replies
	g.GET("/:id/replies", func(c *gin.Context) {
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

	// POST /social/posts/:id/replies
	g.POST("/:id/replies", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		var req struct {
			Content  string `json:"content"   binding:"required"`
			ParentID string `json:"parent_id"` // 可选，楼中楼
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		reply, err := svc.CreateReply(c.Param("id"), accountID, req.ParentID, req.Content)
		if errors.Is(err, forum.ErrPostNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": reply})
	})

	// POST /social/posts/:id/vote
	g.POST("/:id/vote", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		err := svc.VotePost(c.Param("id"), accountID)
		switch {
		case errors.Is(err, forum.ErrPostNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "post not found"})
		case errors.Is(err, reaction.ErrAlreadyReacted):
			c.JSON(http.StatusConflict, gin.H{"error": "already voted"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
		}
	})

	// DELETE /social/posts/:id/vote
	g.DELETE("/:id/vote", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		err := svc.UnvotePost(c.Param("id"), accountID)
		switch {
		case errors.Is(err, reaction.ErrNotReacted):
			c.JSON(http.StatusNotFound, gin.H{"error": "vote not found"})
		case err != nil:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
		}
	})
}
