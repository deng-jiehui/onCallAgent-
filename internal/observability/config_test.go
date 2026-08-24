package observability

import (
	"context"
	"testing"
)

func TestLoadConfigUsesSafeDefaults(t *testing.T) {
	t.Setenv("SUPERBIZAGENT_OTEL_ENABLED", "")
	t.Setenv("SUPERBIZAGENT_OTEL_ENDPOINT", "")
	t.Setenv("SUPERBIZAGENT_OTEL_SAMPLE_RATIO", "")

	cfg := LoadConfig(context.Background())
	if !cfg.Enabled || cfg.ServiceName != "superbizagent" || cfg.OTLPEndpoint != "localhost:4317" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
	if cfg.SampleRatio != 1 || !cfg.Insecure || cfg.DebugContent {
		t.Fatalf("unexpected safety defaults: %#v", cfg)
	}
}

func TestLoadConfigReadsEnvironment(t *testing.T) {
	t.Setenv("SUPERBIZAGENT_OTEL_ENABLED", "false")
	t.Setenv("SUPERBIZAGENT_OTEL_ENDPOINT", "collector:4317")
	t.Setenv("SUPERBIZAGENT_OTEL_SAMPLE_RATIO", "0.25")
	t.Setenv("SUPERBIZAGENT_OTEL_DEBUG_CONTENT", "true")

	cfg := LoadConfig(context.Background())
	if cfg.Enabled || cfg.OTLPEndpoint != "collector:4317" || cfg.SampleRatio != 0.25 || !cfg.DebugContent {
		t.Fatalf("environment was not applied: %#v", cfg)
	}
}

func TestRequestInfoRoundTrip(t *testing.T) {
	want := RequestInfo{RequestID: "req-1", ConversationID: "conv-1", TenantID: "tenant-1", UserID: "user-1"}
	got := RequestInfoFromContext(WithRequestInfo(context.Background(), want))
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
