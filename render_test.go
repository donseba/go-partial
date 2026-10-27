package partial

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/donseba/go-partial/connector"
)

func TestWriteRendersSafeErrorPageOnTemplateError(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").ID("broken").SetFileSystem(fsys).Use(testErrorStage(false))
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, req, p)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(body, "Template render error") {
		t.Fatalf("expected failure response, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml") {
		t.Fatalf("expected template name in failure response, got %q", body)
	}
	if !strings.Contains(body, "Partial ID") {
		t.Fatalf("expected partial ID label in failure response, got %q", body)
	}
	if !strings.Contains(body, "<dt>Template</dt>") {
		t.Fatalf("expected singular template label in failure response, got %q", body)
	}
	if strings.Contains(body, "unexpected EOF") {
		t.Fatalf("expected safe mode to hide detailed error, got %q", body)
	}
	if strings.Contains(body, "stack trace:") {
		t.Fatalf("expected safe mode to hide stack trace, got %q", body)
	}
}

func TestWriteRendersDetailedErrorPageOnTemplateError(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").ID("broken").SetFileSystem(fsys).Use(testErrorStage(true))
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, req, p)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(body, "unexpected EOF") {
		t.Fatalf("expected detailed error in failure response, got %q", body)
	}
	if !strings.Contains(body, "broken.gohtml:1") {
		t.Fatalf("expected template error location in failure response, got %q", body)
	}
	if strings.Contains(body, "stack trace:") {
		t.Fatalf("expected detailed RenderStage to omit stack trace, got %q", body)
	}
}

func TestWriteUsesCustomErrorStage(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").
		ID("broken").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
				if ctx.Kind != renderKindError {
					return next(ctx)
				}
				return template.HTML(`<div id="custom-error">` + ctx.Partial.PartialID() + `</div>`), nil
			},
		})

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, req, p)
	if err == nil {
		t.Fatal("expected original render error")
	}
	if body := rec.Body.String(); body != `<div id="custom-error">broken</div>` {
		t.Fatalf("unexpected custom error body: %q", body)
	}
}

func TestWriteRendersSwappableErrorFragmentForHTMX(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	p := New("broken.gohtml").
		ID("content").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		Use(testErrorStage(true))
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")
	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, req, p)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx-swappable 200, got %d", rec.Code)
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected fragment error output, got full document: %q", body)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected partial error fragment, got %q", body)
	}
}

func TestWriteAppendsAncestorOOBToHTMXErrorFragment(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)
	fsys.AddFile("header.gohtml", `<header id="app-header"{{ oobAttr }}>Header</header>`)

	wrapper := NewID("shell", "shell.gohtml").SetFileSystem(fsys)
	wrapper.WithOOB(NewID("header", "header.gohtml").SetFileSystem(fsys).SetAlwaysSwapOOB(true))
	content := NewID("content", "broken.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		Use(testErrorStage(false))
	wrapper.With(content)

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")
	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, req, content)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected htmx-swappable 200, got %d", rec.Code)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected partial error fragment, got %q", body)
	}
	if !strings.Contains(body, `id="app-header" hx-swap-oob="true"`) {
		t.Fatalf("expected OOB header in error response, got %q", body)
	}
}

func TestRegisteredTargetTemplateErrorRendersSectionFailureResponse(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("shell.gohtml", `<html><body><header>Header</header><main>{{ content }}</main><footer>Footer</footer></body></html>`)
	fsys.AddFile("broken.gohtml", `{{ if .Missing }}missing`)

	wrapper := NewID("shell", "shell.gohtml").SetFileSystem(fsys)
	content := NewID("content", "broken.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		Use(testErrorStage(false))
	wrapper.With(content)

	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HeaderTarget.String(), "content")
	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, req, content)
	if err == nil {
		t.Fatal("expected original render error")
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected swappable fragment status 200, got %d", rec.Code)
	}
	if !strings.Contains(body, `class="go-partial-error"`) {
		t.Fatalf("expected target error fragment, got %q", body)
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Fatalf("expected section failure response, got full error document: %q", body)
	}
}

