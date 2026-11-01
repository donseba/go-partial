package templateutil

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// TestScannersOfStoresAreApart checks that two stores keep the scans,
// existing files and include matches of one file system apart, and that a
// store keeps them for a key and not without one.
func TestScannersOfStoresAreApart(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml":           &fstest.MapFile{Data: []byte(`{{ define "a" }}a{{ end }}`)},
		"partials/card.gohtml":  &fstest.MapFile{Data: []byte(`card`)},
		"partials/dir/x.gohtml": &fstest.MapFile{Data: []byte(`x`)},
	}

	first := NewStore().Scanner(fsys, "fs")
	if first.IsFile("500.gohtml") {
		t.Fatal("500.gohtml exists before it is added")
	}

	matches, err := first.Glob("partials/*")
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(matches, []string{"partials/card.gohtml"}) {
		t.Fatalf("matches = %v, want the file and not the directory", matches)
	}

	scan, err := first.File("page.gohtml")
	if err != nil || !slices.Contains(scan.Defined, "a") {
		t.Fatalf("scan = %+v, %v", scan, err)
	}

	fsys["500.gohtml"] = &fstest.MapFile{Data: []byte(`error`)}
	fsys["partials/more.gohtml"] = &fstest.MapFile{Data: []byte(`more`)}
	fsys["page.gohtml"] = &fstest.MapFile{Data: []byte(`{{ define "b" }}b{{ end }}`)}

	// The same store keeps its answers for the key.
	again := first
	if again.IsFile("500.gohtml") {
		t.Fatal("the store forgot that 500.gohtml was missing")
	}

	if matches, _ := again.Glob("partials/*"); len(matches) != 1 {
		t.Fatalf("matches = %v, want those found before", matches)
	}

	// Another store over the same file system and key asks again.
	second := NewStore().Scanner(fsys, "fs")
	if !second.IsFile("500.gohtml") {
		t.Fatal("a second store does not see 500.gohtml")
	}

	if matches, _ := second.Glob("partials/*"); len(matches) != 2 {
		t.Fatalf("second store matches = %v, want both files", matches)
	}

	if scan, _ := second.File("page.gohtml"); !slices.Contains(scan.Defined, "b") {
		t.Fatalf("second store scan = %+v, want the new content", scan)
	}

	// Without a key, or without a store, nothing is kept.
	for _, scanner := range []Scanner{NewStore().Scanner(fsys, ""), (*Store)(nil).Scanner(fsys, "fs")} {
		delete(fsys, "500.gohtml")

		if scanner.IsFile("500.gohtml") {
			t.Fatal("an uncached scanner sees a deleted file")
		}

		fsys["500.gohtml"] = &fstest.MapFile{Data: []byte(`error`)}

		if !scanner.IsFile("500.gohtml") {
			t.Fatal("an uncached scanner does not see a new file")
		}
	}
}

// errFS fails every call with err.
type errFS struct {
	err error
}

func (f errFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: f.err}
}

func TestScanTemplateReadsDeclarationsAndUses(t *testing.T) {
	src := `{{/* @model Site github.com/acme/site/Site */}}` +
		`{{/* @dot github.com/acme/site.Page */}}` +
		`{{/* @func money func(int) string */}}` +
		`{{ define "row" }}{{ template "cell" . }}{{ money .Price }}{{ end }}` +
		`{{ if eq .A 1 }}{{ template "row" . }}{{ end }}`

	scan := ScanTemplate("page.gohtml", src)
	if scan.ParseErr != nil {
		t.Fatal(scan.ParseErr)
	}

	if !slices.Equal(scan.Defined, []string{"page.gohtml", "row"}) {
		t.Fatalf("Defined = %v", scan.Defined)
	}

	if !slices.Equal(scan.Referenced, []string{"cell", "row"}) {
		t.Fatalf("Referenced = %v", scan.Referenced)
	}

	if !slices.Equal(scan.Required, []string{"money"}) {
		t.Fatalf("Required = %v, want money without the builtin eq", scan.Required)
	}

	if scan.Dot != "github.com/acme/site.Page" {
		t.Fatalf("Dot = %q", scan.Dot)
	}

	want := []ContractDeclaration{{Annotation: "model", Name: "Site", Type: "github.com/acme/site.Site"}}
	if !slices.Equal(scan.Contracts, want) {
		t.Fatalf("Contracts = %+v, want %+v (no @dot or @func)", scan.Contracts, want)
	}
}

