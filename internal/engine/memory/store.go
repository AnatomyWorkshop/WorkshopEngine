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
	"mvu-backend/internal/engine/tokenizer"
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

	// FactPrefix 旧格式回退解析中事实条目的行前缀（不含空格）。默认 "事实："。
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

// SaveFact 写入一条明确事实记忆（如：玩家拿了钥匙），不带 fact_key（自由文本）
func (s *Store) SaveFact(sessionID, content string, importance float64) error {
	mem := dbmodels.Memory{
		SessionID:  sessionID,
		Content:    content,
		Type:       dbmodels.MemoryFact,
		Importance: importance,
	}
	return s.db.Create(&mem).Error
}

// UpsertFact 按 factKey 更新或新建一条结构化事实。
//
// 行为：
//   - 若该 session + factKey 已有未废弃记录：将旧行 deprecated=true，再新建一行。
//     返回 (newID, oldID, nil)，调用方可据此写入 "updates" 关系边。
//   - 若无已有记录：直接新建一行。
//     返回 (newID, "", nil)。
//
// stageTags 为阶段标签列表（空 slice = 无阶段限制）。
func (s *Store) UpsertFact(sessionID, factKey, content string, importance float64, sourceFloor int, stageTags []string) (newID, oldID string, err error) {
	var existing dbmodels.Memory
	findErr := s.db.Where("session_id = ? AND fact_key = ? AND deprecated = false", sessionID, factKey).
		First(&existing).Error
	if findErr == nil {
		// 旧行存在 → 废弃旧行
		oldID = existing.ID
		if err = s.db.Model(&existing).Update("deprecated", true).Error; err != nil {
			return "", "", err
		}
	}
	// 编码 stage_tags
	if stageTags == nil {
		stageTags = []string{}
	}
	tagsJSON, _ := json.Marshal(stageTags)
	// 新建行
	newMem := &dbmodels.Memory{
		SessionID:   sessionID,
		FactKey:     factKey,
		Content:     content,
		Type:        dbmodels.MemoryFact,
		Importance:  importance,
		SourceFloor: sourceFloor,
		StageTags:   tagsJSON,
	}
	if err = s.db.Create(newMem).Error; err != nil {
		return "", "", err
	}
	return newMem.ID, oldID, nil
}

// DeprecateFactsByKey 按 factKey 列表批量废弃事实，返回被废弃的记录 ID 列表（供写入 edge 用）。
func (s *Store) DeprecateFactsByKey(sessionID string, keys []string) ([]dbmodels.Memory, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	var mems []dbmodels.Memory
	if err := s.db.Where("session_id = ? AND fact_key IN ? AND deprecated = false", sessionID, keys).
		Find(&mems).Error; err != nil {
		return nil, err
	}
	if len(mems) == 0 {
		return nil, nil
	}
	ids := make([]string, len(mems))
	for i, m := range mems {
		ids[i] = m.ID
	}
	if err := s.db.Model(&dbmodels.Memory{}).
		Where("id IN ?", ids).
		Update("deprecated", true).Error; err != nil {
		return nil, err
	}
	return mems, nil
}

// GetForInjection 按重要度衰减排序，在 Token 预算内取最相关的记忆拼成注入文本。
// currentStage 对应 ctx.Variables["game_stage"]：
//   - 空字符串 → 不过滤（注入所有未废弃记忆，向后兼容）
//   - 非空字符串 → 只注入 stage_tags 为空（无阶段限制）或包含 currentStage 的条目
//
// 复刻 TH 的衰减排序逻辑（按时间半衰期降权）
func (s *Store) GetForInjection(sessionID string, tokenBudget int, currentStage string) (string, error) {
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

	// 阶段过滤：stage_tags 为空（无限制）或包含 currentStage 的条目才注入
	if currentStage != "" {
		filtered := mems[:0:0]
		for _, m := range mems {
			if stageMatches(m.StageTags, currentStage) {
				filtered = append(filtered, m)
			}
		}
		mems = filtered
		if len(mems) == 0 {
			return "", nil
		}
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

	// 按 Token 预算裁剪（使用启发式 tokenizer，比字节估算更准确）
	usedTokens := 0
	var parts []string
	for _, sm := range scored {
		line := formatMemoryLine(sm.mem)
		est := tokenizer.Estimate(line)
		if usedTokens+est > tokenBudget {
			break
		}
		parts = append(parts, line)
		usedTokens += est + 1 // +1 for separator token
	}

	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, "\n"), nil
}

