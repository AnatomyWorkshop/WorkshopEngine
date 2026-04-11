package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
)

// ── .thchat 导出格式 ─────────────────────────────────────────────────────────

// ThchatExport .thchat 文件顶层结构
type ThchatExport struct {
	Version    string            `json:"version"`
	Format     string            `json:"format"`
	ExportedAt time.Time         `json:"exported_at"`
	GameID     string            `json:"game_id"`
	GameTitle  string            `json:"game_title"`
	Session    ThchatSession     `json:"session"`
	Floors     []ThchatFloor     `json:"floors"`
	Memories   []ThchatMemory    `json:"memories"`
	MemEdges   []ThchatEdge      `json:"memory_edges"`
	Branches   []ThchatBranch    `json:"branches"`
}

type ThchatSession struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	Status            string          `json:"status"`
	Variables         json.RawMessage `json:"variables"`
	MemorySummary     string          `json:"memory_summary"`
	CharacterCardID   string          `json:"character_card_id,omitempty"`
	CharacterSnapshot json.RawMessage `json:"character_snapshot,omitempty"`
	FloorCount        int             `json:"floor_count"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

type ThchatFloor struct {
	ID       string        `json:"id"`
	Seq      int           `json:"seq"`
	BranchID string        `json:"branch_id"`
	Status   string        `json:"status"`
	Pages    []ThchatPage  `json:"pages"`
}

type ThchatPage struct {
	ID        string          `json:"id"`
	IsActive  bool            `json:"is_active"`
	Messages  json.RawMessage `json:"messages"`
	PageVars  json.RawMessage `json:"page_vars,omitempty"`
	TokenUsed int             `json:"token_used"`
}

type ThchatMemory struct {
	ID          string          `json:"id"`
	FactKey     string          `json:"fact_key,omitempty"`
	Content     string          `json:"content"`
	Type        string          `json:"type"`
	Importance  float64         `json:"importance"`
	SourceFloor int             `json:"source_floor"`
	Deprecated  bool            `json:"deprecated"`
	StageTags   json.RawMessage `json:"stage_tags,omitempty"`
}

type ThchatEdge struct {
	ID       string `json:"id"`
	FromID   string `json:"from_id"`
	ToID     string `json:"to_id"`
	Relation string `json:"relation"`
}

type ThchatBranch struct {
	ID           string `json:"id"`
	BranchID     string `json:"branch_id"`
	ParentBranch string `json:"parent_branch"`
	OriginSeq    int    `json:"origin_seq"`
}

// ── 导出 ─────────────────────────────────────────────────────────────────────

// ExportThchat 导出会话为 .thchat 格式
func (e *GameEngine) ExportThchat(sessionID string) (*ThchatExport, error) {
	var sess dbmodels.GameSession
	if err := e.db.First(&sess, "id = ?", sessionID).Error; err != nil {
		return nil, err
	}

	// 游戏标题
	var gameTitle string
	var tmpl dbmodels.GameTemplate
	if e.db.Select("title").First(&tmpl, "id = ?", sess.GameID).Error == nil {
		gameTitle = tmpl.Title
	}

	// 楼层 + 页面
	var floors []dbmodels.Floor
	e.db.Where("session_id = ?", sessionID).Order("seq ASC").Find(&floors)

	floorIDs := make([]string, len(floors))
	for i, f := range floors {
		floorIDs[i] = f.ID
	}

	var pages []dbmodels.MessagePage
	if len(floorIDs) > 0 {
		e.db.Where("floor_id IN ?", floorIDs).Find(&pages)
	}
	pagesByFloor := map[string][]dbmodels.MessagePage{}
	for _, p := range pages {
		pagesByFloor[p.FloorID] = append(pagesByFloor[p.FloorID], p)
	}

	exportFloors := make([]ThchatFloor, len(floors))
	for i, f := range floors {
		fp := pagesByFloor[f.ID]
		ep := make([]ThchatPage, len(fp))
		for j, p := range fp {
			ep[j] = ThchatPage{
				ID: p.ID, IsActive: p.IsActive,
				Messages: json.RawMessage(p.Messages),
				PageVars: json.RawMessage(p.PageVars),
				TokenUsed: p.TokenUsed,
			}
		}
		exportFloors[i] = ThchatFloor{
			ID: f.ID, Seq: f.Seq, BranchID: f.BranchID,
			Status: string(f.Status), Pages: ep,
		}
	}

	// 记忆
	var mems []dbmodels.Memory
	e.db.Where("session_id = ?", sessionID).Find(&mems)
	exportMems := make([]ThchatMemory, len(mems))
	for i, m := range mems {
		exportMems[i] = ThchatMemory{
			ID: m.ID, FactKey: m.FactKey, Content: m.Content,
			Type: string(m.Type), Importance: m.Importance,
			SourceFloor: m.SourceFloor, Deprecated: m.Deprecated,
			StageTags: json.RawMessage(m.StageTags),
		}
	}

	// 记忆边
	var edges []dbmodels.MemoryEdge
	e.db.Where("session_id = ?", sessionID).Find(&edges)
	exportEdges := make([]ThchatEdge, len(edges))
	for i, ed := range edges {
		exportEdges[i] = ThchatEdge{
			ID: ed.ID, FromID: ed.FromID, ToID: ed.ToID,
			Relation: string(ed.Relation),
		}
	}

	// 分支
	var branches []dbmodels.SessionBranch
	e.db.Where("session_id = ?", sessionID).Find(&branches)
	exportBranches := make([]ThchatBranch, len(branches))
	for i, b := range branches {
		exportBranches[i] = ThchatBranch{
			ID: b.ID, BranchID: b.BranchID,
			ParentBranch: b.ParentBranch, OriginSeq: b.OriginSeq,
		}
	}

	return &ThchatExport{
		Version:    "1.0",
		Format:     "thchat",
		ExportedAt: time.Now(),
		GameID:     sess.GameID,
		GameTitle:  gameTitle,
		Session: ThchatSession{
			ID: sess.ID, Title: sess.Title, Status: sess.Status,
			Variables: json.RawMessage(sess.Variables),
			MemorySummary: sess.MemorySummary, FloorCount: sess.FloorCount,
			CharacterCardID:   sess.CharacterCardID,
			CharacterSnapshot: json.RawMessage(sess.CharacterSnapshot),
			CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
		},
		Floors:   exportFloors,
		Memories: exportMems,
		MemEdges: exportEdges,
		Branches: exportBranches,
	}, nil
}

// ExportJSONL 导出会话为 ST 兼容 JSONL 格式（仅 main 分支 active page 消息）
func (e *GameEngine) ExportJSONL(sessionID string) ([]byte, error) {
	var floors []dbmodels.Floor
	e.db.Where("session_id = ? AND branch_id = 'main' AND status = 'committed'", sessionID).
		Order("seq ASC").Find(&floors)

	floorIDs := make([]string, len(floors))
	for i, f := range floors {
		floorIDs[i] = f.ID
	}

	var pages []dbmodels.MessagePage
	if len(floorIDs) > 0 {
		e.db.Where("floor_id IN ? AND is_active = true", floorIDs).Find(&pages)
	}
	activeByFloor := map[string]dbmodels.MessagePage{}
	for _, p := range pages {
		activeByFloor[p.FloorID] = p
	}

	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	for _, f := range floors {
		p, ok := activeByFloor[f.ID]
		if !ok {
			continue
		}
		var msgs []msg
		if json.Unmarshal(p.Messages, &msgs) != nil {
			continue
		}
		for _, m := range msgs {
			enc.Encode(m)
		}
	}
	return buf.Bytes(), nil
}

// ── 导入 ─────────────────────────────────────────────────────────────────────

// ImportThchat 从 .thchat 导入会话，所有 ID 重映射为新 UUID
func (e *GameEngine) ImportThchat(data *ThchatExport, userID string) (string, error) {
	newSessionID := uuid.New().String()

	err := e.db.Transaction(func(tx *gorm.DB) error {
		// 会话
		sess := dbmodels.GameSession{
			ID:                newSessionID,
			GameID:            data.GameID,
			UserID:            userID,
			Title:             data.Session.Title,
			Status:            "active",
			Variables:         datatypes.JSON(data.Session.Variables),
			MemorySummary:     data.Session.MemorySummary,
			CharacterCardID:   data.Session.CharacterCardID,
			CharacterSnapshot: datatypes.JSON(data.Session.CharacterSnapshot),
		}
		if err := tx.Create(&sess).Error; err != nil {
			return err
		}

		// ID 映射表
		floorMap := map[string]string{}  // old → new
		memMap := map[string]string{}

		// 楼层 + 页面
		for _, f := range data.Floors {
			newFloorID := uuid.New().String()
			floorMap[f.ID] = newFloorID
			floor := dbmodels.Floor{
				ID: newFloorID, SessionID: newSessionID,
				Seq: f.Seq, BranchID: f.BranchID,
				Status: dbmodels.FloorStatus(f.Status),
			}
			if err := tx.Create(&floor).Error; err != nil {
				return err
			}
			for _, p := range f.Pages {
				page := dbmodels.MessagePage{
					ID: uuid.New().String(), FloorID: newFloorID,
					IsActive: p.IsActive,
					Messages: datatypes.JSON(p.Messages),
					PageVars: datatypes.JSON(p.PageVars),
					TokenUsed: p.TokenUsed,
				}
				if err := tx.Create(&page).Error; err != nil {
					return err
				}
			}
		}

		// 记忆
		for _, m := range data.Memories {
			newMemID := uuid.New().String()
			memMap[m.ID] = newMemID
			mem := dbmodels.Memory{
				ID: newMemID, SessionID: newSessionID,
				FactKey: m.FactKey, Content: m.Content,
				Type: dbmodels.MemoryType(m.Type), Importance: m.Importance,
				SourceFloor: m.SourceFloor, Deprecated: m.Deprecated,
				StageTags: datatypes.JSON(m.StageTags),
			}
			if err := tx.Create(&mem).Error; err != nil {
				return err
			}
		}

		// 记忆边（跳过引用不存在的记忆）
		for _, ed := range data.MemEdges {
			newFrom, ok1 := memMap[ed.FromID]
			newTo, ok2 := memMap[ed.ToID]
			if !ok1 || !ok2 {
				continue
			}
			edge := dbmodels.MemoryEdge{
				ID: uuid.New().String(), SessionID: newSessionID,
				FromID: newFrom, ToID: newTo,
				Relation: dbmodels.MemoryRelation(ed.Relation),
			}
			if err := tx.Create(&edge).Error; err != nil {
				return err
			}
		}

		// 分支
		for _, b := range data.Branches {
			branch := dbmodels.SessionBranch{
				ID: uuid.New().String(), SessionID: newSessionID,
				BranchID: b.BranchID, ParentBranch: b.ParentBranch,
				OriginSeq: b.OriginSeq,
			}
			if err := tx.Create(&branch).Error; err != nil {
				return err
			}
		}

		// 更新 floor_count
		return tx.Model(&dbmodels.GameSession{}).Where("id = ?", newSessionID).
			Update("floor_count", len(data.Floors)).Error
	})

	if err != nil {
		return "", err
	}
	return newSessionID, nil
}

// ── 路由 handler ─────────────────────────────────────────────────────────────

func registerExportImportRoutes(rg *gin.RouterGroup, engine *GameEngine) {
	// GET /sessions/:id/export?format=thchat|jsonl
	rg.GET("/sessions/:id/export", func(c *gin.Context) {
		sid := c.Param("id")
		format := c.DefaultQuery("format", "thchat")

		switch format {
		case "jsonl":
			data, err := engine.ExportJSONL(sid)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.Header("Content-Disposition", "attachment; filename=session-"+sid+".jsonl")
			c.Data(http.StatusOK, "text/plain; charset=utf-8", data)

		default: // thchat
			export, err := engine.ExportThchat(sid)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.Header("Content-Disposition", "attachment; filename=session-"+sid+".thchat")
			c.JSON(http.StatusOK, export)
		}
	})

	// POST /sessions/import
	rg.POST("/sessions/import", func(c *gin.Context) {
		var data ThchatExport
		if err := c.ShouldBindJSON(&data); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if data.Format != "thchat" || data.Version == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid format, expected thchat"})
			return
		}
		userID := c.GetString("account_id")
		sessID, err := engine.ImportThchat(&data, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"session_id": sessID}})
	})
}