func TestScanTemplateKeepsTheParseError(t *testing.T) {
	scan := ScanTemplate("broken.gohtml", "{{/* @model Site a.B */}}{{ if }}")

	if scan.ParseErr == nil {
		t.Fatal("ParseErr = nil for a broken template")
	}

	if len(scan.Defined) != 0 || len(scan.Referenced) != 0 || len(scan.Required) != 0 {
		t.Fatalf("broken scan = %+v, want no names", scan)
	}

	if len(scan.Contracts) != 1 {
		t.Fatalf("Contracts = %+v, want the declaration read before parsing", scan.Contracts)
	}
}

// TestScannerReadsSetsOfFiles checks the functions, calls, names and typed
// roots of several files, and how files that are missing or do not parse
// are handled.
func TestScannerReadsSetsOfFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/page.gohtml": &fstest.MapFile{Data: []byte(
			`{{/* @model Site a.Site */}}{{ define "body" }}{{ money 1 }}{{ end }}{{ template "card" }}`,
		)},
		"templates/card.gohtml": &fstest.MapFile{Data: []byte(
			`{{/* @model Site a.Site */}}{{/* @model User a.User */}}{{ upper "x" }}{{ template "badge" }}`,
		)},
		"templates/other.gohtml":  &fstest.MapFile{Data: []byte(`{{/* @model Site b.Site */}}`)},
		"templates/broken.gohtml": &fstest.MapFile{Data: []byte(`{{ if }}`)},
	}

	scanner := NewStore().Scanner(fsys, "sets")

	funcs, err := scanner.RequiredFuncs([]string{"templates/page.gohtml", "templates/card.gohtml"})
	if err != nil {
		t.Fatal(err)
	}

	if !maps.Equal(funcs, map[string]struct{}{"money": {}, "upper": {}}) {
		t.Fatalf("RequiredFuncs() = %v", funcs)
	}

	if _, err := scanner.RequiredFuncs([]string{"templates/missing.gohtml"}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("RequiredFuncs() of a missing file error = %v", err)
	}

	if _, err := scanner.RequiredFuncs([]string{"templates/broken.gohtml"}); err == nil {
		t.Fatal("RequiredFuncs() of a broken file: want its parse error")
	}

	refs := scanner.ReferencedTemplates([]string{
		"templates/page.gohtml", "templates/card.gohtml", "templates/broken.gohtml", "templates/missing.gohtml",
	})
	if !maps.Equal(refs, map[string]struct{}{"card": {}, "badge": {}}) {
		t.Fatalf("ReferencedTemplates() = %v", refs)
	}

	defined := scanner.DefinedTemplates([]string{"templates/page.gohtml", "templates/broken.gohtml"})
	for _, name := range []string{
		"templates/page.gohtml", "/templates/page.gohtml", "page.gohtml", "body",
		"templates/broken.gohtml", "broken.gohtml",
	} {
		if _, ok := defined[name]; !ok {
			t.Errorf("DefinedTemplates() lacks %q: %v", name, defined)
		}
	}

	one, err := scanner.RootContracts([]string{"templates/card.gohtml"})
	if err != nil {
		t.Fatal(err)
	}

	if len(one) != 2 || one["User"].Type != "a.User" || one["Site"].Annotation != "model" {
		t.Fatalf("RootContracts() of one file = %+v", one)
	}

	both, err := scanner.RootContracts([]string{"templates/page.gohtml", "templates/card.gohtml"})
	if err != nil {
		t.Fatal(err)
	}

	if len(both) != 2 || both["Site"].Type != "a.Site" {
		t.Fatalf("RootContracts() of two files = %+v", both)
	}

	_, err = scanner.RootContracts([]string{"templates/page.gohtml", "templates/other.gohtml"})
	if err == nil || !strings.Contains(err.Error(), "@model Site is declared as both a.Site and b.Site") {
		t.Fatalf("RootContracts() of conflicting files error = %v", err)
	}

	for _, names := range [][]string{{"templates/missing.gohtml"}, {"templates/page.gohtml", "templates/missing.gohtml"}} {
		if _, err := scanner.RootContracts(names); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("RootContracts(%v) error = %v", names, err)
		}
	}
}

