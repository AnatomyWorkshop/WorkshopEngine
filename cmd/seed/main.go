// cmd/seed/main.go — 从 .data/games/*/game.json 批量导入游戏数据到 WE 数据库
//
// 用法：
//   go run ./cmd/seed --data ../../.data/games
//
// 每次运行是幂等的：已存在的 slug 会跳过（不覆盖）。
// 加 --force 参数会删除已有数据后重新导入。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
	dbmodels "mvu-backend/internal/core/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GameSeedFile 对应 .data/games/*/game.json 的结构
// 支持两种格式：
//   1. 扁平格式：slug/title/config 等在顶层
//   2. 导出格式：template 对象 + worldbook_entries 数组
type GameSeedFile struct {
	// 扁平格式字段
	Slug         string         `json:"slug"`
	Title        string         `json:"title"`
	Type         string         `json:"type"`
	ShortDesc    string         `json:"short_desc"`
	Notes        string         `json:"notes"`
	CoverURL     string         `json:"cover_url"`
	Status       string         `json:"status"`
	Config       map[string]any `json:"config"`
	SystemPrompt string         `json:"system_prompt"`
	// 导出格式字段
	Template *TemplateSeed `json:"template"`
	// 共享
	WorldbookEntries []WBSeed `json:"worldbook_entries"`
	PresetEntries    []PESeed `json:"preset_entries"`
}

// TemplateSeed 导出格式中的 template 对象
type TemplateSeed struct {
	Slug                 string         `json:"slug"`
	Title                string         `json:"title"`
	Type                 string         `json:"type"`
	ShortDesc            string         `json:"short_desc"`
	Description          string         `json:"description"`
	Notes                string         `json:"notes"`
	CoverURL             string         `json:"cover_url"`
	Status               string         `json:"status"`
	AuthorID             string         `json:"author_id"`
	Config               map[string]any `json:"config"`
	SystemPromptTemplate string         `json:"system_prompt_template"`
}

type WBSeed struct {
	Keys            []string `json:"keys"`
	SecondaryKeys   []string `json:"secondary_keys"`
	Content         string   `json:"content"`
	Constant        bool     `json:"constant"`
	Position        string   `json:"position"`
	Priority        int      `json:"priority"`
	ScanDepth       int      `json:"scan_depth"`
	Comment         string   `json:"comment"`
	PlayerVisible   *bool    `json:"player_visible"`   // nil = default true
	DisplayCategory string   `json:"display_category"`
}

type PESeed struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name"`
	Role              string `json:"role"`
	Content           string `json:"content"`
	InjectionOrder    int    `json:"injection_order"`
	InjectionPosition string `json:"injection_position"`
	IsSystemPrompt    bool   `json:"is_system_prompt"`
	Enabled           bool   `json:"enabled"`
	Comment           string `json:"comment"`
}

func main() {
	dataDir := flag.String("data", "../.data/games", "path to .data/games directory")
	force := flag.Bool("force", false, "delete and re-import existing games")
	flag.Parse()

	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}

	entries, err := filepath.Glob(filepath.Join(*dataDir, "*/game.json"))
	if err != nil || len(entries) == 0 {
		log.Fatalf("no game.json files found in %s", *dataDir)
	}

	for _, path := range entries {
		if err := importGame(db, path, *force); err != nil {
			log.Printf("SKIP %s: %v", path, err)
		}
	}
	fmt.Println("seed done")
}

