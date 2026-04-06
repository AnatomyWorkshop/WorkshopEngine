// Package tools — ResourceToolProvider
//
// 提供 14 个内置资源工具，让 LLM 在游戏回合中读写 WE 资源层（世界书、预设条目、
// 素材库、游戏模板、会话状态、记忆）。游戏设计师在 GameTemplate.Config 的 enabled_tools
// 中写入 "resource:*" 即可一次性启用所有资源工具，无需写任何后端代码。
//
// 工具一览：
//
//	worldbook_search   — 按关键词搜索世界书条目（safe）
//	worldbook_get      — 按 ID 获取单条世界书条目（safe）
//	worldbook_create   — 创建新世界书条目（confirm_on_replay）
//	worldbook_update   — 更新世界书条目（confirm_on_replay）
//	worldbook_delete   — 软删除世界书条目（never_auto_replay）
//	preset_list        — 列出游戏所有预设条目（safe）
//	preset_get         — 按 identifier 获取预设条目（safe）
//	preset_create      — 创建新预设条目（confirm_on_replay）
//	preset_update      — 更新预设条目内容/启用状态（confirm_on_replay）
//	material_create    — 向素材库添加内容（confirm_on_replay）
//	template_info      — 获取当前游戏模板基本信息（safe）
//	session_summary    — 获取会话状态（楼层数 + 记忆摘要）（safe）
//	floor_history      — 获取最近 N 回合的对话内容（safe）
//	memory_create      — 写入一条明确事实记忆（confirm_on_replay）
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/engine/memory"
)

// NewResourceToolProvider 返回资源工具列表（全部 14 个）。
// 调用方决定注册哪些：逐一 Register 或在 "resource:*" 开关下全部注册。
func NewResourceToolProvider(db *gorm.DB, gameID, sessionID string, memStore *memory.Store) []Tool {
	return []Tool{
		&worldbookSearchTool{db: db, gameID: gameID},
		&worldbookGetTool{db: db, gameID: gameID},
		&worldbookCreateTool{db: db, gameID: gameID},
		&worldbookUpdateTool{db: db, gameID: gameID},
		&worldbookDeleteTool{db: db, gameID: gameID},
		&presetListTool{db: db, gameID: gameID},
		&presetGetTool{db: db, gameID: gameID},
		&presetCreateTool{db: db, gameID: gameID},
		&presetUpdateTool{db: db, gameID: gameID},
		&materialCreateTool{db: db, gameID: gameID},
		&templateInfoTool{db: db, gameID: gameID},
		&sessionSummaryTool{db: db, sessionID: sessionID},
		&floorHistoryTool{db: db, sessionID: sessionID},
		&memoryCreateTool{sessionID: sessionID, memStore: memStore},
	}
}

// ── worldbook_search ──────────────────────────────────────────────────────────

type worldbookSearchTool struct {
	db     *gorm.DB
	gameID string
}

func (t *worldbookSearchTool) Name() string        { return "worldbook_search" }
func (t *worldbookSearchTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *worldbookSearchTool) Description() string {
	return "在当前游戏的世界书中搜索词条（匹配 content 或 keys），返回最多 limit 条结果"
}
func (t *worldbookSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "搜索关键词，匹配词条内容或触发关键词"},
			"limit": {"type": "integer", "description": "最多返回条数，默认 5，最大 20"}
		},
		"required": ["query"]
	}`)
}
func (t *worldbookSearchTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Limit <= 0 || p.Limit > 20 {
		p.Limit = 5
	}
	like := "%" + p.Query + "%"
	var entries []dbmodels.WorldbookEntry
	t.db.Where(
		"game_id = ? AND enabled = true AND (content ILIKE ? OR keys::text ILIKE ?)",
		t.gameID, like, like,
	).Order("priority ASC").Limit(p.Limit).Find(&entries)

	type row struct {
		ID      string `json:"id"`
		Keys    []string `json:"keys"`
		Content string `json:"content"`
		Constant bool   `json:"constant"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		var keys []string
		_ = json.Unmarshal(e.Keys, &keys)
		out = append(out, row{ID: e.ID, Keys: keys, Content: e.Content, Constant: e.Constant})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── worldbook_get ─────────────────────────────────────────────────────────────

type worldbookGetTool struct {
	db     *gorm.DB
	gameID string
}

