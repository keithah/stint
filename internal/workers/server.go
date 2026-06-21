package workers

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/cache"
	"github.com/keithah/stint/internal/config"
	"github.com/keithah/stint/internal/db"
	"github.com/keithah/stint/internal/jobs"
)

const goalsEvaluateScheduleSpec = "@hourly"

func Run(ctx context.Context, cfg config.Config, store *db.Store) error {
	opt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		return err
	}
	server := asynq.NewServer(opt, asynq.Config{Concurrency: 5})
	mux := asynq.NewServeMux()
	stats := StatsWorker{Store: store}
	mux.HandleFunc(jobs.TypeStatsRecompute, stats.HandleStatsRecomputeTask)
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

	scheduler := asynq.NewScheduler(opt, &asynq.SchedulerOpts{Location: time.UTC})
	hasScheduledTasks := false
	goalsTask, err := jobs.NewScheduledGoalsEvaluateTask()
	if err != nil {
		return err
	}
	if _, err := scheduler.Register(goalsEvaluateScheduleSpec, goalsTask, asynq.Queue("default"), asynq.MaxRetry(3)); err != nil {
		return err
	}
	hasScheduledTasks = true
	leaderboardTask, err := jobs.NewLeaderboardUpdateTask("last_7_days")
	if err != nil {
		return err
	}
	if _, err := scheduler.Register("@hourly", leaderboardTask, asynq.Queue("default"), asynq.MaxRetry(3)); err != nil {
		return err
	}
	hasScheduledTasks = true
	heartbeatsTask, err := jobs.NewHeartbeatsPurgeTask(cfg.HeartbeatRetentionDays, time.Time{})
	if err != nil {
		return err
	}
	if _, err := scheduler.Register("@weekly", heartbeatsTask, asynq.Queue("default"), asynq.MaxRetry(3)); err != nil {
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
