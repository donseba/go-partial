package partial

import (
	"io/fs"
	"strings"
	"time"
)

type InMemoryFS struct {
	Files map[string]string
}

func (f *InMemoryFS) AddFile(name, content string) {
	if f.Files == nil {
		f.Files = make(map[string]string)
	}
	f.Files[name] = content
}

func (f *InMemoryFS) Open(name string) (fs.File, error) {
	content, ok := f.Files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &InMemoryFile{
		Reader: strings.NewReader(content),
		name:   name,
	}, nil
}

type InMemoryFile struct {
	*strings.Reader
	name string
}

func (f *InMemoryFile) Stat() (fs.FileInfo, error) {
	return &InMemoryFileInfo{name: f.name, size: int64(f.Len())}, nil
}

func (f *InMemoryFile) ReadDir(count int) ([]fs.DirEntry, error) {
	return nil, fs.ErrNotExist
}

func (f *InMemoryFile) Close() error {
	return nil
}

type InMemoryFileInfo struct {
	name string
	size int64
}

func (fi *InMemoryFileInfo) Name() string       { return fi.name }
func (fi *InMemoryFileInfo) Size() int64        { return fi.size }
func (fi *InMemoryFileInfo) Mode() fs.FileMode  { return 0444 }
func (fi *InMemoryFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *InMemoryFileInfo) IsDir() bool        { return false }
func (fi *InMemoryFileInfo) Sys() interface{}   { return nil }
