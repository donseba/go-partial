package actions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

var errAction = errors.New("boom")

// renderAction renders page.gohtml, src, with the action stage and the
// template action given, for a request with the action header set to
// action.
func renderAction(t *testing.T, src string, templateAction Action, action string) (string, error) {
	t.Helper()

	fsys := fstest.MapFS{
		"page.gohtml":   &fstest.MapFile{Data: []byte(src)},
		"result.gohtml": &fstest.MapFile{Data: []byte(`result`)},
	}

	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Stage())

	if templateAction != nil {
		WithTemplateAction(p, templateAction)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if action != "" {
		req.Header.Set(connector.HeaderAction.String(), action)
	}

	out, err := partial.RenderWithRequest(context.Background(), req, p)

	return string(out), err
}

// TestTemplateActionReportsProblems checks what {{ action }} writes when the
// partial has no template action, when the action fails, and when it gives
// no partial to render.
func TestTemplateActionReportsProblems(t *testing.T) {
	tests := []struct {
		name   string
		action Action
		want   string
	}{
		{
			name: "no action",
			want: "no action callback found in partial 'page'",
		},
		{
			name: "failing action",
			action: func(context.Context, *partial.Partial, *partial.Runtime) (*partial.Partial, error) {
				return nil, errAction
			},
			want: "error in action function: boom",
		},
		{
			name: "no partial",
			action: func(context.Context, *partial.Partial, *partial.Runtime) (*partial.Partial, error) {
				return nil, nil
			},
			want: "error rendering action partial: partial is not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := renderAction(t, `{{ action }}`, tt.action, "")
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if out != tt.want {
				t.Fatalf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

func TestActionIsMatchesTheRequestsAction(t *testing.T) {
	src := `{{ if actionIs "save" "publish" }}matched{{ else }}other{{ end }}`

	for action, want := range map[string]string{"publish": "matched", "delete": "other", "": "other"} {
		out, err := renderAction(t, src, nil, action)
		if err != nil {
			t.Fatalf("Render() error = %v", err)
		}

		if out != want {
			t.Errorf("action %q: output = %q, want %q", action, out, want)
		}
	}
}

// TestFuncMapHelpersTakeTheRenderContext checks the helpers of FuncMap
// given the render context, without the stage, and without a context.
func TestFuncMapHelpersTakeTheRenderContext(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml":   &fstest.MapFile{Data: []byte(`{{ actionHeader ctx }}={{ actionValue ctx }}:{{ action ctx }}:{{ actionIs "save" }}`)},
		"result.gohtml": &fstest.MapFile{Data: []byte(`result`)},
	}

	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(FuncMap())
	WithTemplateAction(p, func(context.Context, *partial.Partial, *partial.Runtime) (*partial.Partial, error) {
		return partial.NewID("result", "result.gohtml").SetFileSystem(fsys), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(connector.HeaderAction.String(), "save")

	out, err := partial.RenderWithRequest(context.Background(), req, p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// The actionIs of FuncMap has no render context: it is false.
	if want := "X-Action=save:result:false"; string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}

	if ActionHTML() != "" || ActionHeader() != "" || ActionValue() != "" || ActionIs("save") {
		t.Fatal("helpers without a render context answered")
	}

	empty := &partial.RenderContext{}
	if ActionHeader(empty) != "" || ActionValue(empty) != "" {
		t.Fatal("helpers without a runtime answered")
	}
}

// TestWithActionErrorsAndKeepsThePartial checks that a failing partial action
// fails the render, and that one that returns no partial keeps the partial.
func TestWithActionErrorsAndKeepsThePartial(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`page`)},
	}

	failing := partial.NewID("page", "page.gohtml").SetFileSystem(fsys).Use(Stage())
	WithAction(failing, func(context.Context, *partial.Partial, *partial.Runtime) (*partial.Partial, error) {
		return nil, errAction
	})

	_, err := partial.Render(context.Background(), failing)
	if !errors.Is(err, errAction) || !strings.Contains(err.Error(), "error in action function") {
		t.Fatalf("Render() error = %v, want the action's error", err)
	}

	kept := partial.NewID("page", "page.gohtml").SetFileSystem(fsys).Use(Stage())
	WithAction(kept, func(context.Context, *partial.Partial, *partial.Runtime) (*partial.Partial, error) {
		return nil, nil
	})

	out, err := partial.Render(context.Background(), kept)
	if err != nil || out != "page" {
		t.Fatalf("Render() = %q, %v, want the partial itself", out, err)
	}

	if getConfig(nil).action != nil {
		t.Fatal("config of a nil partial has an action")
	}
}
