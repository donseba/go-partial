package templateutil

import (
	"slices"
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
