package flash

import (
	"context"
	"encoding/base64"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	partial "github.com/donseba/go-partial"
)

// renderWithFuncMap renders src with FuncMap and without the flash stage,
// so the package functions run with the render context the template
// passes them.
func renderWithFuncMap(t *testing.T, ctx context.Context, src string) string {
	t.Helper()

	page := partial.NewID("page", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": src,
	}))
	page.SetFunc(FuncMap())

	out, err := partial.Render(ctx, page)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	return string(out)
}

// TestFuncMapHelpersTakeTheRenderContext checks the helpers of FuncMap
// without the stage, given the render context with ctx.
func TestFuncMapHelpersTakeTheRenderContext(t *testing.T) {
	ctx := Add(context.Background(), Success("Saved"))

	got := renderWithFuncMap(t, ctx, `{{ flash ctx }}|{{ flashTarget ctx }}|{{ len (flashes ctx) }}|{{ hasFlashes ctx }}`)

	parts := strings.Split(got, "|")
	if len(parts) != 4 {
		t.Fatalf("output = %q", got)
	}

	if !strings.Contains(parts[0], `data-flash-level="success"`) || !strings.Contains(parts[0], "Saved") {
		t.Fatalf("flash = %q", parts[0])
	}

	if !strings.Contains(parts[1], `id="flash-messages"`) || !strings.Contains(parts[1], "partial-flash-target") {
		t.Fatalf("flashTarget = %q", parts[1])
	}

	if parts[2] != "1" || parts[3] != "true" {
		t.Fatalf("flashes = %q, hasFlashes = %q", parts[2], parts[3])
	}

	empty := renderWithFuncMap(t, context.Background(), `[{{ flash ctx }}]{{ hasFlashes ctx }}`)
	if empty != "[]false" {
		t.Fatalf("output without messages = %q", empty)
	}
}

func TestFuncMapHelpersWithoutARenderContext(t *testing.T) {
	if Flash() != "" || FlashTarget() != "" || Flashes() != nil || HasFlashes() {
		t.Fatal("helpers without a render context rendered something")
	}

	if Template("info") != "" {
		t.Fatal("Template() without a render context rendered something")
	}

	empty := &partial.RenderContext{}
	if Flash(empty) != "" || FlashTarget(empty) != "" {
		t.Fatal("helpers with a render context without a runtime rendered something")
	}

	if got := renderOOB(empty, nil, defaultTargetID, []Message{Info("x")}); got != "" {
		t.Fatalf("renderOOB() without a runtime = %q", got)
	}

	if out, ok := RenderOOB(nil, Info("x")); ok || out != "" {
		t.Fatalf("RenderOOB(nil) = %q, %v", out, ok)
	}
}

// TestTemplateTakesLevels checks that flashTemplate renders the templates of
// the levels given as strings and as Levels, normalized.
func TestTemplateTakesLevels(t *testing.T) {
	page := partial.NewID("page", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `{{ flashTemplate ctx level " Error " }}`,
	}))
	page.SetFunc(template.FuncMap{
		"flashTemplate": Template,
		"level": func() Level {
			return LevelSuccess
		},
	})

	out, err := partial.Render(context.Background(), page)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got := string(out)
	if strings.Count(got, "<template data-flash-template=") != 2 {
		t.Fatalf("output = %q, want two templates", got)
	}

	if !strings.Contains(got, `<template data-flash-template="success">`) || !strings.Contains(got, `<template data-flash-template="error">`) {
		t.Fatalf("output = %q", got)
	}

	if !strings.Contains(got, textMarker) {
		t.Fatalf("output = %q, want the text marker", got)
	}
}

