package partial

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
)

var (
	// templateCache is the cache for parsed templates
	templateCache = sync.Map{}
	// mutexCache is a cache of mutexes for each template key
	mutexCache = sync.Map{}
)

type (
	// Partial represents a renderable component with optional children and data.
	Partial struct {
		id          string
		parent      *Partial
		isOOB       bool
		fs          fs.FS
		useCache    bool
		templates   []string
		functions   template.FuncMap
		data        map[string]any
		layoutData  map[string]any
		globalData  map[string]any
		mu          sync.RWMutex
		children    map[string]*Partial
		oobChildren map[string]struct{}
		partials    map[string]template.HTML
	}

	// Data represents the data available to the partial.
	Data struct {
		// Ctx is the context of the request
		Ctx context.Context
		// URL is the URL of the request
		URL *url.URL
		// Data contains the data specific to this partial
		Data map[string]any
		// Service contains global data available to all partials
		Service map[string]any
		// LayoutData contains data specific to the service
		Layout map[string]any
		// Partials contains the rendered HTML of child partials
		Partials map[string]template.HTML
	}

	// GlobalData represents the global data available to all partials.
	GlobalData map[string]any
)

// New creates a new root.
func New(templates ...string) *Partial {
	return &Partial{
		id:          "root",
		templates:   templates,
		functions:   make(template.FuncMap),
		data:        make(map[string]any),
		layoutData:  make(map[string]any),
		globalData:  make(map[string]any),
		children:    make(map[string]*Partial),
		oobChildren: make(map[string]struct{}),
		partials:    make(map[string]template.HTML),
	}
}

// NewID creates a new instance with the provided ID.
func NewID(id string, templates ...string) *Partial {
	return New(templates...).ID(id)
}

// ID sets the ID of the partial.
func (p *Partial) ID(id string) *Partial {
	p.id = id
	return p
}

// Templates sets the templates for the partial.
func (p *Partial) Templates(templates ...string) *Partial {
	p.templates = templates
	return p
}

// Reset resets the partial to its initial state.
func (p *Partial) Reset() *Partial {
	p.data = make(map[string]any)
	p.layoutData = make(map[string]any)
	p.globalData = make(map[string]any)
	p.children = make(map[string]*Partial)
	p.oobChildren = make(map[string]struct{})
	p.partials = make(map[string]template.HTML)

	return p
}

// SetData sets the data for the partial.
func (p *Partial) SetData(data map[string]any) *Partial {
	p.data = data
	return p
}

// AddData adds data to the partial.
func (p *Partial) AddData(key string, value any) *Partial {
	p.data[key] = value
	return p
}

// SetFuncs sets the functions for the partial.
func (p *Partial) SetFuncs(funcs template.FuncMap) *Partial {
	p.functions = funcs
	return p
}

// AddFunc adds a function to the partial.
func (p *Partial) AddFunc(name string, fn interface{}) *Partial {
	p.functions[name] = fn
	return p
}

// AppendFuncs appends functions to the partial if they do not exist.
func (p *Partial) AppendFuncs(funcs template.FuncMap) *Partial {
	for k, v := range funcs {
		if _, ok := p.functions[k]; !ok {
			p.functions[k] = v
		}
	}

	return p
}

// AddTemplate adds a template to the partial.
func (p *Partial) AddTemplate(template string) *Partial {
	p.templates = append(p.templates, template)
	return p
}

// With adds a child partial to the partial.
func (p *Partial) With(child *Partial) *Partial {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.children[child.id] = child
	p.children[child.id].globalData = p.globalData
	p.children[child.id].parent = p

	return p
}

// WithOOB adds an out-of-band child partial to the partial.
func (p *Partial) WithOOB(child *Partial) *Partial {
	p.With(child)
	p.mu.Lock()
	p.oobChildren[child.id] = struct{}{}
	p.mu.Unlock()

	return p
}

func (p *Partial) getFS() fs.FS {
	if p.fs != nil {
		return p.fs
	}
	if p.parent != nil {
		return p.parent.getFS()
	}
	return nil
}

