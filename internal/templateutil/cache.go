package templateutil

import (
	"fmt"
	"html/template"
	"sync"
)

type CachedTemplate struct {
	base          *template.Template
	requiredFuncs map[string]struct{}
	pool          sync.Pool
}

type Store struct {
	templates sync.Map
	mutexes   sync.Map
}

func NewStore() *Store {
	return &Store{}
}

func NewCachedTemplate(base *template.Template, requiredFuncs map[string]struct{}) *CachedTemplate {
	return &CachedTemplate{
		base:          base,
		requiredFuncs: requiredFuncs,
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

func (store *Store) Mutex(key string) *sync.Mutex {
	if store == nil {
		return &sync.Mutex{}
	}
	value, _ := store.mutexes.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (cached *CachedTemplate) Template(functions template.FuncMap) (*template.Template, func(), error) {
	if cached == nil {
		return nil, nil, fmt.Errorf("cached template is not configured")
	}
	functions = FilterFuncMap(functions, cached.requiredFuncs)

	if pooled := cached.pool.Get(); pooled != nil {
		t, ok := pooled.(*template.Template)
		if !ok {
			return nil, nil, fmt.Errorf("cached template pool contained %T", pooled)
		}
		if len(functions) > 0 {
			t.Funcs(functions)
		}
		return t, func() { cached.pool.Put(t) }, nil
	}

	t, err := cached.base.Clone()
	if err != nil {
		return nil, nil, fmt.Errorf("error cloning cached template: %w", err)
	}
	if len(functions) > 0 {
		t.Funcs(functions)
	}
	return t, func() { cached.pool.Put(t) }, nil
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
