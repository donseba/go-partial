package partial

import (
	"context"
	"fmt"
	"html/template"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

type greetingKey struct{}

func TestReservedFuncNames(t *testing.T) {
	stage := RenderStageHooks{FuncNames: []string{"locale", "localizer"}}

	names := ReservedFuncNames(stage)
	for _, name := range []string{"content", "partial", "runtime", "url", "locale", "localizer"} {
		if !slices.Contains(names, name) {
			t.Fatalf("ReservedFuncNames() = %v, want %s", names, name)
		}
	}

	if !slices.IsSorted(names) {
		t.Fatalf("ReservedFuncNames() = %v, want sorted", names)
	}

	if !IsReservedFuncName("locale", stage) || !IsReservedFuncName("_private") || !IsReservedFuncName("ctx") {
		t.Fatal("IsReservedFuncName() = false for a reserved name")
	}

	if IsReservedFuncName("money", stage) || IsReservedFuncName("locale") {
		t.Fatal("IsReservedFuncName() = true for a free name")
	}
}

func TestContextFuncsReceiveTheRenderContext(t *testing.T) {
	fsys := Sources{
		"page.gohtml": `{{ greet "Ada" }}|{{ join "-" "a" "b" }}|{{ partial runtime "card.gohtml" }}|{{ content }}`,
		"card.gohtml": `{{ greet "card" }}`,
		"body.gohtml": `{{ greet "body" }}`,
	}
	funcs := template.FuncMap{
		"greet": func(ctx context.Context, name string) (string, error) {
			return fmt.Sprintf("%v %s", ctx.Value(greetingKey{}), name), nil
		},
		"join": func(ctx context.Context, separator string, parts ...string) string {
			return strings.Join(parts, separator)
		},
	}

	for _, cache := range []bool{false, true} {
		root := NewID("page", "page.gohtml").SetFileSystem(fsys).UseTemplateCache(cache).SetFunc(funcs)
		root.SetContent(NewID("body", "body.gohtml").UseTemplateCache(cache))

		for _, greeting := range []string{"hello", "hi"} {
			ctx := context.WithValue(context.Background(), greetingKey{}, greeting)

			out, err := Render(ctx, root.Clone())
			if err != nil {
				t.Fatalf("cache %v: Render() error = %v", cache, err)
			}

			want := greeting + " Ada|a-b|" + greeting + " card|" + greeting + " body"
			if string(out) != want {
				t.Fatalf("cache %v: Render() = %q, want %q", cache, out, want)
			}
		}
	}
}

func TestNearerFuncReplacesContextFunc(t *testing.T) {
	fsys := Sources{"page.gohtml": `{{ name }}`}

	root := New().SetFileSystem(fsys).SetFunc(template.FuncMap{
		"name": func(context.Context) string { return "root" },
	})

	for _, cache := range []bool{false, true} {
		child := NewID("child", "page.gohtml").UseTemplateCache(cache).SetFunc(template.FuncMap{
			"name": func() string { return "child" },
		})
		root.With(child)

		out, err := Render(context.Background(), child)
		if err != nil || out != "child" {
			t.Fatalf("cache %v: Render() = %q, %v; want the child's function", cache, out, err)
		}
	}
}

func TestContextFuncsAreBoundOncePerContext(t *testing.T) {
	contextual, ok := asContextFunc(func(ctx context.Context) string { return "" })
	if !ok {
		t.Fatal("asContextFunc() = false for a function with a context")
	}

	ctx := withContextBindings(context.Background())
	bindings := contextBindingsFrom(ctx)

	bindings.bind(ctx, "name", contextual)
	bindings.bind(ctx, "name", contextual)

	if got := bindings.funcs[contextBindingKey{name: "name", code: contextual.fn.Pointer()}]; got.ctx != ctx {
		t.Fatal("bind() did not keep the binding")
	}

	derived := context.WithValue(ctx, greetingKey{}, "x")
	bindings.bind(derived, "name", contextual)

	if got := bindings.funcs[contextBindingKey{name: "name", code: contextual.fn.Pointer()}]; got.ctx != derived {
		t.Fatal("bind() kept a binding of another context")
	}

	if len(bindings.funcs) != 1 {
		t.Fatalf("bindings = %d, want 1", len(bindings.funcs))
	}
}

func TestBindContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), greetingKey{}, "hello")
	funcs := BindContext(ctx, template.FuncMap{
		"greet": func(ctx context.Context, name string) string { return fmt.Sprint(ctx.Value(greetingKey{}), " ", name) },
		"upper": strings.ToUpper,
	})

	tmpl := template.Must(template.New("mail").Funcs(funcs).Parse(`{{ greet "Ada" | upper }}`))

	var out strings.Builder
	if err := tmpl.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}

	if out.String() != "HELLO ADA" {
		t.Fatalf("Execute() = %q", out.String())
	}
}

