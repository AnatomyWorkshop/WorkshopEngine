package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"mvu-backend/internal/core/llm"
)

// Config 全局配置（强类型，不散落 os.Getenv）
type Config struct {
	Server ServerConfig
	DB     DBConfig
	LLM    LLMConfig
	Worker WorkerConfig
}

type ServerConfig struct {
	Port        string
	CORSOrigins []string
}

type DBConfig struct {
	DSN string // PostgreSQL DSN 或 sqlite://path
}

// LLMConfig LLM 服务配置。
//
// 采样参数使用指针：nil 表示「未配置」，调用时不发送该字段，
// 让 API / 模型使用其自身默认值。这与 SillyTavern 预设的行为一致：
// 不填 = 不覆盖模型默认，而非「设为 0」。
type LLMConfig struct {
	BaseURL    string
	APIKey     string
	Model      string // Narrator 默认模型（env: LLM_MODEL）
	TimeoutSec int
	MaxRetries int

	// 生成上限（有明确含义的零值可配置为 0，使用 int 而非指针）
	MaxTokens        int // LLM_MAX_TOKENS, default 2048
	TokenBudget      int // LLM_TOKEN_BUDGET, default 8000
	MaxHistoryFloors int // LLM_MAX_HISTORY_FLOORS, default 20
	MaxToolIter      int // LLM_MAX_TOOL_ITER, default 5

	// 采样参数（指针，nil = 不发送给 API）
	Temperature      *float64 // LLM_TEMPERATURE
	TopP             *float64 // LLM_TOP_P
	TopK             *int     // LLM_TOP_K
	FrequencyPenalty *float64 // LLM_FREQUENCY_PENALTY
	PresencePenalty  *float64 // LLM_PRESENCE_PENALTY
	ReasoningEffort  string   // LLM_REASONING_EFFORT: "low"|"medium"|"high"|""
	StopSequences    []string // LLM_STOP_SEQUENCES: 逗号分隔
}

// DefaultOptions 将 LLMConfig 的采样参数转为 llm.Options，
// 供 llm.Client.WithDefaults() 使用。
func (c *LLMConfig) DefaultOptions() llm.Options {
	return llm.Options{
		MaxTokens:        c.MaxTokens,
		Temperature:      c.Temperature,
		TopP:             c.TopP,
		TopK:             c.TopK,
		FrequencyPenalty: c.FrequencyPenalty,
		PresencePenalty:  c.PresencePenalty,
		ReasoningEffort:  c.ReasoningEffort,
		Stop:             c.StopSequences,
	}
}

type WorkerConfig struct {
	// 每隔多少回合触发一次记忆摘要整合
	MemoryTriggerEveryRounds int // MEMORY_TRIGGER_ROUNDS, default 10
	// 摘要整合使用的（廉价）模型，为空时复用 LLM.Model
	MemoryModel string // MEMORY_MODEL
	// 摘要整合 LLM 最大输出 token 数
	MemoryMaxTokens int // MEMORY_MAX_TOKENS, default 512
	// 注入记忆时的 token 预算（控制注入文本长度）
	MemoryTokenBudget int // MEMORY_TOKEN_BUDGET, default 600
	// Worker 轮询间隔（秒）
	PollIntervalSec int // WORKER_POLL_INTERVAL_SEC, default 30
	// Worker 每批扫描最多处理多少个 session
	BatchSize int // MEMORY_WORKER_BATCH_SIZE, default 20
	// Worker 最大并发 LLM 调用数
	MaxConcurrent int // MEMORY_WORKER_MAX_CONCURRENT, default 4
	// Worker 会话处理租约有效期（秒），防止同批次重复处理
	LeaseTTLSec int // MEMORY_WORKER_LEASE_TTL_SEC, default 120

	// ── 维护策略（对应 TH MemoryMaintenancePolicy）─────────────────
	// 将 N 天前的 summary 类记忆标记为 deprecated（0 = 禁用）
	DeprecateAfterDays int // MEMORY_DEPRECATE_AFTER_DAYS, default 7
	// 将 deprecated 且超过 N 天的记忆物理删除（0 = 禁用）
	PurgeAfterDays int // MEMORY_PURGE_AFTER_DAYS, default 30
	// 维护扫描间隔（秒），独立于整合轮询
	MaintenanceIntervalSec int // MEMORY_MAINTENANCE_INTERVAL_SEC, default 3600
}

