package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func TestRendererRecordsRenderMetrics(t *testing.T) {
	var records []Record
	now := tickingClock(
		time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
		25*time.Millisecond,
	)

	p := partial.New("page.gohtml").
		ID("metrics-page").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`Hello {{ .Name }}`)},
		}).
		SetDot(map[string]string{"Name": "Ada"}).
		Use(Stage(SinkFunc(func(record Record) {
			records = append(records, record)
		}), WithClock(now), WithTag("area", "showcase")))
	WithPartialLabel(p, "main panel")

	req := httptest.NewRequest("GET", "/metrics", nil)
	ctx := WithRequestID(context.Background(), "req-test")
	ctx = WithTraceID(ctx, "trace-test")
	ctx = WithParentRequestID(ctx, "parent-test")
	out, err := p.RenderWithRequest(ctx, req)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if out != "Hello Ada" {
		t.Fatalf("Render() = %q", out)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	record := records[0]
	if record.Kind != partial.RenderKindPartial {
		t.Fatalf("Kind = %q, want %q", record.Kind, partial.RenderKindPartial)
	}
	if record.PartialID != "metrics-page" {
		t.Fatalf("PartialID = %q", record.PartialID)
	}
	if record.ParentID != "" {
		t.Fatalf("ParentID = %q", record.ParentID)
	}
	if record.PartialLabel != "main panel" {
		t.Fatalf("PartialLabel = %q", record.PartialLabel)
	}
	if record.RequestID != "req-test" {
		t.Fatalf("RequestID = %q", record.RequestID)
	}
	if record.TraceID != "trace-test" {
		t.Fatalf("TraceID = %q", record.TraceID)
	}
	if record.ParentRequestID != "parent-test" {
		t.Fatalf("ParentRequestID = %q", record.ParentRequestID)
	}
	if len(record.Templates) != 1 || record.Templates[0] != "page.gohtml" {
		t.Fatalf("Templates = %#v", record.Templates)
	}
	if record.Method != "GET" || record.Path != "/metrics" {
		t.Fatalf("request = %s %s", record.Method, record.Path)
	}
	if record.Size != len("Hello Ada") {
		t.Fatalf("Size = %d", record.Size)
	}
	if !record.Rendered {
		t.Fatal("Rendered = false, want true")
	}
	if record.Duration != 25*time.Millisecond {
		t.Fatalf("Duration = %s", record.Duration)
	}
	if record.Tags["area"] != "showcase" {
		t.Fatalf("Tags = %#v", record.Tags)
	}
	if record.Error != nil {
		t.Fatalf("Error = %v", record.Error)
	}
}

func TestRendererUsesCorrelationHeaderFallbacks(t *testing.T) {
	var record Record
	p := partial.New("page.gohtml").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`ok`)},
		}).
		Use(Stage(SinkFunc(func(next Record) {
			record = next
		})))

	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set(HeaderRequestID, "req-header")
	req.Header.Set(HeaderTraceID, "trace-header")
	req.Header.Set(HeaderParentRequestID, "parent-header")
	if _, err := p.RenderWithRequest(context.Background(), req); err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if record.RequestID != "req-header" {
		t.Fatalf("RequestID = %q", record.RequestID)
	}
	if record.TraceID != "trace-header" {
		t.Fatalf("TraceID = %q", record.TraceID)
	}
	if record.ParentRequestID != "parent-header" {
		t.Fatalf("ParentRequestID = %q", record.ParentRequestID)
	}
}