// TestScannerRootContractsOfOneFileConflict checks that a file declaring a
// root twice with two types reports it, each time it is asked.
func TestScannerRootContractsOfOneFileConflict(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte("{{/*\n@model Site a.Site\n@model Site b.Site\n*/}}")},
	}
	scanner := NewStore().Scanner(fsys, "conflict")

	for range 2 {
		_, err := scanner.RootContracts([]string{"page.gohtml"})
		if err == nil || !strings.Contains(err.Error(), "declared as both") {
			t.Fatalf("RootContracts() error = %v", err)
		}
	}
}

// TestScannerKeepsScansForItsKey checks that a keyed scanner reads a file
// once, and that an unkeyed one reads it every time.
func TestScannerKeepsScansForItsKey(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`{{ a }}`)},
	}

	keyed := NewStore().Scanner(fsys, "keyed")
	unkeyed := NewStore().Scanner(fsys, "")

	first, err := keyed.File("page.gohtml")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := unkeyed.File("page.gohtml"); err != nil {
		t.Fatal(err)
	}

	fsys["page.gohtml"] = &fstest.MapFile{Data: []byte(`{{ b }}`)}

	again, err := keyed.File("page.gohtml")
	if err != nil {
		t.Fatal(err)
	}

	if again != first || !slices.Equal(again.Required, []string{"a"}) {
		t.Fatalf("keyed scan = %+v, want the first scan", again)
	}

	fresh, err := unkeyed.File("page.gohtml")
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(fresh.Required, []string{"b"}) {
		t.Fatalf("unkeyed scan = %+v, want the new content", fresh)
	}

	if _, err := unkeyed.File("missing.gohtml"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("File() of a missing file error = %v", err)
	}
}

// TestScannerReportsFileSystemErrors checks that errors other than a missing
// file are returned and not remembered, and that a bad pattern is an error.
func TestScannerReportsFileSystemErrors(t *testing.T) {
	scanner := NewStore().Scanner(errFS{err: fs.ErrPermission}, "denied")

	exists, err := scanner.Exists("page.gohtml")
	if exists || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("Exists() = %v, %v, want a permission error", exists, err)
	}

	if _, err := scanner.Exists("page.gohtml"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("second Exists() error = %v, want the error asked again", err)
	}

	if scanner.IsFile("page.gohtml") {
		t.Fatal("IsFile() = true on a failing file system")
	}

	missing := NewStore().Scanner(errFS{err: fs.ErrNotExist}, "")
	if exists, err := missing.Exists("page.gohtml"); exists || err != nil {
		t.Fatalf("Exists() of a missing file = %v, %v", exists, err)
	}

	if _, err := NewStore().Scanner(fstest.MapFS{}, "").Glob("[bad"); err == nil {
		t.Fatal("Glob() of a bad pattern: want an error")
	}
}

// TestScannersOfOneKeyShare checks that two scanners a store gives for one
// key share what they learn, and that an unkeyed scanner globs every time.
func TestScannersOfOneKeyShare(t *testing.T) {
	fsys := fstest.MapFS{
		"a.gohtml": &fstest.MapFile{Data: []byte(`a`)},
	}
	store := NewStore()

	first := store.Scanner(fsys, "shared")
	if matches, _ := first.Glob("*.gohtml"); len(matches) != 1 {
		t.Fatalf("first matches = %v", matches)
	}

	fsys["b.gohtml"] = &fstest.MapFile{Data: []byte(`b`)}

	second := store.Scanner(fsys, "shared")
	if matches, _ := second.Glob("*.gohtml"); len(matches) != 1 {
		t.Fatalf("second matches = %v, want those of the first scanner", matches)
	}

	matches, err := store.Scanner(fsys, "").Glob("*.gohtml")
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(matches, []string{"a.gohtml", "b.gohtml"}) {
		t.Fatalf("unkeyed matches = %v", matches)
	}
}
