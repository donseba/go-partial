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
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/donseba/go-partial/connector"
)

var (
	// protectedFunctionNames is a set of function names that are protected from being overridden
	protectedFunctionNames = map[string]struct{}{
		"action":          {},
		"async":           {},
		"and":             {},
		"child":           {},
		"childIf":         {},
		"context":         {},
		"dict":            {},
		"debug":           {},
		"eq":              {},
		"ge":              {},
		"gt":              {},
		"html":            {},
		"index":           {},
		"js":              {},
		"joinPath":        {},
		"island":          {},
		"le":              {},
		"len":             {},
		"lt":              {},
		"ne":              {},
		"not":             {},
		"on":              {},
		"or":              {},
		"oob":             {},
		"oobAttr":         {},
		"partial":         {},
		"poll":            {},
		"prefetch":        {},
		"print":           {},
		"printf":          {},
		"println":         {},
		"refresh":         {},
		"reveal":          {},
		"slice":           {},
		"stream":          {},
		"urlquery":        {},
		"actionHeader":    {},
		"actionIs":        {},
		"actionValue":     {},
		"selectionHeader": {},
		"selectionIs":     {},
		"selectionValue":  {},
		"targetHeader":    {},
		"targetIs":        {},
		"targetValue":     {},
		"scoped":          {},
		"selection":       {},
		"url":             {},
		"urlContains":     {},
		"urlIs":           {},
		"urlPath":         {},
		"urlStarts":       {},
	}

	requestFuncSignature = functionNameSignatureFromNames([]string{
		"action",
		"actionHeader",
		"actionIs",
		"actionValue",
		"async",
		"child",
		"childIf",
		"context",
		"debug",
		"island",
		"joinPath",
		"on",
		"oob",
		"oobAttr",
		"partial",
		"poll",
		"prefetch",
		"refresh",
		"reveal",
		"scoped",
		"selection",
		"selectionHeader",
		"selectionIs",
		"selectionValue",
		"stream",
		"targetHeader",
		"targetIs",
		"targetValue",
		"url",
		"urlContains",
		"urlIs",
		"urlPath",
		"urlStarts",
	})
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

	// Partial represents a renderable component with optional children and data.
	Partial struct {
		id                    string
		parent                *Partial
		request               *http.Request
		renderOOB             bool
		alwaysSwapOOB         bool
		fs                    fs.FS
		logger                Logger
		connector             connector.Connector
		useCache              bool
		templates             []string
		staticFuncs           template.FuncMap
		customFuncs           template.FuncMap
		templateFuncSignature string
		hasCustomFunctions    bool
		basePath              string
		data                  map[string]any
		layoutData            map[string]any
		globalData            map[string]any
		serviceData           map[string]any
		interact              map[string]any
		responseHeaders       map[string]string
		response              connector.Response
		errorRenderer         ErrorRenderer
		debugRenderer         DebugRenderer
		interactionRenderer   InteractionRenderer
		templateCache         *templateStore
		errorMode             ErrorMode
		errorModeSet          bool
		targetResolver        TargetResolver
		mu                    sync.RWMutex
		children              map[string]*Partial
		oobChildren           map[string]struct{}
		selection             *Selection
		templateAction        func(ctx context.Context, p *Partial, data *Data) (*Partial, error)
		action                func(ctx context.Context, p *Partial, data *Data) (*Partial, error)
	}

	Selection struct {
		Partials map[string]*Partial
		Default  string
	}

	// Data represents the data available to the partial.
	Data struct {
		// Ctx is the render context.
		Ctx context.Context
		// URL is the request URL.
		URL *url.URL
		// Request contains the http.Request.
		Request *http.Request
		// Data contains data for the current partial.
		Data map[string]any
		// Interact contains client interaction declarations for the current partial.
		Interact map[string]any
		// Service contains data configured on the Service.
		Service map[string]any
		// Layout contains data configured on the current Layout.
		Layout map[string]any
		// Global contains data inherited through the partial tree.
		Global map[string]any
		// Parent contains the immediate parent partial's data.
		Parent map[string]any
		// Loc contains the request localizer.
		Loc Localizer
		// Csrf contains the request CSRF token.
		Csrf CsrfToken
		// BasePath is the base path of the partial.
		BasePath string
	}

	// GlobalData represents the global data available to all partials.
	GlobalData map[string]any

	TargetResolver func(ctx context.Context, r *http.Request, target string) (*Partial, map[string]any, bool)

	ErrorRenderer func(ctx context.Context, p *Partial, r *http.Request, err error) (template.HTML, error)

	DebugRenderer func(ctx context.Context, p *Partial, data *Data, value any) (template.HTML, error)

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
)

