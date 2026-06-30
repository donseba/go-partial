package localization

import (
	"context"
	"html/template"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

type testLocalizer struct {
	locale string
}

func (l testLocalizer) GetLocale() string {
	return l.locale
}

func TestRendererAddsLocaleHelpers(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ locale }}:{{ localizer.GetLocale }}`)},
	}
	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())

	req := httptest.NewRequest("GET", "/", nil)
	ctx := WithLocalizer(context.Background(), testLocalizer{locale: "nl_NL"})
	out, err := p.RenderWithRequest(ctx, req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if string(out) != "nl_NL:nl_NL" {
		t.Fatalf("output = %q", out)
	}
}

func TestFuncMapProvidesParsePlaceholders(t *testing.T) {
	if _, ok := FuncMap()["locale"].(func(...*partial.RenderContext) string); !ok {
		t.Fatalf("locale placeholder missing")
	}
	if _, ok := FuncMap()["localizer"].(func(...*partial.RenderContext) Localizer); !ok {
		t.Fatalf("localizer placeholder missing")
	}
}

var _ template.HTML

func TestRendererAddsLocaleHelpersConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ locale }}`)},
	}
	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := "locale_" + strconv.Itoa(i)
			req := httptest.NewRequest("GET", "/", nil)
			out, err := p.RenderWithRequest(WithLocalizer(req.Context(), testLocalizer{locale: value}), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != value {
				errs <- "locale got " + got + " want " + value
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
