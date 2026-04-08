package forum

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Post 社区论坛帖子。
//
// 与 comment/ 的核心区别：
//   - GameTags 弱关联（可空 text[]），不是 game_id 强绑定
//   - 帖子可以独立存在，游戏标签只是过滤维度
//   - RepliesCount / LastReplyAt 反规范化到 Post 行（来自 Flarum discussions 表设计），
//     热帖排序无需 JOIN
//
// 搜索：search_vector 列由 PostgreSQL 触发器维护（见 MigrateSQL），
// 首次部署后需手动执行一次 forum.MigrateSQL()。
// MVP 阶段回退到 ILIKE 搜索，触发器建好后自动升级为 GIN 全文搜索。
type Post struct {
	ID            string     `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	AuthorID      string     `gorm:"not null;index"                                 json:"author_id"`
	Title         string     `gorm:"not null"                                       json:"title"`
	Slug          string     `gorm:"uniqueIndex;not null"                           json:"slug"`
	Content       string     `gorm:"type:text;not null"                             json:"content"`       // 存储渲染后的 HTML（经 bluemonday 净化）
	ContentRaw    string     `gorm:"type:text"                                      json:"content_raw"`   // 原始 Markdown，供编辑回显
	GameTags      []string   `gorm:"type:text[]"                                    json:"game_tags"`     // 弱关联游戏，可空可多值
	PostType      string     `gorm:"default:'discussion'"                           json:"post_type"`     // discussion|guide|journal|fanart
	Status        string     `gorm:"default:'published'"                            json:"status"`        // published|draft|archived
	// 反规范化缓存（来自 Flarum discussions 表）：热帖排序无需 JOIN
	RepliesCount  int        `gorm:"default:0"                                      json:"replies_count"`
	VoteUp        int        `gorm:"default:0"                                      json:"vote_up"`       // reaction.syncCount 维护
	LastReplyAt   *time.Time `json:"last_reply_at"`
	LastReplyUser string     `json:"last_reply_user"` // 最后回复者 authorID（显示用）
	// 全文搜索向量（由 PostgreSQL 触发器维护）
	SearchVector  string     `gorm:"type:tsvector;-:migration"                      json:"-"` // 不参与 AutoMigrate，由触发器填充
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ForumReply 帖子回复（盖楼）。
//
// Number 来自 Flarum posts.number 设计，表示帖子内的楼层序号（从 1 起），
// 方便引用（"3楼"），在事务内由 replies_count 递增获得。
type ForumReply struct {
	ID        string    `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	PostID    string    `gorm:"not null;index"                                 json:"post_id"`
	AuthorID  string    `gorm:"not null;index"                                 json:"author_id"`
	ParentID  string    `gorm:"index;default:''"                               json:"parent_id"` // 楼中楼，空=顶层回复
	Number    int       `gorm:"not null"                                       json:"number"`    // 楼层序号（Flarum posts.number）
	Content   string    `gorm:"type:text;not null"                             json:"content"`
	VoteUp    int       `gorm:"default:0"                                      json:"vote_up"`
	Status    string    `gorm:"default:'visible'"                              json:"status"` // visible | deleted
	CreatedAt time.Time `json:"created_at"`
}

// ErrPostNotFound 帖子不存在
var ErrPostNotFound = errors.New("post not found")

// ErrForbidden 无权操作
var ErrForbidden = errors.New("forbidden")

// Migrate 创建 posts 和 forum_replies 表（幂等）
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Post{}, &ForumReply{})
}

// MigrateSQL 返回需要在首次部署后手动执行一次的全文搜索触发器 SQL。
// 执行时机：Migrate() 之后，数据库就绪后运行一次。
func MigrateSQL() string {
	return `
-- 全文搜索 GIN 索引
CREATE INDEX IF NOT EXISTS posts_search_idx ON posts USING GIN(search_vector);

-- 维护 search_vector 的触发器函数
CREATE OR REPLACE FUNCTION update_post_search() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('simple',
    COALESCE(NEW.title, '') || ' ' || COALESCE(NEW.content_raw, ''));
  RETURN NEW;
END $$ LANGUAGE plpgsql;

-- 触发器：INSERT 或 UPDATE 时自动更新 search_vector
CREATE TRIGGER post_search_trigger
  BEFORE INSERT OR UPDATE ON posts
  FOR EACH ROW EXECUTE FUNCTION update_post_search();
`
}
