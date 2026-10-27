package partial

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/internal/templateutil"
)

var (
	// coreFunctionNames are the helpers go-partial injects per render and needs
	// for its own tree, request, connector, and runtime behavior. Optional
	// helper providers must not overwrite these names.
	coreFunctionNames = templateutil.Names(placeholderRequestFuncMap())
)

type (
	// Partial stores reusable template, data, and child-tree configuration.
	Partial struct {
		id        string
		parent    *Partial
		contentID string
		// entry is the template executed, when not the first file's; see
		// SetEntry.
		entry     string
		fs        fs.FS
		connector connector.Connector
		templates []string
		// includes are the patterns of the templates every template of the
		// partial and its descendants can call; see SetIncludes.
		includes []string
		// childIncludes are those of its descendants only, nil without;
		// see SetChildIncludes. The slice is not changed once set.
		childIncludes *[]string
		// ownTemplate holds the one template of a partial {{ partial }}
		// renders, so its templates need no allocation of their own.
		ownTemplate [1]string
		// funcs are the functions registered with SetFunc, nil without;
		// clones share them. See funcTable.
		funcs    *funcTable
		basePath string
		// dot is the value templates receive as "." when hasDot; see SetDot.
		dot any
		// dotFunc computes the dot when the partial renders; see SetDotSource.
		dotFunc   *lazyDot
		contracts []contractInformation
		// extension is the partial's one extension, until it has more:
		// then extensions holds them all. See SetExtension.
		extension       extensionEntry
		extensions      map[any]any
		responseHeaders map[string]string
		responseStatus  int
		response        connector.Response
		targetFallback  *TargetFallback
		events          EventSink
		stages          []RenderStage
		templateCache   *templateutil.Store
		mu              sync.RWMutex
		children        map[string]*Partial
		// oobChildren are the IDs of the out-of-band children, in the
		// order WithOOB registered them, which is the order they render.
		oobChildren []string
		// oobElements are the IDs of the elements the partial sends when it
		// goes out of band; see SetOOBElements.
		oobElements []string
		// oobElementsFunc chooses them per request; see
		// SetOOBElementsFunc.
		oobElementsFunc func(*http.Request) OOBElements

		// The flags are kept together, so a partial takes less memory.
		renderOOB     bool
		alwaysSwapOOB bool
		fsSet         bool
		// cache is the template cache setting of the partial; unset, it is
		// that of its nearest ancestor that sets it. See UseTemplateCache.
		cache  cacheSetting
		hasDot bool
		// lazy is set when the partial has a value computed when it
		// renders: a dotFunc or a model of ModelFuncOf.
		lazy bool
		// failurePolicy is the FailurePolicy plus one, 0 when unset; see
		// SetFailurePolicy.
		failurePolicy uint8
	}

	extensionEntry struct {
		key   any
		value any
		set   bool
	}

	// RenderContext contains request-scoped values exposed by the ctx template helper.
	RenderContext struct {
		Context  context.Context
		Request  *http.Request
		URL      *url.URL
		BasePath string
		Partial  *Partial
		Runtime  *Runtime
		Kind     RenderKind
		Name     string
		Data     any
		Values   RenderValues
		Error    error
		Response *RenderResponse
		Funcs    template.FuncMap
		Events   EventSink

		// funcResolvers look up functions by name; see SetFuncResolver.
		funcResolvers []func(name string) (any, bool)
		// stages are those of the render, asked for the functions templates
		// call when stageResolvers; see TemplateFuncResolver.
		stages         []RenderStage
		stageResolvers bool
		// runtime and response are those newRenderContext gives, made with
		// the render context.
		runtime  Runtime
		response RenderResponse
		// omitOOB leaves the ancestors' out-of-band regions out of the
		// response to a resolved target; see OmitOOB.
		omitOOB bool
		// failureResponse is set in the error render of a failure
		// response; see FailureResponse.
		failureResponse bool
		// failureStatus is the status of WriteError; see FailureStatus.
		failureStatus int
	}

	contractKind string

	// contractInformation binds a Go value to a typed go-doc root declaration.
	//
	// Annotation is the declaration family, such as "model", "interaction", or
	// "component". Name is optional for type-matched values and required when
	// more than one declaration has the same Go type.
	contractInformation struct {
		Kind       contractKind
		Annotation string
		Name       string
		// Value is the value, or the *lazyDot that computes it when a
		// template calls the root; see ModelFuncOf.
		Value any
	}

	// NamedContract lets values choose their go-doc contract name.
	NamedContract interface {
		ContractName() string
	}
)

const (
	contractRoot contractKind = "root"
)

// cacheSetting is the template cache setting of a partial; see
// UseTemplateCache.
type cacheSetting uint8

const (
	// cacheInherit uses the setting of the nearest ancestor that sets one.
	cacheInherit cacheSetting = iota
	cacheOn
	cacheOff
)

// New creates a root partial with the default ID "root".
func New(templates ...string) *Partial {
	// Children, extensions and the template store are made when a partial
	// needs them: partials built for one render, such as regions, need none.
	return &Partial{
		id:        "root",
		templates: templates,
		fs:        os.DirFS("./"),
	}
}

// NewID creates a partial with the provided ID.
func NewID(id string, templates ...string) *Partial {
	return New(templates...).ID(id)
}

// ID sets the stable ID used for target lookup, child registration, and diagnostics.
func (p *Partial) ID(id string) *Partial {
	p.id = id
	return p
}

// PartialID returns the stable render ID for this partial.
func (p *Partial) PartialID() string {
	if p == nil {
		return ""
	}
	return p.id
}

// ParentID returns the ID of the parent partial, if one is attached.
func (p *Partial) ParentID() string {
	if p == nil || p.parent == nil {
		return ""
	}
	return p.parent.PartialID()
}

// TemplatePaths returns the template paths configured for this partial.
func (p *Partial) TemplatePaths() []string {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return slices.Clone(p.templates)
}

// SetTemplates replaces the template paths while preserving the partial's
// configured filesystem, functions, stages, connector, and cache. It is useful
// when cloning a configured partial as a request-scoped blueprint.
func (p *Partial) SetTemplates(templates ...string) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.templates = slices.Clone(templates)
	return p
}

// SetEntry makes the partial render the template defined as name, such as
// {{ define "html" }} in one of its files, instead of its first file. Use it
// for a file that defines several parts, such as an e-mail with its subject,
// text and HTML. The @dot declaration of the first file still types the
// dot. An empty name renders the first file again.
func (p *Partial) SetEntry(name string) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.entry = strings.TrimSpace(name)

	return p
}

// SetParent renders the partial under parent without making it one of
// parent's children: the partial inherits what parent configures, such as
// its functions, render stages, connector, file system, template cache,
// failure policy and values for @model declarations, and parent is not
// changed, so one configured partial can be the parent of many partials
// rendered at the same time. Partial requests cannot target the partial and
// parent's templates do not include it; use With for that. A nil parent
// detaches the partial; a parent that would make the partial its own
// ancestor is ignored.
//
//	card := partial.NewID("card", "card.gohtml").SetDot(data).SetParent(root)
//	html, err := partial.RenderSelf(ctx, r, card)
func (p *Partial) SetParent(parent *Partial) *Partial {
	if p == nil {
		return nil
	}

	for ancestor := parent; ancestor != nil; ancestor = ancestor.parentPartial() {
		if ancestor == p {
			return p
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.parent = parent

	return p
}

// Parent returns the partial p renders under: the partial it was added to
// with With, SetContent or WithOOB, or the one set with SetParent; nil for
// a root.
func (p *Partial) Parent() *Partial {
	if p == nil {
		return nil
	}

	return p.parentPartial()
}

func (p *Partial) parentPartial() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.parent
}

// IsOOB reports whether the partial is currently being rendered out-of-band.
func (p *Partial) IsOOB() bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.renderOOB
}

