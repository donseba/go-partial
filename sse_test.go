package partial

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSEWriterAppliesExpectedHeaders(t *testing.T) {
	rec := httptest.NewRecorder()

	NewSSEWriter(rec)

	if got := rec.Header().Get(SSEHeaderContentType.String()); got != SSEContentTypeEventStream.String() {
		t.Fatalf("expected content type %q, got %q", SSEContentTypeEventStream, got)
	}
	if got := rec.Header().Get(SSEHeaderCacheControl.String()); got != SSECacheControlNoCache.String() {
		t.Fatalf("expected cache control %q, got %q", SSECacheControlNoCache, got)
	}
	if got := rec.Header().Get(SSEHeaderConnection.String()); got != SSEConnectionKeepAlive.String() {
		t.Fatalf("expected connection %q, got %q", SSEConnectionKeepAlive, got)
	}
	if got := rec.Header().Get(SSEHeaderXAccelBuffering.String()); got != SSEXAccelBufferingNo.String() {
		t.Fatalf("expected accel buffering %q, got %q", SSEXAccelBufferingNo, got)
	}
}

func TestSSEWriterPatchHTML(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewSSEWriter(rec)

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

func TestSSEWriterSignal(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewSSEWriter(rec)

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

func TestSSEWriterPatchPartial(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("notice.gohtml", `<div>{{ .Data.Message }}</div>`)

	partial := NewID("notice", "notice.gohtml").
		SetFileSystem(fsys).
		SetData(map[string]any{"Message": "Saved"})

	rec := httptest.NewRecorder()
	writer := NewSSEWriter(rec)
	req := httptest.NewRequest(http.MethodGet, "/events", nil)

	if err := writer.PatchPartial(context.Background(), req, "#notice", partial); err != nil {
		t.Fatalf("PatchPartial() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `data: {"target":"#notice","html":"<div>Saved</div>"}`+"\n\n") {
		t.Fatalf("expected rendered partial patch, got %q", body)
	}
}

func TestSSEWriterMultilineData(t *testing.T) {
	rec := httptest.NewRecorder()
	writer := NewSSEWriter(rec)

	if err := writer.Event(SSEEventName("message"), "one\ntwo"); err != nil {
		t.Fatalf("Event() error = %v", err)
	}

	expected := "event: message\ndata: one\ndata: two\n\n"
	if rec.Body.String() != expected {
		t.Fatalf("expected %q, got %q", expected, rec.Body.String())
	}
}
