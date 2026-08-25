package conversation

import (
	"context"
	"errors"
)

var ErrRunLimiterClosed = errors.New("conversation run limiter is closed")

// RunLimiter bounds the number of turns executing in one process. It is not a
// replacement for distributed tenant quotas.
type RunLimiter struct {
	tokens chan struct{}
	done   chan struct{}
}

func NewRunLimiter(maxRuns int) *RunLimiter {
	if maxRuns <= 0 {
		maxRuns = 100
	}
	return &RunLimiter{tokens: make(chan struct{}, maxRuns), done: make(chan struct{})}
}

func (l *RunLimiter) Acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-l.done:
		return ErrRunLimiterClosed
	default:
	}
	select {
	case <-l.done:
		return ErrRunLimiterClosed
	case <-ctx.Done():
		return ctx.Err()
	case l.tokens <- struct{}{}:
		return nil
	}
}

func (l *RunLimiter) Release() {
	if l == nil {
		return
	}
	select {
	case <-l.tokens:
	default:
	}
}

func (l *RunLimiter) Close() {
	if l == nil {
		return
	}
	select {
	case <-l.done:
	default:
		close(l.done)
	}
}
