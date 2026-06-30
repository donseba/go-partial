package logger

import (
	"bytes"
	"context"
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