// Load 从环境变量加载配置。
// 依赖外部工具（如 godotenv）在调用前将 .env 文件载入环境。
func Load() (*Config, error) {
	c := &Config{
		Server: ServerConfig{
			Port:        envOr("PORT", "8080"),
			CORSOrigins: strings.Split(envOr("CORS_ORIGINS", "http://localhost:5173"), ","),
		},
		DB: DBConfig{
			DSN: envOr("DATABASE_URL", "host=localhost user=postgres password=postgres dbname=game_workshop sslmode=disable"),
		},
		LLM: LLMConfig{
			BaseURL:          envOr("LLM_BASE_URL", "https://open.bigmodel.cn/api/paas/v4"),
			APIKey:           env("LLM_API_KEY"),
			Model:            envOr("LLM_MODEL", "glm-4-flash"),
			TimeoutSec:       envInt("LLM_TIMEOUT_SEC", 60),
			MaxRetries:       envInt("LLM_MAX_RETRIES", 2),
			MaxTokens:        envInt("LLM_MAX_TOKENS", 2048),
			TokenBudget:      envInt("LLM_TOKEN_BUDGET", 8000),
			MaxHistoryFloors: envInt("LLM_MAX_HISTORY_FLOORS", 20),
			MaxToolIter:      envInt("LLM_MAX_TOOL_ITER", 5),

			// 采样参数：nil = 不发送（让 API 使用模型默认值）
			Temperature:      envFloat("LLM_TEMPERATURE"),
			TopP:             envFloat("LLM_TOP_P"),
			TopK:             envIntPtr("LLM_TOP_K"),
			FrequencyPenalty: envFloat("LLM_FREQUENCY_PENALTY"),
			PresencePenalty:  envFloat("LLM_PRESENCE_PENALTY"),
			ReasoningEffort:  envOr("LLM_REASONING_EFFORT", ""),
			StopSequences:    envStringSlice("LLM_STOP_SEQUENCES"),
		},
		Worker: WorkerConfig{
			MemoryTriggerEveryRounds: envInt("MEMORY_TRIGGER_ROUNDS", 10),
			MemoryModel:              envOr("MEMORY_MODEL", ""),
			MemoryMaxTokens:          envInt("MEMORY_MAX_TOKENS", 512),
			MemoryTokenBudget:        envInt("MEMORY_TOKEN_BUDGET", 600),
			PollIntervalSec:          envInt("WORKER_POLL_INTERVAL_SEC", 30),
			BatchSize:                envInt("MEMORY_WORKER_BATCH_SIZE", 20),
			MaxConcurrent:            envInt("MEMORY_WORKER_MAX_CONCURRENT", 4),
			LeaseTTLSec:              envInt("MEMORY_WORKER_LEASE_TTL_SEC", 120),
			DeprecateAfterDays:       envInt("MEMORY_DEPRECATE_AFTER_DAYS", 7),
			PurgeAfterDays:           envInt("MEMORY_PURGE_AFTER_DAYS", 30),
			MaintenanceIntervalSec:   envInt("MEMORY_MAINTENANCE_INTERVAL_SEC", 3600),
		},
	}

	if c.LLM.APIKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY is required")
	}
	return c, nil
}

// ── 环境变量辅助函数 ──────────────────────────────────────

func env(key string) string    { return os.Getenv(key) }
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// envFloat 解析浮点环境变量，未设置或解析失败返回 nil。
func envFloat(key string) *float64 {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}

// envIntPtr 解析整数环境变量，未设置或解析失败返回 nil。
func envIntPtr(key string) *int {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

// envStringSlice 解析逗号分隔的字符串列表，未设置返回 nil。
func envStringSlice(key string) []string {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
