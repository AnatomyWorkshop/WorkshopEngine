package library

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mvu-backend/internal/platform/auth"
)

// RegisterLibraryRoutes 挂载个人游戏库接口到 /api/users/:id/library
//
// GET    /users/:id/library              — 列出个人游戏库（需登录，只能查自己）
// POST   /users/:id/library              — 导入游戏到个人库（幂等）
// DELETE /users/:id/library/:entry_id    — 从个人库移除
func RegisterLibraryRoutes(rg *gin.RouterGroup, db *gorm.DB) {
	g := rg.Group("/users/:id/library")

	g.GET("", func(c *gin.Context) {
		if !selfOnly(c) {
			return
		}
		var entries []LibraryEntry
		db.Where("user_id = ?", c.Param("id")).
			Order("created_at DESC").
			Find(&entries)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entries})
	})

	g.POST("", func(c *gin.Context) {
		if !selfOnly(c) {
			return
		}
		var req struct {
			GameID    string `json:"game_id" binding:"required"`
			SeriesKey string `json:"series_key"`
			Source    string `json:"source"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 幂等：已存在则直接返回
		var existing LibraryEntry
		err := db.Where("user_id = ? AND game_id = ?", c.Param("id"), req.GameID).
			First(&existing).Error
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": existing})
			return
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		source := req.Source
		if source == "" {
			source = "catalog"
		}
		seriesKey := req.SeriesKey
		if seriesKey == "" {
			seriesKey = req.GameID
		}
		now := time.Now()
		entry := LibraryEntry{
			UserID:       c.Param("id"),
			GameID:       req.GameID,
			SeriesKey:    seriesKey,
			Source:       source,
			LastPlayedAt: &now,
		}
		if err := db.Create(&entry).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": entry})
	})

	g.DELETE("/entries/:entry_id", func(c *gin.Context) {
		if !selfOnly(c) {
			return
		}
		result := db.Where("id = ? AND user_id = ?", c.Param("entry_id"), c.Param("id")).
			Delete(&LibraryEntry{})
		if result.RowsAffected == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "entry not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"ok": true}})
	})
}

// selfOnly 校验当前登录用户只能操作自己的库（路径参数为 :id）
func selfOnly(c *gin.Context) bool {
	return selfOnlyUID(c, c.Param("id"))
}

// selfOnlyUID 校验当前登录用户只能操作指定 uid 的资源
func selfOnlyUID(c *gin.Context, uid string) bool {
	accountID := auth.GetAccountID(c)
	if accountID == auth.DefaultAccountID || accountID != uid {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	return true
}
