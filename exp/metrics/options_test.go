package metrics

import (
	"bytes"
	"context"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	partial "github.com/donseba/go-partial"
)

func TestContextIDsIgnoreEmptyValues(t *testing.T) {
	base := context.Background()

	if WithRequestID(base, "") != base || WithTraceID(base, "") != base || WithParentRequestID(base, "") != base {
		t.Fatal("an empty ID made a new context")
	}

	//nolint:staticcheck // A nil context is what the helpers guard against.
	ctx := WithParentRequestID(WithTraceID(WithRequestID(nil, "req"), "trace"), "parent")

	if RequestIDFromContext(ctx) != "req" || TraceIDFromContext(ctx) != "trace" || ParentRequestIDFromContext(ctx) != "parent" {
		t.Fatalf("IDs = %q %q %q", RequestIDFromContext(ctx), TraceIDFromContext(ctx), ParentRequestIDFromContext(ctx))
	}

	//nolint:staticcheck // A nil context is what the helpers guard against.
	if WithTraceID(nil, "") == nil || WithParentRequestID(nil, "") == nil {
		t.Fatal("a nil context was returned")
	}

	//nolint:staticcheck // A nil context is what the helpers guard against.
	if RequestIDFromContext(nil) != "" || TraceIDFromContext(nil) != "" || ParentRequestIDFromContext(nil) != "" {
		t.Fatal("a nil context has IDs")
	}
}

func TestPartialLabelOfNilsAndEmptyLabels(t *testing.T) {
	if WithPartialLabel(nil, "x") != nil {
		t.Fatal("WithPartialLabel(nil) is not nil")
	}

	p := partial.NewID("content", "content.gohtml")
	if WithPartialLabel(p, "") != p || PartialLabel(p) != "" {
		t.Fatal("an empty label was set")
	}

	if PartialLabel(nil) != "" {
		t.Fatal("PartialLabel(nil) is not empty")
	}
}

// TestStageRecordsSlotNamesAndSkipsEmptyTags checks WithSlotName, that a tag
// without a key is left out, and that a nil sink records nothing.
func TestStageRecordsSlotNamesAndSkipsEmptyTags(t *testing.T) {
	var records []Record

	sink := SinkFunc(func(record Record) {
		records = append(records, record)
	})

	slotName := func(p *partial.Partial) string {
		return "slot-" + p.PartialID()
	}

	p := partial.NewID("card", "card.gohtml").
		SetFileSystem(fstest.MapFS{"card.gohtml": &fstest.MapFile{Data: []byte(`card`)}}).
		Use(Stage(sink, nil, WithTag("", "skipped"), WithClock(nil), WithSlotName(slotName)))

	if _, err := partial.Render(context.Background(), p); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	if records[0].SlotName != "slot-card" {
		t.Fatalf("SlotName = %q", records[0].SlotName)
	}

	if records[0].Tags != nil {
		t.Fatalf("Tags = %v, want none", records[0].Tags)
	}

	if records[0].StartedAt.IsZero() {
		t.Fatal("StartedAt is zero with the default clock")
	}

	silent := partial.NewID("card", "card.gohtml").
		SetFileSystem(fstest.MapFS{"card.gohtml": &fstest.MapFile{Data: []byte(`card`)}}).
		Use(Stage(nil))

	out, err := partial.Render(context.Background(), silent)
	if err != nil || out != "card" {
		t.Fatalf("Render() with a nil sink = %q, %v", out, err)
	}
}

// TestEventSinkFillsFieldsFromThePartial checks the record of an event
// that names its partial and template, of one whose fields come from the
// render's partial, and that a nil sink records nothing.
func TestEventSinkFillsFieldsFromThePartial(t *testing.T) {
	var records []Record

	clock := func() time.Time {
		return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	}

	slotName := func(p *partial.Partial) string {
		return "slot-" + p.PartialID()
	}

	sink := EventSink(SinkFunc(func(record Record) {
		records = append(records, record)
	}), nil, WithClock(clock), WithSlotName(slotName))

	parent := partial.NewID("page", "page.gohtml")
	child := partial.NewID("card", "card.gohtml").SetParent(parent)
	WithPartialLabel(child, "card label")

	req := httptest.NewRequest("POST", "/save", nil)
	req.Header.Set(HeaderParentRequestID, "parent-header")

	ctx := &partial.RenderContext{Context: context.Background(), Request: req, Partial: child}

	sink.Emit(ctx, partial.Event{
		Kind:      partial.EventTemplateMissing,
		PartialID: "named",
		ParentID:  "named-parent",
		Name:      "row.gohtml",
	})
	sink.Emit(ctx, partial.Event{Kind: partial.EventTemplateMissing})
	sink.Emit(nil, partial.Event{Kind: partial.EventTemplateMissing})

	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}

	named := records[0]
	if named.PartialID != "named" || named.ParentID != "named-parent" || named.Name != "row.gohtml" {
		t.Fatalf("named record = %+v", named)
	}

	if !named.StartedAt.Equal(clock()) {
		t.Fatalf("StartedAt = %v, want the clock's time", named.StartedAt)
	}

	if named.ParentRequestID != "parent-header" || named.Method != "POST" || named.Path != "/save" {
		t.Fatalf("request fields = %+v", named)
	}

	if named.SlotName != "slot-card" || named.PartialLabel != "card label" || len(named.Templates) != 1 {
		t.Fatalf("partial fields = %+v", named)
	}

	fromPartial := records[1]
	if fromPartial.PartialID != "card" || fromPartial.ParentID != "page" || fromPartial.Name != partial.EventTemplateMissing {
		t.Fatalf("record from the partial = %+v", fromPartial)
	}

	if bare := records[2]; bare.PartialID != "" || bare.Method != "" || bare.RequestID != "" {
		t.Fatalf("record without a render context = %+v", bare)
	}

	EventSink(nil).Emit(ctx, partial.Event{Kind: partial.EventTemplateMissing})
}

func TestBuildRecordWithoutARenderContext(t *testing.T) {
	cfg := config{
		now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
		tags: map[string]string{"area": "test"},
	}

	record := buildRecord(nil, "out", nil, cfg)

	if record.Size != 3 || record.Rendered || record.Duration != 0 || record.Tags["area"] != "test" || record.Kind != "" {
		t.Fatalf("record = %+v", record)
	}

	if requestPath(nil) != "" || requestID(nil) != "" || traceID(nil) != "" || parentRequestID(nil) != "" {
		t.Fatal("IDs of a nil render context are not empty")
	}

	if getState(nil) != nil || getState(&partial.RenderContext{}) != nil {
		t.Fatal("state of a render context without values")
	}

	if cloneEventFields(nil) != nil || cloneTags(nil) != nil {
		t.Fatal("clones of empty maps are not nil")
	}
}

// TestWriterSinkKeepsItsFirstError checks that a record that cannot be
// encoded is the sink's error, that nothing is written after it, and that
// nil sinks and writers are ignored.
func TestWriterSinkKeepsItsFirstError(t *testing.T) {
	var out bytes.Buffer

	sink := NewWriterSink(&out)
	sink.Record(Record{EventFields: map[string]any{"ch": make(chan int)}})

	if sink.Err() == nil {
		t.Fatal("Err() = nil after a record that cannot be encoded")
	}

	sink.Record(Record{PartialID: "content"})

	if out.Len() != 0 {
		t.Fatalf("wrote %q after the error", out.String())
	}

	var none *WriterSink
	none.Record(Record{})

	if none.Err() != nil {
		t.Fatal("nil sink has an error")
	}

	NewWriterSink(nil).Record(Record{})
}
