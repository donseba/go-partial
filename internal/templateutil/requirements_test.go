package templateutil

import (
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
