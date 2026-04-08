// Package memory — 记忆摘要 Worker。
//
// Worker 封装了记忆整合的完整生命周期，供 cmd/worker/main.go 启动。
// 设计原则：
//   - 单一职责：只做记忆整合，不触碰 HTTP / 路由
//   - 可配置：所有参数通过 WorkerConfig 注入，无硬编码
//   - In-memory lease：防止同批次对同一 session 重复处理
//   - Graceful shutdown：ctx 取消后等待所有 in-flight goroutine 完成
//   - 批次并发限制：最多 MaxConcurrent 个 goroutine 同时调用 LLM
package memory

import (
	"context"
	"log"
	"sync"
	"time"

	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/session"
)

// WorkerConfig 记忆 Worker 的可配置参数（来自 config.WorkerConfig）。
type WorkerConfig struct {
	TriggerRounds  int           // 每 N 回合触发整合
	MaxTokens      int           // 摘要 LLM 最大输出 token
	TokenBudget    int           // 注入摘要时的 token 预算
	BatchSize      int           // 每次扫描最多处理几个 session
	MaxConcurrent  int           // 最大并发 LLM 调用数
	PollInterval   time.Duration // 整合扫描间隔
	LeaseTTL       time.Duration // 会话处理租约有效期（防双处理）

	// ── 维护策略（对应 TH MemoryMaintenancePolicy）─────────────────
	DeprecateAfterDays  int           // 超过 N 天的 summary 记忆自动废弃（0 = 禁用）
	PurgeAfterDays      int           // deprecated 且超过 N 天的记忆物理删除（0 = 禁用）
	MaintenanceInterval time.Duration // 维护扫描间隔（独立于整合轮询）
}

// defaults 填充 WorkerConfig 中的零值
func (c *WorkerConfig) defaults() {
	if c.TriggerRounds <= 0 {
		c.TriggerRounds = 10
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 512
	}
	if c.TokenBudget <= 0 {
		c.TokenBudget = 600
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 4
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 30 * time.Second
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 2 * time.Minute
	}
	// 维护策略：零值 = 禁用，不覆盖（让调用方显式传 0 来关闭）
	if c.MaintenanceInterval <= 0 {
		c.MaintenanceInterval = time.Hour
	}
}

// lease 表示某个 session 正在被处理的租约
type lease struct {
	expiresAt time.Time
}

// Worker 异步记忆整合服务对象。
type Worker struct {
	llmClient  *llm.Client
	memStore   *Store
	sessionMgr *session.Manager
	cfg        WorkerConfig

	// in-memory lease 集合：sessionID → lease
	leases sync.Map
}

// NewWorker 创建 Worker。
//
//	llmClient — 用于记忆摘要的（廉价）LLM 客户端
//	memStore  — 记忆存储
//	sessionMgr — session 管理器（用于读取对话历史）
func NewWorker(
	llmClient *llm.Client,
	memStore *Store,
	sessionMgr *session.Manager,
	cfg WorkerConfig,
) *Worker {
	cfg.defaults()
	return &Worker{
		llmClient:  llmClient,
		memStore:   memStore,
		sessionMgr: sessionMgr,
		cfg:        cfg,
	}
}

// Run 启动 Worker 主循环，阻塞直到 ctx 取消。
//
// 启动时立即执行一次整合扫描；维护扫描按独立定时器运行。
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[worker] started — trigger every %d rounds, poll %s, maintenance %s, batch %d, max_concurrent %d",
		w.cfg.TriggerRounds, w.cfg.PollInterval, w.cfg.MaintenanceInterval,
		w.cfg.BatchSize, w.cfg.MaxConcurrent)

	// 启动时立即整合扫描一次
	w.processBatch(ctx)

	consolidateTicker := time.NewTicker(w.cfg.PollInterval)
	defer consolidateTicker.Stop()

	maintenanceTicker := time.NewTicker(w.cfg.MaintenanceInterval)
	defer maintenanceTicker.Stop()

	for {
		select {
		case <-consolidateTicker.C:
			w.processBatch(ctx)
		case <-maintenanceTicker.C:
			w.runMaintenance()
		case <-ctx.Done():
			log.Printf("[worker] shutting down, waiting for in-flight jobs…")
			w.drainLeases()
			log.Printf("[worker] stopped")
			return
		}
	}
}

// runMaintenance 执行全局记忆维护：废弃过期摘要 + 物理删除长期废弃条目。
func (w *Worker) runMaintenance() {
	if w.cfg.DeprecateAfterDays > 0 {
		n, err := w.memStore.DeprecateOldMemoriesGlobal(w.cfg.DeprecateAfterDays)
		if err != nil {
			log.Printf("[worker] maintenance deprecate: %v", err)
		} else if n > 0 {
			log.Printf("[worker] maintenance: deprecated %d summary memories (>%dd)", n, w.cfg.DeprecateAfterDays)
		}
	}
	if w.cfg.PurgeAfterDays > 0 {
		n, err := w.memStore.PurgeDeprecatedMemoriesGlobal(w.cfg.PurgeAfterDays)
		if err != nil {
			log.Printf("[worker] maintenance purge: %v", err)
		} else if n > 0 {
			log.Printf("[worker] maintenance: purged %d deprecated memories (>%dd)", n, w.cfg.PurgeAfterDays)
		}
	}
}

