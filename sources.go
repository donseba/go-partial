package partial

import (
	"errors"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"
)

// Sources is a file system of template sources held in memory, by path,
// such as the default markup a package ships for a template that
// applications may replace:
//
//	defaults := partial.Sources{
//	    "share.gohtml": `<nav class="share">{{ range .Links }}…{{ end }}</nav>`,
//	}
//	p := partial.NewID("share", "share.gohtml").
//	    SetFileSystem(partial.Overlay(themeFiles, defaults))
//
// Paths use forward slashes without a leading one, as io/fs paths do;
// directories are implied by the paths of their files.
//
// The template cache tells file systems apart by identity: create Sources
// once, such as when the application starts, rather than for every render.
// A Sources must not change while it is used.
type Sources map[string]string

// Open opens the source or the directory name.
func (s Sources) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	if source, ok := s[name]; ok {
		return &sourceFile{
			Reader: strings.NewReader(source),
			info:   sourceInfo{name: path.Base(name), size: int64(len(source))},
		}, nil
	}

	entries, ok := s.dir(name)
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return &listedDir{info: sourceInfo{name: path.Base(name), dir: true}, entries: entries}, nil
}

// ReadFile returns the source name.
func (s Sources) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrInvalid}
	}

	source, ok := s[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}

	return []byte(source), nil
}

// ReadDir lists the directory name.
func (s Sources) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	entries, ok := s.dir(name)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	return entries, nil
}

// dir lists the files and directories directly below dir, and reports
// whether dir is a directory: the root, or the parent of a source.
func (s Sources) dir(dir string) ([]fs.DirEntry, bool) {
	prefix := ""
	if dir != "." {
		prefix = dir + "/"
	}

	found := dir == "."
	seen := make(map[string]bool)

	var entries []fs.DirEntry

	for name, source := range s {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok || rest == "" {
			continue
		}

		found = true

		child, _, isDir := strings.Cut(rest, "/")
		if seen[child] {
			continue
		}

		seen[child] = true

		info := sourceInfo{name: child, dir: isDir}
		if !isDir {
			info.size = int64(len(source))
		}

		entries = append(entries, fs.FileInfoToDirEntry(info))
	}

	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})

	return entries, found
}

type sourceInfo struct {
	name string
	size int64
	dir  bool
}

// Name is the base name of the source or directory.
func (i sourceInfo) Name() string {
	return i.name
}

// Size is the length of the source in bytes; 0 for a directory.
func (i sourceInfo) Size() int64 {
	return i.size
}

// ModTime is the zero time: sources have no modification time.
func (i sourceInfo) ModTime() time.Time {
	return time.Time{}
}

// IsDir reports whether the entry is a directory.
func (i sourceInfo) IsDir() bool {
	return i.dir
}

// Sys is nil: sources have no underlying data source.
func (i sourceInfo) Sys() any {
	return nil
}

func (i sourceInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o555
	}

	return 0o444
}

type sourceFile struct {
	*strings.Reader
	info sourceInfo
}

// Stat describes the source.
func (f *sourceFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

// Close does nothing: the source stays in memory.
func (f *sourceFile) Close() error {
	return nil
}

// listedDir is an open directory with the entries it was given.
type listedDir struct {
	info    fs.FileInfo
	entries []fs.DirEntry
	offset  int
}

// Stat describes the directory.
func (d *listedDir) Stat() (fs.FileInfo, error) {
	return d.info, nil
}

// Close does nothing: the entries stay in memory.
func (d *listedDir) Close() error {
	return nil
}

func (d *listedDir) Read([]byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.info.Name(), Err: errors.New("is a directory")}
}

func (d *listedDir) ReadDir(count int) ([]fs.DirEntry, error) {
	rest := d.entries[d.offset:]
	if count <= 0 {
		d.offset = len(d.entries)
		return slices.Clone(rest), nil
	}

	if len(rest) == 0 {
		return nil, io.EOF
	}

	count = min(count, len(rest))
	d.offset += count

	return slices.Clone(rest[:count]), nil
}