func (t *worldbookGetTool) Name() string        { return "worldbook_get" }
func (t *worldbookGetTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *worldbookGetTool) Description() string { return "按 ID 获取单条世界书词条的完整内容" }
func (t *worldbookGetTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "WorldbookEntry UUID"}
		},
		"required": ["id"]
	}`)
}
func (t *worldbookGetTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct{ ID string `json:"id"` }
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	var e dbmodels.WorldbookEntry
	if err := t.db.First(&e, "id = ? AND game_id = ?", p.ID, t.gameID).Error; err != nil {
		return `{"found":false}`, nil
	}
	var keys, secondaryKeys []string
	_ = json.Unmarshal(e.Keys, &keys)
	_ = json.Unmarshal(e.SecondaryKeys, &secondaryKeys)
	out := map[string]any{
		"found":           true,
		"id":              e.ID,
		"keys":            keys,
		"secondary_keys":  secondaryKeys,
		"secondary_logic": e.SecondaryLogic,
		"content":         e.Content,
		"constant":        e.Constant,
		"priority":        e.Priority,
		"scan_depth":      e.ScanDepth,
		"position":        e.Position,
		"whole_word":      e.WholeWord,
		"enabled":         e.Enabled,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── worldbook_create ──────────────────────────────────────────────────────────

type worldbookCreateTool struct {
	db     *gorm.DB
	gameID string
}

func (t *worldbookCreateTool) Name() string        { return "worldbook_create" }
func (t *worldbookCreateTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *worldbookCreateTool) Description() string {
	return "在当前游戏中创建一条新世界书词条（角色、场景、概念均可）"
}
func (t *worldbookCreateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"keys":     {"type": "array", "items": {"type": "string"}, "description": "触发关键词列表"},
			"content":  {"type": "string", "description": "注入的世界书内容"},
			"constant": {"type": "boolean", "description": "是否常驻注入（不依赖关键词匹配），默认 false"},
			"priority": {"type": "integer", "description": "优先级偏移，默认 0，数值越小越靠前"}
		},
		"required": ["keys", "content"]
	}`)
}
func (t *worldbookCreateTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Keys     []string `json:"keys"`
		Content  string   `json:"content"`
		Constant bool     `json:"constant"`
		Priority int      `json:"priority"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	keysJSON, _ := json.Marshal(p.Keys)
	entry := dbmodels.WorldbookEntry{
		GameID:   t.gameID,
		Keys:     keysJSON,
		Content:  p.Content,
		Constant: p.Constant,
		Priority: p.Priority,
		Enabled:  true,
	}
	if err := t.db.Create(&entry).Error; err != nil {
		return "", fmt.Errorf("worldbook_create: %w", err)
	}
	return fmt.Sprintf(`{"ok":true,"id":%q}`, entry.ID), nil
}

// ── worldbook_update ──────────────────────────────────────────────────────────

type worldbookUpdateTool struct {
	db     *gorm.DB
	gameID string
}

func (t *worldbookUpdateTool) Name() string        { return "worldbook_update" }
func (t *worldbookUpdateTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *worldbookUpdateTool) Description() string {
	return "更新世界书词条的内容、关键词或启用状态（只传需要修改的字段）"
}
func (t *worldbookUpdateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id":      {"type": "string", "description": "WorldbookEntry UUID"},
			"content": {"type": "string", "description": "新内容（不传则不修改）"},
			"keys":    {"type": "array", "items": {"type": "string"}, "description": "新关键词列表（不传则不修改）"},
			"enabled": {"type": "boolean", "description": "启用/禁用（不传则不修改）"}
		},
		"required": ["id"]
	}`)
}
func (t *worldbookUpdateTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		ID      string   `json:"id"`
		Content *string  `json:"content"`
		Keys    []string `json:"keys"`
		Enabled *bool    `json:"enabled"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	updates := map[string]any{}
	if p.Content != nil {
		updates["content"] = *p.Content
	}
	if len(p.Keys) > 0 {
		keysJSON, _ := json.Marshal(p.Keys)
		updates["keys"] = keysJSON
	}
	if p.Enabled != nil {
		updates["enabled"] = *p.Enabled
	}
	if len(updates) == 0 {
		return `{"ok":true,"updated":false}`, nil
	}
	if err := t.db.Model(&dbmodels.WorldbookEntry{}).
		Where("id = ? AND game_id = ?", p.ID, t.gameID).
		Updates(updates).Error; err != nil {
		return "", fmt.Errorf("worldbook_update: %w", err)
	}
	return `{"ok":true,"updated":true}`, nil
}

// ── worldbook_delete ──────────────────────────────────────────────────────────

type worldbookDeleteTool struct {
	db     *gorm.DB
	gameID string
}

func (t *worldbookDeleteTool) Name() string        { return "worldbook_delete" }
func (t *worldbookDeleteTool) ReplaySafety() ReplaySafety { return ReplayNeverAutoReplay }
func (t *worldbookDeleteTool) Description() string {
	return "软删除世界书词条（将 enabled 设为 false，不物理删除）"
}
func (t *worldbookDeleteTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "WorldbookEntry UUID"}
		},
		"required": ["id"]
	}`)
}
func (t *worldbookDeleteTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct{ ID string `json:"id"` }
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if err := t.db.Model(&dbmodels.WorldbookEntry{}).
		Where("id = ? AND game_id = ?", p.ID, t.gameID).
		Update("enabled", false).Error; err != nil {
		return "", fmt.Errorf("worldbook_delete: %w", err)
	}
	return `{"ok":true}`, nil
}

