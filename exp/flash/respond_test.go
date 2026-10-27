package flash

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
)

func appFS() fstest.MapFS {
	return fstest.MapFS{
		"toast.gohtml": {
			Data: []byte(
				`{{ range .Messages }}<p class="toast toast-{{ .Level }}" data-flash-level="{{ .Level }}" title="{{ .Text }}">{{ .Text }}</p>{{ end }}`,
			),
		},
		"page.gohtml":  {Data: []byte(`<div id="toasts">{{ flashTemplate "error" "info" }}</div>`)},
		"plain.gohtml": {Data: []byte(`{{ flashTemplate }}`)},
		"oob.gohtml":   {Data: []byte(`ok`)},
	}
}

func appRoot() *partial.Partial {
	return partial.NewID("root").
		SetFileSystem(appFS()).
		SetConnector(connector.NewHTMX4(nil)).
		UseTemplateCache(true).
		Use(Stage(WithTemplate("toast.gohtml"), WithTargetID("toasts")))
}

func TestRespondAddsTheMessagesOutOfBand(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/save", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	if err := Respond(rec, req, appRoot(), http.StatusForbidden, Error("You may not do this."), Info(" ")); err != nil {
		t.Fatal(err)
	}

	want := `<div id="toasts" hx-swap-oob="beforeend"><p class="toast toast-error" data-flash-level="error" title="You may not do this.">You may not do this.</p></div>`
	if rec.Code != http.StatusForbidden || rec.Body.String() != want {
		t.Fatalf("response = %d %q, want %q", rec.Code, rec.Body.String(), want)
	}

	if rec.Header().Get("HX-Reswap") != "none" || rec.Header().Get("Cache-Control") != "no-store" ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("headers = %v", rec.Header())
	}
}

func TestRespondWithoutAFlashStageUsesTheDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/save", nil)
	req.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()

	root := partial.NewID("root").SetConnector(connector.NewHTMX(nil))
	if err := Respond(rec, req, root, 0, Warn("Careful")); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(body, `<div id="flash-messages" hx-swap-oob="beforeend">`) ||
		!strings.Contains(body, `data-flash-level="warn"`) {
		t.Fatalf("response = %d %q", rec.Code, body)
	}
}

func TestRespondToAPlainRequestWithText(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Respond(
		rec,
		httptest.NewRequest(http.MethodPost, "/save", nil),
		appRoot(),
		http.StatusConflict,
		Error("Changed meanwhile."),
		Info("Reload."),
	); err != nil {
		t.Fatal(err)
	}

	if rec.Code != http.StatusConflict || rec.Body.String() != "Changed meanwhile.\nReload." ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("response = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
}

func TestFlashTemplateRendersTheAppsTemplate(t *testing.T) {
	page := partial.NewID("page", "page.gohtml").SetParent(appRoot())

	out, err := partial.Render(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}

	want := `<div id="toasts">` +
		`<template data-flash-template="error"><p class="toast toast-error" data-flash-level="error" title="__partial_flash_text__">__partial_flash_text__</p></template>` +
		`<template data-flash-template="info"><p class="toast toast-info" data-flash-level="info" title="__partial_flash_text__">__partial_flash_text__</p></template>` +
		`</div>`
	if string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	all, err := partial.Render(context.Background(), partial.NewID("plain", "plain.gohtml").SetParent(appRoot()))
	if err != nil || strings.Count(string(all), "<template data-flash-template=") != 4 {
		t.Fatalf("all levels = %q, %v", all, err)
	}
}

func TestFlashTemplateFuncMapUsesTheDefaultTemplate(t *testing.T) {
	files := fstest.MapFS{"page.gohtml": {Data: []byte(`{{ flashTemplate ctx "warn" }}`)}}
	page := partial.NewID("page", "page.gohtml").SetFileSystem(files).SetFunc(template.FuncMap{"flashTemplate": Template})

	out, err := partial.Render(context.Background(), page)
	if err != nil || !strings.HasPrefix(string(out), `<template data-flash-template="warn">`) ||
		!strings.Contains(string(out), "partial-flash__message--warn") {
		t.Fatalf("output = %q, %v", out, err)
	}
}

func TestRenderOOBNeedsAFlashStage(t *testing.T) {
	var withStage, without bool

	probe := partial.RenderStageHooks{
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if ctx.Partial.PartialID() != "oob" {
				return next(ctx)
			}

			out, ok := RenderOOB(ctx, Success("Saved"))
			if ok && strings.Contains(string(out), `data-flash-level="success"`) {
				withStage = true
			}

			if !ok {
				without = true
			}

			return next(ctx)
		},
	}

	if _, err := partial.Render(
		context.Background(),
		partial.NewID("oob", "oob.gohtml").SetParent(appRoot()).Use(probe),
	); err != nil {
		t.Fatal(err)
	}

	if _, err := partial.Render(
		context.Background(),
		partial.NewID("oob", "oob.gohtml").SetFileSystem(appFS()).Use(probe),
	); err != nil {
		t.Fatal(err)
	}

	if !withStage || !without {
		t.Fatalf("with a stage %v, without %v", withStage, without)
	}
}
