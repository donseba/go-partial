package debug

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
)

func TestWithFuncGivesTemplatesTheBoundHelper(t *testing.T) {
	for _, cached := range []bool{false, true} {
		fsys := fstest.MapFS{
			"debug.gohtml": &fstest.MapFile{Data: []byte(`{{ debug . }}|{{ debug runtime "explicit" }}`)},
		}

		p := partial.NewID("debug", "debug.gohtml").
			SetFileSystem(fsys).
			UseTemplateCache(cached).
			SetDot("bound").
			Use(Stage(WithFunc()))

		out, err := partial.Render(context.Background(), p)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cached, err)
		}

		body := string(out)
		if strings.Count(body, `class="go-partial-debug"`) != 2 || !strings.Contains(body, "&#34;bound&#34;") ||
			!strings.Contains(body, "&#34;explicit&#34;") {
			t.Fatalf("cache %v: output = %q", cached, body)
		}
	}
}

func TestDisabledRendersNothing(t *testing.T) {
	fsys := fstest.MapFS{
		"debug.gohtml": &fstest.MapFile{Data: []byte(`[{{ debug . }}{{ debug runtime . }}]`)},
	}

	p := partial.NewID("debug", "debug.gohtml").
		SetFileSystem(fsys).
		UseTemplateCache(true).
		SetDot("secret").
		SetFunc(Disabled())

	out, err := partial.Render(context.Background(), p)
	if err != nil || string(out) != "[]" {
		t.Fatalf("Render() = %q, %v", out, err)
	}
}

func TestUnlessDefinedLeavesTheApplicationsDebug(t *testing.T) {
	fsys := fstest.MapFS{
		"debug.gohtml": &fstest.MapFile{Data: []byte(`[{{ debug . }}]`)},
	}

	for _, cached := range []bool{false, true} {
		var warnings int
		sink := partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
			if event.Kind == partial.EventFuncProtected {
				warnings++
			}
		})

		// Disabled is no function of the application's: the stage wins.
		dev := partial.NewID("debug", "debug.gohtml").SetFileSystem(fsys).UseTemplateCache(cached).SetDot("v").
			SetEvents(sink).
			SetFunc(Disabled()).
			Use(Stage(WithFunc(UnlessDefined())))

		out, err := partial.Render(context.Background(), dev)
		if err != nil || !strings.Contains(string(out), `class="go-partial-debug"`) {
			t.Fatalf("cache %v: with Disabled = %q, %v", cached, out, err)
		}

		// The application's own debug is called.
		own := partial.NewID("debug", "debug.gohtml").SetFileSystem(fsys).UseTemplateCache(cached).SetDot("v").
			SetEvents(sink).
			SetFunc(map[string]any{"debug": strings.ToUpper}).
			Use(Stage(WithFunc(UnlessDefined())))

		out, err = partial.Render(context.Background(), own)
		if err != nil || string(out) != "[V]" {
			t.Fatalf("cache %v: with its own = %q, %v", cached, out, err)
		}

		if warnings != 0 {
			t.Fatalf("cache %v: %d warnings", cached, warnings)
		}
	}

	if partial.IsReservedFuncName("debug", Stage(WithFunc(UnlessDefined()))) {
		t.Fatal("debug is reserved with UnlessDefined")
	}
}

func TestDebugInTextIsPlainText(t *testing.T) {
	fsys := fstest.MapFS{
		"mail.gohtml": &fstest.MapFile{Data: []byte(`Subject {{ debug . }}`)},
	}

	p := partial.NewID("mail", "mail.gohtml").SetFileSystem(fsys).SetDot(map[string]int{"a": 1}).Use(Stage(WithFunc()))

	out, err := partial.RenderText(context.Background(), p)
	if err != nil || out != "Subject {\n  \"a\": 1\n}" {
		t.Fatalf("RenderText() = %q, %v", out, err)
	}
}
