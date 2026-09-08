package service

import (
	"context"
	"time"
)

const postCommitTimeout = 5 * time.Second

// runPostCommit keeps database follow-up work alive if the client disconnects.
// It preserves context values for tracing, but always applies its own deadline.
func runPostCommit(ctx context.Context, fn func(context.Context) error) error {
	postCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), postCommitTimeout)
	defer cancel()
	return fn(postCtx)
}
