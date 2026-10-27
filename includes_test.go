package partial

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func includesFS() fstest.MapFS {
	return fstest.MapFS{
		"layout.gohtml": {Data: []byte(`<main>{{ template "badge" "layout" }}{{ content }}</main>`)},
		"content.gohtml": {
			Data: []byte(`<p>{{ template "badge" "content" }}{{ partial runtime "partials/card.gohtml" }}</p>`),
		},
		"partials/badge.gohtml":  {Data: []byte(`{{ define "badge" }}<b>{{ . }}</b>{{ end }}`)},
		"partials/card.gohtml":   {Data: []byte(`<div>{{ template "badge" "card" }}</div>`)},
		"partials/nested/x.html": {Data: []byte(`x`)},
	}
}

func TestIncludesReachChildrenContentAndPartials(t *testing.T) {
	for _, cached := range []bool{false, true} {
		root := NewID("root", "layout.gohtml").
			SetFileSystem(includesFS()).
			UseTemplateCache(cached).
			SetIncludes("partials/*.gohtml").
			SetContent(NewID("content", "content.gohtml"))

		out, err := Render(context.Background(), root)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cached, err)
		}

		want := `<main><b>layout</b><p><b>content</b><div><b>card</b></div></p></main>`
		if string(out) != want {
			t.Fatalf("cache %v: output = %q, want %q", cached, out, want)
		}
	}
}

func TestIncludesReachPartialsRenderedUnderAParent(t *testing.T) {
	root := NewID("root").
		SetFileSystem(includesFS()).
		UseTemplateCache(true).
		SetIncludes("partials/*.gohtml")

	card := NewID("card", "partials/card.gohtml").SetParent(root)

	out, err := RenderSelf(context.Background(), nil, card)
	if err != nil {
		t.Fatalf("RenderSelf() error = %v", err)
	}

	if string(out) != `<div><b>card</b></div>` {
		t.Fatalf("output = %q", out)
	}
}

func TestIncludesAreParsedOnceAndMergeWithAncestors(t *testing.T) {
	root := NewID("root", "layout.gohtml").
		SetFileSystem(includesFS()).
		SetIncludes("partials/*.gohtml", "partials/badge.gohtml", "/partials/*.gohtml")
	child := NewID("child", "partials/card.gohtml").SetIncludes("partials/badge.gohtml", "content.gohtml")
	root.With(child)

	patterns := []string{"partials/*.gohtml", "partials/badge.gohtml", "content.gohtml"}
	if got := child.getIncludePatterns(); !reflect.DeepEqual(got, patterns) {
		t.Fatalf("getIncludePatterns() = %v", got)
	}

	templates, err := child.renderTemplates(false)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"partials/card.gohtml", "partials/badge.gohtml", "content.gohtml"}
	if !reflect.DeepEqual(templates, want) {
		t.Fatalf("renderTemplates() = %v, want %v", templates, want)
	}

	if root.SetIncludes(); root.getIncludePatterns() != nil {
		t.Fatalf("SetIncludes() without patterns should remove them, got %v", root.getIncludePatterns())
	}
}

func TestIncludesWithABadPatternFailTheRender(t *testing.T) {
	root := NewID("root", "layout.gohtml").SetFileSystem(includesFS()).SetIncludes("partials/[")

	if _, err := Render(context.Background(), root); err == nil || !strings.Contains(err.Error(), "includes") {
		t.Fatalf("Render() error = %v, want an include error", err)
	}
}

func TestIncludesAreResolvedAgainWithoutTheCache(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml": {Data: []byte(`{{ template "extra" }}`)},
	}

	root := NewID("root", "page.gohtml").SetFileSystem(files).SetIncludes("partials/*.gohtml")
	if _, err := Render(context.Background(), root); err == nil {
		t.Fatal("Render() should fail without the included template")
	}

	files["partials/extra.gohtml"] = &fstest.MapFile{Data: []byte(`{{ define "extra" }}extra{{ end }}`)}

	out, err := Render(context.Background(), root)
	if err != nil || string(out) != "extra" {
		t.Fatalf("Render() = %q, %v; want the new include", out, err)
	}
}

