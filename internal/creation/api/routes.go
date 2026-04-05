package creation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/creation/card"
	"mvu-backend/internal/platform/auth"
)

// RegisterCreationRoutes 注册创作工具接口（/api/v2/create/...）
func RegisterCreationRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	create := rg.Group("/create")

	// ── 角色卡接口 ────────────────────────────────────────────

	// POST /api/v2/create/cards/import — 导入角色卡 PNG
	create.POST("/cards/import", func(c *gin.Context) {
		file, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
			return
		}
		f, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer f.Close()

		parsed, err := card.ParsePNG(f)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		rawJSON, _ := json.Marshal(parsed.RawData)
		tagsJSON, _ := json.Marshal(parsed.Tags)

		cc := dbmodels.CharacterCard{
			Slug:      slugify(parsed.Name),
			Name:      parsed.Name,
			Spec:      parsed.Spec,
			Data:      rawJSON,
			Tags:      tagsJSON,
			IsPublic:  true,
		}
		if err := db.Where("slug = ?", cc.Slug).FirstOrCreate(&cc).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// 同步导入内嵌世界书词条
		if parsed.CharacterBook != nil {
			for _, entry := range parsed.CharacterBook.Entries {
				keysJSON, _ := json.Marshal(entry.Keys)
				wb := dbmodels.WorldbookEntry{
					GameID:   cc.ID, // 暂用 CharacterCard.ID 作为 GameID
					Keys:     keysJSON,
					Content:  entry.Content,
					Constant: entry.Constant,
					Priority: entry.Priority,
					Enabled:  entry.Enabled,
					Comment:  entry.Comment,
				}
				db.Create(&wb)
			}
		}

		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"id":       cc.ID,
			"slug":     cc.Slug,
			"name":     cc.Name,
			"spec":     cc.Spec,
			"lorebook_entries": func() int {
				if parsed.CharacterBook != nil {
					return len(parsed.CharacterBook.Entries)
				}
				return 0
			}(),
		}})
	})

	// GET /api/v2/create/cards — 角色卡列表
	create.GET("/cards", func(c *gin.Context) {
		var cards []dbmodels.CharacterCard
		db.Select("id, slug, name, spec, tags, avatar_url, created_at").
			Where("is_public = true").
			Order("created_at DESC").
			Limit(50).Find(&cards)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cards})
	})

	// GET /api/v2/create/cards/:slug — 角色卡详情
	create.GET("/cards/:slug", func(c *gin.Context) {
		var cc dbmodels.CharacterCard
		if err := db.Where("slug = ?", c.Param("slug")).First(&cc).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cc})
	})

	// ── 世界书接口 ────────────────────────────────────────────

	// GET /api/v2/create/templates/:id/lorebook — 获取游戏世界书词条
	create.GET("/templates/:id/lorebook", func(c *gin.Context) {
		var entries []dbmodels.WorldbookEntry
		db.Where("game_id = ?", c.Param("id")).Order("priority DESC").Find(&entries)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
	})

	// POST /api/v2/create/templates/:id/lorebook — 新增/更新词条
	create.POST("/templates/:id/lorebook", func(c *gin.Context) {
		var entry dbmodels.WorldbookEntry
		if err := c.ShouldBindJSON(&entry); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		entry.GameID = c.Param("id")
		if entry.ID != "" {
			db.Save(&entry)
		} else {
			db.Create(&entry)
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entry})
	})

	// DELETE /api/v2/create/templates/:id/lorebook/:eid — 删除词条
	create.DELETE("/templates/:id/lorebook/:eid", func(c *gin.Context) {
		db.Where("id = ? AND game_id = ?", c.Param("eid"), c.Param("id")).
			Delete(&dbmodels.WorldbookEntry{})
		c.JSON(http.StatusOK, gin.H{"code": 0})
	})

	// ── 游戏模板接口 ──────────────────────────────────────────

	// GET /api/v2/create/templates — 模板列表
	create.GET("/templates", func(c *gin.Context) {
		status := c.DefaultQuery("status", "published")
		query := db.Order("created_at DESC")
		if status != "all" {
			query = query.Where("status = ?", status)
		}
		var templates []dbmodels.GameTemplate
		query.Find(&templates)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": templates})
	})

	// POST /api/v2/create/templates — 创建模板
	create.POST("/templates", func(c *gin.Context) {
		var tmpl dbmodels.GameTemplate
		if err := c.ShouldBindJSON(&tmpl); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if tmpl.Slug == "" {
			tmpl.Slug = slugify(tmpl.Title)
		}
		if err := db.Create(&tmpl).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": tmpl})
	})

	// PATCH /api/v2/create/templates/:id — 更新模板
	create.PATCH("/templates/:id", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// 防止覆盖主键/slug
		delete(updates, "id")
		if err := db.Model(&dbmodels.GameTemplate{}).Where("id = ?", c.Param("id")).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var tmpl dbmodels.GameTemplate
		db.First(&tmpl, "id = ?", c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": tmpl})
	})

	// DELETE /api/v2/create/templates/:id — 删除模板
	create.DELETE("/templates/:id", func(c *gin.Context) {
		if err := db.Where("id = ?", c.Param("id")).Delete(&dbmodels.GameTemplate{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": c.Param("id"), "deleted": true}})
	})

	// DELETE /api/v2/create/cards/:slug — 删除角色卡
	create.DELETE("/cards/:slug", func(c *gin.Context) {
		if err := db.Where("slug = ?", c.Param("slug")).Delete(&dbmodels.CharacterCard{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"slug": c.Param("slug"), "deleted": true}})
	})

	// ── LLM 配置接口（复刻 TH /llm-profiles）────────────────

	registerLLMProfileRoutes(create, db)
	registerPresetEntryRoutes(create, db)
	registerRegexProfileRoutes(create, db)
	registerMaterialRoutes(create, db)
	registerPresetToolRoutes(create, db)
}

