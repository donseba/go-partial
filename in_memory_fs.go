package partial

import (
	"io/fs"
	"strings"
	"time"
)

type inMemoryFS struct {
	Files map[string]string
}

func (f *inMemoryFS) AddFile(name, content string) {
	if f.Files == nil {
		f.Files = make(map[string]string)
	}
	f.Files[name] = content
}

func (f *inMemoryFS) Open(name string) (fs.File, error) {
	content, ok := f.Files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &inMemoryFile{
		Reader: strings.NewReader(content),
		name:   name,
	}, nil
}

type inMemoryFile struct {
	*strings.Reader
	name string
}

func (f *inMemoryFile) Stat() (fs.FileInfo, error) {
	return &inMemoryFileInfo{name: f.name, size: int64(f.Len())}, nil
}

func (f *inMemoryFile) ReadDir(count int) ([]fs.DirEntry, error) {
	return nil, fs.ErrNotExist
}

func (f *inMemoryFile) Close() error {
	return nil
}

type inMemoryFileInfo struct {
	name string
	size int64
}

func (fi *inMemoryFileInfo) Name() string       { return fi.name }
func (fi *inMemoryFileInfo) Size() int64        { return fi.size }
func (fi *inMemoryFileInfo) Mode() fs.FileMode  { return 0444 }
func (fi *inMemoryFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *inMemoryFileInfo) IsDir() bool        { return false }
func (fi *inMemoryFileInfo) Sys() interface{}   { return nil }
