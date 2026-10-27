package partial

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-partial/connector"
)

// oobMenu is a menu region whose items carry IDs, nested two levels deep.
const oobMenu = `<nav id="menu"{{oobAttr}}>` +
	`<div id="item-a" class="group"><a id="link-a" href="/a">A</a><div><a id="link-a1" href="/a/1">A1</a></div></div>` +
	`<a id="link-b" href="/b" aria-current="page">B</a>` +
	`<img id="logo" src="/logo.png">` +
	`</nav>`

// renderOOBElements writes a page with the menu region out of band,
// sending ids of it, for an htmx 4 navigation (partial) or a full page.
func renderOOBElements(t *testing.T, menu string, partial bool, ids ...string) string {
	t.Helper()

	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<!doctype html>{{content}}`)},
		"content.gohtml": {Data: []byte(`<main id="content">Page</main>`)},
		"menu.gohtml":    {Data: []byte(menu)},
	}

	root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(connector.NewHTMX4(nil))
	root.SetContent(NewID("content", "content.gohtml"))
	root.WithOOB(NewID("menu", "menu.gohtml").SetAlwaysSwapOOB(true).SetOOBElements(ids...))

	r := httptest.NewRequest("GET", "/", nil)
	if partial {
		r.Header.Set("HX-Request", "true")
		r.Header.Set("HX-Request-Type", "partial")
		r.Header.Set("HX-Target", "main#content")
	}

	w := httptest.NewRecorder()

	if err := Write(context.Background(), w, r, root); err != nil {
		t.Fatal(err)
	}

	return w.Body.String()
}

// TestOOBElements checks that a region with OOB elements sends only those
// elements of it, each out of band, along with the content.
func TestOOBElements(t *testing.T) {
	body := renderOOBElements(t, oobMenu, true, "link-a1", "link-b", "link-c", "logo")

	want := `<main id="content">Page</main>` +
		`<a hx-swap-oob="true" id="link-a1" href="/a/1">A1</a>` +
		`<a hx-swap-oob="true" id="link-b" href="/b" aria-current="page">B</a>` +
		`<img hx-swap-oob="true" id="logo" src="/logo.png">`
	if body != want {
		t.Fatalf("got  %s\nwant %s", body, want)
	}
}

// TestOOBElementsNested checks that an element inside another one that
// goes along is not sent twice.
func TestOOBElementsNested(t *testing.T) {
	body := renderOOBElements(t, oobMenu, true, "link-a", "item-a", "link-a1")

	want := `<main id="content">Page</main>` +
		`<div hx-swap-oob="true" id="item-a" class="group"><a id="link-a" href="/a">A</a><div><a id="link-a1" href="/a/1">A1</a></div></div>`
	if body != want {
		t.Fatalf("got  %s\nwant %s", body, want)
	}
}

// TestOOBElementsFallBack checks that the whole region goes when none of
// the IDs is in it, or an element has no end, and on a full page.
func TestOOBElementsFallBack(t *testing.T) {
	whole := `<main id="content">Page</main>` + strings.Replace(oobMenu, "{{oobAttr}}", ` hx-swap-oob="true"`, 1)

	if body := renderOOBElements(t, oobMenu, true, "link-c", "missing"); body != whole {
		t.Fatalf("no ID found: %s", body)
	}

	if body := renderOOBElements(t, oobMenu, true); body != whole {
		t.Fatalf("no IDs: %s", body)
	}

	unclosed := `<ul id="menu"{{oobAttr}}><li id="a">A<li id="b">B</ul>`
	want := `<ul id="menu" hx-swap-oob="true"><li id="a">A<li id="b">B</ul>`

	if body := renderOOBElements(t, unclosed, true, "a"); !strings.Contains(body, want) {
		t.Fatalf("unclosed element: %s", body)
	}

	page := renderOOBElements(t, `<!doctype html><body>{{content}}</body>`, false, "link-b")
	if !strings.Contains(page, `<main id="content">Page</main>`) {
		t.Fatalf("full page: %s", page)
	}
}

// oobElementsFuncRoot is a page whose menu region chooses its elements
// with fn.
func oobElementsFuncRoot(conn connector.Connector, fn func(r *http.Request) OOBElements) *Partial {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<!doctype html><body>{{content}}</body>`)},
		"content.gohtml": {Data: []byte(`<main id="content">Page</main>`)},
		"menu.gohtml":    {Data: []byte(oobMenu)},
	}

	root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(conn)
	root.SetContent(NewID("content", "content.gohtml"))
	root.WithOOB(NewID("menu", "menu.gohtml").SetAlwaysSwapOOB(true).SetOOBElementsFunc(fn))

	return root
}