func TestWriteAppliesFluentConnectorResponse(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("notice.gohtml", `<div id="notice">Saved</div>`)

	p := NewID("notice", "notice.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil))
	p.Response().
		Retarget("#notice").
		TriggerWith(connector.NewTrigger().AddEvent("saved"))

	req := httptest.NewRequest(http.MethodGet, "/notice", nil)
	rec := httptest.NewRecorder()

	if err := Write(context.Background(), rec, req, p); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	if got := rec.Header().Get(connector.HTMXHeaderRetarget.String()); got != "#notice" {
		t.Fatalf("expected HX-Retarget header, got %q", got)
	}
	if got := rec.Header().Get(connector.HTMXHeaderTrigger.String()); got != `{"saved":null}` {
		t.Fatalf("expected HX-Trigger header, got %q", got)
	}
}

func TestWriteAppliesStructConnectorResponse(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("notice.gohtml", `<div id="notice">Saved</div>`)

	p := NewID("notice", "notice.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		SetResponse(connector.Response{
			Retarget: "#notice",
			Trigger:  connector.NewTrigger().AddEvent("saved").String(),
		})

	req := httptest.NewRequest(http.MethodGet, "/notice", nil)
	rec := httptest.NewRecorder()

	if err := Write(context.Background(), rec, req, p); err != nil {
		t.Fatalf("write partial: %v", err)
	}

	if got := rec.Header().Get(connector.HTMXHeaderRetarget.String()); got != "#notice" {
		t.Fatalf("expected HX-Retarget header, got %q", got)
	}
	if got := rec.Header().Get(connector.HTMXHeaderTrigger.String()); got != `{"saved":null}` {
		t.Fatalf("expected HX-Trigger header, got %q", got)
	}
}

func TestWriteAppliesRenderResponse(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("page.gohtml", `ok`)

	p := New("page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{
			FinalizeFunc: func(ctx *RenderContext, out template.HTML, err error) (template.HTML, error) {
				ctx.Response.Status = http.StatusAccepted
				ctx.Response.Headers["X-Render-Response"] = "applied"
				return out, err
			},
		})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	if err := Write(context.Background(), rec, req, p); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if got := rec.Header().Get("X-Render-Response"); got != "applied" {
		t.Fatalf("X-Render-Response = %q", got)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestPackageRenderMatchesPartialRender(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("page.gohtml", `hello {{.}}`)

	p := New("page.gohtml").SetFileSystem(fsys).SetDot("world")

	got, err := Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if got != "hello world" {
		t.Fatalf("Render() = %q, want %q", got, "hello world")
	}
}

func TestPackageRenderWithRequestRendersTargetAndOOB(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("page.gohtml", `<main>{{ template "content.gohtml" . }}</main>`)
	fsys.AddFile("content.gohtml", `<section id="content">Content</section>`)
	fsys.AddFile("notice.gohtml", `<aside id="notice"{{ oobAttr }}>Notice</aside>`)

	page := NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil))
	content := NewID("content", "content.gohtml").SetFileSystem(fsys)
	page.With(content)
	page.WithOOB(NewID("notice", "notice.gohtml").SetFileSystem(fsys).SetAlwaysSwapOOB(true))

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "content")

	out, err := RenderWithRequest(context.Background(), req, page)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
	body := string(out)
	if !strings.Contains(body, `<section id="content">Content</section>`) {
		t.Fatalf("expected target output, got %q", body)
	}
	if !strings.Contains(body, `id="notice" hx-swap-oob="true"`) {
		t.Fatalf("expected OOB output, got %q", body)
	}
}