func TestRendererMarksOOBPartials(t *testing.T) {
	var records []Record
	svc := partial.NewService(&partial.Config{
		Connector: connector.NewHTMX(nil),
		FS: fstest.MapFS{
			"layout.gohtml":  &fstest.MapFile{Data: []byte(`{{ content }}`)},
			"content.gohtml": &fstest.MapFile{Data: []byte(`<main>content</main>`)},
			"header.gohtml":  &fstest.MapFile{Data: []byte(`<aside{{ oobAttr }}>header</aside>`)},
		},
		Stages: []partial.RenderStage{
			Stage(SinkFunc(func(record Record) {
				records = append(records, record)
			})),
		},
	})

	content := WithPartialLabel(partial.NewID("content", "content.gohtml"), "main")
	wrapper := WithPartialLabel(partial.NewID("layout", "layout.gohtml"), "shell").
		WithOOB(WithPartialLabel(partial.NewID("header", "header.gohtml").SetAlwaysSwapOOB(true), "sidebar"))
	layout := svc.NewLayout().Set(content).Wrap(wrapper)

	req := httptest.NewRequest("GET", "/rows", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")
	recorder := httptest.NewRecorder()
	if err := layout.WriteWithRequest(WithRequestID(context.Background(), "req-oob"), recorder, req); err != nil {
		t.Fatalf("WriteWithRequest() error = %v", err)
	}

	seenContent := false
	seenHeader := false
	for _, record := range records {
		if record.RequestID != "req-oob" {
			t.Fatalf("RequestID = %q", record.RequestID)
		}
		switch record.PartialID {
		case "content":
			seenContent = true
			if record.ParentID != "layout" {
				t.Fatalf("content ParentID = %q", record.ParentID)
			}
			if record.PartialLabel != "main" {
				t.Fatalf("content PartialLabel = %q", record.PartialLabel)
			}
			if record.OOB {
				t.Fatal("content OOB = true, want false")
			}
		case "header":
			seenHeader = true
			if record.ParentID != "layout" {
				t.Fatalf("header ParentID = %q", record.ParentID)
			}
			if record.PartialLabel != "sidebar" {
				t.Fatalf("header PartialLabel = %q", record.PartialLabel)
			}
			if !record.OOB {
				t.Fatal("header OOB = false, want true")
			}
		}
	}
	if !seenContent || !seenHeader {
		t.Fatalf("records = %#v, want content and header", records)
	}
}

func TestRendererRecordsRenderErrors(t *testing.T) {
	renderErr := errors.New("boom")
	var records []Record
	RenderStage := Stage(SinkFunc(func(record Record) {
		records = append(records, record)
	}))
	ctx := &partial.RenderContext{
		Kind:   partial.RenderKindPartial,
		Values: make(partial.RenderValues),
	}

	ctx, err := RenderStage.Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	out, err := RenderStage.Render(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return template.HTML("partial output"), renderErr
	})
	_, err = RenderStage.Finalize(ctx, out, err)
	if !errors.Is(err, renderErr) {
		t.Fatalf("Finalize() error = %v, want %v", err, renderErr)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Size != len("partial output") {
		t.Fatalf("Size = %d", record.Size)
	}
	if !record.Rendered {
		t.Fatal("Rendered = false, want true")
	}
	if !errors.Is(record.Error, renderErr) {
		t.Fatalf("record.Error = %v, want %v", record.Error, renderErr)
	}
}

func TestWriterSinkWritesJSONLines(t *testing.T) {
	var out bytes.Buffer
	sink := NewWriterSink(&out)
	startedAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	sink.Record(Record{
		Kind:            partial.RenderKindPartial,
		RequestID:       "req-json",
		TraceID:         "trace-json",
		ParentRequestID: "parent-json",
		PartialID:       "content",
		PartialLabel:    "main",
		Method:          "GET",
		Path:            "/metrics",
		Size:            12,
		Rendered:        true,
		StartedAt:       startedAt,
		Duration:        3 * time.Millisecond,
		Error:           errors.New("boom"),
		EventKind:       "render.error",
		EventLevel:      partial.EventError,
		EventMessage:    "render failed",
		EventFields:     map[string]any{"template": "page.gohtml"},
		Tags:            map[string]string{"chain": "showcase"},
	})

	if err := sink.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1: %q", len(lines), out.String())
	}

	var got map[string]any
	if err := json.Unmarshal(lines[0], &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got["kind"] != "partial" || got["requestID"] != "req-json" || got["partialID"] != "content" {
		t.Fatalf("unexpected identity fields: %#v", got)
	}
	if got["traceID"] != "trace-json" || got["parentRequestID"] != "parent-json" || got["partialLabel"] != "main" {
		t.Fatalf("unexpected correlation fields: %#v", got)
	}
	if got["duration"] != "3ms" || got["durationNS"] != float64((3*time.Millisecond).Nanoseconds()) {
		t.Fatalf("unexpected duration fields: %#v", got)
	}
	if got["error"] != "boom" {
		t.Fatalf("error = %#v", got["error"])
	}
	if got["eventKind"] != "render.error" || got["eventLevel"] != "error" || got["eventMessage"] != "render failed" {
		t.Fatalf("unexpected event fields: %#v", got)
	}
}

func TestRendererRecordsConcurrentRenders(t *testing.T) {
	var mu sync.Mutex
	records := make(map[string]Record)
	p := partial.New("page.gohtml").
		ID("metrics-page").
		SetFileSystem(fstest.MapFS{
			"page.gohtml": &fstest.MapFile{Data: []byte(`{{ (request).URL.Query.Get "value" }}`)},
		}).
		Use(Stage(SinkFunc(func(record Record) {
			mu.Lock()
			defer mu.Unlock()
			records[record.RequestID] = record
		})))

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest("GET", "/metrics?value="+value, nil)
			out, err := p.RenderWithRequest(WithRequestID(req.Context(), "req-"+value), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != value {
				errs <- "render " + value + " got " + got
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for i := range renders {
		requestID := "req-" + strconv.Itoa(i)
		record, ok := records[requestID]
		if !ok {
			t.Fatalf("missing record %s", requestID)
		}
		if record.Path != "/metrics" || record.RequestID != requestID {
			t.Fatalf("record %s = %#v", requestID, record)
		}
	}
}

func TestWriterSinkRecordsConcurrently(t *testing.T) {
	var out bytes.Buffer
	sink := NewWriterSink(&out)

	const writes = 64
	var wg sync.WaitGroup
	for i := range writes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sink.Record(Record{
				Kind:      partial.RenderKindPartial,
				RequestID: "req-" + strconv.Itoa(i),
				PartialID: "content",
			})
		}(i)
	}
	wg.Wait()

	if err := sink.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != writes {
		t.Fatalf("lines = %d, want %d", len(lines), writes)
	}
}

func TestEventSinkRecordsDiagnosticEvents(t *testing.T) {
	var records []Record
	ctx := &partial.RenderContext{
		Context: context.Background(),
		Kind:    partial.RenderKindPartial,
		Request: httptest.NewRequest("GET", "/events", nil),
		Partial: partial.NewID("content", "content.gohtml").
			SetFileSystem(fstest.MapFS{"content.gohtml": &fstest.MapFile{Data: []byte("ok")}}),
	}
	ctx.Context = WithRequestID(ctx.Context, "req-event")
	ctx.Context = WithTraceID(ctx.Context, "trace-event")

	sink := EventSink(SinkFunc(func(record Record) {
		records = append(records, record)
	}), WithTag("chain", "events"))
	sink.Emit(ctx, partial.Event{
		Time:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		Kind:    partial.EventTemplateMissing,
		Level:   partial.EventWarn,
		Message: "partial template path not found",
		Fields:  map[string]any{"path": "missing.gohtml"},
	})

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != "event" || record.EventKind != partial.EventTemplateMissing {
		t.Fatalf("event identity = kind %q event %q", record.Kind, record.EventKind)
	}
	if record.RequestID != "req-event" || record.TraceID != "trace-event" {
		t.Fatalf("correlation = %q %q", record.RequestID, record.TraceID)
	}
	if record.PartialID != "content" || record.Method != "GET" || record.Path != "/events" {
		t.Fatalf("context fields = %#v", record)
	}
	if record.EventLevel != partial.EventWarn || record.EventMessage != "partial template path not found" {
		t.Fatalf("event fields = %#v", record)
	}
	if record.EventFields["path"] != "missing.gohtml" {
		t.Fatalf("EventFields = %#v", record.EventFields)
	}
	if record.Tags["chain"] != "events" {
		t.Fatalf("Tags = %#v", record.Tags)
	}
}

func TestWriterSinkStoresFirstError(t *testing.T) {
	sink := NewWriterSink(failingWriter{})

	sink.Record(Record{Kind: partial.RenderKindPartial})

	if !errors.Is(sink.Err(), errWriteFailed) {
		t.Fatalf("Err() = %v, want %v", sink.Err(), errWriteFailed)
	}
}

func TestFanoutSinkRecordsToEverySink(t *testing.T) {
	var first []Record
	var second []Record
	sink := Fanout(
		SinkFunc(func(record Record) {
			first = append(first, record)
		}),
		nil,
		SinkFunc(func(record Record) {
			second = append(second, record)
		}),
	)

	sink.Record(Record{PartialID: "content"})

	if len(first) != 1 || first[0].PartialID != "content" {
		t.Fatalf("first = %#v", first)
	}
	if len(second) != 1 || second[0].PartialID != "content" {
		t.Fatalf("second = %#v", second)
	}
}

func tickingClock(start time.Time, step time.Duration) func() time.Time {
	current := start.Add(-step)
	return func() time.Time {
		current = current.Add(step)
		return current
	}
}

var errWriteFailed = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

var _ io.Writer = failingWriter{}