// Overlay returns a file system that looks files up in each layer in turn
// and uses the first that has them; directories list the files of every
// layer. Use it to let applications replace templates a package ships, or
// to add in-memory Sources to a directory of templates:
//
//	partial.Overlay(os.DirFS("theme"), defaults)
//
// Overlays of the same layers share their entries in the template cache,
// so an overlay may be built for every render, as long as its layers are
// not; see Sources.
func Overlay(layers ...fs.FS) fs.FS {
	kept := make([]fs.FS, 0, len(layers))
	for _, layer := range layers {
		if layer != nil {
			kept = append(kept, layer)
		}
	}

	return overlayFS{layers: kept}
}

// Layers returns the layers of fsys when it is an Overlay, in the order it
// looks files up in, and fsys alone otherwise; nil for a nil fsys. Use it
// to add a layer below an overlay without nesting one overlay in another,
// which FirstExistingIn would try as one layer:
//
//	partial.Overlay(append(partial.Layers(theme), defaults)...)
func Layers(fsys fs.FS) []fs.FS {
	return slices.Clone(layersOf(fsys))
}

// layersOf is Layers without the copy: the slice of an Overlay is its own,
// so it must not be changed.
func layersOf(fsys fs.FS) []fs.FS {
	if overlay, ok := fsys.(overlayFS); ok {
		return overlay.layers
	}

	if fsys == nil {
		return nil
	}

	return []fs.FS{fsys}
}

type overlayFS struct {
	layers []fs.FS
}

// Open opens name from the first layer that has it; a directory lists the
// entries of every layer, as ReadDir does.
func (o overlayFS) Open(name string) (fs.File, error) {
	file, err := fromFirstLayer(o, "open", name, fs.FS.Open)
	if err != nil {
		return nil, err
	}

	return o.merged(name, file)
}

// merged returns file, or for a directory one that lists every layer.
func (o overlayFS) merged(name string, file fs.File) (fs.File, error) {
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	if !info.IsDir() {
		return file, nil
	}

	_ = file.Close()

	entries, err := o.ReadDir(name)
	if err != nil {
		return nil, err
	}

	return &listedDir{info: info, entries: entries}, nil
}

// ReadFile reads name from the first layer that has it.
func (o overlayFS) ReadFile(name string) ([]byte, error) {
	return fromFirstLayer(o, "read", name, fs.ReadFile)
}

// Stat describes name in the first layer that has it.
func (o overlayFS) Stat(name string) (fs.FileInfo, error) {
	return fromFirstLayer(o, "stat", name, fs.Stat)
}

// fromFirstLayer asks the layers in turn for name with get, and returns the
// answer of the first layer that has it, or the first error other than a
// missing file. op names the operation in the errors, such as "read".
func fromFirstLayer[T any](o overlayFS, op, name string, get func(fs.FS, string) (T, error)) (T, error) {
	var none T

	if !fs.ValidPath(name) {
		return none, &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}

	for _, layer := range o.layers {
		value, err := get(layer, name)
		if err == nil {
			return value, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return none, err
		}
	}

	return none, &fs.PathError{Op: op, Path: name, Err: fs.ErrNotExist}
}

// ReadDir lists the directory name in every layer that has it; a name in
// more than one layer is listed as the first layer has it.
func (o overlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	var (
		entries []fs.DirEntry
		found   bool
	)

	seen := make(map[string]bool)

	for _, layer := range o.layers {
		list, err := fs.ReadDir(layer, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, err
		}

		found = true

		for _, entry := range list {
			if seen[entry.Name()] {
				continue
			}

			seen[entry.Name()] = true
			entries = append(entries, entry)
		}
	}

	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		return strings.Compare(a.Name(), b.Name())
	})

	return entries, nil
}