func TestPackageWriteAppliesResponseBehavior(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("page.gohtml", `ok`)

	p := New("page.gohtml").
		SetFileSystem(fsys).
		SetConnector(connector.NewHTMX(nil)).
		SetResponseHeaders(map[string]string{"X-Partial": "configured"}).
		SetStatus(http.StatusAccepted).
		Use(RenderStageHooks{
			FinalizeFunc: func(ctx *RenderContext, out template.HTML, err error) (template.HTML, error) {
				ctx.Response.Headers["X-Stage"] = "applied"
				return out, err
			},
		})
	p.Response().Retarget("#page")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	if err := Write(context.Background(), rec, req, p); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if got := rec.Header().Get("X-Partial"); got != "configured" {
		t.Fatalf("X-Partial = %q", got)
	}
	if got := rec.Header().Get("X-Stage"); got != "applied" {
		t.Fatalf("X-Stage = %q", got)
	}
	if got := rec.Header().Get(connector.HTMXHeaderRetarget.String()); got != "#page" {
		t.Fatalf("HX-Retarget = %q", got)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestTargetResolverRendersDynamicRowTarget(t *testing.T) {
	type row struct {
		ID   int
		Name string
	}

	rows := []row{
		{ID: 1, Name: "Coffee"},
		{ID: 2, Name: "Tea"},
	}

	fsys := &inMemoryFS{}
	fsys.AddFile("table.gohtml", `<table><tbody>{{ range .Rows }}{{ template "row.gohtml" . }}{{ end }}</tbody></table>`)
	fsys.AddFile("row.gohtml", `<tr id="row-{{ .ID }}"><td>{{ .Name }}</td></tr>`)

	table := NewID("content", "table.gohtml").
		SetFileSystem(fsys).
		SetDot(map[string]any{"Rows": rows}).
		SetFunc(testTargetFuncMap()).
		Use(testTargetStage())
	rowPartial := NewID("row", "row.gohtml").SetFileSystem(fsys)
	table.With(rowPartial)
	testUseTargetResolver(table, func(ctx context.Context, r *http.Request, target string) (*Partial, bool) {
		if !strings.HasPrefix(target, "row-") {
			return nil, false
		}
		id, err := strconv.Atoi(strings.TrimPrefix(target, "row-"))
		if err != nil {
			return nil, false
		}
		for _, candidate := range rows {
			if candidate.ID == id {
				return NewID(target, "row.gohtml").SetFileSystem(fsys).SetDot(candidate), true
			}
		}
		return nil, false
	})

	req := httptest.NewRequest(http.MethodGet, "/rows", nil)
	req.Header.Set(connector.HeaderTarget.String(), "row-2")

	out, err := RenderWithRequest(context.Background(), req, table)
	if err != nil {
		t.Fatalf("render with dynamic target: %v", err)
	}

	if string(out) != `<tr id="row-2"><td>Tea</td></tr>` {
		t.Fatalf("unexpected row output: %q", out)
	}
}

func testErrorStage(detailed bool) RenderStage {
	return RenderStageHooks{
		RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
			if ctx.Kind != renderKindError {
				return next(ctx)
			}

			templates := ctx.Partial.TemplatePaths()
			label := "Templates"
			if len(templates) == 1 {
				label = "Template"
			}

			body := `Template render error Partial ID ` + ctx.Partial.PartialID() + ` <dt>` + label + `</dt> ` + strings.Join(templates, ", ")
			if detailed && ctx.Error != nil {
				body += " " + ctx.Error.Error()
			}
			if ctx.Name == "fragment" {
				body = `<section class="go-partial-error">` + body + `</section>`
			}

			return template.HTML(body), nil
		},
	}
}

// countingFS counts the files opened, to show what a render reads.
type countingFS struct {
	fs.FS
	opened atomic.Int64
}

func (c *countingFS) Open(name string) (fs.File, error) {
	c.opened.Add(1)
	return c.FS.Open(name)
}

func TestCachedRendersReadNoFiles(t *testing.T) {
	model := `{{/* @model Page github.com/donseba/go-partial.contractPage */}}`
	files := &countingFS{FS: &inMemoryFS{Files: map[string]string{
		"layout.gohtml": model + `<title>{{ Page.Title }}</title>{{ content }}`,
		"page.gohtml":   `<main>{{ range . }}{{ partial runtime "card.gohtml" . }}{{ end }}</main>`,
		"card.gohtml":   model + `<p>{{ . }} on {{ Page.Title }}</p>`,
		"menu.gohtml":   `<nav id="menu"{{ oobAttr }}>menu</nav>`,
	}}}
	root := NewID("shell", "layout.gohtml").
		SetFileSystem(files).
		UseTemplateCache(true).
		SetModel(contractPage{Title: "Home"})

	render := func() string {
		// Content and regions are new partials on every render, as an
		// application builds them per request.
		page := root.Clone().
			SetContent(NewID("content", "page.gohtml").UseTemplateCache(true).SetDot([]string{"wheel", "glaze"})).
			WithOOB(NewID("menu", "menu.gohtml").UseTemplateCache(true).SetAlwaysSwapOOB(true))
		out, err := Render(context.Background(), page)
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return string(out)
	}

	first := render()
	if want := "<title>Home</title><main><p>wheel on Home</p><p>glaze on Home</p></main>"; first != want {
		t.Fatalf("first render = %q, want %q", first, want)
	}

	files.opened.Store(0)
	if second := render(); second != first {
		t.Fatalf("second render = %q, want %q", second, first)
	}
	if opened := files.opened.Load(); opened != 0 {
		t.Fatalf("a cached render opened %d files, want none", opened)
	}
}

func TestMapFileSystemsDoNotShareCachedTemplates(t *testing.T) {
	render := func(text string) string {
		files := fstest.MapFS{"page.gohtml": &fstest.MapFile{Data: []byte(text)}}
		out, err := Render(context.Background(), New("page.gohtml").SetFileSystem(files).UseTemplateCache(true))
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		return string(out)
	}

	for _, text := range []string{"first", "second"} {
		if got := render(text); got != text {
			t.Fatalf("render = %q, want %q: map file systems must not share cached templates", got, text)
		}
	}
}

func TestSetParentInheritsWithoutChangingTheParent(t *testing.T) {
	fsys := Sources{
		"card.gohtml": `{{/* @model Site github.com/donseba/go-partial.contractSite */}}{{ upper . }} in {{ Site.Name }}{{ stage }}`,
	}
	stage := RenderStageHooks{
		FuncNames: []string{"stage"},
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			ctx.SetFunc("stage", func() string { return "!" })
			return ctx, nil
		},
	}
	root := New().
		SetFileSystem(fsys).
		UseTemplateCache(true).
		SetFunc(template.FuncMap{"upper": strings.ToUpper, "stage": func() string { return "" }}).
		Use(stage).
		SetModel(contractSite{Name: "acme"})

	for _, name := range []string{"wheel", "glaze"} {
		card := NewID("card", "card.gohtml").UseTemplateCache(true).SetDot(name).SetParent(root)

		out, err := RenderSelf(context.Background(), nil, card)
		if err != nil {
			t.Fatalf("RenderSelf() error = %v", err)
		}

		if want := strings.ToUpper(name) + " in acme!"; string(out) != want {
			t.Fatalf("RenderSelf() = %q, want %q", out, want)
		}

		if card.ParentID() != "root" {
			t.Fatalf("ParentID() = %q, want root", card.ParentID())
		}
	}

	if len(root.children) != 0 {
		t.Fatalf("root has %d children, want SetParent to leave it unchanged", len(root.children))
	}

	child := NewID("child")
	child.SetParent(root)
	root.SetParent(child)

	if root.ParentID() != "" || child.ParentID() != "root" {
		t.Fatalf("SetParent() made a cycle: root's parent %q, child's %q", root.ParentID(), child.ParentID())
	}

	if child.SetParent(nil).ParentID() != "" {
		t.Fatal("SetParent(nil) kept the parent")
	}
}

