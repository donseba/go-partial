package selection

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

func TestRendererRendersConcurrentSelections(t *testing.T) {
	fsys := fstest.MapFS{
		"content.gohtml": &fstest.MapFile{Data: []byte(`{{ selection }}`)},
		"a.gohtml":       &fstest.MapFile{Data: []byte(`a:{{ (request).URL.Query.Get "value" }}`)},
		"b.gohtml":       &fstest.MapFile{Data: []byte(`b:{{ (request).URL.Query.Get "value" }}`)},
	}
	content := partial.NewID("content", "content.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewPartial(nil)).
		SetFunc(FuncMap()).
		Use(Renderer())
	WithSelectMap(content, "a", map[string]*partial.Partial{
		"a": partial.NewID("a", "a.gohtml").SetFileSystem(fsys),
		"b": partial.NewID("b", "b.gohtml").SetFileSystem(fsys),
	})

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			selected := "a"
			if i%2 == 1 {
				selected = "b"
			}
			req := httptest.NewRequest(http.MethodGet, "/tabs?value="+value, nil)
			req.Header.Set(connector.HeaderSelect.String(), selected)
			out, err := content.RenderWithRequest(req.Context(), req)
			if err != nil {
				errs <- err.Error()
				return
			}
			want := selected + ":" + value
			if got := string(out); got != want {
				errs <- "selection " + value + " got " + got + " want " + want
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
