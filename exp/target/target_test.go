package target

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func TestRendererResolvesDynamicTarget(t *testing.T) {
	fsys := fstest.MapFS{
		"table.gohtml": &fstest.MapFile{Data: []byte(`<table></table>`)},
		"row.gohtml":   &fstest.MapFile{Data: []byte(`<tr id="{{ .ID }}">{{ .Name }}</tr>`)},
	}

	table := partial.NewID("content", "table.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())

	WithResolver(table, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
		if target != "row-2" {
			return nil, false
		}
		return partial.NewID(target, "row.gohtml").SetFileSystem(fsys).SetDot(map[string]any{
			"ID":   "row-2",
			"Name": "Tea",
		}), true
	})

	req := httptest.NewRequest(http.MethodGet, "/rows", nil)
	req.Header.Set(connector.HeaderTarget.String(), "row-2")
	out, err := table.RenderWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if string(out) != `<tr id="row-2">Tea</tr>` {
		t.Fatalf("output = %q", out)
	}
}

func TestRendererAddsTargetHelpers(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ targetHeader }}={{ targetValue }}:{{ targetIs "content" }}`)},
	}
	p := partial.NewID("content", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderTarget.String(), "content")

	out, err := p.RenderWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if string(out) != "X-Target=content:true" {
		t.Fatalf("output = %q", out)
	}
}

func TestRendererResolvesTargetsConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"table.gohtml": &fstest.MapFile{Data: []byte(`<table></table>`)},
		"row.gohtml":   &fstest.MapFile{Data: []byte(`<tr id="{{ .ID }}">{{ (request).URL.Query.Get "value" }}</tr>`)},
	}
	table := partial.NewID("content", "table.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())
	WithResolver(table, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
		return partial.NewID(target, "row.gohtml").SetFileSystem(fsys).SetDot(map[string]any{
			"ID": target,
		}), true
	})

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			target := "row-" + value
			req := httptest.NewRequest(http.MethodGet, "/rows?value="+value, nil)
			req.Header.Set(connector.HeaderTarget.String(), target)
			out, err := table.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			want := `<tr id="` + target + `">` + value + `</tr>`
			if got := string(out); got != want {
				errs <- "target " + value + " got " + got + " want " + want
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
