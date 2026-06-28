package partial

import (
	"reflect"
	"testing"
)

func TestRequiredFuncsFindsTopLevelFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ partial "templates/content.gohtml" }}{{ if eq .Status "ok" }}{{ debug . }}{{ end }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"debug", "partial"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

func TestRequiredFuncsFindsDefinedTemplateFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ define "row" }}{{ partial "templates/row.gohtml" . }}{{ end }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"partial"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

func TestRequiredFuncsFindsPipelineFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ .Price | money }} {{ printf "%s" .Name }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"money"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}
