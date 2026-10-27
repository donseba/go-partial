package errors

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/exp/flash"
)

// partialRequest is an htmx request aimed at the target "content".
func partialRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "content")

	return req
}

func flashPage(policy partial.FailurePolicy, options ...Option) *partial.Partial {
	files := themeFS()
	files["toast.gohtml"] = &fstest.MapFile{
		Data: []byte(
			`{{ range .Messages }}<p class="toast toast-{{ .Level }}" data-flash-level="{{ .Level }}">{{ .Text }}</p>{{ end }}`,
		),
	}

	return partial.NewID("page", "page.gohtml").
		SetFileSystem(files).
		SetConnector(connector.NewHTMX4(nil)).
		UseTemplateCache(true).
		SetModel(site{Name: "Acme"}).
		SetFunc(map[string]any{"fail": func() (string, error) { return "", stderrors.New("boom: secret") }}).
		SetFailurePolicy(policy).
		Use(flash.Stage(flash.WithTemplate("toast.gohtml"), flash.WithTargetID("toasts")), Stage(options...)).
		SetContent(partial.NewID("content", "broken.gohtml"))
}

func TestWithFlashAnswersAFailedPartialRequestWithAFlashMessage(t *testing.T) {
	for _, policy := range []partial.FailurePolicy{partial.FailInline, partial.FailResponse} {
		rec := httptest.NewRecorder()
		err := partial.Write(context.Background(), rec, partialRequest(), flashPage(policy, WithFlash(nil)))

		var written *partial.FailureResponseError
		if !stderrors.As(err, &written) {
			t.Fatalf("policy %v: Write() error = %v", policy, err)
		}

		want := `<div id="toasts" hx-swap-oob="beforeend"><p class="toast toast-error" data-flash-level="error">` + SafeMessage + `</p></div>`
		if body := rec.Body.String(); body != want {
			t.Fatalf("policy %v: body = %q, want %q", policy, body, want)
		}

		if rec.Header().Get("HX-Reswap") != "none" {
			t.Fatalf("policy %v: headers = %v, want HX-Reswap none", policy, rec.Header())
		}
	}
}

