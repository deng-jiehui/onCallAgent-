package observability

import (
	"context"
	"testing"
)

func TestInitDisabledIsSafeAndIdempotent(t *testing.T) {
	shutdown, err := Init(context.Background(), Config{ServiceName: "test", SampleRatio: 1})
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown returned error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown returned error: %v", err)
	}
}

func TestInitRejectsInvalidSamplingRatio(t *testing.T) {
	if _, err := Init(context.Background(), Config{Enabled: false, SampleRatio: 2}); err == nil {
		t.Fatal("expected invalid sample ratio error")
	}
}
