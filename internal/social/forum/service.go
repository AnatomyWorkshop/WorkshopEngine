package forum

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"gorm.io/gorm"
	"mvu-backend/internal/core/util"
	"mvu-backend/internal/social/reaction"
)

// Service 社区论坛帖子业务逻辑
type Service struct {
	db          *gorm.DB
	reactionSvc *reaction.Service
	md          goldmark.Markdown
	sanitizer   *bluemonday.Policy
}

// New 创建 Service，注入 reaction.Service 供帖子点赞使用
func New(db *gorm.DB, reactionSvc *reaction.Service) *Service {
	return &Service{
		db:          db,
		reactionSvc: reactionSvc,
		md:          goldmark.New(),
		sanitizer:   bluemonday.UGCPolicy(), // 允许常见格式化标签，过滤危险属性
	}
}

// ── 内容管线 ─────────────────────────────────────────────────────────────────

// RenderContent Markdown → 净化 HTML
// 原始 Markdown 单独存入 content_raw 供编辑回显，
// content 字段存储最终 HTML（防 XSS）。
func (s *Service) RenderContent(raw string) (html string, err error) {
	var buf bytes.Buffer
	if err = s.md.Convert([]byte(raw), &buf); err != nil {
		return "", fmt.Errorf("markdown render: %w", err)
	}
	return s.sanitizer.Sanitize(buf.String()), nil
}

// ── 创建帖子 ─────────────────────────────────────────────────────────────────

// CreatePost 发布帖子。
// content 参数为原始 Markdown，服务层负责渲染 + 净化后存库。
func (s *Service) CreatePost(authorID, title, rawContent string, gameTags []string, postType string) (*Post, error) {
	if postType == "" {
		postType = "discussion"
	}
	html, err := s.RenderContent(rawContent)
	if err != nil {
		return nil, err
	}
	post := &Post{
		AuthorID:   authorID,
		Title:      title,
		Slug:       util.Slugify(title, "post"),
		Content:    html,
		ContentRaw: rawContent,
		GameTags:   gameTags,
		PostType:   postType,
	}
	if err := s.db.Create(post).Error; err != nil {
		return nil, err
	}
	return post, nil
}

// ── 查询帖子 ─────────────────────────────────────────────────────────────────

// GetPost 按 ID 或 Slug 获取帖子
func (s *Service) GetPost(idOrSlug string) (*Post, error) {
	var post Post
	err := s.db.Where("id = ? OR slug = ?", idOrSlug, idOrSlug).
		Where("status != 'archived'").First(&post).Error
	if err != nil {
		return nil, ErrPostNotFound
	}
	return &post, nil
}

// ListResult 帖子列表分页结果
type ListResult struct {
	Total int64   `json:"total"`
	Items []Post  `json:"items"`
}

// ListPosts 帖子列表，支持过滤、排序、全文搜索。
//
// gameTag：过滤含指定游戏标签的帖子（PostgreSQL ANY 操作符）
// postType：过滤帖子类型
// sort："hot"（热度）| "new"（最新，默认）
// q：关键词搜索（MVP 使用 ILIKE，触发器就绪后自动升级为 GIN 全文搜索）
func (s *Service) ListPosts(gameTag, postType, sort, q string, limit, offset int) (ListResult, error) {
	query := s.db.Model(&Post{}).Where("status = 'published'")

	if gameTag != "" {
		query = query.Where("? = ANY(game_tags)", gameTag)
	}
	if postType != "" {
		query = query.Where("post_type = ?", postType)
	}
	if q != "" {
		like := "%" + strings.ReplaceAll(q, "%", "\\%") + "%"
		query = query.Where("title ILIKE ? OR content_raw ILIKE ?", like, like)
	}

	var total int64
	query.Count(&total)

	query = applyPostSort(query, sort).Limit(limit).Offset(offset)

	var posts []Post
	if err := query.Find(&posts).Error; err != nil {
		return ListResult{}, err
	}
	return ListResult{Total: total, Items: posts}, nil
}

// CountByGameTag 统计含指定游戏标签的已发布帖子数（用于 /social/games/:id/stats）
func (s *Service) CountByGameTag(gameTag string) int64 {
	var n int64
	s.db.Model(&Post{}).
		Where("status = 'published' AND ? = ANY(game_tags)", gameTag).
		Count(&n)
	return n
}

// ── 编辑 / 删除帖子 ──────────────────────────────────────────────────────────

