// Package memory — 记忆摘要 Worker。
//
// Worker 封装了记忆整合的完整生命周期，供 cmd/server/main.go 启动。
// P-4G：任务调度已迁移至 scheduler.Scheduler（DB 持久化），
// Worker 只负责消费 memory_consolidation 类型的 Job 并执行整合逻辑。
package memory

import (
	"context"
	"log"
	"sync"
	"time"

	"mvu-backend/internal/core/llm"
	"mvu-backend/internal/engine/scheduler"
	"mvu-backend/internal/engine/session"
)

// WorkerConfig 记忆 Worker 的可配置参数（来自 config.WorkerConfig）。
type WorkerConfig struct {
	TriggerRounds  int           // 每 N 回合触发整合
	MaxTokens      int           // 摘要 LLM 最大输出 token
	TokenBudget    int           // 注入摘要时的 token 预算
	MaxConcurrent  int           // 最大并发 LLM 调用数
	PollInterval   time.Duration // Job 轮询间隔

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
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 4
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 10 * time.Second
	}
	if c.MaintenanceInterval <= 0 {
		c.MaintenanceInterval = time.Hour
	}
}

// Worker 异步记忆整合服务对象。
type Worker struct {
	llmClient  llm.Provider
	memStore   *Store
	sessionMgr *session.Manager
	sched      *scheduler.Scheduler
	cfg        WorkerConfig
}

// NewWorker 创建 Worker。
func NewWorker(
	llmClient llm.Provider,
	memStore *Store,
	sessionMgr *session.Manager,
	sched *scheduler.Scheduler,
	cfg WorkerConfig,
) *Worker {
	cfg.defaults()
	return &Worker{
		llmClient:  llmClient,
		memStore:   memStore,
		sessionMgr: sessionMgr,
		sched:      sched,
		cfg:        cfg,
	}
}

// Run 启动 Worker 主循环，阻塞直到 ctx 取消。
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[worker] started — trigger every %d rounds, poll %s, maintenance %s, max_concurrent %d",
		w.cfg.TriggerRounds, w.cfg.PollInterval, w.cfg.MaintenanceInterval, w.cfg.MaxConcurrent)

	// 启动时恢复超时租约
	w.sched.RecoverStale()

	// 立即处理一批
	w.processBatch(ctx)

	pollTicker := time.NewTicker(w.cfg.PollInterval)
	defer pollTicker.Stop()

	maintenanceTicker := time.NewTicker(w.cfg.MaintenanceInterval)
	defer maintenanceTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			w.processBatch(ctx)
		case <-maintenanceTicker.C:
			w.runMaintenance()
		case <-ctx.Done():
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
	// 清理 7 天前的已完成/死信任务
	w.sched.CleanDone(7)
}

// processBatch 从 scheduler 租约获取任务并并发执行。
func (w *Worker) processBatch(ctx context.Context) {
	sem := make(chan struct{}, w.cfg.MaxConcurrent)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			break
		default:
		}

		job, err := w.sched.LeaseJob(ctx, "memory_consolidation")
		if err != nil {
			log.Printf("[worker] lease error: %v", err)
			break
		}
		if job == nil {
			break // 无更多任务
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(jobID, sessionID string) {
			defer func() { <-sem }()
			defer wg.Done()

			if err := w.processSession(ctx, sessionID); err != nil {
				log.Printf("[worker] session %s failed: %v", sessionID, err)
				w.sched.Fail(jobID, err.Error())
			} else {
				w.sched.Complete(jobID)
				log.Printf("[worker] session %s done", sessionID)
			}
		}(job.ID, job.SessionID)
	}

	wg.Wait()
}

// processSession 对单个 session 执行记忆整合。
func (w *Worker) processSession(ctx context.Context, sessionID string) error {
	history, err := w.sessionMgr.GetHistory(sessionID, "main", 20)
	if err != nil {
		return err
	}

	prompt, err := w.memStore.BuildConsolidationPrompt(sessionID, history)
	if err != nil {
		return err
	}

	resp, err := w.llmClient.Chat(ctx, []llm.Message{
		{Role: "user", Content: prompt},
	}, llm.Options{MaxTokens: w.cfg.MaxTokens})
	if err != nil {
		return err
	}

	floorCount := w.memStore.GetFloorCount(sessionID)

	if err := w.memStore.ParseConsolidationResult(sessionID, resp.Content, floorCount); err != nil {
		return err
	}

	summary, _ := w.memStore.GetForInjection(sessionID, w.cfg.TokenBudget, "")
	return w.memStore.UpdateSessionSummaryCache(sessionID, summary)
}
