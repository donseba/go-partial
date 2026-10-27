package partial

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
)

type sourcePage struct {
	Title string
}

func TestSourcesIsAValidFileSystem(t *testing.T) {
	sources := Sources{
		"page.gohtml":           `<h1>{{ .Title }}</h1>`,
		"partials/card.gohtml":  `<div>{{ . }}</div>`,
		"partials/inner/x.html": `x`,
	}

	if err := fstest.TestFS(sources, "page.gohtml", "partials/card.gohtml", "partials/inner/x.html"); err != nil {
		t.Fatal(err)
	}

	matches, err := fs.Glob(sources, "partials/*.gohtml")
	if err != nil || len(matches) != 1 || matches[0] != "partials/card.gohtml" {
		t.Fatalf("Glob() = %v, %v", matches, err)
	}
}

func TestOverlayIsAValidFileSystem(t *testing.T) {
	theme := fstest.MapFS{
		"page.gohtml":          &fstest.MapFile{Data: []byte(`theme`)},
		"partials/nav.gohtml":  &fstest.MapFile{Data: []byte(`nav`)},
		"partials/card.gohtml": &fstest.MapFile{Data: []byte(`theme card`)},
	}
	defaults := Sources{
		"partials/card.gohtml":  `default card`,
		"partials/share.gohtml": `share`,
	}
	overlay := Overlay(theme, defaults)

	if err := fstest.TestFS(
		overlay,
		"page.gohtml",
		"partials/nav.gohtml",
		"partials/card.gohtml",
		"partials/share.gohtml",
	); err != nil {
		t.Fatal(err)
	}

	data, err := fs.ReadFile(overlay, "partials/card.gohtml")
	if err != nil || string(data) != "theme card" {
		t.Fatalf("ReadFile() = %q, %v; want the theme's file", data, err)
	}

	matches, err := fs.Glob(overlay, "partials/*.gohtml")
	if err != nil || len(matches) != 3 {
		t.Fatalf("Glob() = %v, %v; want the files of both layers", matches, err)
	}
}

func TestLayersFlattenAnOverlay(t *testing.T) {
	theme := fstest.MapFS{"news/post.gohtml": &fstest.MapFile{Data: []byte(`theme`)}}
	base := fstest.MapFS{"publishing/post.gohtml": &fstest.MapFile{Data: []byte(`base`)}}
	defaults := Sources{"publishing/post.gohtml": `default`}

	if layers := Layers(theme); len(layers) != 1 {
		t.Fatalf("Layers(dir) = %v; want the file system alone", layers)
	}

	if layers := Layers(nil); layers != nil {
		t.Fatalf("Layers(nil) = %v", layers)
	}

	flat := Overlay(append(Layers(Overlay(theme, base)), defaults)...)

	if layers := Layers(flat); len(layers) != 3 {
		t.Fatalf("Layers() = %d layers; want 3", len(layers))
	}

	// Every candidate is tried in a layer before the next one, so the
	// theme's news/post.gohtml comes before base's publishing/post.gohtml,
	// which it would not with the overlay nested as one layer.
	file, err := New().SetFileSystem(flat).FirstExistingIn(nil, "publishing/post.gohtml", "news/post.gohtml")
	if err != nil || file != "news/post.gohtml" {
		t.Fatalf("FirstExistingIn() = %q, %v; want the theme's file", file, err)
	}
}

func TestRenderFromSourcesWithPartialsModelsAndCache(t *testing.T) {
	sources := Sources{
		"share.gohtml": `{{/* @model Page github.com/donseba/go-partial.sourcePage */}}` +
			`<nav>{{ Page.Title }}:{{ template "partials/link.gohtml" . }}{{ partial runtime "partials/icon.gohtml" "x" }}</nav>`,
		"partials/link.gohtml": `<a>{{ .URL }}</a>`,
		"partials/icon.gohtml": `<i>{{ . }}</i>`,
	}

	for _, cache := range []bool{false, true} {
		root := New().SetFileSystem(sources).UseTemplateCache(cache).SetModel(sourcePage{Title: "Hello"})

		for range 2 {
			share := NewID("share", "share.gohtml", "partials/link.gohtml").
				UseTemplateCache(cache).
				SetDot(map[string]string{"URL": "/a"})
			root.With(share)

			out, err := Render(context.Background(), share)
			if err != nil {
				t.Fatalf("cache %v: Render() error = %v", cache, err)
			}

			if out != `<nav>Hello:<a>/a</a><i>x</i></nav>` {
				t.Fatalf("cache %v: Render() = %q", cache, out)
			}
		}
	}
}

func TestOverlayLetsATemplateReplaceADefault(t *testing.T) {
	defaults := Sources{"site/share.gohtml": `default {{ . }}`}
	theme := fstest.MapFS{"site/share.gohtml": &fstest.MapFile{Data: []byte(`theme {{ . }}`)}}
	bare := fstest.MapFS{}

	render := func(fsys fs.FS) string {
		p := NewID("share", "site/share.gohtml").SetFileSystem(fsys).UseTemplateCache(true).SetDot("x")

		out, err := Render(context.Background(), p)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		return string(out)
	}

	// Overlays are built per render here, as an application may; those of
	// the same layers share their cache entry, others do not.
	for range 2 {
		if got := render(Overlay(theme, defaults)); got != "theme x" {
			t.Fatalf("themed render = %q", got)
		}

		if got := render(Overlay(bare, defaults)); got != "default x" {
			t.Fatalf("default render = %q", got)
		}
	}

	first, unique := fileSystemKey(Overlay(theme, defaults))

	second, _ := fileSystemKey(Overlay(theme, defaults))
	if !unique || first != second {
		t.Fatalf("overlay keys = %q, %q (unique %v), want one key", first, second, unique)
	}
}

// mapHolder is a file system that cannot be compared, so its cache key does
// not tell two of them apart.
type mapHolder struct {
	files fstest.MapFS
}

func (m mapHolder) Open(name string) (fs.File, error) {
	return m.files.Open(name)
}

func TestTemplateCacheKeepsFileSystemsWithoutIdentityApart(t *testing.T) {
	one := mapHolder{files: fstest.MapFS{"page.gohtml": &fstest.MapFile{Data: []byte(`one`)}}}
	two := mapHolder{files: fstest.MapFS{"page.gohtml": &fstest.MapFile{Data: []byte(`two`)}}}

	for _, fsys := range []mapHolder{one, two, one} {
		want := string(fsys.files["page.gohtml"].Data)

		out, err := Render(context.Background(), New("page.gohtml").SetFileSystem(fsys).UseTemplateCache(true))
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		if string(out) != want {
			t.Fatalf("Render() = %q, want %q", out, want)
		}
	}
}

func TestSourceCanRenderABlockAFileDefines(t *testing.T) {
	entry := Sources{"mail-html.gohtml": `{{ template "html" . }}`}
	theme := Sources{
		"mail/confirmation.gohtml": `{{ define "subject" }}Hi {{ . }}{{ end }}{{ define "html" }}<p>Hello {{ . }}</p>{{ end }}`,
	}

	for _, cache := range []bool{false, true} {
		p := NewID("mail", "mail-html.gohtml", "mail/confirmation.gohtml").
			SetFileSystem(Overlay(entry, theme)).
			UseTemplateCache(cache).
			SetDot("<Ada>")

		out, err := Render(context.Background(), p)
		if err != nil || out != "<p>Hello &lt;Ada&gt;</p>" {
			t.Fatalf("cache %v: Render() = %q, %v", cache, out, err)
		}
	}
}
