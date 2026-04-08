package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	dbmodels "mvu-backend/internal/core/db"
)

// ErrConcurrentGeneration 当 session.generating=true 且 generation_mode=reject 时返回此错误。
// 调用方（路由层）应将其转换为 HTTP 409 Conflict。
var ErrConcurrentGeneration = errors.New("session is currently generating; retry later")

// Manager 管理 Floor / MessagePage 的生命周期
type Manager struct {
	db *gorm.DB
}

func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

// StartTurn 开始一个新回合
//
// branchID 指定本回合属于哪个分支（空字符串或 "main" 均视为主干）。
// 并发保护（M13）：在 DB 事务内以 FOR UPDATE 锁住 session 行，若 generating=true 返回 ErrConcurrentGeneration。
func (m *Manager) StartTurn(sessionID, userInput, branchID string) (floorID, pageID string, err error) {
	if branchID == "" {
		branchID = "main"
	}
	var floorRes, pageRes string

	txErr := m.db.Transaction(func(tx *gorm.DB) error {
		var sess dbmodels.GameSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&sess, "id = ?", sessionID).Error; err != nil {
			return fmt.Errorf("session not found: %w", err)
		}

		if sess.Generating {
			mode := sess.GenerationMode
			if mode == "" {
				mode = "reject"
			}
			return ErrConcurrentGeneration
		}

		if err := tx.Model(&sess).Update("generating", true).Error; err != nil {
			return fmt.Errorf("set generating: %w", err)
		}

		var maxSeq int
		tx.Model(&dbmodels.Floor{}).
			Where("session_id = ?", sessionID).
			Select("COALESCE(MAX(seq), 0)").
			Scan(&maxSeq)

		floor := dbmodels.Floor{
			SessionID: sessionID,
			Seq:       maxSeq + 1,
			BranchID:  branchID,
			Status:    dbmodels.FloorDraft,
		}
		if err := tx.Create(&floor).Error; err != nil {
			return fmt.Errorf("create floor: %w", err)
		}

		msgs, _ := json.Marshal([]map[string]string{
			{"role": "user", "content": userInput},
		})
		page := dbmodels.MessagePage{
			FloorID:  floor.ID,
			IsActive: true,
			Messages: msgs,
			PageVars: []byte("{}"),
		}
		if err := tx.Create(&page).Error; err != nil {
			return fmt.Errorf("create page: %w", err)
		}

		tx.Model(&floor).Update("status", dbmodels.FloorGenerating)

		floorRes = floor.ID
		pageRes = page.ID
		return nil
	})

	if txErr != nil {
		return "", "", txErr
	}
	return floorRes, pageRes, nil
}

// ClearGenerating 将 session.generating 复位为 false。
// 应在 CommitTurn / FailTurn 后调用（由 game_loop / StreamTurn 负责，而非 Manager 内部，
// 以避免在事务外部做额外查询）。
func (m *Manager) ClearGenerating(sessionID string) {
	m.db.Model(&dbmodels.GameSession{}).
		Where("id = ?", sessionID).
		Update("generating", false)
}

// CommitTurn 生成成功：写入 AI 回复，更新变量，锁定楼层
func (m *Manager) CommitTurn(pageID string, assistantContent string, pageVars map[string]any) error {
	var page dbmodels.MessagePage
	if err := m.db.First(&page, "id = ?", pageID).Error; err != nil {
		return fmt.Errorf("page not found: %w", err)
	}

	// 追加 assistant 消息
	var msgs []map[string]string
	_ = json.Unmarshal(page.Messages, &msgs)
	msgs = append(msgs, map[string]string{"role": "assistant", "content": assistantContent})
	newMsgs, _ := json.Marshal(msgs)

	// 写入 Page 沙箱变量
	newVars, _ := json.Marshal(pageVars)

	if err := m.db.Model(&page).Updates(map[string]any{
		"messages":  newMsgs,
		"page_vars": newVars,
	}).Error; err != nil {
		return fmt.Errorf("update page: %w", err)
	}

	// 锁定楼层
	if err := m.db.Model(&dbmodels.Floor{}).
		Where("id = ?", page.FloorID).
		Update("status", dbmodels.FloorCommitted).Error; err != nil {
		return err
	}

	// 提升 Page 变量到 Session（Chat 级）
	return m.promotePageVarsToSession(page.FloorID, pageVars)
}

