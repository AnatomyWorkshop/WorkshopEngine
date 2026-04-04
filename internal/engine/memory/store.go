package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
)

// StoreConfig 记忆系统可配置参数（全部有默认值，零值均安全）。
type StoreConfig struct {
	// HalfLifeDays 记忆衰减半衰期（天）。默认 7.0。
	// 对应 TH 的 MemoryDecayConfig.halfLife。
	HalfLifeDays float64

	// MinDecayFactor 最小衰减系数（0–1）。默认 0（可衰减至 0）。
	// > 0 时为老记忆保底权重，防止全部被抛弃。
	// 对应 TH 的 MemoryDecayConfig.minFactor。
	MinDecayFactor float64

	// MaxCandidates 注入前从 DB 取的最大候选条数。默认 50。
	MaxCandidates int

	// ConsolidationInstruction 发送给 LLM 的摘要整合指令头（纯文本）。
	// 留空则使用内置默认值。支持 {existing_memory} 和 {dialogue} 占位符。
	ConsolidationInstruction string

	// FactPrefix 整合结果中事实条目的行前缀（不含空格）。默认 "事实："。
	// ParseConsolidationResult 同时兼容 ASCII 冒号变体（"事实:"）。
	FactPrefix string
}

func (c *StoreConfig) applyDefaults() {
	if c.HalfLifeDays <= 0 {
		c.HalfLifeDays = 7.0
	}
	if c.MaxCandidates <= 0 {
		c.MaxCandidates = 50
	}
	if c.FactPrefix == "" {
		c.FactPrefix = "事实："
	}
}

// Store 记忆系统的持久化与检索
type Store struct {
	db  *gorm.DB
	cfg StoreConfig
}

// NewStore 创建记忆存储。可选传入 StoreConfig，不传则使用默认值。
func NewStore(db *gorm.DB, cfgs ...StoreConfig) *Store {
	cfg := StoreConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}
	cfg.applyDefaults()
	return &Store{db: db, cfg: cfg}
}

// SaveFromParser 从 parser 的 Summary 字段异步写入记忆（由 goroutine 调用）
func (s *Store) SaveFromParser(sessionID, summary string, sourceFloor int) error {
	if summary == "" {
		return nil
	}
	mem := dbmodels.Memory{
		SessionID:   sessionID,
		Content:     summary,
		Type:        dbmodels.MemorySummary,
		Importance:  1.0,
		SourceFloor: sourceFloor,
	}
	return s.db.Create(&mem).Error
}

// SaveFact 写入一条明确事实记忆（如：玩家拿了钥匙）
func (s *Store) SaveFact(sessionID, content string, importance float64) error {
	mem := dbmodels.Memory{
		SessionID:  sessionID,
		Content:    content,
		Type:       dbmodels.MemoryFact,
		Importance: importance,
	}
	return s.db.Create(&mem).Error
}