func importGame(db *gorm.DB, path string, force bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var seed GameSeedFile
	if err := json.Unmarshal(data, &seed); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// 兼容导出格式：从 template 对象提升字段到顶层
	if seed.Template != nil {
		t := seed.Template
		if seed.Slug == "" { seed.Slug = t.Slug }
		if seed.Title == "" { seed.Title = t.Title }
		if seed.Type == "" { seed.Type = t.Type }
		if seed.ShortDesc == "" { seed.ShortDesc = t.ShortDesc }
		if seed.Notes == "" { seed.Notes = t.Notes }
		if seed.CoverURL == "" { seed.CoverURL = t.CoverURL }
		if seed.Status == "" { seed.Status = t.Status }
		if seed.Config == nil { seed.Config = t.Config }
		if seed.SystemPrompt == "" { seed.SystemPrompt = t.SystemPromptTemplate }
	}

	// 检查是否已存在
	var existing dbmodels.GameTemplate
	db.Where("slug = ?", seed.Slug).First(&existing)
	if existing.ID != "" {
		if !force {
			fmt.Printf("  SKIP (exists): %s\n", seed.Slug)
			return nil
		}
		// force: 删除旧数据
		db.Where("game_id = ?", existing.ID).Delete(&dbmodels.WorldbookEntry{})
		db.Where("game_id = ?", existing.ID).Delete(&dbmodels.PresetEntry{})
		db.Delete(&existing)
		fmt.Printf("  DELETED: %s\n", seed.Slug)
	}

	// 构建 Config JSONB（合并 system_prompt 到 config）
	cfg := seed.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	if seed.SystemPrompt != "" {
		cfg["system_prompt"] = seed.SystemPrompt
	}
	cfgJSON, _ := json.Marshal(cfg)

	// 创建 GameTemplate
	tmpl := dbmodels.GameTemplate{
		Slug:                 seed.Slug,
		Title:                seed.Title,
		Type:                 seed.Type,
		ShortDesc:            seed.ShortDesc,
		Notes:                seed.Notes,
		CoverURL:             seed.CoverURL,
		Status:               seed.Status,
		Config:               cfgJSON,
		SystemPromptTemplate: seed.SystemPrompt,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	if tmpl.Type == "" {
		tmpl.Type = "text"
	}
	if tmpl.Status == "" {
		tmpl.Status = "published"
	}

	if err := db.Create(&tmpl).Error; err != nil {
		return fmt.Errorf("create template %s: %w", seed.Slug, err)
	}
	fmt.Printf("  CREATED game: %s (%s)\n", tmpl.Title, tmpl.ID)

	// 创建 WorldbookEntry
	for _, wb := range seed.WorldbookEntries {
		keysJSON, _ := json.Marshal(wb.Keys)
		secKeysJSON, _ := json.Marshal(wb.SecondaryKeys)
		pv := true
		if wb.PlayerVisible != nil {
			pv = *wb.PlayerVisible
		}
		entry := dbmodels.WorldbookEntry{
			GameID:          tmpl.ID,
			Keys:            keysJSON,
			SecondaryKeys:   secKeysJSON,
			Content:         wb.Content,
			Constant:        wb.Constant,
			Position:        wb.Position,
			Priority:        wb.Priority,
			ScanDepth:       wb.ScanDepth,
			Comment:         wb.Comment,
			PlayerVisible:   pv,
			DisplayCategory: wb.DisplayCategory,
			Enabled:         true,
			CreatedAt:       time.Now(),
		}
		if entry.Position == "" {
			entry.Position = "before_template"
		}
		if err := db.Create(&entry).Error; err != nil {
			log.Printf("  WARN worldbook entry: %v", err)
		}
	}
	fmt.Printf("  CREATED %d worldbook entries\n", len(seed.WorldbookEntries))

	// 创建 PresetEntry
	for _, pe := range seed.PresetEntries {
		entry := dbmodels.PresetEntry{
			GameID:            tmpl.ID,
			Identifier:        pe.Identifier,
			Name:              pe.Name,
			Role:              pe.Role,
			Content:           pe.Content,
			InjectionOrder:    pe.InjectionOrder,
			InjectionPosition: pe.InjectionPosition,
			IsSystemPrompt:    pe.IsSystemPrompt,
			Enabled:           pe.Enabled,
			Comment:           pe.Comment,
			CreatedAt:         time.Now(),
			UpdatedAt:         time.Now(),
		}
		if entry.Role == "" {
			entry.Role = "system"
		}
		if entry.InjectionOrder == 0 {
			entry.InjectionOrder = 1000
		}
		if err := db.Create(&entry).Error; err != nil {
			log.Printf("  WARN preset entry: %v", err)
		}
	}
	fmt.Printf("  CREATED %d preset entries\n", len(seed.PresetEntries))

	return nil
}