// registerLLMProfileRoutes 注册 LLM 配置 CRUD 路由（/api/v2/create/llm-profiles/...）
func registerLLMProfileRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	lp := rg.Group("/llm-profiles")

	// POST — 创建配置
	lp.POST("", func(c *gin.Context) {
		var body struct {
			Name     string         `json:"name"     binding:"required"`
			Provider string         `json:"provider"`
			ModelID  string         `json:"model_id" binding:"required"`
			BaseURL  string         `json:"base_url"`
			APIKey   string         `json:"api_key"  binding:"required"`
			Params   map[string]any `json:"params"` // 采样参数（temperature/top_p/top_k/…）
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if body.Provider == "" {
			body.Provider = "openai-compatible"
		}
		paramsJSON, _ := json.Marshal(body.Params)
		profile := dbmodels.LLMProfile{
			AccountID: auth.GetAccountID(c),
			Name:      body.Name,
			Provider:  body.Provider,
			ModelID:   body.ModelID,
			BaseURL:   body.BaseURL,
			APIKey:    body.APIKey,
			Params:    paramsJSON,
			Status:    "active",
		}
		if err := db.Create(&profile).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": toProfileResp(profile)})
	})

	// GET — 列出当前账户的配置
	lp.GET("", func(c *gin.Context) {
		accountID := auth.GetAccountID(c)
		var profiles []dbmodels.LLMProfile
		db.Where("account_id = ? AND status != 'deleted'", accountID).
			Order("created_at DESC").Find(&profiles)
		resp := make([]gin.H, 0, len(profiles))
		for _, p := range profiles {
			resp = append(resp, toProfileResp(p))
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": resp})
	})

	// GET /:id — 获取单个配置（仅限当前账户）
	lp.GET("/:id", func(c *gin.Context) {
		var profile dbmodels.LLMProfile
		if err := db.First(&profile, "id = ? AND account_id = ?", c.Param("id"), auth.GetAccountID(c)).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": toProfileResp(profile)})
	})

	// PATCH /:id — 更新配置（仅限当前账户）
	lp.PATCH("/:id", func(c *gin.Context) {
		var body struct {
			Name    string         `json:"name"`
			ModelID string         `json:"model_id"`
			BaseURL string         `json:"base_url"`
			APIKey  string         `json:"api_key"`
			Status  string         `json:"status"`
			Params  map[string]any `json:"params"` // 采样参数（增量合并，传 null 某字段可清除）
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates := map[string]any{"updated_at": time.Now()}
		if body.Name != "" {
			updates["name"] = body.Name
		}
		if body.ModelID != "" {
			updates["model_id"] = body.ModelID
		}
		if body.BaseURL != "" {
			updates["base_url"] = body.BaseURL
		}
		if body.APIKey != "" {
			updates["api_key"] = body.APIKey
		}
		if body.Status == "active" || body.Status == "disabled" {
			updates["status"] = body.Status
		}
		if body.Params != nil {
			paramsJSON, _ := json.Marshal(body.Params)
			updates["params"] = paramsJSON
		}
		if err := db.Model(&dbmodels.LLMProfile{}).
			Where("id = ? AND account_id = ?", c.Param("id"), auth.GetAccountID(c)).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var profile dbmodels.LLMProfile
		db.First(&profile, "id = ?", c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": toProfileResp(profile)})
	})

	// DELETE /:id — 软删除（仅限当前账户）
	lp.DELETE("/:id", func(c *gin.Context) {
		if err := db.Model(&dbmodels.LLMProfile{}).
			Where("id = ? AND account_id = ?", c.Param("id"), auth.GetAccountID(c)).
			Update("status", "deleted").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": c.Param("id"), "deleted": true}})
	})

	// POST /:id/activate — 绑定配置到指定作用域/插槽
	lp.POST("/:id/activate", func(c *gin.Context) {
		var body struct {
			Scope   string         `json:"scope"`    // global | session（默认 global）
			ScopeID string         `json:"scope_id"` // session UUID 或留空（global 时自动填 "global"）
			Slot    string         `json:"slot"`     // * | narrator | memory（默认 *）
			Params  map[string]any `json:"params"`   // 在 profile params 之上额外覆盖的采样参数
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		accountID := auth.GetAccountID(c)
		if body.Scope == "" {
			body.Scope = "global"
		}
		if body.ScopeID == "" {
			body.ScopeID = "global"
		}
		if body.Slot == "" {
			body.Slot = "*"
		}

		// 确认 profile 属于当前账户
		var profile dbmodels.LLMProfile
		if err := db.First(&profile, "id = ? AND account_id = ?", c.Param("id"), accountID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}

		paramsJSON, _ := json.Marshal(body.Params)

		// Upsert binding（同一 account+scope+scopeID+slot 唯一）
		binding := dbmodels.LLMProfileBinding{
			ID:        uuid.New().String(),
			AccountID: accountID,
			ProfileID: profile.ID,
			Scope:     body.Scope,
			ScopeID:   body.ScopeID,
			Slot:      body.Slot,
			Params:    paramsJSON,
		}
		db.Where("account_id = ? AND scope = ? AND scope_id = ? AND slot = ?",
			accountID, body.Scope, body.ScopeID, body.Slot).
			Assign(binding).FirstOrCreate(&binding)

		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"profile_id": profile.ID,
			"scope":      body.Scope,
			"scope_id":   body.ScopeID,
			"slot":       body.Slot,
			"params":     body.Params,
			"activated":  true,
		}})
	})
}

// toProfileResp 将 LLMProfile 转为对外响应（隐藏明文 APIKey，只返回掩码）
func toProfileResp(p dbmodels.LLMProfile) gin.H {
	// 将 Params JSONB 反序列化为 map，方便前端读取
	var params map[string]any
	_ = json.Unmarshal(p.Params, &params)
	if params == nil {
		params = map[string]any{}
	}
	return gin.H{
		"id":             p.ID,
		"account_id":     p.AccountID,
		"name":           p.Name,
		"provider":       p.Provider,
		"model_id":       p.ModelID,
		"base_url":       p.BaseURL,
		"api_key_masked": maskAPIKey(p.APIKey),
		"params":         params,
		"status":         p.Status,
		"is_global":      p.IsGlobal,
		"created_at":     p.CreatedAt,
		"updated_at":     p.UpdatedAt,
	}
}

// maskAPIKey 返回 API Key 的掩码（保留后 4 位）
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return strings.Repeat("*", len(key)-4) + key[len(key)-4:]
}

