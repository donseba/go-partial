package debug

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestRendererRendersDebugBox(t *testing.T) {
	ctx := &partial.RenderContext{
		Kind: RenderKindDebug,
		Data: map[string]any{"name": "Ada"},
	}

	out, err := Stage().Render(ctx, func(ctx *partial.RenderContext) (template.HTML, error) {
		return "", nil
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
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
		Use(Stage())

	out, err := partial.Render(context.Background(), p)
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
		Use(partial.RenderStageHooks{
			RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
				if ctx.Kind != RenderKindDebug {
					return next(ctx)
				}
				return template.HTML(`<aside class="custom-debug">` + ctx.Data.(string) + `</aside>`), nil
			},
		})

	out, err := partial.Render(context.Background(), p)
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
		Use(partial.RenderStageHooks{
			RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
				if ctx.Kind != RenderKindDebug {
					return next(ctx)
				}
				return template.HTML(`<aside class="child-debug">` + ctx.Data.(string) + `</aside>`), nil
			},
		})

	out, err := partial.Render(context.Background(), parent)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="child-debug">Ada</aside>` {
		t.Fatalf("expected child debug RenderStage to survive clone, got %q", out)
	}
}

func TestFormatValueUsesJSONWhenPossible(t *testing.T) {
	if got := FormatValue(map[string]any{"a": 1}); !strings.Contains(got, `"a": 1`) {
		t.Fatalf("FormatValue() = %q", got)
	}
}

func TestFuncMapRendersConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"debug.gohtml": &fstest.MapFile{Data: []byte(`{{ debug runtime (url).RawQuery }}`)},
	}
	p := partial.NewID("debug", "debug.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Stage())

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+value, nil)
			out, err := partial.RenderWithRequest(req.Context(), req, p)
			if err != nil {
				errs <- err.Error()
				return
			}
			body := string(out)
			if !strings.Contains(body, `class="go-partial-debug"`) || !strings.Contains(body, "value="+value) {
				errs <- "debug " + value + " got " + body
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
