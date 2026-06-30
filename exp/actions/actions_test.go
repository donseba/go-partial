package actions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	exterrors "github.com/donseba/go-partial/ext/errors"
)

func TestWithActionCanReplacePartial(t *testing.T) {
	fsys := fstest.MapFS{
		"start.gohtml": &fstest.MapFile{Data: []byte(`start`)},
		"next.gohtml":  &fstest.MapFile{Data: []byte(`next`)},
	}
	p := partial.NewID("start", "start.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())
	WithAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("next", "next.gohtml").SetFileSystem(fsys), nil
	})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if string(out) != "next" {
		t.Fatalf("output = %q", out)
	}
}

func TestTemplateActionAndHelpers(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml":   &fstest.MapFile{Data: []byte(`{{ actionHeader }}={{ actionValue }}:{{ action }}`)},
		"result.gohtml": &fstest.MapFile{Data: []byte(`result`)},
	}
	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())
	WithTemplateAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("result", "result.gohtml").SetFileSystem(fsys), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderAction.String(), "save")
	out, err := p.RenderWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if string(out) != "X-Action=save:result" {
		t.Fatalf("output = %q", out)
	}
}

func TestTemplateActionUsesErrorFallback(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml":   &fstest.MapFile{Data: []byte(`{{ action }}`)},
		"broken.gohtml": &fstest.MapFile{Data: []byte(`{{ if .Missing }}broken`)},
	}
	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer(), exterrors.Renderer(exterrors.WithMode(exterrors.ModeDetailed)))
	WithTemplateAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("broken", "broken.gohtml").SetFileSystem(fsys), nil
	})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected configured error fragment, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml") {
		t.Fatalf("expected action partial template in error output, got %q", body)
	}
}
