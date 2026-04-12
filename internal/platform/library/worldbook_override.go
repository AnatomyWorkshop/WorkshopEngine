package library

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	dbmodels "mvu-backend/internal/core/db"
)

// PlayerWorldbookOverride 玩家对游戏世界书词条的私有覆盖。
//
// 设计原则：
//   - 原始 WorldbookEntry 永远不被修改（可恢复性）
//   - 覆盖跟随 game+user 级别，不是 session 级别（多存档共享同一套修改）
//   - is_new=true 时 entry_id 为空，代表玩家完全新增的词条
//   - 字段为 NULL 时表示"使用原始值"（只覆盖传入的字段）
type PlayerWorldbookOverride struct {
	ID      string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID  string `gorm:"not null;index"                                json:"game_id"`
	UserID  string `gorm:"not null;index"                                json:"user_id"`
	EntryID string `gorm:"not null;default:'';index"                     json:"entry_id"` // 空 = 玩家新增词条

	// 可覆盖字段（NULL = 使用原始值）
	Content        *string        `gorm:"type:text"    json:"content,omitempty"`
	Enabled        *bool          `json:"enabled,omitempty"`
	Keys           datatypes.JSON `gorm:"type:jsonb"   json:"keys,omitempty"`
	SecondaryKeys  datatypes.JSON `gorm:"type:jsonb"   json:"secondary_keys,omitempty"`
	SecondaryLogic *string        `json:"secondary_logic,omitempty"`
	Constant       *bool          `json:"constant,omitempty"`
	Priority       *int           `json:"priority,omitempty"`

	// is_new=true 时 EntryID 为空，这是玩家完全新增的词条
	IsNew bool `gorm:"default:false" json:"is_new"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func MigrateWorldbookOverride(db *gorm.DB) error {
	if err := db.AutoMigrate(&PlayerWorldbookOverride{}); err != nil {
		return err
	}
	// 原始词条覆盖：同一 game+user+entry 只能有一条（entry_id 非空时唯一）
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_wb_override_unique
		ON player_worldbook_overrides(game_id, user_id, entry_id)
		WHERE entry_id != ''`).Error
}

// RegisterWorldbookOverrideRoutes 注册玩家世界书覆盖 API。
//
// 路由前缀：/users/:uid/library/:game_id/worldbook
//
//	GET    /                    — 读取合并后的完整词条列表
//	PATCH  /:entry_id           — upsert 覆盖（只更新传入字段）
//	POST   /                    — 新增玩家私有词条（is_new=true）
//	DELETE /:entry_id           — 恢复原始（删除 override）或删除 is_new 词条
//	DELETE /                    — 重置所有覆盖
func RegisterWorldbookOverrideRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	// GET /users/:uid/library/:game_id/worldbook
	rg.GET("", func(c *gin.Context) {
		uid := c.Param("id")
		gameID := c.Param("game_id")

		if !selfOnlyUID(c, uid) {
			return
		}

		// 读取原始词条
		var origEntries []dbmodels.WorldbookEntry
		db.Where("game_id = ?", gameID).Find(&origEntries)

		// 读取玩家覆盖
		var overrides []PlayerWorldbookOverride
		db.Where("game_id = ? AND user_id = ?", gameID, uid).Find(&overrides)

		merged := mergeWorldbook(origEntries, overrides)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": merged})
	})

	// PATCH /users/:uid/library/:game_id/worldbook/:entry_id
	rg.PATCH("/:entry_id", func(c *gin.Context) {
		uid := c.Param("id")
		gameID := c.Param("game_id")
		entryID := c.Param("entry_id")

		if !selfOnlyUID(c, uid) {
			return
		}

		var body struct {
			Content        *string        `json:"content"`
			Enabled        *bool          `json:"enabled"`
			Keys           datatypes.JSON `json:"keys"`
			SecondaryKeys  datatypes.JSON `json:"secondary_keys"`
			SecondaryLogic *string        `json:"secondary_logic"`
			Constant       *bool          `json:"constant"`
			Priority       *int           `json:"priority"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		override := PlayerWorldbookOverride{
			GameID:         gameID,
			UserID:         uid,
			EntryID:        entryID,
			Content:        body.Content,
			Enabled:        body.Enabled,
			Keys:           body.Keys,
			SecondaryKeys:  body.SecondaryKeys,
			SecondaryLogic: body.SecondaryLogic,
			Constant:       body.Constant,
			Priority:       body.Priority,
		}

		// upsert：entry_id 存在则更新传入字段，不存在则插入
		result := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "game_id"}, {Name: "user_id"}, {Name: "entry_id"}},
			DoUpdates: clause.Assignments(buildUpdateMap(body.Content, body.Enabled,
				body.Keys, body.SecondaryKeys, body.SecondaryLogic, body.Constant, body.Priority)),
		}).Create(&override)

		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": override})
	})

	// POST /users/:uid/library/:game_id/worldbook — 新增玩家私有词条
	rg.POST("", func(c *gin.Context) {
		uid := c.Param("id")
		gameID := c.Param("game_id")

		if !selfOnlyUID(c, uid) {
			return
		}

		var body struct {
			Content        string         `json:"content" binding:"required"`
			Keys           datatypes.JSON `json:"keys"`
			SecondaryKeys  datatypes.JSON `json:"secondary_keys"`
			SecondaryLogic *string        `json:"secondary_logic"`
			Enabled        *bool          `json:"enabled"`
			Constant       *bool          `json:"constant"`
			Priority       *int           `json:"priority"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}

		override := PlayerWorldbookOverride{
			GameID:         gameID,
			UserID:         uid,
			EntryID:        "", // is_new 词条无 entry_id
			Content:        &body.Content,
			Enabled:        &enabled,
			Keys:           body.Keys,
			SecondaryKeys:  body.SecondaryKeys,
			SecondaryLogic: body.SecondaryLogic,
			Constant:       body.Constant,
			Priority:       body.Priority,
			IsNew:          true,
		}

		if err := db.Create(&override).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"code": 0, "data": override})
	})

	// DELETE /users/:uid/library/:game_id/worldbook/:entry_id
	rg.DELETE("/:entry_id", func(c *gin.Context) {
		uid := c.Param("id")
		gameID := c.Param("game_id")
		entryID := c.Param("entry_id")

		if !selfOnlyUID(c, uid) {
			return
		}

		// 删除对应的 override（无论是原始词条的覆盖还是 is_new 词条）
		result := db.Where("game_id = ? AND user_id = ? AND (entry_id = ? OR (is_new = true AND id = ?))",
			gameID, uid, entryID, entryID).
			Delete(&PlayerWorldbookOverride{})

		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": result.RowsAffected}})
	})

	// DELETE /users/:uid/library/:game_id/worldbook — 重置所有覆盖
	rg.DELETE("", func(c *gin.Context) {
		uid := c.Param("id")
		gameID := c.Param("game_id")

		if !selfOnlyUID(c, uid) {
			return
		}

		result := db.Where("game_id = ? AND user_id = ?", gameID, uid).
			Delete(&PlayerWorldbookOverride{})

		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"deleted": result.RowsAffected}})
	})
}

