package partial

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donseba/go-partial/connector"
)

func fallbackPage(conn connector.Connector) *Partial {
	fsys := Sources{
		"shell.gohtml":   `<body>{{ content }}</body>`,
		"content.gohtml": `<main id="content">{{ request.Header.Get "HX-Target" }}</main>`,
		"header.gohtml":  `<header id="header" {{ oobAttr }}></header>`,
	}

	page := NewID("shell", "shell.gohtml").SetFileSystem(fsys).SetConnector(conn)
	page.SetContent(NewID("content", "content.gohtml"))
	page.WithOOB(NewID("header", "header.gohtml").SetAlwaysSwapOOB(true))

	return page
}

func TestTargetFallbackAnswersUnknownTargets(t *testing.T) {
	page := fallbackPage(connector.NewHTMX4(nil)).
		SetTargetFallback(TargetFallback{ID: "content", PushURL: true})

	req := httptest.NewRequest(http.MethodGet, "/other?page=2", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "section#comments")

	rec := httptest.NewRecorder()
	if err := Write(context.Background(), rec, req, page); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	body := rec.Body.String()
	if !strings.HasPrefix(body, `<main id="content">content</main>`) || !strings.Contains(body, `id="header"`) {
		t.Fatalf("body = %q, want the content as if it was asked for, and the regions", body)
	}

	headers := map[string]string{
		"HX-Retarget": "#content",
		"HX-Reswap":   "outerHTML",
		"HX-Push-Url": "/other?page=2",
	}
	for key, want := range headers {
		if got := rec.Header().Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestTargetFallbackOptions(t *testing.T) {
	page := fallbackPage(connector.NewHTMX(nil)).SetTargetFallback(TargetFallback{
		ID:       "content",
		Retarget: "main",
		Swap:     connector.NewSwap().Style(connector.SwapInnerHTML),
		PushURL:  true,
	})

	req := httptest.NewRequest(http.MethodPost, "/save", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "form")

	rec := httptest.NewRecorder()
	if err := Write(context.Background(), rec, req, page); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if rec.Header().Get("HX-Retarget") != "main" || rec.Header().Get("HX-Reswap") != "innerHTML" {
		t.Fatalf("headers = %v", rec.Header())
	}

	if rec.Header().Get("HX-Push-Url") != "" {
		t.Fatal("HX-Push-Url set for a POST")
	}
}

func TestUnknownTargetWithoutFallbackFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "nowhere")

	_, err := RenderWithRequest(context.Background(), req, fallbackPage(connector.NewHTMX(nil)))
	if err == nil || !strings.Contains(err.Error(), "requested partial nowhere not found") {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}
}

func TestKnownTargetsIgnoreTheFallback(t *testing.T) {
	page := fallbackPage(connector.NewHTMX(nil)).SetTargetFallback(TargetFallback{ID: "content"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", "header")

	rec := httptest.NewRecorder()
	if err := Write(context.Background(), rec, req, page); err != nil {
		t.Fatal(err)
	}

	if rec.Header().Get("HX-Retarget") != "" || !strings.HasPrefix(rec.Body.String(), `<header id="header"`) {
		t.Fatalf("response = %v %q", rec.Header(), rec.Body.String())
	}
}