// RegenTurn 玩家不满意，在同一楼层创建新 Page（旧 Page 标记 inactive）
func (m *Manager) RegenTurn(floorID, userInput string) (newPageID string, err error) {
	// 1. 将当前 active page 设为 inactive
	m.db.Model(&dbmodels.MessagePage{}).
		Where("floor_id = ? AND is_active = true", floorID).
		Update("is_active", false)

	// 2. 创建新 Page（Page 沙箱全新，不继承旧 Page 变量 —— 这是沙箱的核心保证）
	msgs, _ := json.Marshal([]map[string]string{
		{"role": "user", "content": userInput},
	})
	page := dbmodels.MessagePage{
		FloorID:  floorID,
		IsActive: true,
		Messages: msgs,
		PageVars: []byte("{}"),
	}
	if err = m.db.Create(&page).Error; err != nil {
		return "", fmt.Errorf("regen page: %w", err)
	}

	// 3. 重置楼层状态为 generating
	m.db.Model(&dbmodels.Floor{}).
		Where("id = ?", floorID).
		Update("status", dbmodels.FloorGenerating)

	return page.ID, nil
}

// FailTurn 生成失败，保留现场（floor.status = failed）
func (m *Manager) FailTurn(floorID string, reason string) error {
	return m.db.Model(&dbmodels.Floor{}).
		Where("id = ?", floorID).
		Updates(map[string]any{
			"status": dbmodels.FloorFailed,
		}).Error
}

// GetHistory 获取会话已提交楼层的消息（用于构建 LLM 上下文）。
//
// branchID 为空或 "main" 时只返回主干楼层。
// 非 main 分支时，返回父分支 ≤ OriginSeq 的楼层 + 本分支所有楼层（按 seq 升序合并）。
func (m *Manager) GetHistory(sessionID, branchID string, maxFloors int) ([]map[string]string, error) {
	if branchID == "" {
		branchID = "main"
	}

	var pages []dbmodels.MessagePage
	if branchID == "main" {
		err := m.db.
			Joins("JOIN floors ON floors.id::text = message_pages.floor_id").
			Where("floors.session_id = ? AND floors.status = ? AND floors.branch_id = ? AND message_pages.is_active = true",
				sessionID, dbmodels.FloorCommitted, "main").
			Order("floors.seq ASC").
			Limit(maxFloors).
			Find(&pages).Error
		if err != nil {
			return nil, err
		}
	} else {
		// 查分支元数据，得到父分支和分叉点 seq
		var meta dbmodels.SessionBranch
		if err := m.db.Where("session_id = ? AND branch_id = ?", sessionID, branchID).
			First(&meta).Error; err != nil {
			return nil, fmt.Errorf("branch not found: %w", err)
		}

		// 父分支楼层（seq ≤ OriginSeq）+ 本分支所有楼层，按 seq 升序
		err := m.db.
			Joins("JOIN floors ON floors.id::text = message_pages.floor_id").
			Where(`floors.session_id = ? AND floors.status = ? AND message_pages.is_active = true
			  AND (
			    (floors.branch_id = ? AND floors.seq <= ?)
			    OR floors.branch_id = ?
			  )`,
				sessionID, dbmodels.FloorCommitted,
				meta.ParentBranch, meta.OriginSeq,
				branchID).
			Order("floors.seq ASC").
			Limit(maxFloors).
			Find(&pages).Error
		if err != nil {
			return nil, err
		}
	}

	var history []map[string]string
	for _, page := range pages {
		var msgs []map[string]string
		_ = json.Unmarshal(page.Messages, &msgs)
		history = append(history, msgs...)
	}
	return history, nil
}

// promotePageVarsToSession 将 Page 沙箱变量合并写入 Session 的 Chat 级变量
func (m *Manager) promotePageVarsToSession(floorID string, pageVars map[string]any) error {
	var floor dbmodels.Floor
	if err := m.db.First(&floor, "id = ?", floorID).Error; err != nil {
		return err
	}

	var session dbmodels.GameSession
	if err := m.db.First(&session, "id = ?", floor.SessionID).Error; err != nil {
		return err
	}

	// 读取现有 Chat 变量
	var chatVars map[string]any
	_ = json.Unmarshal(session.Variables, &chatVars)
	if chatVars == nil {
		chatVars = map[string]any{}
	}

	// 合并（Page 变量覆盖 Chat 变量）
	for k, v := range pageVars {
		chatVars[k] = v
	}

	newVars, _ := json.Marshal(chatVars)
	return m.db.Model(&session).Update("variables", newVars).Error
}

