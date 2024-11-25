package partial

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
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
		"action":             {},
		"actionHeader":       {},
		"child":              {},
		"context":            {},
		"ifRequestedAction":  {},
		"ifRequestedPartial": {},
		"ifRequestedSelect":  {},
		"ifSwapOOB":          {},
		"partialHeader":      {},
		"requestedPartial":   {},
		"requestedAction":    {},
		"requestedSelect":    {},
		"selectHeader":       {},
		"selection":          {},
		"swapOOB":            {},
		"url":                {},
	}
)

type (
	// Partial represents a renderable component with optional children and data.
	Partial struct {
		id                string
		parent            *Partial
		request           *http.Request
		swapOOB           bool
		fs                fs.FS
		logger            Logger
		partialHeader     string
		selectHeader      string
		actionHeader      string
		requestedPartial  string
		requestedAction   string
		requestedSelect   string
		useCache          bool
		templates         []string
		combinedFunctions template.FuncMap
		data              map[string]any
		layoutData        map[string]any
		globalData        map[string]any
		mu                sync.RWMutex
		children          map[string]*Partial
		oobChildren       map[string]struct{}
		selection         *Selection
		templateAction    func(ctx context.Context, p *Partial, data *Data) (*Partial, error)
		action            func(ctx context.Context, p *Partial, data *Data) (*Partial, error)
	}

	Selection struct {
		Partials map[string]*Partial
		Default  string
	}

	// Data represents the data available to the partial.
	Data struct {
		// Ctx is the context of the request
		Ctx context.Context
		// URL is the URL of the request
		URL *url.URL
		// Request contains the http.Request
		Request *http.Request
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
		p.getLogger().Warn("function name is protected and cannot be overwritten", "function", name)
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
			p.getLogger().Warn("function name is protected and cannot be overwritten", "function", k)
			continue
		}

		p.combinedFunctions[k] = v
	}
}

// SetLogger sets the logger for the partial.
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

// WithAction adds callback action to the partial, which can do some logic and return a partial to render.
func (p *Partial) WithAction(action func(ctx context.Context, p *Partial, data *Data) (*Partial, error)) *Partial {
	p.action = action
	return p
}

func (p *Partial) WithTemplateAction(templateAction func(ctx context.Context, p *Partial, data *Data) (*Partial, error)) *Partial {
	p.templateAction = templateAction
	return p
}

// WithSelectMap adds a selection partial to the partial.
func (p *Partial) WithSelectMap(defaultKey string, partialsMap map[string]*Partial) *Partial {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.selection = &Selection{
		Default:  defaultKey,
		Partials: partialsMap,
	}

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

// RenderWithRequest renders the partial with the given http.Request.
func (p *Partial) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	p.request = r
	p.requestedPartial = r.Header.Get(p.getPartialHeader())
	p.requestedAction = r.Header.Get(p.getActionHeader())
	p.requestedSelect = r.Header.Get(p.getSelectHeader())

	return p.renderWithTarget(ctx, r)
}

// WriteWithRequest writes the partial to the http.ResponseWriter.
func (p *Partial) WriteWithRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if p == nil {
		_, err := fmt.Fprintf(w, "partial is not initialized")
		return err
	}

	out, err := p.RenderWithRequest(ctx, r)
	if err != nil {
		p.getLogger().Error("error rendering partial", "error", err)
		return err
	}

	_, err = w.Write([]byte(out))
	if err != nil {
		p.getLogger().Error("error writing partial to response", "error", err)
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

func (p *Partial) mergeFuncMapInternal(funcMap template.FuncMap) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, v := range funcMap {
		p.combinedFunctions[k] = v
	}
}

// getFuncMap returns the combined function map of the partial.
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

