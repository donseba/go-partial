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
	"github.com/donseba/go-partial/exp/templatehelpers"
)

type contractPage struct {
	Title string
}

type testNamedContract struct {
	name string
	Kind string
	URL  string
}

func (c testNamedContract) ContractName() string {
	return c.name
}

func testContract(name string, kind string, url string) testNamedContract {
	return testNamedContract{
		name: name,
		Kind: kind,
		URL:  url,
	}
}

func TestNewRoot(t *testing.T) {
	root := New("template.gohtml")

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

	if len(root.children) != 0 {
		t.Errorf("NewRoot should have 0 children, got %d", len(root.children))
	}

	if len(root.oobChildren) != 0 {
		t.Errorf("NewRoot should have 0 oobChildren, got %d", len(root.oobChildren))
	}

}

func TestWithTemplateRegistersChildByFileName(t *testing.T) {
	root := NewID("root", "templates/page.gohtml").
		WithTemplate("templates/sidebar.gohtml")

	child, ok := root.children["sidebar"]
	if !ok {
		t.Fatal("WithTemplate should register a child using the template file name")
	}
	if child.id != "sidebar" {
		t.Fatalf("child id = %q, want %q", child.id, "sidebar")
	}
	if len(child.templates) != 1 || child.templates[0] != "templates/sidebar.gohtml" {
		t.Fatalf("child templates = %#v, want sidebar path", child.templates)
	}
	if child.parent != root {
		t.Fatal("WithTemplate child should be attached to the parent partial")
	}
}

func TestSetModelRegistersGoDocModelContracts(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.gohtml": `{{/* @model Page github.com/donseba/go-partial.contractPage */}}<h1>{{ Page.Title }}</h1>`,
	}}

	content := NewID("content", "templates/page.gohtml").
		SetFileSystem(fsys).
		UseTemplateCache(false).
		SetModel(contractPage{Title: "Typed page"})
	out, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("render contract model: %v", err)
	}
	if string(out) != "<h1>Typed page</h1>" {
		t.Fatalf("expected typed model render, got %q", out)
	}
}

func TestSetModelRegistersGoDocModelContractsWithCache(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.gohtml": `{{/* @model Page github.com/donseba/go-partial.contractPage */}}<h1>{{ Page.Title }}</h1>`,
	}}

	content := NewID("content", "templates/page.gohtml").
		SetFileSystem(fsys).
		SetModel(contractPage{Title: "First"})

	out, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("render first contract model: %v", err)
	}
	if string(out) != "<h1>First</h1>" {
		t.Fatalf("expected first typed model render, got %q", out)
	}

	second := content.clone()
	second.contracts = nil
	second.SetModel(contractPage{Title: "Second"})
	out, err = Render(context.Background(), second)
	if err != nil {
		t.Fatalf("render second contract model: %v", err)
	}
	if string(out) != "<h1>Second</h1>" {
		t.Fatalf("expected second typed model render, got %q", out)
	}
}

func TestSetModelRejectsProtectedHelperCollision(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.gohtml": `{{/* @model content github.com/donseba/go-partial.contractPage */}}{{ content.Title }}`,
	}}

	content := NewID("content", "templates/page.gohtml").
		SetFileSystem(fsys).
		UseTemplateCache(false).
		SetModel(contractPage{Title: "Typed page"})
	_, err := Render(context.Background(), content)
	if err == nil || !strings.Contains(err.Error(), "conflicts with a go-partial template helper") {
		t.Fatalf("expected protected helper collision, got %v", err)
	}
}

