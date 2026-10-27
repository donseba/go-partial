package partial

import (
	"context"
	"errors"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-partial/connector"
)

type lazyNav struct {
	Items []string
}

func lazyModelFS() fstest.MapFS {
	nav := `{{/*
@model Nav github.com/donseba/go-partial.lazyNav
*/}}`

	return fstest.MapFS{
		"layout.gohtml": {
			Data: []byte(nav + `<body>{{ len Nav.Items }}{{ content }}{{ partial runtime "sidebar.gohtml" }}</body>`),
		},
		"content.gohtml": {Data: []byte(nav + `<main id="content">{{ index Nav.Items 0 }}</main>`)},
		"sidebar.gohtml": {Data: []byte(nav + `<nav>{{ range Nav.Items }}{{ . }};{{ end }}</nav>`)},
		"header.gohtml":  {Data: []byte(nav + `<header id="header" {{ oobAttr }}>{{ len Nav.Items }}</header>`)},
		"plain.gohtml":   {Data: []byte(`<main id="content">plain</main>`)},
	}
}

func TestLazyModelIsComputedOncePerRenderTree(t *testing.T) {
	for _, cached := range []bool{false, true} {
		var calls atomic.Int32

		root := NewID("root", "layout.gohtml").
			SetFileSystem(lazyModelFS()).
			UseTemplateCache(cached).
			SetConnector(connector.NewHTMX(nil)).
			SetContent(NewID("content", "content.gohtml"))
		root.SetModel(ModelFuncOf(func(ctx context.Context) (lazyNav, error) {
			calls.Add(1)
			return lazyNav{Items: []string{"a", "b"}}, nil
		}))
		root.WithOOB(NewID("header", "header.gohtml"))

		out, err := Render(context.Background(), root)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cached, err)
		}

		if want := `<body>2<main id="content">a</main><nav>a;b;</nav></body>`; string(out) != want {
			t.Fatalf("cache %v: output = %q, want %q", cached, out, want)
		}

		if calls.Load() != 1 {
			t.Fatalf("cache %v: computed %d times on a page, want once", cached, calls.Load())
		}

		// A partial request renders the content and the header region,
		// apart: they still share the value.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "content")

		calls.Store(0)

		rec := httptest.NewRecorder()
		if err := Write(context.Background(), rec, req, root); err != nil {
			t.Fatalf("cache %v: Write() error = %v", cached, err)
		}

		if body := rec.Body.String(); !strings.Contains(body, `<main id="content">a</main>`) ||
			!strings.Contains(body, `hx-swap-oob="true">2</header>`) {
			t.Fatalf("cache %v: partial response = %q", cached, body)
		}

		if calls.Load() != 1 {
			t.Fatalf("cache %v: computed %d times for a partial response, want once", cached, calls.Load())
		}
	}
}

func TestLazyModelIsNotComputedWhenNotCalled(t *testing.T) {
	var calls atomic.Int32

	root := NewID("root", "plain.gohtml").SetFileSystem(lazyModelFS()).UseTemplateCache(true)
	root.SetModel(ModelFuncOf(func(ctx context.Context) (lazyNav, error) {
		calls.Add(1)
		return lazyNav{}, nil
	}))

	if _, err := Render(context.Background(), root); err != nil {
		t.Fatal(err)
	}

	if calls.Load() != 0 {
		t.Fatalf("computed %d times without a template that calls it", calls.Load())
	}
}

func TestLazyModelContractsAndErrors(t *testing.T) {
	root := NewID("root", "sidebar.gohtml").SetFileSystem(lazyModelFS())

	if err := root.CheckContracts(); err == nil {
		t.Fatal("CheckContracts() without the model passed")
	}

	var calls atomic.Int32
	root.SetModel(ModelFuncOf(func(ctx context.Context) (lazyNav, error) {
		calls.Add(1)
		return lazyNav{}, errors.New("navigation down")
	}))

	if err := root.CheckContracts(); err != nil {
		t.Fatalf("CheckContracts() = %v", err)
	}

	if calls.Load() != 0 {
		t.Fatal("CheckContracts computed the model")
	}

	_, err := Render(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "navigation down") {
		t.Fatalf("Render() error = %v, want the model's error", err)
	}

	other := NewID("other", "sidebar.gohtml").SetFileSystem(lazyModelFS())
	other.SetModel(ModelFuncOf(func(ctx context.Context) (lazyStats, error) { return lazyStats{}, nil }))

	if err := other.CheckContracts(); err == nil {
		t.Fatal("CheckContracts() accepted a model of another type")
	}
}