// ── preset_list ───────────────────────────────────────────────────────────────

type presetListTool struct {
	db     *gorm.DB
	gameID string
}

func (t *presetListTool) Name() string        { return "preset_list" }
func (t *presetListTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *presetListTool) Description() string {
	return "列出当前游戏所有已启用的预设条目（identifier、name、injection_order）"
}
func (t *presetListTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *presetListTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	var entries []dbmodels.PresetEntry
	t.db.Where("game_id = ? AND enabled = true", t.gameID).
		Order("injection_order ASC").Find(&entries)
	type row struct {
		Identifier     string `json:"identifier"`
		Name           string `json:"name"`
		InjectionOrder int    `json:"injection_order"`
		Role           string `json:"role"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		out = append(out, row{
			Identifier:     e.Identifier,
			Name:           e.Name,
			InjectionOrder: e.InjectionOrder,
			Role:           e.Role,
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── preset_get ────────────────────────────────────────────────────────────────

type presetGetTool struct {
	db     *gorm.DB
	gameID string
}

func (t *presetGetTool) Name() string        { return "preset_get" }
func (t *presetGetTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *presetGetTool) Description() string {
	return "按 identifier 获取单条预设条目的完整内容"
}
func (t *presetGetTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"identifier": {"type": "string", "description": "预设条目的 identifier（如 main_persona）"}
		},
		"required": ["identifier"]
	}`)
}
func (t *presetGetTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct{ Identifier string `json:"identifier"` }
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	var e dbmodels.PresetEntry
	if err := t.db.First(&e, "game_id = ? AND identifier = ?", t.gameID, p.Identifier).Error; err != nil {
		return `{"found":false}`, nil
	}
	out := map[string]any{
		"found":      true,
		"id":         e.ID,
		"identifier": e.Identifier,
		"name":       e.Name,
		"role":       e.Role,
		"content":    e.Content,
		"injection_order": e.InjectionOrder,
		"enabled":    e.Enabled,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── preset_create ─────────────────────────────────────────────────────────────

type presetCreateTool struct {
	db     *gorm.DB
	gameID string
}

func (t *presetCreateTool) Name() string             { return "preset_create" }
func (t *presetCreateTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *presetCreateTool) Description() string {
	return "创建一条新的预设条目（叙事规则/角色设定/格式要求等）"
}
func (t *presetCreateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"identifier":      {"type": "string", "description": "唯一标识符（英文，如 main_persona）"},
			"name":            {"type": "string", "description": "显示名称"},
			"role":            {"type": "string", "description": "消息角色：system|user|assistant，默认 system"},
			"content":         {"type": "string", "description": "条目内容（支持 {{宏}} 变量）"},
			"injection_order": {"type": "integer", "description": "注入顺序（1-9 顶部，990-1009 角色人设槽），默认 1000"},
			"is_system_prompt":{"type": "boolean", "description": "是否为主角色人设槽，默认 false"}
		},
		"required": ["identifier", "name", "content"]
	}`)
}
func (t *presetCreateTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Identifier     string `json:"identifier"`
		Name           string `json:"name"`
		Role           string `json:"role"`
		Content        string `json:"content"`
		InjectionOrder *int   `json:"injection_order"`
		IsSystemPrompt bool   `json:"is_system_prompt"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Role == "" {
		p.Role = "system"
	}
	injOrder := 1000
	if p.InjectionOrder != nil {
		injOrder = *p.InjectionOrder
	}
	entry := dbmodels.PresetEntry{
		GameID:            t.gameID,
		Identifier:        p.Identifier,
		Name:              p.Name,
		Role:              p.Role,
		Content:           p.Content,
		InjectionPosition: "system",
		InjectionOrder:    injOrder,
		Enabled:           true,
		IsSystemPrompt:    p.IsSystemPrompt,
	}
	if err := t.db.Create(&entry).Error; err != nil {
		return "", fmt.Errorf("preset_create: %w", err)
	}
	return fmt.Sprintf(`{"ok":true,"id":%q,"identifier":%q}`, entry.ID, entry.Identifier), nil
}

