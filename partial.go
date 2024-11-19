package partial

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"sync"
)

var (
	// templateCache is the cache for parsed templates
	templateCache = sync.Map{}
	// mutexCache is a cache of mutexes for each template key
	mutexCache = sync.Map{}
	// protectedFunctionNames is a set of function names that are protected from being overridden
	protectedFunctionNames = map[string]struct{}{
		"child":         {},
		"context":       {},
		"partialHeader": {},
		"requestHeader": {},
		"swapOOB":       {},
		"url":           {},
	}
)

type (
	// Partial represents a renderable component with optional children and data.
	Partial struct {
		id                string
		parent            *Partial
		swapOOB           bool
		fs                fs.FS
		logger            Logger
		partialHeader     string
		requestHeader     string
		useCache          bool
		templates         []string
		combinedFunctions template.FuncMap
		data              map[string]any
		layoutData        map[string]any
		globalData        map[string]any
		mu                sync.RWMutex
		children          map[string]*Partial
		oobChildren       map[string]struct{}
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
	}

	// GlobalData represents the global data available to all partials.
	GlobalData map[string]any
)

// New creates a new root.
func New(templates ...string) *Partial {
	return &Partial{
		id:                "root",
		templates:         templates,
		combinedFunctions: make(template.FuncMap),
		data:              make(map[string]any),
		layoutData:        make(map[string]any),
		globalData:        make(map[string]any),
		children:          make(map[string]*Partial),
		oobChildren:       make(map[string]struct{}),
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

// MergeData merges the data into the partial.
func (p *Partial) MergeData(data map[string]any, override bool) *Partial {
	for k, v := range data {
		if _, ok := p.data[k]; ok && !override {
			continue
		}

		p.data[k] = v
	}
	return p
}

// AddFunc adds a function to the partial.
func (p *Partial) AddFunc(name string, fn interface{}) *Partial {
	if _, ok := protectedFunctionNames[name]; ok {
		if p.logger != nil {
			p.logger.Warn("function name is protected and cannot be overwritten", "function", name)
		}
		return p
	}

	p.mu.Lock()
	p.combinedFunctions[name] = fn
	p.mu.Unlock()

	return p
}

// MergeFuncMap merges the given FuncMap with the existing FuncMap in the Partial.
func (p *Partial) MergeFuncMap(funcMap template.FuncMap) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range funcMap {
		if _, ok := protectedFunctionNames[k]; ok {
			if p.logger != nil {
				p.logger.Warn("function name is protected and cannot be overwritten", "function", k)
			}
			continue
		}

		p.combinedFunctions[k] = v
	}
}

func (p *Partial) mergeFuncMapInternal(funcMap template.FuncMap) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range funcMap {
		p.combinedFunctions[k] = v
	}
}

func (p *Partial) getFuncMap() template.FuncMap {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.parent != nil {
		for k, v := range p.parent.getFuncMap() {
			p.combinedFunctions[k] = v
		}

		return p.combinedFunctions
	}

	return p.combinedFunctions
}

func (p *Partial) SetLogger(logger Logger) *Partial {
	p.logger = logger
	return p
}

// SetFileSystem sets the file system for the partial.
func (p *Partial) SetFileSystem(fs fs.FS) *Partial {
	p.fs = fs
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

func (p *Partial) Clone() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Create a new Partial instance
	clone := &Partial{
		id:                p.id,
		parent:            p.parent,
		swapOOB:           p.swapOOB,
		fs:                p.fs,
		logger:            p.logger,
		partialHeader:     p.partialHeader,
		requestHeader:     p.requestHeader,
		useCache:          p.useCache,
		templates:         append([]string{}, p.templates...), // Copy the slice
		combinedFunctions: make(template.FuncMap),
		data:              make(map[string]any),
		layoutData:        make(map[string]any),
		globalData:        make(map[string]any),
		children:          make(map[string]*Partial),
		oobChildren:       make(map[string]struct{}),
		// Do not copy the mutex (mu)
	}

	// Copy the maps
	for k, v := range p.combinedFunctions {
		clone.combinedFunctions[k] = v
	}

	for k, v := range p.data {
		clone.data[k] = v
	}

	for k, v := range p.layoutData {
		clone.layoutData[k] = v
	}

	for k, v := range p.globalData {
		clone.globalData[k] = v
	}

	// Copy the children map
	for k, v := range p.children {
		clone.children[k] = v
	}

	// Copy the out-of-band children set
	for k, v := range p.oobChildren {
		clone.oobChildren[k] = v
	}

	return clone
}

