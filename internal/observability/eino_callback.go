package observability

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type callbackStartKey struct{}

func EinoHandler(metricsSet *Metrics) callbacks.Handler {
	builder := callbacks.NewHandlerBuilder()
	builder.OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		component, name := runInfoValues(info)
		ctx, span := otel.Tracer("superbizagent/eino").Start(ctx, "eino."+strings.ToLower(component)+"."+name)
		span.SetAttributes(
			attribute.String("eino.component", component),
			attribute.String("eino.type", runInfoType(info)),
			attribute.String("eino.name", name),
		)
		setInputSummary(span, info, input)
		return context.WithValue(ctx, callbackStartKey{}, time.Now())
	})
	builder.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		finishEinoSpan(ctx, info, output, nil, metricsSet)
		return ctx
	})
	builder.OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
		finishEinoSpan(ctx, info, nil, err, metricsSet)
		return ctx
	})
	builder.OnEndWithStreamOutputFn(func(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
		copies := output.Copy(2)
		*output = *copies[0]
		observer := copies[1]
		go observeStream(ctx, info, observer, metricsSet)
		return ctx
	})
	return builder.Build()
}

func observeStream(ctx context.Context, info *callbacks.RunInfo, reader *schema.StreamReader[callbacks.CallbackOutput], metricsSet *Metrics) {
	defer reader.Close()
	var last callbacks.CallbackOutput
	var firstTokenDuration time.Duration
	for {
		chunk, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			recordFirstToken(ctx, info, last, firstTokenDuration, metricsSet)
			finishEinoSpan(ctx, info, last, nil, metricsSet)
			return
		}
		if err != nil {
			finishEinoSpan(ctx, info, last, err, metricsSet)
			return
		}
		if firstTokenDuration == 0 {
			firstTokenDuration = time.Since(callbackStartedAt(ctx))
		}
		last = chunk
	}
}

func recordFirstToken(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput, duration time.Duration, metricsSet *Metrics) {
	if duration == 0 || info == nil || info.Component != components.ComponentOfChatModel {
		return
	}
	modelName := "unknown"
	if converted := model.ConvCallbackOutput(output); converted != nil && converted.Config != nil && converted.Config.Model != "" {
		modelName = converted.Config.Model
	}
	seconds := duration.Seconds()
	trace.SpanFromContext(ctx).SetAttributes(attribute.Float64("llm.first_token_seconds", seconds))
	if metricsSet != nil {
		metricsSet.LLMFirstToken.Record(ctx, seconds, metric.WithAttributes(attribute.String("model", modelName)))
	}
}

func finishEinoSpan(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput, runErr error, metricsSet *Metrics) {
	span := trace.SpanFromContext(ctx)
	component, name := runInfoValues(info)
	componentType := runInfoType(info)
	duration := time.Since(callbackStartedAt(ctx)).Seconds()
	if metricsSet != nil {
		metricsSet.ComponentDuration.Record(ctx, duration, metric.WithAttributes(
			attribute.String("component", component), attribute.String("type", componentType),
		))
	}

	if runErr != nil {
		errorType := classifyError(runErr)
		span.RecordError(runErr)
		span.SetStatus(codes.Error, errorType)
		span.SetAttributes(attribute.String("error.type", errorType))
		if metricsSet != nil {
			metricsSet.ComponentErrors.Add(ctx, 1, metric.WithAttributes(
				attribute.String("component", component), attribute.String("type", componentType), attribute.String("error_type", errorType),
			))
			if info != nil && info.Component == components.ComponentOfTool {
				metricsSet.ToolCalls.Add(ctx, 1, metric.WithAttributes(attribute.String("tool", name), attribute.String("status", "error")))
			}
		}
		span.End()
		return
	}

	setOutputSummary(ctx, span, info, output, metricsSet)
	if metricsSet != nil && info != nil && info.Component == components.ComponentOfTool {
		metricsSet.ToolCalls.Add(ctx, 1, metric.WithAttributes(attribute.String("tool", name), attribute.String("status", "ok")))
	}
	span.End()
}

func setInputSummary(span trace.Span, info *callbacks.RunInfo, input callbacks.CallbackInput) {
	if info == nil {
		return
	}
	switch info.Component {
	case components.ComponentOfTool:
		if converted := tool.ConvCallbackInput(input); converted != nil {
			span.SetAttributes(
				attribute.Int("tool.arguments.length", len(converted.ArgumentsInJSON)),
				attribute.String("tool.arguments.sha256", hashText(converted.ArgumentsInJSON)),
			)
		}
	case components.ComponentOfRetriever:
		if converted := retriever.ConvCallbackInput(input); converted != nil {
			span.SetAttributes(attribute.Int("retrieval.query.length", len(converted.Query)))
		}
	}
}

