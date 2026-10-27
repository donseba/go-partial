package partial

import (
	"reflect"
	"strings"
	"testing"
)

type contractRow struct {
	ID int
}

type contractSite struct {
	Name string
}

func TestParseDeclarations(t *testing.T) {
	declarations, err := ParseDeclarations(
		`{{/*
@dot github.com/donseba/go-partial.contractRow
@model Site github.com/donseba/go-partial/contractSite
*/}}`,
		`{{/* @model Site github.com/donseba/go-partial.contractSite
@interaction Likes github.com/donseba/go-partial/exp/interactions.Interaction */}}`,
	)
	if err != nil {
		t.Fatalf("ParseDeclarations() error = %v", err)
	}

	want := []Declaration{
		{Annotation: "dot", Type: "github.com/donseba/go-partial.contractRow"},
		{Annotation: "model", Name: "Site", Type: "github.com/donseba/go-partial.contractSite"},
		{Annotation: "interaction", Name: "Likes", Type: "github.com/donseba/go-partial/exp/interactions.Interaction"},
	}
	if !reflect.DeepEqual(declarations, want) {
		t.Fatalf("ParseDeclarations() = %#v, want %#v", declarations, want)
	}

	_, err = ParseDeclarations(`{{/* @model Site a.B */}}`, `{{/* @model Site a.C */}}`)
	if err == nil {
		t.Fatal("ParseDeclarations() with a root of two types: want an error")
	}
}

func TestCheckContracts(t *testing.T) {
	fsys := Sources{
		"row.gohtml": `{{/*
@dot github.com/donseba/go-partial.contractRow
@model Site github.com/donseba/go-partial.contractSite
*/}}{{ Site.Name }}:{{ .ID }}`,
		"rows.gohtml": `{{/* @dot []github.com/donseba/go-partial.contractRow */}}{{ len . }}`,
		"any.gohtml":  `{{/* @dot any */}}{{ . }}`,
		"none.gohtml": `plain`,
	}

	root := New().SetFileSystem(fsys).SetModel(contractSite{Name: "acme"})
	check := func(p *Partial) error {
		root.With(p)
		return p.CheckContracts()
	}

	if err := check(NewID("row", "row.gohtml").SetDot(contractRow{ID: 1})); err != nil {
		t.Fatalf("row: CheckContracts() error = %v", err)
	}

	if err := check(NewID("row", "row.gohtml").SetDot(&contractRow{ID: 1})); err != nil {
		t.Fatalf("row behind a pointer: CheckContracts() error = %v", err)
	}

	if err := check(NewID("rows", "rows.gohtml").SetDot([]contractRow{{ID: 1}})); err != nil {
		t.Fatalf("rows: CheckContracts() error = %v", err)
	}

	if err := check(NewID("any", "any.gohtml")); err != nil {
		t.Fatalf("any: CheckContracts() error = %v", err)
	}

	if err := check(NewID("none", "none.gohtml")); err != nil {
		t.Fatalf("none: CheckContracts() error = %v", err)
	}

	err := check(NewID("row", "row.gohtml").SetDot("not a row"))
	if err == nil || !strings.Contains(err.Error(), "@dot expects github.com/donseba/go-partial.contractRow, got string") {
		t.Fatalf("wrong dot: CheckContracts() error = %v", err)
	}

	alone := NewID("row", "row.gohtml").SetFileSystem(fsys).SetDot(contractRow{ID: 1})

	err = alone.CheckContracts()
	if err == nil || !strings.Contains(err.Error(), "@model Site has no matching value") {
		t.Fatalf("without the model: CheckContracts() error = %v", err)
	}

	missing := NewID("missing", "missing.gohtml").SetFileSystem(fsys)
	if err := missing.CheckContracts(); err == nil {
		t.Fatal("missing template: CheckContracts() want an error")
	}
}
