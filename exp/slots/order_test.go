package slots

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// orderPage is a page whose regions directory holds names, set as
// out-of-band slots with Glob, and whose layout places them in reverse.
func orderPage(names []string) *partial.Partial {
	files := fstest.MapFS{
		"fragments/card.gohtml": {Data: []byte(`<div id="card">card</div>`)},
	}

	layout := "<body>"
	for i := len(names) - 1; i >= 0; i-- {
		layout += `{{ slot "` + names[i] + `" }}`
		files["regions/"+names[i]+".gohtml"] = &fstest.MapFile{Data: []byte(`<div id="` + names[i] + `"{{ oobAttr }}></div>`)}
	}

	files["layout.gohtml"] = &fstest.MapFile{Data: []byte(layout + "{{ content }}</body>")}

	root := partial.NewID("root", "layout.gohtml").
		SetFileSystem(files).
		SetConnector(connector.NewHTMX(nil)).
		Use(Stage()).
		SetContent(partial.NewID("content", "fragments/card.gohtml"))

	return root
}

// TestGlobOOBOrder checks that the slots Glob sets out of band are sent in
// the order of their file names, the same bytes on every render, and that
// a full page places them where its layout does, also on every render.
func TestGlobOOBOrder(t *testing.T) {
	names := []string{"breadcrumbs", "cart", "head", "menu", "notice", "title", "toast"}

	var firstPage, firstPartial string

	for i := range 100 {
		root := orderPage(names)
		if err := Glob(root, "regions/*.gohtml", OOB()); err != nil {
			t.Fatal(err)
		}

		page, err := partial.Render(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}

		rec := httptest.NewRecorder()
		if err := partial.Write(context.Background(), rec, targetRequest("content"), root); err != nil {
			t.Fatal(err)
		}

		if i == 0 {
			firstPage = string(page)
			firstPartial = rec.Body.String()
		}

		if string(page) != firstPage {
			t.Fatalf("page changed between renders:\n%s\n%s", firstPage, page)
		}

		if rec.Body.String() != firstPartial {
			t.Fatalf("partial response changed between renders:\n%s\n%s", firstPartial, rec.Body.String())
		}
	}

	want := `<div id="card">card</div>`
	for _, name := range names {
		want += `<div id="` + name + `" hx-swap-oob="true"></div>`
	}

	if firstPartial != want {
		t.Fatalf("partial response = %s\nwant %s", firstPartial, want)
	}

	if !strings.HasPrefix(firstPage, `<body><div id="toast"></div><div id="title"></div>`) {
		t.Fatalf("page = %s", firstPage)
	}
}

// TestSetOOBOrder checks that out-of-band slots set one by one are sent in
// the order Set registered them, and that setting one again keeps its
// place.
func TestSetOOBOrder(t *testing.T) {
	names := []string{"title", "menu", "cart", "head"}

	root := orderPage(names)
	for _, name := range names {
		Set(root, name, partial.NewID(name, "regions/"+name+".gohtml"), OOB())
	}

	Set(root, "menu", partial.NewID("menu", "regions/menu.gohtml"), OOB())

	for range 100 {
		rec := httptest.NewRecorder()
		if err := partial.Write(context.Background(), rec, targetRequest("content"), root); err != nil {
			t.Fatal(err)
		}

		want := `<div id="card">card</div>` +
			`<div id="title" hx-swap-oob="true"></div>` +
			`<div id="menu" hx-swap-oob="true"></div>` +
			`<div id="cart" hx-swap-oob="true"></div>` +
			`<div id="head" hx-swap-oob="true"></div>`
		if rec.Body.String() != want {
			t.Fatalf("partial response = %s\nwant %s", rec.Body.String(), want)
		}
	}
}
