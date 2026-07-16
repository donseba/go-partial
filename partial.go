package partial

import (
	"bytes"
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
	"slices"
	"strings"
	"sync"

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
		id              string
		parent          *Partial
		contentID       string
		renderOOB       bool
		alwaysSwapOOB   bool
		fs              fs.FS
		fsSet           bool
		connector       connector.Connector
		useCache        bool
		templates       []string
		staticFuncs     template.FuncMap
		basePath        string
		contracts       []contractInformation
		extensions      map[any]any
		responseHeaders map[string]string
		responseStatus  int
		response        connector.Response
		events          EventSink
		stages          []RenderStage
		templateCache   *templateutil.Store
		mu              sync.RWMutex
		children        map[string]*Partial
		oobChildren     map[string]struct{}
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
		Value      any
	}

	// NamedContract lets values choose their go-doc contract name.
	NamedContract interface {
		ContractName() string
	}
)

const (
	contractRoot contractKind = "root"
	contractDot  contractKind = "dot"
	contractFunc contractKind = "func"
)

// New creates a root partial with the default ID "root".
func New(templates ...string) *Partial {
	functions := make(template.FuncMap)
	return &Partial{
		id:            "root",
		templates:     templates,
		staticFuncs:   functions,
		children:      make(map[string]*Partial),
		oobChildren:   make(map[string]struct{}),
		extensions:    make(map[any]any),
		fs:            os.DirFS("./"),
		templateCache: templateutil.NewStore(),
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
	return p.clone()
}

// SetExtension stores extension-owned data on this partial.
func (p *Partial) SetExtension(key any, value any) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.extensions == nil {
		p.extensions = make(map[any]any)
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
func (p *Partial) Use(stages ...RenderStage) *Partial {
	if p == nil {
		return nil
	}

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
	p.upsertContractLocked(contractInformation{Kind: contractDot, Value: value}, func(existing contractInformation) bool {
		return existing.Kind == contractDot
	})
	return p
}

// ClearDot removes the explicit root value.
func (p *Partial) ClearDot() *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removeContractsLocked(func(existing contractInformation) bool {
		return existing.Kind == contractDot
	})
	return p
}

// SetContract registers typed values for go-doc root declarations.
// Values are matched by type unless they implement NamedContract.
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

// SetModel registers values for typed model declarations.
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

func (p *Partial) getResponseHeaders() map[string]string {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	headers := maps.Clone(p.responseHeaders)
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

// SetAlwaysSwapOOB makes this out-of-band partial render on every partial request.
func (p *Partial) SetAlwaysSwapOOB(alwaysSwapOOB bool) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.alwaysSwapOOB = alwaysSwapOOB
	return p
}

// SetFunc registers template functions in the Partial scope.
func (p *Partial) SetFunc(funcMaps ...template.FuncMap) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, funcMap := range funcMaps {
		p.setFuncMapLocked(funcMap)
	}
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

// UseTemplateCache sets the parsed template cache usage flag for the partial.
func (p *Partial) UseTemplateCache(useCache bool) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.useCache = useCache
	return p
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

// WithOOB registers an out-of-band child partial on the partial tree.
func (p *Partial) WithOOB(child *Partial) *Partial {
	if p == nil || child == nil {
		return p
	}

	p.With(child)
	p.mu.Lock()
	p.oobChildren[child.id] = struct{}{}
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
		maps.Copy(funcs, p.staticFuncs)
		return funcs
	}

	return maps.Clone(p.staticFuncs)
}

func (p *Partial) getCustomFuncMap() template.FuncMap {
	p.mu.RLock()
	defer p.mu.RUnlock()

	funcs := p.contractFuncMapLocked()
	if p.parent != nil {
		parentFuncs := p.parent.getCustomFuncMap()
		maps.Copy(parentFuncs, funcs)
		return parentFuncs
	}

	return funcs
}

func (p *Partial) contractFuncMapLocked() template.FuncMap {
	funcs := make(template.FuncMap)
	for _, contract := range p.contracts {
		if contract.Kind != contractFunc || contract.Name == "" || contract.Value == nil {
			continue
		}
		funcs[contract.Name] = contract.Value
	}
	return funcs
}

