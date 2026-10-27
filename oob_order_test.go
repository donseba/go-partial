package partial

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-partial/connector"
)

// oobOrderRoot is a page with out-of-band regions registered in the order
// of names, which is not the order of their IDs.
func oobOrderRoot(names []string) *Partial {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<!doctype html><body>{{content}}</body>`)},
		"content.gohtml": {Data: []byte(`<main id="content">Page</main>`)},
	}
	for _, name := range names {
		files[name+".gohtml"] = &fstest.MapFile{Data: []byte(`<div id="` + name + `"{{oobAttr}}>` + name + `</div>`)}
	}

	root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(connector.NewHTMX4(nil))
	root.SetContent(NewID("content", "content.gohtml"))

	for _, name := range names {
		region := NewID(name, name+".gohtml").SetAlwaysSwapOOB(true)
		region.SetOOBElementsFunc(func(*http.Request) OOBElements {
			return OOBElements{Vary: []string{"X-" + name}}
		})
		root.WithOOB(region)
	}

	return root
}

// writeOOBOrder answers an htmx navigation for root and returns the body
// and the Vary header.
func writeOOBOrder(t *testing.T, root *Partial) (string, string) {
	t.Helper()

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Request-Type", "partial")
	r.Header.Set("HX-Target", "main#content")

	w := httptest.NewRecorder()
	if err := Write(context.Background(), w, r, root); err != nil {
		t.Fatal(err)
	}

	return w.Body.String(), strings.Join(w.Header().Values("Vary"), ",")
}

// TestOOBOrder checks that out-of-band regions are sent in the order they
// were registered, the same bytes on every render, and that registering a
// region again keeps its place.
func TestOOBOrder(t *testing.T) {
	names := []string{"title", "menu", "head", "zeta", "alpha", "cart", "notice", "breadcrumbs"}
	root := oobOrderRoot(names)
	root.WithOOB(NewID("menu", "menu.gohtml").SetAlwaysSwapOOB(true))

	first, firstVary := writeOOBOrder(t, root)

	want := `<main id="content">Page</main>`
	for _, name := range names {
		want += fmt.Sprintf(`<div id="%s" hx-swap-oob="true">%s</div>`, name, name)
	}

	if first != want {
		t.Fatalf("got  %s\nwant %s", first, want)
	}

	for range 200 {
		body, vary := writeOOBOrder(t, root)
		if body != first {
			t.Fatalf("output changed between renders:\n%s\n%s", first, body)
		}

		if vary != firstVary {
			t.Fatalf("Vary changed between renders: %q, %q", firstVary, vary)
		}
	}
}

// TestOOBOrderAncestors checks that the regions of nested partials come
// nearest first, each partial's own in the order they were registered.
func TestOOBOrderAncestors(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<!doctype html>{{content}}`)},
		"section.gohtml": {Data: []byte(`<section>{{content}}</section>`)},
		"content.gohtml": {Data: []byte(`<main id="content">Page</main>`)},
		"region.gohtml":  {Data: []byte(`<div id="{{.}}"{{oobAttr}}></div>`)},
	}

	root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(connector.NewHTMX4(nil))
	section := NewID("section", "section.gohtml")
	content := NewID("content", "content.gohtml")
	section.SetContent(content)
	root.SetContent(section)

	for _, id := range []string{"outer-b", "outer-a"} {
		root.WithOOB(NewID(id, "region.gohtml").SetDot(id))
	}

	for _, id := range []string{"inner-b", "inner-a"} {
		section.WithOOB(NewID(id, "region.gohtml").SetDot(id))
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Request-Type", "partial")
	r.Header.Set("HX-Target", "main#content")

	var first string

	for i := range 100 {
		w := httptest.NewRecorder()
		if err := Write(context.Background(), w, r, root); err != nil {
			t.Fatal(err)
		}

		if i == 0 {
			first = w.Body.String()
		}

		if w.Body.String() != first {
			t.Fatalf("output changed between renders:\n%s\n%s", first, w.Body.String())
		}
	}

	order := []string{"inner-b", "inner-a", "outer-b", "outer-a"}
	last := -1

	for _, id := range order {
		at := strings.Index(first, `id="`+id+`"`)
		if at < last {
			t.Fatalf("%s out of order in %s", id, first)
		}

		last = at
	}
}
