// Package scheduler — DB 持久化的后台任务调度器（P-4G）。
//
// 替代 memory/worker.go 中的 sync.Map 内存租约，所有任务状态持久化到 runtime_job 表。
// 进程重启后自动恢复 leased 超时的任务。
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	db "mvu-backend/internal/core/db"
)

// Scheduler DB 持久化任务调度器。
type Scheduler struct {
	db           *gorm.DB
	leaseTTL     time.Duration
	maxRetries   int
	pollInterval time.Duration
}

// Config 调度器配置。
type Config struct {
	LeaseTTL     time.Duration // 租约有效期（默认 2 分钟）
	MaxRetries   int           // 最大重试次数（默认 3）
	PollInterval time.Duration // 轮询间隔（默认 10 秒）
}

func (c *Config) defaults() {
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 2 * time.Minute
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 10 * time.Second
	}
}

// New 创建调度器。
func New(gormDB *gorm.DB, cfg Config) *Scheduler {
	cfg.defaults()
	return &Scheduler{
		db:           gormDB,
		leaseTTL:     cfg.LeaseTTL,
		maxRetries:   cfg.MaxRetries,
		pollInterval: cfg.PollInterval,
	}
}

// Enqueue 入队一个任务。dedupeKey 非空时去重（已存在则跳过）。
func (s *Scheduler) Enqueue(jobType, sessionID, dedupeKey string) error {
	if dedupeKey == "" {
		dedupeKey = jobType + ":" + sessionID
	}
	job := db.RuntimeJob{
		ID:         uuid.New().String(),
		JobType:    jobType,
		SessionID:  sessionID,
		Status:     db.JobQueued,
		MaxRetries: s.maxRetries,
		DedupeKey:  dedupeKey,
	}
	result := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "dedupe_key"}},
		DoNothing: true,
	}).Create(&job)
	return result.Error
}

// LeaseJob 原子地获取一个 queued 任务并设置租约。返回 nil 表示无可用任务。
func (s *Scheduler) LeaseJob(ctx context.Context, jobType string) (*db.RuntimeJob, error) {
	var job db.RuntimeJob
	now := time.Now()
	leaseUntil := now.Add(s.leaseTTL)

	// 原子 UPDATE ... RETURNING（PostgreSQL）
	result := s.db.WithContext(ctx).Raw(`
		UPDATE runtime_jobs SET status = ?, lease_until = ?, updated_at = ?
		WHERE id = (
			SELECT id FROM runtime_jobs
			WHERE job_type = ? AND status = ?
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *
	`, db.JobLeased, leaseUntil, now, jobType, db.JobQueued).Scan(&job)

	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &job, nil
}

// Complete 标记任务完成，清除 dedupe_key 允许同 session 后续入队。
func (s *Scheduler) Complete(jobID string) error {
	return s.db.Model(&db.RuntimeJob{}).Where("id = ?", jobID).
		Updates(map[string]any{
			"status":     db.JobDone,
			"dedupe_key": jobID, // 改为唯一值，释放去重槽
			"updated_at": time.Now(),
		}).Error
}

// Fail 标记任务失败。retry_count < max_retries 时回到 queued，否则进入 dead。
func (s *Scheduler) Fail(jobID string, errMsg string) error {
	var job db.RuntimeJob
	if err := s.db.First(&job, "id = ?", jobID).Error; err != nil {
		return err
	}
	job.RetryCount++
	job.ErrorLog = errMsg
	job.LeaseUntil = nil
	if job.RetryCount >= job.MaxRetries {
		job.Status = db.JobDead
	} else {
		job.Status = db.JobQueued
	}
	return s.db.Save(&job).Error
}

// RecoverStale 启动时恢复超时的 leased 任务（租约过期 → 回到 queued）。
func (s *Scheduler) RecoverStale() (int64, error) {
	result := s.db.Model(&db.RuntimeJob{}).
		Where("status = ? AND lease_until < ?", db.JobLeased, time.Now()).
		Updates(map[string]any{
			"status":      db.JobQueued,
			"lease_until": nil,
			"updated_at":  time.Now(),
		})
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected > 0 {
		log.Printf("[scheduler] recovered %d stale leased jobs", result.RowsAffected)
	}
	return result.RowsAffected, nil
}

// CleanDone 清理已完成的旧任务（保留最近 N 天）。
func (s *Scheduler) CleanDone(olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := s.db.Where("status IN ? AND updated_at < ?",
		[]db.JobStatus{db.JobDone, db.JobDead}, cutoff).
		Delete(&db.RuntimeJob{})
	return result.RowsAffected, result.Error
}

// PollInterval 返回配置的轮询间隔。
func (s *Scheduler) PollInterval() time.Duration {
	return s.pollInterval
}

// CountPending 返回指定类型的待处理任务数。
func (s *Scheduler) CountPending(jobType string) (int64, error) {
	var count int64
	err := s.db.Model(&db.RuntimeJob{}).
		Where("job_type = ? AND status = ?", jobType, db.JobQueued).
		Count(&count).Error
	return count, err
}

// EnqueueIfDue 检查 session 是否满足整合条件，满足则入队。
// triggerRounds: 每 N 回合触发一次。
func (s *Scheduler) EnqueueIfDue(sessionID string, floorCount, triggerRounds int) error {
	if triggerRounds <= 0 || floorCount < triggerRounds || floorCount%triggerRounds != 0 {
		return nil
	}
	return s.Enqueue("memory_consolidation", sessionID, fmt.Sprintf("memory_consolidation:%s", sessionID))
}