// ── registerPresetEntryRoutes ─────────────────────────────────────────────────

// registerPresetEntryRoutes 注册 Preset Entry CRUD 路由
// （/api/v2/create/templates/:id/preset-entries/...）
//
// # Preset Entry — 条目化 Prompt 组装
//
// 替代单一 SystemPromptTemplate，允许创作者将系统 Prompt 拆分为
// 多条有名称、有顺序的独立条目，引擎按 injection_order 排序后合并注入。
//
// injection_position 仅供前端 UI 分组展示（top|system|bottom），
// 真正决定注入顺序的是 injection_order（直接映射为 PromptBlock.Priority）。
func registerPresetEntryRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	pe := rg.Group("/templates/:id/preset-entries")

	// GET — 列出所有条目（按 injection_order 升序）
	pe.GET("", func(c *gin.Context) {
		var entries []dbmodels.PresetEntry
		query := db.Where("game_id = ?", c.Param("id")).Order("injection_order ASC")
		// ?enabled=true/false 过滤
		if enabled := c.Query("enabled"); enabled == "true" {
			query = query.Where("enabled = true")
		} else if enabled == "false" {
			query = query.Where("enabled = false")
		}
		query.Find(&entries)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
	})

	// POST — 新建条目
	pe.POST("", func(c *gin.Context) {
		var body struct {
			Identifier        string `json:"identifier"         binding:"required"`
			Name              string `json:"name"               binding:"required"`
			Role              string `json:"role"`               // 默认 system
			Content           string `json:"content"`
			InjectionPosition string `json:"injection_position"` // 默认 system
			InjectionOrder    *int   `json:"injection_order"`    // nil = 1000
			Enabled           *bool  `json:"enabled"`            // nil = true
			IsSystemPrompt    bool   `json:"is_system_prompt"`
			Comment           string `json:"comment"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		entry := dbmodels.PresetEntry{
			GameID:            c.Param("id"),
			Identifier:        body.Identifier,
			Name:              body.Name,
			Role:              orStr(body.Role, "system"),
			Content:           body.Content,
			InjectionPosition: orStr(body.InjectionPosition, "system"),
			InjectionOrder:    orInt(body.InjectionOrder, 1000),
			Enabled:           orBool(body.Enabled, true),
			IsSystemPrompt:    body.IsSystemPrompt,
			Comment:           body.Comment,
		}
		// identifier 在 game 内唯一
		if err := db.Where("game_id = ? AND identifier = ?", entry.GameID, entry.Identifier).
			FirstOrCreate(&entry).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entry})
	})

	// PATCH /:identifier — 更新条目（部分更新）
	pe.PATCH("/:eid", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// 禁止覆盖主键和 game_id
		delete(updates, "id")
		delete(updates, "game_id")
		delete(updates, "identifier")
		if err := db.Model(&dbmodels.PresetEntry{}).
			Where("game_id = ? AND identifier = ?", c.Param("id"), c.Param("eid")).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var entry dbmodels.PresetEntry
		db.Where("game_id = ? AND identifier = ?", c.Param("id"), c.Param("eid")).First(&entry)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entry})
	})

	// DELETE /:identifier — 删除条目
	pe.DELETE("/:eid", func(c *gin.Context) {
		if err := db.Where("game_id = ? AND identifier = ?", c.Param("id"), c.Param("eid")).
			Delete(&dbmodels.PresetEntry{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"identifier": c.Param("eid"), "deleted": true}})
	})

	// PUT /reorder — 批量调整 injection_order（传入 [{identifier, injection_order}] 数组）
	pe.PUT("/reorder", func(c *gin.Context) {
		var items []struct {
			Identifier     string `json:"identifier"      binding:"required"`
			InjectionOrder int    `json:"injection_order"`
		}
		if err := c.ShouldBindJSON(&items); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		gameID := c.Param("id")
		for _, item := range items {
			db.Model(&dbmodels.PresetEntry{}).
				Where("game_id = ? AND identifier = ?", gameID, item.Identifier).
				Update("injection_order", item.InjectionOrder)
		}
		// 返回更新后的完整列表
		var entries []dbmodels.PresetEntry
		db.Where("game_id = ?", gameID).Order("injection_order ASC").Find(&entries)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
	})
}

// registerPresetToolRoutes 注册 Preset Tool CRUD 路由（/api/v2/create/templates/:id/tools）
func registerPresetToolRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	pt := rg.Group("/templates/:id/tools")

	pt.GET("", func(c *gin.Context) {
		var list []dbmodels.PresetTool
		db.Where("game_id = ?", c.Param("id")).Order("created_at ASC").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})

	pt.POST("", func(c *gin.Context) {
		var body struct {
			Name        string          `json:"name"        binding:"required"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
			Endpoint    string          `json:"endpoint"    binding:"required"`
			TimeoutMs   int             `json:"timeout_ms"`
			Enabled     *bool           `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		params := body.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tool := dbmodels.PresetTool{
			GameID:      c.Param("id"),
			Name:        body.Name,
			Description: body.Description,
			Parameters:  datatypes.JSON(params),
			Endpoint:    body.Endpoint,
			TimeoutMs:   orInt(&body.TimeoutMs, 5000),
			Enabled:     body.Enabled == nil || *body.Enabled,
		}
		if err := db.Create(&tool).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": tool})
	})

	pt.PATCH("/:tid", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		delete(updates, "id")
		delete(updates, "game_id")
		if err := db.Model(&dbmodels.PresetTool{}).
			Where("id = ? AND game_id = ?", c.Param("tid"), c.Param("id")).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var tool dbmodels.PresetTool
		db.Where("id = ? AND game_id = ?", c.Param("tid"), c.Param("id")).First(&tool)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": tool})
	})

	pt.DELETE("/:tid", func(c *gin.Context) {
		if err := db.Where("id = ? AND game_id = ?", c.Param("tid"), c.Param("id")).
			Delete(&dbmodels.PresetTool{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": c.Param("tid"), "deleted": true}})
	})
}

// ── 辅助函数 ──────────────────────────────────────────────────────────────────

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func orInt(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

func orBool(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// registerRegexProfileRoutes 注册 Regex Profile CRUD 路由
// （/api/v2/create/templates/:id/regex-profiles/ 和 /regex-profiles/:pid/rules/）
func registerRegexProfileRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	rp := rg.Group("/templates/:id/regex-profiles")

	// GET — 列出该游戏的 Regex Profile
	rp.GET("", func(c *gin.Context) {
		var profiles []dbmodels.RegexProfile
		db.Where("game_id = ?", c.Param("id")).Order("created_at ASC").Find(&profiles)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": profiles})
	})

	// POST — 创建 Profile
	rp.POST("", func(c *gin.Context) {
		var body struct {
			Name    string `json:"name" binding:"required"`
			Enabled *bool  `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		profile := dbmodels.RegexProfile{
			GameID:  c.Param("id"),
			Name:    body.Name,
			Enabled: orBool(body.Enabled, true),
		}
		if err := db.Create(&profile).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": profile})
	})

	// PATCH /:pid — 更新 Profile
	rp.PATCH("/:pid", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		delete(updates, "id")
		delete(updates, "game_id")
		if err := db.Model(&dbmodels.RegexProfile{}).
			Where("id = ? AND game_id = ?", c.Param("pid"), c.Param("id")).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var profile dbmodels.RegexProfile
		db.First(&profile, "id = ?", c.Param("pid"))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": profile})
	})

	// DELETE /:pid — 删除 Profile 及其规则
	rp.DELETE("/:pid", func(c *gin.Context) {
		db.Where("profile_id = ?", c.Param("pid")).Delete(&dbmodels.RegexRule{})
		db.Where("id = ? AND game_id = ?", c.Param("pid"), c.Param("id")).Delete(&dbmodels.RegexProfile{})
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": c.Param("pid"), "deleted": true}})
	})

	// ── Rules 子路由（/regex-profiles/:pid/rules）─────────────
	rules := rg.Group("/regex-profiles/:pid/rules")

	// GET — 列出规则（按 order 升序）
	rules.GET("", func(c *gin.Context) {
		var list []dbmodels.RegexRule
		db.Where("profile_id = ?", c.Param("pid")).Order("\"order\" ASC").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})

	// POST — 新建规则
	rules.POST("", func(c *gin.Context) {
		var body struct {
			Name        string `json:"name"`
			Pattern     string `json:"pattern"  binding:"required"`
			Replacement string `json:"replacement"`
			ApplyTo     string `json:"apply_to"`
			Order       *int   `json:"order"`
			Enabled     *bool  `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		rule := dbmodels.RegexRule{
			ProfileID:   c.Param("pid"),
			Name:        body.Name,
			Pattern:     body.Pattern,
			Replacement: body.Replacement,
			ApplyTo:     orStr(body.ApplyTo, "ai_output"),
			Order:       orInt(body.Order, 0),
			Enabled:     orBool(body.Enabled, true),
		}
		if err := db.Create(&rule).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": rule})
	})

	// PATCH /:rid — 部分更新规则
	rules.PATCH("/:rid", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		delete(updates, "id")
		delete(updates, "profile_id")
		if err := db.Model(&dbmodels.RegexRule{}).
			Where("id = ? AND profile_id = ?", c.Param("rid"), c.Param("pid")).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var rule dbmodels.RegexRule
		db.First(&rule, "id = ?", c.Param("rid"))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": rule})
	})

	// DELETE /:rid — 删除规则
	rules.DELETE("/:rid", func(c *gin.Context) {
		db.Where("id = ? AND profile_id = ?", c.Param("rid"), c.Param("pid")).
			Delete(&dbmodels.RegexRule{})
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": c.Param("rid"), "deleted": true}})
	})

	// PUT /reorder — 批量调整 order
	rules.PUT("/reorder", func(c *gin.Context) {
		var items []struct {
			ID    string `json:"id"    binding:"required"`
			Order int    `json:"order"`
		}
		if err := c.ShouldBindJSON(&items); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for _, item := range items {
			db.Model(&dbmodels.RegexRule{}).
				Where("id = ? AND profile_id = ?", item.ID, c.Param("pid")).
				Update("order", item.Order)
		}
		var list []dbmodels.RegexRule
		db.Where("profile_id = ?", c.Param("pid")).Order("\"order\" ASC").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})
}

// registerMaterialRoutes 注册素材库接口（/templates/:id/materials/...）
func registerMaterialRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	mat := rg.Group("/templates/:id/materials")

	// POST — 新建素材条目
	mat.POST("", func(c *gin.Context) {
		var m dbmodels.Material
		if err := c.ShouldBindJSON(&m); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		m.GameID = c.Param("id")
		if m.Tags == nil {
			m.Tags, _ = json.Marshal([]string{})
		}
		if m.WorldTags == nil {
			m.WorldTags, _ = json.Marshal([]string{})
		}
		if err := db.Create(&m).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": m})
	})

	// GET — 列出素材（?type=&mood=&style=&function_tag=&enabled=&limit=&offset=）
	mat.GET("", func(c *gin.Context) {
		q := db.Where("game_id = ?", c.Param("id"))
		if t := c.Query("type"); t != "" {
			q = q.Where("type = ?", t)
		}
		if mood := c.Query("mood"); mood != "" {
			q = q.Where("mood = ?", mood)
		}
		if style := c.Query("style"); style != "" {
			q = q.Where("style = ?", style)
		}
		if ft := c.Query("function_tag"); ft != "" {
			q = q.Where("function_tag = ?", ft)
		}
		if c.Query("enabled") == "true" {
			q = q.Where("enabled = true")
		} else if c.Query("enabled") == "false" {
			q = q.Where("enabled = false")
		}
		limit := 50
		offset := 0
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		if v := c.Query("offset"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		var list []dbmodels.Material
		q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})

	// PATCH /:mid — 更新素材字段
	mat.PATCH("/:mid", func(c *gin.Context) {
		var updates map[string]any
		if err := c.ShouldBindJSON(&updates); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		delete(updates, "game_id")
		delete(updates, "id")
		if err := db.Model(&dbmodels.Material{}).
			Where("id = ? AND game_id = ?", c.Param("mid"), c.Param("id")).
			Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var m dbmodels.Material
		db.First(&m, "id = ?", c.Param("mid"))
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": m})
	})

	// DELETE /:mid — 删除素材
	mat.DELETE("/:mid", func(c *gin.Context) {
		if err := db.Where("id = ? AND game_id = ?", c.Param("mid"), c.Param("id")).
			Delete(&dbmodels.Material{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": true}})
	})

	// POST /batch — 批量导入素材（[]Material）
	mat.POST("/batch", func(c *gin.Context) {
		var items []dbmodels.Material
		if err := c.ShouldBindJSON(&items); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		gameID := c.Param("id")
		for i := range items {
			items[i].GameID = gameID
			if items[i].Tags == nil {
				items[i].Tags, _ = json.Marshal([]string{})
			}
			if items[i].WorldTags == nil {
				items[i].WorldTags, _ = json.Marshal([]string{})
			}
		}
		if err := db.Create(&items).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"created": len(items)}})
	})
}

// slugify 将名称转为 URL-safe slug（保留 ASCII 字母数字，中文用 uuid 兜底）
func slugify(name string) string {
	var result strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			result.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			result.WriteRune(unicode.ToLower(r))
		case r == ' ' || r == '-' || r == '_':
			result.WriteByte('-')
		}
		// 非 ASCII 字符（中文等）跳过
	}
	if result.Len() == 0 {
		return fmt.Sprintf("card-%s", uuid.New().String()[:8])
	}
	return result.String()
}
