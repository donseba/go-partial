package partial

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/donseba/go-partial/connector"
)

func TestWriteWithRequestAppliesFluentConnectorResponse(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("notice.gohtml", `<div id="notice">Saved</div>`)

	p := NewID("notice", "notice.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil))
	p.Response().
		Retarget("#notice").
		TriggerWith(connector.NewTrigger().AddEvent("saved"))

	req := httptest.NewRequest(http.MethodGet, "/notice", nil)
	rec := httptest.NewRecorder()

	if err := p.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	if got := rec.Header().Get(connector.HTMXHeaderRetarget.String()); got != "#notice" {
		t.Fatalf("expected HX-Retarget header, got %q", got)
	}
	if got := rec.Header().Get(connector.HTMXHeaderTrigger.String()); got != `{"saved":null}` {
		t.Fatalf("expected HX-Trigger header, got %q", got)
	}
}

func TestWriteWithRequestAppliesStructConnectorResponse(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("notice.gohtml", `<div id="notice">Saved</div>`)

	p := NewID("notice", "notice.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		SetResponse(connector.Response{
			Retarget: "#notice",
			Trigger:  connector.NewTrigger().AddEvent("saved").String(),
		})

	req := httptest.NewRequest(http.MethodGet, "/notice", nil)
	rec := httptest.NewRecorder()

	if err := p.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	if got := rec.Header().Get(connector.HTMXHeaderRetarget.String()); got != "#notice" {
		t.Fatalf("expected HX-Retarget header, got %q", got)
	}
	if got := rec.Header().Get(connector.HTMXHeaderTrigger.String()); got != `{"saved":null}` {
		t.Fatalf("expected HX-Trigger header, got %q", got)
	}
}
