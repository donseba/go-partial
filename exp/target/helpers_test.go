package target

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// renderTarget renders the table partial, with the target stage, for a
// request for target, after configure.
func renderTarget(t *testing.T, target string, configure func(table *partial.Partial)) string {
	t.Helper()

	fsys := fstest.MapFS{
		"table.gohtml": &fstest.MapFile{Data: []byte(`<table></table>`)},
	}

	table := partial.NewID("content", "table.gohtml").
		SetFileSystem(fsys).
		Use(Stage())
	configure(table)

	req := httptest.NewRequest(http.MethodGet, "/rows", nil)
	req.Header.Set(connector.HeaderTarget.String(), target)

	out, err := partial.RenderWithRequest(context.Background(), req, table)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	return string(out)
}

// TestResolverThatDoesNotResolve checks that the table renders as it is when
// the resolver resolves nothing, is nil, or is not a Resolver.
func TestResolverThatDoesNotResolve(t *testing.T) {
	tests := map[string]func(table *partial.Partial){
		"no resolver": func(*partial.Partial) {
			// The table keeps no resolver.
		},
		"resolver says no": func(table *partial.Partial) {
			WithResolver(table, func(context.Context, *http.Request, string) (*partial.Partial, bool) {
				return nil, false
			})
		},
		"resolver gives nil": func(table *partial.Partial) {
			WithResolver(table, func(context.Context, *http.Request, string) (*partial.Partial, bool) {
				return nil, true
			})
		},
		"nil resolver": func(table *partial.Partial) {
			WithResolver(table, nil)
		},
		"another value": func(table *partial.Partial) {
			table.SetExtension(extensionKey{}, "not a resolver")
		},
	}

	for name, configure := range tests {
		t.Run(name, func(t *testing.T) {
			if got := renderTarget(t, "content", configure); got != "<table></table>" {
				t.Fatalf("output = %q, want the table", got)
			}
		})
	}
}

// TestFuncMapHelpersTakeTheRenderContext checks the helpers of FuncMap
// given the render context, without the stage, and without a context.
func TestFuncMapHelpersTakeTheRenderContext(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ targetHeader ctx }}={{ targetValue ctx }}:{{ targetIs "content" }}`)},
	}

	page := partial.NewID("content", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderTarget.String(), "content")

	out, err := partial.RenderWithRequest(context.Background(), req, page)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	// The targetIs of FuncMap has no render context: it is false.
	if want := "X-Target=content:false"; string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	if TargetHeader() != "" || TargetValue() != "" || TargetIs("content") {
		t.Fatal("helpers without a render context answered")
	}

	empty := &partial.RenderContext{}
	if TargetHeader(empty) != "" || TargetValue(empty) != "" {
		t.Fatal("helpers without a runtime answered")
	}

	if WithResolver(nil, nil) != nil {
		t.Fatal("WithResolver(nil) is not nil")
	}

	if fn, ok := resolveFunc(empty, "other"); ok || fn != nil {
		t.Fatal("resolveFunc() gave a function it does not have")
	}
}
