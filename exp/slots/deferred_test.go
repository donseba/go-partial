package slots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func dashboardFS() fstest.MapFS {
	return fstest.MapFS{
		"layout.gohtml":    {Data: []byte(`<body>{{ content }}</body>`)},
		"dashboard.gohtml": {Data: []byte(`<main>{{ slot "recent" }}</main>`)},
		"recent.gohtml": {
			Data: []byte(
				"{{/*\n@dot github.com/donseba/go-partial/exp/slots.comment\n*/}}<section id=\"recent\">{{ .Text }}</section>",
			),
		},
		"loading.gohtml": {
			Data: []byte(
				"{{/*\n@dot github.com/donseba/go-partial/exp/slots.PlaceholderData\n*/}}<section id=\"{{ .ID }}\" class=\"panel\"{{ .Attrs }}>Loading {{ .Name }}</section>",
			),
		},
		"regions/menu.gohtml":   {Data: []byte(`<nav id="menu"{{ oobAttr }}>{{ . }}</nav>`)},
		"regions/title.gohtml":  {Data: []byte(`<title id="title"{{ oobAttr }}>{{ . }}</title>`)},
		"regions/notes.txt":     {Data: []byte(`not a template`)},
		"glob-layout.gohtml":    {Data: []byte(`<body>{{ slot "menu" }}{{ slot "title" }}{{ content }}</body>`)},
		"missing.gohtml":        {Data: []byte(`<main>{{ slot "missing" }}</main>`)},
		"required.gohtml":       {Data: []byte(`<main>{{ requiredSlot "missing" }}</main>`)},
		"optional.gohtml":       {Data: []byte(`<main>{{ if hasSlot "missing" }}{{ slot "missing" }}{{ end }}</main>`)},
		"fragments/card.gohtml": {Data: []byte(`<div id="card">card</div>`)},
	}
}

func deferredPage(calls *atomic.Int32, conn connector.Connector, opts ...Option) *partial.Partial {
	root := partial.NewID("root", "layout.gohtml").
		SetFileSystem(dashboardFS()).
		SetConnector(conn).
		UseTemplateCache(true).
		Use(Stage()).
		SetContent(partial.NewID("content", "dashboard.gohtml"))

	recent := partial.NewID("recent", "recent.gohtml").SetDotSource(partial.DotFuncOf(func(ctx context.Context) (comment, error) {
		calls.Add(1)
		return comment{Text: "loaded"}, nil
	}))

	Set(root, "recent", recent, opts...)

	return root
}

