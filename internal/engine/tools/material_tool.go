package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
)

// SearchMaterialTool 从素材库按标签/情绪/风格检索匹配条目，供 LLM 获取 Tier3 内容。
//
// 触发示例（LLM 工具调用）：
//
//	search_material({"tags": ["恐惧", "黑暗"], "limit": 3})
//	search_material({"mood": "tense", "function_tag": "atmosphere", "limit": 5})
//
// 检索规则：
//  1. tags 与 world_tags 使用 PostgreSQL JSONB `?|` 操作符（任意一个标签命中即返回）
//  2. mood / style / function_tag / type 为精确匹配（可选，可组合）
//  3. 命中后 used_count+1（异步，不阻塞响应）
type SearchMaterialTool struct {
	db        *gorm.DB
	gameID    string
	sessionID string
}

func NewSearchMaterialTool(db *gorm.DB, gameID, sessionID string) *SearchMaterialTool {
	return &SearchMaterialTool{db: db, gameID: gameID, sessionID: sessionID}
}

func (t *SearchMaterialTool) Name() string        { return "search_material" }
func (t *SearchMaterialTool) ReplaySafety() ReplaySafety { return ReplaySafe }
func (t *SearchMaterialTool) Description() string {
	return "从素材库检索匹配标签/情绪/风格的内容片段，返回可直接使用的文本素材列表"
}
func (t *SearchMaterialTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tags": {
				"type": "array",
				"items": {"type": "string"},
				"description": "通用标签列表，任意一个命中即返回（支持 tags 和 world_tags 字段）"
			},
			"type": {
				"type": "string",
				"description": "素材类型：post|dialogue|description|event|atmosphere"
			},
			"mood": {
				"type": "string",
				"description": "情绪标签：happy|sad|tense|melancholy|neutral 等"
			},
			"style": {
				"type": "string",
				"description": "文风标签：lyrical|aggressive|neutral|humorous 等"
			},
			"function_tag": {
				"type": "string",
				"description": "功能标签：atmosphere|plot_hook|dialogue|lore 等"
			},
			"limit": {
				"type": "integer",
				"description": "返回条数上限（默认 5，最大 20）",
				"default": 5
			}
		}
	}`)
}

func (t *SearchMaterialTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var p struct {
		Tags        []string `json:"tags"`
		Type        string   `json:"type"`
		Mood        string   `json:"mood"`
		Style       string   `json:"style"`
		FunctionTag string   `json:"function_tag"`
		Limit       int      `json:"limit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return "", fmt.Errorf("search_material: invalid params: %w", err)
	}
	if p.Limit <= 0 {
		p.Limit = 5
	}
	if p.Limit > 20 {
		p.Limit = 20
	}

	q := t.db.WithContext(ctx).
		Where("game_id = ? AND enabled = true", t.gameID)

	// 标签检索：tags ?| array['tag1','tag2'] OR world_tags ?| array['tag1','tag2']
	if len(p.Tags) > 0 {
		tagsJSON, _ := json.Marshal(p.Tags)
		q = q.Where(
			"(tags ?| array(select jsonb_array_elements_text(?::jsonb))) OR "+
				"(world_tags ?| array(select jsonb_array_elements_text(?::jsonb)))",
			string(tagsJSON), string(tagsJSON),
		)
	}

	if p.Type != "" {
		q = q.Where("type = ?", p.Type)
	}
	if p.Mood != "" {
		q = q.Where("mood = ?", p.Mood)
	}
	if p.Style != "" {
		q = q.Where("style = ?", p.Style)
	}
	if p.FunctionTag != "" {
		q = q.Where("function_tag = ?", p.FunctionTag)
	}

	var results []dbmodels.Material
	if err := q.Order("used_count ASC, created_at DESC").Limit(p.Limit).Find(&results).Error; err != nil {
		return "", fmt.Errorf("search_material: db query: %w", err)
	}

	if len(results) == 0 {
		return `{"found":false,"items":[]}`, nil
	}

	// 异步递增 used_count（不阻塞响应）
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	go func() {
		t.db.Model(&dbmodels.Material{}).
			Where("id IN ?", ids).
			UpdateColumn("used_count", gorm.Expr("used_count + 1"))
	}()

	type item struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Content     string `json:"content"`
		Mood        string `json:"mood,omitempty"`
		Style       string `json:"style,omitempty"`
		FunctionTag string `json:"function_tag,omitempty"`
	}
	items := make([]item, len(results))
	for i, r := range results {
		items[i] = item{
			ID:          r.ID,
			Type:        r.Type,
			Content:     r.Content,
			Mood:        r.Mood,
			Style:       r.Style,
			FunctionTag: r.FunctionTag,
		}
	}
	b, _ := json.Marshal(map[string]any{"found": true, "items": items})
	return string(b), nil
}
