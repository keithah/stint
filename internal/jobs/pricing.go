package jobs

import (
	"github.com/hibiken/asynq"
)

const TypePricingRefresh = "pricing:refresh"

// NewPricingRefreshTask builds the (payload-less) weekly task that re-fetches
// upstream price tables and caches them for the engine refresher.
func NewPricingRefreshTask() (*asynq.Task, error) {
	return asynq.NewTask(TypePricingRefresh, nil), nil
}
