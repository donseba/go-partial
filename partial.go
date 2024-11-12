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
	// DefaultPartialHeader is the default header used to determine which partial to render.
	DefaultPartialHeader = "X-Partial"
	// UseTemplateCache is a flag to enable or disable the template cache
	UseTemplateCache = false
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
		oob         bool
		isOOB       bool
		isWrapper   bool
		fs          fs.FS
		templates   []string
		functions   template.FuncMap
		data        map[string]any
		globalData  *GlobalData
		mu          sync.RWMutex
		children    map[string]*Partial
		oobChildren map[string]struct{}
		partials    map[string]template.HTML
		wrapper     *Partial
	}

	// Data represents the data available to the partial.
	Data struct {
		// Ctx is the context of the request
		Ctx context.Context
		// URL is the URL of the request
		URL *url.URL
		// Data contains the data specific to this partial
		Data map[string]any
		// Global contains global data available to all partials
		Global map[string]any
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
		globalData:  &GlobalData{},
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
	p.globalData = &GlobalData{}
	p.children = make(map[string]*Partial)
	p.oobChildren = make(map[string]struct{})
	p.partials = make(map[string]template.HTML)
	p.wrapper = nil

	return p
}

// WithFS sets the file system where the templates are located.
func (p *Partial) WithFS(fsys fs.FS) *Partial {
	p.fs = fsys

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

// SetGlobalData sets the global data for the partial.
func (p *Partial) SetGlobalData(data map[string]any) *Partial {
	*p.globalData = data
	return p
}

// AddGlobalData adds global data to the partial.
func (p *Partial) AddGlobalData(key string, value any) *Partial {
	(*p.globalData)[key] = value
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
	p.children[child.id].fs = p.fs
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
	child.oob = true

	return p
}

// Wrap wraps the component with the given renderer
func (p *Partial) Wrap(renderer *Partial) *Partial {
	p.wrapper = renderer
	p.wrapper.With(p)

	if renderer.fs != nil && p.fs == nil {
		p.fs = renderer.fs
	}

	return p
}

// RenderWithRequest renders the partial based on the provided HTTP request.
// It respects the "X-Partial" header to determine which partial to render.
func (p *Partial) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	var renderTarget = r.Header.Get(DefaultPartialHeader)

	// safeguard against directly calling a parent which is also the wrapper
	for k, v := range p.children {
		if v.wrapper != nil && v.wrapper.id == p.id {
			return "", fmt.Errorf("partial %s is a wrapper for %s, cannot render directly", p.id, k)
		}
	}

	if (renderTarget == "" || renderTarget == p.id) && p.wrapper != nil {
		return p.renderWrapped(ctx, r)
	}

	if renderTarget != "" {
		return p.renderTargetPartial(ctx, r.URL, renderTarget)
	}

	return p.renderFullPage(ctx, r)
}

// renderWrapped renders the partial with the wrapper.
func (p *Partial) renderWrapped(ctx context.Context, r *http.Request) (template.HTML, error) {
	parent := p.wrapper
	parent.isWrapper = true
	p.wrapper = nil

	return parent.RenderWithRequest(ctx, r)
}

// renderTargetPartial renders the partial with the target.
func (p *Partial) renderTargetPartial(ctx context.Context, currentURL *url.URL, target string) (template.HTML, error) {
	c := recursiveChildLookup(p, target, make(map[string]bool))
	if c == nil {
		return "", fmt.Errorf("requested partial %s not found", target)
	}

	c.AppendFuncs(p.functions)
	out, err := c.renderNamed(ctx, currentURL, path.Base(c.templates[0]), c.templates)
	if err != nil {
		return "", err
	}

	// find all the oob children and add them to the output
	if c.parent != nil {
		out += renderChildren(ctx, currentURL, c.parent, true)
	}

	return out, nil
}

// renderFullPage renders the full page with all children.
func (p *Partial) renderFullPage(ctx context.Context, r *http.Request) (template.HTML, error) {
	// gather all children and render them into a map
	for id, child := range p.children {
		childData, err := child.RenderWithRequest(ctx, r)
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

// recursiveChildLookup looks up a child recursively.
func recursiveChildLookup(p *Partial, id string, visited map[string]bool) *Partial {
	if visited[p.id] {
		return nil
	}
	visited[p.id] = true

	if c, ok := p.children[id]; ok {
		if c.fs == nil {
			c.fs = p.fs
		}

		return c
	}

	for _, child := range p.children {
		if c := recursiveChildLookup(child, id, visited); c != nil {
			return c
		}
	}

	return nil
}

// renderChildren renders the children of the partial add sets the isOOB flag if attachOOB is true.
func renderChildren(ctx context.Context, currentURL *url.URL, p *Partial, attachOOB bool) (out template.HTML) {
	for id := range p.oobChildren {
		if child, cok := p.children[id]; cok {
			child.AppendFuncs(p.functions)
			child.isOOB = attachOOB
			if childData, childErr := child.renderNamed(ctx, currentURL, path.Base(child.templates[0]), child.templates); childErr == nil {
				out += childData
			} else {
				out += template.HTML(childErr.Error())
			}
		}
	}

	return out
}

// renderNamed renders the partial with the given name and templates.
func (p *Partial) renderNamed(ctx context.Context, currentURL *url.URL, name string, templates []string) (template.HTML, error) {
	if len(templates) == 0 {
		return "", errors.New("no templates provided for rendering")
	}

	var err error
	functions := make(template.FuncMap)
	for key, value := range DefaultTemplateFuncMap {
		functions[key] = value
	}

	if p.functions != nil {
		for key, value := range p.functions {
			functions[key] = value
		}
	}

	functions["_isOOB"] = func() bool {
		return p.isOOB
	}

	cacheKey := generateCacheKey(templates, functions)
	tmpl, cached := templateCache.Load(cacheKey)
	if !cached || !UseTemplateCache {
		// Obtain or create a mutex for this cache key
		muInterface, _ := mutexCache.LoadOrStore(cacheKey, &sync.Mutex{})
		mu := muInterface.(*sync.Mutex)

		// Lock the mutex to ensure only one goroutine parses the template
		mu.Lock()
		defer mu.Unlock()

		// Double-check if another goroutine has already parsed the template
		tmpl, cached = templateCache.Load(cacheKey)
		if !cached || !UseTemplateCache {
			t := template.New(name).Funcs(functions)
			var tErr error
			if p.fs != nil {
				tmpl, tErr = t.ParseFS(p.fs, templates...)
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
		Global:   *p.globalData,
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
