// Package telemetry provides the application's observability client backed by OpenTelemetry.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	appLogger "hourglass-rejections-rpa/src/integrations/logger"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Client wraps the application's OpenTelemetry providers and structured logger.
type Client struct {
	enabled        bool
	logger         *slog.Logger
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	logProvider    *sdklog.LoggerProvider
	errorCounter   metric.Int64Counter
	messageCounter metric.Int64Counter
}

// Config holds OpenTelemetry configuration.
type Config struct {
	Endpoint       string
	Headers        string
	Environment    string
	ServiceName    string
	Release        string
	MetricInterval time.Duration
	Level          string
}

// New initializes OpenTelemetry providers and returns an observability client.
func New(cfg Config) (*Client, error) {
	fallback := newFallbackLogger(cfg.Level)
	if cfg.Endpoint == "" {
		return &Client{enabled: false, logger: fallback}, nil
	}

	ctx := context.Background()
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithProcess(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Release),
			attribute.String("deployment.environment.name", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build otel resource: %w", err)
	}

	headers := parseHeaders(cfg.Headers)

	traceExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(cfg.Endpoint),
		otlptracehttp.WithHeaders(headers),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize trace exporter: %w", err)
	}

	metricExporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(cfg.Endpoint),
		otlpmetrichttp.WithHeaders(headers),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metric exporter: %w", err)
	}

	logExporter, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(cfg.Endpoint),
		otlploghttp.WithHeaders(headers),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize log exporter: %w", err)
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(cfg.MetricInterval))),
	)
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	logglobal.SetLoggerProvider(logProvider)

	meter := meterProvider.Meter("hourglass-rejections-rpa/observability")
	errorCounter, err := meter.Int64Counter("hourglass.errors.total")
	if err != nil {
		return nil, fmt.Errorf("failed to create error counter: %w", err)
	}
	messageCounter, err := meter.Int64Counter("hourglass.messages.total")
	if err != nil {
		return nil, fmt.Errorf("failed to create message counter: %w", err)
	}

	otelHandler := otelslog.NewHandler(cfg.ServiceName,
		otelslog.WithLoggerProvider(logProvider),
		otelslog.WithVersion(cfg.Release),
		otelslog.WithSource(true),
	)
	logger := slog.New(newMultiHandler(otelHandler, fallback.Handler()))

	return &Client{
		enabled:        true,
		logger:         logger,
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		logProvider:    logProvider,
		errorCounter:   errorCounter,
		messageCounter: messageCounter,
	}, nil
}

// Logger returns the configured structured logger.
func (c *Client) Logger() *slog.Logger {
	if c == nil || c.logger == nil {
		return slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	return c.logger
}

// CaptureError records an error through OpenTelemetry logs and metrics.
func (c *Client) CaptureError(err error, extras map[string]interface{}) {
	if c == nil || err == nil {
		return
	}

	attrs := make([]slog.Attr, 0, len(extras)+1)
	attrs = append(attrs, slog.String("error.message", err.Error()))
	for key, value := range extras {
		attrs = append(attrs, slog.Any(key, value))
	}

	if c.logger != nil {
		c.logger.LogAttrs(context.Background(), slog.LevelError, "captured error", attrs...)
	}
	if c.enabled {
		c.errorCounter.Add(context.Background(), 1)
	}
}

// CaptureMessage records a message through OpenTelemetry logs and metrics.
func (c *Client) CaptureMessage(message string, level string) {
	if c == nil || message == "" {
		return
	}

	if c.logger != nil {
		c.logger.LogAttrs(context.Background(), parseLevel(level), message)
	}
	if c.enabled {
		c.messageCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("level", level)))
	}
}

// Flush forces providers to export buffered telemetry.
func (c *Client) Flush(timeout time.Duration) {
	if c == nil || !c.enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_ = c.tracerProvider.ForceFlush(ctx)
	_ = c.meterProvider.ForceFlush(ctx)
	_ = c.logProvider.ForceFlush(ctx)
}

// Close shuts down all OpenTelemetry providers.
func (c *Client) Close() {
	if c == nil || !c.enabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = c.logProvider.Shutdown(ctx)
	_ = c.meterProvider.Shutdown(ctx)
	_ = c.tracerProvider.Shutdown(ctx)
}

// IsEnabled returns true if OpenTelemetry export is enabled.
func (c *Client) IsEnabled() bool {
	return c != nil && c.enabled
}

func parseHeaders(raw string) map[string]string {
	headers := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return headers
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newFallbackLogger(level string) *slog.Logger {
	logCfg := appLogger.ForTerminal()
	logCfg.Level = level
	return appLogger.New(logCfg)
}

type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	filtered := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			filtered = append(filtered, handler)
		}
	}

	return &multiHandler{handlers: filtered}
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, record.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithAttrs(attrs))
	}
	return &multiHandler{handlers: next}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithGroup(name))
	}
	return &multiHandler{handlers: next}
}
