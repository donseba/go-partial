package otel

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

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

// TestSinkAddsTheEventIDs checks the attributes of an event's identifiers,
// and that events without a recording span, or without a context, are
// dropped.
func TestSinkAddsTheEventIDs(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	sink := Sink(nil, WithMinLevel(""))
	event := partial.Event{
		Kind:         partial.EventRenderFinish,
		Level:        partial.EventInfo,
		PartialID:    "content",
		ParentID:     "page",
		Name:         "card.gohtml",
		TraceID:      "trace-1",
		SpanID:       "span-1",
		ParentSpanID: "span-0",
	}

	sink.Emit(nil, event)
	sink.Emit(&partial.RenderContext{}, event)
	sink.Emit(&partial.RenderContext{Context: context.Background()}, event)

	ctx, span := provider.Tracer("test").Start(context.Background(), "render")
	sink.Emit(&partial.RenderContext{Context: ctx}, event)
	span.End()

	events := recorder.Ended()[0].Events()
	if len(events) != 1 {
		t.Fatalf("span events = %d, want 1", len(events))
	}

	if events[0].Name != partial.EventRenderFinish {
		t.Fatalf("event name = %q, want the kind", events[0].Name)
	}

	for key, value := range map[string]string{
		"partial.partial_id":     "content",
		"partial.parent_id":      "page",
		"partial.name":           "card.gohtml",
		"partial.trace_id":       "trace-1",
		"partial.span_id":        "span-1",
		"partial.parent_span_id": "span-0",
	} {
		if !hasAttr(events[0].Attributes, key, value) {
			t.Errorf("missing %s=%s: %#v", key, value, events[0].Attributes)
		}
	}
}

type named struct{}

func (named) String() string {
	return "named value"
}

func TestAttributeForValueKeepsTheType(t *testing.T) {
	tests := []struct {
		value any
		kind  attribute.Type
		want  string
	}{
		{value: "text", kind: attribute.STRING, want: "text"},
		{value: true, kind: attribute.BOOL, want: "true"},
		{value: 1, kind: attribute.INT64, want: "1"},
		{value: int8(-8), kind: attribute.INT64, want: "-8"},
		{value: int16(16), kind: attribute.INT64, want: "16"},
		{value: int32(32), kind: attribute.INT64, want: "32"},
		{value: int64(64), kind: attribute.INT64, want: "64"},
		{value: uint(1), kind: attribute.INT64, want: "1"},
		{value: uint8(8), kind: attribute.INT64, want: "8"},
		{value: uint16(16), kind: attribute.INT64, want: "16"},
		{value: uint32(32), kind: attribute.INT64, want: "32"},
		{value: uint64(64), kind: attribute.INT64, want: "64"},
		{value: uint64(math.MaxUint64), kind: attribute.STRING, want: "18446744073709551615"},
		{value: float32(1.5), kind: attribute.FLOAT64, want: "1.5"},
		{value: 2.25, kind: attribute.FLOAT64, want: "2.25"},
		{value: 3 * time.Millisecond, kind: attribute.INT64, want: "3000000"},
		{value: named{}, kind: attribute.STRING, want: "named value"},
		{value: []int{1, 2}, kind: attribute.STRING, want: "[1 2]"},
	}

	for _, tt := range tests {
		attr := attributeForValue("key", tt.value)
		if attr.Value.Type() != tt.kind || fmt.Sprint(attr.Value.AsInterface()) != tt.want {
			t.Errorf("attributeForValue(%#v) = %v %v, want %v %s", tt.value, attr.Value.Type(), attr.Value.AsInterface(), tt.kind, tt.want)
		}
	}
}

func TestLevelRankTreatsUnknownLevelsAsInfo(t *testing.T) {
	if levelRank("custom") != levelRank(partial.EventInfo) {
		t.Fatalf("levelRank(custom) = %d", levelRank("custom"))
	}

	if levelRank(partial.EventDebug) >= levelRank(partial.EventInfo) || levelRank(partial.EventError) <= levelRank(partial.EventWarn) {
		t.Fatal("levels are not ranked in order")
	}
}
