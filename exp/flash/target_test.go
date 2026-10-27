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
	"github.com/donseba/go-partial/exp/slots"
)

func TestFlashTargetAttrsWriteTheRequestErrorsOfTheStage(t *testing.T) {
	errors := RequestErrors{
		Text:      "The request failed.",
		Replace:   true,
		Body:      true,
		SkipFlash: true,
	}

	tests := []struct {
		name     string
		template string
		stage    partial.RenderStage
		want     string
	}{
		{
			name:     "the stage's settings",
			template: `<div{{ flashTargetAttrs }}>`,
			stage:    Stage(WithTargetID("toasts"), WithRequestErrors(errors)),
			want:     `<div id="toasts" data-flash-request-error="The request failed." data-flash-request-error-replace data-flash-request-error-body data-flash-request-error-skip-flash>`,
		},
		{
			name:     "a text of the template's own",
			template: `<div{{ flashTargetAttrs "Mislukt & opnieuw" }}>`,
			stage:    Stage(WithRequestErrors(RequestErrors{Text: "Failed", Replace: true})),
			want:     `<div id="flash-messages" data-flash-request-error="Mislukt &amp; opnieuw" data-flash-request-error-replace>`,
		},
		{
			name:     "only the id without request errors",
			template: `<div{{ flashTargetAttrs }}>`,
			stage:    Stage(),
			want:     `<div id="flash-messages">`,
		},
		{
			name:     "an empty text keeps the settings",
			template: `<div{{ flashTargetAttrs "" }}>`,
			stage:    Stage(WithRequestErrors(RequestErrors{Body: true})),
			want:     `<div id="flash-messages" data-flash-request-error="" data-flash-request-error-body>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := partial.NewID("page", "page.gohtml").
				SetFileSystem(fstest.MapFS{"page.gohtml": {Data: []byte(test.template)}}).
				Use(test.stage)

			out, err := partial.Render(context.Background(), page)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if string(out) != test.want {
				t.Fatalf("Render() =\n%s\nwant\n%s", out, test.want)
			}
		})
	}
}

func TestFlashTargetAttrsFuncMap(t *testing.T) {
	page := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{"page.gohtml": {Data: []byte(`<div{{ flashTargetAttrs }}><div{{ flashTargetAttrs "Failed" }}>`)}}).
		SetFunc(template.FuncMap{"flashTargetAttrs": TargetAttrs})

	out, err := partial.Render(context.Background(), page)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := `<div id="flash-messages"><div id="flash-messages" data-flash-request-error="Failed">`
	if string(out) != want {
		t.Fatalf("Render() = %s, want %s", out, want)
	}
}

func TestFlashTargetWritesTheRequestErrors(t *testing.T) {
	page := partial.NewID("page", "page.gohtml").
		SetFileSystem(fstest.MapFS{"page.gohtml": {Data: []byte(`{{ flashTarget }}`)}}).
		Use(Stage(WithRequestErrors(RequestErrors{Text: "Failed", SkipFlash: true})))

	out, err := partial.Render(context.Background(), page)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := `<div id="flash-messages" data-flash-request-error="Failed" data-flash-request-error-skip-flash class="partial-flash-target"`
	if !strings.Contains(string(out), want) {
		t.Fatalf("Render() = %s, want it to contain %s", out, want)
	}
}

func TestFlashTemplateWritesNothingOutOfBand(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml": {Data: []byte(`<body>{{ slot "toasts" }}{{ content }}</body>`)},
		"toasts.gohtml": {
			Data: []byte(`<div{{ flashTargetAttrs }} {{ oobAttr "beforeend" }}>{{ flash }}{{ flashTemplate "error" }}</div>`),
		},
		"content.gohtml": {Data: []byte(`<main id="content">page</main>`)},
	}

	root := func() *partial.Partial {
		layout := partial.NewID("root", "layout.gohtml").
			SetFileSystem(files).
			SetConnector(connector.NewHTMX4(nil)).
			Use(slots.Stage(), Stage(WithTargetID("toasts"))).
			SetContent(partial.NewID("content", "content.gohtml"))

		return slots.Set(layout, "toasts", partial.NewID("toasts", "toasts.gohtml"), slots.OOB())
	}

	page, err := partial.RenderWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), root())
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	if !strings.Contains(string(page), `<template data-flash-template="error">`) {
		t.Fatalf("the page lacks the flash template: %s", page)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	connector.SetRequestTarget(connector.NewHTMX4(nil), req, "content")

	ctx := Add(context.Background(), Success("Saved"))

	oob, err := partial.RenderWithRequest(ctx, req, root())
	if err != nil {
		t.Fatalf("partial: %v", err)
	}

	if strings.Contains(string(oob), "data-flash-template") || !strings.Contains(string(oob), "Saved") ||
		!strings.Contains(string(oob), `hx-swap-oob="beforeend"`) {
		t.Fatalf("the out-of-band flash target = %s", oob)
	}
}