// Clone returns an independent copy of the partial configuration and child tree.
func (p *Partial) Clone() *Partial {
	if p == nil {
		return nil
	}

	// The clone shares the template store of the tree.
	p.getTemplateStore()

	return p.clone()
}

// SetExtension stores extension-owned data on this partial.
func (p *Partial) SetExtension(key any, value any) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	// Most partials have one extension at most: it is kept without a map.
	if p.extensions == nil && (!p.extension.set || p.extension.key == key) {
		p.extension = extensionEntry{key: key, value: value, set: true}
		return p
	}

	if p.extensions == nil {
		p.extensions = map[any]any{p.extension.key: p.extension.value}
		p.extension = extensionEntry{}
	}
	p.extensions[key] = value
	return p
}

// Extension returns extension-owned data from this partial or its parents.
func (p *Partial) Extension(key any) (any, bool) {
	if p == nil {
		return nil, false
	}
	p.mu.RLock()
	value, ok := p.extensions[key]
	if !ok && p.extension.set && p.extension.key == key {
		value = p.extension.value
		ok = true
	}

	p.mu.RUnlock()
	if ok {
		return value, true
	}
	if p.parent != nil {
		return p.parent.Extension(key)
	}
	return nil, false
}

// Use appends stages to this partial's render chain.
//
// The functions a stage gives templates (see TemplateFuncNamer) need no
// stand-ins registered with SetFunc: templates are parsed with stand-ins
// for them, which the stage replaces when they execute. A function
// registered on the partial or its ancestors under such a name is never
// called: the partial's event sink receives an EventFuncProtected warning.
func (p *Partial) Use(stages ...RenderStage) *Partial {
	if p == nil {
		return nil
	}
	p.warnShadowedFuncs(stages)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.stages = append(p.stages, stages...)

	return p
}

// SetBasePath sets the base path for the partial.
func (p *Partial) SetBasePath(basePath string) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.basePath = basePath

	return p
}

// SetDot sets the root value passed to html/template Execute.
// Templates receive this value as "." and can still use core render helpers such
// as ctx, request, url, basePath, runtime, partial, and content. Extension
// helpers such as debug, locale, and csrf remain available when their FuncMaps
// and stages are used.
func (p *Partial) SetDot(value any) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dot = value
	p.hasDot = true
	p.dotFunc = nil
	return p
}

// ClearDot removes the explicit root value.
func (p *Partial) ClearDot() *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dot = nil
	p.hasDot = false
	p.dotFunc = nil
	return p
}

// SetContract registers typed values for go-doc root declarations.
// Values are matched by type unless they implement NamedContract. A
// ModelFunc (see ModelFuncOf) is computed only when a template calls it,
// and matched by the type it computes.
func (p *Partial) SetContract(annotation string, values ...any) *Partial {
	if p == nil {
		return nil
	}
	annotation = strings.TrimSpace(annotation)
	if annotation == "" {
		return p
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, value := range values {
		// A nil value matches no declaration.
		if value == nil {
			continue
		}

		if model, ok := value.(ModelFunc); ok {
			if model.IsZero() {
				continue
			}

			p.contracts = append(p.contracts, contractInformation{
				Kind:       contractRoot,
				Annotation: annotation,
				Value:      model.lazy,
			})
			p.lazy = true

			continue
		}

		name := ""
		if named, ok := value.(NamedContract); ok {
			name = strings.TrimSpace(named.ContractName())
			if name == "" {
				continue
			}
		}
		p.contracts = append(p.contracts, contractInformation{
			Kind:       contractRoot,
			Annotation: annotation,
			Name:       name,
			Value:      value,
		})
	}
	return p
}

// SetModel registers values for typed model declarations. Nil values are
// ignored, so optional models can be passed as they are. A ModelFunc (see
// ModelFuncOf) is computed only when a template calls the model.
func (p *Partial) SetModel(values ...any) *Partial {
	return p.SetContract("model", values...)
}

// SetResponseHeaders configures plain HTTP headers written by Write.
func (p *Partial) SetResponseHeaders(headers map[string]string) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.responseHeaders = maps.Clone(headers)
	return p
}

// getResponseHeaders returns the response headers of the partial or its
// nearest parent with them. The map is not changed once set, see
// SetResponseHeaders; callers only read it.
func (p *Partial) getResponseHeaders() map[string]string {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	headers := p.responseHeaders
	parent := p.parent
	p.mu.RUnlock()

	if headers != nil {
		return headers
	}

	if parent != nil {
		return parent.getResponseHeaders()
	}
	return nil
}

// SetStatus configures the HTTP status written by Write. A zero status clears
// the local value and falls back to the parent partial, then to net/http's
// default status.
func (p *Partial) SetStatus(status int) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responseStatus = status
	return p
}

func (p *Partial) getStatus() int {
	if p == nil {
		return 0
	}
	p.mu.RLock()
	status := p.responseStatus
	parent := p.parent
	p.mu.RUnlock()
	if status > 0 {
		return status
	}
	if parent != nil {
		return parent.getStatus()
	}
	return 0
}

// Response returns a builder for connector-specific response instructions.
func (p *Partial) Response() *connector.ResponseBuilder {
	if p == nil {
		return connector.NewResponseBuilder(nil)
	}
	return connector.NewResponseBuilder(&p.response)
}

// SetResponse configures connector-specific response instructions.
func (p *Partial) SetResponse(response connector.Response) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.response = response
	return p
}

// SetEvents configures the diagnostic event sink inherited by this partial tree.
func (p *Partial) SetEvents(events EventSink) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.events = events
	return p
}

// GetBasePath returns the configured base path, falling back to parents.
func (p *Partial) GetBasePath() string {
	if p == nil {
		return ""
	}

	p.mu.RLock()
	basePath := p.basePath
	parent := p.parent
	p.mu.RUnlock()

	if basePath != "" {
		return basePath
	}

	if parent != nil {
		return parent.GetBasePath()
	}

	return ""
}

// SetConnector configures the request connector used by this partial.
func (p *Partial) SetConnector(c connector.Connector) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.connector = c
	return p
}

// Connector returns the connector the partial uses: its own, its nearest
// parent's, or connector.NewPartial(nil).
func (p *Partial) Connector() connector.Connector {
	if p == nil {
		return connector.NewPartial(nil)
	}

	return p.getConnectorOrDefault()
}

// SetAlwaysSwapOOB makes this out-of-band partial render on every partial
// request. A region registered with WithOOB goes out of band with partial
// requests for the partials below the one it is registered on; one that
// always swaps also goes with requests for that partial itself, such as a
// notice a page registers for a toast container of the layout. It then goes
// after the partial's output, unless the output places it already, such as
// a layout does with {{ slot }} of exp/slots.
func (p *Partial) SetAlwaysSwapOOB(alwaysSwapOOB bool) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.alwaysSwapOOB = alwaysSwapOOB
	return p
}

// SetFunc registers template functions in the Partial scope. Functions
// named like a go-partial helper are ignored, and those named like a
// function a render stage of the partial or its ancestors gives templates
// are never called (see ReservedFuncNames): for each, the partial's event
// sink receives an EventFuncProtected warning.
func (p *Partial) SetFunc(funcMaps ...template.FuncMap) *Partial {
	if p == nil {
		return nil
	}
	p.warnReservedFuncs(funcMaps)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.funcs = p.funcs.with(funcMaps...)

	return p
}

// SetFileSystem sets the file system for the partial.
func (p *Partial) SetFileSystem(fs fs.FS) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.fs = fs
	p.fsSet = true
	return p
}

