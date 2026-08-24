package observability

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestEinoHandlerRecordsModelTokens(t *testing.T) {
	recorder := installSpanRecorder(t)
	handler := EinoHandler(nil)
	info := &callbacks.RunInfo{Name: "fake", Type: "FakeModel", Component: components.ComponentOfChatModel}
	ctx := handler.OnStart(context.Background(), info, &model.CallbackInput{})
	handler.OnEnd(ctx, info, &model.CallbackOutput{
		Config:     &model.Config{Model: "fake-model"},
		TokenUsage: &model.TokenUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	})

	span := onlySpan(t, recorder)
	if span.Name() != "eino.chatmodel.fake" {
		t.Fatalf("unexpected span name: %q", span.Name())
	}
	assertAttribute(t, span.Attributes(), "llm.model", "fake-model")
	assertAttribute(t, span.Attributes(), "llm.token.total", int64(14))
}

func TestEinoHandlerSummarizesRetrievalDistances(t *testing.T) {
	recorder := installSpanRecorder(t)
	handler := EinoHandler(nil)
	info := &callbacks.RunInfo{Name: "milvus", Type: "Milvus", Component: components.ComponentOfRetriever}
	ctx := handler.OnStart(context.Background(), info, "secret query")
	handler.OnEnd(ctx, info, &retriever.CallbackOutput{Docs: []*schema.Document{
		{ID: "a", MetaData: map[string]any{"_retrieval_distance": float64(8)}},
		{ID: "b", MetaData: map[string]any{"_retrieval_distance": float64(17)}},
	}})

	span := onlySpan(t, recorder)
	assertAttribute(t, span.Attributes(), "retrieval.document_count", int64(2))
	assertAttribute(t, span.Attributes(), "retrieval.distance.min", float64(8))
	assertAttribute(t, span.Attributes(), "retrieval.distance.max", float64(17))
	for _, attr := range span.Attributes() {
		if strings.Contains(attr.Value.Emit(), "secret query") {
			t.Fatalf("query leaked into span: %s", attr.Value.Emit())
		}
	}
}

func TestEinoHandlerDoesNotExportToolSecrets(t *testing.T) {
	recorder := installSpanRecorder(t)
	handler := EinoHandler(nil)
	info := &callbacks.RunInfo{Name: "mysql", Type: "Tool", Component: components.ComponentOfTool}
	input := `{"api_key":"secret-key","password":"secret-password","dsn":"root:secret@tcp(db)"}`
	ctx := handler.OnStart(context.Background(), info, &tool.CallbackInput{ArgumentsInJSON: input})
	handler.OnEnd(ctx, info, &tool.CallbackOutput{Response: "ok"})

	span := onlySpan(t, recorder)
	for _, attr := range span.Attributes() {
		value := attr.Value.Emit()
		if strings.Contains(value, "secret") || strings.Contains(value, "root:") {
			t.Fatalf("secret leaked into span attribute %s=%s", attr.Key, value)
		}
	}
}

func TestEinoHandlerRecordsErrors(t *testing.T) {
	recorder := installSpanRecorder(t)
	handler := EinoHandler(nil)
	info := &callbacks.RunInfo{Name: "tool", Type: "Tool", Component: components.ComponentOfTool}
	ctx := handler.OnStart(context.Background(), info, "{}")
	handler.OnError(ctx, info, errors.New("failed"))

	span := onlySpan(t, recorder)
	if span.Status().Code.String() != "Error" {
		t.Fatalf("span status is not Error: %v", span.Status())
	}
}

func installSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})
	return recorder
}

func onlySpan(t *testing.T, recorder *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("want one ended span, got %d", len(spans))
	}
	return spans[0]
}

func assertAttribute(t *testing.T, attrs []attribute.KeyValue, key string, want any) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			if attr.Value.AsInterface() != want {
				t.Fatalf("attribute %s=%#v, want %#v", key, attr.Value.AsInterface(), want)
			}
			return
		}
	}
	t.Fatalf("attribute %s not found", key)
}
