package partial

import (
	"reflect"
	"testing"
)

func TestRequiredFuncsFindsTopLevelFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ child "content" }}{{ if eq .Status "ok" }}{{ debug . }}{{ end }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"child", "debug"}
	if !reflect.DeepEqual(funcs, want) {
		t.Fatalf("RequiredFuncs() = %#v, want %#v", funcs, want)
	}
}

func TestRequiredFuncsFindsDefinedTemplateFunctions(t *testing.T) {
	funcs, err := RequiredFuncs("page.gohtml", `{{ define "row" }}{{ partial "row" "Row" . }}{{ scoped.Row.ID }}{{ end }}`)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"partial", "scoped"}
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
