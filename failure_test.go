package partial

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donseba/go-partial/connector"
)

func failurePage() *Partial {
	fsys := Sources{
		"shell.gohtml":  `<main>{{ content }}</main>`,
		"broken.gohtml": `<p>{{ index . 3 }}</p>`,
		"card.gohtml":   `<p>{{ partial runtime "nope.gohtml" }}</p>`,
	}
	return NewID("shell", "shell.gohtml").SetFileSystem(fsys)
}

func TestFailInlineRendersTheRestOfThePage(t *testing.T) {
	page := failurePage().SetContent(NewID("content", "broken.gohtml").SetDot([]int{}))

	rec := httptest.NewRecorder()
	if err := Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page); err != nil {
		t.Fatalf("Write() error = %v, want the page with the failure inline", err)
	}

	if !strings.HasPrefix(rec.Body.String(), "<main>") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestFailResponseFailsTheWholeResponse(t *testing.T) {
	tests := map[string]*Partial{
		"failing content":  NewID("content", "broken.gohtml").SetDot([]int{}),
		"missing template": NewID("content", "card.gohtml"),
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			page := failurePage().SetFailurePolicy(FailResponse).SetContent(content)
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			rec := httptest.NewRecorder()
			if err := Write(context.Background(), rec, req, page); err == nil {
				t.Fatal("Write() error = nil, want the failure")
			}

			if rec.Body.Len() != 0 {
				t.Fatalf("body = %q, want nothing written", rec.Body.String())
			}

			out, err := RenderWithRequest(context.Background(), req, page)
			if err == nil || out != "" {
				t.Fatalf("RenderWithRequest() = %q, %v; want an error and no HTML", out, err)
			}

			if _, err := Render(context.Background(), page); err == nil {
				t.Fatal("Render() error = nil, want the failure")
			}
		})
	}
}

func TestFailResponseWritesTheErrorStageResponse(t *testing.T) {
	page := failurePage().
		SetFailurePolicy(FailResponse).
		Use(testErrorStage(true)).
		SetContent(NewID("content", "broken.gohtml").SetDot([]int{}))

	rec := httptest.NewRecorder()

	err := Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)
	if err == nil || !strings.Contains(err.Error(), "render content") {
		t.Fatalf("Write() error = %v, want the content's failure", err)
	}

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "<main>") {
		t.Fatalf("response = %d %q, want the error page alone", rec.Code, rec.Body.String())
	}

	var written *FailureResponseError
	if !errors.As(err, &written) || written.Status != http.StatusInternalServerError {
		t.Fatalf("Write() error = %#v, want a *FailureResponseError with status 500", err)
	}

	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestFailResponseKeepsAnErrorStatusForPartialRequests(t *testing.T) {
	// The error stage answers partial requests with a fragment and status
	// 200, which FailInline swaps in place.
	fragmentOK := RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			if ctx.Kind == renderKindError && ctx.Name == "fragment" {
				ctx.Response.Status = http.StatusOK
			}

			return ctx, nil
		},
	}
	page := func(policy FailurePolicy) *Partial {
		return failurePage().
			SetConnector(connector.NewHTMX(nil)).
			SetFailurePolicy(policy).
			Use(testErrorStage(true), fragmentOK).
			SetContent(NewID("content", "broken.gohtml").SetDot([]int{}))
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "content")

	rec := httptest.NewRecorder()
	err := Write(context.Background(), rec, req, page(FailResponse))

	var written *FailureResponseError
	if !errors.As(err, &written) {
		t.Fatalf("Write() error = %v, want a *FailureResponseError", err)
	}

	if rec.Code != http.StatusInternalServerError || written.Status != rec.Code {
		t.Fatalf("status = %d (error %d), want 500", rec.Code, written.Status)
	}

	if body := rec.Body.String(); !strings.Contains(body, "go-partial-error") {
		t.Fatalf("body = %q, want the error fragment", body)
	}

	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}

	// FailInline answers the targeted content that fails as before.
	rec = httptest.NewRecorder()

	err = Write(context.Background(), rec, req, page(FailInline))
	if !errors.As(err, &written) || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go-partial-error") {
		t.Fatalf("FailInline: %d %q, %v; want the fragment with status 200", rec.Code, rec.Body.String(), err)
	}
}

func TestWriteWithoutAnErrorStageWritesNothing(t *testing.T) {
	page := failurePage().SetFailurePolicy(FailResponse).SetContent(NewID("content", "broken.gohtml").SetDot([]int{}))

	rec := httptest.NewRecorder()
	err := Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page)

	var written *FailureResponseError
	if err == nil || errors.As(err, &written) || rec.Body.Len() != 0 {
		t.Fatalf("Write() = %q, %v; want the plain error and nothing written", rec.Body.String(), err)
	}
}

func TestRenderSelfIgnoresTheRequestedTarget(t *testing.T) {
	fsys := Sources{"page.gohtml": `page:{{ request.URL.Path }}`, "row.gohtml": `row`}
	page := NewID("page", "page.gohtml").SetFileSystem(fsys).SetConnector(connector.NewHTMX(nil))
	page.With(NewID("row", "row.gohtml"))

	req := httptest.NewRequest(http.MethodGet, "/rows", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "row")

	if out, err := RenderWithRequest(context.Background(), req, page); err != nil || out != "row" {
		t.Fatalf("RenderWithRequest() = %q, %v", out, err)
	}

	if out, err := RenderSelf(context.Background(), req, page); err != nil || out != "page:/rows" {
		t.Fatalf("RenderSelf() = %q, %v", out, err)
	}
}

func TestWriteAddsVaryHeaders(t *testing.T) {
	page := NewID("page", "page.gohtml").
		SetFileSystem(Sources{"page.gohtml": `page`}).
		SetConnector(connector.NewHTMX4(nil))

	rec := httptest.NewRecorder()
	rec.Header().Add("Vary", "Accept-Encoding, hx-request")

	if err := Write(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), page); err != nil {
		t.Fatal(err)
	}

	vary := strings.Join(rec.Header().Values("Vary"), ", ")
	for _, header := range []string{"HX-Target", "HX-Request-Type", "HX-History-Restore-Request"} {
		if !strings.Contains(vary, header) {
			t.Fatalf("Vary = %q, want %s", vary, header)
		}
	}

	if strings.Count(strings.ToLower(vary), "hx-request,")+strings.Count(strings.ToLower(vary), "hx-request ") > 1 {
		t.Fatalf("Vary = %q, lists HX-Request twice", vary)
	}
}
