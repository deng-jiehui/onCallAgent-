package conversation

import (
	"context"
	"testing"
	"time"
)

func TestRunLimiterBoundsAndCancelsAcquire(t *testing.T) {
	limiter := NewRunLimiter(1)
	if err := limiter.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := limiter.Acquire(ctx); err == nil {
		t.Fatal("second acquire should wait and time out")
	}
	limiter.Release()
	if err := limiter.Acquire(context.Background()); err != nil {
		t.Fatalf("released slot was not reusable: %v", err)
	}
	limiter.Close()
	limiter.Release()
	if err := limiter.Acquire(context.Background()); err != ErrRunLimiterClosed {
		t.Fatalf("acquire after close = %v", err)
	}
}
