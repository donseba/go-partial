package partial

import (
	"context"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/donseba/go-partial/connector"
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
				"templates/index.html":   `<html><body>{{ child "content" }}</body></html>`,
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
		request.Header.Set("X-Target", "content")
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
				"templates/index.html":   `<html><body>{{ child "content" }}</body></html>`,
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
		request.Header.Set("X-Target", "content")
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
				"templates/index.html":   `<html><body>{{ child "content" }}{{ child "footer" }}</body></html>`,
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/footer.html":  "<div {{ oobSwapIfEnabled \"true\"}} id='footer'>{{.Data.Text}}</div>",
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

		expected := "<html><body><div>Welcome to the home page</div><div  id='footer'>This is the footer</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Target", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div x-swap-oob=\"true\" id='footer'>This is the footer</div>"
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
				"templates/index.html":   `<html><body>{{ child "content" }}{{ child "footer" }}</body></html>`,
				"templates/content.html": "<div>{{.Data.Text}}</div>",
				"templates/footer.html":  "<div {{ oobSwapIfEnabled \"true\" }} id='footer'>{{.Data.Text}}</div>",
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

		expected := "<html><body><div>Welcome to the home page</div><div  id='footer'>This is the footer</div></body></html>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})

	t.Run("partial", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("X-Target", "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div x-swap-oob=\"true\" id='footer'>This is the footer</div>"
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
				"templates/index.html":   `<html><body>{{ child "content" }}</body></html>`,
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
		request.Header.Set("X-Target", "nested")
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
				"templates/index.html": `<html><body>{{ child "content" }}</body></html>`,
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
	request.Header.Set("X-Target", "nonexistent")
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
				"templates/index.html":   `<html><head><title>{{ .Service.Title }}</title></head><body>{{ child "content" }}</body></html>`,
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

func TestWithSelectMap(t *testing.T) {
	fsys := &InMemoryFS{
		Files: map[string]string{
			"index.gohtml":   `<html><body>{{ child "content" }}</body></html>`,
			"content.gohtml": `<div class="content">{{selection}}</div>`,
			"tab1.gohtml":    "Tab 1 Content",
			"tab2.gohtml":    "Tab 2 Content",
			"default.gohtml": "Default Tab Content",
		},
	}

	// Create a map of selection keys to partials
	partialsMap := map[string]*Partial{
		"tab1":    New("tab1.gohtml").ID("tab1"),
		"tab2":    New("tab2.gohtml").ID("tab2"),
		"default": New("default.gohtml").ID("default"),
	}

	// Create the content partial with the selection map
	contentPartial := New("content.gohtml").
		ID("content").
		WithSelectMap("default", partialsMap)

	// Create the layout partial
	index := New("index.gohtml")

	// Set up the service and layout
	svc := NewService(&Config{
		FS:        fsys, // Set the file system in the service config
		Connector: connector.NewPartial(nil),
	})

	layout := svc.NewLayout().
		Set(contentPartial).
		Wrap(index)

	// Set up a test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.TODO()
		err := layout.WriteWithRequest(ctx, w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Define test cases
	testCases := []struct {
		name            string
		selectHeader    string
		expectedContent string
	}{
		{
			name:            "Select tab1",
			selectHeader:    "tab1",
			expectedContent: "Tab 1 Content",
		},
		{
			name:            "Select tab2",
			selectHeader:    "tab2",
			expectedContent: "Tab 2 Content",
		},
		{
			name:            "Select default",
			selectHeader:    "",
			expectedContent: "Default Tab Content",
		},
		{
			name:            "Invalid selection",
			selectHeader:    "invalid",
			expectedContent: "selected partial 'invalid' not found in parent 'content'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tc.selectHeader != "" {
				req.Header.Set("X-Select", tc.selectHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Read response body
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}
			bodyString := string(bodyBytes)

			// Check if the expected content is in the response
			if !strings.Contains(bodyString, tc.expectedContent) {
				t.Errorf("Expected response to contain %q, but got %q", tc.expectedContent, bodyString)
			}
		})
	}
}

func BenchmarkWithSelectMap(b *testing.B) {
	fsys := &InMemoryFS{
		Files: map[string]string{
			"index.gohtml":   `<html><body>{{ child "content" }}</body></html>`,
			"content.gohtml": `<div class="content">{{selection}}</div>`,
			"tab1.gohtml":    "Tab 1 Content",
			"tab2.gohtml":    "Tab 2 Content",
			"default.gohtml": "Default Tab Content",
		},
	}

	service := NewService(&Config{
		Connector: connector.NewPartial(nil),
		UseCache:  false,
	})
	layout := service.NewLayout().FS(fsys)

	content := New("content.gohtml").
		ID("content").
		WithSelectMap("default", map[string]*Partial{
			"tab1":    New("tab1.gohtml").ID("tab1"),
			"tab2":    New("tab2.gohtml").ID("tab2"),
			"default": New("default.gohtml").ID("default"),
		})

	index := New("index.gohtml")

	layout.Set(content).Wrap(index)

	req := httptest.NewRequest("GET", "/", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Call the function you want to benchmark
		_, err := layout.RenderWithRequest(context.Background(), req)
		if err != nil {
			b.Fatalf("Error rendering: %v", err)
		}
	}
}

func BenchmarkRenderWithRequest(b *testing.B) {
	// Setup configuration and service
	cfg := &Config{
		Connector: connector.NewPartial(nil),
		UseCache:  false,
	}

	service := NewService(cfg)

	fsys := &InMemoryFS{
		Files: map[string]string{
			"templates/index.html":   `<html><head><title>{{ .Service.Title }}</title></head><body>{{ child "content" }}</body></html>`,
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

	index := NewID("index", "templates/index.html")

	// Set the content partial in the layout
	layout.Set(content).Wrap(index)

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

func TestRenderTable(t *testing.T) {
	svc := NewService(&Config{})

	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		// Define in-memory templates for the table and the row
		fsys := &InMemoryFS{
			Files: map[string]string{
				"templates/table.html": `<table>{{ range $i := .Data.Rows }}{{ child "row" "RowNumber" $i }}{{ end }}</table>`,
				"templates/row.html":   `<tr><td>Row {{ .Data.RowNumber }}</td></tr>`,
			},
		}

		// Create the row partial
		rowPartial := New("templates/row.html").ID("row")

		// Create the table partial and set data
		tablePartial := New("templates/table.html").ID("table")
		tablePartial.SetData(map[string]any{
			"Rows": []int{1, 2, 3, 4, 5}, // Generate 5 rows
		})
		tablePartial.With(rowPartial)

		// Render the table partial
		out, err := svc.NewLayout().FS(fsys).Set(tablePartial).RenderWithRequest(r.Context(), r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(out))
	}

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handleRequest(response, request)

	expected := `<table><tr><td>Row 1</td></tr><tr><td>Row 2</td></tr><tr><td>Row 3</td></tr><tr><td>Row 4</td></tr><tr><td>Row 5</td></tr></table>`

	if strings.TrimSpace(response.Body.String()) != expected {
		t.Errorf("expected %s, got %s", expected, response.Body.String())
	}
}

func TestMergeFuncMap(t *testing.T) {
	svc := NewService(&Config{
		FuncMap: template.FuncMap{
			"existingFunc": func() string { return "existing" },
		},
	})

	svc.MergeFuncMap(template.FuncMap{
		"newFunc": func() string { return "new" },
		"child":   func() string { return "should not overwrite" },
	})

	if _, ok := svc.config.FuncMap["newFunc"]; !ok {
		t.Error("newFunc should be added to FuncMap")
	}

	if svc.config.FuncMap["newFunc"].(func() string)() != "new" {
		t.Error("newFunc should return 'new'")
	}

	if _, ok := svc.config.FuncMap["child"]; ok {
		t.Error("child should not be overwritten in FuncMap")
	}
}
