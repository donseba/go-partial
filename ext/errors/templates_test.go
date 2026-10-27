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
)

type site struct {
	Name string
}

func themeFS() fstest.MapFS {
	return fstest.MapFS{
		"page.gohtml":   {Data: []byte(`<main>{{ content }}</main>`)},
		"broken.gohtml": {Data: []byte(`{{ fail }}`)},
		"500.gohtml": {
			Data: []byte(
				"{{/*\n@model Site github.com/donseba/go-partial/ext/errors.site\n@dot github.com/donseba/go-partial/ext/errors.Data\n*/}}" +
					`<h1>{{ Site.Name }} is down</h1>{{ if .Detailed }}{{ .Message }}{{ end }}`,
			),
		},
		"toast.gohtml":      {Data: []byte(`<div class="toast">{{ .PartialID }} failed</div>`)},
		"broken-500.gohtml": {Data: []byte(`{{ .Missing.Field }}`)},
	}
}

func failingPage(options ...Option) *partial.Partial {
	content := partial.NewID("content", "broken.gohtml")

	return partial.NewID("page", "page.gohtml").
		SetFileSystem(themeFS()).
		SetConnector(connector.NewHTMX(nil)).
		UseTemplateCache(true).
		SetModel(site{Name: "Acme"}).
		SetFunc(map[string]any{"fail": func() (string, error) { return "", stderrors.New("boom") }}).
		SetFailurePolicy(partial.FailResponse).
		Use(Stage(options...)).
		SetContent(content)
}

func TestWithTemplateRendersTheThemesErrorPage(t *testing.T) {
	rec := httptest.NewRecorder()
	err := partial.Write(
		context.Background(),
		rec,
		httptest.NewRequest(http.MethodGet, "/", nil),
		failingPage(WithTemplate("500.gohtml")),
	)

	var written *partial.FailureResponseError
	if !stderrors.As(err, &written) {
		t.Fatalf("Write() error = %v, want a failure response", err)
	}

	if rec.Code != http.StatusInternalServerError || rec.Body.String() != "<h1>Acme is down</h1>" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestWithFragmentTemplateRendersTheErrorFragment(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "content")

	rec := httptest.NewRecorder()
	page := failingPage(WithTemplate("500.gohtml"), WithFragmentTemplate("toast.gohtml"))
	_ = partial.Write(context.Background(), rec, req, page)

	if !strings.Contains(rec.Body.String(), `<div class="toast">`) {
		t.Fatalf("body = %q, want the fragment template", rec.Body.String())
	}
}

func TestFailingErrorTemplateFallsBackToTheBuiltInPage(t *testing.T) {
	var warned bool

	page := failingPage(WithTemplate("broken-500.gohtml")).
		SetEvents(partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
			if event.Kind == EventTemplateFailed {
				warned = true
			}
		}))

	rec := httptest.NewRecorder()
	_ = partial.Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)

	if !strings.Contains(rec.Body.String(), "<h1>Internal Server Error</h1>") || !warned {
		t.Fatalf("body = %q, warned = %v; want the built-in page and a warning", rec.Body.String(), warned)
	}
}

func TestFromContextMarksTheRenderOfAnErrorTemplate(t *testing.T) {
	var outside, inside []int

	mark := func(ctx context.Context) string {
		data, ok := FromContext(ctx)
		if !ok {
			outside = append(outside, 0)
			return "page"
		}

		inside = append(inside, data.Status)

		return "error"
	}

	files := themeFS()
	files["page.gohtml"] = &fstest.MapFile{Data: []byte(`{{ mark }}<main>{{ content }}</main>`)}
	files["500.gohtml"] = &fstest.MapFile{Data: []byte(`<h1>{{ mark }}</h1>`)}

	page := failingPage(WithTemplate("500.gohtml")).
		SetFileSystem(files).
		SetFunc(map[string]any{
			"fail": func() (string, error) { return "", stderrors.New("boom") },
			"mark": mark,
		})

	rec := httptest.NewRecorder()
	_ = partial.Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)

	if rec.Body.String() != "<h1>error</h1>" {
		t.Fatalf("body = %q, want the error template to see the error render", rec.Body.String())
	}

	if len(inside) == 0 {
		t.Fatal("the error template's render was not marked")
	}

	for _, status := range inside {
		if status != http.StatusInternalServerError {
			t.Fatalf("error render statuses = %v, want 500", inside)
		}
	}

	if len(outside) == 0 {
		t.Fatal("the page's own render was marked as an error render")
	}
}

func TestFromContextWithoutAnErrorRender(t *testing.T) {
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("FromContext(background) reported an error render")
	}

	if _, ok := FromContext(nil); ok { //nolint:staticcheck // a nil context is reported as no error render.
		t.Fatal("FromContext(nil) reported an error render")
	}
}
