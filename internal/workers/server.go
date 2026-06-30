package workers

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/cache"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
	"github.com/keithah/stint/internal/pricingrefresh"
)

const goalsEvaluateScheduleSpec = "@hourly"

func Run(ctx context.Context, cfg config.Config, store *db.Store) error {
	opt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		return err
	}
	server := asynq.NewServer(opt, asynq.Config{
		Concurrency: workerConcurrency(),
		Queues: map[string]int{
			jobs.QueueCritical:    6,
			jobs.QueueDefault:     4,
			jobs.QueueMaintenance: 2,
			jobs.QueueBulk:        1,
		},
	})
	mux := asynq.NewServeMux()
	stats := NewStatsWorker(store, nil)
	mux.HandleFunc(jobs.TypeStatsRecompute, stats.HandleStatsRecomputeTask)
	mux.HandleFunc(jobs.TypeProjectStatsRecompute, stats.HandleProjectStatsRecomputeTask)
	// Keep the worker's shared pricing engine in sync with the weekly refresh so
	// cost baked into the stats cache uses up-to-date prices.
	if eng := stats.pricingEngine(); eng != nil {
		go pricingrefresh.Refresher{Store: store, Engine: eng}.Run(ctx, 30*time.Minute)
	}
	dumps := DumpWorker{Store: store, Config: cfg}
	mux.HandleFunc(jobs.TypeDataDumpProcess, dumps.HandleDataDumpProcessTask)
	customRules := CustomRulesWorker{Store: store}
	mux.HandleFunc(jobs.TypeCustomRulesApply, customRules.HandleCustomRulesApplyTask)
	imports := ImportWorker{Store: store}
	mux.HandleFunc(jobs.TypeWakaTimeImport, imports.HandleWakaTimeImportTask)
	heartbeats := HeartbeatsWorker{Store: store}
	mux.HandleFunc(jobs.TypeHeartbeatsPurge, heartbeats.HandleHeartbeatsPurgeTask)
	leaderboardCache, err := cache.NewRedisLeaderboardCache(cfg.RedisURL)
	if err != nil {
		return err
	}
	leaderboard := LeaderboardWorker{Store: store, Cache: leaderboardCache}
	mux.HandleFunc(jobs.TypeLeaderboardUpdate, leaderboard.HandleLeaderboardUpdateTask)
	goals := GoalsWorker{Store: store}
	mux.HandleFunc(jobs.TypeGoalsEvaluate, goals.HandleGoalsEvaluateTask)
	pricingRefresh := PricingWorker{Store: store}
	mux.HandleFunc(jobs.TypePricingRefresh, pricingRefresh.HandlePricingRefreshTask)

	scheduler := asynq.NewScheduler(opt, &asynq.SchedulerOpts{Location: time.UTC})
	hasScheduledTasks := false
	goalsTask, err := jobs.NewScheduledGoalsEvaluateTask()
	if err != nil {
		return err
	}
	if _, err := scheduler.Register(goalsEvaluateScheduleSpec, goalsTask, asynq.Queue(jobs.QueueMaintenance), asynq.MaxRetry(3)); err != nil {
		return err
	}
	hasScheduledTasks = true
	leaderboardTask, err := jobs.NewLeaderboardUpdateTask("last_7_days")
	if err != nil {
		return err
	}
	if _, err := scheduler.Register("@hourly", leaderboardTask, asynq.Queue(jobs.QueueDefault), asynq.MaxRetry(3)); err != nil {
		return err
	}
	hasScheduledTasks = true
	heartbeatsTask, err := jobs.NewHeartbeatsPurgeTask(cfg.HeartbeatRetentionDays, time.Time{})
	if err != nil {
		return err
	}
	if _, err := scheduler.Register("@weekly", heartbeatsTask, asynq.Queue(jobs.QueueMaintenance), asynq.MaxRetry(3)); err != nil {
		return err
	}
	hasScheduledTasks = true
	pricingTask, err := jobs.NewPricingRefreshTask()
	if err != nil {
		return err
	}
	if _, err := scheduler.Register("@weekly", pricingTask, asynq.Queue(jobs.QueueMaintenance), asynq.MaxRetry(3)); err != nil {
		return err
	}
	hasScheduledTasks = true
	if hasScheduledTasks {
		go func() {
			_ = scheduler.Run()
		}()
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(mux)
	}()
	select {
	case <-ctx.Done():
		if hasScheduledTasks {
			scheduler.Shutdown()
		}
		server.Shutdown()
		return ctx.Err()
	case err := <-errCh:
		if hasScheduledTasks {
			scheduler.Shutdown()
		}
		return err
	}
}

func workerConcurrency() int {
	raw := strings.TrimSpace(os.Getenv("STINT_WORKER_CONCURRENCY"))
	if raw == "" {
		return 10
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 10
	}
	return value
}