// TestOOBElementsFunc checks that the function of SetOOBElementsFunc
// chooses the elements of a partial response, and that its Vary goes to
// the response; and that it is not called for a full page, whose Vary
// does not name its headers.
func TestOOBElementsFunc(t *testing.T) {
	calls := 0
	conn := connector.NewHTMX4(nil)

	root := oobElementsFuncRoot(conn, func(r *http.Request) OOBElements {
		calls++

		ids := []string{"link-b"}
		if connector.CurrentURL(conn, r) == nil {
			ids = nil
		}

		return OOBElements{IDs: ids, Vary: []string{connector.CurrentURLHeader(conn), ""}}
	})

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Request-Type", "partial")
	r.Header.Set("HX-Target", "main#content")
	r.Header.Set("HX-Current-URL", "http://example.com/a")

	w := httptest.NewRecorder()
	if err := Write(context.Background(), w, r, root); err != nil {
		t.Fatal(err)
	}

	want := `<main id="content">Page</main><a hx-swap-oob="true" id="link-b" href="/b" aria-current="page">B</a>`
	if body := w.Body.String(); body != want {
		t.Fatalf("got  %s\nwant %s", body, want)
	}

	if !varyLists(w.Header().Values("Vary"), "HX-Current-URL") || calls != 1 {
		t.Fatalf("partial response: Vary = %v, calls = %d", w.Header().Values("Vary"), calls)
	}

	full := httptest.NewRecorder()
	if err := Write(context.Background(), full, httptest.NewRequest("GET", "/", nil), root); err != nil {
		t.Fatal(err)
	}

	if calls != 1 || varyLists(full.Header().Values("Vary"), "HX-Current-URL") {
		t.Fatalf("full page: Vary = %v, calls = %d", full.Header().Values("Vary"), calls)
	}

	// Without IDs, the whole region goes, and the Vary still does.
	r.Header.Del("HX-Current-URL")

	whole := httptest.NewRecorder()
	if err := Write(context.Background(), whole, r, root); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(whole.Body.String(), `<nav id="menu" hx-swap-oob="true">`) ||
		!varyLists(whole.Header().Values("Vary"), "HX-Current-URL") {
		t.Fatalf("no IDs: Vary = %v, body %s", whole.Header().Values("Vary"), whole.Body.String())
	}
}

// TestOOBElementsFuncReplaces checks that SetOOBElements and
// SetOOBElementsFunc replace each other.
func TestOOBElementsFuncReplaces(t *testing.T) {
	menu := NewID("menu", "menu.gohtml").SetOOBElements("a").SetOOBElementsFunc(func(*http.Request) OOBElements {
		return OOBElements{IDs: []string{"b"}}
	})

	if out, _ := menu.oobOutput(nil, `<p id="a">a</p><p id="b">b</p>`); out != `<p hx-swap-oob="true" id="b">b</p>` {
		t.Fatalf("func after IDs: %s", out)
	}

	menu.SetOOBElements("a")

	if out, _ := menu.oobOutput(nil, `<p id="a">a</p><p id="b">b</p>`); out != `<p hx-swap-oob="true" id="a">a</p>` {
		t.Fatalf("IDs after func: %s", out)
	}
}