func (p *Partial) getFuncs() template.FuncMap {
	funcs := make(template.FuncMap)
	if p.parent != nil {
		parentFuncs := p.parent.getFuncs()
		for k, v := range parentFuncs {
			funcs[k] = v
		}
	}
	for k, v := range p.functions {
		funcs[k] = v
	}
	return funcs
}

func (p *Partial) getGlobalData() map[string]any {
	if p.parent != nil {
		globalData := p.parent.getGlobalData()
		for k, v := range p.globalData {
			globalData[k] = v
		}
		return globalData
	}
	return p.globalData
}

func (p *Partial) getLayoutData() map[string]any {
	if p.parent != nil {
		layoutData := p.parent.getLayoutData()
		for k, v := range p.layoutData {
			layoutData[k] = v
		}
		return layoutData
	}
	return p.layoutData
}

// render renders the full page with all children.
func (p *Partial) render(ctx context.Context, r *http.Request) (template.HTML, error) {
	// gather all children and render them into a map
	for id, child := range p.children {
		childData, err := child.render(ctx, r)
		p.mu.Lock()
		if err == nil {
			p.partials[id] = childData
		} else {
			p.partials[id] = template.HTML(err.Error())
		}
		p.mu.Unlock()
	}

	out, err := p.renderNamed(ctx, r.URL, path.Base(p.templates[0]), p.templates)
	if err != nil {
		return template.HTML(err.Error()), err
	}

	return out, err
}

// renderNamed renders the partial with the given name and templates.
func (p *Partial) renderNamed(ctx context.Context, currentURL *url.URL, name string, templates []string) (template.HTML, error) {
	if len(templates) == 0 {
		return "", errors.New("no templates provided for rendering")
	}

	var err error
	functions := p.getFuncs()
	functions["_isOOB"] = func() bool {
		return p.isOOB
	}

	cacheKey := generateCacheKey(templates, functions)
	tmpl, cached := templateCache.Load(cacheKey)
	if !cached || !p.useCache {
		// Obtain or create a mutex for this cache key
		muInterface, _ := mutexCache.LoadOrStore(cacheKey, &sync.Mutex{})
		mu := muInterface.(*sync.Mutex)

		// Lock the mutex to ensure only one goroutine parses the template
		mu.Lock()
		defer mu.Unlock()

		// Double-check if another goroutine has already parsed the template
		tmpl, cached = templateCache.Load(cacheKey)
		if !cached || !p.useCache {
			t := template.New(name).Funcs(functions)
			var tErr error

			if fsys := p.getFS(); fsys != nil {
				tmpl, tErr = t.ParseFS(fsys, templates...)
			} else {
				tmpl, tErr = t.ParseFiles(templates...)
			}

			if tErr != nil {
				return "", fmt.Errorf("error executing template '%s': %w", name, tErr)
			}
			templateCache.Store(cacheKey, tmpl)
		}
	}

	data := &Data{
		URL:      currentURL,
		Ctx:      ctx,
		Data:     p.data,
		Service:  p.getGlobalData(),
		Layout:   p.getLayoutData(),
		Partials: p.partials,
	}

	if t, ok := tmpl.(*template.Template); ok {
		var buf bytes.Buffer
		err = t.Execute(&buf, data)
		if err != nil {
			return "", fmt.Errorf("error parsing templates %v: %w", templates, err)
		}

		return template.HTML(buf.String()), nil // Return rendered content
	}

	return "", errors.New("template is not a *template.Template")
}

// Generate a hash of the function names to include in the cache key
func generateCacheKey(templates []string, funcMap template.FuncMap) string {
	var builder strings.Builder

	// Include all template names
	for _, tmpl := range templates {
		builder.WriteString(tmpl)
		builder.WriteString(";")
	}
	builder.WriteString(":")

	funcNames := make([]string, 0, len(funcMap))
	for name := range funcMap {
		funcNames = append(funcNames, name)
	}
	sort.Strings(funcNames)

	for _, name := range funcNames {
		builder.WriteString(name)
		builder.WriteString(",")
	}

	hash := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(hash[:])
}
