package errors

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestExtractTemplateLocation(t *testing.T) {
	tests := map[string]string{
		"template: broken.gohtml:5: unexpected EOF":                                                "broken.gohtml:5",
		`template: broken.gohtml:2:6: executing "broken.gohtml" at <fail>: error calling fail: no`: "broken.gohtml:2:6",
		"plain error": "",
	}

	for input, want := range tests {
		if got := ExtractTemplateLocation(errors.New(input)); got != want {
			t.Fatalf("ExtractTemplateLocation(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRendererRendersDetailedFragment(t *testing.T) {
	p := partial.New("broken.gohtml").ID("broken")
	req := httptest.NewRequest("GET", "/broken", nil)
	ctx := &partial.RenderContext{
		Context: req.Context(),
		Request: req,
		URL:     req.URL,
		Partial: p,
		Kind:    RenderKindError,
		Name:    "fragment",
		Error:   errors.New("template: broken.gohtml:1: unexpected EOF"),
	}

	out, err := Renderer(WithMode(ModeDetailed)).Render(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected fragment error class, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml:1") {
		t.Fatalf("expected detailed location, got %q", body)
	}
}

func TestRendererUsesAllLifecyclePhases(t *testing.T) {
	p := partial.New("broken.gohtml").ID("broken")
	req := httptest.NewRequest("GET", "/broken", nil)
	ctx := &partial.RenderContext{
		Context: req.Context(),
		Request: req,
		URL:     req.URL,
		Partial: p,
		Kind:    RenderKindError,
		Name:    "fragment",
		Error:   errors.New("template: broken.gohtml:1: unexpected EOF"),
		Values:  make(partial.RenderValues),
	}
	renderer := Renderer(WithMode(ModeDetailed))

	prepared, err := renderer.Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	data, ok := prepared.Data.(Data)
	if !ok {
		t.Fatalf("Prepare() data = %T, want Data", prepared.Data)
	}
	if !data.Detailed {
		t.Fatal("Prepare() data.Detailed = false, want true")
	}

	out, err := renderer.Render(prepared, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	out, err = renderer.Finalize(prepared, out, err)
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if !strings.Contains(string(out), "broken.gohtml:1") {
		t.Fatalf("expected detailed location, got %q", out)
	}
}

func TestWriteWithRequestUsesErrorRenderResponse(t *testing.T) {
	p := partial.New("broken.gohtml").
		ID("broken").
		SetFileSystem(fstest.MapFS{
			"broken.gohtml": &fstest.MapFile{Data: []byte(`{{ if .Missing }}missing`)},
		}).
		Use(Renderer(WithMode(ModeDetailed)))

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()
	err := p.WriteWithRequest(req.Context(), rec, req)
	if err == nil {
		t.Fatal("WriteWithRequest() error = nil, want original render error")
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if !strings.Contains(rec.Body.String(), "Template render error") {
		t.Fatalf("expected rendered error body, got %q", rec.Body.String())
	}
}

func TestRendererFinalizeKeepsOriginalRenderErrorWhenErrorTemplateFails(t *testing.T) {
	originalErr := errors.New("original render failed")
	rendererErr := errors.New("error template failed")
	ctx := &partial.RenderContext{
		Kind:   RenderKindError,
		Error:  originalErr,
		Values: make(partial.RenderValues),
	}
	renderer := Renderer()

	prepared, err := renderer.Prepare(ctx)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	_, err = renderer.Finalize(prepared, "", rendererErr)
	if !errors.Is(err, rendererErr) {
		t.Fatalf("Finalize() error = %v, want renderer error", err)
	}
	if !errors.Is(err, originalErr) {
		t.Fatalf("Finalize() error = %v, want original error", err)
	}
}
