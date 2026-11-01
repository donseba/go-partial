package partial

import (
	"errors"
	"html/template"
	"strings"
	"testing"
)

// resolverStage is a render stage of its own type, not RenderStageHooks,
// that gives templates greet.
type resolverStage struct{}

func (resolverStage) Prepare(ctx *RenderContext) (*RenderContext, error) {
	return ctx, nil
}

func (resolverStage) Render(ctx *RenderContext, next RenderNext) (template.HTML, error) {
	out, err := next(ctx)

	return "[" + out + "]", err
}

func (resolverStage) Finalize(_ *RenderContext, out template.HTML, err error) (template.HTML, error) {
	return out, err
}

func (resolverStage) TemplateFuncNames() []string {
	return []string{"greet"}
}

func (resolverStage) ResolveTemplateFunc(_ *RenderContext, name string) (any, bool) {
	if name != "greet" {
		return nil, false
	}

	return func() string {
		return "hello from a stage type"
	}, true
}

// TestStagesOfEveryKind checks a render through a nil stage, a pointer to
// RenderStageHooks that only resolves functions, and a stage type of its
// own that resolves functions and wraps the output.
func TestStagesOfEveryKind(t *testing.T) {
	pointer := &RenderStageHooks{
		FuncNames: []string{"shout"},
		ResolveFunc: func(_ *RenderContext, name string) (any, bool) {
			if name != "shout" {
				return nil, false
			}

			return func(s string) string {
				return strings.ToUpper(s)
			}, true
		},
	}

	for _, cached := range []bool{false, true} {
		p := NewID("page", "page.gohtml").
			SetFileSystem(Sources{"page.gohtml": `{{ shout "hi" }} {{ greet }}`}).
			UseTemplateCache(cached).
			Use(nil, pointer, resolverStage{})

		out, err := Render(t.Context(), p)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cached, err)
		}

		if want := "[HI hello from a stage type]"; string(out) != want {
			t.Fatalf("cache %v: output = %q, want %q", cached, out, want)
		}
	}

	if !resolvesFuncs(resolverStage{}) || resolvesFuncs((*RenderStageHooks)(nil)) || resolvesFuncs(RenderStageHooks{}) {
		t.Fatal("resolvesFuncs() is wrong")
	}

	if rendersThrough(resolverStage{}) || rendersThrough((*RenderStageHooks)(nil)) || !rendersThrough(pointer) {
		t.Fatal("rendersThrough() is wrong")
	}
}

// TestPrepareFailures checks the errors of a stage whose Prepare fails or
// returns no render context, and that a Prepare that drops the response
// gets a new one.
func TestPrepareFailures(t *testing.T) {
	errPrepare := errors.New("prepare failed")

	tests := []struct {
		name    string
		prepare func(ctx *RenderContext) (*RenderContext, error)
		want    string
	}{
		{
			name: "error",
			prepare: func(ctx *RenderContext) (*RenderContext, error) {
				return ctx, errPrepare
			},
			want: "prepare failed",
		},
		{
			name: "error without a context",
			prepare: func(*RenderContext) (*RenderContext, error) {
				return nil, errPrepare
			},
			want: "prepare failed",
		},
		{
			name: "no context",
			prepare: func(*RenderContext) (*RenderContext, error) {
				return nil, nil
			},
			want: "render stage prepare returned nil context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewID("page", "page.gohtml").
				SetFileSystem(Sources{"page.gohtml": `page`}).
				Use(RenderStageHooks{PrepareFunc: tt.prepare})

			_, err := Render(t.Context(), p)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Render() error = %v, want %q", err, tt.want)
			}
		})
	}

	dropped := NewID("page", "page.gohtml").
		SetFileSystem(Sources{"page.gohtml": `page`}).
		Use(RenderStageHooks{PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
			ctx.Response = nil
			return ctx, nil
		}})

	out, err := Render(t.Context(), dropped)
	if err != nil || out != "page" {
		t.Fatalf("Render() with a dropped response = %q, %v", out, err)
	}

	if result := renderWithChainResult(nil, nil, renderTemplateTerminal); result.Err == nil {
		t.Fatal("renderWithChainResult(nil) error = nil")
	}

	if _, err := renderTemplateTerminal(&RenderContext{}); err == nil || !strings.Contains(err.Error(), "requires a partial") {
		t.Fatalf("renderTemplateTerminal() without a partial error = %v", err)
	}
}

func TestRenderValuesAndNilRenderContexts(t *testing.T) {
	var values RenderValues
	values.Set("key", "value")

	if values.Get("key") != nil {
		t.Fatal("a nil RenderValues kept a value")
	}

	if clone := values.Clone(); clone == nil || len(clone) != 0 {
		t.Fatalf("Clone() of nil values = %v, want an empty map", clone)
	}

	full := RenderValues{"a": 1, "b": 2}
	clone := full.Clone()
	clone.Set("a", 3)

	if full.Get("a") != 1 || clone.Get("b") != 2 {
		t.Fatalf("Clone() is not a copy: %v %v", full, clone)
	}

	var ctx *RenderContext
	ctx.OmitOOB()
	ctx.SetFunc("name", strings.ToUpper)
	ctx.SetFuncResolver(func(string) (any, bool) {
		return nil, false
	})

	if ctx.TargetResolved() || ctx.FailureResponse() || ctx.FailureStatus() != 0 {
		t.Fatal("a nil render context reports a resolved target or failure")
	}

	if fn, ok := ctx.ResolveFunc("name"); ok || fn != nil {
		t.Fatal("a nil render context resolves a function")
	}

	empty := &RenderContext{}
	empty.SetFunc("", strings.ToUpper)
	empty.SetFunc("upper", nil)
	empty.SetFuncResolver(nil)

	if len(empty.Funcs) != 0 || len(empty.funcResolvers) != 0 {
		t.Fatal("SetFunc() or SetFuncResolver() kept an empty name or nil function")
	}

	empty.SetFunc("upper", strings.ToUpper)

	if fn, ok := empty.ResolveFunc("upper"); !ok || fn == nil {
		t.Fatal("ResolveFunc() did not find a function set with SetFunc")
	}

	if fn, ok := empty.ResolveFunc(""); ok || fn != nil {
		t.Fatal("ResolveFunc(\"\") found a function")
	}

	if (&RenderContext{Kind: RenderKindTarget}).TargetResolved() {
		t.Fatal("a target render without a partial reports a resolved target")
	}
}
