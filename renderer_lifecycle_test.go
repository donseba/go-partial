package partial

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestRendererChainOrder(t *testing.T) {
	var calls []string
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`hello {{.}}`)},
	}

	stage := func(name string) RenderStage {
		return RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				calls = append(calls, "pre:"+name)
				return ctx, nil
			},
			RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
				calls = append(calls, "in-before:"+name)
				out, err := next(ctx)
				calls = append(calls, "in-after:"+name)
				return template.HTML(name + "[" + string(out) + "]"), err
			},
			FinalizeFunc: func(ctx *RenderContext, out template.HTML, err error) (template.HTML, error) {
				calls = append(calls, "post:"+name)
				return template.HTML(name + "-post(" + string(out) + ")"), err
			},
		}
	}

	out, err := New("page.gohtml").
		SetFileSystem(fsys).
		SetDot("world").
		Use(stage("a"), stage("b")).
		Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if got, want := string(out), "a-post(b-post(a[b[hello world]]))"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}

	wantCalls := []string{
		"pre:a",
		"pre:b",
		"in-before:a",
		"in-before:b",
		"in-after:b",
		"in-after:a",
		"post:b",
		"post:a",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
}

func TestRendererPrepareEnrichesTemplateContext(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{(ctx).Values.Get "message"}}`)},
	}

	out, err := New("page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				ctx.Values.Set("message", "from prepare")
				return ctx, nil
			},
		}).
		Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if got, want := string(out), "from prepare"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderTemplateRequiresRenderContext(t *testing.T) {
	_, err := NewID("page", "page.gohtml").renderTemplate(nil)
	if err == nil {
		t.Fatal("renderTemplate(nil) error = nil, want error")
	}
}

func TestServiceRendererAppliesToLayoutPartials(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`content`)},
		"layout.gohtml":  &fstest.MapFile{Data: []byte(`layout:{{content}}`)},
	}

	svc := NewService(&Config{FS: fsys})
	svc.Use(RenderStageHooks{
		FinalizeFunc: func(ctx *RenderContext, out template.HTML, err error) (template.HTML, error) {
			return template.HTML("[" + string(out) + "]"), err
		},
	})

	req := httptest.NewRequest("GET", "/", nil)
	out, err := svc.NewLayout().
		Set(NewID("content", "content.gohtml")).
		Wrap(NewID("layout", "layout.gohtml")).
		RenderWithRequest(req.Context(), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if got, want := string(out), "[layout:[content]]"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRendererCanHandleErrorKind(t *testing.T) {
	fsys := fstest.MapFS{
		"broken.gohtml": &fstest.MapFile{Data: []byte(`{{ if .Missing }}missing`)},
	}

	p := New("broken.gohtml").
		ID("broken").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
				if ctx.Kind != renderKindError {
					return next(ctx)
				}
				return template.HTML(`<main data-kind="` + string(ctx.Kind) + `">` + ctx.Partial.PartialID() + `</main>`), nil
			},
		})

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()
	err := p.WriteWithRequest(context.Background(), rec, req)
	if err == nil {
		t.Fatal("expected original render error")
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got, want := rec.Body.String(), `<main data-kind="error">broken</main>`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestRendererCanHandleRuntimeRenderKind(t *testing.T) {
	fsys := fstest.MapFS{
		"inspect.gohtml": &fstest.MapFile{Data: []byte(`{{ inspect runtime . }}`)},
	}

	const renderKindInspect RenderKind = "inspect"

	p := New("inspect.gohtml").
		SetFileSystem(fsys).
		SetDot("Ada").
		SetFunc(template.FuncMap{
			"inspect": func(runtime *Runtime, value any) template.HTML {
				out, err := runtime.RenderWith(renderKindInspect, "", value, func(ctx *RenderContext) (template.HTML, error) {
					return template.HTML(template.HTMLEscapeString(fmt.Sprint(ctx.Data))), nil
				})
				if err != nil {
					return template.HTML(template.HTMLEscapeString(err.Error()))
				}
				return out
			},
		}).
		Use(RenderStageHooks{
			RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
				if ctx.Kind != renderKindInspect {
					return next(ctx)
				}
				return template.HTML(`<aside data-kind="` + string(ctx.Kind) + `">` + ctx.Data.(string) + `</aside>`), nil
			},
		})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if got, want := string(out), `<aside data-kind="inspect">Ada</aside>`; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

var _ fs.FS = fstest.MapFS{}
