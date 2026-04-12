package library

import (
	"time"

	"gorm.io/gorm"
)

// LibraryEntry 玩家个人游戏库条目。
//
// 与 GameSession 解耦：库里有游戏不代表有存档，存档不代表在库里。
// SeriesKey 由前端生成（通常为 game.slug），用于将同一游戏的多个版本归组。
type LibraryEntry struct {
	ID           string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	UserID       string     `gorm:"not null;index"                                json:"user_id"`
	GameID       string     `gorm:"not null;index"                                json:"game_id"`
	SeriesKey    string     `gorm:"not null;default:''"                           json:"series_key"` // 前端生成，通常为 game.slug
	Source       string     `gorm:"not null;default:'catalog'"                    json:"source"`     // catalog | local
	LastPlayedAt *time.Time `json:"last_played_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&LibraryEntry{}); err != nil {
		return err
	}
	// 同一用户不能重复导入同一游戏
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_library_user_game
		ON library_entries(user_id, game_id)`).Error
}
