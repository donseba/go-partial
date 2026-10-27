package errors

import (
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// respondPage is a page whose Stage has the theme's error page and an
// error fragment that shows the status and message.
func respondPage(options ...Option) *partial.Partial {
	files := themeFS()
	files["status.gohtml"] = &fstest.MapFile{Data: []byte(`<p class="status">{{ .Status }} {{ .Message }}|{{ .PartialID }}</p>`)}

	page := failingPage(append([]Option{WithTemplate("500.gohtml"), WithFragmentTemplate("status.gohtml")}, options...)...)

	return page.SetFileSystem(files)
}

// TestRespondAnswersAFullPageWithTheErrorPage checks that an error outside
// a render gets the theme's error page, with the status given, never
// cached.
func TestRespondAnswersAFullPageWithTheErrorPage(t *testing.T) {
	rec := httptest.NewRecorder()
	Respond(
		rec,
		httptest.NewRequest(http.MethodPost, "/comments", nil),
		respondPage(),
		http.StatusInternalServerError,
		stderrors.New("db down"),
	)

	if rec.Code != http.StatusInternalServerError || rec.Body.String() != "<h1>Acme is down</h1>" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}

	if rec.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("headers = %v", rec.Header())
	}
}

// TestRespondAnswersAPartialRequest checks that a partial request gets the
// error page for a 5xx status, which reloadOnError shows as the page, and
// the error fragment for another status, with the status and the message
// of WithMessage, and no partial named.
func TestRespondAnswersAPartialRequest(t *testing.T) {
	message := WithMessage(func(ctx *partial.RenderContext, data Data) string {
		return "translated " + strconv.Itoa(data.Status)
	})

	rec := httptest.NewRecorder()
	Respond(rec, partialRequest(), respondPage(message), http.StatusServiceUnavailable, stderrors.New("db down"))

	if rec.Code != http.StatusServiceUnavailable || rec.Body.String() != "<h1>Acme is down</h1>" {
		t.Fatalf("5xx = %d %q, want the error page", rec.Code, rec.Body.String())
	}

	if vary := rec.Header().Values("Vary"); !strings.Contains(strings.Join(vary, ","), "HX-Target") {
		t.Fatalf("Vary = %v, want the connector's headers", vary)
	}

	rec = httptest.NewRecorder()
	Respond(rec, partialRequest(), respondPage(message), http.StatusTooManyRequests, stderrors.New("slow down"))

	if want := `<p class="status">429 translated 429|</p>`; rec.Code != http.StatusTooManyRequests || rec.Body.String() != want {
		t.Fatalf("429 = %d %q, want %q", rec.Code, rec.Body.String(), want)
	}
}

// TestRespondWithFlash checks that a partial request gets the error flash
// message out of band with WithFlash, leaving the target as it is,
// with the status given; a full page still gets the error page.
func TestRespondWithFlash(t *testing.T) {
	rec := httptest.NewRecorder()
	Respond(
		rec,
		partialRequest(),
		flashPage(partial.FailResponse, WithFlash(nil)),
		http.StatusInternalServerError,
		stderrors.New("db down"),
	)

	want := `<div id="toasts" hx-swap-oob="beforeend"><p class="toast toast-error" data-flash-level="error">` + SafeMessage + `</p></div>`
	if rec.Code != http.StatusInternalServerError || rec.Body.String() != want {
		t.Fatalf("response = %d %q, want %q", rec.Code, rec.Body.String(), want)
	}

	if rec.Header().Get("HX-Reswap") != "none" {
		t.Fatalf("headers = %v, want HX-Reswap none", rec.Header())
	}

	rec = httptest.NewRecorder()
	Respond(
		rec,
		httptest.NewRequest(http.MethodPost, "/", nil),
		flashPage(partial.FailInline, WithFlash(nil)),
		http.StatusInternalServerError,
		stderrors.New("db down"),
	)

	if strings.Contains(rec.Body.String(), "data-flash-level") || !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("full page = %q, want the built-in error page", rec.Body.String())
	}
}