const (
	ErrorModeSafe ErrorMode = iota
	ErrorModeDetailed
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
		id:                    "root",
		templates:             templates,
		staticFuncs:           functions,
		customFuncs:           make(template.FuncMap),
		templateFuncSignature: templateFuncSignature(functions),
		data:                  make(map[string]any),
		layoutData:            make(map[string]any),
		globalData:            make(map[string]any),
		serviceData:           make(map[string]any),
		interact:              make(map[string]any),
		children:              make(map[string]*Partial),
		oobChildren:           make(map[string]struct{}),
		fs:                    os.DirFS("./"),
		errorRenderer:         DefaultErrorRenderer(),
		debugRenderer:         DefaultDebugRenderer(),
		interactionRenderer:   DefaultInteractionRenderer(),
		templateCache:         newTemplateStore(),
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
	return func(ctx context.Context, p *Partial, data *Data, value any) (template.HTML, error) {
		debugData := DebugData{
			Value:  value,
			Output: formatDebugValue(value),
		}
		if p != nil {
			debugData.PartialID = p.id
			debugData.Templates = slices.Clone(p.templates)
		}
		if data != nil {
			debugData.Request = data.Request
			debugData.URL = data.URL
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
	p.serviceData = make(map[string]any)
	p.interact = make(map[string]any)
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

func (p *Partial) SetInteractionRenderer(renderer InteractionRenderer) *Partial {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.interactionRenderer = renderer

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

// SetInteractions sets client interaction declarations for the partial.
func (p *Partial) SetInteractions(interactions map[string]any) *Partial {
	if p == nil {
		return nil
	}
	p.interact = interactions
	if p.interact == nil {
		p.interact = make(map[string]any)
	}
	return p
}

// AddInteraction adds one client interaction declaration for the partial.
func (p *Partial) AddInteraction(name string, interaction any) *Partial {
	if p == nil {
		return nil
	}
	if p.interact == nil {
		p.interact = make(map[string]any)
	}
	p.interact[name] = interaction
	return p
}

func (p *Partial) SetResponseHeaders(headers map[string]string) *Partial {
	if p.parent != nil {
		p.parent.SetResponseHeaders(headers)
	}

	p.responseHeaders = headers
	return p
}

func (p *Partial) GetResponseHeaders() map[string]string {
	if p == nil {
		return nil
	}

	if p.responseHeaders == nil {
		if p.parent != nil {
			return maps.Clone(p.parent.GetResponseHeaders())
		}
		return nil
	}

	return maps.Clone(p.responseHeaders)
}

func (p *Partial) Response() *connector.ResponseBuilder {
	return connector.NewResponseBuilder(&p.response)
}

func (p *Partial) SetResponse(response connector.Response) *Partial {
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
	p.connector = connector
	return p
}

// MergeData merges the data into the partial.
func (p *Partial) MergeData(data map[string]any, override bool) *Partial {
	if p.data == nil {
		p.data = make(map[string]any)
	}
	if override {
		maps.Copy(p.data, data)
		return p
	}
	for k, v := range data {
		if _, ok := p.data[k]; ok {
			continue
		}

		p.data[k] = v
	}
	return p
}

func (p *Partial) SetAlwaysSwapOOB(alwaysSwapOOB bool) *Partial {
	p.alwaysSwapOOB = alwaysSwapOOB
	return p
}

// UseFuncs adds template functions to the Partial.
func (p *Partial) UseFuncs(funcMap template.FuncMap) {
	p.mu.Lock()
	defer p.mu.Unlock()

	customFuncs := make(template.FuncMap, len(funcMap))
	for k, v := range funcMap {
		if isProtectedFunctionName(k) {
			p.getLogger().Warn("function name is protected and cannot be overwritten", "function", k)
			continue
		}

		p.staticFuncs[k] = v
		customFuncs[k] = v
	}
	if len(customFuncs) > 0 {
		p.hasCustomFunctions = true
	}
	if p.customFuncs == nil {
		p.customFuncs = make(template.FuncMap)
	}
	maps.Copy(p.customFuncs, customFuncs)
	p.templateFuncSignature = templateFuncSignature(p.staticFuncs)
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

// UseTemplateCache sets the parsed template cache usage flag for the partial.
func (p *Partial) UseTemplateCache(useCache bool) *Partial {
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
	p.children[child.id].serviceData = p.serviceData
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

// WithTargetResolver maps dynamic DOM targets to a reusable partial and render data.
func (p *Partial) WithTargetResolver(resolver TargetResolver) *Partial {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.targetResolver = resolver

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
			p.getLogger().Error("error rendering OOB children for fallback error template", "error", oobErr)
			return fmt.Errorf("error rendering OOB children for fallback error template: %w; original render error: %v", oobErr, renderErr)
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
	if len(customFuncMap) > 0 {
		p.hasCustomFunctions = true
		if p.customFuncs == nil {
			p.customFuncs = make(template.FuncMap, len(customFuncMap))
		}
		maps.Copy(p.customFuncs, customFuncMap)
	}
	p.templateFuncSignature = templateFuncSignature(p.staticFuncs)
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

	if p.parent != nil {
		funcs := maps.Clone(p.parent.getCustomFuncMap())
		maps.Copy(funcs, p.customFuncs)
		return funcs
	}

	return maps.Clone(p.customFuncs)
}

func (p *Partial) getFunctionSignature() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	signature := p.templateFuncSignature
	if p.parent != nil {
		signature = mergeFunctionSignatures(p.parent.getFunctionSignature(), signature)
	}
	return signature
}

func (p *Partial) getHasCustomFunctions() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.hasCustomFunctions {
		return true
	}
	if p.parent != nil {
		return p.parent.getHasCustomFunctions()
	}
	return false
}

func (p *Partial) getRequestFuncMap(data *Data) template.FuncMap {
	funcs := make(template.FuncMap, 40)
	p.addRequestFuncs(funcs, data)
	return funcs
}

func (p *Partial) addRequestFuncs(funcs template.FuncMap, data *Data) {
	funcs["child"] = childFunc(p, data)
	funcs["childIf"] = childIfFunc(p, data)
	funcs["partial"] = partialFunc(p, data)
	funcs["async"] = interactionFunc(p, data, connector.InteractionAsync)
	funcs["reveal"] = interactionFunc(p, data, connector.InteractionReveal)
	funcs["poll"] = interactionFunc(p, data, connector.InteractionPoll)
	funcs["stream"] = interactionFunc(p, data, connector.InteractionStream)
	funcs["prefetch"] = interactionFunc(p, data, connector.InteractionPrefetch)
	funcs["refresh"] = interactionFunc(p, data, connector.InteractionRefresh)
	funcs["on"] = onFunc(p, data)
	funcs["island"] = islandFunc(p, data)
	funcs["selection"] = selectionFunc(p, data)
	funcs["action"] = actionFunc(p, data)
	funcs["debug"] = debugFunc(p, data)

	funcs["url"] = func() *url.URL {
		return data.URL
	}

	funcs["urlIs"] = func(current string) bool {
		if data.URL == nil {
			return false
		}
		return strings.Trim(data.URL.Path, "/") == strings.Trim(current, "/")
	}

	funcs["urlStarts"] = func(current string) bool {
		if data.URL == nil {
			return false
		}
		return strings.HasPrefix(data.URL.Path, current)
	}

	funcs["urlContains"] = func(current string) bool {
		if data.URL == nil {
			return false
		}
		return strings.Contains(data.URL.Path, current)
	}

	funcs["joinPath"] = func(parts ...string) string {
		return path.Join(parts...)
	}

	funcs["urlPath"] = func(base string, parts ...string) template.URL {
		allParts := append([]string{base}, parts...)
		return template.URL(path.Join(allParts...))
	}

	funcs["context"] = func() context.Context {
		return data.Ctx
	}

	funcs["scoped"] = func() map[string]any {
		return copyDataMap(data.Data)
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
		"child":           func(string, ...any) template.HTML { return "" },
		"childIf":         func(string, ...any) template.HTML { return "" },
		"partial":         func(string, ...any) template.HTML { return "" },
		"async":           func(any, ...any) template.HTML { return "" },
		"reveal":          func(any, ...any) template.HTML { return "" },
		"poll":            func(any, ...any) template.HTML { return "" },
		"stream":          func(any, ...any) template.HTML { return "" },
		"prefetch":        func(any, ...any) template.HTML { return "" },
		"refresh":         func(any, ...any) template.HTML { return "" },
		"on":              func(any, ...any) template.HTML { return "" },
		"island":          func(any, ...any) template.HTML { return "" },
		"selection":       func() template.HTML { return "" },
		"action":          func() template.HTML { return "" },
		"debug":           func(any) template.HTML { return "" },
		"url":             func() *url.URL { return nil },
		"urlIs":           func(string) bool { return false },
		"urlStarts":       func(string) bool { return false },
		"urlContains":     func(string) bool { return false },
		"joinPath":        func(...string) string { return "" },
		"urlPath":         func(string, ...string) template.URL { return "" },
		"context":         func() context.Context { return nil },
		"scoped":          func() map[string]any { return nil },
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

func addMissingFuncs(funcs template.FuncMap, defaults template.FuncMap) {
	for name, fn := range defaults {
		if _, ok := funcs[name]; ok {
			continue
		}
		funcs[name] = fn
	}
}

func isProtectedFunctionName(name string) bool {
	if _, ok := protectedFunctionNames[name]; ok {
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

func copyDataMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return maps.Clone(in)
}

func (p *Partial) getGlobalData() map[string]any {
	if p.parent != nil {
		return p.parent.getGlobalDataMergedWithOwn()
	}
	return p.globalData
}

func (p *Partial) getGlobalDataMergedWithOwn() map[string]any {
	parentGlobal := map[string]any{}
	if p.parent != nil {
		parentGlobal = p.parent.getGlobalDataMergedWithOwn()
	}
	merged := maps.Clone(parentGlobal)
	maps.Copy(merged, p.data)
	return merged
}

func (p *Partial) getLayoutData() map[string]any {
	if p.parent != nil {
		layoutData := p.parent.getLayoutData()
		merged := maps.Clone(layoutData)
		maps.Copy(merged, p.layoutData)
		return merged
	}
	return p.layoutData
}

func (p *Partial) getServiceData() map[string]any {
	if p.parent != nil {
		serviceData := p.parent.getServiceData()
		merged := maps.Clone(serviceData)
		maps.Copy(merged, p.serviceData)
		return merged
	}
	return p.serviceData
}

func (p *Partial) getParentData() map[string]any {
	if p.parent != nil {
		return maps.Clone(p.parent.data)
	}
	return nil
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

	// Cache the default logger in p.logger
	p.logger = slog.Default().WithGroup("partial")

	return p.logger
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

func (p *Partial) getInteractionRenderer() InteractionRenderer {
	if p == nil {
		return DefaultInteractionRenderer()
	}

	if p.interactionRenderer != nil {
		return p.interactionRenderer
	}

	if p.parent != nil {
		return p.parent.getInteractionRenderer()
	}

	return DefaultInteractionRenderer()
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
	th := p.getConnector().GetTargetValue(p.GetRequest())
	if th != "" {
		return th
	}
	if p.parent != nil {
		return p.parent.GetRequestTargetValue()
	}
	return ""
}

func (p *Partial) GetRequestActionValue() string {
	ah := p.getConnector().GetActionValue(p.GetRequest())
	if ah != "" {
		return ah
	}
	if p.parent != nil {
		return p.parent.GetRequestActionValue()
	}
	return ""
}

func (p *Partial) GetRequestSelectionValue() string {
	as := p.getConnector().GetSelectValue(p.GetRequest())
	if as != "" {
		return as
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

		// Render OOB children of parent if necessary
		oobOutAll, oobErr := p.renderAllAncestorOOBChildren(ctx, r, true)
		if oobErr != nil {
			p.getLogger().Error("error rendering OOB children from ancestors", "error", oobErr)
			return "", fmt.Errorf("error rendering OOB children from ancestors: %w", oobErr)
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
					p.getLogger().Error("error rendering OOB children from ancestors", "error", oobErr)
					return "", fmt.Errorf("error rendering OOB children from ancestors: %w", oobErr)
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

	resolvedPartial, data, ok := resolver(ctx, r, target)
	if !ok || resolvedPartial == nil {
		return "", false, nil
	}

	resolvedClone := resolvedPartial.clone()
	resolvedClone.parent = p
	if data != nil {
		resolvedClone.MergeData(data, true)
	}

	out, err := resolvedClone.renderSelf(ctx, r)
	if err != nil {
		return "", true, fmt.Errorf("error rendering resolved target '%s': %w", target, err)
	}

	return out, true, nil
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
		URL:      currentURL,
		BasePath: p.getBasePath(),
		Request:  r,
		Ctx:      ctx,
		Data:     p.data,
		Interact: p.interact,
		Global:   p.getGlobalData(),
		Service:  p.getServiceData(),
		Layout:   p.getLayoutData(),
		Parent:   p.getParentData(),
		Loc:      getLocalizer(ctx),
		Csrf:     getCsrfToken(ctx),
	}

	if p.action != nil {
		var err error
		p, err = p.action(ctx, p, data)
		if err != nil {
			p.getLogger().Error("error in action function", "error", err)
			return "", fmt.Errorf("error in action function: %w", err)
		}
	}

	cacheKey := p.generateCacheKey(p.templates, p.getFunctionSignature())
	var funcs template.FuncMap
	if p.useCache {
		funcs = p.getRequestFuncMap(data)
	} else {
		funcs = p.getStaticFuncMap()
		p.addRequestFuncs(funcs, data)
	}

	tmpl, releaseTemplate, err := p.getTemplateForRender(cacheKey, funcs, p.getHasCustomFunctions(), !p.useCache)
	if err != nil {
		p.getLogger().Error("error getting or parsing template", "error", err)
		return "", err
	}
	if releaseTemplate != nil {
		defer releaseTemplate()
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, data); err != nil {
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
			return "", fmt.Errorf("error rendering OOB child '%s': %w", id, err)
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
			return "", fmt.Errorf("error rendering OOB children from ancestor '%s': %w", ancestor.id, err)
		}
		out += chunk
		ancestor = ancestor.parent
	}
	return out, nil
}

func (p *Partial) getTemplateForRender(cacheKey string, funcs template.FuncMap, applyFullFuncs bool, funcsAreFull bool) (*template.Template, func(), error) {
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
	tmpl, err := t.ParseFS(p.getFS(), p.templates...)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing templates: %w", err)
	}

	if p.useCache {
		requiredFuncs, err := requiredFuncsFromFS(p.getFS(), p.templates)
		if err != nil {
			return nil, nil, fmt.Errorf("error scanning template requirements: %w", err)
		}
		entry := &cachedTemplate{base: tmpl, requiredFuncs: requiredFuncs}
		store.templates.Store(cacheKey, entry)
		return p.templateFromCacheEntry(entry, funcs, applyFullFuncs, funcsAreFull)
	}

	return tmpl, nil, nil
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
	return mergeFunctionSignatures(functionNameSignature(funcs), requestFuncSignature)
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

	// Create a new Partial instance
	clone := &Partial{
		id:                    p.id,
		parent:                p.parent,
		request:               p.request,
		renderOOB:             p.renderOOB,
		alwaysSwapOOB:         p.alwaysSwapOOB,
		fs:                    p.fs,
		logger:                p.logger,
		connector:             p.connector,
		useCache:              p.useCache,
		selection:             p.selection,
		targetResolver:        p.targetResolver,
		templates:             slices.Clone(p.templates),
		staticFuncs:           maps.Clone(p.staticFuncs),
		customFuncs:           maps.Clone(p.customFuncs),
		templateFuncSignature: p.templateFuncSignature,
		hasCustomFunctions:    p.hasCustomFunctions,
		basePath:              p.basePath,
		data:                  maps.Clone(p.data),
		layoutData:            maps.Clone(p.layoutData),
		globalData:            maps.Clone(p.globalData),
		serviceData:           maps.Clone(p.serviceData),
		interact:              maps.Clone(p.interact),
		response:              p.response,
		errorRenderer:         p.errorRenderer,
		debugRenderer:         p.debugRenderer,
		interactionRenderer:   p.interactionRenderer,
		templateCache:         p.templateCache,
		errorMode:             p.errorMode,
		errorModeSet:          p.errorModeSet,
		children:              maps.Clone(p.children),
		oobChildren:           maps.Clone(p.oobChildren),
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
