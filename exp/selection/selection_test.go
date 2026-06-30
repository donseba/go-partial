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
	exterrors "github.com/donseba/go-partial/ext/errors"
)

func TestRendererRendersSelectedPartial(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`{{ selection }}`)},
		"summary.gohtml": &fstest.MapFile{Data: []byte(`summary:{{ selectionValue }}`)},
		"details.gohtml": &fstest.MapFile{Data: []byte(`details`)},
	}
	content := partial.NewID("content", "content.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewPartial(nil)).
		SetFunc(FuncMap()).
		Use(Renderer())
	WithSelectMap(content, "summary", map[string]*partial.Partial{
		"summary": partial.NewID("summary", "summary.gohtml").SetFileSystem(fsys),
		"details": partial.NewID("details", "details.gohtml").SetFileSystem(fsys),
	})

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	req.Header.Set(connector.HeaderSelect.String(), "summary")
	out, err := content.RenderWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	if string(out) != "summary:summary" {
		t.Fatalf("output = %q", out)
	}
}

func TestSelectionIsUsesDefault(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`{{ selectionHeader }}:{{ if selectionIs "summary" }}yes{{ end }}`)},
	}
	content := partial.NewID("content", "content.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewPartial(nil)).
		SetFunc(FuncMap()).
		Use(Renderer())
	WithSelectMap(content, "summary", nil)

	out, err := content.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if string(out) != "X-Select:yes" {
		t.Fatalf("output = %q", out)
	}
}

func TestRendererUsesErrorFallbackForSelectedPartial(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`{{ selection }}`)},
		"broken.gohtml":  &fstest.MapFile{Data: []byte(`{{ if .Missing }}broken`)},
	}
	content := partial.NewID("content", "content.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewPartial(nil)).
		SetFunc(FuncMap()).
		Use(Renderer(), exterrors.Renderer(exterrors.WithMode(exterrors.ModeDetailed)))
	WithSelectMap(content, "broken", map[string]*partial.Partial{
		"broken": partial.NewID("broken", "broken.gohtml").SetFileSystem(fsys),
	})

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	req.Header.Set(connector.HeaderSelect.String(), "broken")
	out, err := content.RenderWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected configured error fragment, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml") {
		t.Fatalf("expected selected partial template in error output, got %q", body)
	}
}