// PatchSessionVariables 将 patch 中的键值合并写入 Session.Variables（不删除已有键）。
// 用于在 PlayTurn 完成后写入 ScheduledTurn 冷却记录等额外状态，无需重建整个沙箱。
func (m *Manager) PatchSessionVariables(sessionID string, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	var session dbmodels.GameSession
	if err := m.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return err
	}
	var vars map[string]any
	_ = json.Unmarshal(session.Variables, &vars)
	if vars == nil {
		vars = map[string]any{}
	}
	for k, v := range patch {
		vars[k] = v
	}
	newVars, _ := json.Marshal(vars)
	return m.db.Model(&session).Update("variables", newVars).Error
}

// IncrFloorCount 更新回合计数（用于触发记忆摘要的阈值判断）
func (m *Manager) IncrFloorCount(sessionID string) (int, error) {
	var session dbmodels.GameSession
	if err := m.db.First(&session, "id = ?", sessionID).Error; err != nil {
		return 0, err
	}
	session.FloorCount++
	session.UpdatedAt = time.Now()
	return session.FloorCount, m.db.Save(&session).Error
}

// ── Floor / Page 查询与操作 ────────────────────────────────────────────────────

// FloorWithPage 楼层 + 当前激活页快照（供前端历史浏览用）
type FloorWithPage struct {
	dbmodels.Floor
	ActivePageID string         `json:"active_page_id"`
	Messages     []map[string]string `json:"messages"`    // 激活页的消息（user+assistant）
	PageVars     map[string]any `json:"page_vars"`
	TokenUsed    int            `json:"token_used"`
}

// ListFloors 返回会话的所有楼层（按 seq 升序）。
// branchID 为空时返回全部分支的楼层；非空时只返回该分支的楼层。
func (m *Manager) ListFloors(sessionID, branchID string) ([]FloorWithPage, error) {
	q := m.db.Where("session_id = ?", sessionID)
	if branchID != "" {
		q = q.Where("branch_id = ?", branchID)
	}
	var floors []dbmodels.Floor
	if err := q.Order("seq ASC").Find(&floors).Error; err != nil {
		return nil, err
	}

	result := make([]FloorWithPage, 0, len(floors))
	for _, f := range floors {
		fp := FloorWithPage{Floor: f}

		var page dbmodels.MessagePage
		if err := m.db.Where("floor_id = ? AND is_active = true", f.ID).
			First(&page).Error; err == nil {
			fp.ActivePageID = page.ID
			fp.TokenUsed = page.TokenUsed
			_ = json.Unmarshal(page.Messages, &fp.Messages)
			var vars map[string]any
			_ = json.Unmarshal(page.PageVars, &vars)
			fp.PageVars = vars
		}
		result = append(result, fp)
	}
	return result, nil
}

// ListPages 返回一个楼层的所有 MessagePage（Swipe 列表）
func (m *Manager) ListPages(floorID string) ([]dbmodels.MessagePage, error) {
	var pages []dbmodels.MessagePage
	err := m.db.Where("floor_id = ?", floorID).
		Order("created_at ASC").Find(&pages).Error
	return pages, err
}

// SetActivePage 将指定页设为激活（Swipe 选择）。
// 同一楼层其余页设为 inactive；若目标页已 committed，其变量提升至 Session。
func (m *Manager) SetActivePage(floorID, pageID string) error {
	// 先把所有页设为 inactive
	if err := m.db.Model(&dbmodels.MessagePage{}).
		Where("floor_id = ?", floorID).
		Update("is_active", false).Error; err != nil {
		return fmt.Errorf("deactivate pages: %w", err)
	}
	// 激活目标页
	if err := m.db.Model(&dbmodels.MessagePage{}).
		Where("id = ? AND floor_id = ?", pageID, floorID).
		Update("is_active", true).Error; err != nil {
		return fmt.Errorf("activate page: %w", err)
	}
	// 如果楼层已 committed，把目标页的变量提升到 Session
	var floor dbmodels.Floor
	if err := m.db.First(&floor, "id = ?", floorID).Error; err != nil {
		return nil // 找不到楼层不影响页激活
	}
	if floor.Status == dbmodels.FloorCommitted {
		var page dbmodels.MessagePage
		if err := m.db.First(&page, "id = ?", pageID).Error; err == nil {
			var pageVars map[string]any
			_ = json.Unmarshal(page.PageVars, &pageVars)
			if len(pageVars) > 0 {
				_ = m.promotePageVarsToSession(floorID, pageVars)
			}
		}
	}
	return nil
}