func TestSetContractRegistersNamedGoDocSymbolContracts(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.gohtml": `{{/*
@widget LikesPoll github.com/donseba/go-partial.testNamedContract
@widget LikeButton github.com/donseba/go-partial.testNamedContract
*/}}<section>{{ LikesPoll.Kind }}:{{ LikesPoll.URL }} {{ LikeButton.Kind }}:{{ LikeButton.URL }}</section>`,
	}}

	content := NewID("content", "templates/page.gohtml").
		SetFileSystem(fsys).
		UseTemplateCache(false).
		SetContract("widget",
			testContract("LikesPoll", "poll", "/likes"),
			testContract("LikeButton", "refresh", "/likes/toggle"),
		)
	out, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("render symbol contracts: %v", err)
	}
	html := string(out)
	if !strings.Contains(html, `poll:/likes`) {
		t.Fatalf("expected LikesPoll contract output, got %s", html)
	}
	if !strings.Contains(html, `refresh:/likes/toggle`) {
		t.Fatalf("expected LikeButton contract output, got %s", html)
	}
}

func TestSetContractRegistersNamedGoDocSymbolContractsWithCache(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.gohtml": `{{/*
@widget LikesPoll github.com/donseba/go-partial.testNamedContract
*/}}<section>{{ LikesPoll.URL }}</section>`,
	}}

	content := NewID("content", "templates/page.gohtml").
		SetFileSystem(fsys).
		SetContract("widget", testContract("LikesPoll", "poll", "/likes/first"))

	first, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("render first symbol contract: %v", err)
	}
	if !strings.Contains(string(first), `/likes/first`) {
		t.Fatalf("expected first contract output, got %s", first)
	}

	second := content.clone()
	second.contracts = nil
	second.SetContract("widget", testContract("LikesPoll", "poll", "/likes/second"))
	out, err := Render(context.Background(), second)
	if err != nil {
		t.Fatalf("render second symbol contract: %v", err)
	}
	if !strings.Contains(string(out), `/likes/second`) {
		t.Fatalf("expected second contract output, got %s", out)
	}
}

func TestSetContractSupportsNamedContracts(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.gohtml": `{{/*
@widget Async github.com/donseba/go-partial.testNamedContract
*/}}<section>{{ Async.URL }}</section>`,
	}}

	content := NewID("content", "templates/page.gohtml").
		SetFileSystem(fsys).
		UseTemplateCache(false).
		SetContract("widget", testContract("Async", "async", "/interactions/async"))
	out, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("render named contract: %v", err)
	}
	if !strings.Contains(string(out), `/interactions/async`) {
		t.Fatalf("expected named contract output, got %s", out)
	}
}

func TestRequestBasic(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/index.html":   `<html><body>{{ content }}</body></html>`,
				"templates/content.html": "<div>{{.Text}}</div>",
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))

		// content
		content := New("templates/content.html").ID("content")
		content.SetDot(map[string]any{
			"Text": "Welcome to the home page",
		})
		p := New("templates/index.html").ID("root")

		out, err := RenderWithRequest(r.Context(), r, svc.Compose(content, p))
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
		request.Header.Set(connector.HeaderTarget.String(), "content")
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
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/index.html":   `<html><body>{{ content }}</body></html>`,
				"templates/content.html": "<div>{{.Text}}</div>",
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))

		index := New("templates/index.html").ID("root")

		// content
		content := New("templates/content.html").ID("content")
		content.SetDot(map[string]any{
			"Text": "Welcome to the home page",
		})

		out, err := RenderWithRequest(r.Context(), r, svc.Compose(content, index))
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
		request.Header.Set(connector.HeaderTarget.String(), "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestTemplateCacheUsesCurrentRenderFunctions(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/shell.html":   `<html><body>{{ content }}</body></html>`,
			"templates/content.html": `<div>{{ .Text }}</div>`,
		},
	}
	svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	render := func(text string) string {
		t.Helper()
		content := NewID("content", "templates/content.html").SetDot(map[string]any{
			"Text": text,
		})
		shell := NewID("shell", "templates/shell.html")
		out, err := RenderWithRequest(request.Context(), request, svc.Compose(content, shell))
		if err != nil {
			t.Fatalf("failed to render %q: %v", text, err)
		}
		return string(out)
	}

	if got := render("first"); got != "<html><body><div>first</div></body></html>" {
		t.Fatalf("unexpected first render: %s", got)
	}
	if got := render("second"); got != "<html><body><div>second</div></body></html>" {
		t.Fatalf("cached render used stale functions or data: %s", got)
	}
}

func TestTemplateCacheIsBlueprintScoped(t *testing.T) {
	firstFS := &inMemoryFS{
		Files: map[string]string{
			"templates/content.html": `<p>first {{ .Name }}</p>`,
		},
	}
	secondFS := &inMemoryFS{
		Files: map[string]string{
			"templates/content.html": `<p>second {{ .Name }}</p>`,
		},
	}
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	render := func(fsys *inMemoryFS, name string) string {
		t.Helper()
		svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
		content := NewID("content", "templates/content.html").SetDot(map[string]any{
			"Name": name,
		})
		out, err := RenderWithRequest(request.Context(), request, svc.Apply(content))
		if err != nil {
			t.Fatalf("failed to render %q: %v", name, err)
		}
		return string(out)
	}

	if got := render(firstFS, "Ada"); got != "<p>first Ada</p>" {
		t.Fatalf("unexpected first blueprint render: %s", got)
	}
	if got := render(secondFS, "Grace"); got != "<p>second Grace</p>" {
		t.Fatalf("blueprint cache reused template from another filesystem: %s", got)
	}
}

func TestTemplateCacheUsesCurrentCustomFunctions(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/content.html": `<p>{{ label }}</p>`,
		},
	}
	svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	render := func(label string) string {
		t.Helper()
		content := NewID("content", "templates/content.html")
		content.SetFunc(template.FuncMap{
			"label": func() string {
				return label
			},
		})
		out, err := RenderWithRequest(request.Context(), request, svc.Apply(content))
		if err != nil {
			t.Fatalf("failed to render %q: %v", label, err)
		}
		return string(out)
	}

	if got := render("first"); got != "<p>first</p>" {
		t.Fatalf("unexpected first render: %s", got)
	}
	if got := render("second"); got != "<p>second</p>" {
		t.Fatalf("cached render used stale custom function: %s", got)
	}
}

func TestTemplateCacheUsesCurrentCustomFunctionsByScope(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/content.html": `<p>{{ label }}</p>`,
		},
	}
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	tests := []struct {
		name   string
		render func(t *testing.T, label string) string
	}{
		{
			name: "blueprint",
			render: func() func(t *testing.T, label string) string {
				svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
				return func(t *testing.T, label string) string {
					t.Helper()
					svc.SetFunc(template.FuncMap{
						"label": func() string {
							return label
						},
					})
					content := NewID("content", "templates/content.html")
					out, renderErr := RenderWithRequest(request.Context(), request, svc.Apply(content))
					if renderErr != nil {
						t.Fatalf("failed to render blueprint label %q: %v", label, renderErr)
					}
					return string(out)
				}
			}(),
		},
		{
			name: "shell",
			render: func() func(t *testing.T, label string) string {
				svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
				return func(t *testing.T, label string) string {
					t.Helper()
					content := NewID("content", "templates/content.html")
					svc.SetFunc(template.FuncMap{
						"label": func() string {
							return label
						},
					})
					out, renderErr := RenderWithRequest(svc.RenderContext(request.Context()), request, svc.Apply(content))
					if renderErr != nil {
						t.Fatalf("failed to render configured partial label %q: %v", label, renderErr)
					}
					return string(out)
				}
			}(),
		},
		{
			name: "partial",
			render: func() func(t *testing.T, label string) string {
				svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
				return func(t *testing.T, label string) string {
					t.Helper()
					content := NewID("content", "templates/content.html")
					content.SetFunc(template.FuncMap{
						"label": func() string {
							return label
						},
					})
					out, renderErr := RenderWithRequest(request.Context(), request, svc.Apply(content))
					if renderErr != nil {
						t.Fatalf("failed to render partial label %q: %v", label, renderErr)
					}
					return string(out)
				}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.render(t, "first"); got != "<p>first</p>" {
				t.Fatalf("unexpected first render: %s", got)
			}
			if got := tt.render(t, "second"); got != "<p>second</p>" {
				t.Fatalf("cached render used stale %s custom function: %s", tt.name, got)
			}
		})
	}
}

func TestFilteredTemplateFuncsRenderRequestHelpers(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/shell.html":   `<main>{{ content }}{{ template "notice.html" . }}</main>`,
			"templates/content.html": `<section>{{ partial runtime "templates/row.html" "Name" "Ada" }}</section>`,
			"templates/row.html":     `<p>{{ .Name }}</p>`,
			"templates/notice.html":  `<aside id="notice"{{ oobAttr }}>{{ .Message }}</aside>`,
		},
	}
	blueprint := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
	content := NewID("content", "templates/content.html")
	notice := NewID("notice", "templates/notice.html").
		SetDot(map[string]any{"Message": "Saved"}).
		SetAlwaysSwapOOB(true)
	shell := NewID("shell", "templates/shell.html").
		SetDot(map[string]any{"Message": "Saved"}).
		WithOOB(notice)
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	out, err := RenderWithRequest(request.Context(), request, blueprint.Compose(content, shell))
	if err != nil {
		t.Fatal(err)
	}

	html := string(out)
	if !strings.Contains(html, `<p>Ada</p>`) {
		t.Fatalf("filtered render did not use path partial helper: %s", html)
	}
	if !strings.Contains(html, `<aside id="notice">Saved</aside>`) {
		t.Fatalf("filtered render did not render second child: %s", html)
	}
}

func TestPartialHelperRendersTemplatePath(t *testing.T) {
	type cardData struct {
		Name string
	}

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/page.gohtml":  `<section>{{ partial runtime "templates/card.gohtml" .Card }}{{ partial runtime "/templates/badge.gohtml" "Label" "Ready" }}</section>`,
			"templates/card.gohtml":  `<article>{{ .Name }}</article>`,
			"templates/badge.gohtml": `<span>{{ .Label }}</span>`,
		},
	}

	content := NewID("page", "templates/page.gohtml").
		SetFileSystem(fsys).
		SetDot(map[string]any{"Card": cardData{Name: "Ada"}})
	out, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	expected := `<section><article>Ada</article><span>Ready</span></section>`
	if string(out) != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestTemplateCacheInheritsParentCustomFunctions(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/shell.html":   `<main>{{ content }}</main>`,
			"templates/content.html": `<p>{{ label }}</p>`,
		},
	}
	svc := newTestBlueprint(testBlueprintFS(fsys), testBlueprintCache(true))
	request, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	render := func(label string) string {
		t.Helper()
		content := NewID("content", "templates/content.html")
		wrapper := NewID("shell", "templates/shell.html")
		wrapper.SetFunc(template.FuncMap{
			"label": func() string {
				return label
			},
		})
		out, renderErr := RenderWithRequest(request.Context(), request, svc.Compose(content, wrapper))
		if renderErr != nil {
			t.Fatalf("failed to render inherited label %q: %v", label, renderErr)
		}
		return string(out)
	}

	if got := render("parent"); got != "<main><p>parent</p></main>" {
		t.Fatalf("unexpected inherited custom function render: %s", got)
	}
	if got := render("fresh"); got != "<main><p>fresh</p></main>" {
		t.Fatalf("cached child render used stale inherited custom function: %s", got)
	}
}

func TestProtectedFunctionsDoNotEnterCustomFuncMap(t *testing.T) {
	svc := newTestBlueprint()
	svc.SetFunc(template.FuncMap{
		"partial": func() string {
			return "blocked"
		},
		"label": func() string {
			return "allowed"
		},
	})

	customFuncs := svc.getCustomFuncMap()
	if _, ok := customFuncs["partial"]; ok {
		t.Fatal("protected partial helper should not be stored as a custom function")
	}
	if _, ok := customFuncs["label"]; !ok {
		t.Fatal("allowed label helper should be stored as a custom function")
	}
}

func TestRequestOOB(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/index.html":   `<html><body>{{ content }}{{ template "footer.html" . }}</body></html>`,
				"templates/content.html": "<div>{{.Text}}</div>",
				"templates/footer.html":  "<div{{ oobAttr }} id='footer'>{{.Text}}</div>",
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))

		p := New("templates/index.html").ID("root")
		p.SetDot(map[string]any{"Text": "This is the footer"})

		// content
		content := New("templates/content.html").ID("content")
		content.SetDot(map[string]any{
			"Text": "Welcome to the home page",
		})
		// oob
		oob := New("templates/footer.html").ID("footer")
		oob.SetDot(map[string]any{
			"Text": "This is the footer",
		})
		p.WithOOB(oob)

		out, err := RenderWithRequest(r.Context(), r, svc.Compose(content, p))
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
		request.Header.Set(connector.HeaderTarget.String(), "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div hx-swap-oob=\"true\" id='footer'>This is the footer</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestRequestOOBSwap(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/index.html":   `<html><body>{{ content }}{{ template "footer.html" . }}</body></html>`,
				"templates/content.html": "<div>{{.Text}}</div>",
				"templates/footer.html":  "<div{{ oobAttr }} id='footer'>{{.Text}}</div>",
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))

		// the main template that will be rendered
		p := New("templates/index.html").ID("root")
		p.SetDot(map[string]any{"Text": "This is the footer"})

		// oob footer that resides on the page
		oob := New("templates/footer.html").ID("footer")
		oob.SetDot(map[string]any{
			"Text": "This is the footer",
		})
		p.WithOOB(oob)

		// the actual content required for the page
		content := New("templates/content.html").ID("content")
		content.SetDot(map[string]any{
			"Text": "Welcome to the home page",
		})

		out, err := RenderWithRequest(r.Context(), r, svc.Compose(content, p))
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
		request.Header.Set(connector.HeaderTarget.String(), "content")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>Welcome to the home page</div><div hx-swap-oob=\"true\" id='footer'>This is the footer</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestDeepNested(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/index.html":   `<html><body>{{ content }}</body></html>`,
				"templates/content.html": "<div>{{.Text}}</div>",
				"templates/nested.html":  `<div>{{ upper .Text }}</div>`,
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))
		svc.SetFunc(templatehelpers.FuncMap())

		p := New("templates/index.html").ID("root")

		// nested content
		nested := New("templates/nested.html").ID("nested")
		nested.SetDot(map[string]any{
			"Text": "This is the nested content",
		})

		// content
		content := New("templates/content.html").ID("content")
		content.SetDot(map[string]any{
			"Text": "Welcome to the home page",
		}).With(nested)

		out, err := RenderWithRequest(r.Context(), r, svc.Compose(content, p))
		if err != nil {
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		_, _ = w.Write([]byte(out))
	}

	t.Run("find nested item and return it", func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(connector.HeaderTarget.String(), "nested")
		response := httptest.NewRecorder()

		handleRequest(response, request)

		expected := "<div>THIS IS THE NESTED CONTENT</div>"
		if response.Body.String() != expected {
			t.Errorf("expected %s, got %s", expected, response.Body.String())
		}
	})
}

func TestMissingPartial(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/index.html": `<html><body></body></html>`,
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))

		p := New("templates/index.html").ID("root")

		out, err := RenderWithRequest(r.Context(), r, svc.Apply(p))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(out))
	}

	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(connector.HeaderTarget.String(), "nonexistent")
	response := httptest.NewRecorder()

	handleRequest(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 500, got %d", response.Code)
	}
}

func TestTypedDotsInTemplates(t *testing.T) {
	type shell struct {
		Title string
	}
	type contentData struct {
		PageTitle string
		User      string
		Articles  []string
	}

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/index.html":   `<html><head><title>{{ .Title }}</title></head><body>{{ content }}</body></html>`,
			"templates/content.html": `<div>{{ .PageTitle }}</div><div>{{ .User }}</div><div>{{ .Articles }}</div>`,
		},
	}
	svc := newTestBlueprint(testBlueprintFS(fsys))

	content := New("templates/content.html").ID("content").SetDot(contentData{
		PageTitle: "Home Page",
		User:      "John Doe",
		Articles:  []string{"Article 1", "Article 2", "Article 3"},
	})
	wrapper := New("templates/index.html").ID("root").SetDot(shell{Title: "My Page"})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	out, err := RenderWithRequest(request.Context(), request, svc.Compose(content, wrapper))
	if err != nil {
		t.Fatal(err)
	}

	expected := "<html><head><title>My Page</title></head><body><div>Home Page</div><div>John Doe</div><div>[Article 1 Article 2 Article 3]</div></body></html>"
	if string(out) != expected {
		t.Errorf("expected %s, got %s", expected, out)
	}
}

func TestWithSelectMap(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"index.gohtml":   `<html><body>{{ content }}</body></html>`,
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
		ID("content")
	testWithSelectMap(contentPartial, "default", partialsMap)

	// Create the shell partial
	index := New("index.gohtml")

	// Set up the root blueprint and shell
	svc := newTestBlueprint(testBlueprintFS(fsys))
	svc.SetFunc(testSelectionFuncMap())
	svc.Use(testSelectionStage())
	root := svc.Compose(contentPartial, index)

	// Set up a test server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		err := Write(svc.RenderContext(ctx), w, r, root)
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
				req.Header.Set(connector.HeaderSelect.String(), tc.selectHeader)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Fatalf("Failed to close response body: %v", err)
				}
			}()

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

func TestSelectionPartialInheritsParentConnectorContext(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("content.gohtml", `{{ selection }}`)
	fsys.AddFile("settings.gohtml", `<div>{{ selectionValue }}</div>`)

	content := NewID("content", "content.gohtml").SetFileSystem(fsys)
	testWithSelectMap(content, "settings", map[string]*Partial{
		"settings": NewID("settings", "settings.gohtml").SetFileSystem(fsys),
	})
	content.SetFunc(testSelectionFuncMap()).Use(testSelectionStage())

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	req.Header.Set(connector.HeaderSelect.String(), "settings")

	out, err := RenderWithRequest(context.Background(), req, content)
	if err != nil {
		t.Fatalf("render selection: %v", err)
	}

	if string(out) != `<div>settings</div>` {
		t.Fatalf("expected selected partial to read parent connector selection, got %q", out)
	}
}

func TestSelectionIfUsesDefaultSelection(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("content.gohtml", `<button class="{{ if selectionIs "summary" }}active{{ end }}">Summary</button>`)

	content := NewID("content", "content.gohtml").SetFileSystem(fsys)
	testWithSelectMap(content, "summary", map[string]*Partial{
		"summary": NewID("summary", "summary.gohtml").SetFileSystem(fsys),
	})
	content.SetFunc(testSelectionFuncMap()).Use(testSelectionStage())

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	out, err := RenderWithRequest(context.Background(), req, content)
	if err != nil {
		t.Fatalf("render content: %v", err)
	}

	if string(out) != `<button class="active">Summary</button>` {
		t.Fatalf("expected selectionIs to use default selection, got %q", out)
	}
}

func TestSelectionPartialUsesErrorFragmentOnRenderError(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("content.gohtml", `{{ selection }}`)
	fsys.AddFile("broken.gohtml", `<div>{{ if .Missing }}broken</div>`)

	content := NewID("content", "content.gohtml").SetFileSystem(fsys)
	testWithSelectMap(content, "broken", map[string]*Partial{
		"broken": NewID("broken", "broken.gohtml").SetFileSystem(fsys).Use(testErrorStage(false)),
	})
	content.SetFunc(testSelectionFuncMap()).Use(testSelectionStage())

	req := httptest.NewRequest(http.MethodGet, "/tabs", nil)
	req.Header.Set(connector.HeaderSelect.String(), "broken")

	out, err := RenderWithRequest(context.Background(), req, content)
	if err != nil {
		t.Fatalf("render selection: %v", err)
	}

	if !strings.Contains(string(out), `class="go-partial-error"`) {
		t.Fatalf("expected selected partial fallback, got %q", out)
	}
	if !strings.Contains(string(out), `broken`) {
		t.Fatalf("expected fallback to name the broken selected partial, got %q", out)
	}
}

type testLocalizer struct {
	locale string
}

func (l testLocalizer) GetLocale() string {
	return l.locale
}

type testLocaleContextKey struct{}

type testSelectionConfig struct {
	Default  string
	Partials map[string]*Partial
}

type testSelectionKey struct{}

type testTargetResolverKey struct{}

func testWithSelectMap(p *Partial, defaultKey string, partials map[string]*Partial) {
	p.SetExtension(testSelectionKey{}, testSelectionConfig{Default: defaultKey, Partials: partials})
}

func testSelectionFuncMap() template.FuncMap {
	return template.FuncMap{
		"selection":       func() template.HTML { return "" },
		"selectionHeader": func() string { return "" },
		"selectionValue":  func() string { return "" },
		"selectionIs":     func(...string) bool { return false },
	}
}

func testSelectionStage() RenderStage {
	return RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			ctx.SetFunc("selectionHeader", func() string {
				return ctx.Runtime.Connector().GetSelectHeader()
			})
			selectionValue := func() string {
				selected := ctx.Runtime.Connector().GetSelectValue(ctx.Request)
				if selected != "" {
					return selected
				}
				value, _ := ctx.Partial.Extension(testSelectionKey{})
				cfg, _ := value.(testSelectionConfig)
				return cfg.Default
			}
			ctx.SetFunc("selectionValue", selectionValue)
			ctx.SetFunc("selectionIs", func(in ...string) bool {
				selected := selectionValue()
				for _, value := range in {
					if value == selected {
						return true
					}
				}
				return false
			})
			ctx.SetFunc("selection", func() template.HTML {
				value, _ := ctx.Partial.Extension(testSelectionKey{})
				cfg, _ := value.(testSelectionConfig)
				key := selectionValue()
				selected := cfg.Partials[key]
				if selected == nil {
					return template.HTML("selected partial '" + key + "' not found in parent '" + ctx.Partial.PartialID() + "'")
				}
				out, err := ctx.Runtime.RenderPartial(selected)
				if err != nil {
					fallback, fallbackErr := renderErrorFragment(ctx.Context, ctx.Request, selected, err)
					if fallbackErr != nil {
						return template.HTML("error rendering selected partial '" + key + "': " + fallbackErr.Error())
					}
					return fallback
				}
				return out
			})
			return ctx, nil
		},
	}
}

func testLocalizationFuncMap() template.FuncMap {
	return template.FuncMap{
		"locale":    func() string { return "" },
		"localizer": func() testLocalizer { return testLocalizer{} },
	}
}

func testLocalizationStage() RenderStage {
	return RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			localizer := func() testLocalizer {
				if loc, ok := ctx.Context.Value(testLocaleContextKey{}).(testLocalizer); ok {
					return loc
				}
				return testLocalizer{locale: "en_US"}
			}
			ctx.SetFunc("localizer", localizer)
			ctx.SetFunc("locale", func() string {
				return localizer().GetLocale()
			})
			return ctx, nil
		},
	}
}

func testTargetFuncMap() template.FuncMap {
	return template.FuncMap{
		"targetHeader": func() string { return "" },
		"targetValue":  func() string { return "" },
		"targetIs":     func(...string) bool { return false },
	}
}

func testUseTargetResolver(p *Partial, resolver func(context.Context, *http.Request, string) (*Partial, bool)) {
	p.SetExtension(testTargetResolverKey{}, resolver)
}

func testTargetStage() RenderStage {
	return RenderStageHooks{
		PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			ctx.SetFunc("targetHeader", func() string {
				return ctx.Runtime.Connector().GetTargetHeader()
			})
			ctx.SetFunc("targetValue", func() string {
				return ctx.Runtime.Connector().GetTargetValue(ctx.Request)
			})
			ctx.SetFunc("targetIs", func(in ...string) bool {
				target := ctx.Runtime.Connector().GetTargetValue(ctx.Request)
				for _, value := range in {
					if value == target {
						return true
					}
				}
				return false
			})
			if ctx.Kind != RenderKindTarget {
				return ctx, nil
			}
			value, _ := ctx.Partial.Extension(testTargetResolverKey{})
			resolver, _ := value.(func(context.Context, *http.Request, string) (*Partial, bool))
			if resolver == nil {
				return ctx, nil
			}
			resolved, ok := resolver(ctx.Context, ctx.Request, ctx.Name)
			if ok && resolved != nil {
				ctx.Partial = resolved
			}
			return ctx, nil
		},
	}
}

func TestBlueprintFuncMapCanAddTranslationFunctions(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("content.gohtml", `{{ tl localizer "hello" }}`)

	svc := newTestBlueprint(testBlueprintFS(fsys))
	svc.SetFunc(testLocalizationFuncMap())
	svc.Use(testLocalizationStage())
	svc.SetFunc(template.FuncMap{
		"tl": func(loc testLocalizer, key string) string {
			return loc.GetLocale() + ":" + key
		},
		"content": func() string {
			return "should not overwrite protected helper"
		},
		"partial": func() string {
			return "should not overwrite protected helper"
		},
	})

	content := NewID("content", "content.gohtml")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(context.Background(), testLocaleContextKey{}, testLocalizer{locale: "nl_NL"})

	out, err := RenderWithRequest(ctx, req, svc.Apply(content))
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	if string(out) != "nl_NL:hello" {
		t.Fatalf("expected translation function output, got %q", out)
	}

	if _, ok := svc.getStaticFuncMap()["partial"]; ok {
		t.Fatal("translation functions should not overwrite protected partial helper")
	}
	if _, ok := svc.getStaticFuncMap()["content"]; ok {
		t.Fatal("translation functions should not overwrite protected content helper")
	}
}

func BenchmarkWithSelectMap(b *testing.B) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"index.gohtml":   `<html><body>{{ content }}</body></html>`,
			"content.gohtml": `<div class="content">{{selection}}</div>`,
			"tab1.gohtml":    "Tab 1 Content",
			"tab2.gohtml":    "Tab 2 Content",
			"default.gohtml": "Default Tab Content",
		},
	}

	blueprint := newTestBlueprint(testBlueprintConnector(connector.NewPartial(nil)), testBlueprintCache(false), testBlueprintFS(fsys))
	content := New("content.gohtml").
		ID("content")
	testWithSelectMap(content, "default", map[string]*Partial{
		"tab1":    New("tab1.gohtml").ID("tab1"),
		"tab2":    New("tab2.gohtml").ID("tab2"),
		"default": New("default.gohtml").ID("default"),
	})
	content.SetFunc(testSelectionFuncMap()).Use(testSelectionStage())

	index := New("index.gohtml")

	root := blueprint.Compose(content, index)

	req := httptest.NewRequest("GET", "/", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Call the function you want to benchmark
		_, err := RenderWithRequest(context.Background(), req, root)
		if err != nil {
			b.Fatalf("Error rendering: %v", err)
		}
	}
}

func BenchmarkRenderWithRequest(b *testing.B) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/index.html":   `<html><head><title>{{ .Title }}</title></head><body>{{ content }}</body></html>`,
			"templates/content.html": `<div>{{ .PageTitle }}</div><div>{{ .User }}</div><div>{{ .Articles }}</div>`,
		},
	}

	// Setup reusable root blueprint.
	blueprint := New().SetConnector(connector.NewPartial(nil)).UseTemplateCache(false).SetFileSystem(fsys)

	// Create content partial
	content := NewID("content", "templates/content.html")
	content.SetDot(map[string]any{
		"PageTitle": "Benchmark Test",
		"User":      "Ada",
		"Articles":  "This is a benchmark test.",
	})

	index := NewID("index", "templates/index.html").SetDot(map[string]any{"Title": "Benchmark Test"})

	root := blueprint.Clone()
	root.templates = index.templates
	root.id = index.id
	root.SetDot(map[string]any{"Title": "Benchmark Test"})
	root.SetContent(content)

	// Create a sample HTTP request
	req := httptest.NewRequest("GET", "/", nil)

	// Reset the timer after setup
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Call the function you want to benchmark
		_, err := RenderWithRequest(context.Background(), req, root)
		if err != nil {
			b.Fatalf("Error rendering: %v", err)
		}
	}
}

func TestRenderTable(t *testing.T) {
	var handleRequest = func(w http.ResponseWriter, r *http.Request) {
		// Define in-memory templates for the table and the row
		fsys := &inMemoryFS{
			Files: map[string]string{
				"templates/table.html": `<table>{{ range $i := .Rows }}{{ partial runtime "templates/row.html" (dict "RowNumber" $i) }}{{ end }}</table>`,
				"templates/row.html":   `<tr><td>Row {{ .RowNumber }}</td></tr>`,
			},
		}
		svc := newTestBlueprint(testBlueprintFS(fsys))
		svc.SetFunc(templatehelpers.FuncMap())

		// Create the table partial and set data
		tablePartial := New("templates/table.html").ID("table")
		tablePartial.SetDot(map[string]any{
			"Rows": []int{1, 2, 3, 4, 5}, // Generate 5 rows
		})
		// Render the table partial
		out, err := RenderWithRequest(r.Context(), r, svc.Apply(tablePartial))
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

func TestSetDotRendersNativeTemplateChildAndTarget(t *testing.T) {
	type row struct {
		ID   int
		Name string
	}
	type page struct {
		Rows []row
	}

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/table.html": `{{/* @dot github.com/example/app.Page */}}<table>{{ range .Rows }}{{ template "/templates/row.html" . }}{{ end }}</table>`,
			"templates/row.html":   `{{/* @dot github.com/example/app.Row */}}<tr id="row-{{ .ID }}"><td>{{ .Name }}</td><td>{{ locale }}</td></tr>`,
		},
	}

	table := NewID("table", "templates/table.html").
		SetFileSystem(fsys).
		SetDot(page{Rows: []row{
			{ID: 1, Name: "Coffee"},
			{ID: 2, Name: "Tea"},
		}})
	table.SetFunc(testLocalizationFuncMap(), testTargetFuncMap())
	table.Use(testLocalizationStage(), testTargetStage())
	rowPartial := NewID("row", "templates/row.html").
		SetFileSystem(fsys)
	table.With(rowPartial)
	testUseTargetResolver(table, func(ctx context.Context, r *http.Request, target string) (*Partial, bool) {
		if target != "row-2" {
			return nil, false
		}
		return NewID(target, "templates/row.html").SetDot(row{ID: 2, Name: "Tea"}), true
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(context.Background(), testLocaleContextKey{}, testLocalizer{locale: "nl_NL"})

	full, err := RenderWithRequest(ctx, req, table)
	if err != nil {
		t.Fatalf("RenderWithRequest() full error = %v", err)
	}
	wantFull := `<table><tr id="row-1"><td>Coffee</td><td>nl_NL</td></tr><tr id="row-2"><td>Tea</td><td>nl_NL</td></tr></table>`
	if strings.TrimSpace(string(full)) != wantFull {
		t.Fatalf("full render = %s, want %s", full, wantFull)
	}

	req.Header.Set(connector.HeaderTarget.String(), "row-2")
	target, err := RenderWithRequest(ctx, req, table)
	if err != nil {
		t.Fatalf("RenderWithRequest() target error = %v", err)
	}
	wantTarget := `<tr id="row-2"><td>Tea</td><td>nl_NL</td></tr>`
	if strings.TrimSpace(string(target)) != wantTarget {
		t.Fatalf("target render = %s, want %s", target, wantTarget)
	}
}

func TestSetDotKeepsRequestHelpersAvailable(t *testing.T) {
	type page struct {
		Title string
	}

	fsys := &inMemoryFS{
		Files: map[string]string{
			"templates/page.html": `<h1>{{ .Title }}</h1><p>{{ locale }} {{ ctx.URL.Path }} {{ basePath }} {{ if request }}request{{ end }}</p>`,
		},
	}

	p := NewID("page", "templates/page.html").
		SetFileSystem(fsys).
		SetBasePath("/app").
		SetDot(page{Title: "Dashboard"}).
		SetFunc(testLocalizationFuncMap()).
		Use(testLocalizationStage())

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := context.WithValue(context.Background(), testLocaleContextKey{}, testLocalizer{locale: "en_US"})
	out, err := RenderWithRequest(ctx, req, p)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	want := `<h1>Dashboard</h1><p>en_US /dashboard /app request</p>`
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("render = %s, want %s", out, want)
	}
}

func TestSetDotReplacesExistingDotContract(t *testing.T) {
	type page struct {
		Title string
	}
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.html": `{{/* @dot github.com/example/app.Page */}}{{ .Title }}`,
	}}

	content := NewID("content", "templates/page.html").
		SetFileSystem(fsys).
		SetDot(page{Title: "First"}).
		SetDot(page{Title: "Second"})
	out, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.TrimSpace(string(out)) != "Second" {
		t.Fatalf("render = %s, want Second", out)
	}
}

func TestPartialSetFuncUsesContractStoreWithCache(t *testing.T) {
	fsys := &inMemoryFS{Files: map[string]string{
		"templates/page.html": `{{ label "Ada" }}`,
	}}

	content := NewID("content", "templates/page.html").
		SetFileSystem(fsys).
		UseTemplateCache(true).
		SetFunc(template.FuncMap{
			"label": func(name string) string { return "first:" + name },
		})

	first, err := Render(context.Background(), content)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	if strings.TrimSpace(string(first)) != "first:Ada" {
		t.Fatalf("first render = %s", first)
	}

	second := content.clone().
		SetFunc(template.FuncMap{
			"label": func(name string) string { return "second:" + name },
		})
	out, err := Render(context.Background(), second)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if strings.TrimSpace(string(out)) != "second:Ada" {
		t.Fatalf("second render = %s", out)
	}
}

func TestSetFunc(t *testing.T) {
	svc := newTestBlueprint()

	svc.SetFunc(template.FuncMap{
		"existingFunc": func() string { return "existing" },
		"newFunc":      func() string { return "new" },
		"dict":         func() string { return "should not overwrite" },
		"content":      func() string { return "should not overwrite" },
		"partial":      func() string { return "should not overwrite" },
	}, template.FuncMap{
		"secondMapFunc": func() string { return "second-map" },
	})

	funcs := svc.getStaticFuncMap()
	if _, ok := funcs["newFunc"]; !ok {
		t.Error("newFunc should be added to FuncMap")
	}

	if funcs["newFunc"].(func() string)() != "new" {
		t.Error("newFunc should return 'new'")
	}

	if funcs["existingFunc"].(func() string)() != "existing" {
		t.Error("existingFunc should return 'existing'")
	}

	if funcs["secondMapFunc"].(func() string)() != "second-map" {
		t.Error("secondMapFunc should return 'second-map'")
	}

	if funcs["dict"].(func() string)() != "should not overwrite" {
		t.Error("dict should be overridable because it is an optional helper, not a core go-partial helper")
	}

	if _, ok := funcs["content"]; ok {
		t.Error("content should not be overwritten in FuncMap")
	}

	if _, ok := funcs["partial"]; ok {
		t.Error("partial should not be overwritten in FuncMap")
	}

}