// UseTemplateCache turns the template cache on or off for the partial and
// the partials rendered under it: its children, its content, the partials
// {{ partial }} renders and those that name it with SetParent, unless they
// set it themselves. A partial that sets nothing uses the setting of its
// nearest ancestor that does; without one, the cache is off. With the
// cache, each template is read and parsed once; without, on every render,
// so edits to template files show. Set it on the root once, for example
// root.UseTemplateCache(!development).
//
// What the cache learns of a file system, its parses, its scans, which
// files exist (see FirstExisting) and what include patterns match (see
// Glob), is kept by the root of the partial tree: its clones, the partials
// rendered under it and those that name it with SetParent share it. Another
// root, also one over the same directory or the same map file system, such
// as a second application in one test process, learns it again: a file
// added after the first root rendered is seen by a root made later, but not
// by the first one while its cache is on. Give the partials built per
// request a configured parent with SetParent, rather than a root of their
// own, so they share its cache.
func (p *Partial) UseTemplateCache(useCache bool) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cache = cacheOff
	if useCache {
		p.cache = cacheOn
	}

	return p
}

func (p *Partial) usesCache() bool {
	for current := p; current != nil; {
		current.mu.RLock()
		setting := current.cache
		parent := current.parent
		current.mu.RUnlock()

		switch setting {
		case cacheOn:
			return true
		case cacheOff:
			return false
		}

		current = parent
	}

	return false
}