// ── preset_update ─────────────────────────────────────────────────────────────

type presetUpdateTool struct {
	db     *gorm.DB
	gameID string
}

func (t *presetUpdateTool) Name() string        { return "preset_update" }
func (t *presetUpdateTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *presetUpdateTool) Description() string {
	return "更新预设条目的内容或启用状态（只传需要修改的字段）"
}
func (t *presetUpdateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"identifier": {"type": "string", "description": "预设条目的 identifier"},
			"content":    {"type": "string", "description": "新内容（不传则不修改）"},
			"enabled":    {"type": "boolean", "description": "启用/禁用（不传则不修改）"}
		},
		"required": ["identifier"]
	}`)
}
func (t *presetUpdateTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Identifier string  `json:"identifier"`
		Content    *string `json:"content"`
		Enabled    *bool   `json:"enabled"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	updates := map[string]any{}
	if p.Content != nil {
		updates["content"] = *p.Content
	}
	if p.Enabled != nil {
		updates["enabled"] = *p.Enabled
	}
	if len(updates) == 0 {
		return `{"ok":true,"updated":false}`, nil
	}
	if err := t.db.Model(&dbmodels.PresetEntry{}).
		Where("game_id = ? AND identifier = ?", t.gameID, p.Identifier).
		Updates(updates).Error; err != nil {
		return "", fmt.Errorf("preset_update: %w", err)
	}
	return `{"ok":true,"updated":true}`, nil
}

// ── material_create ───────────────────────────────────────────────────────────

type materialCreateTool struct {
	db     *gorm.DB
	gameID string
}

func (t *materialCreateTool) Name() string             { return "material_create" }
func (t *materialCreateTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *materialCreateTool) Description() string {
	return "向素材库添加一条内容（台词片段、氛围描写、场景文本等）"
}
func (t *materialCreateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content":      {"type": "string", "description": "素材正文"},
			"type":         {"type": "string", "description": "素材类型（如 post/dialogue/description/event/atmosphere），默认 text"},
			"mood":         {"type": "string", "description": "情绪标签（如 happy/sad/tense/neutral）"},
			"style":        {"type": "string", "description": "风格标签（如 lyrical/aggressive/neutral）"},
			"function_tag": {"type": "string", "description": "功能标签（如 atmosphere/plot_hook/dialogue/lore）"},
			"tags":         {"type": "array", "items": {"type": "string"}, "description": "通用标签列表"}
		},
		"required": ["content"]
	}`)
}
func (t *materialCreateTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Content     string   `json:"content"`
		Type        string   `json:"type"`
		Mood        string   `json:"mood"`
		Style       string   `json:"style"`
		FunctionTag string   `json:"function_tag"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Type == "" {
		p.Type = "text"
	}
	tagsJSON, _ := json.Marshal(p.Tags)
	if p.Tags == nil {
		tagsJSON = []byte(`[]`)
	}
	m := dbmodels.Material{
		GameID:      t.gameID,
		Type:        p.Type,
		Content:     p.Content,
		Tags:        tagsJSON,
		WorldTags:   []byte(`[]`),
		Mood:        p.Mood,
		Style:       p.Style,
		FunctionTag: p.FunctionTag,
		Enabled:     true,
	}
	if err := t.db.Create(&m).Error; err != nil {
		return "", fmt.Errorf("material_create: %w", err)
	}
	return fmt.Sprintf(`{"ok":true,"id":%q}`, m.ID), nil
}

// ── template_info ─────────────────────────────────────────────────────────────

type templateInfoTool struct {
	db     *gorm.DB
	gameID string
}