// TestRespondWithoutAStage checks that a tree without a Stage gets the
// built-in markup, in ModeSafe: the page, or the fragment for a partial
// request with a status other than 5xx.
func TestRespondWithoutAStage(t *testing.T) {
	root := partial.NewID("page", "page.gohtml").SetFileSystem(themeFS()).SetConnector(connector.NewHTMX4(nil))

	rec := httptest.NewRecorder()
	Respond(rec, httptest.NewRequest(http.MethodPost, "/", nil), root, 0, stderrors.New("secret detail"))

	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError || !strings.Contains(body, "<h1>Internal Server Error</h1>") ||
		!strings.Contains(body, SafeMessage) {
		t.Fatalf("page = %d %q", rec.Code, body)
	}

	if strings.Contains(body, "secret detail") || strings.Contains(body, "Template render error") {
		t.Fatalf("page shows the error or a template failure: %q", body)
	}

	rec = httptest.NewRecorder()
	Respond(rec, partialRequest(), root, http.StatusTooManyRequests, nil)

	body = rec.Body.String()
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(body, `class="go-partial-error"`) ||
		!strings.Contains(body, "Too Many Requests") {
		t.Fatalf("fragment = %d %q", rec.Code, body)
	}

	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", rec.Header())
	}
}

// TestWriteErrorWithoutAStageWritesNothing checks that partial.WriteError
// returns the error, and writes nothing, when no error stage answers it.
func TestWriteErrorWithoutAStageWritesNothing(t *testing.T) {
	root := partial.NewID("page", "page.gohtml").SetFileSystem(themeFS())
	cause := stderrors.New("db down")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := partial.WriteError(rec, req, root, http.StatusInternalServerError, cause); !stderrors.Is(err, cause) {
		t.Fatalf("WriteError() = %v, want the error", err)
	}

	if rec.Body.Len() != 0 || len(rec.Header()) != 0 {
		t.Fatalf("wrote %v %q", rec.Header(), rec.Body.String())
	}
}

// TestWithLang checks that the built-in error page has the language of
// WithLang, for a failed render and for an error of Respond, there with
// the text of WithMessage, that a custom template gets it as Data.Lang,
// and that the page stays en without it.
func TestWithLang(t *testing.T) {
	lang := WithLang(func(*partial.RenderContext, Data) string {
		return "nl"
	})
	message := WithMessage(func(*partial.RenderContext, Data) string {
		return "Probeer het later opnieuw."
	})

	builtIn := func(options ...Option) *partial.Partial {
		return partial.NewID("page", "page.gohtml").
			SetFileSystem(themeFS()).
			SetConnector(connector.NewHTMX(nil)).
			SetFunc(map[string]any{"fail": func() (string, error) { return "", stderrors.New("boom") }}).
			SetFailurePolicy(partial.FailResponse).
			Use(Stage(options...)).
			SetContent(partial.NewID("content", "broken.gohtml"))
	}

	rec := httptest.NewRecorder()
	_ = partial.Write(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/", nil), builtIn(lang, message))

	if body := rec.Body.String(); rec.Code != http.StatusInternalServerError || !strings.Contains(body, `<html lang="nl">`) {
		t.Fatalf("failed render = %d %q", rec.Code, body)
	}

	rec = httptest.NewRecorder()
	Respond(
		rec,
		httptest.NewRequest(http.MethodPost, "/", nil),
		builtIn(lang, message),
		http.StatusInternalServerError,
		stderrors.New("db down"),
	)

	if body := rec.Body.String(); !strings.Contains(body, `<html lang="nl">`) ||
		!strings.Contains(body, "<p>Probeer het later opnieuw.</p>") {
		t.Fatalf("Respond = %q, want lang nl and the message", body)
	}

	rec = httptest.NewRecorder()
	Respond(
		rec,
		httptest.NewRequest(http.MethodPost, "/", nil),
		builtIn(),
		http.StatusInternalServerError,
		stderrors.New("db down"),
	)

	if body := rec.Body.String(); !strings.Contains(body, `<html lang="en">`) {
		t.Fatalf("without WithLang = %q, want lang en", body)
	}

	files := themeFS()
	files["lang.gohtml"] = &fstest.MapFile{Data: []byte(`<html lang="{{ .Lang }}">{{ .Message }}</html>`)}

	rec = httptest.NewRecorder()
	Respond(rec, httptest.NewRequest(http.MethodPost, "/", nil), builtIn(WithTemplate("lang.gohtml"), lang).SetFileSystem(files),
		http.StatusInternalServerError, stderrors.New("db down"))

	if want := `<html lang="nl">` + SafeMessage + `</html>`; rec.Body.String() != want {
		t.Fatalf("custom template = %q, want %q", rec.Body.String(), want)
	}
}

// TestSafeModeShowsNoInternals checks that, in ModeSafe, the built-in page
// and fragment of a failed render name no partial, template or error, and
// show the status text, the text of WithMessage and the language of
// WithLang, and that ModeDetailed shows them all.
func TestSafeModeShowsNoInternals(t *testing.T) {
	lang := WithLang(func(*partial.RenderContext, Data) string {
		return "nl"
	})
	message := WithMessage(func(_ *partial.RenderContext, data Data) string {
		if data.Detailed {
			return ""
		}

		return "Probeer het later opnieuw."
	})

	internals := []string{"Template render error", "Partial ID", ".gohtml", "boom"}

	requests := map[string]func() *http.Request{
		"page": func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/", nil)
		},
		"fragment": func() *http.Request {
			return partialRequest()
		},
	}

	for name, request := range requests {
		rec := httptest.NewRecorder()
		_ = partial.Write(t.Context(), rec, request(), failingPage(lang, message))

		body := rec.Body.String()
		for _, internal := range internals {
			if strings.Contains(body, internal) {
				t.Fatalf("safe %s shows %q: %q", name, internal, body)
			}
		}

		if !strings.Contains(body, "Internal Server Error</h1>") || !strings.Contains(body, "Probeer het later opnieuw.</p>") {
			t.Fatalf("safe %s = %q, want the status text and the message", name, body)
		}

		if name == "page" && !strings.Contains(body, `<html lang="nl">`) {
			t.Fatalf("safe page = %q, want lang nl", body)
		}

		rec = httptest.NewRecorder()
		_ = partial.Write(t.Context(), rec, request(), failingPage(WithMode(ModeDetailed), message))

		body = rec.Body.String()
		for _, internal := range internals {
			if !strings.Contains(body, internal) {
				t.Fatalf("detailed %s lacks %q: %q", name, internal, body)
			}
		}
	}
}

