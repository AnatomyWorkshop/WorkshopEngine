package types

import (
	"time"
)

// Session 代表一次完整的游玩会话
type Session struct {
	ID        string            `json:"id"`
	GameID    string            `json:"game_id"`   // 关联的游戏模板/角色卡
	UserID    string            `json:"user_id"`
	Variables map[string]any    `json:"variables"` // Chat(Session) 级变量持久化
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// FloorStatus 楼层状态机
type FloorStatus string

const (
	FloorStatusDraft      FloorStatus = "draft"      // 等待生成
	FloorStatusGenerating FloorStatus = "generating" // 正在调用LLM
	FloorStatusCommitted  FloorStatus = "committed"  // 生成完成且玩家已接受，锁定不可改
	FloorStatusFailed     FloorStatus = "failed"     // 生成失败
)

// Floor 代表一个游戏回合 (如：玩家说了一句话，AI回复的过程就是一层)
type Floor struct {
	ID        string      `json:"id"`
	SessionID string      `json:"session_id"`
	Seq       int         `json:"seq"`    // 楼层顺序，决定对话时间线
	Status    FloorStatus `json:"status"` // 状态机控制不可变性
	CreatedAt time.Time   `json:"created_at"`
}

// MessageRole 消息角色
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleNarrator  MessageRole = "narrator"
)

// Message 代表单条信息
type Message struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

// MessagePage 代表楼层中的一个生成版本。
// 玩家点击“重新生成(Swipe)”，就会在同一个 Floor 下产生一个新的 Page。
type MessagePage struct {
	ID         string         `json:"id"`
	FloorID    string         `json:"floor_id"`
	IsActive   bool           `json:"is_active"`   // 当前楼层生效的是哪一页
	Messages   []Message      `json:"messages"`    // 当前版本的消息(如用户输入+AI输出)
	PageVars   map[string]any `json:"page_vars"`   // Page 级变量沙箱！重试时不污染Session
	TokenUsage int            `json:"token_usage"` // 可选：记录本次消耗
	CreatedAt  time.Time      `json:"created_at"`
}
