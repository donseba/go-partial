package debug

import (
	"context"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestRendererRendersDebugBox(t *testing.T) {
	ctx := &partial.RenderContext{
		Kind: RenderKindDebug,
		Data: map[string]any{"name": "Ada"},
	}

	out, err := Renderer().InFlight(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("InFlight() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-debug"`) {
		t.Fatalf("expected debug class, got %q", body)
	}
	if !strings.Contains(body, "Ada") {
		t.Fatalf("expected formatted value, got %q", body)
	}
}

func TestFuncMapRendersDebugBox(t *testing.T) {
	fsys := fstest.MapFS{
		"debug.gohtml": &fstest.MapFile{Data: []byte(`{{ debug runtime . }}`)},
	}

	p := partial.NewID("debug", "debug.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		SetDot(map[string]any{
			"a": 1,
			"b": "test",
		}).
		Use(Renderer())

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-debug"`) {
		t.Fatalf("expected styled debug box, got %q", body)
	}
	if !strings.Contains(body, `&#34;a&#34;: 1`) || !strings.Contains(body, `&#34;b&#34;: &#34;test&#34;`) {
		t.Fatalf("expected debug output to contain data, got %q", body)
	}
}

func TestFuncMapCanUseCustomRenderer(t *testing.T) {
	fsys := fstest.MapFS{
		"debug.gohtml": &fstest.MapFile{Data: []byte(`{{ debug runtime .Name }}`)},
	}

	p := partial.NewID("debug", "debug.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		SetDot(map[string]any{"Name": "Ada"}).
		Use(partial.RendererHooks{
			InFlightFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
				if ctx.Kind != RenderKindDebug {
					return next(ctx)
				}
				return template.HTML(`<aside class="custom-debug">` + ctx.Data.(string) + `</aside>`), nil
			},
		})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="custom-debug">Ada</aside>` {
		t.Fatalf("unexpected custom debug output: %q", out)
	}
}

func TestFuncMapDebugRendererSurvivesPartialClone(t *testing.T) {
	fsys := fstest.MapFS{
		"parent.gohtml": &fstest.MapFile{Data: []byte(`{{ partial runtime "child.gohtml" }}`)},
		"child.gohtml":  &fstest.MapFile{Data: []byte(`{{ debug runtime .Name }}`)},
	}

	parent := partial.NewID("parent", "parent.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		SetDot(map[string]any{"Name": "Ada"}).
		Use(partial.RendererHooks{
			InFlightFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
				if ctx.Kind != RenderKindDebug {
					return next(ctx)
				}
				return template.HTML(`<aside class="child-debug">` + ctx.Data.(string) + `</aside>`), nil
			},
		})

	out, err := parent.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="child-debug">Ada</aside>` {
		t.Fatalf("expected child debug renderer to survive clone, got %q", out)
	}
}

func TestFormatValueUsesJSONWhenPossible(t *testing.T) {
	if got := FormatValue(map[string]any{"a": 1}); !strings.Contains(got, `"a": 1`) {
		t.Fatalf("FormatValue() = %q", got)
	}
}