// MergedWorldbookEntry 合并后的词条（含覆盖状态标记）
type MergedWorldbookEntry struct {
	dbmodels.WorldbookEntry
	IsOverridden bool `json:"is_overridden"` // 是否有玩家覆盖
	IsNew        bool `json:"is_new"`        // 是否为玩家新增词条
}

// mergeWorldbook 将原始词条与玩家覆盖合并。
// 覆盖规则：override 中非 NULL 的字段覆盖原始值。
// is_new=true 的覆盖追加到列表末尾。
func mergeWorldbook(entries []dbmodels.WorldbookEntry, overrides []PlayerWorldbookOverride) []MergedWorldbookEntry {
	overrideMap := map[string]PlayerWorldbookOverride{}
	var newEntries []PlayerWorldbookOverride
	for _, o := range overrides {
		if o.IsNew {
			newEntries = append(newEntries, o)
		} else {
			overrideMap[o.EntryID] = o
		}
	}

	result := make([]MergedWorldbookEntry, 0, len(entries)+len(newEntries))

	for _, e := range entries {
		merged := MergedWorldbookEntry{WorldbookEntry: e}
		if o, ok := overrideMap[e.ID]; ok {
			merged.IsOverridden = true
			if o.Content != nil {
				merged.Content = *o.Content
			}
			if o.Enabled != nil {
				merged.Enabled = *o.Enabled
			}
			if len(o.Keys) > 0 {
				merged.Keys = o.Keys
			}
			if len(o.SecondaryKeys) > 0 {
				merged.SecondaryKeys = o.SecondaryKeys
			}
			if o.SecondaryLogic != nil {
				merged.SecondaryLogic = *o.SecondaryLogic
			}
			if o.Constant != nil {
				merged.Constant = *o.Constant
			}
			if o.Priority != nil {
				merged.Priority = *o.Priority
			}
		}
		result = append(result, merged)
	}

	// 追加玩家新增词条
	for _, o := range newEntries {
		var keys, secKeys []string
		_ = json.Unmarshal(o.Keys, &keys)
		_ = json.Unmarshal(o.SecondaryKeys, &secKeys)

		content := ""
		if o.Content != nil {
			content = *o.Content
		}
		enabled := true
		if o.Enabled != nil {
			enabled = *o.Enabled
		}
		constant := false
		if o.Constant != nil {
			constant = *o.Constant
		}
		priority := 0
		if o.Priority != nil {
			priority = *o.Priority
		}
		secLogic := "and_any"
		if o.SecondaryLogic != nil {
			secLogic = *o.SecondaryLogic
		}

		entry := dbmodels.WorldbookEntry{
			ID:             o.ID, // 用 override ID 作为词条 ID
			GameID:         o.GameID,
			Content:        content,
			Enabled:        enabled,
			Constant:       constant,
			Priority:       priority,
			SecondaryLogic: secLogic,
			Position:       "before_template",
			Probability:    100,
		}
		if len(keys) > 0 {
			entry.Keys = o.Keys
		}
		if len(secKeys) > 0 {
			entry.SecondaryKeys = o.SecondaryKeys
		}

		result = append(result, MergedWorldbookEntry{
			WorldbookEntry: entry,
			IsNew:          true,
		})
	}

	return result
}

// buildUpdateMap 构建 upsert 时只更新非 nil 字段的 map。
func buildUpdateMap(
	content *string, enabled *bool,
	keys, secKeys datatypes.JSON,
	secLogic *string, constant *bool, priority *int,
) map[string]interface{} {
	m := map[string]interface{}{"updated_at": time.Now()}
	if content != nil {
		m["content"] = *content
	}
	if enabled != nil {
		m["enabled"] = *enabled
	}
	if len(keys) > 0 {
		m["keys"] = keys
	}
	if len(secKeys) > 0 {
		m["secondary_keys"] = secKeys
	}
	if secLogic != nil {
		m["secondary_logic"] = *secLogic
	}
	if constant != nil {
		m["constant"] = *constant
	}
	if priority != nil {
		m["priority"] = *priority
	}
	return m
}
