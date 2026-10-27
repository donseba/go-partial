// Package asset packages the files go-partial's packages embed, such as
// their browser clients and stylesheets.
package asset

import (
	"bytes"
	"io/fs"
	"testing/fstest"
)

// FS returns a file system that holds a copy of each of files, by name,
// readable by everyone.
func FS(files map[string][]byte) fs.FS {
	fsys := make(fstest.MapFS, len(files))
	for name, data := range files {
		fsys[name] = &fstest.MapFile{Data: bytes.Clone(data), Mode: 0o444}
	}

	return fsys
}