// TestSafeModeDataNamesNoPartial checks that the Data of a failed render
// names the partial that failed in ModeDetailed only.
func TestSafeModeDataNamesNoPartial(t *testing.T) {
	ctx := &partial.RenderContext{
		Kind:    RenderKindError,
		Partial: partial.NewID("content", "broken.gohtml"),
		Error:   stderrors.New("boom"),
	}

	safe := BuildData(ctx, ModeSafe)
	if safe.PartialID != "" || len(safe.Templates) != 0 || safe.Title != "Internal Server Error" {
		t.Fatalf("safe data = %+v", safe)
	}

	if data := BuildData(ctx, ModeDetailed); data.PartialID != "content" || len(data.Templates) != 1 {
		t.Fatalf("detailed data = %+v", data)
	}
}

// TestWithTitle checks that the built-in page and fragment have the
// heading of WithTitle, for a failed render and for an error of Respond,
// that a custom template gets it as Data.Title, and that a detailed
// failed render keeps its own heading.
func TestWithTitle(t *testing.T) {
	title := WithTitle(func(_ *partial.RenderContext, data Data) string {
		return "Fout " + strconv.Itoa(data.Status) + " (" + data.Title + ")"
	})

	rec := httptest.NewRecorder()
	_ = partial.Write(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/", nil), failingPage(title))

	want := "Fout 500 (Internal Server Error)"
	if body := rec.Body.String(); !strings.Contains(body, "<title>"+want+"</title>") ||
		!strings.Contains(body, "<h1>"+want+"</h1>") {
		t.Fatalf("failed render = %q, want the title", body)
	}

	rec = httptest.NewRecorder()
	Respond(rec, partialRequest(), failingPage(title), http.StatusTooManyRequests, stderrors.New("slow down"))

	if body := rec.Body.String(); !strings.Contains(body, "Fout 429 (Too Many Requests)</h1>") {
		t.Fatalf("Respond fragment = %q, want the title", body)
	}

	files := themeFS()
	files["title.gohtml"] = &fstest.MapFile{Data: []byte(`<h1>{{ .Title }}</h1>`)}

	rec = httptest.NewRecorder()
	Respond(
		rec,
		httptest.NewRequest(http.MethodPost, "/", nil),
		failingPage(WithTemplate("title.gohtml"), title).SetFileSystem(files),
		http.StatusServiceUnavailable,
		stderrors.New("db down"),
	)

	if want := "<h1>Fout 503 (Service Unavailable)</h1>"; rec.Body.String() != want {
		t.Fatalf("custom template = %q, want %q", rec.Body.String(), want)
	}

	rec = httptest.NewRecorder()
	_ = partial.Write(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/", nil), failingPage(WithMode(ModeDetailed), title))

	if body := rec.Body.String(); !strings.Contains(body, "<h1>Template render error</h1>") || strings.Contains(body, "Fout") {
		t.Fatalf("detailed failed render = %q, want its own heading", body)
	}
}
