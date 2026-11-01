package asset

import (
	"io/fs"
	"testing"
)

// TestFSHoldsReadOnlyCopies checks that the files can be read by name, are
// readable by everyone, and do not change with the bytes they came from.
func TestFSHoldsReadOnlyCopies(t *testing.T) {
	data := []byte("console.log(1)")
	fsys := FS(map[string][]byte{"client.js": data, "style.css": []byte("p{}")})

	data[0] = 'X'

	got, err := fs.ReadFile(fsys, "client.js")
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "console.log(1)" {
		t.Fatalf("client.js = %q, want the bytes as they were given", got)
	}

	info, err := fs.Stat(fsys, "style.css")
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o444 {
		t.Fatalf("style.css mode = %v, want 0444", info.Mode().Perm())
	}

	if _, err := fs.Stat(fsys, "missing.js"); err == nil {
		t.Fatal("Stat() found a file that was not given")
	}
}