func TestDeferredSlotRendersAPlaceholderThatLoadsIt(t *testing.T) {
	var calls atomic.Int32

	root := deferredPage(&calls, connector.NewHTMX4(nil), Deferred(), OmitOOB())

	page := httptest.NewRequest(http.MethodGet, "/dashboard?tab=1", nil)

	out, err := partial.RenderWithRequest(context.Background(), page, root)
	if err != nil {
		t.Fatal(err)
	}

	want := `<body><main><div id="recent" hx-get="/dashboard?tab=1" hx-swap="outerHTML" hx-target="this" hx-trigger="load" aria-busy="true"></div></main></body>`
	if string(out) != want {
		t.Fatalf("page = %q, want %q", out, want)
	}

	if calls.Load() != 0 {
		t.Fatal("the page computed the deferred slot's data")
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard?tab=1", nil)
	connector.SetRequestTarget(root.Connector(), req, "recent")

	rec := httptest.NewRecorder()
	if err := partial.Write(context.Background(), rec, req, root); err != nil {
		t.Fatal(err)
	}

	if body := rec.Body.String(); body != `<section id="recent">loaded</section>` {
		t.Fatalf("slot response = %q", body)
	}

	if calls.Load() != 1 {
		t.Fatalf("computed %d times, want once", calls.Load())
	}
}

func TestDeferredSlotWithAPlaceholderAndTurbo(t *testing.T) {
	var calls atomic.Int32

	root := deferredPage(&calls, connector.NewHTMX(nil), Placeholder(partial.NewID("loading", "loading.gohtml")))

	out, err := partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/d", nil), root)
	if err != nil {
		t.Fatal(err)
	}

	want := `<body><main><section id="recent" class="panel" hx-get="/d" hx-swap="outerHTML" hx-target="this" hx-trigger="load">Loading recent</section></main></body>`
	if string(out) != want {
		t.Fatalf("page = %q, want %q", out, want)
	}

	turbo := deferredPage(&calls, connector.NewTurbo(nil), Deferred())

	out, err = partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/d", nil), turbo)
	if err != nil ||
		!strings.Contains(string(out), `<turbo-frame id="recent" loading="lazy" src="/d" aria-busy="true"></turbo-frame>`) {
		t.Fatalf("turbo page = %q, %v", out, err)
	}
}

func TestDeferredSlotRendersInPlaceWithoutARequest(t *testing.T) {
	var calls atomic.Int32

	out, err := partial.Render(context.Background(), deferredPage(&calls, connector.NewHTMX4(nil), Deferred()))
	if err != nil || string(out) != `<body><main><section id="recent">loaded</section></main></body>` {
		t.Fatalf("Render() = %q, %v", out, err)
	}
}

func TestGlobSetsASlotPerFile(t *testing.T) {
	for _, cached := range []bool{false, true} {
		root := partial.NewID("root", "glob-layout.gohtml").
			SetFileSystem(dashboardFS()).
			SetConnector(connector.NewHTMX(nil)).
			UseTemplateCache(cached).
			Use(Stage()).
			SetContent(partial.NewID("content", "fragments/card.gohtml"))

		var names []string

		err := Glob(root, "regions/*", OOB(), Setup(func(name string, child *partial.Partial) {
			names = append(names, name)
			child.SetDot("dot of " + name)
		}))
		if err != nil {
			t.Fatal(err)
		}

		if strings.Join(names, ",") != "menu,notes,title" {
			t.Fatalf("cache %v: slots = %v", cached, names)
		}

		out, err := partial.Render(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}

		want := `<body><nav id="menu">dot of menu</nav><title id="title">dot of title</title><div id="card">card</div></body>`
		if string(out) != want {
			t.Fatalf("cache %v: page = %q, want %q", cached, out, want)
		}

		rec := httptest.NewRecorder()
		if err := partial.Write(context.Background(), rec, targetRequest("content"), root); err != nil {
			t.Fatal(err)
		}

		if body := rec.Body.String(); !strings.Contains(body, `<nav id="menu" hx-swap-oob="true">`) ||
			!strings.Contains(body, `<title id="title" hx-swap-oob="true">`) {
			t.Fatalf("cache %v: partial response = %q", cached, body)
		}
	}

	if FileName("regions/menu.gohtml") != "menu" || FileName("a/b.html.gohtml") != "b.html" {
		t.Fatal("FileName is wrong")
	}

	if err := Glob(partial.NewID("root").SetFileSystem(dashboardFS()), "[", OOB()); err == nil {
		t.Fatal("Glob() accepted a malformed pattern")
	}
}

func TestMissingAndRequiredSlots(t *testing.T) {
	render := func(file string) (string, error) {
		p := partial.NewID("page", file).SetFileSystem(dashboardFS()).Use(Stage())
		out, err := partial.Render(context.Background(), p)
		return string(out), err
	}

	if out, err := render("missing.gohtml"); err != nil || out != "<main></main>" {
		t.Fatalf("missing = %q, %v", out, err)
	}

	if _, err := render("required.gohtml"); err == nil || !strings.Contains(err.Error(), `slot "missing" is not set`) {
		t.Fatalf("required error = %v", err)
	}

	if out, err := render("optional.gohtml"); err != nil || out != "<main></main>" {
		t.Fatalf("optional = %q, %v", out, err)
	}
}
