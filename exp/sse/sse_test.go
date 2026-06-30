package sse

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestWriterAppliesExpectedHeaders(t *testing.T) {
	rec := httptest.NewRecorder()

	NewWriter(rec)

	if got := rec.Header().Get(HeaderContentType.String()); got != ContentTypeEventStream.String() {
		t.Fatalf("expected content type %q, got %q", ContentTypeEventStream, got)
	}
	if got := rec.Header().Get(HeaderCacheControl.String()); got != CacheControlNoCache.String() {
		t.Fatalf("expected cache control %q, got %q", CacheControlNoCache, got)
	}
	if got := rec.Header().Get(HeaderConnection.String()); got != ConnectionKeepAlive.String() {
		t.Fatalf("expected connection %q, got %q", ConnectionKeepAlive, got)
	}
	if got := rec.Header().Get(HeaderXAccelBuffering.String()); got != XAccelBufferingNo.String() {
		t.Fatalf("expected accel buffering %q, got %q", XAccelBufferingNo, got)
	}
}

func TestWriterPatchHTML(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewWriter(rec)

	if err := writer.PatchHTML("#notice", template.HTML(`<div>Saved</div>`)); err != nil {
		t.Fatalf("PatchHTML() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: partial:patch\n") {
		t.Fatalf("expected patch event, got %q", body)
	}
	if !strings.Contains(body, `data: {"target":"#notice","html":"<div>Saved</div>"}`+"\n\n") {
		t.Fatalf("expected patch payload, got %q", body)
	}
}

func TestWriterSignal(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewWriter(rec)

	if err := writer.Signal("saved", true); err != nil {
		t.Fatalf("Signal() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: partial:signal\n") {
		t.Fatalf("expected signal event, got %q", body)
	}
	if !strings.Contains(body, `data: {"name":"saved","value":true}`+"\n\n") {
		t.Fatalf("expected signal payload, got %q", body)
	}
}

func TestWriterPatchPartial(t *testing.T) {
	fsys := fstest.MapFS{
		"notice.gohtml": &fstest.MapFile{Data: []byte(`<div>{{ .Message }}</div>`)},
	}

	notice := partial.NewID("notice", "notice.gohtml").
		SetFileSystem(fsys).
		SetDot(map[string]any{"Message": "Saved"})

	rec := httptest.NewRecorder()
	writer := NewWriter(rec)
	req := httptest.NewRequest(http.MethodGet, "/events", nil)

	if err := writer.PatchPartial(context.Background(), req, "#notice", notice); err != nil {
		t.Fatalf("PatchPartial() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data: {"target":"#notice","html":"<div>Saved</div>"}`+"\n\n") {
		t.Fatalf("expected rendered partial patch, got %q", body)
	}
}

func TestWriterPatchPartialUsesStages(t *testing.T) {
	fsys := fstest.MapFS{
		"notice.gohtml": &fstest.MapFile{Data: []byte(`<div>{{ marker }}</div>`)},
	}

	notice := partial.NewID("notice", "notice.gohtml").
		SetFileSystem(fsys)

	rec := httptest.NewRecorder()
	writer := NewWriter(rec).Use(partial.RenderStageHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			ctx.SetFunc("marker", func() string { return "rendered" })
			return ctx, nil
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/events", nil)

	if err := writer.PatchPartial(context.Background(), req, "#notice", notice); err != nil {
		t.Fatalf("PatchPartial() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data: {"target":"#notice","html":"<div>rendered</div>"}`+"\n\n") {
		t.Fatalf("expected RenderStage-enhanced partial patch, got %q", body)
	}
}

func TestWriterMultilineData(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewWriter(rec)

	if err := writer.Event(EventName("message"), "one\ntwo"); err != nil {
		t.Fatalf("Event() error = %v", err)
	}

	expected := "event: message\ndata: one\ndata: two\n\n"
	if rec.Body.String() != expected {
		t.Fatalf("expected %q, got %q", expected, rec.Body.String())
	}
}