func TestWithFlashAndDetailedMode(t *testing.T) {
	rec := httptest.NewRecorder()
	page := flashPage(partial.FailResponse, WithFlash(func(ctx *partial.RenderContext, data Data) string {
		return "Het verzoek is mislukt."
	}))

	_ = partial.Write(context.Background(), rec, partialRequest(), page)
	if !strings.Contains(rec.Body.String(), ">Het verzoek is mislukt.</p>") {
		t.Fatalf("body = %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()

	_ = partial.Write(
		context.Background(),
		rec,
		partialRequest(),
		flashPage(partial.FailResponse, WithFlash(nil), WithMode(ModeDetailed)),
	)
	if !strings.Contains(rec.Body.String(), "boom: secret") {
		t.Fatalf("detailed body = %q, want the error", rec.Body.String())
	}
}

func TestWithFlashLeavesPagesAndFailingPartsAlone(t *testing.T) {
	// A full page that fails gets the error page.
	rec := httptest.NewRecorder()

	_ = partial.Write(
		context.Background(),
		rec,
		httptest.NewRequest(http.MethodGet, "/", nil),
		flashPage(partial.FailResponse, WithFlash(nil)),
	)
	if strings.Contains(rec.Body.String(), "data-flash-level") || rec.Code != http.StatusInternalServerError {
		t.Fatalf("page = %d %q", rec.Code, rec.Body.String())
	}

	// A failing part of a page takes the error fragment in place.
	out, err := partial.Render(context.Background(), flashPage(partial.FailInline, WithFlash(nil)))
	if err != nil || !strings.Contains(string(out), "go-partial-error") {
		t.Fatalf("Render() = %q, %v", out, err)
	}

	// Without a flash stage, the error fragment answers.
	page := failingPage(WithFlash(nil))
	page.SetFailurePolicy(partial.FailInline)
	rec = httptest.NewRecorder()

	_ = partial.Write(context.Background(), rec, partialRequest(), page)
	if !strings.Contains(rec.Body.String(), "go-partial-error") {
		t.Fatalf("body without a flash stage = %q", rec.Body.String())
	}
}

func TestSafeModeHidesTheErrorMessage(t *testing.T) {
	files := themeFS()
	files["leak.gohtml"] = &fstest.MapFile{Data: []byte(`<p>{{ .Message }}|{{ .Location }}</p>`)}

	page := failingPage(WithTemplate("leak.gohtml"))
	page.SetFileSystem(files)

	rec := httptest.NewRecorder()

	_ = partial.Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)
	if body := rec.Body.String(); body != "<p>"+SafeMessage+"|</p>" {
		t.Fatalf("body = %q, want the safe message", body)
	}
}

func TestWithMessageSetsTheMessageOfEveryErrorResponse(t *testing.T) {
	calls := 0
	translate := WithMessage(func(ctx *partial.RenderContext, data Data) string {
		calls++

		if data.Detailed {
			return ""
		}

		if ctx.Request == nil || data.Message != SafeMessage {
			t.Errorf("message(%v, %q): want the request and the safe message", ctx.Request, data.Message)
		}

		return "Er ging iets mis."
	})

	files := themeFS()
	files["leak.gohtml"] = &fstest.MapFile{Data: []byte(`<p>{{ .Message }}</p>`)}

	// The error page and the error fragment.
	for _, req := range []*http.Request{httptest.NewRequest(http.MethodGet, "/", nil), partialRequest()} {
		page := failingPage(WithTemplate("leak.gohtml"), translate)
		page.SetFileSystem(files)

		calls = 0
		rec := httptest.NewRecorder()

		_ = partial.Write(context.Background(), rec, req, page)
		if body := rec.Body.String(); body != "<p>Er ging iets mis.</p>" {
			t.Fatalf("body = %q, want the translated message", body)
		}

		if calls == 0 {
			t.Fatal("message not called")
		}
	}

	// The error flash message, unless WithFlash overrides it.
	rec := httptest.NewRecorder()

	_ = partial.Write(
		context.Background(),
		rec,
		partialRequest(),
		flashPage(partial.FailResponse, WithFlash(nil), translate),
	)
	if !strings.Contains(rec.Body.String(), ">Er ging iets mis.</p>") {
		t.Fatalf("flash body = %q, want the translated message", rec.Body.String())
	}

	override := WithFlash(func(ctx *partial.RenderContext, data Data) string {
		return "Flash: " + data.Message
	})
	rec = httptest.NewRecorder()

	_ = partial.Write(context.Background(), rec, partialRequest(), flashPage(partial.FailResponse, translate, override))
	if !strings.Contains(rec.Body.String(), ">Flash: Er ging iets mis.</p>") {
		t.Fatalf("flash body = %q, want the flash message", rec.Body.String())
	}

	// An empty text keeps the error in the detailed mode.
	page := failingPage(WithTemplate("leak.gohtml"), WithMode(ModeDetailed), translate)
	page.SetFileSystem(files)

	rec = httptest.NewRecorder()

	_ = partial.Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)
	if !strings.Contains(rec.Body.String(), "boom") {
		t.Fatalf("detailed body = %q, want the error", rec.Body.String())
	}
}

func TestErrorTemplatesStandInForEachOther(t *testing.T) {
	// Only a page template: partial requests get it too.
	rec := httptest.NewRecorder()

	_ = partial.Write(context.Background(), rec, partialRequest(), failingPage(WithTemplate("500.gohtml")))
	if !strings.Contains(rec.Body.String(), "<h1>Acme is down</h1>") {
		t.Fatalf("fragment = %q, want the page template", rec.Body.String())
	}

	// Only a fragment template: full pages get it too.
	rec = httptest.NewRecorder()

	_ = partial.Write(
		context.Background(),
		rec,
		httptest.NewRequest(http.MethodGet, "/", nil),
		failingPage(WithFragmentTemplate("toast.gohtml")),
	)
	if !strings.Contains(rec.Body.String(), `<div class="toast">`) {
		t.Fatalf("page = %q, want the fragment template", rec.Body.String())
	}
}

func TestErrorPageUsesTheChildIncludesOfTheRoot(t *testing.T) {
	files := themeFS()
	files["partials/brand.gohtml"] = &fstest.MapFile{Data: []byte(`{{ define "brand" }}<b>{{ . }}</b>{{ end }}`)}
	files["branded-500.gohtml"] = &fstest.MapFile{Data: []byte(`<h1>{{ template "brand" "Acme" }} is down</h1>`)}

	page := failingPage(WithTemplate("branded-500.gohtml"))
	page.SetFileSystem(files).SetChildIncludes("partials/*.gohtml")

	rec := httptest.NewRecorder()

	_ = partial.Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)
	if body := rec.Body.String(); body != "<h1><b>Acme</b> is down</h1>" {
		t.Fatalf("body = %q, want the error page with the include", body)
	}
}
