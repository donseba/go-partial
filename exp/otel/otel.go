package otel

import (
	"fmt"
	"math"
	"time"

	partial "github.com/donseba/go-partial"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type (
	config struct {
		minLevel partial.EventLevel
		prefix   string
	}

	// Option configures the OpenTelemetry event sink.
	Option func(*config)
)

// WithMinLevel ignores events below level.
func WithMinLevel(level partial.EventLevel) Option {
	return func(cfg *config) {
		if level != "" {
			cfg.minLevel = level
		}
	}
}

// WithAttributePrefix changes the attribute key prefix used by Sink. Pass an
// empty string when attributes should not be prefixed.
func WithAttributePrefix(prefix string) Option {
	return func(cfg *config) {
		cfg.prefix = prefix
	}
}

// Sink returns a diagnostic event consumer that adds go-partial events to the active span.
//
// The sink does not create spans, tracer providers, exporters, or samplers.
// Applications should create spans in their own HTTP middleware and render with
// that request context.
func Sink(options ...Option) partial.EventSink {
	cfg := config{
		minLevel: partial.EventInfo,
		prefix:   "partial.",
	}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	return partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
		if levelRank(event.Level) < levelRank(cfg.minLevel) {
			return
		}
		if ctx == nil || ctx.Context == nil {
			return
		}
		span := trace.SpanFromContext(ctx.Context)
		if !span.IsRecording() {
			return
		}

		attrs := []attribute.KeyValue{
			attribute.String(cfg.prefix+"event", event.Kind),
			attribute.String(cfg.prefix+"level", string(event.Level)),
		}
		if event.PartialID != "" {
			attrs = append(attrs, attribute.String(cfg.prefix+"partial_id", event.PartialID))
		}
		if event.ParentID != "" {
			attrs = append(attrs, attribute.String(cfg.prefix+"parent_id", event.ParentID))
		}
		if event.Name != "" {
			attrs = append(attrs, attribute.String(cfg.prefix+"name", event.Name))
		}
		if event.TraceID != "" {
			attrs = append(attrs, attribute.String(cfg.prefix+"trace_id", event.TraceID))
		}
		if event.SpanID != "" {
			attrs = append(attrs, attribute.String(cfg.prefix+"span_id", event.SpanID))
		}
		if event.ParentSpanID != "" {
			attrs = append(attrs, attribute.String(cfg.prefix+"parent_span_id", event.ParentSpanID))
		}
		for key, value := range event.Fields {
			attrs = append(attrs, attributeForValue(cfg.prefix+key, value))
		}
		if event.Error != nil {
			span.RecordError(event.Error)
			attrs = append(attrs, attribute.String(cfg.prefix+"error", event.Error.Error()))
		}

		name := event.Kind
		if event.Message != "" {
			name = event.Message
		}
		span.AddEvent(name, trace.WithAttributes(attrs...))
	})
}

func attributeForValue(key string, value any) attribute.KeyValue {
	switch v := value.(type) {
	case string:
		return attribute.String(key, v)
	case bool:
		return attribute.Bool(key, v)
	case int:
		return attribute.Int(key, v)
	case int8:
		return attribute.Int64(key, int64(v))
	case int16:
		return attribute.Int64(key, int64(v))
	case int32:
		return attribute.Int64(key, int64(v))
	case int64:
		return attribute.Int64(key, v)
	case uint:
		return attribute.Int64(key, int64(v))
	case uint8:
		return attribute.Int64(key, int64(v))
	case uint16:
		return attribute.Int64(key, int64(v))
	case uint32:
		return attribute.Int64(key, int64(v))
	case uint64:
		if v <= math.MaxInt64 {
			return attribute.Int64(key, int64(v))
		}
		return attribute.String(key, fmt.Sprint(v))
	case float32:
		return attribute.Float64(key, float64(v))
	case float64:
		return attribute.Float64(key, v)
	case time.Duration:
		return attribute.Int64(key, int64(v))
	case fmt.Stringer:
		return attribute.String(key, v.String())
	default:
		return attribute.String(key, fmt.Sprint(v))
	}
}

func levelRank(level partial.EventLevel) int {
	switch level {
	case partial.EventDebug:
		return 0
	case partial.EventInfo:
		return 1
	case partial.EventWarn:
		return 2
	case partial.EventError:
		return 3
	default:
		return 1
	}
}
