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

func TestStageChainOrder(t *testing.T) {
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

	p := New("page.gohtml").
		SetFileSystem(fsys).
		SetDot("world").
		Use(stage("a"), stage("b"))
	out, err := Render(context.Background(), p)
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

func TestStagePrepareEnrichesTemplateContext(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{(ctx).Values.Get "message"}}`)},
	}

	p := New("page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				ctx.Values.Set("message", "from prepare")
				return ctx, nil
			},
		})
	out, err := Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if got, want := string(out), "from prepare"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRenderTemplateRequiresRenderContext(t *testing.T) {
	_, err := renderTemplate(nil)
	if err == nil {
		t.Fatal("renderTemplate(nil) error = nil, want error")
	}
}

func TestRootStageAppliesToContentPartials(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`content`)},
		"shell.gohtml":   &fstest.MapFile{Data: []byte(`shell:{{content}}`)},
	}

	svc := newTestBlueprint(testBlueprintFS(fsys))
	svc.Use(RenderStageHooks{
		FinalizeFunc: func(ctx *RenderContext, out template.HTML, err error) (template.HTML, error) {
			return template.HTML("[" + string(out) + "]"), err
		},
	})

	req := httptest.NewRequest("GET", "/", nil)
	root := svc.Compose(NewID("content", "content.gohtml"), NewID("shell", "shell.gohtml"))
	out, err := RenderWithRequest(svc.RenderContext(req.Context()), req, root)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if got, want := string(out), "[shell:[content]]"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestStageCanHandleErrorKind(t *testing.T) {
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
	err := Write(context.Background(), rec, req, p)
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

func TestStageCanHandleRuntimeRenderKind(t *testing.T) {
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

	out, err := Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if got, want := string(out), `<aside data-kind="inspect">Ada</aside>`; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

var _ fs.FS = fstest.MapFS{}

func TestStageFuncResolver(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{greet "Ada"}} {{shout "hi"}}`)},
	}

	var asked []string
	stage := RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			ctx.SetFuncResolver(func(name string) (any, bool) {
				asked = append(asked, name)
				switch name {
				case "greet":
					return func(name string) string { return "hello " + name }, true
				case "shout":
					return func(string) string { return "resolved" }, true
				}
				return nil, false
			})
			ctx.SetFunc("shout", func(s string) string { return s + "!" })
			return ctx, nil
		},
	}

	for _, useCache := range []bool{true, false} {
		asked = nil
		p := New("page.gohtml").
			SetFileSystem(fsys).
			UseTemplateCache(useCache).
			SetFunc(template.FuncMap{
				"greet": func(string) string { return "stand-in" },
				"shout": func(string) string { return "stand-in" },
				"other": func() string { return "unused" },
			}).
			Use(stage)

		out, err := Render(context.Background(), p)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", useCache, err)
		}
		if got, want := string(out), "hello Ada hi!"; got != want {
			t.Fatalf("cache %v: output = %q, want %q", useCache, got, want)
		}
		if useCache && !reflect.DeepEqual(asked, []string{"greet"}) {
			t.Fatalf("cache %v: resolver asked for %v, want only the functions the template calls", useCache, asked)
		}
	}
}