func TestSetEntryRendersADefinedTemplate(t *testing.T) {
	fsys := Sources{
		"mail.gohtml": `{{/* @dot string */}}{{ define "subject" }}Hello {{ . }}{{ end }}{{ define "html" }}<p>Hello {{ . }}, {{ partial runtime "sign.gohtml" }}</p>{{ end }}`,
		"sign.gohtml": `the team`,
	}

	for _, cache := range []bool{false, true} {
		mail := New("mail.gohtml").SetFileSystem(fsys).UseTemplateCache(cache).SetDot("Ada")

		out, err := Render(context.Background(), mail.SetEntry("html"))
		if err != nil || out != "<p>Hello Ada, the team</p>" {
			t.Fatalf("cache %v: entry html = %q, %v", cache, out, err)
		}

		out, err = Render(context.Background(), mail.SetEntry("subject"))
		if err != nil || out != "Hello Ada" {
			t.Fatalf("cache %v: entry subject = %q, %v", cache, out, err)
		}

		_, err = Render(context.Background(), mail.SetEntry("text"))
		if err == nil || !strings.Contains(err.Error(), "'text' of 'mail.gohtml'") {
			t.Fatalf("cache %v: missing entry error = %v", cache, err)
		}

		if out, err := Render(context.Background(), mail.SetEntry("")); err != nil || out != "" {
			t.Fatalf("cache %v: the file itself = %q, %v", cache, out, err)
		}
	}
}