// stageToTags 将单个 stage 字符串转为 stage_tags 切片（空字符串 → 空切片 = 无阶段限制）。
func stageToTags(stage string) []string {
	if stage == "" {
		return []string{}
	}
	return []string{stage}
}

// stageMatches 判断记忆条目的 stage_tags 是否允许在 currentStage 下注入。
// stage_tags 为空数组 → 无阶段限制，始终返回 true。
// stage_tags 非空 → 仅当 currentStage 包含在标签列表中时返回 true。
func stageMatches(stageTagsJSON []byte, currentStage string) bool {
	if len(stageTagsJSON) == 0 {
		return true
	}
	var tags []string
	if err := json.Unmarshal(stageTagsJSON, &tags); err != nil || len(tags) == 0 {
		return true // 解析失败或空数组 → 无限制
	}
	for _, t := range tags {
		if t == currentStage {
			return true
		}
	}
	return false
}

// formatMemoryLine 将记忆条目格式化为注入文本的一行。
// type=fact 且 fact_key 非空时加 [key] 前缀，方便 Narrator LLM 与后续整合时的 key 对应。
func formatMemoryLine(m dbmodels.Memory) string {
	if m.Type == dbmodels.MemoryFact && m.FactKey != "" {
		return fmt.Sprintf("[%s] %s", m.FactKey, m.Content)
	}
	return m.Content
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

// consolidationJSONInstruction 结构化整合指令（JSON 输出模式，优先使用）。
const consolidationJSONInstruction = `你是游戏记忆整合助手。根据下方的【已有事实】和【近期对话】，输出一个 JSON 对象，格式严格如下：

{
  "turn_summary": "一句话整体叙事摘要（不超过60字）",
  "facts_add":    [{"key": "唯一键（英文蛇形命名，如 player_affinity）", "content": "事实描述"}],
  "facts_update": [{"key": "已有事实的 key", "content": "更新后的描述"}],
  "facts_deprecate": ["需要废弃的事实 key1", "key2"]
}

规则：
- key 必须是英文蛇形命名（a-z 和下划线），全局唯一，不能与已有 key 重复（除非是 facts_update）
- 已有事实如果仍然准确，不需要出现在任何数组中
- 仅输出 JSON，不要有任何前缀或解释文字`

// BuildConsolidationPrompt 构建摘要整合用的 Prompt（供 Worker 调用廉价模型生成新摘要）
// 复刻 TH Memory Worker 的核心逻辑
func (s *Store) BuildConsolidationPrompt(sessionID string, recentDialogue []map[string]string) (string, error) {
	// 读取已有未废弃的事实（含 key）
	var existingFacts []dbmodels.Memory
	if err := s.db.Where("session_id = ? AND deprecated = false AND type = ?",
		sessionID, dbmodels.MemoryFact).
		Order("importance DESC, created_at DESC").
		Limit(30).Find(&existingFacts).Error; err != nil {
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

	// 使用结构化 JSON 指令
	prompt := consolidationJSONInstruction + "\n\n"

	if len(existingFacts) > 0 {
		prompt += "【已有事实】\n"
		for _, f := range existingFacts {
			if f.FactKey != "" {
				prompt += fmt.Sprintf("- [%s] %s\n", f.FactKey, f.Content)
			} else {
				prompt += fmt.Sprintf("- %s\n", f.Content)
			}
		}
		prompt += "\n"
	}

	prompt += "【近期对话】\n" + dialogue

	return prompt, nil
}

// consolidationJSON 是 LLM 结构化整合输出的反序列化目标。
type consolidationJSON struct {
	TurnSummary    string      `json:"turn_summary"`
	FactsAdd       []factEntry `json:"facts_add"`
	FactsUpdate    []factEntry `json:"facts_update"`
	FactsDeprecate []string    `json:"facts_deprecate"`
}

type factEntry struct {
	Key     string `json:"key"`
	Content string `json:"content"`
	Stage   string `json:"stage,omitempty"` // 可选阶段标签（如 "act_1"），写入 stage_tags
}

// ParseConsolidationResult 解析整合结果并写入库。
//
// 优先解析结构化 JSON（三路操作：add / update / deprecate）；
// 若 JSON 解析失败，回退到旧格式（<Summary> + 事实：前缀行）保证向后兼容。
func (s *Store) ParseConsolidationResult(sessionID string, result string, sourceFloor int) error {
	// ── 尝试 JSON 解析 ──────────────────────────────────────────────
	// 找第一个 '{' 和最后一个 '}'，提取 JSON 子串（LLM 可能在前后加说明文字）
	start := strings.Index(result, "{")
	end := strings.LastIndex(result, "}")
	if start >= 0 && end > start {
		var out consolidationJSON
		if err := json.Unmarshal([]byte(result[start:end+1]), &out); err == nil {
			return s.applyStructuredResult(sessionID, sourceFloor, out)
		}
	}

	// ── 回退：旧格式解析 ─────────────────────────────────────────────
	return s.parseLegacyResult(sessionID, result, sourceFloor)
}

// applyStructuredResult 把结构化 JSON 三路操作写入库，并写入关系边。
func (s *Store) applyStructuredResult(sessionID string, sourceFloor int, out consolidationJSON) error {
	var errs []string

	// 1. 废弃（LLM 明确指定废弃的 key）— 不写 edge，纯标记
	if _, err := s.DeprecateFactsByKey(sessionID, out.FactsDeprecate); err != nil {
		errs = append(errs, "deprecate: "+err.Error())
	}

	// 2. 新增（全新 key，无旧行可引用，不写 edge）
	for _, f := range out.FactsAdd {
		if f.Key == "" || f.Content == "" {
			continue
		}
		tags := stageToTags(f.Stage)
		if _, _, err := s.UpsertFact(sessionID, f.Key, f.Content, 0.9, sourceFloor, tags); err != nil {
			errs = append(errs, "add "+f.Key+": "+err.Error())
		}
	}

	// 3. 更新（更新已有 key）— 写入 "updates" 关系边
	for _, f := range out.FactsUpdate {
		if f.Key == "" || f.Content == "" {
			continue
		}
		tags := stageToTags(f.Stage)
		newID, oldID, err := s.UpsertFact(sessionID, f.Key, f.Content, 0.9, sourceFloor, tags)
		if err != nil {
			errs = append(errs, "update "+f.Key+": "+err.Error())
			continue
		}
		// 有旧行被废弃时写入 updates 边（新行 → 旧行）
		if oldID != "" {
			if _, edgeErr := s.SaveEdge(sessionID, newID, oldID, dbmodels.MemoryRelationUpdates); edgeErr != nil {
				errs = append(errs, "edge updates "+f.Key+": "+edgeErr.Error())
			}
		}
	}

	// 4. 本轮摘要
	if out.TurnSummary != "" {
		if err := s.SaveFromParser(sessionID, out.TurnSummary, sourceFloor); err != nil {
			errs = append(errs, "summary: "+err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("consolidation partial errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// parseLegacyResult 旧格式回退解析（<Summary> + 事实：行前缀）。
func (s *Store) parseLegacyResult(sessionID, result string, sourceFloor int) error {
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

// UpdateMemory 部分更新记忆字段（content / importance / type / deprecated / stage_tags）
func (s *Store) UpdateMemory(memID, sessionID string, updates map[string]any) (*dbmodels.Memory, error) {
	// 只允许更新安全字段
	allowed := map[string]struct{}{
		"content": {}, "importance": {}, "type": {}, "deprecated": {}, "stage_tags": {},
	}
	safe := make(map[string]any, len(updates))
	for k, v := range updates {
		if _, ok := allowed[k]; !ok {
			continue
		}
		// stage_tags 可能从 JSON 解析为 []interface{}，需转换为 []string 再序列化
		if k == "stage_tags" {
			var tags []string
			switch t := v.(type) {
			case []string:
				tags = t
			case []any:
				for _, item := range t {
					if s, ok := item.(string); ok {
						tags = append(tags, s)
					}
				}
			}
			if tags == nil {
				tags = []string{}
			}
			b, _ := json.Marshal(tags)
			safe[k] = b
			continue
		}
		safe[k] = v
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

// GetFloorCount 返回指定 session 的当前楼层数。
func (s *Store) GetFloorCount(sessionID string) int {
	var count int
	s.db.Model(&dbmodels.GameSession{}).Where("id = ?", sessionID).
		Select("floor_count").Scan(&count)
	return count
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

// ── Memory Edge（记忆关系图）──────────────────────────────────────────────────
//
// edge 仅用于溯源、审计和未来的 Memory Lint，不参与 Prompt 注入。
// 对应 TH memory_edge 表，WE 简化为 4 种 relation（去掉 TH 双层压缩专用的 derived_from / compacts）。

// SaveEdge 写入一条记忆关系边。
// from_id → to_id 表示"from 对 to 施加 relation"（如 新事实 updates 旧事实）。
func (s *Store) SaveEdge(sessionID, fromID, toID string, relation dbmodels.MemoryRelation) (*dbmodels.MemoryEdge, error) {
	edge := &dbmodels.MemoryEdge{
		SessionID: sessionID,
		FromID:    fromID,
		ToID:      toID,
		Relation:  relation,
	}
	if err := s.db.Create(edge).Error; err != nil {
		return nil, err
	}
	return edge, nil
}

// ListEdges 列出一条记忆条目的所有关联边（双向：from 或 to 匹配均返回）。
// 对应 TH findEdges(itemId)。
func (s *Store) ListEdges(sessionID, memoryID string) ([]dbmodels.MemoryEdge, error) {
	var edges []dbmodels.MemoryEdge
	err := s.db.Where(
		"session_id = ? AND (from_id = ? OR to_id = ?)",
		sessionID, memoryID, memoryID,
	).Order("created_at DESC").Find(&edges).Error
	return edges, err
}

// ListEdgesBySession 列出一个会话的所有边（供 API 分页查询用）。
func (s *Store) ListEdgesBySession(sessionID string, relation dbmodels.MemoryRelation, limit, offset int) ([]dbmodels.MemoryEdge, error) {
	if limit <= 0 {
		limit = 50
	}
	q := s.db.Where("session_id = ?", sessionID)
	if relation != "" {
		q = q.Where("relation = ?", relation)
	}
	var edges []dbmodels.MemoryEdge
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&edges).Error
	return edges, err
}

// DeleteEdge 物理删除一条边（管理员 / 调试用）。
func (s *Store) DeleteEdge(sessionID, edgeID string) error {
	result := s.db.Where("id = ? AND session_id = ?", edgeID, sessionID).Delete(&dbmodels.MemoryEdge{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("edge not found: %s", edgeID)
	}
	return nil
}

// UpdateEdgeRelation 修改一条边的 relation 类型（调试用，relation 标错时无需删除重建）。
func (s *Store) UpdateEdgeRelation(sessionID, edgeID string, relation dbmodels.MemoryRelation) (*dbmodels.MemoryEdge, error) {
	if err := s.db.Model(&dbmodels.MemoryEdge{}).
		Where("id = ? AND session_id = ?", edgeID, sessionID).
		Update("relation", relation).Error; err != nil {
		return nil, err
	}
	var edge dbmodels.MemoryEdge
	if err := s.db.First(&edge, "id = ? AND session_id = ?", edgeID, sessionID).Error; err != nil {
		return nil, fmt.Errorf("edge not found: %s", edgeID)
	}
	return &edge, nil
}
