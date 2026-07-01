package otel

import (
	"context"
	"errors"
	"fmt"
	"testing"

	partial "github.com/donseba/go-partial"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSinkAddsEventToActiveSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	ctx, span := provider.Tracer("test").Start(context.Background(), "render")
	sink := Sink(WithMinLevel(partial.EventDebug))
	sink.Emit(&partial.RenderContext{Context: ctx}, partial.Event{
		Kind:      partial.EventRenderFinish,
		Level:     partial.EventDebug,
		Message:   "render finished",
		PartialID: "content",
		Fields:    map[string]any{"size": 42},
	})
	span.End()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	events := spans[0].Events()
	if len(events) != 1 {
		t.Fatalf("span events = %d, want 1", len(events))
	}
	if events[0].Name != "render finished" {
		t.Fatalf("event name = %q, want render finished", events[0].Name)
	}
	if !hasAttr(events[0].Attributes, "partial.event", partial.EventRenderFinish) {
		t.Fatalf("missing partial.event attribute: %#v", events[0].Attributes)
	}
	if !hasAttr(events[0].Attributes, "partial.size", "42") {
		t.Fatalf("missing partial.size attribute: %#v", events[0].Attributes)
	}
}

func TestSinkFiltersByLevelAndRecordsErrors(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	ctx, span := provider.Tracer("test").Start(context.Background(), "render")
	sink := Sink(WithMinLevel(partial.EventWarn))
	sink.Emit(&partial.RenderContext{Context: ctx}, partial.Event{
		Kind:  partial.EventRenderStart,
		Level: partial.EventDebug,
	})
	sink.Emit(&partial.RenderContext{Context: ctx}, partial.Event{
		Kind:  partial.EventRenderError,
		Level: partial.EventError,
		Error: errors.New("boom"),
	})
	span.End()

	spans := recorder.Ended()
	if len(spans[0].Events()) != 2 {
		t.Fatalf("span events = %d, want 2", len(spans[0].Events()))
	}
	if !hasNamedEvent(spans[0].Events(), partial.EventRenderError) {
		t.Fatalf("missing render error event: %#v", spans[0].Events())
	}
}

func TestSinkSupportsAttributePrefixAndTypedFields(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	ctx, span := provider.Tracer("test").Start(context.Background(), "render")
	Sink(WithAttributePrefix("gp.")).Emit(&partial.RenderContext{Context: ctx}, partial.Event{
		Kind:  partial.EventRenderFinish,
		Level: partial.EventInfo,
		Fields: map[string]any{
			"cached": true,
			"size":   42,
		},
	})
	span.End()

	attrs := recorder.Ended()[0].Events()[0].Attributes
	if !hasAttr(attrs, "gp.cached", "true") || !hasAttr(attrs, "gp.size", "42") {
		t.Fatalf("missing typed attributes: %#v", attrs)
	}
}

func hasAttr(attrs []attribute.KeyValue, key, value string) bool {
	for _, attr := range attrs {
		if string(attr.Key) == key && fmt.Sprint(attr.Value.AsInterface()) == value {
			return true
		}
	}
	return false
}

func hasNamedEvent(events []sdktrace.Event, name string) bool {
	for _, event := range events {
		if event.Name == name {
			return true
		}
	}
	return false
}
