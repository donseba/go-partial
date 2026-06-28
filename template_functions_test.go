package partial

import (
	"context"
	"html/template"
	"strings"
	"testing"
)

func TestDebugRendersDefaultDebugBox(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("debug.gohtml", `{{ debug runtime . }}`)

	p := NewID("debug", "debug.gohtml").SetFileSystem(fsys).SetDot(map[string]any{
		"a": 1,
		"b": "test",
	})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `class="go-partial-debug"`) {
		t.Fatalf("expected styled debug box, got %q", body)
	}
	if !strings.Contains(body, `&#34;a&#34;: 1`) || !strings.Contains(body, `&#34;b&#34;: &#34;test&#34;`) {
		t.Fatalf("expected debug output to contain data, got %q", body)
	}
}

func TestDebugUsesCustomDebugRenderer(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("debug.gohtml", `{{ debug runtime .Name }}`)

	p := NewID("debug", "debug.gohtml").
		SetFileSystem(fsys).
		SetDot(map[string]any{"Name": "Ada"}).
		SetDebugRenderer(func(ctx context.Context, p *Partial, runtime *Runtime, value any) (template.HTML, error) {
			return template.HTML(`<aside class="custom-debug">` + value.(string) + `</aside>`), nil
		})

	out, err := p.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="custom-debug">Ada</aside>` {
		t.Fatalf("unexpected custom debug output: %q", out)
	}
}

func TestTemplatePathPartialDebugRendererSurvivesClone(t *testing.T) {
	fsys := &inMemoryFS{}
	fsys.AddFile("parent.gohtml", `{{ partial runtime "child.gohtml" }}`)
	fsys.AddFile("child.gohtml", `{{ debug runtime .Name }}`)

	parent := NewID("parent", "parent.gohtml").
		SetFileSystem(fsys).
		SetDot(map[string]any{"Name": "Ada"}).
		SetDebugRenderer(func(ctx context.Context, p *Partial, runtime *Runtime, value any) (template.HTML, error) {
			return template.HTML(`<aside class="child-debug">` + value.(string) + `</aside>`), nil
		})

	out, err := parent.Render(context.Background())
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if string(out) != `<aside class="child-debug">Ada</aside>` {
		t.Fatalf("expected child debug renderer to survive clone, got %q", out)
	}
}
