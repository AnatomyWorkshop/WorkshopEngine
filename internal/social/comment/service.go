package comment

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"mvu-backend/internal/social/reaction"
)

// Service 游戏评论区业务逻辑
type Service struct {
	db          *gorm.DB
	reactionSvc *reaction.Service
}

// New 创建 Service，注入 reaction.Service 供 Vote 使用
func New(db *gorm.DB, reactionSvc *reaction.Service) *Service {
	return &Service{db: db, reactionSvc: reactionSvc}
}

// ── 创建 ────────────────────────────────────────────────────────────────────

// Create 发主楼评论（Rid == ""）
func (s *Service) Create(gameID, authorID, content, threadType string) (*Comment, error) {
	if threadType == "" {
		threadType = "linear"
	}
	c := &Comment{
		GameID:     gameID,
		AuthorID:   authorID,
		Content:    content,
		ThreadType: threadType,
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, err
	}
	// 主楼的 RootID = 自身 ID（写入后才知道 ID）
	if err := s.db.Model(c).Update("root_id", c.ID).Error; err != nil {
		return nil, err
	}
	c.RootID = c.ID
	return c, nil
}

// Reply 回复某楼（Rid = parentID）
func (s *Service) Reply(gameID, authorID, content, parentID string) (*Comment, error) {
	// 验证父节点存在且属于同一游戏
	var parent Comment
	if err := s.db.Where("id = ? AND status != 'deleted'", parentID).First(&parent).Error; err != nil {
		return nil, ErrCommentNotFound
	}
	if parent.GameID != gameID {
		return nil, ErrGameMismatch
	}

	// RootID 继承：若父节点是主楼（rid=""），rootID = parentID；否则继承父节点的 rootID
	rootID := parent.RootID
	if rootID == "" {
		rootID = parent.ID
	}

	c := &Comment{
		GameID:     gameID,
		AuthorID:   authorID,
		Content:    content,
		Rid:        parentID,
		RootID:     rootID,
		ThreadType: parent.ThreadType,
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

// ── 查询 ───────────────────────────────────────────────────────────��────────

// CommentWithReplies 主楼 + 子节点列表（嵌套模式用）
type CommentWithReplies struct {
	Comment
	Replies []Comment `json:"replies"`
}

// ListByGame 查询某游戏的主楼列表（分页）。
//
// 线性模式：直接返回主楼列表，不附带 Replies。
// 嵌套模式：附带每个主楼的子节点（一次额外 IN 查询，参考 Artalk 双索引方案）。
func (s *Service) ListByGame(gameID, sort, threadType string, limit, offset int) ([]CommentWithReplies, int64, error) {
	q := s.db.Model(&Comment{}).
		Where("game_id = ? AND rid = '' AND status = 'visible'", gameID)

	var total int64
	q.Count(&total)

	q = applySort(q, sort).Limit(limit).Offset(offset)

	var roots []Comment
	if err := q.Find(&roots).Error; err != nil {
		return nil, 0, err
	}

	result := make([]CommentWithReplies, len(roots))
	for i, r := range roots {
		result[i] = CommentWithReplies{Comment: r}
	}

	// 嵌套模式：一次 IN 查询取出所有子节点，内存重组
	if threadType == "nested" && len(roots) > 0 {
		rootIDs := make([]string, len(roots))
		for i, r := range roots {
			rootIDs[i] = r.ID
		}

		var replies []Comment
		s.db.Where("root_id IN ? AND rid != '' AND status = 'visible'", rootIDs).
			Order("created_at ASC").Find(&replies)

		// 按 root_id 分组挂载
		replyMap := make(map[string][]Comment)
		for _, rep := range replies {
			replyMap[rep.RootID] = append(replyMap[rep.RootID], rep)
		}
		for i := range result {
			result[i].Replies = replyMap[result[i].ID]
		}
	}

	return result, total, nil
}

// ListReplies 查询某主楼下的所有子评论（平铺，已按 created_at ASC 排序）
// 供前端自行重组树形或线性展示
func (s *Service) ListReplies(rootID string, limit, offset int) ([]Comment, int64, error) {
	q := s.db.Model(&Comment{}).
		Where("root_id = ? AND rid != '' AND status = 'visible'", rootID)

	var total int64
	q.Count(&total)

	var replies []Comment
	err := q.Order("created_at ASC").Limit(limit).Offset(offset).Find(&replies).Error
	return replies, total, err
}

// CountByGame 统计某游戏的可见评论总数（用于 /social/games/:id/stats）
func (s *Service) CountByGame(gameID string) int64 {
	var n int64
	s.db.Model(&Comment{}).Where("game_id = ? AND status = 'visible'", gameID).Count(&n)
	return n
}

// ── 编辑 / 删除 ─────────────────────────────────────────────────────────────

// EditDeadline 允许编辑的时间窗口（5 分钟）
const EditDeadline = 5 * time.Minute

// Edit 编辑评论内容（仅作者，5 分钟内）
func (s *Service) Edit(commentID, requesterID, newContent string) (*Comment, error) {
	var c Comment
	if err := s.db.Where("id = ? AND status != 'deleted'", commentID).First(&c).Error; err != nil {
		return nil, ErrCommentNotFound
	}
	if c.AuthorID != requesterID {
		return nil, ErrForbidden
	}
	if time.Since(c.CreatedAt) > EditDeadline {
		return nil, fmt.Errorf("edit window expired (5 min)")
	}
	if err := s.db.Model(&c).Update("content", newContent).Error; err != nil {
		return nil, err
	}
	c.Content = newContent
	return &c, nil
}

// Delete 软删除（作者或游戏设计者均可）
// 不清除内容，将 status 设为 "deleted"，保持树形结构完整
func (s *Service) Delete(commentID, requesterID, gameAuthorID string) error {
	var c Comment
	if err := s.db.Where("id = ?", commentID).First(&c).Error; err != nil {
		return ErrCommentNotFound
	}
	if c.AuthorID != requesterID && c.GameID != gameAuthorID {
		// gameAuthorID 传入游戏设计者的 AccountID，创作者可管理自己游戏下的评论
		return ErrForbidden
	}
	return s.db.Model(&c).Update("status", "deleted").Error
}

// ── 点赞 ─────────────────────────────────────────────────────────────────────

// Vote 对评论点赞（调用 reaction.Service，成功后 vote_up 由 syncCount 自动更新）
func (s *Service) Vote(commentID, authorID string) error {
	// 验证评论存在
	var exists int64
	s.db.Model(&Comment{}).Where("id = ? AND status = 'visible'", commentID).Count(&exists)
	if exists == 0 {
		return ErrCommentNotFound
	}
	return s.reactionSvc.Add(reaction.TargetComment, commentID, authorID, reaction.TypeLike)
}

// Unvote 取消点赞
func (s *Service) Unvote(commentID, authorID string) error {
	return s.reactionSvc.Remove(reaction.TargetComment, commentID, authorID, reaction.TypeLike)
}

// ── 配置 ─────────────────────────────────────────────────────────────────────

// GetConfig 获取游戏评论区配置，不存在时返回默认配置
func (s *Service) GetConfig(gameID string) GameCommentConfig {
	var cfg GameCommentConfig
	if err := s.db.Where("game_id = ?", gameID).First(&cfg).Error; err != nil {
		// 不存在时返回默认配置
		return GameCommentConfig{
			GameID:      gameID,
			DefaultMode: "linear",
		}
	}
	return cfg
}

// UpsertConfig 更新或创建游戏评论区配置（仅游戏设计者）
func (s *Service) UpsertConfig(cfg GameCommentConfig) error {
	return s.db.Save(&cfg).Error
}

// ── 内部工具 ─────────────────────────────────────────────────────────────────

func applySort(q *gorm.DB, sort string) *gorm.DB {
	switch sort {
	case SortDateAsc:
		return q.Order("created_at ASC")
	case SortVote:
		return q.Order("is_pinned DESC, vote_up DESC, created_at DESC")
	default: // SortDateDesc
		return q.Order("is_pinned DESC, created_at DESC")
	}
}
