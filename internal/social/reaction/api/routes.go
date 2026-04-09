package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"mvu-backend/internal/platform/auth"
	"mvu-backend/internal/social/reaction"
)

// RegisterReactionRoutes 挂载点赞/收藏接口到 /api/social/reactions
//
// POST   /social/reactions/:target_type/:target_id/:type  — 点赞/收藏（需登录）
// DELETE /social/reactions/:target_type/:target_id/:type  — 取消（需登录）
// GET    /social/reactions/counts                         — 批量查计数（无需登录）
// GET    /social/reactions/mine/:target_type/:target_id   — 查自己的状态（需登录）
func RegisterReactionRoutes(rg *gin.RouterGroup, svc *reaction.Service) {
	g := rg.Group("/social/reactions")

	// GET /social/reactions/counts?targets=comment:id1,forum_post:id2
	// 必须在 /:target_type 之前注册，防止路径冲突
	g.GET("/counts", func(c *gin.Context) {
		raw := c.Query("targets")
		if raw == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "targets query param required"})
			return
		}
		targets := splitAndTrim(raw, ",")
		counts, err := svc.CountBatch(targets)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": counts})
	})

	// GET /social/reactions/mine/:target_type/:target_id
	g.GET("/mine/:target_type/:target_id", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			// 未登录：全部返回 false，不报错
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": reaction.MineResult{}})
			return
		}
		tt := reaction.TargetType(c.Param("target_type"))
		if !reaction.IsValidTarget(tt) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type"})
			return
		}
		res := svc.CheckMine(tt, c.Param("target_id"), accountID)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": res})
	})

	// POST /social/reactions/:target_type/:target_id/:type
	g.POST("/:target_type/:target_id/:type", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}

		tt, rt, ok := parseParams(c)
		if !ok {
			return
		}

		err := svc.Add(tt, c.Param("target_id"), accountID, rt)
		if errors.Is(err, reaction.ErrAlreadyReacted) {
			c.JSON(http.StatusConflict, gin.H{"error": "already reacted"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
	})

	// DELETE /social/reactions/:target_type/:target_id/:type
	g.DELETE("/:target_type/:target_id/:type", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		if accountID == auth.DefaultAccountID {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}

		tt, rt, ok := parseParams(c)
		if !ok {
			return
		}

		err := svc.Remove(tt, c.Param("target_id"), accountID, rt)
		if errors.Is(err, reaction.ErrNotReacted) {
			c.JSON(http.StatusNotFound, gin.H{"error": "reaction not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
	})
}

// parseParams 从路径参数解析并校验 target_type 和 reaction type
func parseParams(c *gin.Context) (reaction.TargetType, reaction.Type, bool) {
	tt := reaction.TargetType(c.Param("target_type"))
	if !reaction.IsValidTarget(tt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type, must be: comment | forum_post | forum_reply | game"})
		return "", "", false
	}
	rt := reaction.Type(c.Param("type"))
	if !reaction.IsValidType(rt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type, must be: like | favorite"})
		return "", "", false
	}
	return tt, rt, true
}

// splitAndTrim 分割并去除每段空白
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
