package logger

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestSinkWritesEventsToSlog(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	Sink(log, WithMinLevel(partial.EventDebug)).Emit(nil, partial.Event{
		Kind:      "render.error",
		Level:     partial.EventError,
		Message:   "render failed",
		PartialID: "content",
		Fields:    map[string]any{"template": "page.gohtml"},
	})

	got := out.String()
	for _, want := range []string{`msg="render failed"`, `kind=render.error`, `partial=content`, `template=page.gohtml`} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q: %s", want, got)
		}
	}
}

func TestSinkFiltersBelowMinimumLevel(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	Sink(log, WithMinLevel(partial.EventWarn)).Emit(nil, partial.Event{
		Kind:  "render.start",
		Level: partial.EventDebug,
	})

	if out.Len() != 0 {
		t.Fatalf("expected no log output, got %s", out.String())
	}
}

func TestLoggerTemplateHelperEmitsEvent(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`before{{ logger "from template" "section" "hero" }}after`)},
	}
	var got partial.Event
	ctx := partial.WithEventSink(context.Background(), partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
		if event.Kind == EventTemplateLog {
			got = event
		}
	}))
	page := partial.NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetFunc(FuncMap()).
		Use(Stage())

	html, err := partial.Render(ctx, page)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if html != template.HTML("beforeafter") {
		t.Fatalf("html = %q, want beforeafter", html)
	}
	if got.Kind != EventTemplateLog {
		t.Fatalf("event kind = %q, want %q", got.Kind, EventTemplateLog)
	}
	if got.Message != "from template" {
		t.Fatalf("message = %q, want from template", got.Message)
	}
	if got.Fields["section"] != "hero" || got.Fields["source"] != "template" {
		t.Fatalf("fields = %#v", got.Fields)
	}
}

func TestSinkHandlesConcurrentEvents(t *testing.T) {
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sink := Sink(log, WithMinLevel(partial.EventDebug))

	const emits = 64
	var wg sync.WaitGroup
	for i := range emits {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sink.Emit(nil, partial.Event{
				Kind:      "render.finish",
				Level:     partial.EventInfo,
				Message:   "rendered " + strconv.Itoa(i),
				PartialID: "content",
			})
		}(i)
	}
	wg.Wait()

	if got := strings.Count(out.String(), "render.finish"); got != emits {
		t.Fatalf("logged events = %d, want %d", got, emits)
	}
}

func TestLoggerTemplateHelperEmitsConcurrentEvents(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`{{ logger "from template" "value" (url).RawQuery }}`)},
	}
	var count sync.Map
	page := partial.NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetFunc(FuncMap()).
		Use(Stage())

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest("GET", "/?value="+value, nil)
			ctx := partial.WithEventSink(req.Context(), partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
				if event.Kind == EventTemplateLog {
					count.Store(event.Fields["value"], true)
				}
			}))
			if _, err := partial.RenderWithRequest(ctx, req, page); err != nil {
				errs <- err.Error()
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for i := range renders {
		value := strconv.Itoa(i)
		if _, ok := count.Load("value=" + value); !ok {
			t.Fatalf("missing event value %s", value)
		}
	}
}

type logKey struct{}

// recordingHandler keeps the records it handles, with the context each
// was logged with.
type recordingHandler struct {
	records  []slog.Record
	contexts []context.Context
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *recordingHandler) Handle(ctx context.Context, record slog.Record) error {
	h.records = append(h.records, record)
	h.contexts = append(h.contexts, ctx)

	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *recordingHandler) WithGroup(string) slog.Handler {
	return h
}

// TestSinkWritesTheEventIDsAndContext checks the attributes of an event's
// identifiers and error, that an event without a message is logged with its
// kind, and that the render's context is the log context.
func TestSinkWritesTheEventIDsAndContext(t *testing.T) {
	handler := &recordingHandler{}
	sink := Sink(slog.New(handler), nil, WithMinLevel(""), WithMinLevel(partial.EventDebug))

	ctx := context.WithValue(context.Background(), logKey{}, "render")
	sink.Emit(&partial.RenderContext{Context: ctx}, partial.Event{
		Kind:     "render.error",
		Level:    "custom",
		ParentID: "page",
		Name:     "card.gohtml",
		TraceID:  "trace-1",
		Error:    errors.New("boom"),
	})

	if len(handler.records) != 1 {
		t.Fatalf("records = %d, want 1", len(handler.records))
	}

	record := handler.records[0]
	if record.Message != "render.error" || record.Level != slog.LevelInfo {
		t.Fatalf("record = %q at %v, want the kind at info", record.Message, record.Level)
	}

	if handler.contexts[0].Value(logKey{}) != "render" {
		t.Fatal("the event was not logged with the render's context")
	}

	attrs := map[string]string{}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.String()
		return true
	})

	for key, want := range map[string]string{"parent": "page", "name": "card.gohtml", "trace": "trace-1", "error": "boom"} {
		if attrs[key] != want {
			t.Errorf("attr %s = %q, want %q", key, attrs[key], want)
		}
	}
}

func TestSlogLevels(t *testing.T) {
	levels := map[partial.EventLevel]slog.Level{
		partial.EventDebug: slog.LevelDebug,
		partial.EventInfo:  slog.LevelInfo,
		partial.EventWarn:  slog.LevelWarn,
		partial.EventError: slog.LevelError,
		"custom":           slog.LevelInfo,
	}

	for level, want := range levels {
		if got := slogLevel(level); got != want {
			t.Errorf("slogLevel(%q) = %v, want %v", level, got, want)
		}
	}
}

// TestLoggerFieldsOfOddPairs checks the fields of pairs whose key is not a
// string, or that lack a value, and the message of an empty one.
func TestLoggerFieldsOfOddPairs(t *testing.T) {
	fields := templateLogFields(1, "one", "", "empty", "last")

	want := map[string]any{"source": "template", "arg0": "one", "arg2": "empty", "last": ""}
	if len(fields) != len(want) {
		t.Fatalf("fields = %#v, want %#v", fields, want)
	}

	for key, value := range want {
		if fields[key] != value {
			t.Errorf("field %s = %#v, want %#v", key, fields[key], value)
		}
	}

	if got := Logger(nil, "ignored"); got != "" {
		t.Fatalf("Logger(nil) = %q", got)
	}

	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`{{ logger "" }}`)},
	}

	var message string

	ctx := partial.WithEventSink(context.Background(), partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
		if event.Kind == EventTemplateLog {
			message = event.Message
		}
	}))

	page := partial.NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetFunc(FuncMap()).
		Use(Stage())

	if _, err := partial.Render(ctx, page); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if message != "template log" {
		t.Fatalf("message = %q, want the default", message)
	}
}
