package partial

import (
	"context"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"
)

// checkStage gives templates the function "region", as a render stage.
var checkStage = RenderStageHooks{FuncNames: []string{"region"}}

func TestCheckTemplatesReportsEveryProblem(t *testing.T) {
	files := fstest.MapFS{
		// Valid: a registered function of the root, a function with a
		// context, go-partial's helpers, the stage's function, a typed
		// root, a template of another file by path and a define of its own.
		"layout.gohtml": {Data: []byte(`{{/*
@model Nav example.com/site.Nav
*/}}<body>{{ upper "x" }} {{ today }} {{ content }} {{ url }} {{ region }}
{{ len Nav.Items }}{{ template "partials/card.gohtml" . }}{{ template "inner" . }}
{{ define "inner" }}{{ printf "%d" 1 }}{{ end }}</body>`)},
		"partials/card.gohtml": {Data: []byte(`<div>{{ . }}</div>`)},
		// Two unknown functions and an unknown template in one file, also
		// in a define and in a branch.
		"page.gohtml": {Data: []byte(`<main>
  {{ newsleterForm "letter" }}
  {{ if true }}{{ range pollz }}{{ . }}{{ end }}{{ end }}
{{ define "aside" }}{{ template "partials/crd.gohtml" . }}{{ end }}
</main>`)},
		// A file that does not parse.
		"regions/menu.gohtml": {Data: []byte("<nav>\n{{ if .Items }}\n<ul></ul>\n</nav>")},
		// Not a template.
		"assets/site.css": {Data: []byte(`{{ nothing }}`)},
	}

	root := NewID("root", "layout.gohtml").
		SetFileSystem(files).
		SetFunc(template.FuncMap{
			"upper":          strings.ToUpper,
			"newsletterForm": func(string) string { return "" },
			"poll":           func() string { return "" },
			"today":          func(context.Context) string { return "" },
		}).
		Use(checkStage)

	problems, err := root.CheckTemplates()
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(problems))
	for _, problem := range problems {
		got = append(got, problem.Error())
	}

	want := []string{
		`page.gohtml:2:6: function "newsleterForm" not defined; did you mean "newsletterForm"?`,
		`page.gohtml:3:25: function "pollz" not defined; did you mean "poll"?`,
		`page.gohtml:4:33: no such template "partials/crd.gohtml"; did you mean "partials/card.gohtml"?`,
		`regions/menu.gohtml:4: unexpected EOF: an {{if}}, {{range}}, {{with}}, {{block}} or {{define}} lacks its {{end}}`,
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if problems[0].Func != "newsleterForm" || problems[0].Suggestion != "newsletterForm" ||
		problems[2].Template != "partials/crd.gohtml" {
		t.Fatalf("problem fields: %+v", problems)
	}
}

// A partial has the functions and stages of its ancestors, as its renders
// do, and the pattern chooses the files.
func TestCheckTemplatesUsesAncestorsAndPatterns(t *testing.T) {
	files := fstest.MapFS{
		"page.tmpl":    {Data: []byte(`{{ upper "x" }}{{ region }}`)},
		"page.gohtml":  {Data: []byte(`{{ missing }}`)},
		"other.gohtml": {Data: []byte(`{{ upper "x" }}`)},
	}

	root := NewID("root").
		SetFileSystem(files).
		SetFunc(template.FuncMap{"upper": strings.ToUpper}).
		Use(checkStage)
	child := NewID("child").SetParent(root)

	problems, err := child.CheckTemplates("*.tmpl")
	if err != nil || len(problems) != 0 {
		t.Fatalf("CheckTemplates(*.tmpl) = %v, %v", problems, err)
	}

	problems, err = child.CheckTemplates()
	if err != nil || len(problems) != 1 || problems[0].Position() != "page.gohtml:1:4" || problems[0].Suggestion != "" {
		t.Fatalf("CheckTemplates() = %+v, %v", problems, err)
	}
}

// Of an Overlay, a problem names the layer that serves the file: a theme's
// own file before the default theme's of the same name.
func TestCheckTemplatesNamesTheLayer(t *testing.T) {
	theme := fstest.MapFS{
		"page.gohtml": {Data: []byte(`{{ themeOnly }}`)},
	}
	base := fstest.MapFS{
		"page.gohtml":   {Data: []byte(`{{ shadowed }}`)},
		"layout.gohtml": {Data: []byte("<body>\n{{ fromBase }}</body>")},
	}

	root := NewID("root").SetFileSystem(Overlay(theme, base))

	problems, err := root.CheckTemplates()
	if err != nil {
		t.Fatal(err)
	}

	if len(problems) != 2 {
		t.Fatalf("problems = %+v", problems)
	}

	if problems[0].File != "layout.gohtml" || problems[0].Layer != 1 || problems[0].Position() != "layout.gohtml:2:4" {
		t.Fatalf("base problem = %+v", problems[0])
	}

	if problems[1].File != "page.gohtml" || problems[1].Layer != 0 || problems[1].Func != "themeOnly" {
		t.Fatalf("theme problem = %+v", problems[1])
	}
}

// What the check passes renders, and what it reports fails the render.
func TestCheckTemplatesAgreesWithRender(t *testing.T) {
	files := fstest.MapFS{
		"good.gohtml": {Data: []byte(`<p>{{ upper "x" }}{{ url }}{{ region }}</p>`)},
		"bad.gohtml":  {Data: []byte(`<p>{{ uper "x" }}</p>`)},
	}

	for _, cached := range []bool{false, true} {
		root := NewID("root").
			SetFileSystem(files).
			UseTemplateCache(cached).
			SetFunc(template.FuncMap{"upper": strings.ToUpper}).
			Use(RenderStageHooks{
				FuncNames: []string{"region"},
				PrepareFunc: func(ctx *RenderContext) (*RenderContext, error) {
					ctx.SetFunc("region", func() string { return "r" })

					return ctx, nil
				},
			})

		problems, err := root.CheckTemplates()
		if err != nil || len(problems) != 1 || problems[0].File != "bad.gohtml" {
			t.Fatalf("cache %v: CheckTemplates() = %+v, %v", cached, problems, err)
		}

		if _, err := Render(context.Background(), NewID("good", "good.gohtml").SetParent(root)); err != nil {
			t.Fatalf("cache %v: good.gohtml: %v", cached, err)
		}

		_, err = Render(context.Background(), NewID("bad", "bad.gohtml").SetParent(root))
		if err == nil || !strings.Contains(err.Error(), `function "uper" not defined`) {
			t.Fatalf("cache %v: bad.gohtml: %v", cached, err)
		}
	}
}
