package partial

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