func (p *Partial) setFuncMapLocked(funcMap template.FuncMap) {
	for name, fn := range funcMap {
		if isProtectedFunctionName(name) {
			continue
		}

		p.staticFuncs[name] = fn
		p.upsertContractLocked(contractInformation{
			Kind:  contractFunc,
			Name:  name,
			Value: fn,
		}, func(existing contractInformation) bool {
			return existing.Kind == contractFunc && existing.Name == name
		})
	}
}

func (p *Partial) upsertContractLocked(contract contractInformation, match func(contractInformation) bool) {
	for i, existing := range p.contracts {
		if match(existing) {
			p.contracts[i] = contract
			return
		}
	}
	p.contracts = append(p.contracts, contract)
}

func (p *Partial) removeContractsLocked(match func(contractInformation) bool) {
	p.contracts = slices.DeleteFunc(p.contracts, match)
}

func (p *Partial) getDotContract() (any, bool) {
	contracts := p.getContracts()
	for i := len(contracts) - 1; i >= 0; i-- {
		if contracts[i].Kind == contractDot {
			return contracts[i].Value, true
		}
	}
	return nil, false
}

func (p *Partial) getFunctionSignature() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	signature := templateFuncSignature(p.staticFuncs)
	if p.parent != nil {
		signature = templateutil.MergeFunctionSignatures(p.parent.getFunctionSignature(), signature)
	}
	return signature
}

func (p *Partial) getHasCustomFunctions() bool {
	return len(p.getCustomFuncMap()) > 0
}

func (p *Partial) getContracts() []contractInformation {
	p.mu.RLock()
	defer p.mu.RUnlock()

	contracts := slices.Clone(p.contracts)
	if p.parent != nil {
		parentContracts := p.parent.getContracts()
		if len(parentContracts) > 0 {
			contracts = append(parentContracts, contracts...)
		}
	}
	return contracts
}

func (p *Partial) getRequestFuncMap(state *RenderContext) template.FuncMap {
	funcs := make(template.FuncMap, 40)
	p.addRequestFuncs(funcs, state)
	return funcs
}

func (p *Partial) addRequestFuncs(funcs template.FuncMap, state *RenderContext) {
	templateRuntime := newRuntime(p, state)

	// go-doc:sig func() *github.com/donseba/go-partial.Runtime
	funcs["runtime"] = func() *Runtime {
		return templateRuntime
	}

	// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, path string) html/template.HTML
	// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, path string, dot any) html/template.HTML
	// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, path string, pairs ...any) html/template.HTML
	funcs["partial"] = func(runtime *Runtime, path string, args ...any) template.HTML {
		return runtime.Partial(path, args...)
	}
	// go-doc:sig func() html/template.HTML
	funcs["content"] = contentFunc(p, state)
	renderCtx := func() *RenderContext {
		return state
	}

	// go-doc:sig func() *github.com/donseba/go-partial.RenderContext
	funcs["ctx"] = renderCtx

	// go-doc:sig func() *net/http.Request
	funcs["request"] = func() *http.Request {
		return state.Request
	}

	// go-doc:sig func() *net/url.URL
	funcs["url"] = func() *url.URL {
		return state.URL
	}

	// go-doc:sig func() string
	funcs["basePath"] = func() string {
		return state.BasePath
	}

	p.addNavigationFuncs(funcs, state)
	maps.Copy(funcs, state.Funcs)
}

