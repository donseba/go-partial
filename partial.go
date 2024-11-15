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
		id            string
		parent        *Partial
		isOOB         bool
		fs            fs.FS
		partialHeader string
		useCache      bool
		templates     []string
		functions     template.FuncMap
		data          map[string]any
		layoutData    map[string]any
		globalData    map[string]any
		mu            sync.RWMutex
		children      map[string]*Partial
		oobChildren   map[string]struct{}
		partials      map[string]template.HTML
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

// SetFileSystem sets the file system for the partial.
func (p *Partial) SetFileSystem(fs fs.FS) *Partial {
	p.fs = fs
	return p
}

// SetFuncMap sets the template function map for the partial.
func (p *Partial) SetFuncMap(funcMap template.FuncMap) *Partial {
	p.functions = funcMap
	return p
}

// UseCache sets the cache usage flag for the partial.
func (p *Partial) UseCache(useCache bool) *Partial {
	p.useCache = useCache
	return p
}

// SetGlobalData sets the global data for the partial.
func (p *Partial) SetGlobalData(data map[string]any) *Partial {
	p.globalData = data
	return p
}

// SetLayoutData sets the layout data for the partial.
func (p *Partial) SetLayoutData(data map[string]any) *Partial {
	p.layoutData = data
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

// SetParent sets the parent of the partial.
func (p *Partial) SetParent(parent *Partial) *Partial {
	p.parent = parent
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

func (p *Partial) getPartialHeader() string {
	if p.partialHeader != "" {
		return p.partialHeader
	}
	if p.parent != nil {
		return p.parent.getPartialHeader()
	}
	return ""
}

// RenderWithRequest renders the partial with the given http.Request.
func (p *Partial) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	renderTarget := r.Header.Get(p.getPartialHeader())
	return p.renderWithTarget(ctx, r, renderTarget)
}

// WriteWithRequest writes the partial to the http.ResponseWriter.
func (p *Partial) WriteWithRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	out, err := p.RenderWithRequest(ctx, r)
	if err != nil {
		return err
	}

	_, err = w.Write([]byte(out))
	if err != nil {
		return err
	}

	return nil
}

// Render renders the partial without requiring an http.Request.
// It can be used when you don't need access to the request data.
func (p *Partial) Render(ctx context.Context) (template.HTML, error) {
	// Since we don't have an http.Request, we'll pass nil where appropriate.
	return p.render(ctx, nil)
}

func (p *Partial) renderWithTarget(ctx context.Context, r *http.Request, renderTarget string) (template.HTML, error) {
	if renderTarget == "" || renderTarget == p.id {
		out, err := p.render(ctx, r)
		if err != nil {
			return "", err
		}
		// Render OOB children of parent if necessary
		if p.parent != nil {
			oobOut, err := p.parent.renderOOBChildren(ctx, r.URL, true)
			if err != nil {
				return "", err
			}
			out += oobOut
		}
		return out, nil
	} else {
		c := p.recursiveChildLookup(renderTarget, make(map[string]bool))
		if c == nil {
			return "", fmt.Errorf("requested partial %s not found", renderTarget)
		}
		return c.renderWithTarget(ctx, r, renderTarget)
	}
}

// recursiveChildLookup looks up a child recursively.
func (p *Partial) recursiveChildLookup(id string, visited map[string]bool) *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if visited[p.id] {
		return nil
	}
	visited[p.id] = true

	if c, ok := p.children[id]; ok {
		return c
	}

	for _, child := range p.children {
		if c := child.recursiveChildLookup(id, visited); c != nil {
			return c
		}
	}

	return nil
}

// render renders the full page with all children.
func (p *Partial) render(ctx context.Context, r *http.Request) (template.HTML, error) {
	// Render children
	if err := p.renderChildren(ctx, r); err != nil {
		return "", err
	}

	// Render self
	return p.renderSelf(ctx, r.URL)
}

func (p *Partial) renderChildren(ctx context.Context, r *http.Request) error {

	for id, child := range p.children {
		childData, err := child.render(ctx, r)
		if err != nil {
			return fmt.Errorf("error rendering child '%s': %w", id, err)
		}
		p.mu.Lock()
		p.partials[id] = childData
		p.mu.Unlock()
	}
	return nil
}

// renderNamed renders the partial with the given name and templates.
func (p *Partial) renderSelf(ctx context.Context, currentURL *url.URL) (template.HTML, error) {
	if len(p.templates) == 0 {
		return "", errors.New("no templates provided for rendering")
	}

	functions := p.getFuncs()
	functions["_isOOB"] = func() bool {
		return p.isOOB
	}

	cacheKey := generateCacheKey(p.templates, functions)
	tmpl, err := p.getOrParseTemplate(cacheKey, functions)
	if err != nil {
		return "", err
	}

	data := &Data{
		URL:      currentURL,
		Ctx:      ctx,
		Data:     p.data,
		Service:  p.getGlobalData(),
		Layout:   p.getLayoutData(),
		Partials: p.partials,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template '%s': %w", p.templates[0], err)
	}

	return template.HTML(buf.String()), nil
}

func (p *Partial) renderOOBChildren(ctx context.Context, currentURL *url.URL, attachOOB bool) (template.HTML, error) {
	var out template.HTML
	p.mu.RLock()
	defer p.mu.RUnlock()

	for id := range p.oobChildren {
		if child, ok := p.children[id]; ok {
			child.isOOB = attachOOB
			childData, err := child.renderSelf(ctx, currentURL)
			if err != nil {
				return "", fmt.Errorf("error rendering OOB child '%s': %w", id, err)
			}
			out += childData
		}
	}
	return out, nil
}

func (p *Partial) getOrParseTemplate(cacheKey string, functions template.FuncMap) (*template.Template, error) {
	if tmpl, cached := templateCache.Load(cacheKey); cached && p.useCache {
		if t, ok := tmpl.(*template.Template); ok {
			return t, nil
		}
	}

	muInterface, _ := mutexCache.LoadOrStore(cacheKey, &sync.Mutex{})
	mu := muInterface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if tmpl, cached := templateCache.Load(cacheKey); cached && p.useCache {
		if t, ok := tmpl.(*template.Template); ok {
			return t, nil
		}
	}

	t := template.New(path.Base(p.templates[0])).Funcs(functions)
	var tmpl *template.Template
	var err error

	if fsys := p.getFS(); fsys != nil {
		tmpl, err = t.ParseFS(fsys, p.templates...)
	} else {
		tmpl, err = t.ParseFiles(p.templates...)
	}

	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %w", err)
	}

	if p.useCache {
		templateCache.Store(cacheKey, tmpl)
	}

	return tmpl, nil
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