// With registers a child partial on the partial tree.
//
// Registered children are addressable by ID for partial requests. During a
// full render, go-partial also includes child templates that are referenced by
// native Go template calls, such as {{ template "row.gohtml" . }}.
func (p *Partial) With(child *Partial) *Partial {
	if p == nil || child == nil {
		return p
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.children == nil {
		p.children = make(map[string]*Partial)
	}

	p.children[child.id] = child
	p.children[child.id].parent = p

	return p
}

// SetContent registers the primary content child rendered by the content helper.
func (p *Partial) SetContent(child *Partial) *Partial {
	if p == nil || child == nil {
		return p
	}
	p.With(child)
	p.mu.Lock()
	p.contentID = child.id
	p.mu.Unlock()
	return p
}

// WithTemplate creates a child partial from a template path and registers it
// on the partial tree. The child ID is inferred from the file name without its
// extension: "templates/sidebar.gohtml" becomes "sidebar".
func (p *Partial) WithTemplate(templatePath string) *Partial {
	return p.With(NewID(inferTemplateID(templatePath), templatePath))
}

func inferTemplateID(templatePath string) string {
	normalized := strings.ReplaceAll(templatePath, `\`, `/`)
	base := path.Base(strings.Trim(normalized, `/`))
	if base == "." || base == "/" || base == "" {
		return strings.Trim(templatePath, `/\`)
	}
	ext := path.Ext(base)
	if ext == "" {
		return base
	}
	return strings.TrimSuffix(base, ext)
}

// WithOOB registers an out-of-band child partial on the partial tree. A
// partial response sends the out-of-band children of a partial in the order
// WithOOB registered them, so it gives the same bytes on every render;
// registering an ID again keeps its place.
func (p *Partial) WithOOB(child *Partial) *Partial {
	if p == nil || child == nil {
		return p
	}

	p.With(child)
	p.mu.Lock()
	if !slices.Contains(p.oobChildren, child.id) {
		p.oobChildren = append(p.oobChildren, child.id)
	}

	p.mu.Unlock()

	return p
}

func (p *Partial) getConnectorResponseHeaders() map[string]string {
	if p == nil {
		return nil
	}

	conn := p.getConnector()
	if conn == nil {
		return nil
	}

	return conn.ResponseHeaders(p.response)
}

func applyRenderResponseHeaders(w http.ResponseWriter, response *RenderResponse) {
	if w == nil || response == nil {
		return
	}
	for key, value := range response.Headers {
		w.Header().Set(key, value)
	}
}

func (p *Partial) isPartialRequest(r *http.Request) bool {
	if p == nil || r == nil {
		return false
	}

	conn := p.getConnector()
	return conn != nil && conn.RenderPartial(r)
}

// getStaticFuncMap returns the combined function map of the partial.
func (p *Partial) getStaticFuncMap() template.FuncMap {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.parent != nil {
		funcs := maps.Clone(p.parent.getStaticFuncMap())
		maps.Copy(funcs, p.funcs.staticFuncs())
		return funcs
	}

	funcs := maps.Clone(p.funcs.staticFuncs())
	if funcs == nil {
		funcs = make(template.FuncMap)
	}

	return funcs
}

// getDot returns the dot of the partial or of its nearest ancestor with
// one: the value set with SetDot, or the function set with SetDotSource
// that computes it.
func (p *Partial) getDot() (value any, lazy *lazyDot, ok bool) {
	for current := p; current != nil; {
		current.mu.RLock()
		value := current.dot
		lazy := current.dotFunc
		ok := current.hasDot
		parent := current.parent
		current.mu.RUnlock()

		if ok || lazy != nil {
			return value, lazy, true
		}

		current = parent
	}

	return nil, nil, false
}

// getFuncSet identifies the names of the functions the partial and its
// ancestors registered, for template cache keys.
func (p *Partial) getFuncSet() uint64 {
	own := p.ownFuncSet()

	p.mu.RLock()
	parent := p.parent
	p.mu.RUnlock()

	if parent == nil {
		return own
	}
	return registeredFuncSets.merge(parent.getFuncSet(), own)
}

// ownFuncSet identifies the names of the partial's own functions, computed
// once per set of functions.
func (p *Partial) ownFuncSet() uint64 {
	p.mu.RLock()
	funcs := p.funcs
	p.mu.RUnlock()

	return funcs.setID()
}

// getContracts returns the contract values of the partial and its parents,
// from the root to the partial. The slice may be shared: do not change it.
func (p *Partial) getContracts() []contractInformation {
	return fromRoot(p, ownContracts)
}

// ownContracts are the contract values set on p itself.
func ownContracts(p *Partial) []contractInformation {
	return p.contracts
}

// fromRoot joins the values own returns for p and each of its ancestors,
// the root's first. own reads a partial with its lock held. The values are
// never changed in place, so the slice of a single partial is shared: do
// not change it.
func fromRoot[T any](p *Partial, own func(*Partial) []T) []T {
	var buffer [8][]T

	levels := buffer[:0]
	size := 0

	for current := p; current != nil; {
		current.mu.RLock()
		values := own(current)
		parent := current.parent
		current.mu.RUnlock()

		if len(values) > 0 {
			levels = append(levels, values)
			size += len(values)
		}

		current = parent
	}

	switch len(levels) {
	case 0:
		return nil
	case 1:
		return slices.Clip(levels[0])
	}

	joined := make([]T, 0, size)
	for i := len(levels) - 1; i >= 0; i-- {
		joined = append(joined, levels[i]...)
	}

	return joined
}

// addRequestFuncs adds go-partial's helpers for the render, and the
// functions render stages set, to funcs, a map of every function.
func (p *Partial) addRequestFuncs(funcs template.FuncMap, state *RenderContext) {
	for name := range coreFunctionNames {
		if fn, ok := p.requestFunc(state, name); ok {
			funcs[name] = fn
		}
	}

	maps.Copy(funcs, state.Funcs)
}

// requestFunc returns the function a render stage set as name, or else
// go-partial's helper name for the render.
func (p *Partial) requestFunc(state *RenderContext, name string) (any, bool) {
	if fn, ok := state.Funcs[name]; ok {
		return fn, true
	}

	switch name {
	case "runtime":
		templateRuntime := state.Runtime
		if templateRuntime == nil || templateRuntime.partial != p {
			templateRuntime = newRuntime(p, state)
		}

		// go-doc:sig func() *github.com/donseba/go-partial.Runtime
		return func() *Runtime {
			return templateRuntime
		}, true

	case "partial":
		// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, path string) html/template.HTML
		// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, path string, dot any) html/template.HTML
		// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, path string, pairs ...any) html/template.HTML
		return partialHelper, true

	case "content":
		// go-doc:sig func() html/template.HTML
		return contentFunc(p, state), true

	case "ctx":
		// go-doc:sig func() *github.com/donseba/go-partial.RenderContext
		return func() *RenderContext {
			return state
		}, true

	case "request":
		// go-doc:sig func() *net/http.Request
		return func() *http.Request {
			return state.Request
		}, true

	case "url":
		// go-doc:sig func() *net/url.URL
		return func() *url.URL {
			return state.URL
		}, true

	case "basePath":
		// go-doc:sig func() string
		return func() string {
			return state.BasePath
		}, true
	}

	return p.navigationFunc(state, name)
}

func partialHelper(runtime *Runtime, path string, args ...any) template.HTML {
	return runtime.Partial(path, args...)
}

func (p *Partial) navigationFunc(state *RenderContext, name string) (any, bool) {
	switch name {
	case "urlIs":
		// go-doc:sig func(current string) bool
		return func(current string) bool {
			if state.URL == nil {
				return false
			}

			return strings.Trim(state.URL.Path, "/") == strings.Trim(current, "/")
		}, true

	case "urlStarts":
		// go-doc:sig func(current string) bool
		return func(current string) bool {
			if state.URL == nil {
				return false
			}

			return strings.HasPrefix(state.URL.Path, current)
		}, true

	case "urlContains":
		// go-doc:sig func(current string) bool
		return func(current string) bool {
			if state.URL == nil {
				return false
			}

			return strings.Contains(state.URL.Path, current)
		}, true

	case "joinPath":
		// go-doc:sig func(parts ...string) string
		return path.Join, true

	case "urlPath":
		// go-doc:sig func(base string, parts ...string) html/template.URL
		return urlPath, true

	case "oob":
		// go-doc:sig func() bool
		if !p.renderOOB {
			return notOOB, true
		}
		return isOOB, true

	case "oobAttr":
		// go-doc:sig func(values ...string) html/template.HTMLAttr
		if !p.renderOOB {
			return noOOBAttr, true
		}

		return oobAttr(p.getConnectorOrDefault()), true

	case "oobSwap":
		// go-doc:sig func(swap ...string) html/template.HTMLAttr
		conn := p.getConnectorOrDefault()
		return func(swap ...string) template.HTMLAttr {
			value := ""
			if len(swap) > 0 {
				value = swap[0]
			}

			return templateutil.HTMLAttrs(connector.OOBAttrs(conn, value))
		}, true
	}

	return nil, false
}

func urlPath(base string, parts ...string) template.URL {
	allParts := append([]string{base}, parts...)
	return template.URL(path.Join(allParts...))
}

func notOOB() bool {
	return false
}

func isOOB() bool {
	return true
}

func noOOBAttr(...string) template.HTMLAttr {
	return ""
}

// oobAttr returns the oobAttr function of a partial that goes out of band:
// the connector's attributes that swap its element out of band (see
// connector.OOBAttrs), with the swap given, such as "beforeend".
func oobAttr(conn connector.Connector) func(values ...string) template.HTMLAttr {
	return func(values ...string) template.HTMLAttr {
		swap := ""
		if len(values) > 0 {
			swap = values[0]
		}

		return templateutil.HTMLAttrs(connector.OOBAttrs(conn, swap))
	}
}

// placeholderRequestFuncMap holds stand-ins, with their signatures, for the
// functions go-partial gives every template when it renders, bound to the
// render: templates are parsed with them, and go-clue reads them to learn
// those functions.
//
// go-clue:funcmap
func placeholderRequestFuncMap() template.FuncMap {
	return template.FuncMap{
		"runtime":     func() *Runtime { return nil },
		"partial":     func(*Runtime, string, ...any) template.HTML { return "" },
		"content":     func() template.HTML { return "" },
		"ctx":         func() *RenderContext { return nil },
		"request":     func() *http.Request { return nil },
		"url":         func() *url.URL { return nil },
		"basePath":    func() string { return "" },
		"urlIs":       func(string) bool { return false },
		"urlStarts":   func(string) bool { return false },
		"urlContains": func(string) bool { return false },
		"joinPath":    func(...string) string { return "" },
		"urlPath":     func(string, ...string) template.URL { return "" },
		"oob":         func() bool { return false },
		"oobAttr":     func(...string) template.HTMLAttr { return "" },
		"oobSwap":     func(...string) template.HTMLAttr { return "" },
	}
}

func isProtectedFunctionName(name string) bool {
	if _, ok := coreFunctionNames[name]; ok {
		return true
	}
	return strings.HasPrefix(name, "_")
}

func (p *Partial) getBasePath() string {
	return p.GetBasePath()
}

func (p *Partial) getConnector() connector.Connector {
	if p == nil {
		return nil
	}
	if p.connector != nil {
		return p.connector
	}
	if p.parent != nil {
		return p.parent.getConnector()
	}
	return nil
}

func (p *Partial) getConnectorOrDefault() connector.Connector {
	if conn := p.getConnector(); conn != nil {
		return conn
	}
	return connector.NewPartial(nil)
}

func (p *Partial) getEvents() EventSink {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	events := p.events
	parent := p.parent
	p.mu.RUnlock()

	if events != nil {
		return events
	}
	if parent != nil {
		return parent.getEvents()
	}
	return nil
}

func (p *Partial) getFS() fs.FS {
	if p == nil {
		return os.DirFS("./")
	}
	p.mu.RLock()
	fsys := p.fs
	fsSet := p.fsSet
	parent := p.parent
	p.mu.RUnlock()

	if fsSet && fsys != nil {
		return fsys
	}
	if parent != nil {
		return parent.getFS()
	}
	if fsys != nil {
		return fsys
	}
	return os.DirFS("./")
}

func (p *Partial) emitWithContext(ctx context.Context, r *http.Request, event Event) {
	if p == nil {
		return
	}
	state := newRenderContext(ctx, p, r, RenderKindPartial)
	state.Emit(event)
}

// getRenderStages returns the stages of the partial's ancestors and its
// own, the root's first. The slice may be shared: do not change it.
func (p *Partial) getRenderStages() []RenderStage {
	return fromRoot(p, ownStages)
}

// ownStages are the render stages added to p itself with Use.
func ownStages(p *Partial) []RenderStage {
	return p.stages
}

func renderWithTargetResult(ctx context.Context, r *http.Request, p *Partial) renderResult {
	requestedTarget := p.getConnectorOrDefault().GetTargetValue(r)
	if requestedTarget == "" || requestedTarget == p.id {
		return renderTargetSelfResult(ctx, r, p)
	}

	c := p.recursiveChildLookup(requestedTarget, make(map[string]bool))
	if c != nil {
		return renderWithTargetResult(ctx, r, c)
	}

	result, ok := renderResolvedTargetResult(ctx, r, p, requestedTarget)
	if result.Err != nil || ok {
		return result
	}

	if result, ok := renderTargetFallback(ctx, r, p, requestedTarget); ok {
		return result
	}

	p.emitWithContext(ctx, r, Event{
		Kind:    EventTargetMissing,
		Level:   EventWarn,
		Message: "requested partial not found in parent",
		Fields:  map[string]any{"target": requestedTarget, "parent": p.id},
	})
	return renderResult{Err: fmt.Errorf("requested partial %s not found in parent %s", requestedTarget, p.id)}
}

// renderTargetSelfResult renders the partial as the target of a partial
// request: itself, followed by the out-of-band regions that go with it.
func renderTargetSelfResult(ctx context.Context, r *http.Request, p *Partial) renderResult {
	ctx = withPlacedRegions(ctx, newPlacedRegions(p))

	result := renderSelfResult(ctx, r, p)
	if result.Err != nil {
		return result
	}

	if err := appendOOBFrom(ctx, r, p, p.parentPartial(), &result); err != nil {
		result.Err = err
	}

	return result
}

// appendOOBFrom adds the out-of-band regions that go with the target p of a
// partial request to its result: those of p itself that always swap and
// that its output does not place (see SetAlwaysSwapOOB), which ctx, that of
// p's render, keeps track of, then those of owner and its ancestors.
func appendOOBFrom(ctx context.Context, r *http.Request, p *Partial, owner *Partial, result *renderResult) error {
	oobOutAll, vary, oobErr := renderTargetOOB(ctx, r, p, owner)
	if oobErr != nil {
		p.emitWithContext(ctx, r, Event{
			Kind:    EventRenderOOBError,
			Level:   EventError,
			Message: "error rendering OOB regions",
			Error:   oobErr,
		})
		return fmt.Errorf("error rendering OOB regions: %w", oobErr)
	}

	result.HTML += oobOutAll
	result.Vary = append(result.Vary, vary...)

	return nil
}

// renderTargetOOB renders the out-of-band regions that go with the target p,
// as appendOOBFrom adds them, and names the request headers they depend on.
func renderTargetOOB(ctx context.Context, r *http.Request, p *Partial, owner *Partial) (template.HTML, []string, error) {
	own, vary, err := renderOOBChildren(ctx, r, p, true, false)
	if err != nil {
		return "", nil, fmt.Errorf("error rendering OOB regions of '%s': %w", p.id, err)
	}

	out, ancestorsVary, err := renderOOBChildrenFrom(ctx, r, owner, true)
	if err != nil {
		return "", nil, err
	}

	return own + out, append(vary, ancestorsVary...), nil
}

// renderResolvedTargetResult renders the partial a render stage resolves
// for target, such as exp/target's resolver, with the out-of-band regions
// that go with it, unless the stage leaves them out (see OmitOOB). ok
// reports whether one was resolved.
func renderResolvedTargetResult(ctx context.Context, r *http.Request, p *Partial, target string) (renderResult, bool) {
	state := newRenderContext(ctx, p, r, RenderKindTarget)
	state.Name = target

	var (
		resolved *Partial
		placed   *placedRegions
	)

	result := renderWithChainResult(state, p.getRenderStages(), func(state *RenderContext) (template.HTML, error) {
		if state.Partial == nil || state.Partial == p {
			return "", nil
		}

		// It renders as Runtime.RenderPartial renders it; the copy that
		// renders and the regions it places are kept for its own
		// out-of-band regions.
		runtime := state.Runtime

		child, err := runtime.preparePartial(state.Partial)
		if err != nil {
			return "", err
		}

		resolved = child
		placed = newPlacedRegions(child)

		rendered := renderSelfResult(withPlacedRegions(runtime.state.Context, placed), runtime.state.Request, child)

		return rendered.HTML, rendered.Err
	})
	if result.Err != nil {
		result.Err = fmt.Errorf("error rendering resolved target '%s': %w", target, result.Err)
		return result, true
	}

	if state.Partial == nil || state.Partial == p {
		return result, false
	}

	if state.omitOOB {
		return result, true
	}

	// The resolved partial renders as a child of p: it gets the regions of p
	// as well, as a registered child does. When a stage answered without
	// rendering it, only those of p go.
	answered := resolved
	if answered == nil {
		answered = p
	}

	if err := appendOOBFrom(withPlacedRegions(ctx, placed), r, answered, p, &result); err != nil {
		result.Err = err
	}

	return result, true
}

// recursiveChildLookup looks up a registered child recursively.
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

	// The children are searched in the order of their IDs, so a tree with
	// the ID in more than one branch always answers with the same partial.
	for _, childID := range slices.Sorted(maps.Keys(p.children)) {
		if c := p.children[childID].recursiveChildLookup(id, visited); c != nil {
			return c
		}
	}

	return nil
}

func renderChildPartial(ctx context.Context, r *http.Request, p *Partial, id string) (template.HTML, error) {
	p.mu.RLock()
	child, ok := p.children[id]
	p.mu.RUnlock()
	if !ok {
		p.emitWithContext(ctx, r, Event{
			Kind:    EventTemplateMissing,
			Level:   EventWarn,
			Message: "child partial not found",
			Fields:  map[string]any{"id": id},
		})
		return "", nil
	}

	// Clone the child partial to avoid modifying the original and prevent data races.
	childClone := child.renderCopy()

	// Set the parent of the cloned child to the current partial.
	childClone.parent = p

	result := renderSelfResult(ctx, r, childClone)
	if result.Err != nil {
		childClone.emitWithContext(ctx, r, Event{
			Kind:    EventRenderError,
			Level:   EventError,
			Message: "error rendering child partial",
			Error:   result.Err,
			Fields:  map[string]any{"id": id},
		})
		fallback, fallbackErr := renderErrorFragment(ctx, r, childClone, result.Err)
		if fallbackErr != nil {
			return "", fallbackErr
		}
		return fallback, nil
	}

	return result.HTML, nil
}

func renderErrorFragment(ctx context.Context, r *http.Request, p *Partial, renderErr error) (template.HTML, error) {
	result := renderErrorResult(ctx, r, p, renderErr, true)
	if result.Err != nil {
		return "", fmt.Errorf("error rendering error response fragment: %w; original render error: %v", result.Err, renderErr)
	}

	return result.HTML, nil
}

func renderErrorResult(ctx context.Context, r *http.Request, p *Partial, renderErr error, fragment bool) renderResult {
	return renderErrorResponse(ctx, r, p, renderErr, errorRender{fragment: fragment})
}

// errorRender says what renderErrorResponse renders for an error.
type errorRender struct {
	// fragment renders the error fragment, which takes the place of a
	// failing part of a page, rather than the error page.
	fragment bool
	// response is set for the failure response of Write and WriteError,
	// which answers the request as a whole.
	response bool
	// status is the status WriteError answers an error outside a render
	// with, 0 for a failed render; see RenderContext.FailureStatus.
	status int
}

// renderErrorResponse renders the error of a failed render, or of an error
// outside a render, through the render stages of p, as how says.
func renderErrorResponse(ctx context.Context, r *http.Request, p *Partial, renderErr error, how errorRender) renderResult {
	if p == nil {
		return renderResult{Err: renderErr}
	}

	state := newRenderContext(ctx, p, r, renderKindError)
	state.failureResponse = how.response
	state.failureStatus = how.status

	if how.fragment {
		state.Name = "fragment"
	}
	state.Error = renderErr
	state.Data = renderErr

	return renderWithChainResult(state, p.getRenderStages(), func(state *RenderContext) (template.HTML, error) {
		return "", state.Error
	})
}

func renderSelfResult(ctx context.Context, r *http.Request, p *Partial) renderResult {
	notePlaced(ctx, p)

	state := newRenderContext(ctx, p, r, RenderKindPartial)

	result := renderWithChainResult(state, p.getRenderStages(), renderTemplateTerminal)
	result.Headers = p.getResponseHeaders()
	return result
}

func renderTemplate(state *RenderContext) (template.HTML, error) {
	var p *Partial
	if state != nil && state.Partial != nil {
		p = state.Partial
	}
	if p == nil {
		return "", errors.New("partial is not initialized")
	}
	if state == nil {
		return "", errors.New("render context is not configured")
	}
	state.Partial = p
	if state.Runtime == nil || state.Runtime.partial != p {
		state.Runtime = newRuntime(p, state)
	}
	if len(p.templates) == 0 {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateMissing,
			Level:   EventError,
			Message: "no templates provided for rendering",
		})
		return "", errors.New("no templates provided for rendering")
	}

	tmpl, entry, err := p.renderableTemplate(state)
	if err != nil {
		return "", err
	}

	if entry != nil {
		defer entry.Release(tmpl)
	}

	root, err := state.executionDot(p)
	if err != nil {
		return "", err
	}

	// Context functions reach the partial they are called from through
	// RuntimeFrom.
	if bindings := contextBindingsFrom(state.Context); bindings != nil {
		bindings.push(state.Runtime)
		defer bindings.pop()
	}

	return p.executeTemplate(state, tmpl, entry, root)
}

// renderableTemplate returns the parsed templates of the render of p, with
// the template cache a copy of the cached ones, and their cache entry,
// which takes the copy back with Release.
func (p *Partial) renderableTemplate(state *RenderContext) (*template.Template, *templateutil.CachedTemplate, error) {
	cached := p.usesCache()

	renderTemplates, err := p.renderTemplates(cached)
	if err != nil {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateParseError,
			Level:   EventError,
			Message: "error resolving included templates",
			Error:   err,
		})

		return nil, nil, err
	}

	cacheKey := p.generateCacheKey(renderTemplates, p.getFuncSet())
	state.shareTreeValues(p)

	// With the template cache, the functions are given once the cached
	// template tells which ones it calls; see templateFromCacheEntry.
	var funcs template.FuncMap
	if !cached {
		funcs = p.renderFuncs(state)
	}

	tmpl, entry, err := p.getTemplateForRender(state, cached, cacheKey, funcs, renderTemplates)
	if err != nil {
		var contractErr *contractError
		if errors.As(err, &contractErr) {
			return nil, nil, contractErr.err
		}

		state.EmitForPartial(p, Event{
			Kind:    EventTemplateParseError,
			Level:   EventError,
			Message: "error getting or parsing template",
			Error:   err,
		})

		return nil, nil, err
	}

	return tmpl, entry, nil
}

// shareTreeValues gives the render's context a place for the functions
// bound to it and the lazy values computed in it, when p has them, so the
// partials rendered from p, which inherit the context, share them.
func (ctx *RenderContext) shareTreeValues(p *Partial) {
	if p.hasContextFuncs() {
		ctx.Context = withContextBindings(ctx.Context)
	}

	if p.lazy {
		ctx.Context = withLazyValues(ctx.Context)
	}
}

// renderFuncs returns every function of a render of p without the template
// cache: the registered ones, bound to the render, go-partial's helpers,
// stand-ins for the functions of the render stages and those the stages'
// resolvers give.
func (p *Partial) renderFuncs(state *RenderContext) template.FuncMap {
	funcs := p.getStaticFuncMap()
	p.bindContextFuncs(funcs, state)
	p.addRequestFuncs(funcs, state)
	addStageStandIns(funcs, state.stages)
	state.resolveFuncs(funcs)

	return funcs
}

// executionDot returns the dot p's templates execute with: its value, a
// lazy one computed once per render tree, or nil without a dot.
func (ctx *RenderContext) executionDot(p *Partial) (any, error) {
	dot, lazy, hasDot := p.getDot()
	if lazy != nil {
		// Partials rendered from this one share the values computed here.
		value, err := ctx.lazyDotValue(lazy)
		if err != nil {
			return nil, fmt.Errorf("error computing the dot of '%s': %w", p.id, err)
		}

		return value, nil
	}

	if !hasDot {
		return nil, nil
	}

	return dot, nil
}

// executeTemplate executes tmpl, or the template of p's entry in it, with
// root as its dot.
func (p *Partial) executeTemplate(
	state *RenderContext,
	tmpl *template.Template,
	entry *templateutil.CachedTemplate,
	root any,
) (template.HTML, error) {
	// The output starts at the size of the template's last output, so it
	// is not grown and copied piece by piece.
	var buf strings.Builder
	buf.Grow(entry.SizeHint(p.entry))

	var err error
	if p.entry != "" {
		err = tmpl.ExecuteTemplate(&buf, p.entry, root)
	} else {
		err = tmpl.Execute(&buf, root)
	}

	if err != nil {
		return "", p.executeError(state, err)
	}

	entry.NoteSize(p.entry, buf.Len())

	return template.HTML(buf.String()), nil
}

// executeError emits the failed execution of p's template and returns the
// error of the render.
func (p *Partial) executeError(state *RenderContext, err error) error {
	fields := map[string]any{"template": p.templates[0]}
	if p.entry != "" {
		fields["entry"] = p.entry
	}

	state.EmitForPartial(p, Event{
		Kind:    EventTemplateExecuteError,
		Level:   EventError,
		Message: "error executing template",
		Error:   err,
		Fields:  fields,
	})

	if p.entry != "" {
		return fmt.Errorf("error executing template '%s' of '%s': %w", p.entry, p.templates[0], err)
	}

	return fmt.Errorf("error executing template '%s': %w", p.templates[0], err)
}

// renderOOBChildren renders the out-of-band regions of p that go with the
// target of a partial request (see swappedOOBChildren). vary names the
// request headers the elements they send depend on; see
// SetOOBElementsFunc.
func renderOOBChildren(
	ctx context.Context,
	r *http.Request,
	p *Partial,
	renderOOB bool,
	isAncestor bool,
) (out template.HTML, vary []string, err error) {
	for _, child := range p.swappedOOBChildren(ctx, isAncestor) {
		id := child.id
		childClone := child.renderCopy()
		childClone.parent = p
		childClone.renderOOB = renderOOB
		result := renderSelfResult(ctx, r, childClone)
		if result.Err != nil {
			return "", nil, fmt.Errorf("error rendering OOB region '%s': %w", id, result.Err)
		}

		if renderOOB {
			var elementsVary []string

			result.HTML, elementsVary = childClone.oobOutput(r, result.HTML)
			vary = append(vary, elementsVary...)
		}

		out += result.HTML
	}

	return out, vary, nil
}

// swappedOOBChildren returns the out-of-band regions of p that go with the
// target of a partial request, in the order they render: all of them when p
// is an ancestor of the target; when p is the target, those that always
// swap (see SetAlwaysSwapOOB) and that its render, that of ctx, did not
// place.
func (p *Partial) swappedOOBChildren(ctx context.Context, isAncestor bool) []*Partial {
	var placed *placedRegions
	if !isAncestor {
		placed = placedRegionsFrom(ctx)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var children []*Partial

	for _, id := range p.oobChildren {
		child, ok := p.children[id]
		if !ok {
			continue
		}

		if isAncestor || (child.alwaysSwapOOB && placed.isUnplaced(id)) {
			children = append(children, child)
		}
	}

	return children
}

func renderAllAncestorOOBChildren(
	ctx context.Context,
	r *http.Request,
	p *Partial,
	renderOOB bool,
) (template.HTML, []string, error) {
	return renderOOBChildrenFrom(ctx, r, p.parentPartial(), renderOOB)
}

// renderOOBChildrenFrom renders the out-of-band regions of owner and its
// ancestors, and names the request headers they depend on.
func renderOOBChildrenFrom(
	ctx context.Context,
	r *http.Request,
	owner *Partial,
	renderOOB bool,
) (template.HTML, []string, error) {
	var (
		out  template.HTML
		vary []string
	)

	for ancestor := owner; ancestor != nil; ancestor = ancestor.parentPartial() {
		chunk, chunkVary, err := renderOOBChildren(ctx, r, ancestor, renderOOB, true)
		if err != nil {
			return "", nil, fmt.Errorf("error rendering OOB regions from ancestor '%s': %w", ancestor.id, err)
		}

		out += chunk
		vary = append(vary, chunkVary...)
	}

	return out, vary, nil
}

// getTemplateForRender returns the template of the render: with the
// template cache, a copy of the cached template with the functions its
// templates call, and its cache entry, which takes it back with Release;
// without, a template parsed with funcs, every function.
func (p *Partial) getTemplateForRender(
	state *RenderContext,
	cached bool,
	cacheKey string,
	funcs template.FuncMap,
	renderTemplates []string,
) (*template.Template, *templateutil.CachedTemplate, error) {
	store := p.getTemplateStore()

	// A file system that cannot be told apart from others of its type, such
	// as a struct holding a map, shares no parses: its templates are parsed
	// for every render.
	_, shared := fileSystemKey(p.getFS())
	scanner := p.scannerFor(cached)

	if cached && shared {
		if entry, cached := store.Load(cacheKey); cached {
			return p.templateFromCacheEntry(state, entry)
		}

		mu := store.Mutex(cacheKey)
		mu.Lock()
		defer mu.Unlock()

		// Double-check after acquiring lock
		if entry, cached := store.Load(cacheKey); cached {
			return p.templateFromCacheEntry(state, entry)
		}
	}

	tmpl, contracts, err := p.parseForRender(state, scanner, cached, funcs, renderTemplates)
	if err != nil {
		return nil, nil, err
	}

	if !cached {
		return tmpl, nil, nil
	}

	requiredFuncs, err := scanner.RequiredFuncs(renderTemplates)
	if err != nil {
		return nil, nil, fmt.Errorf("error scanning template requirements: %w", err)
	}

	entry := templateutil.NewCachedTemplate(tmpl, requiredFuncs, contracts)
	if shared {
		store.Store(cacheKey, entry)
	}

	return p.templateFromCacheEntry(state, entry)
}

// parseForRender parses the templates of a render, with the typed roots
// they declare, which it returns: without the template cache with funcs,
// every function, and the roots' values; with it, with stand-ins for the
// functions the templates call and for the roots, as the copies of the
// cached templates get them for each render.
func (p *Partial) parseForRender(
	state *RenderContext,
	scanner templateutil.Scanner,
	cached bool,
	funcs template.FuncMap,
	renderTemplates []string,
) (*template.Template, map[string]templateutil.RootContract, error) {
	parseFuncs := funcs
	if cached {
		parseFuncs = p.cacheParseFuncs(state, scanner, renderTemplates)
	}

	t := template.New(path.Base(p.templates[0])).Funcs(parseFuncs)

	contracts, err := scanner.RootContracts(renderTemplates)
	if err != nil {
		return nil, nil, fmt.Errorf("error scanning template contracts: %w", err)
	}

	if err := validateRootContracts(contracts); err != nil {
		return nil, nil, err
	}

	if len(contracts) > 0 {
		if cached {
			t.Funcs(placeholderRootFuncMap(contracts))
		} else if err := registerRootContracts(state, t, contracts, p.getContracts()); err != nil {
			return nil, nil, err
		}
	}

	tmpl, err := t.ParseFS(p.getFS(), renderTemplates...)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing templates: %w", err)
	}

	if err := templateutil.AddPathAliases(tmpl, renderTemplates); err != nil {
		return nil, nil, fmt.Errorf("error adding template path aliases: %w", err)
	}

	return tmpl, contracts, nil
}

// cacheParseFuncs are the functions the templates of a render are parsed
// with for the template cache: the functions are given for each render,
// and these stand in, only those the templates call, so each copy of the
// cached template carries a few. Templates that do not scan are parsed
// with every function, so the parse reports what is wrong.
func (p *Partial) cacheParseFuncs(state *RenderContext, scanner templateutil.Scanner, renderTemplates []string) template.FuncMap {
	funcs := templateutil.MergeFuncMaps(p.getStaticFuncMap(), placeholderRequestFuncMap())
	addStageStandIns(funcs, state.stages)

	required, err := scanner.RequiredFuncs(renderTemplates)
	if err != nil {
		return funcs
	}

	return templateutil.FilterFuncMap(funcs, required)
}

func (p *Partial) templateTree() []string {
	// A partial without children renders its own templates.
	p.mu.RLock()
	templates := p.templates
	leaf := len(p.children) == 0
	p.mu.RUnlock()

	if leaf && !hasDuplicateNames(templates) {
		return slices.Clip(templates)
	}

	seen := make(map[string]struct{})
	refs := make(map[string]struct{})
	return p.collectTemplateTree(seen, refs)
}

func hasDuplicateNames(names []string) bool {
	for i, name := range names {
		if slices.Contains(names[:i], name) {
			return true
		}
	}

	return false
}

func (p *Partial) collectTemplateTree(seen map[string]struct{}, refs map[string]struct{}) []string {
	if p == nil {
		return nil
	}

	var templates []string
	for _, name := range p.templates {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		templates = append(templates, name)
	}
	maps.Copy(refs, p.scanner().ReferencedTemplates(p.templates))

	p.mu.RLock()
	children := make([]*Partial, 0, len(p.children))
	for _, child := range p.children {
		children = append(children, child)
	}
	p.mu.RUnlock()

	slices.SortFunc(children, func(a, b *Partial) int {
		return strings.Compare(a.id, b.id)
	})

	for _, child := range children {
		if !child.matchesTemplateReference(refs) {
			continue
		}
		templates = append(templates, child.collectTemplateTree(seen, refs)...)
	}

	return templates
}

func (p *Partial) matchesTemplateReference(refs map[string]struct{}) bool {
	if p == nil || len(refs) == 0 {
		return false
	}

	defined := p.scanner().DefinedTemplates(p.templates)
	for name := range defined {
		if _, ok := refs[name]; ok {
			return true
		}
	}
	return false
}

// templateFromCacheEntry gives a cached template the functions it calls:
// the values of its typed roots, the render's own functions and go-partial's
// helpers, those of the render stages' resolvers, and else the partial's
// registered functions, which may differ from those of the partial that
// parsed it.
func (p *Partial) templateFromCacheEntry(
	state *RenderContext,
	entry *templateutil.CachedTemplate,
) (*template.Template, *templateutil.CachedTemplate, error) {
	contracts := entry.RootContracts()

	var bindings []contractInformation
	if len(contracts) > 0 {
		bindings = p.getContracts()
		if err := checkRootContracts(contracts, bindings); err != nil {
			return nil, nil, &contractError{err: err}
		}
	}

	tmpl, err := entry.TemplateResolving(func(name string) (any, bool) {
		if contract, ok := contracts[name]; ok {
			value, err := resolveContractValue(name, contract, bindings)
			if err == nil {
				return rootValueFunc(state, value), true
			}
		}

		if fn, ok := p.requestFunc(state, name); ok {
			return fn, true
		}
		if fn, ok := state.resolveFunc(name); ok {
			return fn, true
		}
		return p.lookupTemplateFunc(state, name)
	})
	if err != nil {
		return nil, nil, err
	}

	return tmpl, entry, nil
}

// contractError is a typed root without its value, which fails the render
// as an execution error rather than a parse error.
type contractError struct {
	err error
}

func (e *contractError) Error() string {
	return e.err.Error()
}

func (e *contractError) Unwrap() error {
	return e.err
}

// funcSets numbers the function-name signatures of partials, so cache keys
// stay short and a render merges two numbers instead of two name lists.
// Signatures are never forgotten: an application has a few function sets.
type funcSets struct {
	mu     sync.RWMutex
	ids    map[string]uint64
	names  []string
	merged map[[2]uint64]uint64
}

var registeredFuncSets = &funcSets{
	ids:    map[string]uint64{},
	names:  []string{""},
	merged: map[[2]uint64]uint64{},
}

func (sets *funcSets) intern(signature string) uint64 {
	sets.mu.RLock()
	id, ok := sets.ids[signature]
	sets.mu.RUnlock()
	if ok {
		return id
	}

	sets.mu.Lock()
	defer sets.mu.Unlock()
	if id, ok := sets.ids[signature]; ok {
		return id
	}
	id = uint64(len(sets.names))
	sets.ids[signature] = id
	sets.names = append(sets.names, signature)
	return id
}

// merge identifies the union of the two sets.
func (sets *funcSets) merge(parent, own uint64) uint64 {
	if parent == own {
		return parent
	}

	pair := [2]uint64{parent, own}
	sets.mu.RLock()
	id, ok := sets.merged[pair]
	parentNames, ownNames := sets.names[parent], sets.names[own]
	sets.mu.RUnlock()
	if ok {
		return id
	}

	id = sets.intern(templateutil.MergeFunctionSignatures(parentNames, ownNames))
	sets.mu.Lock()
	sets.merged[pair] = id
	sets.mu.Unlock()
	return id
}

// coreFuncSignature is the signature of go-partial's own helpers, which
// every template has.
var coreFuncSignature = templateutil.FunctionNameSignatureFromSet(coreFunctionNames)

func templateFuncSignature(funcs template.FuncMap) string {
	if len(funcs) == 0 {
		return coreFuncSignature
	}

	return templateutil.MergeFunctionSignatures(templateutil.FunctionNameSignature(funcs), coreFuncSignature)
}

// getTemplateStore returns the store of the tree's root, so partials created
// for one render, such as regions, share the parses and scans of the tree
// they render in. Cache keys tell file systems and function sets apart.
func (p *Partial) getTemplateStore() *templateutil.Store {
	p.mu.RLock()
	parent := p.parent
	store := p.templateCache
	p.mu.RUnlock()

	if parent != nil {
		return parent.getTemplateStore()
	}

	if store != nil {
		return store
	}

	// The store of a tree's root is made at its first render, or when it is
	// cloned, so its clones share it.
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.templateCache == nil {
		p.templateCache = templateutil.NewStore()
	}

	return p.templateCache
}

// scanner reads the partial's template files. With the template cache, each
// file is scanned once per file system; without, on every render, so edits
// show.
func (p *Partial) scanner() templateutil.Scanner {
	return p.scannerFor(p.usesCache())
}

// scannerFor is scanner with the partial's template cache setting.
func (p *Partial) scannerFor(cached bool) templateutil.Scanner {
	fsys := p.getFS()
	key := ""

	if cached {
		if id, unique := fileSystemKey(fsys); unique {
			key = id
		}
	}
	return p.getTemplateStore().Scanner(fsys, key)
}

// clone returns an independent copy of the partial and its child tree.
func (p *Partial) clone() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clone := p.copyLocked()

	clone.children = make(map[string]*Partial, len(p.children))
	for id, child := range p.children {
		childClone := child.clone()
		childClone.parent = clone
		clone.children[id] = childClone
	}

	return clone
}

// renderCopy returns a copy of the partial to render: it shares the
// partials of its child tree, which renders copy in turn before rendering
// them, so a render never changes the tree it was given.
func (p *Partial) renderCopy() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clone := p.copyLocked()
	if len(p.children) > 0 {
		clone.children = maps.Clone(p.children)
	}

	return clone
}

// copyLocked copies the partial's own configuration, without its children.
// The copy shares the slices of the partial, clipped, so an append to either
// does not reach the other; none is changed in place.
func (p *Partial) copyLocked() *Partial {
	clone := &Partial{
		id:              p.id,
		parent:          p.parent,
		contentID:       p.contentID,
		renderOOB:       p.renderOOB,
		alwaysSwapOOB:   p.alwaysSwapOOB,
		oobElements:     p.oobElements,
		oobElementsFunc: p.oobElementsFunc,
		entry:           p.entry,
		fs:              p.fs,
		fsSet:           p.fsSet,
		connector:       p.connector,
		cache:           p.cache,
		includes:        slices.Clip(p.includes),
		childIncludes:   p.childIncludes,
		templates:       slices.Clip(p.templates),
		funcs:           p.funcs,
		basePath:        p.basePath,
		dot:             p.dot,
		hasDot:          p.hasDot,
		dotFunc:         p.dotFunc,
		lazy:            p.lazy,
		contracts:       slices.Clip(p.contracts),
		responseHeaders: p.responseHeaders,
		responseStatus:  p.responseStatus,
		response:        p.response,
		failurePolicy:   p.failurePolicy,
		targetFallback:  p.targetFallback,
		events:          p.events,
		stages:          slices.Clip(p.stages),
		templateCache:   p.templateCache,
	}

	clone.extension = p.extension
	if len(p.extensions) > 0 {
		clone.extensions = maps.Clone(p.extensions)
	}

	if len(p.oobChildren) > 0 {
		clone.oobChildren = slices.Clip(p.oobChildren)
	}

	return clone
}

// Generate a hash of the template paths and available function names to include in the cache key.
func (p *Partial) generateCacheKey(templates []string, funcSet uint64) string {
	fsID := fileSystemID(p.getFS())

	var number [20]byte
	funcs := strconv.AppendUint(number[:0], funcSet, 10)

	size := len("fs:") + len(fsID) + len(";funcs:") + len(funcs)
	for _, tmpl := range templates {
		size += len(tmpl) + 1
	}

	var builder strings.Builder
	builder.Grow(size)

	for _, tmpl := range templates {
		builder.WriteString(tmpl)
		builder.WriteString(";")
	}

	builder.WriteString("fs:")
	builder.WriteString(fsID)
	builder.WriteString(";funcs:")
	builder.Write(funcs)

	return builder.String()
}

var (
	fileSystemIDs  sync.Map
	nextFileSystem atomic.Uint64
)

// fileSystemID tells file systems apart in cache keys; see fileSystemKey.
func fileSystemID(fsys fs.FS) string {
	id, _ := fileSystemKey(fsys)
	return id
}

type fileSystemEntry struct {
	id   string
	fsys fs.FS
}

// fileSystemKey tells file systems apart in cache keys, so partials that use
// the same template names from different file systems share neither cached
// templates nor scans. Comparable file systems are told apart by value, maps
// by identity. Other file systems fall back to their type name, which is not
// unique: unique reports whether id is.
func fileSystemKey(fsys fs.FS) (id string, unique bool) {
	if fsys == nil {
		return "", false
	}

	// An overlay is identified by its layers, so overlays built for every
	// render share their cache entries.
	if overlay, ok := fsys.(overlayFS); ok {
		return overlayKey(overlay)
	}

	if !comparableFileSystem(fsys) {
		value := reflect.ValueOf(fsys)
		if value.Kind() != reflect.Map {
			return fmt.Sprintf("%T", fsys), false
		}
		return mapFileSystemID(value.Pointer(), fsys), true
	}

	if stored, ok := fileSystemIDs.Load(fsys); ok {
		return stored.(*fileSystemEntry).id, true
	}

	entry := &fileSystemEntry{id: strconv.FormatUint(nextFileSystem.Add(1), 10), fsys: fsys}
	stored, _ := fileSystemIDs.LoadOrStore(fsys, entry)

	return stored.(*fileSystemEntry).id, true
}

// mapFileSystemIDs identify file systems that are maps, such as
// fstest.MapFS, by their address, and keep them, so the address is not
// reused by another one.
var mapFileSystemIDs = struct {
	sync.RWMutex
	entries map[uintptr]*fileSystemEntry
}{entries: make(map[uintptr]*fileSystemEntry)}

func mapFileSystemID(pointer uintptr, fsys fs.FS) string {
	mapFileSystemIDs.RLock()
	entry, ok := mapFileSystemIDs.entries[pointer]
	mapFileSystemIDs.RUnlock()

	if ok {
		return entry.id
	}

	mapFileSystemIDs.Lock()
	defer mapFileSystemIDs.Unlock()

	if entry, ok := mapFileSystemIDs.entries[pointer]; ok {
		return entry.id
	}

	entry = &fileSystemEntry{id: strconv.FormatUint(nextFileSystem.Add(1), 10), fsys: fsys}
	mapFileSystemIDs.entries[pointer] = entry

	return entry.id
}

// comparableFileSystem reports whether fsys can be compared with ==, without
// looking at its value for the common kinds, such as os.DirFS.
func comparableFileSystem(fsys fs.FS) bool {
	typ := reflect.TypeOf(fsys)
	switch typ.Kind() {
	case reflect.Interface, reflect.Struct, reflect.Array:
		return reflect.ValueOf(fsys).Comparable()
	default:
		return typ.Comparable()
	}
}

func overlayKey(overlay overlayFS) (string, bool) {
	var builder strings.Builder

	unique := true

	builder.WriteString("overlay(")

	for i, layer := range overlay.layers {
		if i > 0 {
			builder.WriteString(",")
		}

		id, layerUnique := fileSystemKey(layer)
		unique = unique && layerUnique
		builder.WriteString(id)
	}

	builder.WriteString(")")

	return builder.String(), unique
}
