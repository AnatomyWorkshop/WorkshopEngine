//go:build integration

// Integration tests: social layer DB (reaction / comment / forum).
// Run: go test -tags integration -v -count=1 -run Social
// Requires TEST_DSN env var, e.g.:
//   TEST_DSN="host=localhost user=postgres password=postgres dbname=gw_test sslmode=disable"
package mvu_test

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mvu-backend/internal/social/comment"
	"mvu-backend/internal/social/forum"
	"mvu-backend/internal/social/reaction"
)

// openTestDB 打开测试数据库，若 TEST_DSN 未设置则 skip
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	// 迁移测试用表（幂等）
	if err := reaction.Migrate(db); err != nil {
		t.Fatalf("migrate reaction: %v", err)
	}
	if err := comment.Migrate(db); err != nil {
		t.Fatalf("migrate comment: %v", err)
	}
	if err := forum.Migrate(db); err != nil {
		t.Fatalf("migrate forum: %v", err)
	}
	return db
}

// ── Reaction ─────────────────────────────────────────────────────────────────

func TestSocial_Reaction_AddRemove(t *testing.T) {
	db := openTestDB(t)
	svc := reaction.New(db)

	targetID := "test-target-" + t.Name()
	authorID := "user-1"

	// 首次点赞
	if err := svc.Add(reaction.TargetComment, targetID, authorID, reaction.TypeLike); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 重复点赞 → ErrAlreadyReacted
	if err := svc.Add(reaction.TargetComment, targetID, authorID, reaction.TypeLike); err != reaction.ErrAlreadyReacted {
		t.Errorf("duplicate Add should return ErrAlreadyReacted, got: %v", err)
	}

	// 取消
	if err := svc.Remove(reaction.TargetComment, targetID, authorID, reaction.TypeLike); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// 再取消 → ErrNotReacted
	if err := svc.Remove(reaction.TargetComment, targetID, authorID, reaction.TypeLike); err != reaction.ErrNotReacted {
		t.Errorf("second Remove should return ErrNotReacted, got: %v", err)
	}

	// 清理
	db.Exec("DELETE FROM reactions WHERE target_id = ?", targetID)
}

func TestSocial_Reaction_CountBatch(t *testing.T) {
	db := openTestDB(t)
	svc := reaction.New(db)

	id1 := "count-target-1-" + t.Name()
	id2 := "count-target-2-" + t.Name()

	svc.Add(reaction.TargetForumPost, id1, "user-a", reaction.TypeLike)
	svc.Add(reaction.TargetForumPost, id1, "user-b", reaction.TypeLike)
	svc.Add(reaction.TargetForumPost, id1, "user-c", reaction.TypeFavorite)

	counts, err := svc.CountBatch([]string{
		"forum_post:" + id1,
		"forum_post:" + id2,
	})
	if err != nil {
		t.Fatalf("CountBatch: %v", err)
	}
	r1 := counts["forum_post:"+id1]
	if r1.Likes != 2 {
		t.Errorf("likes = %d, want 2", r1.Likes)
	}
	if r1.Favorites != 1 {
		t.Errorf("favorites = %d, want 1", r1.Favorites)
	}
	r2 := counts["forum_post:"+id2]
	if r2.Likes != 0 {
		t.Errorf("id2 likes = %d, want 0", r2.Likes)
	}

	db.Exec("DELETE FROM reactions WHERE target_id IN (?, ?)", id1, id2)
}

func TestSocial_Reaction_CheckMine(t *testing.T) {
	db := openTestDB(t)
	svc := reaction.New(db)

	targetID := "mine-target-" + t.Name()
	userID := "user-mine"

	svc.Add(reaction.TargetComment, targetID, userID, reaction.TypeLike)

	mine := svc.CheckMine(reaction.TargetComment, targetID, userID)
	if !mine.Liked {
		t.Error("CheckMine.Liked should be true")
	}
	if mine.Favorited {
		t.Error("CheckMine.Favorited should be false")
	}

	db.Exec("DELETE FROM reactions WHERE target_id = ?", targetID)
}

// ── Comment ──────────────────────────────────────────────────────────────────

