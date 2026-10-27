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
		Use(Stage())

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
	out, err := partial.RenderWithRequest(context.Background(), req, table)
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
		Use(Stage())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderTarget.String(), "content")

	out, err := partial.RenderWithRequest(context.Background(), req, p)
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
		Use(Stage())
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
			out, err := partial.RenderWithRequest(req.Context(), req, table)
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

type fragmentPage struct {
	Title string
}

func TestTargetHelpersWithoutStandIns(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ targetValue }}`)},
	}
	p := partial.NewID("content", "page.gohtml").
		SetFileSystem(fsys).
		Use(Stage())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderTarget.String(), "content")

	out, err := partial.RenderWithRequest(context.Background(), req, p)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if string(out) != "content" {
		t.Fatalf("output = %q", out)
	}
}

// The benchmarks render a page with regions, and a fragment of it that a
// resolver answers.

func BenchmarkResolvedTargetPage(b *testing.B) {
	benchmarkResolvedTarget(b, "")
}

func BenchmarkResolvedTargetFragment(b *testing.B) {
	benchmarkResolvedTarget(b, "section#comments")
}

func benchmarkResolvedTarget(b *testing.B, target string) {
	fsys := partial.Sources{
		"shell.gohtml":    `{{/* @model Page github.com/donseba/go-partial/exp/target.fragmentPage */}}<title>{{ Page.Title }}</title>{{ content }}<footer>{{ targetValue }}</footer>`,
		"content.gohtml":  `<main id="content">{{ range . }}<p>{{ . }}</p>{{ end }}</main>`,
		"region.gohtml":   `<nav id="{{ . }}"{{ oobAttr }}>{{ . }}</nav>`,
		"comments.gohtml": `{{/* @model Page github.com/donseba/go-partial/exp/target.fragmentPage */}}<section id="comments">{{ Page.Title }}</section>`,
	}

	base := partial.NewID("base").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX4(nil)).
		UseTemplateCache(true).
		SetFunc(FuncMap()).
		Use(Stage())

	render := func() {
		root := partial.NewID("shell", "shell.gohtml").
			SetParent(base).
			UseTemplateCache(true).
			SetModel(fragmentPage{Title: "Hello"}).
			SetContent(partial.NewID("content", "content.gohtml").UseTemplateCache(true).SetDot([]string{"a", "b", "c"}))

		for _, id := range []string{"menu", "languages", "flash"} {
			root.WithOOB(partial.NewID(id, "region.gohtml").UseTemplateCache(true).SetDot(id).SetAlwaysSwapOOB(true))
		}

		WithResolver(root, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
			if target != "comments" {
				return nil, false
			}

			return partial.NewID("comments", "comments.gohtml").UseTemplateCache(true), true
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if target != "" {
			req.Header.Set("HX-Request", "true")
			req.Header.Set("HX-Target", target)
		}

		rec := httptest.NewRecorder()
		if err := partial.Write(context.Background(), rec, req, root); err != nil {
			b.Fatal(err)
		}

		if rec.Body.Len() == 0 {
			b.Fatal("empty render")
		}
	}

	render()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		render()
	}
}
