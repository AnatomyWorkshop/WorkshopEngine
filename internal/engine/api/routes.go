package api

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterGameRoutes 注册游玩层接口（/api/v2/play/...）
func RegisterGameRoutes(rg *gin.RouterGroup, engine *GameEngine) {
	play := rg.Group("/play")

	play.POST("/sessions", func(c *gin.Context) {
		var req struct {
			GameID string `json:"game_id" binding:"required"`
			UserID string `json:"user_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		sessID, err := engine.CreateSession(c.Request.Context(), req.GameID, req.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"session_id": sessID}})
	})

	play.POST("/sessions/:id/turn", func(c *gin.Context) {
		var req TurnRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.SessionID = c.Param("id")
		resp, err := engine.PlayTurn(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
	})

	play.POST("/sessions/:id/regen", func(c *gin.Context) {
		var req TurnRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		req.SessionID = c.Param("id")
		req.IsRegen = true
		resp, err := engine.PlayTurn(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
	})

	play.GET("/sessions/:id/state", func(c *gin.Context) {
		state, err := engine.GetState(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": state})
	})

	play.GET("/sessions/:id/stream", func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		userInput := c.Query("input")
		if userInput == "" {
			c.SSEvent("error", "input required")
			return
		}

		tokenCh, errCh := engine.StreamTurn(c.Request.Context(), TurnRequest{
			SessionID: c.Param("id"),
			UserInput: userInput,
		})

		c.Stream(func(w io.Writer) bool {
			select {
			case token, ok := <-tokenCh:
				if !ok {
					c.SSEvent("done", "")
					return false
				}
				c.SSEvent("token", token)
				return true
			case err := <-errCh:
				if err != nil {
					c.SSEvent("error", err.Error())
				}
				return false
			}
		})
	})

	// PATCH /sessions/:id — 更新会话标题/状态
	play.PATCH("/sessions/:id", func(c *gin.Context) {
		var req UpdateSessionReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		sess, err := engine.UpdateSession(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": sess})
	})

	// DELETE /sessions/:id — 删除会话及所有关联数据
	play.DELETE("/sessions/:id", func(c *gin.Context) {
		if err := engine.DeleteSession(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": c.Param("id"), "deleted": true}})
	})

	// GET /sessions/:id/variables — 获取会话变量快照
	play.GET("/sessions/:id/variables", func(c *gin.Context) {
		state, err := engine.GetState(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": state.Variables})
	})

	// PATCH /sessions/:id/variables — 合并更新会话变量
	play.PATCH("/sessions/:id/variables", func(c *gin.Context) {
		var patch map[string]any
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		vars, err := engine.PatchVariables(c.Request.Context(), c.Param("id"), patch)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": vars})
	})

	// GET /sessions — 列出会话（?game_id=&user_id=&limit=&offset=）
	play.GET("/sessions", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		sessions, err := engine.ListSessions(c.Request.Context(), ListSessionsReq{
			GameID: c.Query("game_id"),
			UserID: c.Query("user_id"),
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": sessions})
	})

	// GET /sessions/:id/floors — 楼层列表（含激活页摘要）
	play.GET("/sessions/:id/floors", func(c *gin.Context) {
		floors, err := engine.ListFloors(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": floors})
	})

	// GET /sessions/:id/floors/:fid/pages — Swipe 页列表
	play.GET("/sessions/:id/floors/:fid/pages", func(c *gin.Context) {
		pages, err := engine.ListPages(c.Request.Context(), c.Param("fid"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": pages})
	})

	// PATCH /sessions/:id/floors/:fid/pages/:pid/activate — Swipe 选页
	play.PATCH("/sessions/:id/floors/:fid/pages/:pid/activate", func(c *gin.Context) {
		if err := engine.SetActivePage(c.Request.Context(), c.Param("fid"), c.Param("pid")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"floor_id": c.Param("fid"), "page_id": c.Param("pid")}})
	})

	// GET /sessions/:id/memories — 列出记忆条目
	play.GET("/sessions/:id/memories", func(c *gin.Context) {
		mems, err := engine.ListMemories(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": mems})
	})

	// POST /sessions/:id/memories — 手动创建记忆（创作者/调试用）
	play.POST("/sessions/:id/memories", func(c *gin.Context) {
		var req CreateMemoryReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		mem, err := engine.CreateMemory(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": mem})
	})

	// PATCH /sessions/:id/memories/:mid — 更新记忆字段（content/importance/type）
	play.PATCH("/sessions/:id/memories/:mid", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		mem, err := engine.UpdateMemory(c.Request.Context(), c.Param("id"), c.Param("mid"), updates)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": mem})
	})

	// DELETE /sessions/:id/memories/:mid — 删除记忆（?hard=true 物理删除）
	play.DELETE("/sessions/:id/memories/:mid", func(c *gin.Context) {
		hard := c.Query("hard") == "true"
		if err := engine.DeleteMemory(c.Request.Context(), c.Param("id"), c.Param("mid"), hard); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": true}})
	})

	// POST /sessions/:id/memories/consolidate — 立即触发记忆整合（同步，调试用）
	play.POST("/sessions/:id/memories/consolidate", func(c *gin.Context) {
		if err := engine.ConsolidateNow(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"consolidated": true}})
	})

	// GET /sessions/:id/prompt-preview — Prompt dry-run（不调用 LLM，供创作者调试）
	play.GET("/sessions/:id/prompt-preview", func(c *gin.Context) {
		preview, err := engine.PromptPreview(c.Request.Context(), c.Param("id"), c.Query("input"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": preview})
	})
}