func TestSocial_Comment_CreateAndTree(t *testing.T) {
	db := openTestDB(t)
	reactionSvc := reaction.New(db)
	svc := comment.New(db, reactionSvc)

	gameID := "game-comment-test-" + t.Name()
	authorID := "author-1"

	// 发主楼
	root, err := svc.Create(gameID, authorID, "主楼内容", "nested")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if root.RootID != root.ID {
		t.Errorf("root.RootID should equal root.ID, got %q", root.RootID)
	}

	// 发回复
	reply, err := svc.Reply(gameID, "author-2", "回复内容", root.ID)
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if reply.RootID != root.ID {
		t.Errorf("reply.RootID = %q, want %q", reply.RootID, root.ID)
	}
	if reply.Rid != root.ID {
		t.Errorf("reply.Rid = %q, want %q", reply.Rid, root.ID)
	}

	// 嵌套模式查询，应包含回复
	list, total, err := svc.ListByGame(gameID, "date_desc", "nested", 20, 0)
	if err != nil {
		t.Fatalf("ListByGame: %v", err)
	}
	if total != 1 {
		t.Errorf("total roots = %d, want 1", total)
	}
	if len(list[0].Replies) != 1 {
		t.Errorf("replies count = %d, want 1", len(list[0].Replies))
	}

	// CountByGame
	n := svc.CountByGame(gameID)
	if n != 2 {
		t.Errorf("CountByGame = %d, want 2", n)
	}

	// 清理
	db.Exec("DELETE FROM comments WHERE game_id = ?", gameID)
}

func TestSocial_Comment_Edit_And_Delete(t *testing.T) {
	db := openTestDB(t)
	svc := comment.New(db, reaction.New(db))

	gameID := "game-edit-test-" + t.Name()
	c, _ := svc.Create(gameID, "author-x", "原始内容", "linear")

	// 编辑（5 分钟内，应成功）
	updated, err := svc.Edit(c.ID, "author-x", "修改后内容")
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if updated.Content != "修改后内容" {
		t.Errorf("content = %q, want '修改后内容'", updated.Content)
	}

	// 他人编辑 → ErrForbidden
	if err := func() error {
		_, err := svc.Edit(c.ID, "other-user", "hack")
		return err
	}(); err != comment.ErrForbidden {
		t.Errorf("other user edit should return ErrForbidden, got: %v", err)
	}

	// 删除（软删除）
	if err := svc.Delete(c.ID, "author-x", ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 删除后不出现在列表
	list, _, _ := svc.ListByGame(gameID, "date_desc", "linear", 20, 0)
	if len(list) != 0 {
		t.Errorf("deleted comment should not appear in list, got %d", len(list))
	}

	db.Exec("DELETE FROM comments WHERE game_id = ?", gameID)
}

// ── Forum ────────────────────────────────────────────────────────────────────

func TestSocial_Forum_CreateAndList(t *testing.T) {
	db := openTestDB(t)
	svc := forum.New(db, reaction.New(db))

	gameID := "game-forum-test"
	authorID := "forum-author"

	post, err := svc.CreatePost(authorID, "测试帖标题 "+t.Name(), "**加粗内容**", []string{gameID}, "discussion")
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.Slug == "" {
		t.Error("Slug should not be empty")
	}
	if post.Content == "" {
		t.Error("Content (rendered HTML) should not be empty")
	}

	// 按 game_tag 过滤
	result, err := svc.ListPosts(gameID, "", "new", "", 20, 0)
	if err != nil {
		t.Fatalf("ListPosts: %v", err)
	}
	if result.Total == 0 {
		t.Error("ListPosts by game_tag returned 0 results")
	}

	// CountByGameTag
	n := svc.CountByGameTag(gameID)
	if n == 0 {
		t.Error("CountByGameTag should be > 0")
	}

	// 清理
	db.Exec("DELETE FROM posts WHERE author_id = ?", authorID)
}

func TestSocial_Forum_Reply(t *testing.T) {
	db := openTestDB(t)
	svc := forum.New(db, reaction.New(db))

	post, err := svc.CreatePost("reply-author", "回复测试帖 "+t.Name(), "内容", nil, "")
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	reply1, err := svc.CreateReply(post.ID, "user-a", "", "第一楼")
	if err != nil {
		t.Fatalf("CreateReply 1: %v", err)
	}
	if reply1.Number != 1 {
		t.Errorf("reply1.Number = %d, want 1", reply1.Number)
	}

	reply2, err := svc.CreateReply(post.ID, "user-b", reply1.ID, "回复一楼")
	if err != nil {
		t.Fatalf("CreateReply 2: %v", err)
	}
	if reply2.Number != 2 {
		t.Errorf("reply2.Number = %d, want 2", reply2.Number)
	}
	if reply2.ParentID != reply1.ID {
		t.Errorf("reply2.ParentID = %q, want %q", reply2.ParentID, reply1.ID)
	}

	// 验证 Post 的反规范化计数
	updated, _ := svc.GetPost(post.ID)
	if updated.RepliesCount != 2 {
		t.Errorf("RepliesCount = %d, want 2", updated.RepliesCount)
	}

	// 清理
	db.Exec("DELETE FROM forum_replies WHERE post_id = ?", post.ID)
	db.Exec("DELETE FROM posts WHERE id = ?", post.ID)
}
