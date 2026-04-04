package variable

import (
	"encoding/json"
	"strings"
	"sync"
)

// Scope 变量作用域定义
type Scope string

const (
	ScopeGlobal  Scope = "global" // 游戏全局配置/世界观设定
	ScopeChat    Scope = "chat"   // (Session级) 玩家当前游玩会话
	ScopeBranch  Scope = "branch" // (预留) 剧情分支
	ScopeFloor   Scope = "floor"  // 回合级锁定变量
	ScopePage    Scope = "page"   // 生成过程中的沙箱变量(最高优先级读取，默认写入)
)

// Sandbox 提供级联的变量读写机制
type Sandbox struct {
	mu sync.RWMutex

	// 实际数据源，按优先级自上而下 (Page 最优先)
	pageVars   map[string]any
	floorVars  map[string]any
	branchVars map[string]any
	chatVars   map[string]any
	globalVars map[string]any
}

// NewSandbox 构造一个新的沙箱。需要传入各层级的现有状态副本。
func NewSandbox(global, chat, branch, floor, page map[string]any) *Sandbox {
	sb := &Sandbox{
		pageVars:   make(map[string]any),
		floorVars:  make(map[string]any),
		branchVars: make(map[string]any),
		chatVars:   make(map[string]any),
		globalVars: make(map[string]any),
	}
	// 深拷贝或浅拷贝取决于性能要求，这里为了安全做简单浅合并
	// 在实际接入DB时，这些字典从 DB 反序列化而来
	mergeMap(sb.globalVars, global)
	mergeMap(sb.chatVars, chat)
	mergeMap(sb.branchVars, branch)
	mergeMap(sb.floorVars, floor)
	mergeMap(sb.pageVars, page)
	return sb
}

func mergeMap(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

// Get 级联读取：Page -> Floor -> Branch -> Chat -> Global
func (s *Sandbox) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.pageVars[key]; ok {
		return val, true
	}
	if val, ok := s.floorVars[key]; ok {
		return val, true
	}
	if val, ok := s.branchVars[key]; ok {
		return val, true
	}
	if val, ok := s.chatVars[key]; ok {
		return val, true
	}
	if val, ok := s.globalVars[key]; ok {
		return val, true
	}
	return nil, false
}

// Set 默认安全写入机制：始终写到 Page 级（沙箱）。
// 这样在重试（Regen）时直接丢弃整个 Page，不会污染持久化的 Chat 数据。
func (s *Sandbox) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageVars[key] = value
}

// SetScope 显式提升或降级写入到指定作用域。
// (在内部合并或特殊逻辑才使用)
func (s *Sandbox) SetScope(scope Scope, key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch scope {
	case ScopePage:
		s.pageVars[key] = value
	case ScopeFloor:
		s.floorVars[key] = value
	case ScopeBranch:
		s.branchVars[key] = value
	case ScopeChat:
		s.chatVars[key] = value
	case ScopeGlobal:
		s.globalVars[key] = value
	}
}

// CommitPageToChat 状态提升！
// 当玩家接受了当前的生成结果，或者一回合正常结束时。
// 引擎会调用此方法，将临时沙箱的改动固化到持久层(Chat)。
func (s *Sandbox) CommitPageToChat() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range s.pageVars {
		s.chatVars[k] = v // 提升
	}
	// 清空 Page 沙箱准备下一回合
	s.pageVars = make(map[string]any)
	return s.chatVars
}

// Flatten 获取当前整个合并后快照，通常用于作为模板变量注入 LLM Prompt 中
func (s *Sandbox) Flatten() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]any)
	// 合并顺序：全局打底，逐层覆盖
	mergeMap(res, s.globalVars)
	mergeMap(res, s.chatVars)
	mergeMap(res, s.branchVars)
	mergeMap(res, s.floorVars)
	mergeMap(res, s.pageVars)
	return res
}

// FlatJSON 返回 JSON 字符串格式，用于给前端响应或存储
func (s *Sandbox) FlatJSON() string {
	b, _ := json.Marshal(s.Flatten())
	return string(b)
}

// ResolveString 简单的字符串插值支持 (如 "{{char}}", "{{hp}}") 替换
func (s *Sandbox) ResolveString(tmpl string) string {
	flattened := s.Flatten()
	res := tmpl
	for k, v := range flattened {
		// 极其简易的替换。复杂宏替换会交给 pipeline 的 TransformNode 处理。
		strVal := ""
		switch val := v.(type) {
		case string:
			strVal = val
		case float64:
			// 简单数字转换
			b, _ := json.Marshal(val)
			strVal = string(b)
		}
		res = strings.ReplaceAll(res, "{{"+k+"}}", strVal)
	}
	return res
}