func TestPartialCallsRunTheStagesOnce(t *testing.T) {
	var prepared atomic.Int64
	stage := RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			prepared.Add(1)
			return ctx, nil
		},
	}
	fsys := Sources{
		"page.gohtml": `{{ partial runtime "card.gohtml" }}`,
		"card.gohtml": `card`,
	}
	page := New("page.gohtml").SetFileSystem(fsys).UseTemplateCache(true).Use(stage)

	if out, err := Render(context.Background(), page); err != nil || out != "card" {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	if got := prepared.Load(); got != 2 {
		t.Fatalf("stage prepared %d renders, want 2: the page and its card", got)
	}
}

// TestNewPartialsShareTemplateScans checks that partials built per request
// under one configured parent read no files after the first.
func TestNewPartialsShareTemplateScans(t *testing.T) {
	files := &countingFS{FS: &inMemoryFS{Files: map[string]string{
		"card.gohtml": `{{/* @dot string */}}{{ . }}`,
	}}}
	root := NewID("root").SetFileSystem(files).UseTemplateCache(true)

	check := func() {
		card := NewID("card", "card.gohtml").SetParent(root).SetDot("x")
		if err := card.CheckContracts(); err != nil {
			t.Fatalf("CheckContracts() error = %v", err)
		}
	}

	check()
	files.opened.Store(0)
	check()

	if opened := files.opened.Load(); opened != 0 {
		t.Fatalf("checking a new partial opened %d files, want none", opened)
	}
}

