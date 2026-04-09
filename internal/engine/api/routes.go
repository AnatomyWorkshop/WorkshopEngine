package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/util"
	"mvu-backend/internal/engine/session"
)

// RegisterGameRoutes 注册游玩层接口（/api/play/...）
func RegisterGameRoutes(rg *gin.RouterGroup, engine *GameEngine) {
	play := rg.Group("/play")

	// GET /play/games — 已发布游戏列表（玩家侧公开字段，支持分页/标签/排序）
	play.GET("/games", func(c *gin.Context) {
		limit, offset := util.ParsePage(c)

		query := engine.db.Model(&dbmodels.GameTemplate{}).Where("status = 'published'")

		// 标签过滤（逗号分隔，AND 语义：每个标签都必须存在）
		if tags := c.Query("tags"); tags != "" {
			for _, tag := range strings.Split(tags, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					query = query.Where("config->'tags' ? ?", tag)
				}
			}
		}

		// 游戏类型过滤
		if t := c.Query("type"); t != "" {
			query = query.Where("type = ?", t)
		}

		// 排序
		switch c.DefaultQuery("sort", "new") {
		case "hot", "play_count":
			query = query.Order("play_count DESC, created_at DESC")
		default:
			query = query.Order("created_at DESC")
		}

		var total int64
		query.Count(&total)

		var templates []dbmodels.GameTemplate
		query.Select("id, slug, title, type, short_desc, notes, cover_url, author_id, play_count, config, created_at").
			Limit(limit).Offset(offset).Find(&templates)

		games := make([]gin.H, 0, len(templates))
		for _, t := range templates {
			games = append(games, publicGameView(t))
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"games":  games,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		}})
	})

	// GET /play/games/:slug — 单个游戏详情（slug 或 UUID）
	play.GET("/games/:slug", func(c *gin.Context) {
		slug := c.Param("slug")
		var tmpl dbmodels.GameTemplate
		// 先按 slug 查，再按 UUID 查
		err := engine.db.Where("status = 'published' AND (slug = ? OR id::text = ?)", slug, slug).
			First(&tmpl).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": publicGameView(tmpl)})
	})

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
		// 原子递增游玩次数
		engine.db.Model(&dbmodels.GameTemplate{}).Where("id = ?", req.GameID).
			UpdateColumn("play_count", gorm.Expr("play_count + 1"))
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
			if errors.Is(err, session.ErrConcurrentGeneration) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "concurrent_generation"})
				return
			}
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
			if errors.Is(err, session.ErrConcurrentGeneration) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "concurrent_generation"})
				return
			}
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
		userInput := c.Query("input")
		if userInput == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "input required"})
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")

		tokenCh, metaCh, errCh := engine.StreamTurn(c.Request.Context(), TurnRequest{
			SessionID: c.Param("id"),
			UserInput: userInput,
			APIKey:    c.Query("api_key"),
			BaseURL:   c.Query("base_url"),
			Model:     c.Query("model"),
		})

		c.Stream(func(w io.Writer) bool {
			select {
			case token, ok := <-tokenCh:
				if !ok {
					// tokenCh 关闭：读取元数据并推送结束事件
					if meta, ok2 := <-metaCh; ok2 {
						if b, err := json.Marshal(meta); err == nil {
							c.SSEvent("meta", string(b))
						}
					}
					c.SSEvent("done", "")
					return false
				}
				c.SSEvent("token", token)
				return true
			case err, ok := <-errCh:
				// errCh 被 close 时（ok=false, err=nil）不视为错误，继续等 tokenCh
				if !ok {
					return true
				}
				if err != nil {
					if errors.Is(err, session.ErrConcurrentGeneration) {
						c.SSEvent("error", `{"code":"concurrent_generation","message":"`+err.Error()+`"}`)
					} else {
						c.SSEvent("error", err.Error())
					}
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
	// ?from=&to= 可选范围过滤（floor seq，用于游记剪辑）
	play.GET("/sessions/:id/floors", func(c *gin.Context) {
		floors, err := engine.ListFloors(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// 范围过滤（可选）
		if fromStr := c.Query("from"); fromStr != "" {
			from, _ := strconv.Atoi(fromStr)
			to, _ := strconv.Atoi(c.DefaultQuery("to", "999999"))
			filtered := floors[:0]
			for _, f := range floors {
				if f.Seq >= from && f.Seq <= to {
					filtered = append(filtered, f)
				}
			}
			floors = filtered
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": floors})
	})

	// GET /play/games/worldbook/:id — 玩家只读世界书（创作者开放后可见）
	play.GET("/games/worldbook/:id", func(c *gin.Context) {
		var tmpl dbmodels.GameTemplate
		if err := engine.db.First(&tmpl, "id = ? AND status = 'published'", c.Param("id")).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
			return
		}
		var cfg map[string]any
		if err := json.Unmarshal(tmpl.Config, &cfg); err != nil || cfg["allow_player_worldbook_view"] != true {
			c.JSON(http.StatusForbidden, gin.H{"error": "worldbook not public"})
			return
		}
		var entries []struct {
			ID      string `json:"id"`
			Keys    any    `json:"keys"`
			Content string `json:"content"`
			Comment string `json:"comment"`
		}
		engine.db.Model(&dbmodels.WorldbookEntry{}).
			Select("id, keys, content, comment").
			Where("game_id = ? AND enabled = true", c.Param("id")).
			Order("priority ASC").
			Find(&entries)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
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

	// POST /sessions/:id/fork — 从指定楼层分叉出新会话（平行时间线 / 存档点）
	// Body: { "from_floor_seq": 5 }（省略 = 复制全部楼层）
	play.POST("/sessions/:id/fork", func(c *gin.Context) {
		var req ForkSessionReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		newSessID, err := engine.ForkSession(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"session_id": newSessID}})
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

	// GET /sessions/:id/floors/:fid/snapshot — Prompt 快照（Verifier 结果 + 命中词条）
	play.GET("/sessions/:id/floors/:fid/snapshot", func(c *gin.Context) {
		var snap dbmodels.PromptSnapshot
		if err := engine.db.Where("floor_id = ?", c.Param("fid")).First(&snap).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "snapshot not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": snap})
	})

	// GET /sessions/:id/tool-executions — 查询工具执行记录
	play.GET("/sessions/:id/tool-executions", func(c *gin.Context) {
		query := engine.db.Model(&dbmodels.ToolExecutionRecord{}).
			Where("session_id = ?", c.Param("id")).
			Order("created_at DESC")
		if floorID := c.Query("floor_id"); floorID != "" {
			query = query.Where("floor_id = ?", floorID)
		}
		limit := 50
		if l := c.Query("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		var records []dbmodels.ToolExecutionRecord
		query.Limit(limit).Find(&records)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": records})
	})

	// ── Memory Edge 路由 ────────────────────────────────────────────────────────
	// 记忆关系边（不参与 Prompt 注入，用于溯源和 Memory Lint）

	// GET /sessions/:id/memory-edges — 列出会话的所有边（?relation=updates|contradicts|supports|resolves&limit=&offset=）
	play.GET("/sessions/:id/memory-edges", func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		relation := dbmodels.MemoryRelation(c.Query("relation"))
		edges, err := engine.memStore.ListEdgesBySession(c.Param("id"), relation, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": edges})
	})

	// GET /sessions/:id/memories/:mid/edges — 列出某条记忆的所有双向边
	play.GET("/sessions/:id/memories/:mid/edges", func(c *gin.Context) {
		edges, err := engine.memStore.ListEdges(c.Param("id"), c.Param("mid"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": edges})
	})

	// POST /sessions/:id/memory-edges — 手动创建关系边（创作者调试用）
	// Body: { "from_id": "uuid", "to_id": "uuid", "relation": "contradicts" }
	play.POST("/sessions/:id/memory-edges", func(c *gin.Context) {
		var req struct {
			FromID   string                  `json:"from_id"  binding:"required"`
			ToID     string                  `json:"to_id"    binding:"required"`
			Relation dbmodels.MemoryRelation `json:"relation" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		edge, err := engine.memStore.SaveEdge(c.Param("id"), req.FromID, req.ToID, req.Relation)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": edge})
	})

	// PATCH /sessions/:id/memory-edges/:eid — 修改 relation 类型（标错时调试用）
	// Body: { "relation": "supports" }
	play.PATCH("/sessions/:id/memory-edges/:eid", func(c *gin.Context) {
		var req struct {
			Relation dbmodels.MemoryRelation `json:"relation" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		edge, err := engine.memStore.UpdateEdgeRelation(c.Param("id"), c.Param("eid"), req.Relation)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": edge})
	})

	// DELETE /sessions/:id/memory-edges/:eid — 删除关系边
	play.DELETE("/sessions/:id/memory-edges/:eid", func(c *gin.Context) {
		if err := engine.memStore.DeleteEdge(c.Param("id"), c.Param("eid")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": true}})
	})

	// ── Session 内分支（P-3G）──────────────────────────────────────────────────────

	// GET /sessions/:id/branches — 列出所有分支（含 main）
	play.GET("/sessions/:id/branches", func(c *gin.Context) {
		branches, err := engine.ListBranches(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": branches})
	})

	// POST /sessions/:id/floors/:fid/branch — 从指定楼层创建新分支
	play.POST("/sessions/:id/floors/:fid/branch", func(c *gin.Context) {
		branchID, err := engine.CreateBranch(c.Request.Context(), c.Param("id"), c.Param("fid"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"branch_id": branchID}})
	})

	// DELETE /sessions/:id/branches/:bid — 删除分支（不能删 main）
	play.DELETE("/sessions/:id/branches/:bid", func(c *gin.Context) {
		if err := engine.DeleteBranch(c.Request.Context(), c.Param("id"), c.Param("bid")); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": true}})
	})
}