// GetForInjection 按重要度衰减排序，在 Token 预算内取最相关的记忆拼成注入文本
// 复刻 TH 的衰减排序逻辑（按时间半衰期降权）
func (s *Store) GetForInjection(sessionID string, tokenBudget int) (string, error) {
	var mems []dbmodels.Memory
	err := s.db.Where("session_id = ? AND deprecated = false", sessionID).
		Order("importance DESC, created_at DESC").
		Limit(s.cfg.MaxCandidates).
		Find(&mems).Error
	if err != nil {
		return "", err
	}
	if len(mems) == 0 {
		return "", nil
	}

	// 计算衰减分数（指数半衰期）
	now := time.Now()
	type scoredMem struct {
		mem   dbmodels.Memory
		score float64
	}
	var scored []scoredMem
	for _, m := range mems {
		ageDays := now.Sub(m.CreatedAt).Hours() / 24.0
		decay := math.Pow(0.5, ageDays/s.cfg.HalfLifeDays)
		if s.cfg.MinDecayFactor > 0 && decay < s.cfg.MinDecayFactor {
			decay = s.cfg.MinDecayFactor
		}
		scored = append(scored, scoredMem{mem: m, score: m.Importance * decay})
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 按 Token 预算（粗估：1 token ≈ 1.5 汉字）裁剪
	maxChars := tokenBudget * 2
	var parts []string
	used := 0
	for _, sm := range scored {
		content := sm.mem.Content
		if used+len(content) > maxChars {
			break
		}
		parts = append(parts, content)
		used += len(content) + 1
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n"), nil
}

// DeprecateFacts 标记过时的记忆（由 Worker 定期调用）
func (s *Store) DeprecateFacts(sessionID string, outdatedIDs []string) error {
	return s.db.Model(&dbmodels.Memory{}).
		Where("id IN ? AND session_id = ?", outdatedIDs, sessionID).
		Update("deprecated", true).Error
}

// ConsolidationInput 摘要整合任务的输入
type ConsolidationInput struct {
	SessionID   string
	RecentFloor int // 从第几楼层开始整合
}

// defaultConsolidationInstruction 内置整合指令（可通过 StoreConfig.ConsolidationInstruction 覆盖）。
const defaultConsolidationInstruction = "以下是一段游戏剧情对话，请提取关键事实和剧情摘要，用简洁的语言列出（每条不超过50字）："

// BuildConsolidationPrompt 构建摘要整合用的 Prompt（供 Worker 调用廉价模型生成新摘要）
// 复刻 TH Memory Worker 的核心逻辑
func (s *Store) BuildConsolidationPrompt(sessionID string, recentDialogue []map[string]string) (string, error) {
	// 读取已有摘要
	existing, err := s.GetForInjection(sessionID, 800)
	if err != nil {
		return "", err
	}

	// 构建对话摘要请求
	var dialogueParts []string
	for _, msg := range recentDialogue {
		if msg["role"] == "system" {
			continue
		}
		dialogueParts = append(dialogueParts, fmt.Sprintf("[%s]: %s", msg["role"], msg["content"]))
	}
	dialogue := strings.Join(dialogueParts, "\n")

	instruction := s.cfg.ConsolidationInstruction
	if instruction == "" {
		instruction = defaultConsolidationInstruction
	}

	prompt := instruction + "\n\n"
	if existing != "" {
		prompt += "【已有记忆背景】\n" + existing + "\n\n"
	}
	prompt += "【近期对话】\n" + dialogue + "\n\n"
	prompt += "输出格式：\n<Summary>一句话整体摘要</Summary>\n每条关键事实单独一行，以「事实：」开头。"

	return prompt, nil
}

// ParseConsolidationResult 解析摘要整合结果并写入库
func (s *Store) ParseConsolidationResult(sessionID string, result string, sourceFloor int) error {
	// 提取 <Summary> 标签
	var summary string
	if idx := strings.Index(result, "<Summary>"); idx >= 0 {
		end := strings.Index(result, "</Summary>")
		if end > idx {
			summary = strings.TrimSpace(result[idx+9 : end])
		}
	}

	// 提取事实列表（支持配置的前缀和 ASCII 冒号变体）
	factPrefix := s.cfg.FactPrefix
	factPrefixAlt := strings.TrimSuffix(factPrefix, "：") + ":" // 兼容 ASCII 冒号
	var errs []string
	for _, line := range strings.Split(result, "\n") {
		line = strings.TrimSpace(line)
		var fact string
		switch {
		case strings.HasPrefix(line, factPrefix):
			fact = strings.TrimSpace(line[len(factPrefix):])
		case strings.HasPrefix(line, factPrefixAlt):
			fact = strings.TrimSpace(line[len(factPrefixAlt):])
		}
		if fact == "" {
			continue
		}
		if err := s.SaveFact(sessionID, fact, 0.9); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// 整体摘要
	if summary != "" {
		if err := s.SaveFromParser(sessionID, summary, sourceFloor); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("consolidation partial errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// UpdateSessionSummaryCache 更新 game_sessions.memory_summary 快照
// （Pipeline 的 MemoryNode 直接读这个字段，避免每次请求都跑复杂查询）
func (s *Store) UpdateSessionSummaryCache(sessionID string, summary string) error {
	data, _ := json.Marshal(summary)
	_ = data
	return s.db.Model(&dbmodels.GameSession{}).
		Where("id = ?", sessionID).
		Update("memory_summary", summary).Error
}

// ── 记忆 CRUD（供 HTTP 层管理用）────────────────────────────────────────────

// ListMemories 返回会话的所有未废弃记忆（按创建时间倒序）
func (s *Store) ListMemories(sessionID string) ([]dbmodels.Memory, error) {
	var mems []dbmodels.Memory
	err := s.db.Where("session_id = ? AND deprecated = false", sessionID).
		Order("created_at DESC").Find(&mems).Error
	return mems, err
}

// UpdateMemory 部分更新记忆字段（content / importance / type）
func (s *Store) UpdateMemory(memID, sessionID string, updates map[string]any) (*dbmodels.Memory, error) {
	// 只允许更新安全字段
	allowed := map[string]struct{}{
		"content": {}, "importance": {}, "type": {}, "deprecated": {},
	}
	safe := make(map[string]any, len(updates))
	for k, v := range updates {
		if _, ok := allowed[k]; ok {
			safe[k] = v
		}
	}
	if len(safe) == 0 {
		var mem dbmodels.Memory
		err := s.db.First(&mem, "id = ? AND session_id = ?", memID, sessionID).Error
		return &mem, err
	}
	if err := s.db.Model(&dbmodels.Memory{}).
		Where("id = ? AND session_id = ?", memID, sessionID).
		Updates(safe).Error; err != nil {
		return nil, err
	}
	var mem dbmodels.Memory
	err := s.db.First(&mem, "id = ? AND session_id = ?", memID, sessionID).Error
	return &mem, err
}

// DeleteMemory 软删除（标记 deprecated=true）；传 hard=true 则物理删除
func (s *Store) DeleteMemory(memID, sessionID string, hard bool) error {
	if hard {
		return s.db.Where("id = ? AND session_id = ?", memID, sessionID).
			Delete(&dbmodels.Memory{}).Error
	}
	return s.db.Model(&dbmodels.Memory{}).
		Where("id = ? AND session_id = ?", memID, sessionID).
		Update("deprecated", true).Error
}

// ── 记忆维护策略 ──────────────────────────────────────────────────────────────

// DeprecateOldMemories 将 olderThanDays 天之前创建的摘要记忆标记为 deprecated。
// 仅处理 type=summary 的条目；fact 类型的记忆需要业务逻辑明确废弃。
// 对应 TH 的 MemoryMaintenancePolicy.deprecateAfterDays。
func (s *Store) DeprecateOldMemories(sessionID string, olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := s.db.Model(&dbmodels.Memory{}).
		Where("session_id = ? AND deprecated = false AND type = ? AND created_at < ?",
			sessionID, dbmodels.MemorySummary, cutoff).
		Update("deprecated", true)
	return result.RowsAffected, result.Error
}

// PurgeDeprecatedMemories 物理删除已 deprecated 且超过 olderThanDays 天的记忆。
// 对应 TH 的 MemoryMaintenancePolicy.purgeAfterDays。
func (s *Store) PurgeDeprecatedMemories(sessionID string, olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := s.db.Where("session_id = ? AND deprecated = true AND updated_at < ?",
		sessionID, cutoff).Delete(&dbmodels.Memory{})
	return result.RowsAffected, result.Error
}

// FindSessionsNeedingConsolidation 查找需要记忆整合的会话
// 条件：floor_count >= triggerRounds，且最新记忆 source_floor 与当前 floor_count 差距 >= triggerRounds
// （避免对刚整合过的会话重复触发）
// batchSize 控制每次最多扫描多少个会话（来自 MEMORY_WORKER_BATCH_SIZE 配置）。
func (s *Store) FindSessionsNeedingConsolidation(triggerRounds, batchSize int) ([]dbmodels.GameSession, error) {
	if batchSize <= 0 {
		batchSize = 20
	}
	var sessions []dbmodels.GameSession
	err := s.db.Raw(`
		SELECT gs.* FROM game_sessions gs
		LEFT JOIN (
			SELECT session_id, MAX(source_floor) AS max_floor
			FROM memories
			WHERE deprecated = false
			GROUP BY session_id
		) m ON gs.id = m.session_id
		WHERE gs.floor_count >= ?
		  AND (m.max_floor IS NULL OR gs.floor_count - m.max_floor >= ?)
		ORDER BY gs.floor_count DESC
		LIMIT ?
	`, triggerRounds, triggerRounds, batchSize).Scan(&sessions).Error
	return sessions, err
}

// ── 全局维护（对应 TH MemoryMaintenancePolicy）────────────────────────────────

// DeprecateOldMemoriesGlobal 将所有 session 中超过 olderThanDays 天的 summary 记忆标记为 deprecated。
// 对应 TH 的 MemoryMaintenancePolicy.deprecateAfterDays，由 Worker 定期调用。
func (s *Store) DeprecateOldMemoriesGlobal(olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := s.db.Model(&dbmodels.Memory{}).
		Where("deprecated = false AND type = ? AND created_at < ?", dbmodels.MemorySummary, cutoff).
		Update("deprecated", true)
	return result.RowsAffected, result.Error
}

// PurgeDeprecatedMemoriesGlobal 物理删除所有 session 中 deprecated 且超过 olderThanDays 天的记忆。
// 对应 TH 的 MemoryMaintenancePolicy.purgeAfterDays，由 Worker 定期调用。
func (s *Store) PurgeDeprecatedMemoriesGlobal(olderThanDays int) (int64, error) {
	if olderThanDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := s.db.Where("deprecated = true AND updated_at < ?", cutoff).
		Delete(&dbmodels.Memory{})
	return result.RowsAffected, result.Error
}