func TestLazyModelInText(t *testing.T) {
	root := NewID("root", "sidebar.gohtml").SetFileSystem(lazyModelFS())
	root.SetModel(ModelFuncOf(func(ctx context.Context) (lazyNav, error) {
		return lazyNav{Items: []string{"x"}}, nil
	}))

	out, err := RenderText(context.Background(), root)
	if err != nil || out != "<nav>x;</nav>" {
		t.Fatalf("RenderText() = %q, %v", out, err)
	}
}

func TestSetModelIgnoresNil(t *testing.T) {
	var missing *lazyNav

	root := NewID("root", "sidebar.gohtml").
		SetFileSystem(lazyModelFS()).
		SetModel(nil, lazyNav{Items: []string{"y"}})

	out, err := Render(context.Background(), root)
	if err != nil || string(out) != "<nav>y;</nav>" {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	// A typed nil pointer is a value, of another type than the template's.
	root.SetModel(missing)

	if len(root.getContracts()) != 2 {
		t.Fatalf("contracts = %d, want 2", len(root.getContracts()))
	}
}

func TestDotSource(t *testing.T) {
	if !(Dot{}).IsZero() || DotFuncOf[lazyStats](nil) != (Dot{}) {
		t.Fatal("IsZero is wrong")
	}

	type holder struct {
		Dot Dot
	}

	var calls atomic.Int32
	typed := holder{Dot: DotFuncOf(func(ctx context.Context) (lazyStats, error) {
		calls.Add(1)
		return lazyStats{Count: 4}, nil
	})}

	stats := NewID("stats", "stats.gohtml").SetFileSystem(lazyFS()).SetDotSource(typed.Dot)
	if err := stats.CheckContracts(); err != nil || calls.Load() != 0 {
		t.Fatalf("CheckContracts() = %v after %d calls", err, calls.Load())
	}

	out, err := Render(context.Background(), stats)
	if err != nil || string(out) != `<div id="stats">4</div>` {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	wrong := NewID("stats", "stats.gohtml").SetFileSystem(lazyFS()).
		SetDotSource(DotFuncOf(func(ctx context.Context) (lazyNav, error) { return lazyNav{}, nil }))
	if err := wrong.CheckContracts(); err == nil {
		t.Fatal("CheckContracts() accepted a dot of another type")
	}

	untyped := NewID("stats", "stats.gohtml").SetFileSystem(lazyFS()).
		SetDotSource(DotFuncOf(func(ctx context.Context) (any, error) { return lazyStats{Count: 5}, nil }))
	if out, err := Render(context.Background(), untyped); err != nil || string(out) != `<div id="stats">5</div>` {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	untyped.SetDotSource(Dot{})

	if _, _, ok := untyped.getDot(); ok {
		t.Fatal("the zero Dot left a dot")
	}
}

func TestSetChildIncludesLeaveTheLayoutOut(t *testing.T) {
	files := includesFS()
	files["layout.gohtml"] = &fstest.MapFile{Data: []byte(`<main>{{ content }}</main>`)}
	files["broken.gohtml"] = &fstest.MapFile{Data: []byte(`{{ define "broken" }}{{ end`)}

	for _, cached := range []bool{false, true} {
		root := NewID("root", "layout.gohtml").
			SetFileSystem(files).
			UseTemplateCache(cached).
			SetChildIncludes("partials/*.gohtml").
			SetContent(NewID("content", "content.gohtml"))

		out, err := Render(context.Background(), root)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cached, err)
		}

		if want := `<main><p><b>content</b><div><b>card</b></div></p></main>`; string(out) != want {
			t.Fatalf("cache %v: output = %q, want %q", cached, out, want)
		}

		if got := root.getIncludePatterns(); len(got) != 0 {
			t.Fatalf("cache %v: the root's includes = %v, want none", cached, got)
		}

		if got := root.children["content"].getIncludePatterns(); len(got) != 1 || got[0] != "partials/*.gohtml" {
			t.Fatalf("cache %v: the content's includes = %v", cached, got)
		}

		// The layout's parse leaves them out: a broken include that only
		// the layout would see changes nothing.
		root.SetChildIncludes("partials/*.gohtml")
		root.SetIncludes()

		layoutOnly := NewID("root", "layout.gohtml").
			SetFileSystem(files).
			UseTemplateCache(cached).
			SetChildIncludes("broken.gohtml")
		if _, err := Render(context.Background(), layoutOnly); err != nil {
			t.Fatalf("cache %v: the layout parsed its child includes: %v", cached, err)
		}
	}
}

func TestGlobDefinesAndHasEntry(t *testing.T) {
	files := fstest.MapFS{
		"regions/menu.gohtml":  {Data: []byte(`menu`)},
		"regions/title.gohtml": {Data: []byte(`title`)},
		"regions/sub/x.gohtml": {Data: []byte(`x`)},
		"mail/welcome.gohtml":  {Data: []byte(`{{ define "subject" }}Hi{{ end }}{{ define "html" }}<p>Hi</p>{{ end }}`)},
		"mail/text.gohtml":     {Data: []byte(`{{ define "subject" }}Hi{{ end }}`)},
		"mail/broken.gohtml":   {Data: []byte(`{{ define "html" }}`)},
	}

	root := NewID("root").SetFileSystem(files).UseTemplateCache(true)

	matches, err := root.Glob("regions/*.gohtml")
	if err != nil || strings.Join(matches, ",") != "regions/menu.gohtml,regions/title.gohtml" {
		t.Fatalf("Glob() = %v, %v", matches, err)
	}

	if ok, err := root.Defines("mail/welcome.gohtml", "html"); !ok || err != nil {
		t.Fatalf("Defines(welcome, html) = %v, %v", ok, err)
	}

	if ok, err := root.Defines("mail/text.gohtml", "html"); ok || err != nil {
		t.Fatalf("Defines(text, html) = %v, %v", ok, err)
	}

	if _, err := root.Defines("mail/missing.gohtml", "html"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Defines(missing) error = %v", err)
	}

	if _, err := root.Defines("mail/broken.gohtml", "html"); err == nil {
		t.Fatal("Defines(broken) has no error")
	}

	mail := NewID("mail", "mail/welcome.gohtml").SetParent(root).SetEntry("html")
	if ok, err := mail.HasEntry(); !ok || err != nil {
		t.Fatalf("HasEntry() = %v, %v", ok, err)
	}

	if ok, _ := mail.Clone().SetTemplates("mail/text.gohtml").HasEntry(); ok {
		t.Fatal("HasEntry() for a file without the entry")
	}
}

func TestFirstExistingIn(t *testing.T) {
	theme := fstest.MapFS{"pages/page.gohtml": {Data: []byte(`theme page`)}}
	defaults := fstest.MapFS{
		"pages/about.gohtml": {Data: []byte(`default about`)},
		"pages/page.gohtml":  {Data: []byte(`default page`)},
	}

	root := NewID("root").SetFileSystem(Overlay(theme, defaults)).UseTemplateCache(true)

	if file, _ := root.FirstExisting("pages/about.gohtml", "pages/page.gohtml"); file != "pages/about.gohtml" {
		t.Fatalf("FirstExisting() = %q", file)
	}

	// The theme's general template wins over the default specific one.
	file, err := root.FirstExistingIn(nil, "pages/about.gohtml", "pages/page.gohtml")
	if err != nil || file != "pages/page.gohtml" {
		t.Fatalf("FirstExistingIn(nil) = %q, %v", file, err)
	}

	file, err = root.FirstExistingIn([]fs.FS{nil, defaults, theme}, "pages/about.gohtml")
	if err != nil || file != "pages/about.gohtml" {
		t.Fatalf("FirstExistingIn(layers) = %q, %v", file, err)
	}

	if _, err := root.FirstExistingIn(nil, "pages/none.gohtml"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("FirstExistingIn(none) error = %v", err)
	}

	plain := NewID("plain").SetFileSystem(defaults)
	if file, _ := plain.FirstExistingIn(nil, "pages/about.gohtml"); file != "pages/about.gohtml" {
		t.Fatalf("FirstExistingIn() without an overlay = %q", file)
	}
}

func TestOOBSwapWritesTheConnectorsAttribute(t *testing.T) {
	files := fstest.MapFS{
		"flash.gohtml": {Data: []byte(`<div id="flash"{{ oobSwap "beforeend" }}></div><p{{ oobSwap }}></p>`)},
	}

	tests := []struct {
		connector connector.Connector
		want      string
	}{
		{connector.NewHTMX4(nil), `<div id="flash" hx-swap-oob="beforeend"></div><p hx-swap-oob="true"></p>`},
		{connector.NewUnpoly(nil), `<div id="flash" up-hungry=""></div><p up-hungry=""></p>`},
		{connector.NewTurbo(nil), `<div id="flash" hx-swap-oob="beforeend"></div><p hx-swap-oob="true"></p>`},
	}

	for _, test := range tests {
		for _, cached := range []bool{false, true} {
			p := NewID("flash", "flash.gohtml").SetFileSystem(files).UseTemplateCache(cached).SetConnector(test.connector)

			out, err := Render(context.Background(), p)
			if err != nil || string(out) != test.want {
				t.Fatalf("%T cache %v: Render() = %q, %v", test.connector, cached, out, err)
			}
		}
	}
}

func TestFuncReturnsTheRegisteredFunction(t *testing.T) {
	upper := strings.ToUpper
	greet := func(ctx context.Context, name string) string {
		return name
	}
	root := NewID("root").SetFunc(template.FuncMap{"upper": upper, "greet": greet})
	child := NewID("child").SetParent(root)

	if fn, ok := child.Func("upper"); !ok || fn.(func(string) string)("a") != "A" {
		t.Fatal("Func(upper) is wrong")
	}

	if fn, ok := child.Func("greet"); !ok {
		t.Fatal("Func(greet) is missing")
	} else if _, takesContext := fn.(func(context.Context, string) string); !takesContext {
		t.Fatalf("Func(greet) = %T, want the function as registered", fn)
	}

	if _, ok := child.Func("missing"); ok {
		t.Fatal("Func(missing) found one")
	}
}

func TestFallbackFuncNames(t *testing.T) {
	files := fstest.MapFS{"page.gohtml": {Data: []byte(`{{ shout "a" }}`)}}

	stage := RenderStageHooks{
		FallbackFuncNames: []string{"shout"},
		ResolveFunc: func(ctx *RenderContext, name string) (any, bool) {
			if name != "shout" {
				return nil, false
			}

			return func(value string) string { return "stage:" + value }, true
		},
	}

	if IsReservedFuncName("shout", stage) || len(stage.TemplateFuncNames()) != 1 {
		t.Fatal("a fallback name is reserved, or not given a stand-in")
	}

	for _, cached := range []bool{false, true} {
		// Without a registered function, the stage gives it.
		page := NewID("page", "page.gohtml").SetFileSystem(files).UseTemplateCache(cached).Use(stage)
		if out, err := Render(context.Background(), page); err != nil || string(out) != "stage:a" {
			t.Fatalf("cache %v: without a function = %q, %v", cached, out, err)
		}

		// A registered function wins, without a warning.
		var warnings atomic.Int32
		sink := EventSinkFunc(func(ctx *RenderContext, event Event) {
			if event.Kind == EventFuncProtected {
				warnings.Add(1)
			}
		})

		own := NewID("page", "page.gohtml").SetFileSystem(files).UseTemplateCache(cached).SetEvents(sink).
			// A function of another package than the stage's: those of
			// the stage's own package stand in for it.
			SetFunc(template.FuncMap{"shout": strings.ToUpper}).
			Use(stage)
		if out, err := Render(context.Background(), own); err != nil || string(out) != "A" {
			t.Fatalf("cache %v: with a function = %q, %v", cached, out, err)
		}

		if warnings.Load() != 0 {
			t.Fatalf("cache %v: %d warnings for a fallback name", cached, warnings.Load())
		}
	}
}

func TestRenderContextResolveFuncAndFailureResponse(t *testing.T) {
	files := fstest.MapFS{
		"page.gohtml":  {Data: []byte(`<main id="content">{{ content }}</main>`)},
		"child.gohtml": {Data: []byte(`{{ fail }}`)},
	}

	var resolved, responses, parts atomic.Int32
	helper := RenderStageHooks{
		FuncNames: []string{"helper"},
		ResolveFunc: func(ctx *RenderContext, name string) (any, bool) {
			if name == "helper" {
				return func() string { return "helped" }, true
			}

			return nil, false
		},
	}
	errorStage := RenderStageHooks{
		RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
			if ctx.Kind != renderKindError {
				return next(ctx)
			}

			if fn, ok := ctx.ResolveFunc("helper"); ok && fn.(func() string)() == "helped" {
				resolved.Add(1)
			}

			if ctx.FailureResponse() {
				responses.Add(1)
			} else {
				parts.Add(1)
			}

			return "<p>failed</p>", nil
		},
	}

	fail := template.FuncMap{"fail": func() (string, error) { return "", errors.New("boom") }}
	root := NewID("root", "page.gohtml").SetFileSystem(files).SetFunc(fail).Use(helper, errorStage).
		SetContent(NewID("child", "child.gohtml"))

	// A failing part of a page takes the error fragment in place.
	if out, err := Render(context.Background(), root); err != nil || !strings.Contains(string(out), "<p>failed</p>") {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	if parts.Load() != 1 || responses.Load() != 0 {
		t.Fatalf("parts %d, responses %d after a part failed", parts.Load(), responses.Load())
	}

	// A failing response, under FailResponse.
	root.SetFailurePolicy(FailResponse)
	rec := httptest.NewRecorder()
	_ = Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), root)

	if responses.Load() != 1 || resolved.Load() != parts.Load()+responses.Load() {
		t.Fatalf("responses %d, resolved %d", responses.Load(), resolved.Load())
	}
}

