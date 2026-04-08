package db

import (
	"time"

	"gorm.io/datatypes"
)

// ──────────────────────────────────────────────────────
// 共享模型（engine / creation / social 层均可引用）
// ──────────────────────────────────────────────────────

// GameTemplate 游戏模板
type GameTemplate struct {
	ID                   string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	Slug                 string         `gorm:"uniqueIndex;not null"                          json:"slug"`
	Title                string         `gorm:"not null"                                      json:"title"`
	Type                 string         `gorm:"not null;default:'text'"                       json:"type"` // text（纯文字）| light（轻前端）| rich（重前端）| 创作者自由描述
	Description          string         `json:"description"`
	SystemPromptTemplate string         `gorm:"type:text"                                     json:"system_prompt_template"` // 支持 {{宏}} 变量展开
	Config               datatypes.JSON `gorm:"type:jsonb;default:'{}'"                       json:"config"`                // 初始变量、资产配置等
	CoverURL             string         `json:"cover_url"`
	Status               string         `gorm:"default:'draft'"                               json:"status"` // draft | published
	AuthorID             string         `gorm:"index"                                         json:"author_id"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

// Material 素材库条目（游戏级内容池）。
//
// 设计初衷：游戏设计师预先准备大量文本内容（发帖体、对话片段、氛围描写等），
// 引擎通过 search_material 工具按标签/情绪/风格检索匹配条目，
// 注入当前回合 LLM 上下文，实现"素材库驱动的 Tier3 NPC 内容生成"。
//
// 标签检索使用 PostgreSQL JSONB `?|` 操作符（数组包含任意一项即匹配）。
type Material struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	GameID      string         `gorm:"not null;index"                                  json:"game_id"`      // 关联 GameTemplate
	Type        string         `gorm:"not null;default:'text'"                         json:"type"`         // post|dialogue|description|event|atmosphere
	Content     string         `gorm:"type:text;not null"                              json:"content"`      // 素材正文
	Tags        datatypes.JSON `gorm:"type:jsonb;default:'[]'"                         json:"tags"`         // []string 通用标签
	WorldTags   datatypes.JSON `gorm:"type:jsonb;default:'[]'"                         json:"world_tags"`   // []string 世界专属标签
	Mood        string         `json:"mood"`                                                                // happy|sad|tense|melancholy|neutral...
	Style       string         `json:"style"`                                                               // lyrical|aggressive|neutral|humorous...
	FunctionTag string         `json:"function_tag"`                                                        // atmosphere|plot_hook|dialogue|lore...
	UsedCount   int            `gorm:"default:0"                                       json:"used_count"`   // 被检索引用次数
	Enabled     bool           `gorm:"default:true"                                    json:"enabled"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