// TestStageWithPartialsAndHelpers checks the stage's flashes and hasFlashes,
// and partials given with WithPartial and WithTargetPartial; nil options
// and partials are ignored.
func TestStageWithPartialsAndHelpers(t *testing.T) {
	fsys := testFS(map[string]string{
		"page.gohtml":   `{{ range flashes }}[{{ .Level }} {{ .Text }}]{{ end }}{{ hasFlashes }}{{ flash }}{{ flashTarget }}`,
		"msgs.gohtml":   `<ol>{{ range .Messages }}<li>{{ .Text }}</li>{{ end }}</ol>`,
		"target.gohtml": `<aside id="{{ .TargetID }}"></aside>`,
	})

	page := partial.NewID("page", "page.gohtml").SetFileSystem(fsys)
	page.SetFunc(FuncMap())
	page.Use(Stage(
		nil,
		WithPartial(nil),
		WithTargetPartial(nil),
		WithTemplate("  "),
		WithTargetTemplate(""),
		WithTargetID("###"),
		WithPartial(partial.NewID("msgs", "msgs.gohtml")),
		WithTargetPartial(partial.NewID("target", "target.gohtml")),
	))

	ctx := Add(context.Background(), Info("One"), Error("Two"))

	out, err := partial.Render(ctx, page)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	want := `[info One][error Two]true<ol><li>One</li><li>Two</li></ol><aside id="flash-messages"></aside>`
	if got := string(out); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestFlashReportsRenderErrors checks that a flash template that fails is
// reported in place of the messages, escaped.
func TestFlashReportsRenderErrors(t *testing.T) {
	fsys := testFS(map[string]string{
		"page.gohtml":  `{{ flash }}`,
		"flash.gohtml": `{{ .Missing.Field }}`,
	})

	page := partial.NewID("page", "page.gohtml").SetFileSystem(fsys)
	page.SetFunc(FuncMap())
	page.Use(Stage(WithTemplate("flash.gohtml")))

	out, err := partial.Render(Add(context.Background(), Info("x")), page)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if got := string(out); !strings.HasPrefix(got, "error rendering flash: ") {
		t.Fatalf("output = %q, want the render error", got)
	}
}

func TestContextHelpersWithoutAStore(t *testing.T) {
	//nolint:staticcheck // A nil context is what the helpers guard against.
	if FromContext(nil) != nil {
		t.Fatal("FromContext(nil) is not nil")
	}

	if Messages(context.Background()) != nil || Drain(context.Background()) != nil || Has(context.Background()) {
		t.Fatal("a context without a store has messages")
	}

	//nolint:staticcheck // A nil context is what the helpers guard against.
	ctx := Add(nil, Info("x"))
	if got := Messages(ctx); len(got) != 1 {
		t.Fatalf("Add(nil) messages = %v", got)
	}

	//nolint:staticcheck // A nil context is what the helpers guard against.
	withStore := WithStore(nil, nil)
	if FromContext(withStore) == nil {
		t.Fatal("WithStore(nil, nil) has no store")
	}

	var store *Store
	store.Add(Info("x"))

	if store.Messages() != nil || store.Drain() != nil {
		t.Fatal("a nil store has messages")
	}
}

func TestNormalizers(t *testing.T) {
	levels := map[Level]Level{
		"":            LevelInfo,
		"  ":          LevelInfo,
		"WARN":        LevelWarn,
		"a--b":        "a-b",
		"Big  Alert!": "big-alert",
	}
	for in, want := range levels {
		if got := normalizeLevel(in); got != want {
			t.Errorf("normalizeLevel(%q) = %q, want %q", in, got, want)
		}
	}

	ids := map[string]string{
		"#notices": "notices",
		"_x":       "x",
		"9lives":   "flash-9lives",
		"#":        "",
		"!!!":      "",
	}
	for in, want := range ids {
		if got := normalizeTargetID(in); got != want {
			t.Errorf("normalizeTargetID(%q) = %q, want %q", in, got, want)
		}
	}

	if firstRune("") != 0 {
		t.Fatal("firstRune(\"\") is not 0")
	}

	if got := normalized([]Message{{Text: " "}, {Level: "WARN", Text: "x"}}); len(got) != 1 || got[0].Level != LevelWarn {
		t.Fatalf("normalized() = %v", got)
	}
}

// TestCookieEdgeCases checks Persist without messages or a writer, Restore
// and Read without a request or cookie, and values that do not decode.
func TestCookieEdgeCases(t *testing.T) {
	cookie := Cookie{Name: "notice", Path: "/app", Domain: "example.com", MaxAge: 30 * time.Second, Secure: true, SameSite: http.SameSiteStrictMode}

	cookie.Persist(nil, Info("x"))

	rec := httptest.NewRecorder()
	cookie.Persist(rec, Info("  "))

	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 || cleared[0].Value != "" {
		t.Fatalf("Persist() without messages set %v, want the cookie cleared", cleared)
	}

	rec = httptest.NewRecorder()
	cookie.Persist(rec, Info("kept"))

	set := rec.Result().Cookies()
	if len(set) != 1 || set[0].MaxAge != 30 || set[0].Path != "/app" || set[0].Domain != "example.com" || !set[0].Secure ||
		set[0].SameSite != http.SameSiteStrictMode || !set[0].HttpOnly {
		t.Fatalf("Persist() set %+v", set)
	}

	if cookie.Restore(httptest.NewRecorder(), nil) != nil {
		t.Fatal("Restore(nil) is not nil")
	}

	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := cookie.Restore(httptest.NewRecorder(), plain); got != plain {
		t.Fatal("Restore() of a request without the cookie returned another request")
	}

	for name, value := range map[string]string{
		"not base64":  "!!!",
		"not JSON":    base64.RawURLEncoding.EncodeToString([]byte("{")),
		"blank texts": base64.RawURLEncoding.EncodeToString([]byte(`[{"l":"info","t":" "}]`)),
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: "flash", Value: value})

		if got := (Cookie{}).Read(r); len(got) != 0 {
			t.Errorf("%s: Read() = %v, want no messages", name, got)
		}

		restored := Cookie{}.Restore(httptest.NewRecorder(), r)
		if Has(restored.Context()) {
			t.Errorf("%s: Restore() added messages", name)
		}
	}
}