func (p *Partial) getFuncs(data *Data) template.FuncMap {
	funcs := p.getFuncMap()

	funcs["child"] = childFunc(p, data)
	funcs["selection"] = selectionFunc(p, data)
	funcs["action"] = actionFunc(p, data)

	funcs["url"] = func() *url.URL {
		return data.URL
	}

	funcs["context"] = func() context.Context {
		return data.Ctx
	}

	funcs["partialHeader"] = func() string {
		return p.getPartialHeader()
	}

	funcs["requestedPartial"] = func() string {
		return p.getRequestedPartial()
	}

	funcs["ifRequestedPartial"] = func(out any, in ...string) any {
		for _, v := range in {
			if v == p.getRequestedPartial() {
				return out
			}
		}
		return nil
	}

	funcs["selectHeader"] = func() string {
		return p.getSelectHeader()
	}

	funcs["requestedSelect"] = func() string {
		if p.getRequestedSelect() == "" {
			return p.selection.Default
		}
		return p.getRequestedSelect()
	}

	funcs["ifRequestedSelect"] = func(out any, in ...string) any {
		for _, v := range in {
			if v == p.getRequestedSelect() {
				return out
			}
		}
		return nil
	}

	funcs["actionHeader"] = func() string {
		return p.getActionHeader()
	}

	funcs["requestedAction"] = func() string {
		return p.GetRequestedAction()
	}

	funcs["ifRequestedAction"] = func(out any, in ...string) any {
		for _, v := range in {
			if v == p.GetRequestedAction() {
				return out
			}
		}
		return nil
	}

	funcs["swapOOB"] = func() bool {
		return p.swapOOB
	}

	funcs["ifSwapOOB"] = func(v string) template.HTML {
		if p.swapOOB {
			return template.HTML("x-swap-oob=\" + v + \"")
		}
		// Return an empty trusted HTML instead of a plain empty string
		return template.HTML("")
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
	return defaultTargetHeader
}

func (p *Partial) getSelectHeader() string {
	if p.selectHeader != "" {
		return p.selectHeader
	}
	if p.parent != nil {
		return p.parent.getSelectHeader()
	}
	return defaultSelectHeader
}

func (p *Partial) getSelectionPartials() map[string]*Partial {
	if p.selection != nil {
		return p.selection.Partials
	}
	return nil
}

func (p *Partial) getActionHeader() string {
	if p.actionHeader != "" {
		return p.actionHeader
	}
	if p.parent != nil {
		return p.parent.getActionHeader()
	}
	return defaultActionHeader
}

func (p *Partial) getRequest() *http.Request {
	if p.request != nil {
		return p.request
	}
	if p.parent != nil {
		return p.parent.getRequest()
	}
	return &http.Request{}
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

func (p *Partial) getLogger() Logger {
	if p == nil {
		return slog.Default().WithGroup("partial")
	}

	if p.logger != nil {
		return p.logger
	}

	if p.parent != nil {
		return p.parent.getLogger()
	}

	// Cache the default logger in p.logger
	p.logger = slog.Default().WithGroup("partial")

	return p.logger
}

func (p *Partial) getRequestedPartial() string {
	if p.requestedPartial != "" {
		return p.requestedPartial
	}
	if p.parent != nil {
		return p.parent.getRequestedPartial()
	}
	return ""
}

func (p *Partial) GetRequestedAction() string {
	if p.requestedAction != "" {
		return p.requestedAction
	}
	if p.parent != nil {
		return p.parent.GetRequestedAction()
	}
	return ""
}

func (p *Partial) getRequestedSelect() string {
	if p.requestedSelect != "" {
		return p.requestedSelect
	}
	if p.parent != nil {
		return p.parent.getRequestedSelect()
	}
	return ""
}

func (p *Partial) renderWithTarget(ctx context.Context, r *http.Request) (template.HTML, error) {
	if p.getRequestedPartial() == "" || p.getRequestedPartial() == p.id {
		out, err := p.renderSelf(ctx, r)
		if err != nil {
			return "", err
		}

		// Render OOB children of parent if necessary
		if p.parent != nil {
			oobOut, oobErr := p.parent.renderOOBChildren(ctx, r, true)
			if oobErr != nil {
				p.getLogger().Error("error rendering OOB children of parent", "error", oobErr, "parent", p.parent.id)
				return "", fmt.Errorf("error rendering OOB children of parent with ID '%s': %w", p.parent.id, oobErr)
			}
			out += oobOut
		}
		return out, nil
	} else {
		c := p.recursiveChildLookup(p.getRequestedPartial(), make(map[string]bool))
		if c == nil {
			p.getLogger().Error("requested partial not found in parent", "id", p.getRequestedPartial(), "parent", p.id)
			return "", fmt.Errorf("requested partial %s not found in parent %s", p.getRequestedPartial(), p.id)
		}
		return c.renderWithTarget(ctx, r)
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
		p.getLogger().Warn("child partial not found", "id", id)
		return "", nil
	}

	// Clone the child partial to avoid modifying the original and prevent data races
	childClone := child.clone()

	// Set the parent of the cloned child to the current partial
	childClone.parent = p

	// If additional data is provided, set it on the cloned child partial
	if data != nil {
		childClone.MergeData(data, true)
	}

	// Render the cloned child partial
	return childClone.renderSelf(ctx, p.getRequest())
}

// renderNamed renders the partial with the given name and templates.
func (p *Partial) renderSelf(ctx context.Context, r *http.Request) (template.HTML, error) {
	if len(p.templates) == 0 {
		p.getLogger().Error("no templates provided for rendering")
		return "", errors.New("no templates provided for rendering")
	}

	var currentURL *url.URL
	if r != nil {
		currentURL = r.URL
	}

	data := &Data{
		URL:     currentURL,
		Request: r,
		Ctx:     ctx,
		Data:    p.data,
		Service: p.getGlobalData(),
		Layout:  p.getLayoutData(),
	}

	if p.action != nil {
		var err error
		p, err = p.action(ctx, p, data)
		if err != nil {
			p.getLogger().Error("error in action function", "error", err)
			return "", fmt.Errorf("error in action function: %w", err)
		}
		//return actionPartial.renderSelf(ctx, r)
	}

	functions := p.getFuncs(data)
	funcMapPtr := reflect.ValueOf(functions).Pointer()

	cacheKey := p.generateCacheKey(p.templates, funcMapPtr)
	tmpl, err := p.getOrParseTemplate(cacheKey, functions)
	if err != nil {
		p.getLogger().Error("error getting or parsing template", "error", err)
		return "", err
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
		p.getLogger().Error("error executing template", "template", p.templates[0], "error", err)
		return "", fmt.Errorf("error executing template '%s': %w", p.templates[0], err)
	}

	return template.HTML(buf.String()), nil
}

func (p *Partial) renderOOBChildren(ctx context.Context, r *http.Request, swapOOB bool) (template.HTML, error) {
	var out template.HTML
	p.mu.RLock()
	defer p.mu.RUnlock()

	for id := range p.oobChildren {
		if child, ok := p.children[id]; ok {
			child.swapOOB = swapOOB
			childData, err := child.renderSelf(ctx, r)
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

func (p *Partial) clone() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Create a new Partial instance
	clone := &Partial{
		id:                p.id,
		parent:            p.parent,
		request:           p.request,
		swapOOB:           p.swapOOB,
		fs:                p.fs,
		logger:            p.logger,
		partialHeader:     p.partialHeader,
		selectHeader:      p.selectHeader,
		actionHeader:      p.actionHeader,
		requestedPartial:  p.requestedPartial,
		useCache:          p.useCache,
		selection:         p.selection,
		templates:         append([]string{}, p.templates...), // Copy the slice
		combinedFunctions: make(template.FuncMap),
		data:              make(map[string]any),
		layoutData:        make(map[string]any),
		globalData:        make(map[string]any),
		children:          make(map[string]*Partial),
		oobChildren:       make(map[string]struct{}),
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

// Generate a hash of the function names to include in the cache key
func (p *Partial) generateCacheKey(templates []string, funcMapPtr uintptr) string {
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