// TestTemplateCacheIsKeptPerRoot checks that two roots over one directory
// keep what their template caches learn apart: a root made after a file was
// added sees it, while a root that looked before keeps its answer as long as
// its cache is on, and sees the file with the cache off.
func TestTemplateCacheIsKeptPerRoot(t *testing.T) {
	for _, cache := range []bool{true, false} {
		t.Run(fmt.Sprintf("cache=%v", cache), func(t *testing.T) {
			dir := t.TempDir()
			write := func(name string, text string) {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			write("page.gohtml", `<main>{{ template "badge" }}</main>`)
			write("partials/badge.gohtml", `{{ define "badge" }}badge{{ end }}`)

			newRoot := func() *Partial {
				return NewID("page", "page.gohtml").
					SetFileSystem(os.DirFS(dir)).
					SetIncludes("partials/*.gohtml").
					UseTemplateCache(cache)
			}

			// What a root looks for: the error page, the includes and a
			// file it renders by path.
			look := func(root *Partial) (string, []string, string) {
				errorPage, _ := root.FirstExisting("500.gohtml", "page.gohtml")

				includes, err := root.Glob("partials/*.gohtml")
				if err != nil {
					t.Fatal(err)
				}

				out, err := Render(context.Background(), root)
				if err != nil {
					t.Fatalf("render: %v", err)
				}

				return errorPage, includes, string(out)
			}

			first := newRoot()

			errorPage, includes, out := look(first)
			if errorPage != "page.gohtml" || len(includes) != 1 || out != "<main>badge</main>" {
				t.Fatalf("first root before = %q, %v, %q", errorPage, includes, out)
			}

			write("500.gohtml", `error`)
			write("partials/more.gohtml", `{{ define "more" }}more{{ end }}`)

			second := newRoot()

			errorPage, includes, _ = look(second)
			if errorPage != "500.gohtml" || len(includes) != 2 {
				t.Fatalf("second root = %q, %v: a root made after the files were added must see them", errorPage, includes)
			}

			errorPage, includes, _ = look(first)
			if cache && (errorPage != "page.gohtml" || len(includes) != 1) {
				t.Fatalf("first root with the cache = %q, %v: it keeps its own answers", errorPage, includes)
			}

			if !cache && (errorPage != "500.gohtml" || len(includes) != 2) {
				t.Fatalf("first root without the cache = %q, %v: it must see new files", errorPage, includes)
			}

			// A clone shares its root's cache.
			errorPage, _, _ = look(first.Clone())
			if cache && errorPage != "page.gohtml" {
				t.Fatalf("clone of the first root = %q, want the first root's answer", errorPage)
			}
		})
	}
}

// TestWriteWithSizeHints checks that pages written one after the other,
// larger and smaller than the last, come out whole, the same as Render
// gives them, whether the writer takes strings or bytes only.
func TestWriteWithSizeHints(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/list.gohtml": `<ul>{{ range .Items }}<li>{{ . }}</li>{{ end }}</ul>`,
	}}

	ctx := context.Background()
	request := httptest.NewRequest(http.MethodGet, "/", nil)

	for _, count := range []int{500, 3, 0, 2000, 40, 2000} {
		items := make([]string, count)
		for i := range items {
			items[i] = fmt.Sprintf("item <%d>", i)
		}

		page := func() *Partial {
			return NewID("list", "templates/list.gohtml").
				SetFileSystem(fsys).
				UseTemplateCache(true).
				SetDot(map[string]any{"Items": items})
		}

		want, err := Render(ctx, page())
		if err != nil {
			t.Fatal(err)
		}

		if !strings.HasPrefix(string(want), "<ul>") || strings.Count(string(want), "<li>") != count {
			t.Fatalf("%d items rendered as %q", count, want)
		}

		// The recorder takes strings; wrapped, only bytes.
		strs := httptest.NewRecorder()
		onlyBytes := httptest.NewRecorder()

		for _, w := range []http.ResponseWriter{strs, struct{ http.ResponseWriter }{onlyBytes}} {
			if err := Write(ctx, w, request, page()); err != nil {
				t.Fatal(err)
			}
		}

		if strs.Body.String() != string(want) || onlyBytes.Body.String() != string(want) {
			t.Fatalf("%d items: written %d and %d bytes, want %d", count, strs.Body.Len(), onlyBytes.Body.Len(), len(want))
		}
	}
}

// TestWriteErrorRunsTheErrorStage checks that WriteError answers an error
// outside a render with the error stage of the tree, which sees the
// status as FailureStatus and the request as a partial request, writes it
// with that status, never cached, varying with the connector's headers.
func TestWriteErrorRunsTheErrorStage(t *testing.T) {
	var seen *RenderContext

	p := New("page.gohtml").
		ID("page").
		SetConnector(connector.NewHTMX(nil)).
		Use(RenderStageHooks{
			RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
				if ctx.Kind != renderKindError {
					return next(ctx)
				}

				seen = ctx

				return template.HTML(
					`<p>` + strconv.Itoa(ctx.FailureStatus()) + ` ` + ctx.Name + ` ` + ctx.Error.Error() + `</p>`,
				), nil
			},
		})

	req := httptest.NewRequest(http.MethodPost, "/comments", nil)
	req.Header.Set(connector.HTMXHeaderRequest.String(), "true")
	req.Header.Set(connector.HTMXHeaderTarget.String(), "comments")

	rec := httptest.NewRecorder()
	if err := WriteError(rec, req, p, http.StatusTooManyRequests, nil); err != nil {
		t.Fatalf("WriteError() = %v", err)
	}

	if body := rec.Body.String(); rec.Code != http.StatusTooManyRequests || body != `<p>429 fragment Too Many Requests</p>` {
		t.Fatalf("response = %d %q", rec.Code, body)
	}

	if !seen.FailureResponse() || rec.Header().Get("Cache-Control") != "no-store" ||
		!varyLists(rec.Header().Values("Vary"), "HX-Target") {
		t.Fatalf("FailureResponse = %v, headers = %v", seen.FailureResponse(), rec.Header())
	}

	if (*RenderContext)(nil).FailureStatus() != 0 {
		t.Fatal("a nil RenderContext has a failure status")
	}
}
