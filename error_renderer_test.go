package partial

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donseba/go-partial/connector"
)

func TestWriteWithRequestRendersSafeDefaultErrorPageOnTemplateError(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").ID("broken").SetFileSystem(fsys).Use(testErrorRenderer(false))
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
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").ID("broken").SetFileSystem(fsys).Use(testErrorRenderer(true))
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

func TestWriteWithRequestUsesCustomErrorRenderer(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").
		ID("broken").
		SetFileSystem(fsys).
		Use(RendererHooks{
			InFlightFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
				if ctx.Kind != renderKindError {
					return next(ctx)
				}
				return template.HTML(`<div id="custom-error">` + ctx.Partial.PartialID() + `</div>`), nil
			},
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
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").
		ID("content").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		Use(testErrorRenderer(true))
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
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected fragment error output, got full document: %q", body)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected partial error fragment, got %q", body)
	}
}

func TestWriteWithRequestAppendsAncestorOOBToHTMXErrorFragment(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)
	fsys.AddFile("header.gohtml", `<header id="app-header"{{ oobAttr }}>Header</header>`)

	wrapper := NewID("layout", "layout.gohtml").SetFileSystem(fsys)
	wrapper.WithOOB(NewID("header", "header.gohtml").SetFileSystem(fsys).SetAlwaysSwapOOB(true))
	content := NewID("content", "broken.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		Use(testErrorRenderer(false))
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

func TestRegisteredTargetTemplateErrorRendersSectionFallback(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("layout.gohtml", `<html><body><header>Header</header><main>{{ content }}</main><footer>Footer</footer></body></html>`)
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	wrapper := NewID("layout", "layout.gohtml").SetFileSystem(fsys)
	content := NewID("content", "broken.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		Use(testErrorRenderer(false))
	wrapper.With(content)

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HeaderTarget.String(), "content")
	rec := httptest.NewRecorder()

	err := content.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected swappable fragment status 200, got %d", rec.Code)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected target error fragment, got %q", body)
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected section fallback, got full error document: %q", body)
	}
}

func testErrorRenderer(detailed bool) Renderer {
	return RendererHooks{
		InFlightFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
			if ctx.Kind != renderKindError {
				return next(ctx)
			}

			templates := ctx.Partial.TemplatePaths()
			label := "Templates"
			if len(templates) == 1 {
				label = "Template"
			}

			body := `Template render error Partial ID ` + ctx.Partial.PartialID() + ` <dt>` + label + `</dt> ` + strings.Join(templates, ", ")
			if detailed && ctx.Error != nil {
				body += " " + ctx.Error.Error()
			}
			if ctx.Name == "fragment" {
				body = `<section class="go-partial-error">` + body + `</section>`
			}

			return template.HTML(body), nil
		},
	}
}
