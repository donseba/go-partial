package slots

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

func fragmentsFS(page string) fstest.MapFS {
	return fstest.MapFS{
		"layout.gohtml":             {Data: []byte(`<body>{{ content }}</body>`)},
		"page.gohtml":               {Data: []byte(page)},
		"fragments/popular.gohtml":  {Data: []byte(`<section id="popular">popular</section>`)},
		"fragments/comments.gohtml": {Data: []byte(`<section id="comments">comments</section>`)},
		"regions/menu.gohtml":       {Data: []byte(`<nav id="menu"{{ oobAttr }}>menu</nav>`)},
	}
}

func fragmentsPage(t *testing.T, page string) *partial.Partial {
	t.Helper()

	root := partial.NewID("root", "layout.gohtml").
		SetFileSystem(fragmentsFS(page)).
		SetConnector(connector.NewHTMX4(nil)).
		Use(Stage()).
		SetContent(partial.NewID("content", "page.gohtml"))

	if err := Glob(root, "fragments/*.gohtml", OmitOOB()); err != nil {
		t.Fatal(err)
	}

	if err := Glob(root, "regions/*.gohtml", OOB()); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestDeferredSlotDefersOneOfTheGlobbedSlots(t *testing.T) {
	root := fragmentsPage(t, `<main>{{ slot "comments" }}{{ deferredSlot "popular" "reveal" }}</main>`)

	out, err := partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/journal", nil), root)
	if err != nil {
		t.Fatal(err)
	}

	want := `<body><main><section id="comments">comments</section>` +
		`<div id="popular" hx-get="/journal" hx-swap="outerHTML" hx-target="this" hx-trigger="revealed" aria-busy="true"></div></main></body>`
	if string(out) != want {
		t.Fatalf("page =\n%s\nwant\n%s", out, want)
	}

	req := httptest.NewRequest(http.MethodGet, "/journal", nil)
	connector.SetRequestTarget(root.Connector(), req, "popular")

	rec := httptest.NewRecorder()
	if err := partial.Write(context.Background(), rec, req, root); err != nil {
		t.Fatal(err)
	}

	if body := rec.Body.String(); body != `<section id="popular">popular</section>` {
		t.Fatalf("slot response = %q", body)
	}
}

func TestDeferredSlotLoadsAtOnceAndInPlaceWithoutARequest(t *testing.T) {
	root := fragmentsPage(t, `<main>{{ deferredSlot "popular" }}</main>`)

	out, err := partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/j", nil), root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `hx-trigger="load"`) {
		t.Fatalf("page = %s", out)
	}

	inPlace, err := partial.Render(context.Background(), fragmentsPage(t, `<main>{{ deferredSlot "popular" "load" }}</main>`))
	if err != nil {
		t.Fatal(err)
	}

	if string(inPlace) != `<body><main><section id="popular">popular</section></main></body>` {
		t.Fatalf("render without a request = %s", inPlace)
	}
}

func TestDeferredSlotFailures(t *testing.T) {
	tests := []struct {
		name string
		page string
		want string
	}{
		{"unknown mode", `{{ deferredSlot "popular" "later" }}`, "deferred mode must be load or reveal"},
		{"out of band", `{{ deferredSlot "menu" }}`, "out of band"},
	}

	for _, test := range tests {
		// The default failure policy shows the error in place of the part.
		out, err := partial.RenderWithRequest(
			context.Background(),
			httptest.NewRequest(http.MethodGet, "/", nil),
			fragmentsPage(t, test.page),
		)
		if err != nil || !strings.Contains(string(out), "error calling deferredSlot") ||
			!strings.Contains(string(out), test.want) {
			t.Errorf("%s: output = %q, %v, want the error %q", test.name, out, err, test.want)
		}
	}

	out, err := partial.RenderWithRequest(
		context.Background(),
		httptest.NewRequest(http.MethodGet, "/", nil),
		fragmentsPage(t, `<main>{{ deferredSlot "missing" }}</main>`),
	)
	if err != nil || string(out) != `<body><main></main></body>` {
		t.Fatalf("missing slot = %q, %v", out, err)
	}
}

func TestDeferredSlotFuncMap(t *testing.T) {
	root := partial.NewID("root", "page.gohtml").
		SetFileSystem(fragmentsFS(`<main>{{ deferredSlot "popular" "reveal" ctx }}</main>`)).
		SetConnector(connector.NewHTMX4(nil)).
		SetFunc(FuncMap())

	if err := Glob(root, "fragments/*.gohtml", OmitOOB()); err != nil {
		t.Fatal(err)
	}

	out, err := partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/j", nil), root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `<div id="popular" hx-get="/j"`) ||
		!strings.Contains(string(out), `hx-trigger="revealed"`) {
		t.Fatalf("page = %s", out)
	}

	if _, err := DeferredSlot("popular"); err == nil {
		t.Fatal("DeferredSlot() without a render context did not fail")
	}
}

type panelHeading struct {
	Title string
}

func TestPlaceholderDataCarriesTheSlotsData(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml": {Data: []byte(`<body>{{ content }}</body>`)},
		"page.gohtml":   {Data: []byte(`<main>{{ slot "recent" }}{{ slot "plain" }}</main>`)},
		"recent.gohtml": {Data: []byte(`<section id="recent">recent</section>`)},
		"plain.gohtml":  {Data: []byte(`<section id="plain">plain</section>`)},
		"loading.gohtml": {
			Data: []byte(`<section id="{{ .ID }}"{{ .Attrs }}>{{ with .Data }}{{ .Title }}{{ else }}untitled{{ end }}</section>`),
		},
	}

	root := partial.NewID("root", "layout.gohtml").
		SetFileSystem(files).
		SetConnector(connector.NewHTMX4(nil)).
		Use(Stage()).
		SetContent(partial.NewID("content", "page.gohtml"))

	loading := partial.NewID("loading", "loading.gohtml")
	Set(
		root,
		"recent",
		partial.NewID("recent", "recent.gohtml"),
		Placeholder(loading),
		Data(panelHeading{Title: "Recent changes"}),
	)
	Set(root, "plain", partial.NewID("plain", "plain.gohtml"), Placeholder(loading))

	out, err := partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), root)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), `hx-trigger="load">Recent changes</section>`) ||
		!strings.Contains(string(out), `hx-trigger="load">untitled</section>`) {
		t.Fatalf("page = %s", out)
	}
}