func (t *templateInfoTool) Name() string        { return "template_info" }
func (t *templateInfoTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *templateInfoTool) Description() string {
	return "获取当前游戏模板的基本信息（标题、类型、描述）"
}
func (t *templateInfoTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *templateInfoTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	var tmpl dbmodels.GameTemplate
	if err := t.db.Select("id, title, type, description, status").
		First(&tmpl, "id = ?", t.gameID).Error; err != nil {
		return `{"found":false}`, nil
	}
	out := map[string]any{
		"found":       true,
		"id":          tmpl.ID,
		"title":       tmpl.Title,
		"type":        tmpl.Type,
		"description": tmpl.Description,
		"status":      tmpl.Status,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── session_summary ───────────────────────────────────────────────────────────

type sessionSummaryTool struct {
	db        *gorm.DB
	sessionID string
}

func (t *sessionSummaryTool) Name() string        { return "session_summary" }
func (t *sessionSummaryTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *sessionSummaryTool) Description() string {
	return "获取当前会话状态：已完成回合数（floor_count）和最新记忆摘要文本"
}
func (t *sessionSummaryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *sessionSummaryTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	var sess dbmodels.GameSession
	if err := t.db.Select("id, floor_count, memory_summary").
		First(&sess, "id = ?", t.sessionID).Error; err != nil {
		return `{"found":false}`, nil
	}
	out := map[string]any{
		"found":          true,
		"floor_count":    sess.FloorCount,
		"memory_summary": sess.MemorySummary,
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── floor_history ─────────────────────────────────────────────────────────────

type floorHistoryTool struct {
	db        *gorm.DB
	sessionID string
}

func (t *floorHistoryTool) Name() string        { return "floor_history" }
func (t *floorHistoryTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *floorHistoryTool) Description() string {
	return "获取最近 N 个已提交回合的对话内容（用户输入 + AI 回复），默认 3 条"
}
func (t *floorHistoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"limit": {"type": "integer", "description": "最近 N 个回合，默认 3，最大 10"}
		}
	}`)
}
func (t *floorHistoryTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct{ Limit int `json:"limit"` }
	_ = json.Unmarshal(params, &p)
	if p.Limit <= 0 || p.Limit > 10 {
		p.Limit = 3
	}

	// 取最近 N 个已提交楼层的 active page
	type floorPage struct {
		Seq      int    `gorm:"column:seq"`
		Messages []byte `gorm:"column:messages"`
	}
	var rows []floorPage
	t.db.Raw(`
		SELECT f.seq, p.messages
		FROM floors f
		JOIN message_pages p ON p.floor_id = f.id AND p.is_active = true
		WHERE f.session_id = ? AND f.status = 'committed'
		ORDER BY f.seq DESC
		LIMIT ?
	`, t.sessionID, p.Limit).Scan(&rows)

	type msgItem struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type histItem struct {
		Seq      int       `json:"seq"`
		Messages []msgItem `json:"messages"`
	}
	out := make([]histItem, 0, len(rows))
	for _, r := range rows {
		var msgs []msgItem
		_ = json.Unmarshal(r.Messages, &msgs)
		out = append(out, histItem{Seq: r.Seq, Messages: msgs})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── memory_create ─────────────────────────────────────────────────────────────

type memoryCreateTool struct {
	sessionID string
	memStore  *memory.Store
}

func (t *memoryCreateTool) Name() string        { return "memory_create" }
func (t *memoryCreateTool) ReplaySafety() ReplaySafety { return ReplayConfirmOnReplay }
func (t *memoryCreateTool) Description() string {
	return "向会话记忆中写入一条明确事实（供后续回合注入上下文）"
}
func (t *memoryCreateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"content":    {"type": "string", "description": "记忆内容（一句话事实描述）"},
			"importance": {"type": "number", "description": "重要度 0–1，默认 0.8；越高越不容易被衰减淘汰"}
		},
		"required": ["content"]
	}`)
}
func (t *memoryCreateTool) Execute(_ context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Content    string  `json:"content"`
		Importance float64 `json:"importance"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", err
	}
	if p.Importance <= 0 || p.Importance > 1 {
		p.Importance = 0.8
	}
	if err := t.memStore.SaveFact(t.sessionID, p.Content, p.Importance); err != nil {
		return "", fmt.Errorf("memory_create: %w", err)
	}
	return `{"ok":true}`, nil
}