func TestTemplateCacheIsInherited(t *testing.T) {
	root := NewID("root").UseTemplateCache(true)
	child := NewID("child")
	root.With(child)
	under := NewID("under").SetParent(root)
	off := NewID("off").UseTemplateCache(false)
	root.With(off)
	below := NewID("below")
	off.With(below)

	cases := map[string]struct {
		p    *Partial
		want bool
	}{
		"root":           {root, true},
		"child":          {child, true},
		"set parent":     {under, true},
		"own setting":    {off, false},
		"nearest setter": {below, false},
		"no setting":     {NewID("alone"), false},
	}
	for name, tc := range cases {
		if got := tc.p.usesCache(); got != tc.want {
			t.Fatalf("%s: usesCache() = %v, want %v", name, got, tc.want)
		}
	}
}

func TestTemplateCacheInheritedKeepsAParse(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<main>{{ content }}</main>`)},
		"content.gohtml": {Data: []byte(`one`)},
	}

	root := NewID("root", "layout.gohtml").SetFileSystem(files).UseTemplateCache(true)
	root.SetContent(NewID("content", "content.gohtml"))

	if out, err := Render(context.Background(), root); err != nil || string(out) != "<main>one</main>" {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	files["content.gohtml"] = &fstest.MapFile{Data: []byte(`two`)}

	if out, err := Render(context.Background(), root); err != nil || string(out) != "<main>one</main>" {
		t.Fatalf("Render() = %q, %v; the content should render from the cache it inherits", out, err)
	}
}

func TestFirstExisting(t *testing.T) {
	for _, cached := range []bool{false, true} {
		files := fstest.MapFS{
			"pages/page.gohtml": {Data: []byte(`page`)},
			"pages/news.gohtml": {Data: []byte(`news`)},
			"pages/dir/x.html":  {Data: []byte(`x`)},
		}
		root := NewID("root").SetFileSystem(files).UseTemplateCache(cached)

		got, err := root.FirstExisting("pages/home.gohtml", "/pages/news.gohtml", "pages/page.gohtml")
		if err != nil || got != "pages/news.gohtml" {
			t.Fatalf("cache %v: FirstExisting() = %q, %v", cached, got, err)
		}

		if _, err := root.FirstExisting("pages/dir", "pages/none.gohtml"); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("cache %v: FirstExisting() error = %v, want fs.ErrNotExist", cached, err)
		}

		files["pages/home.gohtml"] = &fstest.MapFile{Data: []byte(`home`)}

		got, _ = root.FirstExisting("pages/home.gohtml", "pages/page.gohtml")

		want := "pages/home.gohtml"
		if cached {
			// The cache remembers the file was missing.
			want = "pages/page.gohtml"
		}

		if got != want {
			t.Fatalf("cache %v: FirstExisting() = %q, want %q", cached, got, want)
		}
	}
}

func TestParentReturnsWhereThePartialRenders(t *testing.T) {
	root := NewID("root")
	child := NewID("child")
	root.With(child)
	under := NewID("under").SetParent(root)

	if child.Parent() != root || under.Parent() != root || root.Parent() != nil {
		t.Fatal("Parent() does not return the partial's parent")
	}
}

func TestWriteDefaultsToHTML(t *testing.T) {
	files := fstest.MapFS{"page.gohtml": {Data: []byte(`plain text`)}}

	rec := httptest.NewRecorder()
	if err := Write(
		context.Background(),
		rec,
		httptest.NewRequest(http.MethodGet, "/", nil),
		NewID("page", "page.gohtml").SetFileSystem(files),
	); err != nil {
		t.Fatal(err)
	}

	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}

	rec = httptest.NewRecorder()

	page := NewID("page", "page.gohtml").SetFileSystem(files).SetResponseHeaders(map[string]string{"Content-Type": "text/plain"})
	if err := Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page); err != nil {
		t.Fatal(err)
	}

	if got := rec.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want the partial's own", got)
	}
}