// processBatch 扫描一批需要整合的 session，并发处理。
func (w *Worker) processBatch(ctx context.Context) {
	sessions, err := w.memStore.FindSessionsNeedingConsolidation(w.cfg.TriggerRounds, w.cfg.BatchSize)
	if err != nil {
		log.Printf("[worker] scan error: %v", err)
		return
	}
	if len(sessions) == 0 {
		return
	}

	log.Printf("[worker] found %d session(s) to consolidate", len(sessions))

	// 信号量：限制并发数
	sem := make(chan struct{}, w.cfg.MaxConcurrent)
	var wg sync.WaitGroup

	for _, sess := range sessions {
		// 检查 ctx 取消
		select {
		case <-ctx.Done():
			break
		default:
		}

		// 跳过仍在租约内的 session
		if !w.tryAcquireLease(sess.ID) {
			log.Printf("[worker] session %s is leased, skipping", sess.ID)
			continue
		}

		// 占用信号量槽
		sem <- struct{}{}
		wg.Add(1)

		go func(sessionID string, floorCount int) {
			defer func() { <-sem }()
			defer wg.Done()
			defer w.releaseLease(sessionID)

			w.processSession(ctx, sessionID, floorCount)
		}(sess.ID, sess.FloorCount)
	}

	wg.Wait()
}

// processSession 对单个 session 执行记忆整合。
func (w *Worker) processSession(ctx context.Context, sessionID string, floorCount int) {
	// 1. 拉取近期对话历史
	history, err := w.sessionMgr.GetHistory(sessionID, "main", 20)
	if err != nil {
		log.Printf("[worker] get history %s: %v", sessionID, err)
		return
	}

	// 2. 构建整合 prompt
	prompt, err := w.memStore.BuildConsolidationPrompt(sessionID, history)
	if err != nil {
		log.Printf("[worker] build prompt %s: %v", sessionID, err)
		return
	}

	// 3. LLM 调用（廉价模型）
	resp, err := w.llmClient.Chat(ctx, []llm.Message{
		{Role: "user", Content: prompt},
	}, llm.Options{MaxTokens: w.cfg.MaxTokens})
	if err != nil {
		log.Printf("[worker] llm call %s: %v", sessionID, err)
		return
	}

	// 4. 解析并落库
	if err := w.memStore.ParseConsolidationResult(sessionID, resp.Content, floorCount); err != nil {
		log.Printf("[worker] save result %s: %v", sessionID, err)
		return
	}

	// 5. 更新 session 摘要缓存（Pipeline 直读，0 延迟）
	summary, _ := w.memStore.GetForInjection(sessionID, w.cfg.TokenBudget, "")
	if err := w.memStore.UpdateSessionSummaryCache(sessionID, summary); err != nil {
		log.Printf("[worker] update cache %s: %v", sessionID, err)
		return
	}

	log.Printf("[worker] ✓ session %s (floor=%d)", sessionID, floorCount)
}

// ── In-memory lease ───────────────────────────────────────

func (w *Worker) tryAcquireLease(sessionID string) bool {
	now := time.Now()
	// 清理过期租约
	if v, loaded := w.leases.Load(sessionID); loaded {
		if v.(lease).expiresAt.After(now) {
			return false // 租约未过期，不能获取
		}
	}
	// 原子地写入新租约（LoadOrStore 如果已有值则返回旧值）
	_, loaded := w.leases.LoadOrStore(sessionID, lease{expiresAt: now.Add(w.cfg.LeaseTTL)})
	if loaded {
		// 有其他 goroutine 抢先设置了租约
		if v, _ := w.leases.Load(sessionID); v.(lease).expiresAt.After(now) {
			return false
		}
		// 旧租约已过期，强制覆盖
		w.leases.Store(sessionID, lease{expiresAt: now.Add(w.cfg.LeaseTTL)})
	}
	return true
}

func (w *Worker) releaseLease(sessionID string) {
	w.leases.Delete(sessionID)
}

// drainLeases 等待所有持有租约的 session 完成（优雅退出用）。
// 最多等待 LeaseTTL，避免无限阻塞。
func (w *Worker) drainLeases() {
	deadline := time.Now().Add(w.cfg.LeaseTTL)
	for time.Now().Before(deadline) {
		count := 0
		w.leases.Range(func(_, _ any) bool {
			count++
			return true
		})
		if count == 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