func (p *Partial) addNavigationFuncs(funcs template.FuncMap, state *RenderContext) {

	// go-doc:sig func(current string) bool
	funcs["urlIs"] = func(current string) bool {
		if state.URL == nil {
			return false
		}
		return strings.Trim(state.URL.Path, "/") == strings.Trim(current, "/")
	}

	// go-doc:sig func(current string) bool
	funcs["urlStarts"] = func(current string) bool {
		if state.URL == nil {
			return false
		}
		return strings.HasPrefix(state.URL.Path, current)
	}

	// go-doc:sig func(current string) bool
	funcs["urlContains"] = func(current string) bool {
		if state.URL == nil {
			return false
		}
		return strings.Contains(state.URL.Path, current)
	}

	// go-doc:sig func(parts ...string) string
	funcs["joinPath"] = func(parts ...string) string {
		return path.Join(parts...)
	}

	// go-doc:sig func(base string, parts ...string) html/template.URL
	funcs["urlPath"] = func(base string, parts ...string) template.URL {
		allParts := append([]string{base}, parts...)
		return template.URL(path.Join(allParts...))
	}

	// go-doc:sig func() bool
	funcs["oob"] = func() bool {
		return p.renderOOB
	}

	// go-doc:sig func(values ...string) html/template.HTMLAttr
	funcs["oobAttr"] = func(values ...string) template.HTMLAttr {
		if p.renderOOB {
			v := "true"
			if len(values) > 0 {
				v = values[0]
			}
			return template.HTMLAttr(` hx-swap-oob="` + v + `"`)
		}
		return template.HTMLAttr("")
	}
}

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
		if parentFS := parent.getFS(); parentFS != nil {
			return parentFS
		}
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

func (p *Partial) getRenderStages() []RenderStage {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	stages := slices.Clone(p.stages)
	parent := p.parent
	p.mu.RUnlock()

	if parent != nil {
		parentStages := parent.getRenderStages()
		if len(parentStages) > 0 {
			stages = append(parentStages, stages...)
		}
	}
	return stages
}

