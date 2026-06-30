package csrf

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

type staticToken struct{}

func (staticToken) Token(context.Context) string { return "token-123" }
func (staticToken) Key() string                  { return "X-Test-CSRF" }

type valueToken string

func (t valueToken) Token(context.Context) string { return string(t) }
func (t valueToken) Key() string                  { return "X-Test-CSRF" }

func TestRendererAddsCSRFHelper(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ csrf.Key }}={{ csrf.Token ctx.Context }}`)},
	}
	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Renderer())

	req := httptest.NewRequest("GET", "/", nil)
	out, err := p.RenderWithRequest(WithToken(context.Background(), staticToken{}), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if string(out) != "X-Test-CSRF=token-123" {
		t.Fatalf("output = %q", out)
	}
}

func TestWithTokenString(t *testing.T) {
	token := FromContext(WithTokenString(context.Background(), "abc"))
	if got := token.Token(WithTokenString(context.Background(), "abc")); got != "abc" {
		t.Fatalf("token = %q", got)
	}
}

var _ template.HTML

func TestRendererAddsCSRFHelperConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ csrf.Token ctx.Context }}`)},
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
			value := "token-" + strconv.Itoa(i)
			req := httptest.NewRequest("GET", "/", nil)
			ctx := WithToken(req.Context(), valueToken(value))
			out, err := p.RenderWithRequest(ctx, req)
			if err != nil {
				errs <- err.Error()
				return
			}
			if got := string(out); got != value {
				errs <- "csrf got " + got + " want " + value
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
