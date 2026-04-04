package session

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
)

// Manager 管理 Floor / MessagePage 的生命周期
type Manager struct {
	db *gorm.DB
}

func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

// StartTurn 开始一个新回合
// 1. 创建 Floor（draft）
// 2. 创建第一个 MessagePage（active）
// 3. 将 Floor 状态推进到 generating
func (m *Manager) StartTurn(sessionID, userInput string) (floorID, pageID string, err error) {
	// 计算下一个楼层序号
	var maxSeq int
	m.db.Model(&dbmodels.Floor{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").
		Scan(&maxSeq)

	floor := dbmodels.Floor{
		SessionID: sessionID,
		Seq:       maxSeq + 1,
		Status:    dbmodels.FloorDraft,
	}
	if err = m.db.Create(&floor).Error; err != nil {
		return "", "", fmt.Errorf("create floor: %w", err)
	}

	// 用户输入作为第一条消息写入 Page
	msgs, _ := json.Marshal([]map[string]string{
		{"role": "user", "content": userInput},
	})

	page := dbmodels.MessagePage{
		FloorID:  floor.ID,
		IsActive: true,
		Messages: msgs,
		PageVars: []byte("{}"),
	}
	if err = m.db.Create(&page).Error; err != nil {
		return "", "", fmt.Errorf("create page: %w", err)
	}

	// 推进状态到 generating
	m.db.Model(&floor).Update("status", dbmodels.FloorGenerating)

	return floor.ID, page.ID, nil
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

// GetHistory 获取会话中所有已提交楼层的消息（用于构建 LLM 上下文）
func (m *Manager) GetHistory(sessionID string, maxFloors int) ([]map[string]string, error) {
	var pages []dbmodels.MessagePage
	err := m.db.
		Joins("JOIN floors ON floors.id = message_pages.floor_id").
		Where("floors.session_id = ? AND floors.status = ? AND message_pages.is_active = true", sessionID, dbmodels.FloorCommitted).
		Order("floors.seq ASC").
		Limit(maxFloors).
		Find(&pages).Error
	if err != nil {
		return nil, err
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

// ListFloors 返回会话的所有楼层（按 seq 升序），每条附带当前激活页的消息摘要。
func (m *Manager) ListFloors(sessionID string) ([]FloorWithPage, error) {
	var floors []dbmodels.Floor
	if err := m.db.Where("session_id = ?", sessionID).
		Order("seq ASC").Find(&floors).Error; err != nil {
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
