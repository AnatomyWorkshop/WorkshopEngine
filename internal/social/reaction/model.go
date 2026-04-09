package reaction

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// TargetType 点赞对象类型（多态）
type TargetType string

const (
	TargetComment    TargetType = "comment"
	TargetForumPost  TargetType = "forum_post"
	TargetForumReply TargetType = "forum_reply"
	TargetGame       TargetType = "game"
)

// Type 点赞/收藏类型（MVP 只做 like + favorite，coin 留枚举占位）
type Type string

const (
	TypeLike     Type = "like"
	TypeFavorite Type = "favorite"
	// TypeCoin = "coin" — Phase 3，依赖经济系统，暂不实现
)

// Reaction 多态互动记录（点赞 / 收藏）
//
// UNIQUE (target_type, target_id, author_id, type) 防止重复操作；
// 该约束通过 Migrate 中的原生 SQL 建立，GORM AutoMigrate 不创建多列唯一索引。
//
// 参考设计：Gitea reaction.go UNIQUE(s) 约束 + Artalk Vote 表思路，
// 采用多态字符串 ID（而非 Gitea 的 IssueID/CommentID 强类型分离）以适应 GW 的灵活场景。
type Reaction struct {
	ID         string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	TargetType TargetType `gorm:"not null;index"                                 json:"target_type"` // comment | forum_post | forum_reply
	TargetID   string     `gorm:"not null;index"                                 json:"target_id"`
	AuthorID   string     `gorm:"not null;index"                                 json:"author_id"`
	Type       Type       `gorm:"not null"                                       json:"type"` // like | favorite
	CreatedAt  time.Time  `json:"created_at"`
}

// ErrAlreadyReacted 已经点赞过（幂等，返回 409）
var ErrAlreadyReacted = errors.New("already reacted")

// ErrNotReacted 尚未点赞，无法取消（返回 404）
var ErrNotReacted = errors.New("reaction not found")

// Migrate 创建表并建立多列唯一索引（幂等，安全重复调用）
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&Reaction{}); err != nil {
		return err
	}
	// UNIQUE (target_type, target_id, author_id, type) — 防重复点赞
	// 参考 Gitea reaction.go UNIQUE(s) 约束，改为多态字符串 ID 版本
	return db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_reactions_unique
		ON reactions (target_type, target_id, author_id, type)
	`).Error
}

// validTargetTypes MVP 允许的目标类型
var validTargetTypes = map[TargetType]bool{
	TargetComment:    true,
	TargetForumPost:  true,
	TargetForumReply: true,
	TargetGame:       true,
}

// validTypes MVP 允许的互动类型
var validTypes = map[Type]bool{
	TypeLike:     true,
	TypeFavorite: true,
}

// IsValidTarget 校验目标类型是否合法
func IsValidTarget(t TargetType) bool { return validTargetTypes[t] }

// IsValidType 校验互动类型是否合法
func IsValidType(t Type) bool { return validTypes[t] }
