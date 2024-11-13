package partial

import (
	"context"
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
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html":   "<html><body>{{.Partials.content }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
			},
		}

		p := New("templates/index.html").ID("root")

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		})
		p.With(content)

		out, err := svc.NewLayout().FS(fsys).Set(p).RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Partial", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestWrap(t *testing.T) {
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html":   "<html><body>{{.Partials.content }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
			},
		}

		index := New("templates/index.html").ID("root")

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		})

		out, err := svc.NewLayout().FS(fsys).Set(content).Wrap(index).RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Partial", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestOOB(t *testing.T) {
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html":   "<html><body>{{.Partials.content }}{{.Partials.footer }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/footer.html":  "<div {{ if _isOOB }}hx-swap-oob='true' {{ end }}id='footer'>{{.Data.Text}}</div>",
			},
		}

		p := New("templates/index.html").ID("root")

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

		out, err := svc.NewLayout().FS(fsys).Set(p).RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div><div id='footer'>This is the footer</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Partial", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div hx-swap-oob='true' id='footer'>This is the footer</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestOOBSwap(t *testing.T) {
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html":   "<html><body>{{.Partials.content }}{{.Partials.footer }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/footer.html":  "<div {{ if _isOOB }}hx-swap-oob='true' {{ end }}id='footer'>{{.Data.Text}}</div>",
			},
		}

		// the main template that will be rendered
		p := New("templates/index.html").ID("root")

		// oob footer that resides on the page
		oob := New("templates/footer.html").ID("footer")
		oob.SetData(map[string]any{
			"Text": "This is the footer",
		})
		p.WithOOB(oob)

		// the actual content required for the page
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		})

		out, err := svc.NewLayout().FS(fsys).Set(content).Wrap(p).RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("basic", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<html><body><div>Welcome to the home page</div><div id='footer'>This is the footer</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Partial", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div hx-swap-oob='true' id='footer'>This is the footer</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestDeepNested(t *testing.T) {
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html":   "<html><body>{{.Partials.content }}</body></html>",
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/nested.html":  `<div>{{ upper .Data.Text }}</div>`,
			},
		}

		p := New("templates/index.html").ID("root")

		// nested content
		nested := New("templates/nested.html").ID("nested")
		nested.SetData(map[string]any{
			"Text": "This is the nested content",
		})

		// content
		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Text": "Welcome to the home page",
		}).With(nested)

		p.With(content)

		out, err := svc.NewLayout().FS(fsys).Set(p).RenderWithRequest(r.Context(), r)
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("find nested item and return it", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Partial", "nested")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>THIS IS THE NESTED CONTENT</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestMissingPartial(t *testing.T) {
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html": "<html><body>{{.Partials.content }}</body></html>",
			},
		}

		p := New("templates/index.html").ID("root")

		out, err := svc.NewLayout().FS(fsys).Set(p).RenderWithRequest(r.Context(), r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(out))
	}

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Partial", "nonexistent")
	response := httptest.NewRecorder()

	handleRequest(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", response.Code)
	}
}

func TestDataInTemplates(t *testing.T) {
	svc := NewService(&Config{})
	svc.AddData("Title", "My Page")

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		// Create a new layout
		layout := svc.NewLayout()

		// Set LayoutData
		layout.SetData(map[string]any{
			"PageTitle": "Home Page",
			"User":      "John Doe",
		})

		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/index.html":   `<html><head><title>{{ .Service.Title }}</title></head><body>{{.Partials.content }}</body></html>`,
				"templates/content.html": `<div>{{ .Layout.PageTitle }}</div><div>{{ .Layout.User }}</div><div>{{ .Data.Articles }}</div>`,
			},
		}

		content := New("templates/content.html").ID("content")
		content.SetData(map[string]any{
			"Articles": []string{"Article 1", "Article 2", "Article 3"},
		})

		p := New("templates/index.html").ID("root")
		p.With(content)

		out, err := layout.FS(fsys).Set(p).RenderWithRequest(r.Context(), r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(out))
	}

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handleRequest(response, request)

	expected := "<html><head><title>My Page</title></head><body><div>Home Page</div><div>John Doe</div><div>[Article 1 Article 2 Article 3]</div></body></html>"
	if response.Body.String() != expected {
		t.Errorf("expected %s, got %s", expected, response.Body.String())
	}
}

func BenchmarkRenderWithRequest(b *testing.B) {
	// Setup configuration and service
	cfg := &Config{
		PartialHeader: "X-Partial",
		UseCache:      false,
	}

	// with cache    : BenchmarkRenderWithRequest-12    	  169927	      6551 ns/op
	// without cache : BenchmarkRenderWithRequest-12    	   51270	     22398 ns/op

	service := NewService(cfg)

	fsys := &InMemoryFS{
		Files: map[string]string{
			"templates/index.html":   `<html><head><title>{{ .Service.Title }}</title></head><body>{{.Partials.content }}</body></html>`,
			"templates/content.html": `<div>{{ .Layout.PageTitle }}</div><div>{{ .Layout.User }}</div><div>{{ .Data.Articles }}</div>`,
		},
	}

	// Create a new layout
	layout := service.NewLayout().FS(fsys)

	// Create content partial
	content := NewID("content", "templates/content.html")
	content.SetData(map[string]any{
		"Title":   "Benchmark Test",
		"Message": "This is a benchmark test.",
	})

	// Set the content partial in the layout
	layout.Set(content)

	// Create a sample HTTP request
	req := httptest.NewRequest("GET", "/", nil)

	// Reset the timer after setup
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Call the function you want to benchmark
		_, err := layout.RenderWithRequest(context.Background(), req)
		if err != nil {
			b.Fatalf("Error rendering: %v", err)
		}
	}
}
