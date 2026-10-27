package head

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/exp/slots"
)

// pageData is the dot of the layout and the head region of the tests.
type pageData struct {
	Title     string
	Languages []string
}

func pageFS() fstest.MapFS {
	return fstest.MapFS{
		"layout.gohtml": {Data: []byte(
			`<html><head><meta charset="utf-8">{{ slot "head" }}<link rel="stylesheet" href="/site.css"></head>` +
				`<body>{{ slot "nav" }}{{ content }}</body></html>`,
		)},
		"head.gohtml": {Data: []byte(
			`<link rel="canonical" href="/{{ .Title }}"><title>{{ .Title }}</title>` +
				`{{ range .Languages }}<link rel="alternate" hreflang="{{ . }}" href="/{{ . }}">{{ end }}` +
				`{{ partial runtime "note.gohtml" . }}`,
		)},
		"note.gohtml":    {Data: []byte(`<meta name="note" content="{{ .Title }}">`)},
		"nav.gohtml":     {Data: []byte(`<nav id="nav"{{ oobAttr }}>{{ .Title }}</nav>`)},
		"content.gohtml": {Data: []byte(`<main id="content">{{ .Title }}</main>`)},
	}
}

// newPage is a page with a head region, with conn as its connector.
func newPage(conn connector.Connector, data pageData) *partial.Partial {
	content := partial.NewID("content", "content.gohtml").SetDot(data)
	root := partial.NewID("root", "layout.gohtml").
		SetFileSystem(pageFS()).
		SetConnector(conn).
		UseTemplateCache(true).
		Use(slots.Stage()).
		SetDot(data).
		SetContent(content)

	region := partial.NewID("head", "head.gohtml").SetDot(data)

	slots.Set(root, "head", Region(region), slots.OOB())
	slots.Set(root, "nav", partial.NewID("nav", "nav.gohtml").SetDot(data), slots.OOB())

	return root
}

func contentRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "content")

	return req
}

func TestRegionOnAFullPage(t *testing.T) {
	page := newPage(connector.NewHTMX4(nil), pageData{Title: "a", Languages: []string{"en", "nl"}})

	out, err := partial.Render(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}

	want := `<html><head><meta charset="utf-8"><title>a</title>` +
		`<template data-partial-head="head"></template>` +
		`<link rel="canonical" href="/a">` +
		`<link rel="alternate" hreflang="en" href="/en"><link rel="alternate" hreflang="nl" href="/nl">` +
		`<meta name="note" content="a">` +
		`<template data-partial-head-end="head"></template>` +
		`<link rel="stylesheet" href="/site.css"></head>` +
		`<body><nav id="nav">a</nav><main id="content">a</main></body></html>`
	if string(out) != want {
		t.Fatalf("Render() =\n%s\nwant\n%s", out, want)
	}
}

func TestRegionOutOfBand(t *testing.T) {
	for _, conn := range []connector.Connector{connector.NewHTMX4(nil), connector.NewHTMX(nil)} {
		page := newPage(conn, pageData{Title: "b", Languages: []string{"en"}})

		out, err := partial.RenderWithRequest(context.Background(), contentRequest(), page)
		if err != nil {
			t.Fatal(err)
		}

		// The title on its own; the elements between the markers removed;
		// the new ones added before the end marker. The partial the region
		// renders is not wrapped again.
		head := `<title>b</title>` +
			`<div hx-swap-oob="delete:[data-partial-head=&#34;head&#34;]~:not([data-partial-head-end=&#34;head&#34;])` +
			`:not([data-partial-head-end=&#34;head&#34;]~*)"></div>` +
			`<div hx-swap-oob="beforebegin:[data-partial-head-end=&#34;head&#34;]">` +
			`<link rel="canonical" href="/b"><link rel="alternate" hreflang="en" href="/en">` +
			`<meta name="note" content="b"></div>`

		got := string(out)
		if !strings.Contains(got, head) || !strings.Contains(got, `<main id="content">b</main>`) ||
			!strings.Contains(got, `<nav id="nav" hx-swap-oob="true">b</nav>`) || strings.Contains(got, "<template") {
			t.Fatalf("%T: RenderWithRequest() =\n%s\nwant\n%s", conn, got, head)
		}
	}
}

func TestRegionOutOfBandWithAnotherConnector(t *testing.T) {
	page := newPage(connector.NewUnpoly(nil), pageData{Title: "c"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Up-Version", "3")
	req.Header.Set("X-Up-Target", "content")

	out, err := partial.RenderWithRequest(context.Background(), req, page)
	if err != nil {
		t.Fatal(err)
	}

	got := string(out)
	if !strings.Contains(got, `<link rel="canonical" href="/c"><title>c</title><meta name="note" content="c">`) ||
		strings.Contains(got, "hx-swap-oob") {
		t.Fatalf("RenderWithRequest() = %s", got)
	}
}

func TestRegionWithoutTitle(t *testing.T) {
	got := string(page("head", `<meta name="description" content="x">`))

	want := `<template data-partial-head="head"></template><meta name="description" content="x">` +
		`<template data-partial-head-end="head"></template>`
	if got != want {
		t.Fatalf("page() = %s", got)
	}
}

func TestMarkerValue(t *testing.T) {
	if got := markerValue(`page head"1`); got != "page_head_1" {
		t.Fatalf("markerValue() = %q", got)
	}
}

func TestCutTitle(t *testing.T) {
	cases := []struct {
		in, title, rest string
	}{
		{`<meta><TITLE lang="en">A &amp; B</TITLE><link>`, `<TITLE lang="en">A &amp; B</TITLE>`, `<meta><link>`},
		{`<!-- <title>x</title> --><title>y</title>`, `<title>y</title>`, `<!-- <title>x</title> -->`},
		{`<script>"<title>x</title>"</script><title>y</title>`, `<title>y</title>`, `<script>"<title>x</title>"</script>`},
		{`<titles>no</titles>`, ``, `<titles>no</titles>`},
		{`<title>open`, `<title>open`, ``},
		{`plain text`, ``, `plain text`},
	}

	for _, tc := range cases {
		title, rest := cutTitle(tc.in)
		if title != tc.title || rest != tc.rest {
			t.Fatalf("cutTitle(%q) = %q, %q", tc.in, title, rest)
		}
	}
}

func TestRegionNil(t *testing.T) {
	if Region(nil) != nil {
		t.Fatal("Region(nil) is not nil")
	}
}

// Rendered on its own, without a layout, the region still writes the
// markers.
func TestRegionAlone(t *testing.T) {
	fsys := fstest.MapFS{"head.gohtml": {Data: []byte(`<link rel="canonical" href="/">`)}}
	region := Region(partial.NewID("head", "head.gohtml").SetFileSystem(fsys))

	out, err := partial.Render(context.Background(), region)
	if err != nil {
		t.Fatal(err)
	}

	want := template.HTML(`<template data-partial-head="head"></template><link rel="canonical" href="/">` +
		`<template data-partial-head-end="head"></template>`)
	if out != want {
		t.Fatalf("Render() = %s", out)
	}
}