// ── 分支管理（P-3G）────────────────────────────────────────────────────────────

// BranchInfo 分支信息（用于 GET /sessions/:id/branches 响应）
type BranchInfo struct {
	BranchID     string    `json:"branch_id"`
	ParentBranch string    `json:"parent_branch"`
	OriginSeq    int       `json:"origin_seq"`
	FloorCount   int       `json:"floor_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// ListBranches 列出 session 的所有分支（含隐式 main 分支）。
func (m *Manager) ListBranches(sessionID string) ([]BranchInfo, error) {
	// main 分支：统计所有 branch_id="main" 的楼层数
	var mainCount int64
	m.db.Model(&dbmodels.Floor{}).
		Where("session_id = ? AND branch_id = 'main'", sessionID).
		Count(&mainCount)

	result := []BranchInfo{{
		BranchID:     "main",
		ParentBranch: "",
		OriginSeq:    0,
		FloorCount:   int(mainCount),
	}}

	// 非 main 分支
	var metas []dbmodels.SessionBranch
	if err := m.db.Where("session_id = ?", sessionID).
		Order("created_at ASC").Find(&metas).Error; err != nil {
		return nil, err
	}
	for _, meta := range metas {
		var cnt int64
		m.db.Model(&dbmodels.Floor{}).
			Where("session_id = ? AND branch_id = ?", sessionID, meta.BranchID).
			Count(&cnt)
		result = append(result, BranchInfo{
			BranchID:     meta.BranchID,
			ParentBranch: meta.ParentBranch,
			OriginSeq:    meta.OriginSeq,
			FloorCount:   int(cnt),
			CreatedAt:    meta.CreatedAt,
		})
	}
	return result, nil
}

// CreateBranch 从 fid 所属楼层的 seq 创建新分支，返回新 branch_id。
func (m *Manager) CreateBranch(sessionID, fromFloorID string) (string, error) {
	var floor dbmodels.Floor
	if err := m.db.First(&floor, "id = ? AND session_id = ?", fromFloorID, sessionID).Error; err != nil {
		return "", fmt.Errorf("floor not found: %w", err)
	}
	if floor.BranchID != "main" {
		return "", fmt.Errorf("can only branch from main branch floors (got branch_id=%q)", floor.BranchID)
	}

	// 生成唯一 branch_id（时间戳 + 随机 4 位 hex）
	branchID := fmt.Sprintf("branch-%d%04x", time.Now().UnixMilli()%100000, rand.Intn(0x10000))

	meta := dbmodels.SessionBranch{
		SessionID:    sessionID,
		BranchID:     branchID,
		ParentBranch: "main",
		OriginSeq:    floor.Seq,
	}
	if err := m.db.Create(&meta).Error; err != nil {
		return "", fmt.Errorf("create branch: %w", err)
	}
	return branchID, nil
}

// DeleteBranch 删除指定分支（不能删除 main）。
// 同时删除该分支下的所有 Floor 和 MessagePage。
func (m *Manager) DeleteBranch(sessionID, branchID string) error {
	if branchID == "main" || branchID == "" {
		return fmt.Errorf("cannot delete main branch")
	}
	// 验证分支存在
	var meta dbmodels.SessionBranch
	if err := m.db.Where("session_id = ? AND branch_id = ?", sessionID, branchID).
		First(&meta).Error; err != nil {
		return fmt.Errorf("branch not found: %w", err)
	}

	// 删除分支下所有楼层的页面
	var floors []dbmodels.Floor
	m.db.Where("session_id = ? AND branch_id = ?", sessionID, branchID).Find(&floors)
	for _, f := range floors {
		m.db.Where("floor_id = ?", f.ID).Delete(&dbmodels.MessagePage{})
	}
	m.db.Where("session_id = ? AND branch_id = ?", sessionID, branchID).Delete(&dbmodels.Floor{})

	// 删除分支元数据
	return m.db.Where("session_id = ? AND branch_id = ?", sessionID, branchID).
		Delete(&dbmodels.SessionBranch{}).Error
}
