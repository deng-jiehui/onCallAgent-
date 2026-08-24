package observability

import (
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	Requests           metric.Int64Counter
	RequestDuration    metric.Float64Histogram
	ComponentDuration  metric.Float64Histogram
	ComponentErrors    metric.Int64Counter
	ToolCalls          metric.Int64Counter
	RetrievalDocuments metric.Int64Histogram
	RetrievalDistance  metric.Float64Histogram
	RetrievalEmpty     metric.Int64Counter
	LLMTokens          metric.Int64Counter
	LLMFirstToken      metric.Float64Histogram
	LLMUsageMissing    metric.Int64Counter
	SSEInterrupts      metric.Int64Counter
}

var (
	metricsMu sync.RWMutex
	metrics   *Metrics
)

func Instruments() *Metrics {
	metricsMu.RLock()
	defer metricsMu.RUnlock()
	return metrics
}

func newMetrics() (*Metrics, error) {
	meter := otel.Meter("superbizagent")
	var result Metrics
	var err error
	if result.Requests, err = meter.Int64Counter("superbizagent_requests_total"); err != nil {
		return nil, err
	}
	if result.RequestDuration, err = meter.Float64Histogram("superbizagent_request_duration_seconds", metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if result.ComponentDuration, err = meter.Float64Histogram("superbizagent_component_duration_seconds", metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if result.ComponentErrors, err = meter.Int64Counter("superbizagent_component_errors_total"); err != nil {
		return nil, err
	}
	if result.ToolCalls, err = meter.Int64Counter("superbizagent_tool_calls_total"); err != nil {
		return nil, err
	}
	if result.RetrievalDocuments, err = meter.Int64Histogram("superbizagent_retrieval_documents"); err != nil {
		return nil, err
	}
	if result.RetrievalDistance, err = meter.Float64Histogram("superbizagent_retrieval_distance"); err != nil {
		return nil, err
	}
	if result.RetrievalEmpty, err = meter.Int64Counter("superbizagent_retrieval_empty_total"); err != nil {
		return nil, err
	}
	if result.LLMTokens, err = meter.Int64Counter("superbizagent_llm_tokens_total"); err != nil {
		return nil, err
	}
	if result.LLMFirstToken, err = meter.Float64Histogram("superbizagent_llm_first_token_seconds", metric.WithUnit("s")); err != nil {
		return nil, err
	}
	if result.LLMUsageMissing, err = meter.Int64Counter("superbizagent_llm_usage_missing_total"); err != nil {
		return nil, err
	}
	if result.SSEInterrupts, err = meter.Int64Counter("superbizagent_sse_interrupts_total"); err != nil {
		return nil, err
	}
	return &result, nil
}

func installMetrics(value *Metrics) {
	metricsMu.Lock()
	metrics = value
	metricsMu.Unlock()
}

func componentAttributes(component, componentType, errorType, _ string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("component", component),
		attribute.String("type", componentType),
		attribute.String("error_type", errorType),
	}
}
