package errors

import (
	"errors"
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"

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

	out, err := Renderer(WithMode(ModeDetailed)).InFlight(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("InFlight() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected fragment error class, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml:1") {
		t.Fatalf("expected detailed location, got %q", body)
	}
}
