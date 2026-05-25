package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

// initTelemetry bootstraps the OpenTelemetry tracer and meter providers.
// When telemetry is disabled in config, this is a no-op and the global
// providers remain noop. Called during Boot() after infrastructure is ready.
func (k *Kernel) initTelemetry() error {
	cfg := k.cfg.Telemetry
	if !cfg.Enabled {
		k.logger.Info("telemetry disabled")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the OTel resource describing this service instance.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
		),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return fmt.Errorf("telemetry: build resource: %w", err)
	}

	// Prepare gRPC dial options for the OTLP exporter.
	dialOpts := []grpc.DialOption{}
	if cfg.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// ── Trace exporter ──────────────────────────────────────────────────

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithDialOption(dialOpts...),
	)
	if err != nil {
		return fmt.Errorf("telemetry: trace exporter: %w", err)
	}

	// Always sample at 100%.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	k.tp = tp

	// ── Metric exporter ─────────────────────────────────────────────────

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.Endpoint),
		otlpmetricgrpc.WithDialOption(dialOpts...),
	)
	if err != nil {
		return fmt.Errorf("telemetry: metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(15*time.Second))),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	k.mp = mp

	// ── Propagation ─────────────────────────────────────────────────────

	// W3C Trace Context + Baggage propagation for distributed tracing
	// across service boundaries.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// ── Log correlation ─────────────────────────────────────────────────

	// Wrap the default slog handler so every log entry includes trace_id
	// and span_id when emitted inside an active span. This lets SigNoz
	// correlate logs with traces.
	slog.SetDefault(slog.New(&traceLogHandler{inner: slog.Default().Handler()}))

	k.logger.Info("telemetry enabled",
		"endpoint", cfg.Endpoint,
		"service", cfg.ServiceName,
		"insecure", cfg.Insecure,
	)
	return nil
}

// shutdownTelemetry flushes pending spans and metrics, then shuts down
// the OTel providers. Called during kernel Shutdown() before closing
// infrastructure connections so in-flight telemetry is not lost.
func (k *Kernel) shutdownTelemetry(ctx context.Context) {
	if k.tp != nil {
		if err := k.tp.Shutdown(ctx); err != nil {
			k.logger.Warn("error shutting down tracer provider", "error", err)
		}
	}
	if k.mp != nil {
		if err := k.mp.Shutdown(ctx); err != nil {
			k.logger.Warn("error shutting down meter provider", "error", err)
		}
	}
}

// ── slog trace correlation ──────────────────────────────────────────────────

// traceLogHandler wraps an slog.Handler to inject trace_id and span_id
// from the current OTel span context into every log record. This lets
// SigNoz correlate log entries with distributed traces.
type traceLogHandler struct {
	inner slog.Handler
}

func (h *traceLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *traceLogHandler) Handle(ctx context.Context, record slog.Record) error {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, record)
}

func (h *traceLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceLogHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *traceLogHandler) WithGroup(name string) slog.Handler {
	return &traceLogHandler{inner: h.inner.WithGroup(name)}
}

// ── GORM tracing plugin ─────────────────────────────────────────────────────
//
// Minimal GORM plugin using callbacks to create OTel spans for every database
// operation. This avoids the heavy gorm.io/plugin/opentelemetry dependency
// which pulls in ClickHouse and MySQL drivers as transitive deps.

const gormTracerName = "kernel/gorm"

// otelGormPlugin implements gorm.Plugin to add OTel tracing via GORM callbacks.
type otelGormPlugin struct{}

func newOtelGormPlugin() gorm.Plugin {
	return &otelGormPlugin{}
}

func (p *otelGormPlugin) Name() string {
	return "otel_tracing"
}

func (p *otelGormPlugin) Initialize(db *gorm.DB) error {
	tracer := otel.Tracer(gormTracerName)

	// Register before/after callback pairs for each operation type.
	db.Callback().Create().Before("gorm:create").Register("otel:create:before", gormBefore(tracer, "gorm.create"))
	db.Callback().Create().After("gorm:create").Register("otel:create:after", gormAfter)

	db.Callback().Query().Before("gorm:query").Register("otel:query:before", gormBefore(tracer, "gorm.query"))
	db.Callback().Query().After("gorm:query").Register("otel:query:after", gormAfter)

	db.Callback().Update().Before("gorm:update").Register("otel:update:before", gormBefore(tracer, "gorm.update"))
	db.Callback().Update().After("gorm:update").Register("otel:update:after", gormAfter)

	db.Callback().Delete().Before("gorm:delete").Register("otel:delete:before", gormBefore(tracer, "gorm.delete"))
	db.Callback().Delete().After("gorm:delete").Register("otel:delete:after", gormAfter)

	db.Callback().Row().Before("gorm:row").Register("otel:row:before", gormBefore(tracer, "gorm.row"))
	db.Callback().Row().After("gorm:row").Register("otel:row:after", gormAfter)

	db.Callback().Raw().Before("gorm:raw").Register("otel:raw:before", gormBefore(tracer, "gorm.raw"))
	db.Callback().Raw().After("gorm:raw").Register("otel:raw:after", gormAfter)

	return nil
}

const gormSpanKey = "otel:span"

// gormBefore starts a new span before a GORM operation.
func gormBefore(tracer trace.Tracer, spanName string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		ctx, span := tracer.Start(db.Statement.Context, spanName, trace.WithSpanKind(trace.SpanKindClient))
		db.Statement.Context = ctx
		db.Set(gormSpanKey, span)
	}
}

// gormAfter ends the span after a GORM operation, recording the SQL
// statement, table name, rows affected, and any errors as attributes.
func gormAfter(db *gorm.DB) {
	v, ok := db.Get(gormSpanKey)
	if !ok {
		return
	}
	span, ok := v.(trace.Span)
	if !ok {
		return
	}
	defer span.End()

	attrs := []attribute.KeyValue{
		semconv.DBSystemPostgreSQL,
	}
	if db.Statement.Table != "" {
		attrs = append(attrs, attribute.String("db.sql.table", db.Statement.Table))
	}
	if db.Statement.RowsAffected > 0 {
		attrs = append(attrs, attribute.Int64("db.rows_affected", db.Statement.RowsAffected))
	}

	// Record the full SQL statement so JOINs, subqueries, and complex
	// queries are visible in the trace.
	if sql := db.Statement.SQL.String(); sql != "" {
		attrs = append(attrs, attribute.String("db.query.text", sql))
	}

	span.SetAttributes(attrs...)

	// Mark the span as error if the operation failed.
	if db.Error != nil {
		span.RecordError(db.Error)
		span.SetStatus(codes.Error, db.Error.Error())
	}
}
