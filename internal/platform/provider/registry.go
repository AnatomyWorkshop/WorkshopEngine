// Package provider 提供多 Provider LLM 注册表和按账户/会话/插槽的动态解析。
//
// # 设计思路
//
// Registry 在进程启动时从环境变量注册静态 Provider（默认 + 记忆模型）。
// ResolveForSlot 在每次游戏回合时动态查询数据库，找出用户为该 slot 绑定的
// LLMProfile，返回对应的客户端和采样参数。
//
// 优先级（对齐 TavernHeadless）：
//  1. session-scope + 精确 slot（如 "narrator"）
//  2. global-scope  + 精确 slot
//  3. session-scope + 通配 slot "*"
//  4. global-scope  + 通配 slot "*"
//  5. Registry.Default()（env 配置兜底）
//
// 支持的 slot 名：
//   - "*"       通配，所有场合
//   - "narrator" 主叙事生成
//   - "memory"   记忆摘要整合（通常使用廉价模型）
package provider

import (
	"encoding/json"
	"sync"

	"gorm.io/gorm"
	dbmodels "mvu-backend/internal/core/db"
	"mvu-backend/internal/core/llm"
)

// ProviderID 预定义的 Provider 名称常量
const (
	IDDefault = "default" // 环境变量配置的主 Provider
	IDMemory  = "memory"  // 记忆整合用的廉价 Provider
)

// Provider 持有一个已配置的 LLM 客户端及其元数据。
type Provider struct {
	ID      string // 逻辑名，如 "default" / "memory" / "user-gpt4"
	Type    string // openai-compatible | openai | anthropic | google | deepseek | xai
	BaseURL string
	ModelID string
	client  *llm.Client // 不对外暴露；通过 Client() 获取
}

// Client 返回该 Provider 的 LLM 客户端。
func (p *Provider) Client() *llm.Client { return p.client }

// Registry 持有命名 Provider，并提供 DB-based 的 slot 解析。
// 线程安全：注册只在启动时发生，并发读取通过 RWMutex 保护。
type Registry struct {
	mu        sync.RWMutex
	providers map[string]*Provider
	defaultID string
}

// New 创建空的注册表。
func New() *Registry {
	return &Registry{providers: make(map[string]*Provider)}
}

// Register 注册一个 Provider。第一个注册的成为默认 Provider。
func (r *Registry) Register(p *Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID] = p
	if r.defaultID == "" {
		r.defaultID = p.ID
	}
}

// Get 按 ID 获取 Provider（线程安全）。
func (r *Registry) Get(id string) (*Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// Default 返回默认 Provider（第一个注册的，或 ID="default"）。
func (r *Registry) Default() *Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[r.defaultID]; ok {
		return p
	}
	return nil
}

// NewFromEnv 从已解析的配置构建含两个 Provider 的注册表：
//   - "default"：主叙事 LLM
//   - "memory"：记忆整合 LLM（memModel 为空时与 default 共享同一客户端）
func NewFromEnv(
	baseURL, apiKey, model string,
	timeoutSec, maxRetries int,
	memModel string,
) *Registry {
	r := New()

	defaultClient := llm.NewClient(baseURL, apiKey, model, timeoutSec, maxRetries)
	r.Register(&Provider{
		ID:      IDDefault,
		Type:    "openai-compatible",
		BaseURL: baseURL,
		ModelID: model,
		client:  defaultClient,
	})

	// 记忆模型：未配置时复用主模型客户端（节省连接池）
	memClient := defaultClient
	if memModel != "" && memModel != model {
		memClient = llm.NewClient(baseURL, apiKey, memModel, timeoutSec, maxRetries)
	}
	r.Register(&Provider{
		ID:      IDMemory,
		Type:    "openai-compatible",
		BaseURL: baseURL,
		ModelID: memModel,
		client:  memClient,
	})

	return r
}

// ── Slot 解析 ─────────────────────────────────────────────────────────────

