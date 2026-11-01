package templateutil

import (
	"bytes"
	"html/template"
	"reflect"
	"testing"
)

func TestRequiredFunctionScannerFindsTopLevelFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ partial runtime "templates/content.gohtml" }}{{ if eq .Status "ok" }}{{ debug runtime . }}{{ end }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"debug", "partial", "runtime"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

func TestRequiredFunctionScannerFindsDefinedTemplateFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ define "row" }}{{ partial runtime "templates/row.gohtml" . }}{{ end }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"partial", "runtime"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

func TestRequiredFunctionScannerFindsPipelineFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ .Price | money }} {{ printf "%s" .Name }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"money"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

// TestRequiredFunctionScannerWalksEveryNode checks that functions are found
// in the pipelines and branches of if, range, with and template actions, and
// in chained calls, and that builtins are left out.
func TestRequiredFunctionScannerWalksEveryNode(t *testing.T) {
	src := `{{ if ok }}{{ a }}{{ else }}{{ b }}{{ end }}` +
		`{{ range items }}{{ c }}{{ else }}{{ d }}{{ end }}` +
		`{{ with item }}{{ e }}{{ else }}{{ f }}{{ end }}` +
		`{{ template "row" g }}` +
		`{{ (h).Field }}` +
		`{{ len (index .List 0) }}`

	funcs, err := RequiredFuncs("page.gohtml", src)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"a", "b", "c", "d", "e", "f", "g", "h", "item", "items", "ok"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

func TestTemplateScannersReportParseErrors(t *testing.T) {
	src := `{{ if }}`

	if _, err := RequiredFuncs("broken.gohtml", src); err == nil {
		t.Fatal("RequiredFuncs() of a broken template: want an error")
	}

	if _, err := ReferencedTemplates("broken.gohtml", src); err == nil {
		t.Fatal("ReferencedTemplates() of a broken template: want an error")
	}

	if _, err := DefinedTemplates("broken.gohtml", src); err == nil {
		t.Fatal("DefinedTemplates() of a broken template: want an error")
	}
}

// TestReferencedTemplatesFindsNestedCalls checks that template calls are
// found inside branches and defined templates, once each, sorted.
func TestReferencedTemplatesFindsNestedCalls(t *testing.T) {
	src := `{{ define "row" }}{{ template "cell" . }}{{ end }}` +
		`{{ if .A }}{{ template "a" }}{{ else }}{{ template "b" }}{{ end }}` +
		`{{ range .Items }}{{ template "row" . }}{{ else }}{{ template "empty" }}{{ end }}` +
		`{{ with .C }}{{ template "c" (.Value) }}{{ else }}{{ template "cell" }}{{ end }}` +
		`{{ template "chained" (.D).Value }}` +
		`{{ .Name | printf "%s" }}`

	names, err := ReferencedTemplates("page.gohtml", src)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"a", "b", "c", "cell", "chained", "empty", "row"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("ReferencedTemplates() = %#v, want %#v", names, want)
	}
}

func TestDefinedTemplatesIncludesTheFile(t *testing.T) {
	names, err := DefinedTemplates("page.gohtml", `{{ define "row" }}r{{ end }}{{ define "cell" }}c{{ end }}page`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"cell", "page.gohtml", "row"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("DefinedTemplates() = %#v, want %#v", names, want)
	}
}

func TestNormalizeContractType(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "Site", want: "Site"},
		{in: "site.Site", want: "site.Site"},
		{in: "github.com/acme/site.Site", want: "github.com/acme/site.Site"},
		{in: "github.com/acme/site/Site", want: "github.com/acme/site.Site"},
	}

	for _, tt := range tests {
		if got := NormalizeContractType(tt.in); got != tt.want {
			t.Errorf("NormalizeContractType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPathBaseAndAliases(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		aliases []string
	}{
		{name: "templates/card.gohtml", base: "card.gohtml", aliases: []string{"templates/card.gohtml", "/templates/card.gohtml"}},
		{name: "/templates/card.gohtml", base: "card.gohtml", aliases: []string{"templates/card.gohtml"}},
		{name: `templates\card.gohtml`, base: "card.gohtml", aliases: []string{`templates\card.gohtml`, `/templates\card.gohtml`}},
		{name: `\card.gohtml`, base: "card.gohtml", aliases: []string{"card.gohtml"}},
		{name: "card.gohtml", base: "card.gohtml", aliases: []string{"card.gohtml", "/card.gohtml"}},
		{name: "/", base: "", aliases: nil},
		{name: "", base: "", aliases: nil},
	}

	for _, tt := range tests {
		if got := PathBase(tt.name); got != tt.base {
			t.Errorf("PathBase(%q) = %q, want %q", tt.name, got, tt.base)
		}

		if got := PathAliases(tt.name); !reflect.DeepEqual(got, tt.aliases) {
			t.Errorf("PathAliases(%q) = %#v, want %#v", tt.name, got, tt.aliases)
		}
	}
}

// TestAddPathAliasesCallsTheBaseTemplate checks that a file parsed under its
// base name can be called by its path, with and without a leading slash,
// and that names without a parsed base, or that are their base, get no
// aliases.
func TestAddPathAliasesCallsTheBaseTemplate(t *testing.T) {
	if err := AddPathAliases(nil, []string{"templates/card.gohtml"}); err != nil {
		t.Fatalf("AddPathAliases(nil) = %v", err)
	}

	tmpl := template.Must(template.New("card.gohtml").Parse(`<p>{{ . }}</p>`))
	template.Must(tmpl.New("/templates/card.gohtml").Parse(`taken`))

	err := AddPathAliases(tmpl, []string{"", "card.gohtml", "templates/card.gohtml", "templates/missing.gohtml"})
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "templates/card.gohtml", "hello"); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "<p>hello</p>" {
		t.Fatalf("alias rendered %q", got)
	}

	out.Reset()

	if err := tmpl.ExecuteTemplate(&out, "/templates/card.gohtml", "hello"); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); got != "taken" {
		t.Fatalf("an existing template was replaced by an alias: %q", got)
	}

	if tmpl.Lookup("templates/missing.gohtml") != nil {
		t.Fatal("an alias was added for a file that was not parsed")
	}
}

func TestAddPathAliasesReportsParseErrors(t *testing.T) {
	tmpl := template.Must(template.New("card.gohtml").Parse(`card`))

	var out bytes.Buffer
	if err := tmpl.Execute(&out, nil); err != nil {
		t.Fatal(err)
	}

	// A template that has executed cannot get new templates.
	if err := AddPathAliases(tmpl, []string{"templates/card.gohtml"}); err == nil {
		t.Fatal("AddPathAliases() after the template executed: want an error")
	}
}

func TestContractScanText(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "no comments", src: "@model Site a.B\n<p></p>", want: "@model Site a.B\n<p></p>"},
		{name: "comment bodies", src: "{{/* @model Site a.B */}}<p>{{/*\n @dot a.C\n*/}}</p>", want: "@model Site a.B\n@dot a.C\n"},
		{name: "empty comments", src: "{{/* */}}<p></p>", want: "{{/* */}}<p></p>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contractScanText(tt.src); got != tt.want {
				t.Fatalf("contractScanText() = %q, want %q", got, tt.want)
			}
		})
	}
}
