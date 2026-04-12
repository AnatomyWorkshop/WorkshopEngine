package play

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/util"
	"mvu-backend/internal/social/comment"
)

// SessionCreator 由 engine.GameEngine 实现，platform/play 通过此接口调用，避免循环依赖。
type SessionCreator interface {
	CreateSession(ctx context.Context, gameID, userID string) (string, error)
}

// Handler 玩家发现层：游戏查询、存档列表、会话创建、worldbook、comment_config。
type Handler struct {
	db      *gorm.DB
	engine  SessionCreator
	comment *comment.Service
}

// NewHandler 创建 Handler，注入 DB、engine 接口和 comment 服务。
func NewHandler(db *gorm.DB, engine SessionCreator, commentSvc *comment.Service) *Handler {
	return &Handler{db: db, engine: engine, comment: commentSvc}
}

// listGames 已发布游戏列表
// @Summary     游戏列表
// @Description 分页查询已发布游戏（支持标签/类型/排序）
// @Tags        play-discovery
// @Produce     json
// @Param       tags   query string false "逗号分隔标签过滤"
// @Param       type   query string false "游戏类型"
// @Param       sort   query string false "排序：new（默认）| hot"
// @Param       limit  query int    false "每页数量" default(20)
// @Param       offset query int    false "偏移量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/games [get]
func (h *Handler) listGames(c *gin.Context) {
	limit, offset := util.ParsePage(c)

	query := h.db.Model(&dbmodels.GameTemplate{}).Where("status = 'published'")

	if tags := c.Query("tags"); tags != "" {
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				query = query.Where("config->'tags' ? ?", tag)
			}
		}
	}

	if t := c.Query("type"); t != "" {
		query = query.Where("type = ?", t)
	}

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
}

// getGame 游戏详情
// @Summary     游戏详情
// @Description 按 slug 或 UUID 查询已发布游戏详情（含 comment_config）
// @Tags        play-discovery
// @Produce     json
// @Param       slug path string true "游戏 slug 或 UUID"
// @Success     200 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/games/{slug} [get]
func (h *Handler) getGame(c *gin.Context) {
	slug := c.Param("slug")
	var tmpl dbmodels.GameTemplate
	err := h.db.Where("status = 'published' AND (slug = ? OR id::text = ?)", slug, slug).
		First(&tmpl).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}

	view := publicGameView(tmpl)

	// A-10：附加 comment_config（platform/play 可合法 import social/comment）
	cfg := h.comment.GetConfig(tmpl.ID)
	view["comment_config"] = map[string]any{
		"default_mode": cfg.DefaultMode,
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": view})
}

// getWorldbook 玩家只读世界书
// @Summary     玩家世界书
// @Description 查看已发布游戏的世界书词条（需游戏开启 allow_player_worldbook_view）
// @Tags        play-discovery
// @Produce     json
// @Param       id path string true "游戏 ID"
// @Success     200 {object} map[string]any
// @Failure     403 {object} map[string]any
// @Failure     404 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/games/worldbook/{id} [get]
func (h *Handler) getWorldbook(c *gin.Context) {
	var tmpl dbmodels.GameTemplate
	if err := h.db.First(&tmpl, "id = ? AND status = 'published'", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		return
	}
	var cfg map[string]any
	if err := json.Unmarshal(tmpl.Config, &cfg); err != nil || cfg["allow_player_worldbook_view"] != true {
		c.JSON(http.StatusForbidden, gin.H{"error": "worldbook not public"})
		return
	}
	var entries []struct {
		ID              string `json:"id"`
		Keys            any    `json:"keys"`
		Content         string `json:"content"`
		Comment         string `json:"comment"`
		Constant        bool   `json:"constant"`
		PlayerVisible   bool   `json:"player_visible"`
		DisplayCategory string `json:"display_category"`
	}
	h.db.Model(&dbmodels.WorldbookEntry{}).
		Select("id, keys, content, comment, constant, player_visible, display_category").
		Where("game_id = ? AND enabled = true", c.Param("id")).
		Order("priority ASC").
		Find(&entries)
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
}

// listSessions 会话列表
// @Summary     会话列表
// @Description 列出玩家会话（支持 game_id/user_id 过滤）
// @Tags        play-discovery
// @Produce     json
// @Param       game_id query string false "按游戏过滤"
// @Param       user_id query string false "按用户过滤"
// @Param       limit   query int    false "每页数量" default(20)
// @Param       offset  query int    false "偏移量"
// @Success     200 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions [get]
func (h *Handler) listSessions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := h.db.Model(&dbmodels.GameSession{}).Order("updated_at DESC").Limit(limit).Offset(offset)
	if gameID := c.Query("game_id"); gameID != "" {
		query = query.Where("game_id = ?", gameID)
	}
	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	var sessions []dbmodels.GameSession
	if err := query.Find(&sessions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": sessions})
}

// createSession 创建会话
// @Summary     创建会话
// @Description 为指定游戏创建新会话（自动注入 first_mes，递增 play_count）
// @Tags        play-discovery
// @Accept      json
// @Produce     json
// @Param       body body object{game_id=string,user_id=string} true "创建请求"
// @Success     200 {object} map[string]any "session_id"
// @Failure     400 {object} map[string]any
// @Security    BearerAuth
// @Router      /play/sessions [post]
func (h *Handler) createSession(c *gin.Context) {
	var req struct {
		GameID string `json:"game_id" binding:"required"`
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sessID, err := h.engine.CreateSession(c.Request.Context(), req.GameID, req.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.db.Model(&dbmodels.GameTemplate{}).Where("id = ?", req.GameID).
		UpdateColumn("play_count", gorm.Expr("play_count + 1"))
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"session_id": sessID}})
}

// publicGameView 构造玩家侧游戏视图，过滤创作者私有字段，提取 ui_config / tags 子字段。
func publicGameView(t dbmodels.GameTemplate) map[string]any {
	var uiConfig map[string]any
	var tags []any
	if len(t.Config) > 0 {
		var cfg map[string]any
		if json.Unmarshal(t.Config, &cfg) == nil {
			if v, ok := cfg["ui_config"]; ok {
				uiConfig, _ = v.(map[string]any)
			}
			if v, ok := cfg["tags"]; ok {
				tags, _ = v.([]any)
			}
		}
	}
	if tags == nil {
		tags = []any{}
	}
	return map[string]any{
		"id":             t.ID,
		"slug":           t.Slug,
		"title":          t.Title,
		"type":           t.Type,
		"short_desc":     t.ShortDesc,
		"notes":          t.Notes,
		"cover_url":      t.CoverURL,
		"author_id":      t.AuthorID,
		// author_name: 当前后端无 users 表，前端显示 author_id 或由前端自行 fallback。
		// 待 platform/user 包建立后补充 JOIN 查询。
		"play_count":     t.PlayCount,
		"like_count":     t.LikeCount,
		"favorite_count": t.FavoriteCount,
		"ui_config":      uiConfig,
		"tags":           tags,
		"created_at":     t.CreatedAt,
	}
}