func renderWithTargetResult(ctx context.Context, r *http.Request, p *Partial) renderResult {
	requestedTarget := p.getConnectorOrDefault().GetTargetValue(r)
	if requestedTarget == "" || requestedTarget == p.id {
		result := renderSelfResult(ctx, r, p)
		if result.Err != nil {
			return result
		}

		// Render OOB regions from the parent tree when necessary.
		oobOutAll, oobErr := renderAllAncestorOOBChildren(ctx, r, p, true)
		if oobErr != nil {
			p.emitWithContext(ctx, r, Event{
				Kind:    EventRenderOOBError,
				Level:   EventError,
				Message: "error rendering OOB regions from ancestors",
				Error:   oobErr,
			})
			result.Err = fmt.Errorf("error rendering OOB regions from ancestors: %w", oobErr)
			return result
		}
		result.HTML += oobOutAll
		return result
	} else {
		c := p.recursiveChildLookup(requestedTarget, make(map[string]bool))
		if c == nil {
			result, ok := renderResolvedTargetResult(ctx, r, p, requestedTarget)
			if result.Err != nil {
				return result
			}
			if ok {
				oobOutAll, oobErr := renderAllAncestorOOBChildren(ctx, r, p, true)
				if oobErr != nil {
					p.emitWithContext(ctx, r, Event{
						Kind:    EventRenderOOBError,
						Level:   EventError,
						Message: "error rendering OOB regions from ancestors",
						Error:   oobErr,
					})
					result.Err = fmt.Errorf("error rendering OOB regions from ancestors: %w", oobErr)
					return result
				}
				result.HTML += oobOutAll
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
		return renderWithTargetResult(ctx, r, c)
	}
}

func renderResolvedTargetResult(ctx context.Context, r *http.Request, p *Partial, target string) (renderResult, bool) {
	state := newRenderContext(ctx, p, r, RenderKindTarget)
	state.Name = target
	result := renderWithChainResult(state, p.getRenderStages(), func(state *RenderContext) (template.HTML, error) {
		if state.Partial == nil || state.Partial == p {
			return "", nil
		}
		return state.Runtime.RenderPartial(state.Partial)
	})
	if result.Err != nil {
		result.Err = fmt.Errorf("error rendering resolved target '%s': %w", target, result.Err)
		return result, true
	}
	if state.Partial == nil || state.Partial == p {
		return result, false
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

	for _, child := range p.children {
		if c := child.recursiveChildLookup(id, visited); c != nil {
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
	childClone := child.clone()

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
	if p == nil {
		return renderResult{Err: renderErr}
	}

	state := newRenderContext(ctx, p, r, renderKindError)
	if fragment {
		state.Name = "fragment"
	}
	state.Error = renderErr
	state.Data = renderErr

	return renderWithChainResult(state, p.getRenderStages(), func(state *RenderContext) (template.HTML, error) {
		return "", state.Error
	})
}

func renderSelfResult(ctx context.Context, r *http.Request, p *Partial) renderResult {
	state := newRenderContext(ctx, p, r, RenderKindPartial)

	stages := append(p.getRenderStages(), templateRenderStage())
	result := renderWithChainResult(state, stages, func(state *RenderContext) (template.HTML, error) {
		return "", errors.New("template RenderStage did not produce output")
	})
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

	dot, hasDot := p.getDotContract()
	renderTemplates := p.templateTree()
	cacheKey := p.generateCacheKey(renderTemplates, p.getFunctionSignature())
	var funcs template.FuncMap
	if p.useCache {
		funcs = p.getRequestFuncMap(state)
	} else {
		funcs = p.getStaticFuncMap()
		p.addRequestFuncs(funcs, state)
	}

	tmpl, releaseTemplate, err := p.getTemplateForRender(cacheKey, funcs, p.getHasCustomFunctions(), !p.useCache, renderTemplates)
	if err != nil {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateParseError,
			Level:   EventError,
			Message: "error getting or parsing template",
			Error:   err,
		})
		return "", err
	}
	if releaseTemplate != nil {
		defer releaseTemplate()
	}
	if p.useCache {
		if err := p.registerContractsForExecution(tmpl, renderTemplates); err != nil {
			return "", err
		}
	}

	var buf bytes.Buffer
	root := any(nil)
	if hasDot {
		root = dot
	}
	if err = tmpl.Execute(&buf, root); err != nil {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateExecuteError,
			Level:   EventError,
			Message: "error executing template",
			Error:   err,
			Fields:  map[string]any{"template": p.templates[0]},
		})
		return "", fmt.Errorf("error executing template '%s': %w", p.templates[0], err)
	}

	return template.HTML(buf.String()), nil
}

func renderOOBChildren(ctx context.Context, r *http.Request, p *Partial, renderOOB bool, isAncestor bool) (template.HTML, error) {
	var out template.HTML

	children := make(map[string]*Partial)
	p.mu.RLock()
	for id := range p.oobChildren {
		if child, ok := p.children[id]; ok {
			if isAncestor || child.alwaysSwapOOB {
				children[id] = child
			}
		}
	}
	p.mu.RUnlock()

	for id, child := range children {
		childClone := child.clone()
		childClone.parent = p
		childClone.renderOOB = renderOOB
		result := renderSelfResult(ctx, r, childClone)
		if result.Err != nil {
			return "", fmt.Errorf("error rendering OOB region '%s': %w", id, result.Err)
		}
		out += result.HTML
	}

	return out, nil
}

func renderAllAncestorOOBChildren(ctx context.Context, r *http.Request, p *Partial, renderOOB bool) (template.HTML, error) {
	var out template.HTML
	ancestor := p.parent
	for ancestor != nil {
		chunk, err := renderOOBChildren(ctx, r, ancestor, renderOOB, true)
		if err != nil {
			return "", fmt.Errorf("error rendering OOB regions from ancestor '%s': %w", ancestor.id, err)
		}
		out += chunk
		ancestor = ancestor.parent
	}
	return out, nil
}

func (p *Partial) getTemplateForRender(cacheKey string, funcs template.FuncMap, applyFullFuncs bool, funcsAreFull bool, renderTemplates []string) (*template.Template, func(), error) {
	store := p.getTemplateStore()
	if entry, cached := store.Load(cacheKey); cached && p.useCache {
		return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
	}

	mu := store.Mutex(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if entry, cached := store.Load(cacheKey); cached && p.useCache {
		return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
	}

	functions := funcs
	if !funcsAreFull {
		functions = templateutil.MergeFuncMaps(p.getStaticFuncMap(), funcs)
	}
	parseFuncs := functions
	if p.useCache {
		parseFuncs = templateutil.MergeFuncMaps(p.getStaticFuncMap(), placeholderRequestFuncMap())
	}
	t := template.New(path.Base(p.templates[0])).Funcs(parseFuncs)
	contracts, err := templateutil.RootContractsFromFS(p.getFS(), renderTemplates)
	if err != nil {
		return nil, nil, fmt.Errorf("error scanning template contracts: %w", err)
	}
	if err := validateRootContracts(contracts); err != nil {
		return nil, nil, err
	}
	if len(contracts) > 0 {
		if p.useCache {
			t.Funcs(placeholderRootFuncMap(contracts))
		} else if err := registerRootContracts(t, contracts, p.getContracts()); err != nil {
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

	if p.useCache {
		requiredFuncs, err := templateutil.RequiredFuncsFromFS(p.getFS(), renderTemplates)
		if err != nil {
			return nil, nil, fmt.Errorf("error scanning template requirements: %w", err)
		}
		entry := templateutil.NewCachedTemplate(tmpl, requiredFuncs)
		store.Store(cacheKey, entry)
		return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
	}

	return tmpl, nil, nil
}

func (p *Partial) registerContractsForExecution(tmpl *template.Template, renderTemplates []string) error {
	if tmpl == nil {
		return nil
	}
	contracts, err := templateutil.RootContractsFromFS(p.getFS(), renderTemplates)
	if err != nil {
		return fmt.Errorf("error scanning template contracts: %w", err)
	}
	if len(contracts) == 0 {
		return nil
	}
	if err := validateRootContracts(contracts); err != nil {
		return err
	}
	return registerRootContracts(tmpl, contracts, p.getContracts())
}

func (p *Partial) templateTree() []string {
	seen := make(map[string]struct{})
	refs := make(map[string]struct{})
	return p.collectTemplateTree(seen, refs)
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
	maps.Copy(refs, templateutil.ReferencedTemplatesFromFS(p.getFS(), p.templates))

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

	defined := templateutil.DefinedTemplatesFromFS(p.getFS(), p.templates)
	for name := range defined {
		if _, ok := refs[name]; ok {
			return true
		}
	}
	return false
}

func (p *Partial) templateFromCacheEntry(entry *templateutil.CachedTemplate, funcs template.FuncMap, applyFullFuncs bool, funcsAreFull bool) (*template.Template, func(), error) {
	functions := funcs
	if applyFullFuncs && !funcsAreFull {
		functions = templateutil.MergeFuncMaps(p.getCustomFuncMap(), funcs)
	}
	return entry.Template(functions)
}

func templateFuncSignature(funcs template.FuncMap) string {
	return templateutil.MergeFunctionSignatures(templateutil.FunctionNameSignature(funcs), templateutil.FunctionNameSignatureFromSet(coreFunctionNames))
}

func (p *Partial) getTemplateStore() *templateutil.Store {
	if p.templateCache != nil {
		return p.templateCache
	}
	if p.parent != nil {
		return p.parent.getTemplateStore()
	}
	p.templateCache = templateutil.NewStore()
	return p.templateCache
}

func (p *Partial) clone() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clone := &Partial{
		id:              p.id,
		parent:          p.parent,
		contentID:       p.contentID,
		renderOOB:       p.renderOOB,
		alwaysSwapOOB:   p.alwaysSwapOOB,
		fs:              p.fs,
		fsSet:           p.fsSet,
		connector:       p.connector,
		useCache:        p.useCache,
		templates:       slices.Clone(p.templates),
		staticFuncs:     maps.Clone(p.staticFuncs),
		basePath:        p.basePath,
		contracts:       slices.Clone(p.contracts),
		extensions:      maps.Clone(p.extensions),
		responseHeaders: maps.Clone(p.responseHeaders),
		responseStatus:  p.responseStatus,
		response:        p.response,
		events:          p.events,
		stages:          slices.Clone(p.stages),
		templateCache:   p.templateCache,
		children:        make(map[string]*Partial, len(p.children)),
		oobChildren:     maps.Clone(p.oobChildren),
	}
	for id, child := range p.children {
		childClone := child.clone()
		childClone.parent = clone
		clone.children[id] = childClone
	}

	return clone
}

// Generate a hash of the template paths and available function names to include in the cache key.
func (p *Partial) generateCacheKey(templates []string, templateFuncSignature string) string {
	var builder strings.Builder

	for _, tmpl := range templates {
		builder.WriteString(tmpl)
		builder.WriteString(";")
	}

	builder.WriteString("funcs:")
	builder.WriteString(templateFuncSignature)

	return builder.String()
}
