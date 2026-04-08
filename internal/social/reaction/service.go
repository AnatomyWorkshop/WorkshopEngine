package reaction

import (
	"fmt"

	"gorm.io/gorm"
)

// Service 处理点赞/收藏的增删查
type Service struct {
	db *gorm.DB
}

// New 创建 Service 实例
func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Add 添加点赞/收藏。
//
// 成功后同步更新目标对象的反规范化计数（vote_up 字段）。
// 若记录已存在，返回 ErrAlreadyReacted（调用方返回 409）。
func (s *Service) Add(targetType TargetType, targetID, authorID string, reactionType Type) error {
	r := Reaction{
		TargetType: targetType,
		TargetID:   targetID,
		AuthorID:   authorID,
		Type:       reactionType,
	}

	// INSERT ... ON CONFLICT DO NOTHING — 幂等写入
	// 通过检查 RowsAffected 区分"新建"和"已存在"
	result := s.db.Create(&r)
	if result.Error != nil {
		// PostgreSQL 唯一冲突错误码 23505
		if isUniqueViolation(result.Error) {
			return ErrAlreadyReacted
		}
		return result.Error
	}

	// 仅 like 更新目标计数（favorite 不增 vote_up）
	if reactionType == TypeLike {
		s.syncCount(targetType, targetID, +1)
	}
	return nil
}

// Remove 取消点赞/收藏。
//
// 成功后同步更新目标对象的反规范化计数。
// 若记录不存在，返回 ErrNotReacted（调用方返回 404）。
func (s *Service) Remove(targetType TargetType, targetID, authorID string, reactionType Type) error {
	result := s.db.
		Where("target_type = ? AND target_id = ? AND author_id = ? AND type = ?",
			targetType, targetID, authorID, reactionType).
		Delete(&Reaction{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotReacted
	}

	if reactionType == TypeLike {
		s.syncCount(targetType, targetID, -1)
	}
	return nil
}

// CountResult 单个目标的计数汇总
type CountResult struct {
	TargetType TargetType `json:"target_type"`
	TargetID   string     `json:"target_id"`
	Likes      int64      `json:"likes"`
	Favorites  int64      `json:"favorites"`
}

// CountBatch 批量查询多个目标的 like + favorite 计数。
//
// targets 格式：[]string{"comment:uuid1", "forum_post:uuid2"}
// 返回 map key 同输入格式，方便前端直接匹配。
func (s *Service) CountBatch(targets []string) (map[string]CountResult, error) {
	// 解析 "type:id" 格式
	type pair struct{ tt TargetType; id string }
	pairs := make([]pair, 0, len(targets))
	for _, t := range targets {
		var tt, id string
		n, _ := fmt.Sscanf(t, "%s", &tt) // 先用简单分割
		_ = n
		// 手动分割，Sscanf 无法按 ":" 分割
		for i, c := range t {
			if c == ':' {
				tt = t[:i]
				id = t[i+1:]
				break
			}
		}
		if tt == "" || id == "" {
			continue
		}
		pairs = append(pairs, pair{TargetType(tt), id})
	}
	if len(pairs) == 0 {
		return map[string]CountResult{}, nil
	}

	// 批量查询：SELECT target_type, target_id, type, COUNT(*) FROM reactions WHERE ... GROUP BY ...
	type row struct {
		TargetType TargetType
		TargetID   string
		Type       Type
		Cnt        int64
	}

	// 构建 (target_type, target_id) IN (...) 条件
	// GORM 不原生支持多列 IN，用 OR 代替（数量通常较小，性能可接受）
	query := s.db.Model(&Reaction{}).
		Select("target_type, target_id, type, COUNT(*) as cnt").
		Group("target_type, target_id, type")

	orCond := s.db.Where("1=0")
	for _, p := range pairs {
		orCond = orCond.Or("target_type = ? AND target_id = ?", p.tt, p.id)
	}
	query = query.Where(orCond)

	var rows []row
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 组装结果 map
	result := make(map[string]CountResult, len(pairs))
	for _, p := range pairs {
		result[string(p.tt)+":"+p.id] = CountResult{
			TargetType: p.tt,
			TargetID:   p.id,
		}
	}
	for _, r := range rows {
		key := string(r.TargetType) + ":" + r.TargetID
		cr := result[key]
		switch r.Type {
		case TypeLike:
			cr.Likes = r.Cnt
		case TypeFavorite:
			cr.Favorites = r.Cnt
		}
		result[key] = cr
	}
	return result, nil
}

// Mine 查询当前用户对指定目标的互动状态（已点赞/已收藏）
type MineResult struct {
	Liked     bool `json:"liked"`
	Favorited bool `json:"favorited"`
}

// CheckMine 查询 authorID 对 (targetType, targetID) 的点赞/收藏状态
func (s *Service) CheckMine(targetType TargetType, targetID, authorID string) MineResult {
	var reactions []Reaction
	s.db.Where("target_type = ? AND target_id = ? AND author_id = ?",
		targetType, targetID, authorID).Find(&reactions)

	var res MineResult
	for _, r := range reactions {
		switch r.Type {
		case TypeLike:
			res.Liked = true
		case TypeFavorite:
			res.Favorited = true
		}
	}
	return res
}

// syncCount 同步更新目标对象的反规范化 vote_up 计数（+1 或 -1）。
//
// 参考 Artalk vote.go：在写入 vote 记录后同步更新 comment.vote_up 字段，
// 避免每次 COUNT(reactions)。此处采用原生 SQL 保证原子性。
//
// 失败不影响主流程（计数可从 reactions 表重建），静默记录。
func (s *Service) syncCount(targetType TargetType, targetID string, delta int) {
	var table, col string
	switch targetType {
	case TargetComment:
		table, col = "comments", "vote_up"
	case TargetForumPost:
		table, col = "posts", "vote_up"
	case TargetForumReply:
		table, col = "forum_replies", "vote_up"
	default:
		return
	}

	op := "+"
	if delta < 0 {
		op = "-"
		delta = -delta
	}
	sql := fmt.Sprintf(
		"UPDATE %s SET %s = GREATEST(0, %s %s %d) WHERE id = ?",
		table, col, col, op, delta,
	)
	s.db.Exec(sql, targetID)
}

// isUniqueViolation 检查是否是 PostgreSQL 唯一键冲突（code 23505）
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// 通过错误字符串检测，避免直接引入 pgx 包
	msg := err.Error()
	return len(msg) > 0 && (contains(msg, "23505") || contains(msg, "unique constraint") || contains(msg, "duplicate key"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
