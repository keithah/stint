package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/keithah/stint/internal/services"
)

var ErrQueueUnavailable = errors.New("job queue is unavailable")

type Client interface {
	EnqueueStatsRecompute(ctx context.Context, userID uuid.UUID, ranges []string) error
	EnqueueDataDumpProcess(ctx context.Context, userID, dumpID uuid.UUID) error
	EnqueueCustomRulesApply(ctx context.Context, userID uuid.UUID) error
	EnqueueWakaTimeImport(ctx context.Context, userID uuid.UUID, heartbeats []HeartbeatImportPayload, defaults services.HeartbeatDefaults) error
	EnqueueHeartbeatsPurge(ctx context.Context, retentionDays int) error
	EnqueueLeaderboardUpdate(ctx context.Context, rangeName string) error
	EnqueueGoalsEvaluate(ctx context.Context, now time.Time) error
	Close() error
}

type NoopClient struct{}

func (NoopClient) EnqueueStatsRecompute(context.Context, uuid.UUID, []string) error {
	return nil
}

func (NoopClient) EnqueueDataDumpProcess(context.Context, uuid.UUID, uuid.UUID) error {
	return ErrQueueUnavailable
}

func (NoopClient) EnqueueCustomRulesApply(context.Context, uuid.UUID) error {
	return ErrQueueUnavailable
}

func (NoopClient) EnqueueWakaTimeImport(context.Context, uuid.UUID, []HeartbeatImportPayload, services.HeartbeatDefaults) error {
	return ErrQueueUnavailable
}

func (NoopClient) EnqueueHeartbeatsPurge(context.Context, int) error {
	return ErrQueueUnavailable
}

func (NoopClient) EnqueueLeaderboardUpdate(context.Context, string) error {
	return ErrQueueUnavailable
}

func (NoopClient) EnqueueGoalsEvaluate(context.Context, time.Time) error {
	return ErrQueueUnavailable
}

func (NoopClient) Close() error {
	return nil
}

type AsynqClient struct {
	client *asynq.Client
}

func NewAsynqClient(redisURL string) (*AsynqClient, error) {
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil, err
	}
	return &AsynqClient{client: asynq.NewClient(opt)}, nil
}

func (c *AsynqClient) EnqueueStatsRecompute(ctx context.Context, userID uuid.UUID, ranges []string) error {
	task, err := NewStatsRecomputeTask(userID, ranges)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) EnqueueDataDumpProcess(ctx context.Context, userID, dumpID uuid.UUID) error {
	task, err := NewDataDumpProcessTask(userID, dumpID)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) EnqueueCustomRulesApply(ctx context.Context, userID uuid.UUID) error {
	task, err := NewCustomRulesApplyTask(userID)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) EnqueueWakaTimeImport(ctx context.Context, userID uuid.UUID, heartbeats []HeartbeatImportPayload, defaults services.HeartbeatDefaults) error {
	task, err := NewWakaTimeImportTask(userID, heartbeats, defaults)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) EnqueueHeartbeatsPurge(ctx context.Context, retentionDays int) error {
	task, err := NewHeartbeatsPurgeTask(retentionDays, time.Time{})
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) EnqueueLeaderboardUpdate(ctx context.Context, rangeName string) error {
	task, err := NewLeaderboardUpdateTask(rangeName)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) EnqueueGoalsEvaluate(ctx context.Context, now time.Time) error {
	task, err := NewGoalsEvaluateTask(now)
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task, asynq.Queue("default"), asynq.MaxRetry(3))
	return err
}

func (c *AsynqClient) Close() error {
	return c.client.Close()
}
