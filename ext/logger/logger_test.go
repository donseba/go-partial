package logger

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"strings"
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
	page := partial.NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetFunc(FuncMap()).
		SetEventSink(partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
			if event.Kind == EventTemplateLog {
				got = event
			}
		})).
		Use(Renderer())

	html, err := page.Render(context.Background())
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
