package partial

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/donseba/go-partial/connector"
)

var (
	// coreFunctionNames are the helpers go-partial injects per render and needs
	// for its own layout, request, connector, and runtime behavior. Optional
	// helper providers must not overwrite these names.
	coreFunctionNames = map[string]struct{}{
		"action":       {},
		"actionHeader": {},
		"actionIs":     {},
		"actionValue":  {},

		"selection":       {},
		"selectionHeader": {},
		"selectionIs":     {},
		"selectionValue":  {},

		"targetHeader": {},
		"targetIs":     {},
		"targetValue":  {},

		"csrf":   {},
		"locale": {},

		"url":         {},
		"urlContains": {},
		"urlIs":       {},
		"urlPath":     {},
		"urlStarts":   {},

		"content":  {},
		"ctx":      {},
		"debug":    {},
		"partial":  {},
		"request":  {},
		"runtime":  {},
		"oob":      {},
		"oobAttr":  {},
		"basePath": {},
		"joinPath": {},
	}
)

type (
	cachedTemplate struct {
		base          *template.Template
		requiredFuncs map[string]struct{}
		pool          sync.Pool
	}

	templateStore struct {
		templates sync.Map
		mutexes   sync.Map
	}

	errorFragmentContextKey struct{}

	// Partial represents a renderable component with optional child partials and data.
	Partial struct {
		id              string
		parent          *Partial
		request         *http.Request
		layoutContentID string
		renderOOB       bool
		alwaysSwapOOB   bool
		fs              fs.FS
		logger          Logger
		connector       connector.Connector
		useCache        bool
		templates       []string
		staticFuncs     template.FuncMap
		basePath        string
		contracts       []ContractInformation
		responseHeaders map[string]string
		response        connector.Response
		errorRenderer   ErrorRenderer
		debugRenderer   DebugRenderer
		templateCache   *templateStore
		errorMode       ErrorMode
		errorModeSet    bool
		targetResolver  TargetResolver
		mu              sync.RWMutex
		children        map[string]*Partial
		oobChildren     map[string]struct{}
		selection       *Selection
		templateAction  func(ctx context.Context, p *Partial, runtime *Runtime) (*Partial, error)
		action          func(ctx context.Context, p *Partial, runtime *Runtime) (*Partial, error)
	}

	Selection struct {
		Partials map[string]*Partial
		Default  string
	}

	// RenderContext contains request-scoped values exposed by the ctx template helper.
	RenderContext struct {
		Context  context.Context
		Request  *http.Request
		URL      *url.URL
		Loc      Localizer
		Locale   string
		Csrf     CsrfToken
		BasePath string
	}

	TargetResolver func(ctx context.Context, r *http.Request, target string) (*Partial, bool)

	ErrorRenderer func(ctx context.Context, p *Partial, r *http.Request, err error) (template.HTML, error)

	DebugRenderer func(ctx context.Context, p *Partial, runtime *Runtime, value any) (template.HTML, error)

	ErrorMode int

	ErrorData struct {
		Error         error
		Message       string
		PartialID     string
		Templates     []string
		Request       *http.Request
		URL           *url.URL
		Location      string
		Detailed      bool
		TemplateLabel string
	}

	DebugData struct {
		Value     any
		Output    string
		PartialID string
		Templates []string
		Request   *http.Request
		URL       *url.URL
	}

	ContractKind string

	// ContractInformation binds a Go value to a typed go-doc root declaration.
	//
	// Annotation is the declaration family, such as "model", "interaction", or
	// "component". Name is optional for type-matched values and required when
	// more than one declaration has the same Go type.
	ContractInformation struct {
		Kind       ContractKind
		Annotation string
		Name       string
		Value      any
	}

	NamedContract interface {
		ContractName() string
	}
)

const (
	ErrorModeSafe ErrorMode = iota
	ErrorModeDetailed

	ContractRoot ContractKind = "root"
	ContractDot  ContractKind = "dot"
	ContractFunc ContractKind = "func"
)

var templateErrorLocationPattern = regexp.MustCompile(`template:\s+([^:]+:\d+(?::\d+)?)`)

const defaultErrorTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Template render error</title>
<style>
body{margin:0;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#f7f7f4;color:#252522}
main{max-width:1040px;margin:0 auto;padding:32px 20px}
section{background:#fff;border:1px solid #d9d7cf;border-radius:8px;padding:20px;box-shadow:0 1px 2px rgba(0,0,0,.04)}
h1{font-size:24px;margin:0 0 8px}
p{margin:0 0 16px;color:#55524a}
dl{display:grid;grid-template-columns:120px 1fr;gap:8px 14px;margin:18px 0}
dt{font-weight:700;color:#3d3b35}
dd{margin:0;min-width:0;overflow-wrap:anywhere}
pre{white-space:pre-wrap;overflow:auto;background:#f2f0e8;border:1px solid #d8d5ca;color:#252522;border-radius:6px;padding:16px;font-size:13px;line-height:1.45}
</style>
</head>
<body>
<main>
<section>
<h1>Template render error</h1>
<p>The fallback error page was rendered by go-partial because a template failed.</p>
<dl>
<dt>Partial ID</dt><dd>{{ .PartialID }}</dd>
<dt>{{ .TemplateLabel }}</dt><dd>{{ range $i, $template := .Templates }}{{ if $i }}, {{ end }}{{ $template }}{{ end }}</dd>
{{ if .URL }}<dt>URL</dt><dd>{{ .URL.String }}</dd>{{ end }}
{{ if .Detailed }}
{{ if .Location }}<dt>Location</dt><dd>{{ .Location }}</dd>{{ end }}
<dt>Error</dt><dd>{{ .Message }}</dd>
{{ end }}
</dl>
</section>
</main>
</body>
</html>`

const defaultPartialErrorTemplate = `<section class="go-partial-error" role="alert" style="background:#fff;border:1px solid #d8d5ca;border-left:4px solid #8a4b12;border-radius:8px;padding:16px;color:#252522;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif">
<h1 style="font-size:20px;margin:0 0 8px">Template render error</h1>
<p style="margin:0 0 12px;color:#55524a">go-partial rendered this fallback because a template failed during an htmx request.</p>
<dl style="display:grid;grid-template-columns:110px 1fr;gap:8px 12px;margin:0 0 12px">
<dt style="font-weight:700">Partial ID</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .PartialID }}</dd>
<dt style="font-weight:700">{{ .TemplateLabel }}</dt><dd style="margin:0;overflow-wrap:anywhere">{{ range $i, $template := .Templates }}{{ if $i }}, {{ end }}{{ $template }}{{ end }}</dd>
{{ if .URL }}<dt style="font-weight:700">URL</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .URL.String }}</dd>{{ end }}
{{ if .Detailed }}
{{ if .Location }}<dt style="font-weight:700">Location</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .Location }}</dd>{{ end }}
<dt style="font-weight:700">Error</dt><dd style="margin:0;overflow-wrap:anywhere">{{ .Message }}</dd>
{{ end }}
</dl>
</section>`

const defaultDebugTemplate = `<section class="go-partial-debug" role="note" style="background:#fff;border:1px solid #d8d5ca;border-left:4px solid #1f6f65;border-radius:8px;color:#252522;font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;margin:12px 0;padding:14px">
<header style="align-items:center;display:flex;gap:8px;justify-content:space-between;margin-bottom:10px">
<strong style="color:#174f49;font-size:13px;text-transform:uppercase">go-partial debug</strong>
{{ if .PartialID }}<span style="color:#67645b;font-size:12px">{{ .PartialID }}</span>{{ end }}
</header>
<pre style="background:#eeece4;border:1px solid #d8d5ca;border-radius:6px;color:#252522;font-family:ui-monospace,SFMono-Regular,Consolas,'Liberation Mono',Menlo,monospace;font-size:12px;line-height:1.45;margin:0;overflow:auto;padding:12px;white-space:pre-wrap">{{ .Output }}</pre>
</section>`

const HeaderGoPartialError = "X-Go-Partial-Error"

// New creates a new root.
func New(templates ...string) *Partial {
	functions := copyFuncMap()
	return &Partial{
		id:            "root",
		templates:     templates,
		staticFuncs:   functions,
		children:      make(map[string]*Partial),
		oobChildren:   make(map[string]struct{}),
		fs:            os.DirFS("./"),
		errorRenderer: DefaultErrorRenderer(),
		debugRenderer: DefaultDebugRenderer(),
		templateCache: newTemplateStore(),
	}
}

func DefaultErrorRenderer() ErrorRenderer {
	return func(ctx context.Context, p *Partial, r *http.Request, err error) (template.HTML, error) {
		var currentURL *url.URL
		if r != nil {
			currentURL = r.URL
		}
		mode := ErrorModeSafe
		if p != nil {
			mode = p.getErrorMode()
		}
		detailed := mode == ErrorModeDetailed

		errorData := ErrorData{
			Error:     err,
			Message:   err.Error(),
			PartialID: "",
			Request:   r,
			URL:       currentURL,
			Location:  extractTemplateErrorLocation(err),
			Detailed:  detailed,
		}
		if p != nil {
			errorData.PartialID = p.id
			errorData.Templates = slices.Clone(p.templates)
		}
		errorData.TemplateLabel = "Templates"
		if len(errorData.Templates) == 1 {
			errorData.TemplateLabel = "Template"
		}

		errorTemplate := defaultErrorTemplate
		if isErrorFragmentContext(ctx) || (p != nil && p.isPartialRequest(r)) {
			errorTemplate = defaultPartialErrorTemplate
		}

		tmpl, parseErr := template.New("go-partial-error").Parse(errorTemplate)
		if parseErr != nil {
			return "", parseErr
		}

		var buf bytes.Buffer
		if execErr := tmpl.Execute(&buf, errorData); execErr != nil {
			return "", execErr
		}

		return template.HTML(buf.String()), nil
	}
}

func DefaultDebugRenderer() DebugRenderer {
	return func(ctx context.Context, p *Partial, runtime *Runtime, value any) (template.HTML, error) {
		debugData := DebugData{
			Value:  value,
			Output: formatDebugValue(value),
		}
		if p != nil {
			debugData.PartialID = p.id
			debugData.Templates = slices.Clone(p.templates)
		}
		if runtime != nil {
			debugData.Request = runtime.Request()
			debugData.URL = runtime.URL()
		}

		tmpl, parseErr := template.New("go-partial-debug").Parse(defaultDebugTemplate)
		if parseErr != nil {
			return "", parseErr
		}

		var buf bytes.Buffer
		if execErr := tmpl.Execute(&buf, debugData); execErr != nil {
			return "", execErr
		}
		return template.HTML(buf.String()), nil
	}
}

func formatDebugValue(value any) string {
	out, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		return string(out)
	}
	return fmt.Sprintf("%#v", value)
}

func extractTemplateErrorLocation(err error) string {
	if err == nil {
		return ""
	}
	match := templateErrorLocationPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func withErrorFragmentContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, errorFragmentContextKey{}, true)
}

func isErrorFragmentContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	forceFragment, _ := ctx.Value(errorFragmentContextKey{}).(bool)
	return forceFragment
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

// Reset resets the partial to its initial state.
func (p *Partial) Reset() *Partial {
	p.contracts = nil
	p.children = make(map[string]*Partial)
	p.oobChildren = make(map[string]struct{})

	return p
}

func (p *Partial) SetErrorRenderer(renderer ErrorRenderer) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.errorRenderer = renderer

	return p
}

func (p *Partial) SetDebugRenderer(renderer DebugRenderer) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.debugRenderer = renderer

	return p
}

func (p *Partial) SetErrorMode(mode ErrorMode) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.errorMode = mode
	p.errorModeSet = true

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
// Templates receive this value as "." and can still use request helpers such as
// ctx, request, url, locale, csrf, basePath, runtime, partial, content, and debug.
func (p *Partial) SetDot(value any) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.upsertContractLocked(ContractInformation{Kind: ContractDot, Value: value}, func(existing ContractInformation) bool {
		return existing.Kind == ContractDot
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
	p.removeContractsLocked(func(existing ContractInformation) bool {
		return existing.Kind == ContractDot
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
		p.getLogger().Warn("contract annotation cannot be empty")
		return p
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for _, value := range values {
		name := ""
		if named, ok := value.(NamedContract); ok {
			name = strings.TrimSpace(named.ContractName())
			if name == "" {
				p.getLogger().Warn("contract name cannot be derived", "annotation", annotation)
				continue
			}
		}
		p.contracts = append(p.contracts, ContractInformation{
			Kind:       ContractRoot,
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

func (p *Partial) SetResponseHeaders(headers map[string]string) *Partial {
	if p == nil {
		return nil
	}
	if p.parent != nil {
		p.parent.SetResponseHeaders(headers)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.responseHeaders = maps.Clone(headers)
	return p
}

func (p *Partial) GetResponseHeaders() map[string]string {
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
		return parent.GetResponseHeaders()
	}
	return nil
}

func (p *Partial) Response() *connector.ResponseBuilder {
	return connector.NewResponseBuilder(&p.response)
}

func (p *Partial) SetResponse(response connector.Response) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.response = response
	return p
}

func (p *Partial) GetBasePath() string {
	if p == nil {
		return ""
	}

	if p.basePath != "" {
		return p.basePath
	}

	if p.parent != nil {
		return p.parent.GetBasePath()
	}

	return ""
}

// SetConnector sets the connector for the partial.
func (p *Partial) SetConnector(connector connector.Connector) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.connector = connector
	return p
}

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

// SetLogger sets the logger for the partial.
func (p *Partial) SetLogger(logger Logger) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logger = logger
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

// WithAction registers a pre-render resolver.
//
// The resolver runs before this partial renders. It can inspect request data,
// perform application work, and return a different partial to render.
func (p *Partial) WithAction(action func(ctx context.Context, p *Partial, runtime *Runtime) (*Partial, error)) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.action = action
	return p
}

// WithTemplateAction registers the callback used by the {{ action }} helper.
//
// Use this when the template decides where the action result should appear.
func (p *Partial) WithTemplateAction(templateAction func(ctx context.Context, p *Partial, runtime *Runtime) (*Partial, error)) *Partial {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.templateAction = templateAction
	return p
}

// WithSelectMap registers selectable partials for the selection helper.
func (p *Partial) WithSelectMap(defaultKey string, partialsMap map[string]*Partial) *Partial {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.selection = &Selection{
		Default:  defaultKey,
		Partials: partialsMap,
	}

	return p
}

// WithTargetResolver maps dynamic DOM targets to a reusable partial and render data.
func (p *Partial) WithTargetResolver(resolver TargetResolver) *Partial {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.targetResolver = resolver

	return p
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

// RenderWithRequest renders the partial with the given http.Request.
func (p *Partial) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	p.request = r
	if p.connector == nil {
		p.connector = connector.NewPartial(nil)
	}

	if p.connector.RenderPartial(r) {
		return p.renderWithTarget(ctx, r)
	}

	return p.renderSelf(ctx, r)
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
		return p.writeRenderError(ctx, w, r, err)
	}

	// get headers
	headers := p.GetResponseHeaders()
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	for k, v := range p.GetConnectorResponseHeaders() {
		w.Header().Set(k, v)
	}

	_, err = w.Write([]byte(out))
	if err != nil {
		p.getLogger().Error("error writing partial to response", "error", err)
		return err
	}

	return nil
}

func (p *Partial) GetConnectorResponseHeaders() map[string]string {
	if p == nil {
		return nil
	}

	conn := p.getConnector()
	if conn == nil {
		return nil
	}

	return conn.ResponseHeaders(p.response)
}

func (p *Partial) writeRenderError(ctx context.Context, w http.ResponseWriter, r *http.Request, renderErr error) error {
	renderer := p.getErrorRenderer()
	if renderer == nil {
		return renderErr
	}

	out, err := renderer(ctx, p, r, renderErr)
	if err != nil {
		return fmt.Errorf("error rendering fallback error template: %w; original render error: %v", err, renderErr)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	status := http.StatusInternalServerError
	if p.isPartialRequest(r) {
		oobOut, oobErr := p.renderAllAncestorOOBChildren(ctx, r, true)
		if oobErr != nil {
			p.getLogger().Error("error rendering OOB regions for fallback error template", "error", oobErr)
			return fmt.Errorf("error rendering OOB regions for fallback error template: %w; original render error: %v", oobErr, renderErr)
		}
		out += oobOut
		status = http.StatusOK
		w.Header().Set(HeaderGoPartialError, "true")
	}
	w.WriteHeader(status)
	if _, err = w.Write([]byte(out)); err != nil {
		return fmt.Errorf("error writing fallback error template: %w; original render error: %v", err, renderErr)
	}

	return renderErr
}

func (p *Partial) isPartialRequest(r *http.Request) bool {
	if p == nil || r == nil {
		return false
	}

	conn := p.getConnector()
	return conn != nil && conn.RenderPartial(r)
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

func (p *Partial) mergeFuncMapInternal(funcMap, customFuncMap template.FuncMap) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.staticFuncs == nil {
		p.staticFuncs = make(template.FuncMap, len(funcMap))
	}
	maps.Copy(p.staticFuncs, funcMap)
	p.setFuncMapLocked(customFuncMap)
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
		if contract.Kind != ContractFunc || contract.Name == "" || contract.Value == nil {
			continue
		}
		funcs[contract.Name] = contract.Value
	}
	return funcs
}

func (p *Partial) setFuncMapLocked(funcMap template.FuncMap) {
	for name, fn := range funcMap {
		if isProtectedFunctionName(name) {
			p.getLogger().Warn("function name is protected and cannot be overwritten", "function", name)
			continue
		}

		p.staticFuncs[name] = fn
		p.upsertContractLocked(ContractInformation{
			Kind:  ContractFunc,
			Name:  name,
			Value: fn,
		}, func(existing ContractInformation) bool {
			return existing.Kind == ContractFunc && existing.Name == name
		})
	}
}

func (p *Partial) upsertContractLocked(contract ContractInformation, match func(ContractInformation) bool) {
	for i, existing := range p.contracts {
		if match(existing) {
			p.contracts[i] = contract
			return
		}
	}
	p.contracts = append(p.contracts, contract)
}

func (p *Partial) removeContractsLocked(match func(ContractInformation) bool) {
	p.contracts = slices.DeleteFunc(p.contracts, match)
}

func (p *Partial) getDotContract() (any, bool) {
	contracts := p.getContracts()
	for i := len(contracts) - 1; i >= 0; i-- {
		if contracts[i].Kind == ContractDot {
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
		signature = mergeFunctionSignatures(p.parent.getFunctionSignature(), signature)
	}
	return signature
}

func (p *Partial) getHasCustomFunctions() bool {
	return len(p.getCustomFuncMap()) > 0
}

func (p *Partial) getContracts() []ContractInformation {
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

	funcs["runtime"] = func() *Runtime {
		return templateRuntime
	}

	funcs["partial"] = func(runtime *Runtime, path string, args ...any) template.HTML {
		return runtime.Partial(path, args...)
	}
	funcs["content"] = contentFunc(p, state)
	funcs["selection"] = selectionFunc(p, state)
	funcs["action"] = actionFunc(p, state)
	funcs["debug"] = func(runtime *Runtime, value any) template.HTML {
		return runtime.Debug(value)
	}

	renderCtx := func() *RenderContext {
		return state
	}

	funcs["ctx"] = renderCtx

	funcs["request"] = func() *http.Request {
		return state.Request
	}

	funcs["url"] = func() *url.URL {
		return state.URL
	}

	funcs["locale"] = func() string {
		return renderCtx().Locale
	}

	funcs["csrf"] = func() CsrfToken {
		return state.Csrf
	}

	funcs["basePath"] = func() string {
		return state.BasePath
	}

	funcs["urlIs"] = func(current string) bool {
		if state.URL == nil {
			return false
		}
		return strings.Trim(state.URL.Path, "/") == strings.Trim(current, "/")
	}

	funcs["urlStarts"] = func(current string) bool {
		if state.URL == nil {
			return false
		}
		return strings.HasPrefix(state.URL.Path, current)
	}

	funcs["urlContains"] = func(current string) bool {
		if state.URL == nil {
			return false
		}
		return strings.Contains(state.URL.Path, current)
	}

	funcs["joinPath"] = func(parts ...string) string {
		return path.Join(parts...)
	}

	funcs["urlPath"] = func(base string, parts ...string) template.URL {
		allParts := append([]string{base}, parts...)
		return template.URL(path.Join(allParts...))
	}

	funcs["targetHeader"] = func() string {
		return p.getConnector().GetTargetHeader()
	}

	funcs["targetValue"] = func() string {
		return p.getConnector().GetTargetValue(p.GetRequest())
	}

	funcs["targetIs"] = func(in ...string) bool {
		target := p.getConnector().GetTargetValue(p.GetRequest())
		return slices.Contains(in, target)
	}

	funcs["selectionHeader"] = func() string {
		return p.getConnector().GetSelectHeader()
	}

	selectionValue := func() string {
		if p.selection == nil {
			return p.getConnector().GetSelectValue(p.GetRequest())
		}
		selectionValue := p.getConnector().GetSelectValue(p.GetRequest())

		if selectionValue == "" {
			return p.selection.Default
		}
		return selectionValue
	}
	funcs["selectionValue"] = selectionValue

	funcs["selectionIs"] = func(in ...string) bool {
		selected := selectionValue()
		return slices.Contains(in, selected)
	}

	funcs["actionHeader"] = func() string {
		return p.getConnector().GetActionHeader()
	}

	funcs["actionValue"] = func() string {
		return p.getConnector().GetActionValue(p.GetRequest())
	}

	funcs["actionIs"] = func(in ...string) bool {
		action := p.getConnector().GetActionValue(p.GetRequest())
		return slices.Contains(in, action)
	}

	funcs["oob"] = func() bool {
		return p.renderOOB
	}

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
		"runtime":         func() *Runtime { return nil },
		"partial":         func(*Runtime, string, ...any) template.HTML { return "" },
		"content":         func() template.HTML { return "" },
		"selection":       func() template.HTML { return "" },
		"action":          func() template.HTML { return "" },
		"debug":           func(*Runtime, any) template.HTML { return "" },
		"ctx":             func() *RenderContext { return nil },
		"request":         func() *http.Request { return nil },
		"url":             func() *url.URL { return nil },
		"locale":          func() string { return "" },
		"csrf":            func() CsrfToken { return nil },
		"basePath":        func() string { return "" },
		"urlIs":           func(string) bool { return false },
		"urlStarts":       func(string) bool { return false },
		"urlContains":     func(string) bool { return false },
		"joinPath":        func(...string) string { return "" },
		"urlPath":         func(string, ...string) template.URL { return "" },
		"targetHeader":    func() string { return "" },
		"targetValue":     func() string { return "" },
		"targetIs":        func(...string) bool { return false },
		"selectionHeader": func() string { return "" },
		"selectionValue":  func() string { return "" },
		"selectionIs":     func(...string) bool { return false },
		"actionHeader":    func() string { return "" },
		"actionValue":     func() string { return "" },
		"actionIs":        func(...string) bool { return false },
		"oob":             func() bool { return false },
		"oobAttr":         func(...string) template.HTMLAttr { return "" },
	}
}

func mergeFuncMaps(staticFuncs, requestFuncs template.FuncMap) template.FuncMap {
	funcs := make(template.FuncMap, len(staticFuncs)+len(requestFuncs))
	maps.Copy(funcs, staticFuncs)
	maps.Copy(funcs, requestFuncs)
	return funcs
}

func isProtectedFunctionName(name string) bool {
	if _, ok := coreFunctionNames[name]; ok {
		return true
	}
	return strings.HasPrefix(name, "_")
}

func filterFuncMap(funcs template.FuncMap, required map[string]struct{}) template.FuncMap {
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

func (p *Partial) getBasePath() string {
	if p.parent != nil {
		bp := p.parent.getBasePath()
		if bp != "" {
			return bp
		}
	}
	return p.basePath
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

func (p *Partial) getSelectionPartials() map[string]*Partial {
	if p.selection != nil {
		return p.selection.Partials
	}
	return nil
}

func (p *Partial) GetRequest() *http.Request {
	if p.request != nil {
		return p.request
	}
	if p.parent != nil {
		return p.parent.GetRequest()
	}
	return &http.Request{}
}

func (p *Partial) getFS() fs.FS {
	if p == nil {
		return os.DirFS("./")
	}
	if p.parent != nil {
		if parentFS := p.parent.getFS(); parentFS != nil {
			return parentFS
		}
	}
	if p.fs != nil {
		return p.fs
	}
	return os.DirFS("./")
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

	return slog.Default().WithGroup("partial")
}

func (p *Partial) getErrorRenderer() ErrorRenderer {
	if p == nil {
		return DefaultErrorRenderer()
	}

	if p.errorRenderer != nil {
		return p.errorRenderer
	}

	if p.parent != nil {
		return p.parent.getErrorRenderer()
	}

	return DefaultErrorRenderer()
}

func (p *Partial) getDebugRenderer() DebugRenderer {
	if p == nil {
		return DefaultDebugRenderer()
	}

	if p.debugRenderer != nil {
		return p.debugRenderer
	}

	if p.parent != nil {
		return p.parent.getDebugRenderer()
	}

	return DefaultDebugRenderer()
}

func (p *Partial) getErrorMode() ErrorMode {
	if p == nil {
		return ErrorModeSafe
	}

	if p.errorModeSet {
		return p.errorMode
	}

	if p.parent != nil {
		return p.parent.getErrorMode()
	}

	return ErrorModeSafe
}

func (p *Partial) getTargetResolver() TargetResolver {
	if p == nil {
		return nil
	}

	if p.targetResolver != nil {
		return p.targetResolver
	}

	if p.parent != nil {
		return p.parent.getTargetResolver()
	}

	return nil
}

func (p *Partial) GetRequestTargetValue() string {
	if p == nil {
		return ""
	}
	if conn := p.getConnector(); conn != nil {
		if target := conn.GetTargetValue(p.GetRequest()); target != "" {
			return target
		}
	}
	if p.parent != nil {
		return p.parent.GetRequestTargetValue()
	}
	return ""
}

func (p *Partial) GetRequestActionValue() string {
	if p == nil {
		return ""
	}
	if conn := p.getConnector(); conn != nil {
		if action := conn.GetActionValue(p.GetRequest()); action != "" {
			return action
		}
	}
	if p.parent != nil {
		return p.parent.GetRequestActionValue()
	}
	return ""
}

func (p *Partial) GetRequestSelectionValue() string {
	if p == nil {
		return ""
	}
	if conn := p.getConnector(); conn != nil {
		if selection := conn.GetSelectValue(p.GetRequest()); selection != "" {
			return selection
		}
	}
	if p.parent != nil {
		return p.parent.GetRequestSelectionValue()
	}
	return ""
}

func (p *Partial) renderWithTarget(ctx context.Context, r *http.Request) (template.HTML, error) {
	requestedTarget := p.getConnector().GetTargetValue(p.GetRequest())
	if requestedTarget == "" || requestedTarget == p.id {
		out, err := p.renderSelf(ctx, r)
		if err != nil {
			return "", err
		}

		// Render OOB regions from the parent tree when necessary.
		oobOutAll, oobErr := p.renderAllAncestorOOBChildren(ctx, r, true)
		if oobErr != nil {
			p.getLogger().Error("error rendering OOB regions from ancestors", "error", oobErr)
			return "", fmt.Errorf("error rendering OOB regions from ancestors: %w", oobErr)
		}
		out += oobOutAll
		return out, nil
	} else {
		c := p.recursiveChildLookup(requestedTarget, make(map[string]bool))
		if c == nil {
			out, ok, err := p.renderResolvedTarget(ctx, r, requestedTarget)
			if err != nil {
				return "", err
			}
			if ok {
				oobOutAll, oobErr := p.renderAllAncestorOOBChildren(ctx, r, true)
				if oobErr != nil {
					p.getLogger().Error("error rendering OOB regions from ancestors", "error", oobErr)
					return "", fmt.Errorf("error rendering OOB regions from ancestors: %w", oobErr)
				}
				return out + oobOutAll, nil
			}

			p.getLogger().Error("requested partial not found in parent", "id", requestedTarget, "parent", p.id)
			return "", fmt.Errorf("requested partial %s not found in parent %s", requestedTarget, p.id)
		}
		return c.renderWithTarget(ctx, r)
	}
}

func (p *Partial) renderResolvedTarget(ctx context.Context, r *http.Request, target string) (template.HTML, bool, error) {
	resolver := p.getTargetResolver()
	if resolver == nil {
		return "", false, nil
	}

	resolvedPartial, ok := resolver(ctx, r, target)
	if !ok || resolvedPartial == nil {
		return "", false, nil
	}

	resolvedClone := resolvedPartial.clone()
	resolvedClone.parent = p

	out, err := resolvedClone.renderSelf(ctx, r)
	if err != nil {
		return "", true, fmt.Errorf("error rendering resolved target '%s': %w", target, err)
	}

	return out, true, nil
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

func (p *Partial) renderChildPartial(ctx context.Context, id string) (template.HTML, error) {
	p.mu.RLock()
	child, ok := p.children[id]
	p.mu.RUnlock()
	if !ok {
		p.getLogger().Warn("child partial not found", "id", id)
		return "", nil
	}

	// Clone the child partial to avoid modifying the original and prevent data races.
	childClone := child.clone()

	// Set the parent of the cloned child to the current partial.
	childClone.parent = p

	out, err := childClone.renderSelf(ctx, p.GetRequest())
	if err != nil {
		childClone.getLogger().Error("error rendering child partial", "id", id, "error", err)
		fallback, fallbackErr := childClone.renderErrorFragment(ctx, p.GetRequest(), err)
		if fallbackErr != nil {
			return "", fallbackErr
		}
		return fallback, nil
	}

	return out, nil
}

func (p *Partial) renderErrorFragment(ctx context.Context, r *http.Request, renderErr error) (template.HTML, error) {
	renderer := p.getErrorRenderer()
	if renderer == nil {
		return "", renderErr
	}

	out, err := renderer(withErrorFragmentContext(ctx), p, r, renderErr)
	if err != nil {
		return "", fmt.Errorf("error rendering fallback error fragment: %w; original render error: %v", err, renderErr)
	}

	return out, nil
}

// renderSelf renders this partial and its referenced template tree.
func (p *Partial) renderSelf(ctx context.Context, r *http.Request) (template.HTML, error) {
	if len(p.templates) == 0 {
		p.getLogger().Error("no templates provided for rendering")
		return "", errors.New("no templates provided for rendering")
	}

	var currentURL *url.URL
	if r != nil {
		currentURL = r.URL
	}

	dot, hasDot := p.getDotContract()

	locale := ""
	localizer := getLocalizer(ctx)
	if localizer != nil {
		locale = localizer.GetLocale()
	}
	state := &RenderContext{
		URL:      currentURL,
		BasePath: p.getBasePath(),
		Request:  r,
		Context:  ctx,
		Loc:      localizer,
		Locale:   locale,
		Csrf:     getCsrfToken(ctx),
	}
	templateRuntime := newRuntime(p, state)

	if p.action != nil {
		var err error
		p, err = p.action(ctx, p, templateRuntime)
		if err != nil {
			p.getLogger().Error("error in action function", "error", err)
			return "", fmt.Errorf("error in action function: %w", err)
		}
	}

	renderTemplates := p.renderTemplates()
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
		p.getLogger().Error("error getting or parsing template", "error", err)
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
		p.getLogger().Error("error executing template", "template", p.templates[0], "error", err)
		return "", fmt.Errorf("error executing template '%s': %w", p.templates[0], err)
	}

	return template.HTML(buf.String()), nil
}

func (p *Partial) renderOOBChildren(ctx context.Context, r *http.Request, renderOOB bool, isAncestor bool) (template.HTML, error) {
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
		childData, err := childClone.renderSelf(ctx, r)
		if err != nil {
			return "", fmt.Errorf("error rendering OOB region '%s': %w", id, err)
		}
		out += childData
	}

	return out, nil
}

func (p *Partial) renderAllAncestorOOBChildren(ctx context.Context, r *http.Request, renderOOB bool) (template.HTML, error) {
	var out template.HTML
	ancestor := p.parent
	for ancestor != nil {
		chunk, err := ancestor.renderOOBChildren(ctx, r, renderOOB, true)
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
	if tmpl, cached := store.templates.Load(cacheKey); cached && p.useCache {
		if entry, ok := tmpl.(*cachedTemplate); ok {
			return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
		}
	}

	muInterface, _ := store.mutexes.LoadOrStore(cacheKey, &sync.Mutex{})
	mu := muInterface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Double-check after acquiring lock
	if tmpl, cached := store.templates.Load(cacheKey); cached && p.useCache {
		if entry, ok := tmpl.(*cachedTemplate); ok {
			return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
		}
	}

	functions := funcs
	if !funcsAreFull {
		functions = mergeFuncMaps(p.getStaticFuncMap(), funcs)
	}
	parseFuncs := functions
	if p.useCache {
		parseFuncs = mergeFuncMaps(p.getStaticFuncMap(), placeholderRequestFuncMap())
	}
	t := template.New(path.Base(p.templates[0])).Funcs(parseFuncs)
	contracts, err := typedRootContractsFromFS(p.getFS(), renderTemplates)
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
	if err := addTemplatePathAliases(tmpl, renderTemplates); err != nil {
		return nil, nil, fmt.Errorf("error adding template path aliases: %w", err)
	}

	if p.useCache {
		requiredFuncs, err := requiredFuncsFromFS(p.getFS(), renderTemplates)
		if err != nil {
			return nil, nil, fmt.Errorf("error scanning template requirements: %w", err)
		}
		entry := &cachedTemplate{base: tmpl, requiredFuncs: requiredFuncs}
		store.templates.Store(cacheKey, entry)
		return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
	}

	return tmpl, nil, nil
}

func (p *Partial) registerContractsForExecution(tmpl *template.Template, renderTemplates []string) error {
	if tmpl == nil {
		return nil
	}
	contracts, err := typedRootContractsFromFS(p.getFS(), renderTemplates)
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

func validateRootContracts(contracts map[string]typedRootContract) error {
	for name := range contracts {
		if _, protected := coreFunctionNames[name]; protected {
			return fmt.Errorf("register contracts: %s conflicts with a go-partial template helper", name)
		}
	}
	return nil
}

func placeholderRootFuncMap(contracts map[string]typedRootContract) template.FuncMap {
	funcs := make(template.FuncMap, len(contracts))
	for name := range contracts {
		funcs[name] = func() any {
			return nil
		}
	}
	return funcs
}

func registerRootContracts(tmpl *template.Template, contracts map[string]typedRootContract, bindings []ContractInformation) error {
	funcs := make(template.FuncMap, len(contracts))
	for name, contract := range contracts {
		value, err := resolveContractValue(name, contract, bindings)
		if err != nil {
			return err
		}
		captured := value
		funcs[name] = func() any {
			return captured
		}
	}
	tmpl.Funcs(funcs)
	return nil
}

func resolveContractValue(name string, contract typedRootContract, bindings []ContractInformation) (any, error) {
	for _, binding := range bindings {
		if binding.Kind != "" && binding.Kind != ContractRoot {
			continue
		}
		if binding.Annotation != "" && binding.Annotation != contract.Annotation {
			continue
		}
		if binding.Name != name {
			continue
		}
		if !contractValueMatchesType(contract.Type, binding.Value) {
			return nil, fmt.Errorf("register contracts: @%s %s expects %s, got %s", contract.Annotation, name, contract.Type, contractValueTypeName(binding.Value))
		}
		return binding.Value, nil
	}

	var matches []any
	for _, binding := range bindings {
		if binding.Kind != "" && binding.Kind != ContractRoot {
			continue
		}
		if binding.Name != "" {
			continue
		}
		if binding.Annotation != "" && binding.Annotation != contract.Annotation {
			continue
		}
		if contractValueMatchesType(contract.Type, binding.Value) {
			matches = append(matches, binding.Value)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("register contracts: @%s %s has no matching value for %s", contract.Annotation, name, contract.Type)
	default:
		return nil, fmt.Errorf("register contracts: @%s %s has multiple matching values for %s; bind it by name", contract.Annotation, name, contract.Type)
	}
}

func contractValueMatchesType(contractType string, value any) bool {
	valueType := contractValueTypeName(value)
	if valueType == "" {
		return false
	}
	contractType = normalizeContractType(contractType)
	if valueType == contractType {
		return true
	}
	return strings.HasPrefix(valueType, "main.") && shortContractTypeName(valueType) == shortContractTypeName(contractType)
}

func contractValueTypeName(value any) string {
	if value == nil {
		return ""
	}
	typ := reflect.TypeOf(value)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Name() == "" || typ.PkgPath() == "" {
		return fmt.Sprintf("%T", value)
	}
	return typ.PkgPath() + "." + typ.Name()
}

func shortContractTypeName(typeName string) string {
	lastDot := strings.LastIndex(typeName, ".")
	if lastDot < 0 || lastDot == len(typeName)-1 {
		return typeName
	}
	return typeName[lastDot+1:]
}

func addTemplatePathAliases(tmpl *template.Template, names []string) error {
	if tmpl == nil {
		return nil
	}
	for _, name := range names {
		base := pathBase(name)
		if name == "" || name == base || tmpl.Lookup(base) == nil {
			continue
		}
		for _, alias := range templatePathAliases(name) {
			if tmpl.Lookup(alias) != nil {
				continue
			}
			if _, err := tmpl.New(alias).Parse(fmt.Sprintf(`{{ template %q . }}`, base)); err != nil {
				return err
			}
		}
	}
	return nil
}

func templatePathAliases(name string) []string {
	trimmed := strings.TrimLeft(name, `/\`)
	if trimmed == "" {
		return nil
	}
	aliases := []string{trimmed}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return aliases
	}
	return append(aliases, "/"+trimmed)
}

func (p *Partial) renderTemplates() []string {
	seen := make(map[string]struct{})
	refs := make(map[string]struct{})
	return p.collectRenderTemplates(seen, refs)
}

func (p *Partial) collectRenderTemplates(seen map[string]struct{}, refs map[string]struct{}) []string {
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
	maps.Copy(refs, referencedTemplatesFromFS(p.getFS(), p.templates))

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
		templates = append(templates, child.collectRenderTemplates(seen, refs)...)
	}

	return templates
}

func (p *Partial) matchesTemplateReference(refs map[string]struct{}) bool {
	if p == nil || len(refs) == 0 {
		return false
	}

	defined := definedTemplatesFromFS(p.getFS(), p.templates)
	for name := range defined {
		if _, ok := refs[name]; ok {
			return true
		}
	}
	return false
}

func (p *Partial) templateFromCacheEntry(entry *cachedTemplate, funcs template.FuncMap, applyFullFuncs bool, funcsAreFull bool) (*template.Template, func(), error) {
	functions := funcs
	if applyFullFuncs && !funcsAreFull {
		functions = mergeFuncMaps(p.getCustomFuncMap(), funcs)
	}
	return entry.template(functions)
}

func newTemplateStore() *templateStore {
	return &templateStore{}
}

func functionNameSignature(funcs template.FuncMap) string {
	names := make([]string, 0, len(funcs))
	for name := range funcs {
		names = append(names, name)
	}
	return functionNameSignatureFromNames(names)
}

func templateFuncSignature(funcs template.FuncMap) string {
	return mergeFunctionSignatures(functionNameSignature(funcs), functionNameSignatureFromSet(coreFunctionNames))
}

func functionNameSignatureFromNames(names []string) string {
	if len(names) == 0 {
		return ""
	}

	names = slices.Clone(names)
	sort.Strings(names)
	names = slices.Compact(names)

	var builder strings.Builder
	for _, name := range names {
		if name == "" {
			continue
		}
		builder.WriteString(name)
		builder.WriteString(";")
	}
	return builder.String()
}

func functionNameSignatureFromSet(names map[string]struct{}) string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	return functionNameSignatureFromNames(out)
}

func mergeFunctionSignatures(signatures ...string) string {
	var names []string
	for _, signature := range signatures {
		for _, name := range strings.Split(signature, ";") {
			if name == "" {
				continue
			}
			names = append(names, name)
		}
	}
	return functionNameSignatureFromNames(names)
}

func (p *Partial) getTemplateStore() *templateStore {
	if p.templateCache != nil {
		return p.templateCache
	}
	if p.parent != nil {
		return p.parent.getTemplateStore()
	}
	p.templateCache = newTemplateStore()
	return p.templateCache
}

func (c *cachedTemplate) template(functions template.FuncMap) (*template.Template, func(), error) {
	functions = filterFuncMap(functions, c.requiredFuncs)

	if pooled := c.pool.Get(); pooled != nil {
		t, ok := pooled.(*template.Template)
		if !ok {
			return nil, nil, fmt.Errorf("cached template pool contained %T", pooled)
		}
		if len(functions) > 0 {
			t.Funcs(functions)
		}
		return t, func() { c.pool.Put(t) }, nil
	}

	t, err := c.base.Clone()
	if err != nil {
		return nil, nil, fmt.Errorf("error cloning cached template: %w", err)
	}
	if len(functions) > 0 {
		t.Funcs(functions)
	}
	return t, func() { c.pool.Put(t) }, nil
}

func (p *Partial) clone() *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	clone := &Partial{
		id:              p.id,
		parent:          p.parent,
		request:         p.request,
		layoutContentID: p.layoutContentID,
		renderOOB:       p.renderOOB,
		alwaysSwapOOB:   p.alwaysSwapOOB,
		fs:              p.fs,
		logger:          p.logger,
		connector:       p.connector,
		useCache:        p.useCache,
		selection:       p.selection,
		targetResolver:  p.targetResolver,
		templates:       slices.Clone(p.templates),
		staticFuncs:     maps.Clone(p.staticFuncs),
		basePath:        p.basePath,
		contracts:       slices.Clone(p.contracts),
		responseHeaders: maps.Clone(p.responseHeaders),
		response:        p.response,
		errorRenderer:   p.errorRenderer,
		debugRenderer:   p.debugRenderer,
		templateCache:   p.templateCache,
		errorMode:       p.errorMode,
		errorModeSet:    p.errorModeSet,
		children:        maps.Clone(p.children),
		oobChildren:     maps.Clone(p.oobChildren),
		templateAction:  p.templateAction,
		action:          p.action,
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
