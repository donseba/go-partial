package templateutil

import (
	"fmt"
	"html/template"
	"sync"
	"sync/atomic"
	texttemplate "text/template"
)

// maxSizeHint bounds the capacity SizeHint gives a render, so that one huge
// output does not make every later render of its entry reserve as much.
const maxSizeHint = 4 << 20

type CachedTemplate struct {
	base          *template.Template
	requiredFuncs map[string]struct{}
	contracts     map[string]RootContract
	pool          sync.Pool
	// sizes are the sizes of the last output of each entry, by entry name,
	// as *atomic.Int64; see SizeHint.
	sizes sync.Map
}

// Store keeps what the template cache of one partial tree learns: parsed
// templates, and of each file system the tree reads, the scans of its
// files, which files exist and what include patterns match. Every tree
// root has a store of its own, so two trees over the same directory, such
// as two applications in one test process, do not see each other's
// answers, such as a file that was missing when the other one looked.
type Store struct {
	templates sync.Map
	mutexes   sync.Map
	// texts are parsed text/template templates, by cache key.
	texts sync.Map
	// scans are maps of FileScans, files maps of the paths known to be
	// files or missing, and matches maps of the files an include pattern
	// matches, by file system key, each by file name or pattern; see
	// Scanner.
	scans   sync.Map
	files   sync.Map
	matches sync.Map
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{}
}

// NewCachedTemplate keeps a parsed template with the functions its
// templates call and their typed root declarations.
func NewCachedTemplate(
	base *template.Template,
	requiredFuncs map[string]struct{},
	contracts map[string]RootContract,
) *CachedTemplate {
	return &CachedTemplate{
		base:          base,
		requiredFuncs: requiredFuncs,
		contracts:     contracts,
	}
}

func (store *Store) Load(key string) (*CachedTemplate, bool) {
	if store == nil {
		return nil, false
	}
	value, ok := store.templates.Load(key)
	if !ok {
		return nil, false
	}
	entry, ok := value.(*CachedTemplate)
	return entry, ok
}

func (store *Store) Store(key string, entry *CachedTemplate) {
	if store == nil || entry == nil {
		return
	}
	store.templates.Store(key, entry)
}

// Text returns the text template stored under key.
func (store *Store) Text(key string) (*texttemplate.Template, bool) {
	if store == nil {
		return nil, false
	}

	value, ok := store.texts.Load(key)
	if !ok {
		return nil, false
	}

	tmpl, ok := value.(*texttemplate.Template)
	return tmpl, ok
}

// StoreText keeps a parsed text template under key; it is cloned for each
// render.
func (store *Store) StoreText(key string, tmpl *texttemplate.Template) {
	if store == nil || tmpl == nil {
		return
	}

	store.texts.Store(key, tmpl)
}

func (store *Store) Mutex(key string) *sync.Mutex {
	if store == nil {
		return &sync.Mutex{}
	}
	value, _ := store.mutexes.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// RequiredFuncs returns the names of the functions the template calls, or
// nil when they are not known. The map is shared; do not change it.
func (cached *CachedTemplate) RequiredFuncs() map[string]struct{} {
	if cached == nil {
		return nil
	}

	return cached.requiredFuncs
}

// RootContracts returns the typed root declarations of the template's
// files by root name. The map is shared; do not change it.
func (cached *CachedTemplate) RootContracts() map[string]RootContract {
	if cached == nil {
		return nil
	}

	return cached.contracts
}

// Template returns a copy of the template with functions, those of the
// functions the template calls. Give it back with Release when it has
// executed.
func (cached *CachedTemplate) Template(functions template.FuncMap) (*template.Template, error) {
	if cached == nil {
		return nil, fmt.Errorf("cached template is not configured")
	}
	return cached.template(FilterFuncMap(functions, cached.requiredFuncs))
}

// TemplateResolving is Template with the functions the template calls looked
// up by resolve, rather than filtered from a map of every function.
func (cached *CachedTemplate) TemplateResolving(resolve func(name string) (any, bool)) (*template.Template, error) {
	if cached == nil {
		return nil, fmt.Errorf("cached template is not configured")
	}
	functions := make(template.FuncMap, len(cached.requiredFuncs))
	for name := range cached.requiredFuncs {
		if fn, ok := resolve(name); ok {
			functions[name] = fn
		}
	}
	return cached.template(functions)
}

// SizeHint returns the capacity to start the output of a render of entry
// ("" for the template itself) with: the size of its last output and an
// eighth more, for output that grows a little, at most 4 MB; 0 before the
// first render.
func (cached *CachedTemplate) SizeHint(entry string) int {
	if cached == nil {
		return 0
	}

	value, ok := cached.sizes.Load(entry)
	if !ok {
		return 0
	}

	size := value.(*atomic.Int64).Load()

	return int(min(size+size/8, maxSizeHint))
}

// NoteSize remembers the size of the output of a render of entry, for
// SizeHint.
func (cached *CachedTemplate) NoteSize(entry string, size int) {
	if cached == nil {
		return
	}

	value, ok := cached.sizes.Load(entry)
	if !ok {
		value, _ = cached.sizes.LoadOrStore(entry, new(atomic.Int64))
	}

	value.(*atomic.Int64).Store(int64(size))
}

// Release takes back a copy of the template that Template or
// TemplateResolving returned, for a later render.
func (cached *CachedTemplate) Release(t *template.Template) {
	if cached == nil || t == nil {
		return
	}

	cached.pool.Put(t)
}

func (cached *CachedTemplate) template(functions template.FuncMap) (*template.Template, error) {
	if pooled := cached.pool.Get(); pooled != nil {
		t, ok := pooled.(*template.Template)
		if !ok {
			return nil, fmt.Errorf("cached template pool contained %T", pooled)
		}
		if len(functions) > 0 {
			t.Funcs(functions)
		}
		return t, nil
	}

	t, err := cached.base.Clone()
	if err != nil {
		return nil, fmt.Errorf("error cloning cached template: %w", err)
	}
	if len(functions) > 0 {
		t.Funcs(functions)
	}
	return t, nil
}

func FilterFuncMap(funcs template.FuncMap, required map[string]struct{}) template.FuncMap {
	if required == nil {
		return funcs
	}

	filtered := make(template.FuncMap, min(len(funcs), len(required)))
	for name := range required {
		if fn, ok := funcs[name]; ok {
			filtered[name] = fn
		}
	}
	return filtered
}
