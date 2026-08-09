package telemetry

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	defaultNewResource       = newResource
	defaultNewTraceExporter  = newTraceExporter
	defaultNewMetricExporter = newMetricExporter
	defaultNewLogExporter    = newLogExporter
	defaultNewErrorCounter   = newErrorCounter
	defaultNewMessageCounter = newMessageCounter
)

func resetTelemetryFactories(t *testing.T) {
	t.Helper()
	originalResource := newResource
	originalTrace := newTraceExporter
	originalMetric := newMetricExporter
	originalLog := newLogExporter
	originalErrorCounter := newErrorCounter
	originalMessageCounter := newMessageCounter
	t.Cleanup(func() {
		newResource = originalResource
		newTraceExporter = originalTrace
		newMetricExporter = originalMetric
		newLogExporter = originalLog
		newErrorCounter = originalErrorCounter
		newMessageCounter = originalMessageCounter
	})

	newResource = defaultNewResource
	newTraceExporter = defaultNewTraceExporter
	newMetricExporter = defaultNewMetricExporter
	newLogExporter = defaultNewLogExporter
	newErrorCounter = defaultNewErrorCounter
	newMessageCounter = defaultNewMessageCounter
}

func TestNew_Disabled(t *testing.T) {
	client, err := New(Config{Level: "info"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if client == nil {
		t.Fatal("New() returned nil")
	}
	if client.IsEnabled() {
		t.Fatal("expected telemetry client to be disabled")
	}
	if client.Logger() == nil {
		t.Fatal("expected fallback logger")
	}
}

func TestClient_DisabledMethods(t *testing.T) {
	client, err := New(Config{Level: "debug"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	client.CaptureError(errors.New("test error"), map[string]any{"key": "value"})
	client.CaptureMessage("test message", "info")
	client.Flush(time.Millisecond)
	client.Close()

	var nilClient *Client
	require.NotNil(t, nilClient.Logger())
	nilClient.CaptureError(errors.New("ignored"), nil)
	client.CaptureError(nil, nil)
	nilClient.CaptureMessage("ignored", "info")
	client.CaptureMessage("", "info")
	nilClient.Flush(time.Millisecond)
	nilClient.Close()
}

func TestNew_EnabledClient(t *testing.T) {
	resetTelemetryFactories(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(Config{
		Endpoint:       server.URL,
		Headers:        "Authorization=test",
		Environment:    "test",
		ServiceName:    "hourglass-test",
		Release:        "test-release",
		MetricInterval: time.Hour,
		Level:          "debug",
	})
	require.NoError(t, err)
	require.True(t, client.IsEnabled())
	require.NotNil(t, client.Logger())

	client.CaptureError(errors.New("captured"), map[string]any{"attempt": 1})
	client.CaptureMessage("message", "warning")
	client.Flush(time.Second)
	client.Close()
}

func TestNew_InitializationErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	config := Config{
		Endpoint:       server.URL,
		ServiceName:    "hourglass-test",
		MetricInterval: time.Hour,
	}

	t.Run("resource", func(t *testing.T) {
		resetTelemetryFactories(t)
		newResource = func(context.Context, ...resource.Option) (*resource.Resource, error) {
			return nil, errors.New("resource failed")
		}
		_, err := New(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build otel resource")
	})

	t.Run("trace exporter", func(t *testing.T) {
		resetTelemetryFactories(t)
		newTraceExporter = func(context.Context, ...otlptracehttp.Option) (*otlptrace.Exporter, error) {
			return nil, errors.New("trace failed")
		}
		_, err := New(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize trace exporter")
	})

	t.Run("metric exporter", func(t *testing.T) {
		resetTelemetryFactories(t)
		newMetricExporter = func(context.Context, ...otlpmetrichttp.Option) (*otlpmetrichttp.Exporter, error) {
			return nil, errors.New("metric failed")
		}
		_, err := New(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize metric exporter")
	})

	t.Run("log exporter", func(t *testing.T) {
		resetTelemetryFactories(t)
		newLogExporter = func(context.Context, ...otlploghttp.Option) (*otlploghttp.Exporter, error) {
			return nil, errors.New("log failed")
		}
		_, err := New(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize log exporter")
	})

	t.Run("error counter", func(t *testing.T) {
		resetTelemetryFactories(t)
		newErrorCounter = func(metric.Meter) (metric.Int64Counter, error) {
			return nil, errors.New("counter failed")
		}
		_, err := New(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create error counter")
	})

	t.Run("message counter", func(t *testing.T) {
		resetTelemetryFactories(t)
		newMessageCounter = func(metric.Meter) (metric.Int64Counter, error) {
			return nil, errors.New("counter failed")
		}
		_, err := New(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create message counter")
	})
}

func TestParseHeaders(t *testing.T) {
	headers := parseHeaders(" , Authorization=Bearer token, stream-name=default, invalid")
	if headers["Authorization"] != "Bearer token" {
		t.Fatalf("unexpected Authorization header: %q", headers["Authorization"])
	}
	if headers["stream-name"] != "default" {
		t.Fatalf("unexpected stream-name header: %q", headers["stream-name"])
	}
	if _, ok := headers["invalid"]; ok {
		t.Fatal("invalid header entry should be ignored")
	}
}

type handlerProbe struct {
	enabled   bool
	handleErr error
	handled   int
	attrs     int
	groups    int
}

func (h *handlerProbe) Enabled(context.Context, slog.Level) bool {
	return h.enabled
}

func (h *handlerProbe) Handle(context.Context, slog.Record) error {
	h.handled++
	return h.handleErr
}

func (h *handlerProbe) WithAttrs([]slog.Attr) slog.Handler {
	h.attrs++
	return h
}

func (h *handlerProbe) WithGroup(string) slog.Handler {
	h.groups++
	return h
}

func TestMultiHandler(t *testing.T) {
	disabled := &handlerProbe{}
	enabled := &handlerProbe{enabled: true}
	handler := newMultiHandler(nil, disabled, enabled)
	require.True(t, handler.Enabled(t.Context(), slog.LevelInfo))

	falseHandler := newMultiHandler(disabled)
	assert.False(t, falseHandler.Enabled(t.Context(), slog.LevelInfo))

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
	require.NoError(t, handler.Handle(t.Context(), record))
	assert.Equal(t, 1, disabled.handled)
	assert.Equal(t, 1, enabled.handled)

	withAttrs := handler.WithAttrs([]slog.Attr{slog.String("key", "value")})
	withGroup := handler.WithGroup("group")
	require.NotNil(t, withAttrs)
	require.NotNil(t, withGroup)
	assert.Equal(t, 1, disabled.attrs)
	assert.Equal(t, 1, enabled.attrs)
	assert.Equal(t, 1, disabled.groups)
	assert.Equal(t, 1, enabled.groups)

	failing := &handlerProbe{enabled: true, handleErr: errors.New("handle failed")}
	err := newMultiHandler(failing, enabled).Handle(t.Context(), record)
	require.Error(t, err)
	assert.Equal(t, 1, failing.handled)
}

func TestParseLevel(t *testing.T) {
	tests := map[string]int{
		"debug":  -4,
		"info":   0,
		"warn":   4,
		"error":  8,
		"random": 0,
	}

	for input, want := range tests {
		if got := int(parseLevel(input)); got != want {
			t.Fatalf("parseLevel(%q) = %d, want %d", input, got, want)
		}
	}
}
