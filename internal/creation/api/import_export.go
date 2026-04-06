package creation

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	dbmodels "mvu-backend/internal/core/db"
)

// registerImportExportRoutes 注册导入/导出路由
func registerImportExportRoutes(rg *gin.RouterGroup, db *gorm.DB) {

	// ── POST /templates/:id/preset/import-st ─────────────────────────────────
	// 接收 ST 预设 JSON，批量写入 PresetEntry
	rg.POST("/templates/:id/preset/import-st", func(c *gin.Context) {
		gameID := c.Param("id")
		var raw map[string]json.RawMessage
		if err := c.ShouldBindJSON(&raw); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 解析 prompts[]
		var prompts []struct {
			Identifier        string          `json:"identifier"`
			Name              string          `json:"name"`
			Role              string          `json:"role"`
			Content           string          `json:"content"`
			Marker            bool            `json:"marker"`
			SystemPrompt      bool            `json:"system_prompt"`
			InjectionPosition int             `json:"injection_position"`
			InjectionOrder    int             `json:"injection_order"`
			ForbidOverrides   bool            `json:"forbid_overrides"`
		}
		if err := json.Unmarshal(raw["prompts"], &prompts); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid prompts field"})
			return
		}

		// 解析 prompt_order，取 character_id=100000 的 order
		type orderEntry struct {
			Identifier string `json:"identifier"`
			Enabled    bool   `json:"enabled"`
		}
		type orderCtx struct {
			CharacterID int          `json:"character_id"`
			Order       []orderEntry `json:"order"`
		}
		var promptOrder []orderCtx
		if raw["prompt_order"] != nil {
			_ = json.Unmarshal(raw["prompt_order"], &promptOrder)
		}

		// 建立 identifier → {enabled, seq} 映射
		type orderMeta struct{ enabled bool; seq int }
		orderMap := map[string]orderMeta{}
		for _, ctx := range promptOrder {
			if ctx.CharacterID == 100000 || len(orderMap) == 0 {
				for i, o := range ctx.Order {
					orderMap[o.Identifier] = orderMeta{o.Enabled, i}
				}
				if ctx.CharacterID == 100000 {
					break
				}
			}
		}

		var entries []dbmodels.PresetEntry
		skipped := 0
		for _, p := range prompts {
			if p.Marker {
				skipped++
				continue
			}
			meta, hasOrder := orderMap[p.Identifier]
			injOrder := 1000
			enabled := true
			if hasOrder {
				injOrder = (meta.seq + 1) * 10
				enabled = meta.enabled
			}
			role := p.Role
			if role == "" {
				role = "system"
			}
			entries = append(entries, dbmodels.PresetEntry{
				GameID:            gameID,
				Identifier:        p.Identifier,
				Name:              p.Name,
				Role:              role,
				Content:           p.Content,
				InjectionPosition: "system",
				InjectionOrder:    injOrder,
				Enabled:           enabled,
				IsSystemPrompt:    p.SystemPrompt,
				Comment:           "imported from ST preset",
				CreatedAt:         time.Now(),
				UpdatedAt:         time.Now(),
			})
		}

		if len(entries) > 0 {
			if err := db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "game_id"}, {Name: "identifier"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "role", "content", "injection_order", "enabled", "is_system_prompt", "updated_at"}),
			}).Create(&entries).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"imported": len(entries),
			"skipped_markers": skipped,
		}})
	})

	// ── POST /templates/:id/lorebook/import-st ───────────────────────────────
	// 接收 ST 世界书 JSON，批量写入 WorldbookEntry
	rg.POST("/templates/:id/lorebook/import-st", func(c *gin.Context) {
		gameID := c.Param("id")
		// ST 世界书格式：{"entries": {"0": {...}, "1": {...}}}
		var raw struct {
			Entries map[string]struct {
				UID           int      `json:"uid"`
				Key           []string `json:"key"`
				SecondaryKeys []string `json:"secondary_keys"`
				Content       string   `json:"content"`
				Constant      bool     `json:"constant"`
				Disable       bool     `json:"disable"`
				Order         int      `json:"order"`
				Comment       string   `json:"comment"`
				Group         string   `json:"group"`
				GroupWeight   float64  `json:"groupWeight"` // ST 使用 camelCase
			} `json:"entries"`
		}
		if err := c.ShouldBindJSON(&raw); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var entries []dbmodels.WorldbookEntry
		for _, e := range raw.Entries {
			keys, _ := json.Marshal(e.Key)
			secKeys, _ := json.Marshal(e.SecondaryKeys)
			entries = append(entries, dbmodels.WorldbookEntry{
				GameID:        gameID,
				Keys:          datatypes.JSON(keys),
				SecondaryKeys: datatypes.JSON(secKeys),
				Content:       e.Content,
				Constant:      e.Constant,
				Priority:      e.Order,
				Enabled:       !e.Disable,
				Comment:       e.Comment,
				Group:         e.Group,
				GroupWeight:   e.GroupWeight,
				CreatedAt:     time.Now(),
			})
		}

		if len(entries) > 0 {
			if err := db.Create(&entries).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"imported": len(entries)}})
	})

	// ── GET /templates/:id/export ─────────────────────────────────────────────
	// 打包导出游戏（GameTemplate + 所有关联数据）
	rg.GET("/templates/:id/export", func(c *gin.Context) {
		gameID := c.Param("id")

		var tmpl dbmodels.GameTemplate
		if err := db.First(&tmpl, "id = ?", gameID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "template not found"})
			return
		}

		var presetEntries []dbmodels.PresetEntry
		db.Where("game_id = ?", gameID).Find(&presetEntries)

		var worldbookEntries []dbmodels.WorldbookEntry
		db.Where("game_id = ?", gameID).Find(&worldbookEntries)

		var regexProfiles []dbmodels.RegexProfile
		db.Where("game_id = ?", gameID).Find(&regexProfiles)
		var regexRules []dbmodels.RegexRule
		if len(regexProfiles) > 0 {
			ids := make([]string, len(regexProfiles))
			for i, p := range regexProfiles {
				ids[i] = p.ID
			}
			db.Where("profile_id IN ?", ids).Find(&regexRules)
		}

		var materials []dbmodels.Material
		db.Where("game_id = ?", gameID).Find(&materials)

		var presetTools []dbmodels.PresetTool
		db.Where("game_id = ?", gameID).Find(&presetTools)

		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"version":          "1.0",
			"exported_at":      time.Now(),
			"template":         tmpl,
			"preset_entries":   presetEntries,
			"worldbook_entries": worldbookEntries,
			"regex_profiles":   regexProfiles,
			"regex_rules":      regexRules,
			"materials":        materials,
			"preset_tools":     presetTools,
		}})
	})

	// ── POST /templates/import ────────────────────────────────────────────────
	// 解包导入游戏（重建所有关联数据，新建 game_id 避免冲突）
	rg.POST("/templates/import", func(c *gin.Context) {
		var pkg struct {
			Version         string                    `json:"version"`
			Template        dbmodels.GameTemplate     `json:"template"`
			PresetEntries   []dbmodels.PresetEntry    `json:"preset_entries"`
			WorldbookEntries []dbmodels.WorldbookEntry `json:"worldbook_entries"`
			RegexProfiles   []dbmodels.RegexProfile   `json:"regex_profiles"`
			RegexRules      []dbmodels.RegexRule      `json:"regex_rules"`
			Materials       []dbmodels.Material       `json:"materials"`
			PresetTools     []dbmodels.PresetTool     `json:"preset_tools"`
		}
		if err := c.ShouldBindJSON(&pkg); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err := db.Transaction(func(tx *gorm.DB) error {
			// 新建模板（清空 ID，生成新 UUID）
			pkg.Template.ID = ""
			// 若 slug 冲突则加后缀
			slug := pkg.Template.Slug
			var count int64
			tx.Model(&dbmodels.GameTemplate{}).Where("slug = ?", slug).Count(&count)
			if count > 0 {
				pkg.Template.Slug = slug + "-imported-" + strings.ReplaceAll(time.Now().Format("20060102150405"), "", "")
			}
			pkg.Template.Status = "draft"
			if err := tx.Create(&pkg.Template).Error; err != nil {
				return err
			}
			newGameID := pkg.Template.ID

			// 重建 PresetEntry（新 game_id，清空 ID）
			for i := range pkg.PresetEntries {
				pkg.PresetEntries[i].ID = ""
				pkg.PresetEntries[i].GameID = newGameID
			}
			if len(pkg.PresetEntries) > 0 {
				if err := tx.Create(&pkg.PresetEntries).Error; err != nil {
					return err
				}
			}

			// 重建 WorldbookEntry
			for i := range pkg.WorldbookEntries {
				pkg.WorldbookEntries[i].ID = ""
				pkg.WorldbookEntries[i].GameID = newGameID
			}
			if len(pkg.WorldbookEntries) > 0 {
				if err := tx.Create(&pkg.WorldbookEntries).Error; err != nil {
					return err
				}
			}

			// 重建 RegexProfile + RegexRule（需要维护 profile_id 映射）
			profileIDMap := map[string]string{}
			for i := range pkg.RegexProfiles {
				oldID := pkg.RegexProfiles[i].ID
				pkg.RegexProfiles[i].ID = ""
				pkg.RegexProfiles[i].GameID = newGameID
				if err := tx.Create(&pkg.RegexProfiles[i]).Error; err != nil {
					return err
				}
				profileIDMap[oldID] = pkg.RegexProfiles[i].ID
			}
			for i := range pkg.RegexRules {
				pkg.RegexRules[i].ID = ""
				if newPID, ok := profileIDMap[pkg.RegexRules[i].ProfileID]; ok {
					pkg.RegexRules[i].ProfileID = newPID
				}
			}
			if len(pkg.RegexRules) > 0 {
				if err := tx.Create(&pkg.RegexRules).Error; err != nil {
					return err
				}
			}

			// 重建 Material
			for i := range pkg.Materials {
				pkg.Materials[i].ID = ""
				pkg.Materials[i].GameID = newGameID
			}
			if len(pkg.Materials) > 0 {
				if err := tx.Create(&pkg.Materials).Error; err != nil {
					return err
				}
			}

			// 重建 PresetTool
			for i := range pkg.PresetTools {
				pkg.PresetTools[i].ID = ""
				pkg.PresetTools[i].GameID = newGameID
			}
			if len(pkg.PresetTools) > 0 {
				if err := tx.Create(&pkg.PresetTools).Error; err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"template_id": pkg.Template.ID,
			"slug":        pkg.Template.Slug,
		}})
	})
}
