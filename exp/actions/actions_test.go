package actions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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
		Use(Stage())
	WithAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("next", "next.gohtml").SetFileSystem(fsys), nil
	})

	out, err := partial.Render(context.Background(), p)
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
		Use(Stage())
	WithTemplateAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("result", "result.gohtml").SetFileSystem(fsys), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderAction.String(), "save")
	out, err := partial.RenderWithRequest(context.Background(), req, p)
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
		Use(Stage(), exterrors.Stage(exterrors.WithMode(exterrors.ModeDetailed)))
	WithTemplateAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("broken", "broken.gohtml").SetFileSystem(fsys), nil
	})

	out, err := partial.Render(context.Background(), p)
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

func TestTemplateActionRendersConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml":   &fstest.MapFile{Data: []byte(`{{ actionValue }}:{{ action }}`)},
		"result.gohtml": &fstest.MapFile{Data: []byte(`{{ (request).URL.Query.Get "value" }}`)},
	}
	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Stage())
	WithTemplateAction(p, func(ctx context.Context, p *partial.Partial, runtime *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("result", "result.gohtml").SetFileSystem(fsys), nil
	})

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?value="+value, nil)
			req.Header.Set(connector.HeaderAction.String(), "save-"+value)
			out, err := partial.RenderWithRequest(req.Context(), req, p)
			if err != nil {
				errs <- err.Error()
				return
			}
			want := "save-" + value + ":" + value
			if got := string(out); got != want {
				errs <- "action " + value + " got " + got + " want " + want
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