// ResolveForSlot 按 TH 优先级从 DB 解析指定 slot 的活跃 LLM 配置。
//
// 返回 ok=false 时，调用方应使用 Registry.Default()。
// 单次 SQL 查询，不会因优先级层数增加而产生 N 次请求。
func (r *Registry) ResolveForSlot(
	db *gorm.DB,
	accountID, sessionID, slot string,
) (client *llm.Client, opts llm.Options, ok bool) {
	type resolveRow struct {
		BindingParams []byte `gorm:"column:binding_params"`
		ProfileID     string `gorm:"column:profile_id"`
		ModelID       string `gorm:"column:model_id"`
		BaseURL       string `gorm:"column:base_url"`
		APIKey        string `gorm:"column:api_key"`
		ProfileParams []byte `gorm:"column:profile_params"`
	}

	var row resolveRow
	err := db.Raw(`
		SELECT
			b.params      AS binding_params,
			p.id          AS profile_id,
			p.model_id    AS model_id,
			p.base_url    AS base_url,
			p.api_key     AS api_key,
			p.params      AS profile_params
		FROM llm_profile_bindings b
		JOIN llm_profiles p
		  ON p.id = b.profile_id
		 AND p.status = 'active'
		WHERE b.account_id = ?
		  AND p.status    = 'active'
		  AND (
		        (b.scope = 'session' AND b.scope_id = ?)
		     OR (b.scope = 'global'  AND b.scope_id = 'global')
		  )
		  AND (b.slot = ? OR b.slot = '*')
		ORDER BY
		  CASE
		    WHEN b.scope = 'session' AND b.slot != '*' THEN 1
		    WHEN b.scope = 'global'  AND b.slot != '*' THEN 2
		    WHEN b.scope = 'session' AND b.slot  = '*' THEN 3
		    WHEN b.scope = 'global'  AND b.slot  = '*' THEN 4
		    ELSE 5
		  END
		LIMIT 1
	`, accountID, sessionID, slot).Scan(&row).Error

	if err != nil || row.ProfileID == "" {
		return nil, llm.Options{}, false
	}

	// 构建客户端（使用 profile 中保存的 BaseURL 和 APIKey）
	// 超时/重试使用 default provider 的设置
	baseURL := row.BaseURL
	def := r.Default()
	if baseURL == "" && def != nil {
		baseURL = def.BaseURL
	}
	timeoutSec := 60
	maxRetries := 2
	if def != nil {
		timeoutSec, maxRetries = extractClientTimeouts(def.client)
	}
	profileClient := llm.NewClient(baseURL, row.APIKey, row.ModelID, timeoutSec, maxRetries)

	// 叠加采样参数：profile params → binding params（binding 优先）
	opts = paramsToOpts(row.ProfileParams, row.ModelID)
	mergeJSONParams(&opts, row.BindingParams)

	return profileClient, opts, true
}

// ── 内部工具 ──────────────────────────────────────────────

// genParams 与 game_loop.go 的 GenParams 对齐的本地类型（避免循环依赖）
type genParams struct {
	MaxTokens        *int     `json:"max_tokens,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	ReasoningEffort  *string  `json:"reasoning_effort,omitempty"`
	Stop             []string `json:"stop,omitempty"`
}

func paramsToOpts(raw []byte, modelID string) llm.Options {
	opts := llm.Options{Model: modelID}
	var p genParams
	if err := json.Unmarshal(raw, &p); err == nil {
		applyGenParams(&opts, &p)
	}
	return opts
}

func mergeJSONParams(opts *llm.Options, raw []byte) {
	var p genParams
	if err := json.Unmarshal(raw, &p); err == nil {
		applyGenParams(opts, &p)
	}
}

func applyGenParams(opts *llm.Options, p *genParams) {
	if p == nil {
		return
	}
	if p.MaxTokens != nil {
		opts.MaxTokens = *p.MaxTokens
	}
	if p.Temperature != nil {
		opts.Temperature = p.Temperature
	}
	if p.TopP != nil {
		opts.TopP = p.TopP
	}
	if p.TopK != nil {
		opts.TopK = p.TopK
	}
	if p.FrequencyPenalty != nil {
		opts.FrequencyPenalty = p.FrequencyPenalty
	}
	if p.PresencePenalty != nil {
		opts.PresencePenalty = p.PresencePenalty
	}
	if p.ReasoningEffort != nil {
		opts.ReasoningEffort = *p.ReasoningEffort
	}
	if len(p.Stop) > 0 {
		opts.Stop = p.Stop
	}
}

// extractClientTimeouts 从 Client 读取超时和重试设置（通过创建一个零值 Options 探测）。
// 由于 Client 字段是私有的，这里用保守的静态值替代；若将来 Client 暴露 getter 则直接调用。
func extractClientTimeouts(_ *llm.Client) (timeoutSec, maxRetries int) {
	return 60, 2
}

// NewProviderFromProfile 根据 LLMProfile DB 记录动态创建 Provider（供未来扩展使用）。
func NewProviderFromProfile(p dbmodels.LLMProfile, timeoutSec, maxRetries int) *Provider {
	baseURL := p.BaseURL
	client := llm.NewClient(baseURL, p.APIKey, p.ModelID, timeoutSec, maxRetries)
	return &Provider{
		ID:      "profile-" + p.ID,
		Type:    p.Provider,
		BaseURL: baseURL,
		ModelID: p.ModelID,
		client:  client,
	}
}