func (p *Partial) getFuncs(data *Data) template.FuncMap {
	funcs := p.getFuncMap()

	funcs["swapOOB"] = func() bool {
		return p.swapOOB
	}

	funcs["child"] = func(id string, vals ...any) template.HTML {
		if len(vals) > 0 && len(vals)%2 != 0 {
			return template.HTML(fmt.Sprintf("invalid child data for partial '%s'", id))
		}

		d := make(map[string]any)
		for i := 0; i < len(vals); i += 2 {
			key, ok := vals[i].(string)
			if !ok {
				return template.HTML(fmt.Sprintf("invalid child data key for partial '%s'", id))
			}
			d[key] = vals[i+1]
		}

		html, err := p.renderChildPartial(data.Ctx, id, d)
		if err != nil {
			// Handle error: you can log it and return an empty string or an error message
			return template.HTML(fmt.Sprintf("error rendering partial '%s': %v", id, err))
		}

		return html
	}

	funcs["url"] = func() *url.URL {
		return data.URL
	}

	funcs["context"] = func() context.Context {
		return data.Ctx
	}

	funcs["requestHeader"] = func() string {
		return p.getRequestHeader()
	}

	funcs["partialHeader"] = func() string {
		return p.getPartialHeader()
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

func (p *Partial) getRequestHeader() string {
	if p.requestHeader != "" {
		return p.requestHeader
	}
	if p.parent != nil {
		return p.parent.getRequestHeader()
	}
	return ""
}

// RenderWithRequest renders the partial with the given http.Request.
func (p *Partial) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	renderTarget := r.Header.Get(p.getPartialHeader())

	return p.renderWithTarget(ctx, r, renderTarget)
}

// WriteWithRequest writes the partial to the http.ResponseWriter.
func (p *Partial) WriteWithRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if p == nil {
		_, err := fmt.Fprintf(w, "partial is not initialized")
		return err
	}

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
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	// Since we don't have an http.Request, we'll pass nil where appropriate.
	return p.renderSelf(ctx, nil)
}

func (p *Partial) renderWithTarget(ctx context.Context, r *http.Request, renderTarget string) (template.HTML, error) {
	if renderTarget == "" || renderTarget == p.id {
		out, err := p.renderSelf(ctx, r.URL)
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

func (p *Partial) renderChildPartial(ctx context.Context, id string, data map[string]any) (template.HTML, error) {
	p.mu.RLock()
	child, ok := p.children[id]
	p.mu.RUnlock()
	if !ok {
		if p.logger != nil {
			p.logger.Warn("child partial not found", "id", id)
		}
		return "", nil
	}

	// Clone the child partial to avoid modifying the original and prevent data races
	childClone := child.Clone()

	// Set the parent of the cloned child to the current partial
	childClone.parent = p

	// If additional data is provided, set it on the cloned child partial
	if data != nil {
		childClone.MergeData(data, true)
	}

	// Render the cloned child partial
	return childClone.renderSelf(ctx, nil)
}

// renderNamed renders the partial with the given name and templates.
func (p *Partial) renderSelf(ctx context.Context, currentURL *url.URL) (template.HTML, error) {
	if len(p.templates) == 0 {
		return "", errors.New("no templates provided for rendering")
	}

	data := &Data{
		URL:     currentURL,
		Ctx:     ctx,
		Data:    p.data,
		Service: p.getGlobalData(),
		Layout:  p.getLayoutData(),
	}

	functions := p.getFuncs(data)
	funcMapPtr := reflect.ValueOf(functions).Pointer()

	cacheKey := generateCacheKey(p.templates, funcMapPtr)
	tmpl, err := p.getOrParseTemplate(cacheKey, functions)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing template '%s': %w", p.templates[0], err)
	}

	return template.HTML(buf.String()), nil
}

func (p *Partial) renderOOBChildren(ctx context.Context, currentURL *url.URL, swapOOB bool) (template.HTML, error) {
	var out template.HTML
	p.mu.RLock()
	defer p.mu.RUnlock()

	for id := range p.oobChildren {
		if child, ok := p.children[id]; ok {
			child.swapOOB = swapOOB
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
func generateCacheKey(templates []string, funcMapPtr uintptr) string {
	var builder strings.Builder

	// Include all template names
	for _, tmpl := range templates {
		builder.WriteString(tmpl)
		builder.WriteString(";")
	}

	// Include function map pointer
	builder.WriteString(fmt.Sprintf("funcMap:%x", funcMapPtr))

	return builder.String()
}
