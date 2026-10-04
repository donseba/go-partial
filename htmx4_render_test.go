package partial

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/donseba/go-partial/connector"
)

func TestHTMX4DocumentAndFragmentRendering(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml": {Data: []byte(`<!doctype html><html><body>{{content}}</body></html>`)},
		"editor.gohtml": {Data: []byte(`<main id="content">Invalid title</main>`)},
	}

	for _, tc := range []struct {
		name, requestType, target, history string
		full                               bool
	}{
		{"targeted editor", "partial", "main#content", "", false},
		{"boosted body", "full", "body", "", true},
		{"select", "full", "main#content", "", true},
		{"history", "full", "body", "true", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(connector.NewHTMX4(nil))
			root.SetContent(NewID("content", "editor.gohtml").SetStatus(422))
			root.SetStatus(422)
			r := httptest.NewRequest("POST", "/editor", nil)
			r.Header.Set("HX-Request", "true")
			r.Header.Set("HX-Request-Type", tc.requestType)
			r.Header.Set("HX-Target", tc.target)
			r.Header.Set("HX-History-Restore-Request", tc.history)
			w := httptest.NewRecorder()

			if err := Write(context.Background(), w, r, root); err != nil {
				t.Fatal(err)
			}

			body := w.Body.String()
			if w.Code != 422 || !strings.Contains(body, `id="content"`) || strings.Contains(body, "<!doctype html>") != tc.full {
				t.Fatalf("status/document/target mismatch: %d %s", w.Code, body)
			}
		})
	}
}

func TestHTMX4WritesConnectorSpecificHeadersAndOOB(t *testing.T) {
	files := fstest.MapFS{
		"layout.gohtml":  {Data: []byte(`<!doctype html>{{content}}`)},
		"content.gohtml": {Data: []byte(`<main id="content">Saved</main>`)},
		"notice.gohtml":  {Data: []byte(`<aside id="notice"{{oobAttr}}>Updated</aside>`)},
	}
	root := NewID("shell", "layout.gohtml").SetFileSystem(files).SetConnector(connector.NewHTMX4(nil))
	root.SetContent(NewID("content", "content.gohtml"))
	root.WithOOB(NewID("notice", "notice.gohtml").SetAlwaysSwapOOB(true))
	root.Response().Retarget("#content").
		ReswapWith(connector.NewSwap().Style(connector.SwapOuterHTML).Swap(120*time.Millisecond).Transition(true).Show("#notice", "top")).
		TriggerWith(connector.NewTrigger().AddEventObject("showcase:notice", map[string]any{"message": "Saved"}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Request-Type", "partial")
	r.Header.Set("HX-Target", "main#content")
	w := httptest.NewRecorder()

	if err := Write(context.Background(), w, r, root); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(w.Body.String(), `id="notice" hx-swap-oob="true"`) || strings.Contains(w.Body.String(), "<!doctype") {
		t.Fatalf("fragment/OOB mismatch: %s", w.Body.String())
	}

	if got := w.Header().Get("HX-Reswap"); got != `outerHTML swap:120ms transition:true show:top showTarget:"#notice"` {
		t.Fatalf("semantic swap did not reach selected connector: %q", got)
	}

	if w.Header().Get("HX-Retarget") != "#content" || w.Header().Get("HX-Trigger") != `{"showcase:notice":{"message":"Saved"}}` {
		t.Fatalf("response headers: %v", w.Header())
	}
}