func setOutputSummary(ctx context.Context, span trace.Span, info *callbacks.RunInfo, output callbacks.CallbackOutput, metricsSet *Metrics) {
	if info == nil {
		return
	}
	switch info.Component {
	case components.ComponentOfChatModel:
		converted := model.ConvCallbackOutput(output)
		if converted == nil {
			return
		}
		modelName := "unknown"
		if converted.Config != nil && converted.Config.Model != "" {
			modelName = converted.Config.Model
		}
		span.SetAttributes(attribute.String("llm.model", modelName))
		if converted.TokenUsage == nil {
			span.SetAttributes(attribute.Bool("llm.usage_missing", true))
			if metricsSet != nil {
				metricsSet.LLMUsageMissing.Add(ctx, 1, metric.WithAttributes(attribute.String("model", modelName)))
			}
			return
		}
		usage := converted.TokenUsage
		span.SetAttributes(
			attribute.Int("llm.token.prompt", usage.PromptTokens),
			attribute.Int("llm.token.completion", usage.CompletionTokens),
			attribute.Int("llm.token.total", usage.TotalTokens),
		)
		if metricsSet != nil {
			for kind, value := range map[string]int{"prompt": usage.PromptTokens, "completion": usage.CompletionTokens, "total": usage.TotalTokens} {
				metricsSet.LLMTokens.Add(ctx, int64(value), metric.WithAttributes(attribute.String("model", modelName), attribute.String("kind", kind)))
			}
		}
	case components.ComponentOfRetriever:
		converted := retriever.ConvCallbackOutput(output)
		if converted == nil {
			return
		}
		setRetrievalSummary(ctx, span, converted.Docs, metricsSet)
	}
}

func setRetrievalSummary(ctx context.Context, span trace.Span, docs []*schema.Document, metricsSet *Metrics) {
	span.SetAttributes(attribute.Int("retrieval.document_count", len(docs)))
	collection := "unknown"
	distances := make([]float64, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		if value, ok := doc.MetaData["_retrieval_distance"].(float64); ok {
			distances = append(distances, value)
		}
		if value, ok := doc.MetaData["_retrieval_collection"].(string); ok && value != "" {
			collection = value
		}
	}
	if metricsSet != nil {
		metricsSet.RetrievalDocuments.Record(ctx, int64(len(docs)), metric.WithAttributes(attribute.String("collection", collection)))
		if len(docs) == 0 {
			metricsSet.RetrievalEmpty.Add(ctx, 1, metric.WithAttributes(attribute.String("collection", collection)))
		}
		for _, distance := range distances {
			metricsSet.RetrievalDistance.Record(ctx, distance, metric.WithAttributes(attribute.String("collection", collection)))
		}
	}
	if len(distances) == 0 {
		span.SetAttributes(attribute.Bool("retrieval.distance_missing", len(docs) > 0))
		return
	}
	sort.Float64s(distances)
	total := 0.0
	for _, value := range distances {
		total += value
	}
	span.SetAttributes(
		attribute.Float64("retrieval.distance.min", distances[0]),
		attribute.Float64("retrieval.distance.max", distances[len(distances)-1]),
		attribute.Float64("retrieval.distance.avg", total/float64(len(distances))),
		attribute.Float64("retrieval.distance.p50", percentile(distances, 0.50)),
		attribute.Float64("retrieval.distance.p95", percentile(distances, 0.95)),
	)
}

func percentile(sorted []float64, quantile float64) float64 {
	index := int(float64(len(sorted)-1)*quantile + 0.5)
	return sorted[index]
}

func runInfoValues(info *callbacks.RunInfo) (string, string) {
	if info == nil {
		return "unknown", "unknown"
	}
	component := string(info.Component)
	if component == "" {
		component = "unknown"
	}
	name := info.Name
	if name == "" {
		name = info.Type
	}
	if name == "" {
		name = "unknown"
	}
	return component, name
}

func runInfoType(info *callbacks.RunInfo) string {
	if info == nil || info.Type == "" {
		return "unknown"
	}
	return info.Type
}

func callbackStartedAt(ctx context.Context) time.Time {
	started, ok := ctx.Value(callbackStartKey{}).(time.Time)
	if !ok {
		return time.Now()
	}
	return started
}

func classifyError(err error) string {
	if err == nil {
		return "none"
	}
	typeOf := reflect.TypeOf(err)
	if typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	return typeOf.PkgPath() + "." + typeOf.Name()
}

func hashText(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}
