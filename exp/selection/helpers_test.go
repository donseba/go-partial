package selection

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// renderSelection renders content.gohtml, src, with the selection stage,
// the select map when partials is not nil, and the select header set to
// selected.
func renderSelection(t *testing.T, src string, partials map[string]string, selected string) string {
	t.Helper()

	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(src)},
	}

	content := partial.NewID("content", "content.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewPartial(nil)).
		SetFunc(FuncMap()).
		Use(Stage())

	if partials != nil {
		children := make(map[string]*partial.Partial, len(partials))
		for key, body := range partials {
			fsys[key+".gohtml"] = &fstest.MapFile{Data: []byte(body)}
			children[key] = partial.NewID(key, key+".gohtml").SetFileSystem(fsys)
		}

		WithSelectMap(content, "summary", children)
	}

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	if selected != "" {
		req.Header.Set(connector.HeaderSelect.String(), selected)
	}

	out, err := partial.RenderWithRequest(context.Background(), req, content)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	return string(out)
}

// TestSelectionReportsProblems checks what {{ selection }} writes without a
// select map, for a key the map lacks, and for a selected partial that
// fails without an error stage to render its fallback.
func TestSelectionReportsProblems(t *testing.T) {
	children := map[string]string{
		"summary": `summary`,
		"broken":  `{{ if .Missing }}broken`,
	}

	if got := renderSelection(t, `{{ selection }}`, nil, ""); got != "selection is not configured" {
		t.Fatalf("without a select map = %q", got)
	}

	if got := renderSelection(t, `{{ selection }}`, children, "missing"); got != "selected partial 'missing' not found in parent 'content'" {
		t.Fatalf("missing key = %q", got)
	}

	got := renderSelection(t, `{{ selection }}`, children, "broken")
	if !strings.HasPrefix(got, "error rendering selected partial 'broken': ") {
		t.Fatalf("failing partial = %q", got)
	}

	if got := renderSelection(t, `{{ selection }}`, children, ""); got != "summary" {
		t.Fatalf("default = %q", got)
	}
}

func TestSelectionValueWithoutASelectMap(t *testing.T) {
	if got := renderSelection(t, `[{{ selectionValue }}]`, nil, ""); got != "[]" {
		t.Fatalf("value without a select map or header = %q", got)
	}

	if got := renderSelection(t, `[{{ selectionValue }}]`, nil, "details"); got != "[details]" {
		t.Fatalf("value from the header = %q", got)
	}
}

// TestFuncMapHelpersTakeTheRenderContext checks the helpers of FuncMap
// given the render context, without the stage, and without a context.
func TestFuncMapHelpersTakeTheRenderContext(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`{{ selectionHeader ctx }}={{ selectionValue ctx }}:{{ selection ctx }}:{{ selectionIs "summary" }}`)},
		"summary.gohtml": &fstest.MapFile{Data: []byte(`summary`)},
	}

	content := partial.NewID("content", "content.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap())
	WithSelectMap(content, "summary", map[string]*partial.Partial{
		"summary": partial.NewID("summary", "summary.gohtml").SetFileSystem(fsys),
	})

	out, err := partial.Render(context.Background(), content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// The selectionIs of FuncMap has no render context: it is false.
	if want := "X-Select=summary:summary:false"; string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	if SelectionHTML() != "" || SelectionHeader() != "" || SelectionValue() != "" || SelectionIs("summary") {
		t.Fatal("helpers without a render context answered")
	}

	if SelectionHeader(&partial.RenderContext{}) != "" {
		t.Fatal("SelectionHeader() without a runtime answered")
	}

	if WithSelectMap(nil, "summary", nil) != nil {
		t.Fatal("WithSelectMap(nil) is not nil")
	}

	if request(nil) == nil {
		t.Fatal("request(nil) is nil")
	}
}
