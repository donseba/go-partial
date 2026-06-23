package partial

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donseba/go-partial/connector"
)

func TestWriteWithRequestRendersSafeDefaultErrorPageOnTemplateError(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Data.Missing }}missing`)

	p := New("broken.gohtml").ID("broken").SetFileSystem(fsys)
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := p.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(body, "Template render error") {
		t.Fatalf("expected fallback error page, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml") {
		t.Fatalf("expected template name in fallback page, got %q", body)
	}
	if !strings.Contains(body, "Partial ID") {
		t.Fatalf("expected partial ID label in fallback page, got %q", body)
	}
	if !strings.Contains(body, "<dt>Template</dt>") {
		t.Fatalf("expected singular template label in fallback page, got %q", body)
	}
	if strings.Contains(body, "unexpected EOF") {
		t.Fatalf("expected safe mode to hide detailed error, got %q", body)
	}
	if strings.Contains(body, "stack trace:") {
		t.Fatalf("expected safe mode to hide stack trace, got %q", body)
	}
}

func TestWriteWithRequestRendersDetailedDefaultErrorPageOnTemplateError(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Data.Missing }}missing`)

	p := New("broken.gohtml").ID("broken").SetFileSystem(fsys).SetErrorMode(ErrorModeDetailed)
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := p.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(body, "unexpected EOF") {
		t.Fatalf("expected detailed error in fallback page, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml:1") {
		t.Fatalf("expected template error location in fallback page, got %q", body)
	}
	if strings.Contains(body, "stack trace:") {
		t.Fatalf("expected default detailed renderer to omit stack trace, got %q", body)
	}
}

func TestExtractTemplateErrorLocation(t *testing.T) {
	tests := map[string]string{
		"template: broken.gohtml:5: unexpected EOF":                                                "broken.gohtml:5",
		`template: broken.gohtml:2:6: executing "broken.gohtml" at <fail>: error calling fail: no`: "broken.gohtml:2:6",
		"plain error": "",
	}

	for input, want := range tests {
		if got := extractTemplateErrorLocation(errors.New(input)); got != want {
			t.Fatalf("extractTemplateErrorLocation(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWriteWithRequestUsesCustomErrorRenderer(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Data.Missing }}missing`)

	p := New("broken.gohtml").
		ID("broken").
		SetFileSystem(fsys).
		SetErrorRenderer(func(ctx context.Context, p *Partial, r *http.Request, err error) (template.HTML, error) {
			return template.HTML(`<div id="custom-error">` + p.id + `</div>`), nil
		})

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := p.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}
	if body := rec.Body.String(); body != `<div id="custom-error">broken</div>` {
		t.Fatalf("unexpected custom error body: %q", body)
	}
}

func TestWriteWithRequestRendersSwappableErrorFragmentForHTMX(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Data.Missing }}missing`)

	p := New("broken.gohtml").
		ID("content").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		SetErrorMode(ErrorModeDetailed)
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")
	rec := httptest.NewRecorder()

	err := p.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx-swappable 200, got %d", rec.Code)
	}
	if rec.Header().Get(HeaderGoPartialError) != "true" {
		t.Fatalf("expected X-Go-Partial-Error header")
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected fragment error output, got full document: %q", body)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected partial error fragment, got %q", body)
	}
}

func TestWriteWithRequestAppendsAncestorOOBToHTMXErrorFragment(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Data.Missing }}missing`)
	fsys.AddFile("header.gohtml", `<header id="app-header"{{ oobAttr }}>Header</header>`)

	wrapper := NewID("layout", "layout.gohtml").SetFileSystem(fsys)
	wrapper.WithOOB(NewID("header", "header.gohtml").SetFileSystem(fsys).SetAlwaysSwapOOB(true))
	content := NewID("content", "broken.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil))
	wrapper.With(content)

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")
	rec := httptest.NewRecorder()

	err := content.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx-swappable 200, got %d", rec.Code)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected partial error fragment, got %q", body)
	}
	if !strings.Contains(body, `id="app-header" hx-swap-oob="true"`) {
		t.Fatalf("expected OOB header in error response, got %q", body)
	}
}

func TestChildTemplateErrorRendersSectionFallbackInFullPage(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("layout.gohtml", `<html><body><header>Header</header><main>{{ child "content" }}</main><footer>Footer</footer></body></html>`)
	fsys.AddFile("broken.gohtml", `{{ if .Data.Missing }}missing`)

	wrapper := NewID("layout", "layout.gohtml").SetFileSystem(fsys)
	content := NewID("content", "broken.gohtml").SetFileSystem(fsys)
	wrapper.With(content)

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := wrapper.WriteWithRequest(context.Background(), rec, req)
	if err != nil {
		t.Fatalf("expected layout to survive child error, got %v", err)
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected surviving page status 200, got %d", rec.Code)
	}
	if !strings.Contains(body, "<header>Header</header>") || !strings.Contains(body, "<footer>Footer</footer>") {
		t.Fatalf("expected surrounding layout to render, got %q", body)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected child error fragment, got %q", body)
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected section fallback, got full error document: %q", body)
	}
}
