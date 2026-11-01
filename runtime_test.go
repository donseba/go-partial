package partial

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type runtimeKey struct{}

// renderRuntime renders page.gohtml, `{{ inspect runtime }}`, of fsys with
// inspect as the template function, for a GET of /items, and returns the
// output.
func renderRuntime(t *testing.T, fsys Sources, inspect func(rt *Runtime) template.HTML, setup ...func(p *Partial)) string {
	t.Helper()

	p := NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(template.FuncMap{"inspect": inspect})

	for _, configure := range setup {
		configure(p)
	}

	req := httptest.NewRequest(http.MethodGet, "/items?page=2", nil)
	ctx := context.WithValue(context.Background(), runtimeKey{}, "from the context")

	out, err := RenderWithRequest(ctx, req, p)
	if err != nil {
		t.Fatalf("RenderWithRequest() error = %v", err)
	}

	return string(out)
}

// TestRuntimeDescribesTheRender checks what a template helper learns of the
// render from its runtime, and that values it sets are kept.
func TestRuntimeDescribesTheRender(t *testing.T) {
	inspect := func(rt *Runtime) template.HTML {
		rt.SetValue(runtimeKey{}, "set by the helper")

		return template.HTML(fmt.Sprintf("%s|%s|%s|%v|%v|%s|%s",
			rt.BasePath(),
			rt.URL().Path,
			rt.Request().Method,
			rt.Context().Value(runtimeKey{}),
			rt.Value(runtimeKey{}),
			rt.RenderContext().Kind,
			rt.Connector().GetTargetHeader(),
		))
	}

	got := renderRuntime(t, Sources{"page.gohtml": `{{ inspect runtime }}`}, inspect, func(p *Partial) {
		p.SetBasePath("/app")
	})

	want := "/app|/items|GET|from the context|set by the helper|partial|X-Target"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestRuntimeWithoutARender checks the answers of a nil runtime, and of one
// without a render context, which helpers get outside a render.
func TestRuntimeWithoutARender(t *testing.T) {
	for name, rt := range map[string]*Runtime{
		"nil":        nil,
		"no context": {partial: New("page.gohtml")},
	} {
		if rt.Context() == nil {
			t.Errorf("%s: Context() = nil", name)
		}

		if rt.Request() != nil || rt.URL() != nil || rt.BasePath() != "" || rt.Value("key") != nil {
			t.Errorf("%s: the runtime describes a request", name)
		}

		rt.SetValue("key", "value")

		if rt.Value("key") != nil {
			t.Errorf("%s: SetValue() kept a value without a render", name)
		}

		if got := rt.Partial("card.gohtml"); !strings.Contains(string(got), "runtime partial render stage is not configured") {
			t.Errorf("%s: Partial() = %q", name, got)
		}

		if _, err := rt.RenderPartial(New("card.gohtml")); err == nil {
			t.Errorf("%s: RenderPartial() error = nil", name)
		}

		if _, err := rt.RenderPartialWithFallback(New("card.gohtml")); err == nil {
			t.Errorf("%s: RenderPartialWithFallback() error = nil", name)
		}

		if err := rt.CheckContracts(New("card.gohtml")); err == nil {
			t.Errorf("%s: CheckContracts() error = nil", name)
		}

		_, err := rt.RenderWith("custom", "", nil, func(*RenderContext) (template.HTML, error) {
			return "", nil
		})
		if err == nil || !strings.Contains(err.Error(), "runtime render stage is not configured") {
			t.Errorf("%s: RenderWith() error = %v", name, err)
		}
	}

	var none *Runtime
	if none.RenderContext() != nil {
		t.Fatal("nil runtime has a render context")
	}

	if got := none.Connector().GetTargetHeader(); got != "X-Target" {
		t.Fatalf("nil runtime connector target header = %q", got)
	}

	withoutValues := newRuntime(New("page.gohtml"), &RenderContext{})
	withoutValues.SetValue("key", "value")

	if withoutValues.Value("key") != "value" {
		t.Fatal("SetValue() did not make the values of a render context without them")
	}
}

// TestRuntimeRendersChildren checks RenderPartial, RenderPartialWithFallback
// and RenderWith from a template helper, for a child that renders, one that
// fails, and none.
func TestRuntimeRendersChildren(t *testing.T) {
	fsys := Sources{
		"page.gohtml":   `{{ inspect runtime }}`,
		"card.gohtml":   `<p>card</p>`,
		"broken.gohtml": `{{ template "missing" }}`,
	}

	inspect := func(rt *Runtime) template.HTML {
		var parts []string

		out, err := rt.RenderPartial(NewID("card", "card.gohtml"))
		parts = append(parts, fmt.Sprintf("%s/%v", out, err))

		_, err = rt.RenderPartial(nil)
		parts = append(parts, fmt.Sprint(err))

		out, err = rt.RenderPartialWithFallback(NewID("card", "card.gohtml"))
		parts = append(parts, fmt.Sprintf("%s/%v", out, err))

		_, err = rt.RenderPartialWithFallback(NewID("broken", "broken.gohtml"))
		parts = append(parts, fmt.Sprint(err != nil && strings.Contains(err.Error(), "error rendering error response fragment")))

		_, err = rt.RenderPartialWithFallback(nil)
		parts = append(parts, fmt.Sprint(err))

		_, err = rt.RenderWith("custom", "", nil, nil)
		parts = append(parts, fmt.Sprint(err))

		return template.HTML(strings.Join(parts, "|"))
	}

	got := renderRuntime(t, fsys, inspect)

	want := strings.Join([]string{
		"<p>card</p>/<nil>",
		"partial is not initialized",
		"<p>card</p>/<nil>",
		"true",
		"partial is not initialized",
		"terminal render stage is not configured",
	}, "|")
	if got != want {
		t.Fatalf("output =\n%s\nwant\n%s", got, want)
	}
}

// TestRuntimeFallbackRendersTheErrorFragment checks that a failing child
// rendered with RenderPartialWithFallback is replaced by the fragment the
// error stage renders.
func TestRuntimeFallbackRendersTheErrorFragment(t *testing.T) {
	fsys := Sources{
		"page.gohtml":   `<main>{{ inspect runtime }}</main>`,
		"broken.gohtml": `{{ template "missing" }}`,
	}

	errorStage := RenderStageHooks{
		RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
			if ctx.Kind != renderKindError {
				return next(ctx)
			}

			return template.HTML(`<p class="failed">` + template.HTMLEscapeString(ctx.Name) + `</p>`), nil
		},
	}

	inspect := func(rt *Runtime) template.HTML {
		out, err := rt.RenderPartialWithFallback(NewID("broken", "broken.gohtml"))
		if err != nil {
			return template.HTML(template.HTMLEscapeString(err.Error()))
		}

		return out
	}

	got := renderRuntime(t, fsys, inspect, func(p *Partial) {
		p.Use(errorStage)
	})

	if want := `<main><p class="failed">fragment</p></main>`; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

// TestRuntimeChecksContractsInTheTree checks that Runtime.CheckContracts
// finds the models of the tree the child would render in.
func TestRuntimeChecksContractsInTheTree(t *testing.T) {
	fsys := Sources{
		"page.gohtml": `{{ inspect runtime }}`,
		"row.gohtml":  `{{/* @model Site github.com/donseba/go-partial/contractSite */}}{{ Site.Name }}`,
	}

	inspect := func(rt *Runtime) template.HTML {
		row := NewID("row", "row.gohtml")

		inTree := rt.CheckContracts(row)
		alone := row.SetFileSystem(fsys).CheckContracts()

		return template.HTML(fmt.Sprintf("%v|%v", inTree, alone != nil))
	}

	got := renderRuntime(t, fsys, inspect, func(p *Partial) {
		p.SetModel(contractSite{Name: "acme"})
	})

	if got != "<nil>|true" {
		t.Fatalf("output = %q, want the check to pass in the tree and fail alone", got)
	}

	if err := (*Runtime)(nil).CheckContracts(nil); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("nil runtime CheckContracts() error = %v", err)
	}
}
