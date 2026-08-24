package observability

import (
	"SuperBizAgent/internal/auth"
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

func StartRequest(ctx context.Context, route, mode, conversationID string) (context.Context, func(error)) {
	info := RequestInfoFromContext(ctx)
	if principal, ok := auth.PrincipalFromContext(ctx); ok {
		if info.TenantID == "" {
			info.TenantID = principal.TenantID
		}
		if info.UserID == "" {
			info.UserID = principal.UserID
		}
	}
	if info.RequestID == "" {
		info.RequestID = uuid.NewString()
	}
	if info.ConversationID == "" {
		info.ConversationID = conversationID
	}
	ctx = WithRequestInfo(ctx, info)
	ctx, span := otel.Tracer("superbizagent/http").Start(ctx, "HTTP "+route)
	span.SetAttributes(
		attribute.String("http.route", route),
		attribute.String("request.mode", mode),
		attribute.String("request.id", info.RequestID),
		attribute.String("conversation.id", info.ConversationID),
	)
	started := time.Now()
	var once sync.Once
	return ctx, func(runErr error) {
		once.Do(func() {
			status := "ok"
			if runErr != nil {
				status = "error"
				typeOf := reflect.TypeOf(runErr)
				span.RecordError(runErr)
				span.SetStatus(codes.Error, typeOf.String())
			}
			if metricsSet := Instruments(); metricsSet != nil {
				attrs := metric.WithAttributes(
					attribute.String("route", route),
					attribute.String("mode", mode),
					attribute.String("status", status),
				)
				metricsSet.Requests.Add(ctx, 1, attrs)
				metricsSet.RequestDuration.Record(ctx, time.Since(started).Seconds(), metric.WithAttributes(
					attribute.String("route", route), attribute.String("mode", mode),
				))
			}
			span.End()
		})
	}
}