func TestRuntimeFromReachesTheCallingPartial(t *testing.T) {
	fsys := Sources{
		"page.gohtml": `{{ shout "page" }}|{{ content }}`,
		"body.gohtml": `{{ card "body" }}`,
		"card.gohtml": `{{/* @model Site github.com/donseba/go-partial.contractSite */}}{{ . }} in {{ Site.Name }}`,
	}
	funcs := template.FuncMap{
		"card": func(ctx context.Context, name string) (template.HTML, error) {
			runtime, ok := RuntimeFrom(ctx)
			if !ok {
				return "", fmt.Errorf("card %s: no runtime", name)
			}

			card := NewID("card", "card.gohtml").SetDot(name + "@" + runtimePartialID(runtime))

			return runtime.RenderPartial(card)
		},
		"shout": func(ctx context.Context, name string) string {
			runtime, _ := RuntimeFrom(ctx)
			return name + "@" + runtimePartialID(runtime)
		},
	}

	for _, cache := range []bool{false, true} {
		root := NewID("page", "page.gohtml").
			SetFileSystem(fsys).
			UseTemplateCache(cache).
			SetFunc(funcs).
			SetContent(NewID("body", "body.gohtml").UseTemplateCache(cache).SetModel(contractSite{Name: "body site"}))

		out, err := Render(context.Background(), root)
		if err != nil {
			t.Fatalf("cache %v: Render() error = %v", cache, err)
		}

		// The card is a child of the body, which has the site.
		want := "page@page|body@body in body site"
		if string(out) != want {
			t.Fatalf("cache %v: Render() = %q, want %q", cache, out, want)
		}
	}

	if _, ok := RuntimeFrom(context.Background()); ok {
		t.Fatal("RuntimeFrom() outside a render = true")
	}
}

func runtimePartialID(runtime *Runtime) string {
	return runtime.RenderContext().Partial.PartialID()
}

func TestSetFuncWarnsAboutReservedNames(t *testing.T) {
	var warned []string

	sink := EventSinkFunc(func(ctx *RenderContext, event Event) {
		if event.Kind == EventFuncProtected && event.Level == EventWarn {
			warned = append(warned, event.Fields["function"].(string))
		}
	})

	root := New().SetEvents(sink).Use(RenderStageHooks{FuncNames: []string{"greet"}})
	root.SetFunc(template.FuncMap{"greet": strings.ToUpper, "content": strings.ToUpper, "title": strings.ToUpper})

	slices.Sort(warned)

	if !slices.Equal(warned, []string{"content", "greet"}) {
		t.Fatalf("warned about %v", warned)
	}

	warned = nil
	root.SetFunc(template.FuncMap{"shout": strings.ToUpper})
	root.Use(RenderStageHooks{FuncNames: []string{"shout"}})

	if !slices.Equal(warned, []string{"shout"}) {
		t.Fatalf("Use warned about %v, want the function it shadows", warned)
	}
}