// EditPost 编辑帖子（仅作者）
func (s *Service) EditPost(postID, requesterID, newTitle, newRawContent string, gameTags []string) (*Post, error) {
	var post Post
	if err := s.db.Where("id = ? AND status != 'archived'", postID).First(&post).Error; err != nil {
		return nil, ErrPostNotFound
	}
	if post.AuthorID != requesterID {
		return nil, ErrForbidden
	}
	html, err := s.RenderContent(newRawContent)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"title":       newTitle,
		"content":     html,
		"content_raw": newRawContent,
		"game_tags":   gameTags,
		"slug":        util.Slugify(newTitle, "post"),
	}
	if err := s.db.Model(&post).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

// DeletePost 软删除（archived）帖子（仅作者）
func (s *Service) DeletePost(postID, requesterID string) error {
	var post Post
	if err := s.db.Where("id = ?", postID).First(&post).Error; err != nil {
		return ErrPostNotFound
	}
	if post.AuthorID != requesterID {
		return ErrForbidden
	}
	return s.db.Model(&post).Update("status", "archived").Error
}

// ── 回复（盖楼）────────────────────────────────────────────────────────────

// CreateReply 盖楼（事务内：INSERT + UPDATE posts 反规范化字段）。
//
// parentID 可空，空=顶层回复。
// Number（楼层序号）= 当前 replies_count + 1（事务内原子获得）。
func (s *Service) CreateReply(postID, authorID, parentID, rawContent string) (*ForumReply, error) {
	html, err := s.RenderContent(rawContent)
	if err != nil {
		return nil, err
	}

	var reply ForumReply
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 悲观锁：避免楼层号并发冲突
		var post Post
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND status = 'published'", postID).First(&post).Error; err != nil {
			return ErrPostNotFound
		}

		now := time.Now()
		reply = ForumReply{
			PostID:   postID,
			AuthorID: authorID,
			ParentID: parentID,
			Number:   post.RepliesCount + 1,
			Content:  html,
		}
		if err := tx.Create(&reply).Error; err != nil {
			return err
		}

		// 更新反规范化缓存（来自 Flarum discussions 表设计）
		return tx.Model(&post).Updates(map[string]any{
			"replies_count":   post.RepliesCount + 1,
			"last_reply_at":   now,
			"last_reply_user": authorID,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

// ListReplies 查询帖子的盖楼列表（按楼层序号 ASC 分页）
func (s *Service) ListReplies(postID string, limit, offset int) ([]ForumReply, int64, error) {
	q := s.db.Model(&ForumReply{}).
		Where("post_id = ? AND status = 'visible'", postID)
	var total int64
	q.Count(&total)
	var replies []ForumReply
	err := q.Order("number ASC").Limit(limit).Offset(offset).Find(&replies).Error
	return replies, total, err
}

// ── 点赞帖子 ─────────────────────────────────────────────────────────────────

// VotePost 对帖子点赞
func (s *Service) VotePost(postID, authorID string) error {
	var exists int64
	s.db.Model(&Post{}).Where("id = ? AND status = 'published'", postID).Count(&exists)
	if exists == 0 {
		return ErrPostNotFound
	}
	return s.reactionSvc.Add(reaction.TargetForumPost, postID, authorID, reaction.TypeLike)
}

// UnvotePost 取消点赞
func (s *Service) UnvotePost(postID, authorID string) error {
	return s.reactionSvc.Remove(reaction.TargetForumPost, postID, authorID, reaction.TypeLike)
}

// ── 热度计算（Wilson score 简化版）────────────────────────────────────────────

// HotScore 计算帖子热度分（供测试和 SQL 表达式参考）。
//
// 公式：(replies_count * 2 + vote_up) / pow(age_hours + 2, 1.5)
// 来源：Hacker News / Reddit 热度算法的简化版本。
// 实际 SQL 排序在 applyPostSort 中内联，此函数供单元测试验证公式正确性。
func HotScore(repliesCount, voteUp int, createdAt time.Time) float64 {
	ageHours := time.Since(createdAt).Hours()
	score := float64(repliesCount*2+voteUp) / math.Pow(ageHours+2, 1.5)
	return score
}

// ── 内部工具 ─────────────────────────────────────────────────────────────────

func applyPostSort(q *gorm.DB, sort string) *gorm.DB {
	switch sort {
	case "hot":
		// Wilson score 简化版：(replies_count * 2 + vote_up) / pow(age_hours + 2, 1.5)
		// PostgreSQL EXTRACT(EPOCH ...) / 3600 = age_hours
		return q.Order(`
			(replies_count * 2 + vote_up) /
			POWER(EXTRACT(EPOCH FROM (NOW() - created_at)) / 3600.0 + 2, 1.5) DESC
		`)
	default: // "new"
		return q.Order("created_at DESC")
	}
}
