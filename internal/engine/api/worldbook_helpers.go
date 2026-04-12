package api

import (
	"encoding/json"
	"math/rand"
	"time"

	dbmodels "mvu-backend/internal/core/db"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// wbOverrideRow 对应 player_worldbook_overrides 表的查询结果（不 import platform/library）
type wbOverrideRow struct {
	ID             string
	GameID         string
	UserID         string
	EntryID        string
	Content        *string
	Enabled        *bool
	Keys           datatypes.JSON
	SecondaryKeys  datatypes.JSON
	SecondaryLogic *string
	Constant       *bool
	Priority       *int
	IsNew          bool
}

// applyWorldbookOverrides 将玩家覆盖合并到原始词条列表。
// 直接查 player_worldbook_overrides 表，不 import platform/library 包。
func applyWorldbookOverrides(db *gorm.DB, gameID, userID string, entries []dbmodels.WorldbookEntry) []dbmodels.WorldbookEntry {
	var rows []wbOverrideRow
	db.Table("player_worldbook_overrides").
		Where("game_id = ? AND user_id = ?", gameID, userID).
		Scan(&rows)
	if len(rows) == 0 {
		return entries
	}

	overrideMap := map[string]wbOverrideRow{}
	var newRows []wbOverrideRow
	for _, r := range rows {
		if r.IsNew {
			newRows = append(newRows, r)
		} else {
			overrideMap[r.EntryID] = r
		}
	}

	result := make([]dbmodels.WorldbookEntry, 0, len(entries)+len(newRows))
	for _, e := range entries {
		if o, ok := overrideMap[e.ID]; ok {
			if o.Content != nil {
				e.Content = *o.Content
			}
			if o.Enabled != nil {
				e.Enabled = *o.Enabled
			}
			if len(o.Keys) > 0 {
				e.Keys = o.Keys
			}
			if len(o.SecondaryKeys) > 0 {
				e.SecondaryKeys = o.SecondaryKeys
			}
			if o.SecondaryLogic != nil {
				e.SecondaryLogic = *o.SecondaryLogic
			}
			if o.Constant != nil {
				e.Constant = *o.Constant
			}
			if o.Priority != nil {
				e.Priority = *o.Priority
			}
		}
		result = append(result, e)
	}

	// 追加玩家新增词条（is_new=true）
	for _, r := range newRows {
		content := ""
		if r.Content != nil {
			content = *r.Content
		}
		enabled := true
		if r.Enabled != nil {
			enabled = *r.Enabled
		}
		constant := false
		if r.Constant != nil {
			constant = *r.Constant
		}
		priority := 0
		if r.Priority != nil {
			priority = *r.Priority
		}
		secLogic := "and_any"
		if r.SecondaryLogic != nil {
			secLogic = *r.SecondaryLogic
		}
		entry := dbmodels.WorldbookEntry{
			ID:             r.ID,
			GameID:         gameID,
			Content:        content,
			Enabled:        enabled,
			Constant:       constant,
			Priority:       priority,
			SecondaryLogic: secLogic,
			Position:       "before_template",
			Probability:    100,
		}
		if len(r.Keys) > 0 {
			entry.Keys = r.Keys
		}
		if len(r.SecondaryKeys) > 0 {
			entry.SecondaryKeys = r.SecondaryKeys
		}
		result = append(result, entry)
	}
	return result
}

// applyStickyAndProbability 处理 sticky 持续激活和 probability 随机过滤。
//
// sticky 逻辑：
//   - 读取 session.StickyEntries（map[entry_id]remaining_turns）
//   - remaining > 0 的词条强制激活，remaining 减 1
//   - 本轮新激活且 sticky > 0 的词条写入 map，remaining = sticky
//
// probability 逻辑：
//   - Constant=true 的词条跳过概率检查（必触发）
//   - 其余词条按 Probability/100 概率决定是否保留
//
// 返回过滤后的词条列表和更新后的 stickyMap（调用方负责持久化）。
func applyStickyAndProbability(
	entries []dbmodels.WorldbookEntry,
	stickyJSON []byte,
	rng *rand.Rand,
) (filtered []dbmodels.WorldbookEntry, updatedStickyMap map[string]int) {
	// 解析当前 sticky 状态
	stickyMap := map[string]int{}
	if len(stickyJSON) > 0 {
		_ = json.Unmarshal(stickyJSON, &stickyMap)
	}

	nextStickyMap := map[string]int{}
	filtered = make([]dbmodels.WorldbookEntry, 0, len(entries))

	for _, e := range entries {
		keep := false

		// sticky 持续激活：remaining > 0 强制保留
		if remaining, ok := stickyMap[e.ID]; ok && remaining > 0 {
			keep = true
			if remaining-1 > 0 {
				nextStickyMap[e.ID] = remaining - 1
			}
			// remaining 归零时不写入 nextStickyMap，自然过期
		}

		if !keep {
			// Constant 词条跳过概率检查
			if e.Constant {
				keep = true
			} else {
				prob := e.Probability
				if prob <= 0 {
					prob = 100 // 默认 100（必触发），兼容旧数据
				}
				keep = prob >= 100 || rng.Intn(100) < prob
			}
		}

		if keep {
			filtered = append(filtered, e)
			// 新激活且有 sticky 的词条：写入 nextStickyMap
			if e.Sticky > 0 {
				if _, alreadyTracked := stickyMap[e.ID]; !alreadyTracked {
					// 首次激活，设置 remaining = sticky（下一轮开始计数）
					nextStickyMap[e.ID] = e.Sticky
				} else if _, stillActive := nextStickyMap[e.ID]; !stillActive {
					// 已在 stickyMap 但本轮是新触发（非 sticky 续期），重置
					nextStickyMap[e.ID] = e.Sticky
				}
			}
		}
	}

	return filtered, nextStickyMap
}

// newRng 创建一个基于当前时间的随机数生成器（每回合独立）
func newRng() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
}
