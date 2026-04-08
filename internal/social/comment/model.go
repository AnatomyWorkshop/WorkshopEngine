package comment

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Comment 游戏评论区单条记录。
//
// 树形结构：
//   - 主楼（root comment）：Rid == ""，RootID == 自身 ID
//   - 回复（reply）：Rid = 直接父节点 ID，RootID = 所在主楼 ID
//
// 树形重建（参考 Artalk 双索引方案）：
//  1. 查主楼：WHERE game_id = ? AND rid = '' ORDER BY ... LIMIT/OFFSET
//  2. 查子节点：WHERE root_id IN (主楼IDs) AND rid != ''  （一次查出所有子节点）
//  3. 在内存中按 rid 分组挂载到主楼下
//
// 线性模式（LinearFeed）：rid 始终为空，仅按 created_at 排序，跳过步骤 2/3。
type Comment struct {
	ID          string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID      string    `gorm:"not null;index"                                 json:"game_id"`    // 强绑定，必填，无 DB 外键约束
	AuthorID    string    `gorm:"not null;index"                                 json:"author_id"`
	Content     string    `gorm:"type:text;not null"                             json:"content"`
	Rid         string    `gorm:"index;default:''"                               json:"rid"`        // 直接父节点 ID（""=主楼）
	RootID      string    `gorm:"index;default:''"                               json:"root_id"`    // 主楼 ID，加速子节点批量查询
	ThreadType  string    `gorm:"default:'linear'"                               json:"thread_type"` // linear | nested
	IsCollapsed bool      `gorm:"default:false"                                  json:"is_collapsed"`
	IsPinned    bool      `gorm:"default:false"                                  json:"is_pinned"`
	IsPending   bool      `gorm:"default:false"                                  json:"is_pending"` // 待审核（RequireApproval=true 时启用）
	VoteUp      int       `gorm:"default:0"                                      json:"vote_up"`    // 反规范化点赞数（reaction.syncCount 维护）
	Status      string    `gorm:"default:'visible'"                              json:"status"`     // visible | hidden | deleted
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SortMode 列表排序
const (
	SortDateDesc = "date_desc" // created_at DESC（默认）
	SortDateAsc  = "date_asc"  // created_at ASC
	SortVote     = "vote"      // vote_up DESC, created_at DESC（热门）
)

// GameCommentConfig 游戏评论区配置（游戏设计者通过 /api/create/games/:id/comment-config 管理）。
//
// 放在 comment 包内而非 core/db，因为只有 comment/ 和 creation/ 需要它，
// 不是跨层共享模型。creation/api 通过 import comment 包读取。
type GameCommentConfig struct {
	GameID          string   `gorm:"primaryKey"                   json:"game_id"`
	EnabledModes    []string `gorm:"type:text[]"                  json:"enabled_modes"`   // ["linear","nested"]
	DefaultMode     string   `gorm:"default:'linear'"             json:"default_mode"`
	AllowAnonymous  bool     `gorm:"default:false"                json:"allow_anonymous"`
	RequireApproval bool     `gorm:"default:false"                json:"require_approval"` // 开启时新评论 is_pending=true
}

// ErrCommentNotFound 评论不存在
var ErrCommentNotFound = errors.New("comment not found")

// ErrForbidden 无权操作（非作者也非游戏设计者）
var ErrForbidden = errors.New("forbidden")

// ErrGameMismatch 回复目标与当前游戏不一致
var ErrGameMismatch = errors.New("parent comment belongs to a different game")

// Migrate 创建 comments 和 game_comment_configs 表（幂等）
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Comment{}, &GameCommentConfig{})
}
