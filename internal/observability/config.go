package observability

import (
	"context"
	"os"
	"strconv"

	"github.com/gogf/gf/v2/frame/g"
)

type Config struct {
	Enabled      bool
	ServiceName  string
	OTLPEndpoint string
	Insecure     bool
	SampleRatio  float64
	DebugContent bool
}

func LoadConfig(ctx context.Context) Config {
	cfg := Config{
		Enabled:      true,
		ServiceName:  "superbizagent",
		OTLPEndpoint: "localhost:4317",
		Insecure:     true,
		SampleRatio:  1,
	}
	if value, err := g.Cfg().Get(ctx, "observability.enabled"); err == nil && !value.IsNil() {
		cfg.Enabled = value.Bool()
	}
	if value, err := g.Cfg().Get(ctx, "observability.service_name"); err == nil && value.String() != "" {
		cfg.ServiceName = value.String()
	}
	if value, err := g.Cfg().Get(ctx, "observability.otlp_endpoint"); err == nil && value.String() != "" {
		cfg.OTLPEndpoint = value.String()
	}
	if value, err := g.Cfg().Get(ctx, "observability.insecure"); err == nil && !value.IsNil() {
		cfg.Insecure = value.Bool()
	}
	if value, err := g.Cfg().Get(ctx, "observability.sample_ratio"); err == nil && !value.IsNil() {
		cfg.SampleRatio = value.Float64()
	}
	if value, err := g.Cfg().Get(ctx, "observability.debug_content"); err == nil && !value.IsNil() {
		cfg.DebugContent = value.Bool()
	}
	if value := os.Getenv("SUPERBIZAGENT_OTEL_ENABLED"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Enabled = parsed
		}
	}
	if value := os.Getenv("SUPERBIZAGENT_OTEL_SERVICE_NAME"); value != "" {
		cfg.ServiceName = value
	}
	if value := os.Getenv("SUPERBIZAGENT_OTEL_ENDPOINT"); value != "" {
		cfg.OTLPEndpoint = value
	}
	if value := os.Getenv("SUPERBIZAGENT_OTEL_INSECURE"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.Insecure = parsed
		}
	}
	if value := os.Getenv("SUPERBIZAGENT_OTEL_SAMPLE_RATIO"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.SampleRatio = parsed
		}
	}
	if value := os.Getenv("SUPERBIZAGENT_OTEL_DEBUG_CONTENT"); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			cfg.DebugContent = parsed
		}
	}
	return cfg
}
