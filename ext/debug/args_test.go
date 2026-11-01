package debug

import (
	"context"
	"errors"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

// TestBoundHelperArguments checks what {{ debug }} shows for no argument,
// for several, and for two whose first is not a runtime.
func TestBoundHelperArguments(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
	}{
		{name: "no argument", template: `{{ debug }}`, want: "null"},
		{name: "several", template: `{{ debug 1 2 3 }}`, want: "[\n  1,\n  2,\n  3\n]"},
		{name: "two values", template: `{{ debug "a" "b" }}`, want: "[\n  &#34;a&#34;,\n  &#34;b&#34;\n]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := partial.NewID("debug", "debug.gohtml").
				SetFileSystem(fstest.MapFS{"debug.gohtml": &fstest.MapFile{Data: []byte(tt.template)}}).
				Use(Stage(nil, WithFunc(nil)))

			out, err := partial.Render(context.Background(), p)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			body := string(out)
			if !strings.Contains(body, `<pre class="go-partial-debug__value">`+tt.want+`</pre>`) {
				t.Fatalf("output = %q, want the value %q", body, tt.want)
			}

			if !strings.Contains(body, `<span class="go-partial-debug__partial">debug</span>`) {
				t.Fatalf("output = %q, want the partial ID", body)
			}
		})
	}
}

// TestDebugWithoutTheStage checks that without the debug stage, or without a
// runtime, the value is written escaped, and that a stage that fails the
// debug render leaves the escaped value too.
func TestDebugWithoutTheStage(t *testing.T) {
	if got := Debug(nil, "<b>"); got != `&#34;&lt;b&gt;&#34;` {
		t.Fatalf("Debug(nil) = %q", got)
	}

	failing := partial.RenderStageHooks{
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if ctx.Kind == RenderKindDebug {
				return "", errors.New("no debug here")
			}

			return next(ctx)
		},
	}

	for name, stages := range map[string][]partial.RenderStage{
		"no stage":      nil,
		"failing stage": {failing},
	} {
		p := partial.NewID("debug", "debug.gohtml").
			SetFileSystem(fstest.MapFS{"debug.gohtml": &fstest.MapFile{Data: []byte(`{{ debug runtime . }}`)}}).
			SetFunc(FuncMap()).
			SetDot(3)

		if len(stages) > 0 {
			p.Use(stages...)
		}

		out, err := partial.Render(context.Background(), p)
		if err != nil {
			t.Fatalf("%s: Render() error = %v", name, err)
		}

		if string(out) != "3" {
			t.Fatalf("%s: output = %q, want the escaped value", name, out)
		}
	}
}

func TestFormatValueFallsBackToGoSyntax(t *testing.T) {
	type point struct {
		X  int
		Fn func()
	}

	if got := FormatValue(point{X: 1}); got != "debug.point{X:1, Fn:(func())(nil)}" {
		t.Fatalf("FormatValue() = %q", got)
	}

	if data := BuildData(nil); data.Value != nil || data.Output != "null" || data.PartialID != "" {
		t.Fatalf("BuildData(nil) = %+v", data)
	}
}