// TestOOBAttrAsksTheConnector checks that oobAttr writes the connector's
// out-of-band attribute: up-hungry for Unpoly, hx-swap-oob with the swap
// given for htmx.
func TestOOBAttrAsksTheConnector(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`{{content}}`)},
		"content.gohtml": {Data: []byte(`<main id="content">Page</main>`)},
		"notice.gohtml":  {Data: []byte(`<aside id="notice"{{oobAttr}}>N</aside><ul id="list"{{oobAttr "beforeend"}}></ul>`)},
	}

	cases := []struct {
		name string
		conn connector.Connector
		set  func(r *http.Request)
		want string
	}{
		{
			name: "htmx",
			conn: connector.NewHTMX(nil),
			set: func(r *http.Request) {
				r.Header.Set("HX-Request", "true")
				r.Header.Set("HX-Target", "content")
			},
			want: `<aside id="notice" hx-swap-oob="true">N</aside><ul id="list" hx-swap-oob="beforeend"></ul>`,
		},
		{
			name: "unpoly",
			conn: connector.NewUnpoly(nil),
			set: func(r *http.Request) {
				r.Header.Set("X-Up-Version", "3")
				r.Header.Set("X-Up-Target", "content")
			},
			want: `<aside id="notice" up-hungry="">N</aside><ul id="list" up-hungry=""></ul>`,
		},
	}

	for _, test := range cases {
		root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(test.conn)
		root.SetContent(NewID("content", "content.gohtml"))
		root.WithOOB(NewID("notice", "notice.gohtml").SetAlwaysSwapOOB(true))

		r := httptest.NewRequest("GET", "/", nil)
		test.set(r)

		w := httptest.NewRecorder()
		if err := Write(context.Background(), w, r, root); err != nil {
			t.Fatal(err)
		}

		if body := w.Body.String(); !strings.HasSuffix(body, test.want) {
			t.Errorf("%s: got %s, want it to end with %s", test.name, body, test.want)
		}
	}
}

// TestSelectElements checks the markup selectElements reads: quoted,
// single-quoted and unquoted IDs, upper-case names, self-closing tags,
// nested elements of the same name and the content of a <script>.
func TestSelectElements(t *testing.T) {
	attrs := template.HTMLAttr(` x-oob="1"`)

	cases := []struct {
		name, html string
		ids        []string
		want       string
	}{
		{
			name: "quotes",
			html: `<p id='a'>1</p><P ID=b>2</P><span data-x="id=&quot;c&quot;" id = "c">3</span>`,
			ids:  []string{"a", "b", "c"},
			want: `<p x-oob="1" id='a'>1</p><P x-oob="1" ID=b>2</P><span x-oob="1" data-x="id=&quot;c&quot;" id = "c">3</span>`,
		},
		{
			name: "nested same name",
			html: `<div id="a"><div>x</div><div><div/>y</div></div><div id="b">z</div>`,
			ids:  []string{"a"},
			want: `<div x-oob="1" id="a"><div>x</div><div><div/>y</div></div>`,
		},
		{
			name: "script content",
			html: `<script id="s">if (a < b) { x = "</div>" }</script><div id="d">d</div>`,
			ids:  []string{"s", "d"},
			want: `<script x-oob="1" id="s">if (a < b) { x = "</div>" }</script><div x-oob="1" id="d">d</div>`,
		},
		{
			name: "comment",
			html: `<!-- <a id="a">not an element</a> --><a id="a">A</a>`,
			ids:  []string{"a"},
			want: `<a x-oob="1" id="a">A</a>`,
		},
		{
			name: "self-closing",
			html: `<svg><use id="u" href="#i"/></svg>`,
			ids:  []string{"u"},
			want: `<use x-oob="1" id="u" href="#i"/>`,
		},
		{
			name: "unclosed tag",
			html: `<div id="a">x</div><div id="b" class="`,
			ids:  []string{"a"},
			want: `<div x-oob="1" id="a">x</div>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(selectElements(template.HTML(tc.html), tc.ids, attrs)); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestAddVary checks that AddVary adds names the Vary header does not list
// yet, in any case, leaves empty names out, keeps "*", and takes a nil
// header.
func TestAddVary(t *testing.T) {
	header := http.Header{}
	header.Add("Vary", "Accept-Encoding, hx-request")

	AddVary(header, "HX-Request", "", "HX-Current-URL", "HX-Current-URL")

	if got := strings.Join(header.Values("Vary"), "|"); got != "Accept-Encoding, hx-request|HX-Current-URL" {
		t.Fatalf("Vary = %q", got)
	}

	all := http.Header{"Vary": {"*"}}
	AddVary(all, "HX-Current-URL")

	if got := all.Values("Vary"); len(got) != 1 {
		t.Fatalf("Vary * = %v", got)
	}

	AddVary(nil, "HX-Current-URL")
}
