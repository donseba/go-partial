package partial

import (
	"context"
	"html/template"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-partial/connector"
)

// alwaysSwapTree is a layout with a content partial and a card that a
// render stage resolves. The content has a region that always swaps and
// one without the setting, the card a region that always swaps, and the
// layout a region of its own. layout and content are the templates of the
// layout and the content; {{ place runtime "id" }} places a region in
// them, as {{ slot }} of exp/slots does.
func alwaysSwapTree(layout, content string) *Partial {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(layout)},
		"content.gohtml": {Data: []byte(content)},
		"card.gohtml":    {Data: []byte(`<div id="card">Card</div>`)},
		"notice.gohtml":  {Data: []byte(`<aside id="notice"{{ oobAttr }}>Notice</aside>`)},
		"quiet.gohtml":   {Data: []byte(`<p id="quiet"{{ oobAttr }}>Quiet</p>`)},
		"count.gohtml":   {Data: []byte(`<span id="count"{{ oobAttr }}>1</span>`)},
		"header.gohtml":  {Data: []byte(`<header id="header"{{ oobAttr }}>Header</header>`)},
	}

	regions := map[string]*Partial{
		"notice": NewID("notice", "notice.gohtml").SetAlwaysSwapOOB(true),
		"quiet":  NewID("quiet", "quiet.gohtml"),
		"count":  NewID("count", "count.gohtml").SetAlwaysSwapOOB(true),
		"header": NewID("header", "header.gohtml").SetAlwaysSwapOOB(true),
	}

	page := NewID("content", "content.gohtml")
	page.WithOOB(regions["notice"])
	page.WithOOB(regions["quiet"])

	card := NewID("card", "card.gohtml")
	card.WithOOB(regions["count"])

	root := NewID("shell", "layout.gohtml").
		SetFileSystem(files).
		SetConnector(connector.NewHTMX(nil)).
		SetFunc(template.FuncMap{
			"place": func(runtime *Runtime, id string) (template.HTML, error) {
				return runtime.RenderPartial(regions[id])
			},
		}).
		Use(RenderStageHooks{
			PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
				if ctx.Kind == RenderKindTarget && ctx.Name == "card" {
					ctx.Partial = card
				}

				return ctx, nil
			},
		})

	root.SetContent(page)
	root.WithOOB(regions["header"])

	return root
}

// TestAlwaysSwapOOBGoesWithItsOwner checks that a region that always swaps
// goes out of band also with a partial request for the partial it is
// registered on, a registered one or one a render stage resolves, before
// the regions of the ancestors; that it does not when that partial's output
// places it already; and that a region without the setting goes only with
// requests for the partial's descendants.
func TestAlwaysSwapOOBGoesWithItsOwner(t *testing.T) {
	const (
		layout     = `<body>{{ content }}</body>`
		content    = `<main id="content">Page</main>`
		notice     = `<aside id="notice" hx-swap-oob="true">Notice</aside>`
		count      = `<span id="count" hx-swap-oob="true">1</span>`
		header     = `<header id="header" hx-swap-oob="true">Header</header>`
		withHeader = `<body>{{ place runtime "header" }}{{ content }}</body>`
		withNotice = `<main id="content">{{ place runtime "notice" }}</main>`
	)

	cases := []struct {
		name, layout, content, target, want string
	}{
		{
			name:    "registered target",
			layout:  layout,
			content: content,
			target:  "content",
			want:    content + notice + header,
		},
		{
			name:    "target that places its region",
			layout:  layout,
			content: withNotice,
			target:  "content",
			want:    `<main id="content"><aside id="notice">Notice</aside></main>` + header,
		},
		{
			name:    "resolved target",
			layout:  layout,
			content: content,
			target:  "card",
			want:    `<div id="card">Card</div>` + count + header,
		},
		{
			name:    "the layout",
			layout:  layout,
			content: content,
			target:  "shell",
			want:    `<body>` + content + `</body>` + header,
		},
		{
			name:    "the layout, which places its region",
			layout:  withHeader,
			content: content,
			target:  "shell",
			want:    `<body><header id="header">Header</header>` + content + `</body>`,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("HX-Request", "true")
			r.Header.Set("HX-Target", test.target)

			w := httptest.NewRecorder()
			if err := Write(context.Background(), w, r, alwaysSwapTree(test.layout, test.content)); err != nil {
				t.Fatal(err)
			}

			if got := w.Body.String(); got != test.want {
				t.Fatalf("got  %s\nwant %s", got, test.want)
			}
		})
	}
}