func TestModelFuncOfIsAModelValue(t *testing.T) {
	var calls atomic.Int32

	load := ModelFuncOf(func(ctx context.Context) (lazyNav, error) {
		calls.Add(1)
		return lazyNav{Items: []string{"a", "b"}}, nil
	})

	root := NewID("root", "layout.gohtml").
		SetFileSystem(lazyModelFS()).
		SetContent(NewID("content", "content.gohtml")).
		SetModel(nil, ModelFuncOf[lazyNav](nil), load)

	if err := root.CheckContracts(); err != nil {
		t.Fatalf("CheckContracts() = %v", err)
	}

	if calls.Load() != 0 {
		t.Fatal("CheckContracts computed the model")
	}

	out, err := Render(context.Background(), root)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if want := `<body>2<main id="content">a</main><nav>a;b;</nav></body>`; string(out) != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}

	if calls.Load() != 1 {
		t.Fatalf("computed %d times, want once per render tree", calls.Load())
	}

	plain := NewID("plain", "plain.gohtml").SetFileSystem(lazyModelFS()).SetModel(load)
	if _, err := Render(context.Background(), plain); err != nil {
		t.Fatal(err)
	}

	if calls.Load() != 1 {
		t.Fatal("computed without a template that calls it")
	}

	if !ModelFuncOf[lazyNav](nil).IsZero() || load.IsZero() {
		t.Fatal("IsZero() is wrong")
	}
}

