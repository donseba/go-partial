package partial

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetBasePathSimple(t *testing.T) {
	p := New()
	p.SetBasePath("/foo")
	if got := p.GetBasePath(); got != "/foo" {
		t.Errorf("expected /foo, got %q", got)
	}
}

func TestGetBasePathParentFallback(t *testing.T) {
	parent := New()
	parent.SetBasePath("/parent")
	child := NewID("child")
	parent.With(child)
	if got := child.GetBasePath(); got != "/parent" {
		t.Errorf("expected /parent, got %q", got)
	}
}

func TestGetBasePathParentChain(t *testing.T) {
	grandparent := New()
	grandparent.SetBasePath("/grand")
	parent := NewID("parent")
	child := NewID("child")
	grandparent.With(parent)
	parent.With(child)
	if got := child.GetBasePath(); got != "/grand" {
		t.Errorf("expected /grand, got %q", got)
	}
}

func TestGetBasePathOverride(t *testing.T) {
	parent := New()
	parent.SetBasePath("/parent")
	child := NewID("child")
	parent.With(child)
	child.SetBasePath("/child")
	if got := child.GetBasePath(); got != "/child" {
		t.Errorf("expected /child, got %q", got)
	}
}

func TestGetBasePathEmpty(t *testing.T) {
	p := New()
	if got := p.GetBasePath(); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTemplateBasePathUsesChildOverride(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"child.gohtml": `{{ basePath }}`,
		},
	}
	parent := NewID("parent")
	child := NewID("child", "child.gohtml").SetFileSystem(fsys)
	parent.SetBasePath("/parent").With(child)
	child.SetBasePath("/child")

	out, err := child.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "/child" {
		t.Fatalf("basePath helper = %q, want %q", got, "/child")
	}
}

func TestChildFileSystemOverridesParentFileSystem(t *testing.T) {
	parentFS := &inMemoryFS{
		Files: map[string]string{
			"shared.gohtml": `parent`,
		},
	}
	childFS := &inMemoryFS{
		Files: map[string]string{
			"shared.gohtml": `child`,
		},
	}
	parent := NewID("parent").SetFileSystem(parentFS)
	child := NewID("child", "shared.gohtml").SetFileSystem(childFS)
	parent.With(child)

	out, err := child.Render(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "child" {
		t.Fatalf("child rendered %q, want child filesystem content", got)
	}
}

func TestChildResponseHeadersDoNotLeakToParent(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"parent.gohtml": `parent`,
			"child.gohtml":  `child`,
		},
	}
	parent := NewID("parent", "parent.gohtml").SetFileSystem(fsys)
	child := NewID("child", "child.gohtml").SetResponseHeaders(map[string]string{
		"X-Child": "true",
	})
	parent.With(child)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := parent.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("X-Child"); got != "" {
		t.Fatalf("parent response header X-Child = %q, want empty", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Target", "child")
	if err := parent.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("X-Child"); got != "true" {
		t.Fatalf("child response header X-Child = %q, want true", got)
	}
}

func TestLayoutWriteWithRequestWritesContentWithoutWrapper(t *testing.T) {
	fsys := &inMemoryFS{
		Files: map[string]string{
			"content.gohtml": `content`,
		},
	}
	layout := NewService(&Config{FS: fsys}).NewLayout().Set(NewID("content", "content.gohtml"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if err := layout.WriteWithRequest(context.Background(), rec, req); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "content" {
		t.Fatalf("body = %q, want content", got)
	}
}
