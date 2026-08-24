package observability

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, fmt.Errorf("observability sample ratio must be between 0 and 1: %v", cfg.SampleRatio)
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = "superbizagent"
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	var traceProvider *sdktrace.TracerProvider
	var meterProvider *sdkmetric.MeterProvider
	if cfg.Enabled {
		res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", cfg.ServiceName)))
		if err != nil {
			return nil, fmt.Errorf("create telemetry resource: %w", err)
		}
		traceOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint)}
		metricOptions := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(cfg.OTLPEndpoint)}
		if cfg.Insecure {
			traceOptions = append(traceOptions, otlptracegrpc.WithInsecure())
			metricOptions = append(metricOptions, otlpmetricgrpc.WithInsecure())
		}
		traceExporter, err := otlptracegrpc.New(ctx, traceOptions...)
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		metricExporter, err := otlpmetricgrpc.New(ctx, metricOptions...)
		if err != nil {
			return nil, fmt.Errorf("create OTLP metric exporter: %w", err)
		}
		traceProvider = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
			sdktrace.WithBatcher(traceExporter),
		)
		meterProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(res),
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(10*time.Second))),
		)
		otel.SetTracerProvider(traceProvider)
		otel.SetMeterProvider(meterProvider)
	}

	metricSet, err := newMetrics()
	if err != nil {
		return nil, fmt.Errorf("create telemetry metrics: %w", err)
	}
	installMetrics(metricSet)

	var once sync.Once
	var shutdownErr error
	shutdown := func(shutdownCtx context.Context) error {
		once.Do(func() {
			var errs []error
			if meterProvider != nil {
				errs = append(errs, meterProvider.Shutdown(shutdownCtx))
			}
			if traceProvider != nil {
				errs = append(errs, traceProvider.Shutdown(shutdownCtx))
			}
			shutdownErr = errors.Join(errs...)
		})
		return shutdownErr
	}
	return shutdown, nil
}