func TestTruncateKeepsWholeCharacters(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("truncate() = %q", got)
	}

	if got := truncate("aé", 2); got != "a" {
		t.Fatalf("truncate() = %q, want the split character dropped", got)
	}
}

// TestStageFallsBackToTheDefaults checks that an option that clears the
// partials and target ID leaves the stage with the default ones.
func TestStageFallsBackToTheDefaults(t *testing.T) {
	reset := func(opts *options) {
		opts.partial = nil
		opts.targetPartial = nil
		opts.targetID = ""
	}

	page := partial.NewID("page", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `{{ flash }}{{ flashTarget }}`,
	}))
	page.SetFunc(FuncMap())
	page.Use(Stage(reset))

	out, err := partial.Render(Add(context.Background(), Warn("Careful")), page)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got := string(out)
	if !strings.Contains(got, `data-flash-level="warn"`) || !strings.Contains(got, `id="flash-messages"`) {
		t.Fatalf("output = %q, want the default message and target templates", got)
	}
}

// TestRenderOOBIgnoresAnotherFunction checks that RenderOOB reports false
// when the name it looks up is a function of another type.
func TestRenderOOBIgnoresAnotherFunction(t *testing.T) {
	var ok bool

	probe := partial.RenderStageHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			ctx.SetFunc(oobFuncName, func() string {
				return "other"
			})

			_, ok = RenderOOB(ctx, Info("x"))

			return ctx, nil
		},
	}

	page := partial.NewID("page", "page.gohtml").SetFileSystem(testFS(map[string]string{
		"page.gohtml": `page`,
	}))
	page.Use(probe)

	if _, err := partial.Render(context.Background(), page); err != nil {
		t.Fatalf("render: %v", err)
	}

	if ok {
		t.Fatal("RenderOOB() = true for a function of another type")
	}
}

func TestStoreSkipsBlankMessages(t *testing.T) {
	store := NewStore(Info(" "), Success("Saved"), Message{Text: "\t"})

	got := store.Messages()
	if len(got) != 1 || got[0] != Success("Saved") {
		t.Fatalf("Messages() = %v", got)
	}
}