func TestStageFuncsNeedNoStandInsWithTheCache(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ greet "you" }}`)},
	}

	p := New("page.gohtml").
		SetFileSystem(fsys).
		UseTemplateCache(true).
		Use(RenderStageHooks{
			FuncNames: []string{"greet"},
			ResolveFunc: func(ctx *RenderContext, name string) (any, bool) {
				return func(who string) string { return "hello " + who }, name == "greet"
			},
		})

	for range 2 {
		out, err := Render(context.Background(), p)
		if err != nil || string(out) != "hello you" {
			t.Fatalf("Render() = %q, %v", out, err)
		}
	}

	declining := New("page.gohtml").
		SetFileSystem(fsys).
		Use(RenderStageHooks{FuncNames: []string{"greet"}})
	if _, err := Render(context.Background(), declining); err == nil || !strings.Contains(err.Error(), "no render stage") {
		t.Fatalf("Render() error = %v, want the stand-in's error", err)
	}
}

func TestSetFuncDoesNotWarnAboutAStagesOwnStandIns(t *testing.T) {
	warned := false

	sink := EventSinkFunc(func(ctx *RenderContext, event Event) {
		warned = warned || event.Kind == EventFuncProtected
	})

	stage := RenderStageHooks{
		FuncNames: []string{"greet"},
		ResolveFunc: func(ctx *RenderContext, name string) (any, bool) {
			return nil, false
		},
	}

	// The stand-in comes from the package of the stage, as slots.FuncMap
	// does for slots.Stage.
	New().SetEvents(sink).Use(stage).SetFunc(template.FuncMap{"greet": func() string { return "" }})
	New().SetEvents(sink).SetFunc(template.FuncMap{"greet": func() string { return "" }}).Use(stage)

	if warned {
		t.Fatal("registering a stage's own stand-in should not warn")
	}

	if got := packageOf("github.com/acme/app.(*T).Method.func1"); got != "github.com/acme/app" {
		t.Fatalf("packageOf() = %q", got)
	}

	// Function names escape the dots of the last element of the path,
	// which reflect's PkgPath, a Package is set from, keeps.
	if got := packageOf("gopkg.in/yaml%2ev3.Unmarshal"); got != "gopkg.in/yaml.v3" {
		t.Fatalf("packageOf() = %q", got)
	}
}

// TestStagePackageNamesTheStagesOwnFunctions checks that a stage with a
// Package is the stage of that package whatever package its hooks come
// from, as where the compiler inlines the function that returns the stage
// into the application, and also without hooks.
func TestStagePackageNamesTheStagesOwnFunctions(t *testing.T) {
	var warned []string

	sink := EventSinkFunc(func(ctx *RenderContext, event Event) {
		if event.Kind == EventFuncProtected {
			warned = append(warned, event.Fields["function"].(string))
		}
	})

	// The hook comes from this package, the stand-in from strings.
	hooked := RenderStageHooks{
		Package:   "strings",
		FuncNames: []string{"upper"},
		ResolveFunc: func(ctx *RenderContext, name string) (any, bool) {
			return nil, false
		},
	}

	bare := RenderStageHooks{
		Package:   "strings",
		FuncNames: []string{"upper"},
	}

	standIn := template.FuncMap{"upper": strings.ToUpper}

	for _, stage := range []RenderStageHooks{hooked, bare} {
		if got := stagePackage(stage); got != "strings" {
			t.Fatalf("stagePackage() = %q, want strings", got)
		}

		New().SetEvents(sink).Use(stage).SetFunc(standIn)
		New().SetEvents(sink).SetFunc(standIn).Use(&stage)
	}

	if len(warned) > 0 {
		t.Fatalf("warned about the stage's own stand-ins %v", warned)
	}

	// A function of this package is the application's, which the stage
	// replaces.
	own := template.FuncMap{
		"upper": func(s string) string {
			return s
		},
	}

	New().SetEvents(sink).Use(hooked).SetFunc(own)

	if !slices.Equal(warned, []string{"upper"}) {
		t.Fatalf("warned about %v, want upper", warned)
	}
}

// TestFallbackIgnoresTheStagesOwnFunctions checks that a function of the
// package a stage names does not count as registered for its
// FallbackFuncNames, whatever package the stage's hooks come from: the
// stage's function is called, and a function of the application's wins.
func TestFallbackIgnoresTheStagesOwnFunctions(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ upper "x" }}`)},
	}

	stage := RenderStageHooks{
		Package:           "strings",
		FallbackFuncNames: []string{"upper"},
		ResolveFunc: func(ctx *RenderContext, name string) (any, bool) {
			return func(string) string { return "stage" }, name == "upper"
		},
	}

	own := template.FuncMap{
		"upper": func(s string) string {
			return "own " + s
		},
	}

	for _, cached := range []bool{false, true} {
		standIn := New("page.gohtml").
			SetFileSystem(fsys).
			UseTemplateCache(cached).
			SetFunc(template.FuncMap{"upper": strings.ToUpper}).
			Use(stage)

		out, err := Render(context.Background(), standIn)
		if err != nil || string(out) != "stage" {
			t.Fatalf("cache %v: with the stage's own = %q, %v", cached, out, err)
		}

		registered := New("page.gohtml").
			SetFileSystem(fsys).
			UseTemplateCache(cached).
			SetFunc(own).
			Use(stage)

		out, err = Render(context.Background(), registered)
		if err != nil || string(out) != "own x" {
			t.Fatalf("cache %v: with the application's = %q, %v", cached, out, err)
		}
	}
}