func TestModelFuncOfOfAnotherTypeDoesNotMatch(t *testing.T) {
	root := NewID("root", "sidebar.gohtml").
		SetFileSystem(lazyModelFS()).
		SetModel(ModelFuncOf(func(ctx context.Context) (lazyStats, error) { return lazyStats{}, nil }))

	if err := root.CheckContracts(); err == nil {
		t.Fatal("CheckContracts() accepted a model of another type")
	}
}

// TestDotModelSharesTheDot checks that Dot.Model is the dot as a model,
// computed once per render tree for the dot and the model together, and
// that a Dot of an interface type or the zero Dot is no model.
func TestDotModelSharesTheDot(t *testing.T) {
	var calls atomic.Int32

	dot := DotFuncOf(func(ctx context.Context) (lazyNav, error) {
		calls.Add(1)

		return lazyNav{Items: []string{"a", "b"}}, nil
	})

	files := lazyModelFS()
	files["dotlayout.gohtml"] = &fstest.MapFile{Data: []byte(`<body>{{ len .Items }}{{ content }}</body>`)}

	for _, cached := range []bool{false, true} {
		calls.Store(0)

		root := NewID("root", "dotlayout.gohtml").
			SetFileSystem(files).
			UseTemplateCache(cached).
			SetDotSource(dot).
			SetModel(dot.Model()).
			SetContent(NewID("content", "content.gohtml"))

		if err := root.CheckContracts(); err != nil {
			t.Fatalf("cache %v: CheckContracts() = %v", cached, err)
		}

		out, err := Render(context.Background(), root)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cached, err)
		}

		if want := `<body>2<main id="content">a</main></body>`; string(out) != want {
			t.Fatalf("cache %v: Render() = %q, want %q", cached, out, want)
		}

		if calls.Load() != 1 {
			t.Fatalf("cache %v: computed %d times, want once for the dot and the model", cached, calls.Load())
		}
	}

	untyped := DotFuncOf(func(context.Context) (any, error) {
		return lazyNav{}, nil
	})

	if !untyped.Model().IsZero() || !(Dot{}).Model().IsZero() {
		t.Fatal("an untyped or zero Dot is a model")
	}
}
