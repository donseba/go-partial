package partial

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRoot(t *testing.T) {
	root := New().Templates("template.gohtml")

	if root == nil {
		t.Error("NewRoot should not return nil")
		return
	}

	if root.id != "root" {
		t.Errorf("NewRoot should have id 'root', got %s", root.id)
	}

	if len(root.templates) != 1 {
		t.Errorf("NewRoot should have 1 template, got %d", len(root.templates))
	}

	if root.templates[0] != "template.gohtml" {
		t.Errorf("NewRoot should have template 'template.gohtml', got %s", root.templates[0])
	}

	if root.globalData == nil {
		t.Error("NewRoot should have non-nil globalData")
	}

	if len(root.children) != 0 {
		t.Errorf("NewRoot should have 0 children, got %d", len(root.children))
	}

	if len(root.oobChildren) != 0 {
		t.Errorf("NewRoot should have 0 oobChildren, got %d", len(root.oobChildren))
	}

	if len(root.partials) != 0 {
		t.Errorf("NewRoot should have 0 partials, got %d", len(root.partials))
	}

	if root.functions == nil {
		t.Error("NewRoot should have non-nil functions")
	}

	if root.data == nil {
		t.Error("NewRoot should have non-nil data")
	}

	if len(root.data) != 0 {
		t.Errorf("NewRoot should have 0 data, got %d", len(root.data))
	}

	if root.Reset() != root {
		t.Error("Reset should return the partial")
	}
}

func TestRequestBasic(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			files: map[string]string{
				"templates/example.html": "<html><body>{{.Partials.content }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
			},
		}

		p := New("templates/example.html").ID("root")
		p.WithFS(fsys)

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		})
		p.With(content)

		out, err := p.RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Hx-Request", "true")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Hx-Request", "true")
		request.Header.Set("Hx-Partial", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestWrap(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			files: map[string]string{
				"templates/example.html": "<html><body>{{.Partials.content }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
			},
		}

		p := New("templates/example.html").ID("root")
		p.WithFS(fsys)

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		})
		content.Wrap(p)

		out, err := content.RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Hx-Request", "true")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestOOB(t *testing.T) {
	UseTemplateCache = false

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			files: map[string]string{
				"templates/example.html": "<html><body>{{.Partials.content }}{{.Partials.footer }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/footer.html":  "<div {{ if _isOOB }}hx-swap-oob='true' {{ end }}id='footer'>{{.Data.Text}}</div>",
			},
		}

		p := New("templates/example.html").ID("root")
		p.WithFS(fsys)

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		})
		p.With(content)

		// oob
		oob := New("templates/footer.html").ID("footer")
		oob.SetData(map[string]any{
			"Text": "This is the footer",
		})
		p.WithOOB(oob)

		out, err := p.RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Hx-Request", "true")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div><div id='footer'>This is the footer</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Hx-Request", "true")
		request.Header.Set("Hx-Partial", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div hx-swap-oob='true' id='footer'>This is the footer</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestOOBSwap(t *testing.T) {
	UseTemplateCache = false

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			files: map[string]string{
				"templates/example.html": "<html><body>{{.Partials.content }}{{.Partials.footer }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/footer.html":  "<div {{ if _isOOB }}hx-swap-oob='true' {{ end }}id='footer'>{{.Data.Text}}</div>",
			},
		}

		p := New("templates/example.html").ID("root")
		p.WithFS(fsys)

		// oob
		oob := New("templates/footer.html").ID("footer")
		oob.SetData(map[string]any{
			"Text": "This is the footer",
		})
		p.WithOOB(oob)

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		}).Wrap(p)

		out, err := content.RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Hx-Request", "true")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		// <html><body><div>Welcome to the home page</div><div id='footer'>This is the footer</div></body></html>
		// <html><body><html><body><div>Welcome to the home page</div><div id='footer'>This is the footer</div></body></html><div id='footer'>This is the footer</div></body></html>
		expected := "<html><body><div>Welcome to the home page</div><div id='footer'>This is the footer</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}
