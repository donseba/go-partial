package partial

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
)

type (
	// RenderKind identifies the current render task for render stages.
	RenderKind string

	// RenderValues stores request-scoped values shared by render stages and helpers.
	RenderValues map[any]any

	// RenderResponse stores generic response metadata set by render stages.
	RenderResponse struct {
		Headers map[string]string
		Status  int
	}

	// RenderNext calls the next render stage in the chain.
	RenderNext func(*RenderContext) (template.HTML, error)

	renderResult struct {
		HTML     template.HTML
		Response *RenderResponse
		Headers  map[string]string
		// Vary names the request headers the out-of-band regions of the
		// response depend on; see SetOOBElementsFunc.
		Vary []string
		Err  error
	}

	// RenderStage observes or changes a render lifecycle.
	//
	// Prepare runs before the terminal template render and is the right place to
	// add request-scoped template funcs, resolve context values, or start timing.
	// Render wraps the terminal render and can replace or decorate the produced
	// HTML. Finalize runs after Render, even when Render returned an error, and is
	// the right place for cleanup, metrics, or error response shaping.
	RenderStage interface {
		Prepare(*RenderContext) (*RenderContext, error)
		Render(*RenderContext, RenderNext) (template.HTML, error)
		Finalize(*RenderContext, template.HTML, error) (template.HTML, error)
	}

	// RenderStageHooks adapts individual lifecycle functions to RenderStage.
	RenderStageHooks struct {
		PrepareFunc  func(*RenderContext) (*RenderContext, error)
		RenderFunc   func(*RenderContext, RenderNext) (template.HTML, error)
		FinalizeFunc func(*RenderContext, template.HTML, error) (template.HTML, error)
		// ResolveFunc gives templates the stage's functions by name, only
		// those a template calls; see TemplateFuncResolver.
		ResolveFunc func(ctx *RenderContext, name string) (fn any, ok bool)
		// FuncNames lists the template functions the stage sets with
		// RenderContext.SetFunc or gives with ResolveFunc, for
		// ReservedFuncNames; templates are parsed with stand-ins for them.
		FuncNames []string
		// FallbackFuncNames lists template functions the stage gives with
		// ResolveFunc only where the application registered no function
		// of that name with SetFunc: templates are parsed with stand-ins
		// for them, a registered function is called instead of the
		// stage's, and the names are not reserved, so registering one
		// raises no warning. Functions of the stage's own package, such as
		// a stand-in it offers for production, do not count as registered;
		// see Package. See TemplateFuncFallback.
		FallbackFuncNames []string
		// Package is the import path of the package that made the stage,
		// such as github.com/acme/app/tenant. Functions of that package,
		// such as the stand-ins its FuncMap offers for the names the stage
		// gives templates, are the stage's own: registering one raises no
		// func.protected warning and does not count as registered for
		// FallbackFuncNames. When Package is empty, the package is that of
		// the hook functions, which is the caller's where the compiler
		// inlines the function that returns the stage, so a package that
		// offers stand-ins sets it, for example to
		// reflect.TypeFor[T]().PkgPath() for a type T of its own.
		Package string
	}

	// TemplateFuncFallback is implemented by render stages that give
	// templates some functions only as a fallback for functions the
	// application did not register; see RenderStageHooks.FallbackFuncNames.
	TemplateFuncFallback interface {
		FallbackTemplateFuncNames() []string
	}

	// TemplateFuncResolver is implemented by render stages that give
	// templates functions bound to the render, such as one that reads the
	// request. go-partial asks the stages of a render for the functions a
	// template calls, by name, when the template is executed: a render whose
	// templates call none of them costs nothing, unlike functions set with
	// RenderContext.SetFunc in Prepare, which are made for every partial.
	// Functions set with SetFunc or SetFuncResolver come first; a later
	// stage is asked before an earlier one. The names must be known when
	// templates are parsed: templates get stand-ins for those the stage
	// declares with TemplateFuncNamer, as RenderStageHooks does with its
	// FuncNames; register stand-ins for others with Partial.SetFunc.
	// RenderStageHooks implements it with its ResolveFunc.
	TemplateFuncResolver interface {
		ResolveTemplateFunc(ctx *RenderContext, name string) (fn any, ok bool)
	}
)

const (
	// RenderKindPartial is the normal server-side partial render path.
	RenderKindPartial RenderKind = "partial"
	// RenderKindTarget is the core target render path selected by a connector.
	RenderKindTarget RenderKind = "target"
)

const (
	// renderKindError is private so core does not expose an error extension API.
	// ext/errors exports its own RenderKindError value for extension users.
	renderKindError RenderKind = "error"
)

// TemplateFuncNames returns the template functions the stage sets; see
// FuncNames and FallbackFuncNames.
func (h RenderStageHooks) TemplateFuncNames() []string {
	if len(h.FallbackFuncNames) == 0 {
		return h.FuncNames
	}

	names := slices.Concat(h.FuncNames, h.FallbackFuncNames)
	slices.Sort(names)

	return slices.Compact(names)
}

// FallbackTemplateFuncNames returns FallbackFuncNames; see
// TemplateFuncFallback.
func (h RenderStageHooks) FallbackTemplateFuncNames() []string {
	return h.FallbackFuncNames
}

// ResolveTemplateFunc returns the function ResolveFunc gives templates as
// name; see TemplateFuncResolver.
func (h RenderStageHooks) ResolveTemplateFunc(ctx *RenderContext, name string) (any, bool) {
	if h.ResolveFunc == nil {
		return nil, false
	}

	return h.ResolveFunc(ctx, name)
}

func (h RenderStageHooks) Prepare(ctx *RenderContext) (*RenderContext, error) {
	if h.PrepareFunc == nil {
		return ctx, nil
	}
	return h.PrepareFunc(ctx)
}

func (h RenderStageHooks) Render(ctx *RenderContext, next RenderNext) (template.HTML, error) {
	if h.RenderFunc == nil {
		return next(ctx)
	}
	return h.RenderFunc(ctx, next)
}

func (h RenderStageHooks) Finalize(ctx *RenderContext, out template.HTML, renderErr error) (template.HTML, error) {
	if h.FinalizeFunc == nil {
		return out, renderErr
	}
	return h.FinalizeFunc(ctx, out, renderErr)
}

// renderTemplateTerminal ends the render chain of a partial: it executes
// the partial's templates.
func renderTemplateTerminal(ctx *RenderContext) (template.HTML, error) {
	if ctx == nil || ctx.Partial == nil {
		return "", fmt.Errorf("template stage requires a partial")
	}
	return renderTemplate(ctx)
}

func (v RenderValues) Set(key any, value any) {
	if v == nil {
		return
	}
	v[key] = value
}

func (v RenderValues) Get(key any) any {
	if v == nil {
		return nil
	}
	return v[key]
}

func (v RenderValues) Clone() RenderValues {
	if len(v) == 0 {
		return make(RenderValues)
	}
	out := make(RenderValues, len(v))
	for key, value := range v {
		out[key] = value
	}
	return out
}

// OmitOOB answers the partial request being rendered with the target
// alone, without the out-of-band regions of its ancestors. Render stages
// that resolve the target of a request, such as exp/slots for a standalone
// slot, call it in Prepare for targets that are requested on their own.
func (ctx *RenderContext) OmitOOB() {
	if ctx == nil {
		return
	}

	ctx.omitOOB = true
}

// TargetResolved reports whether a render stage resolved the target of the
// partial request being rendered, replacing Partial in Prepare: a later
// stage that resolves targets leaves it as it is.
func (ctx *RenderContext) TargetResolved() bool {
	if ctx == nil || ctx.Kind != RenderKindTarget || ctx.Partial == nil {
		return false
	}

	return ctx.runtime.partial != nil && ctx.Partial != ctx.runtime.partial
}

func (ctx *RenderContext) SetFunc(name string, fn any) {
	if ctx == nil || name == "" || fn == nil {
		return
	}
	if ctx.Funcs == nil {
		ctx.Funcs = make(template.FuncMap)
	}
	ctx.Funcs[name] = fn
}

// SetFuncResolver gives templates functions that are looked up by name, only
// for the functions a template calls. It suits stages that offer many
// functions, of which a template calls a few. Functions set with SetFunc come
// first; a later resolver is asked before an earlier one. As with SetFunc, the
// names must be known when templates are parsed: register stand-ins for them
// with Partial.SetFunc.
func (ctx *RenderContext) SetFuncResolver(resolve func(name string) (fn any, ok bool)) {
	if ctx == nil || resolve == nil {
		return
	}
	ctx.funcResolvers = append(ctx.funcResolvers, resolve)
}

func (ctx *RenderContext) hasFuncResolvers() bool {
	return ctx != nil && (len(ctx.funcResolvers) > 0 || ctx.stageResolvers)
}

func (ctx *RenderContext) resolveFunc(name string) (any, bool) {
	if ctx == nil {
		return nil, false
	}

	for i := len(ctx.funcResolvers) - 1; i >= 0; i-- {
		if fn, ok := ctx.funcResolvers[i](name); ok && fn != nil {
			return fn, true
		}
	}

	if !ctx.stageResolvers {
		return nil, false
	}

	for i := len(ctx.stages) - 1; i >= 0; i-- {
		resolver, ok := ctx.stages[i].(TemplateFuncResolver)
		if !ok {
			continue
		}

		if ctx.registeredOver(ctx.stages[i], name) {
			continue
		}

		if fn, ok := resolver.ResolveTemplateFunc(ctx, name); ok && fn != nil {
			return fn, true
		}
	}

	return nil, false
}

// ResolveFunc returns the function name as the render gives it to
// templates through functions set with SetFunc and SetFuncResolver and
// through its render stages (see TemplateFuncResolver), such as the
// function of another stage, from a Render hook: the stages of the render
// are known once every stage has prepared it. Functions registered on the
// partial with Partial.SetFunc are not looked up.
func (ctx *RenderContext) ResolveFunc(name string) (any, bool) {
	if ctx == nil || name == "" {
		return nil, false
	}

	if fn, ok := ctx.Funcs[name]; ok && fn != nil {
		return fn, true
	}

	return ctx.resolveFunc(name)
}

// registeredOver reports whether the stage gives name only as a fallback
// (see RenderStageHooks.FallbackFuncNames) and the partial being rendered
// has a function of that name registered with SetFunc, which then wins.
func (ctx *RenderContext) registeredOver(stage RenderStage, name string) bool {
	fallback, ok := stage.(TemplateFuncFallback)
	if !ok || ctx.Partial == nil {
		return false
	}

	names := fallback.FallbackTemplateFuncNames()
	if len(names) == 0 || !slices.Contains(names, name) {
		return false
	}

	fn, registered := ctx.Partial.Func(name)

	return registered && !standsInFor(name, fn, []RenderStage{stage})
}

// FailureResponse reports, in an error render (see ext/errors), whether
// the failure response answers the request as a whole, as Write writes
// it, rather than taking the place of a failing part of a page.
func (ctx *RenderContext) FailureResponse() bool {
	return ctx != nil && ctx.failureResponse
}

// FailureStatus is, in the error render of WriteError, which answers an
// error that happened outside a render, the status the response is
// written with, such as 500 or 429; it is 0 for a render that failed.
func (ctx *RenderContext) FailureStatus() int {
	if ctx == nil {
		return 0
	}

	return ctx.failureStatus
}

// resolveFuncs puts the resolvers' functions into funcs, a map of every
// function, for renders without the template cache: those of the names in
// funcs, and those the stages declare (see TemplateFuncNamer), as SetFunc
// in Prepare would. go-partial's own helpers and functions set with SetFunc
// keep precedence.
func (ctx *RenderContext) resolveFuncs(funcs template.FuncMap) {
	if !ctx.hasFuncResolvers() {
		return
	}

	for name := range funcs {
		ctx.resolveInto(funcs, name)
	}

	if !ctx.stageResolvers {
		return
	}

	for _, stage := range ctx.stages {
		namer, ok := stage.(TemplateFuncNamer)
		if !ok || !resolvesFuncs(stage) {
			continue
		}

		for _, name := range namer.TemplateFuncNames() {
			ctx.resolveInto(funcs, name)
		}
	}
}

// resolveInto sets funcs[name] to the function a resolver gives for name,
// unless name is one of go-partial's helpers or set with SetFunc.
func (ctx *RenderContext) resolveInto(funcs template.FuncMap, name string) {
	if _, core := coreFunctionNames[name]; core {
		return
	}

	if _, set := ctx.Funcs[name]; set {
		return
	}

	if fn, ok := ctx.resolveFunc(name); ok {
		funcs[name] = fn
	}
}

// resolvesFuncs reports whether the stage gives templates functions; see
// TemplateFuncResolver.
func resolvesFuncs(stage RenderStage) bool {
	switch hooks := stage.(type) {
	case RenderStageHooks:
		return hooks.ResolveFunc != nil
	case *RenderStageHooks:
		return hooks != nil && hooks.ResolveFunc != nil
	}

	_, ok := stage.(TemplateFuncResolver)

	return ok
}

// rendersThrough reports whether the stage's Render only calls the next
// stage, so the chain can leave it out.
func rendersThrough(stage RenderStage) bool {
	switch hooks := stage.(type) {
	case RenderStageHooks:
		return hooks.RenderFunc == nil
	case *RenderStageHooks:
		return hooks != nil && hooks.RenderFunc == nil
	}

	return false
}

func newRenderContext(ctx context.Context, p *Partial, r *http.Request, kind RenderKind) *RenderContext {
	if ctx == nil {
		if r != nil {
			ctx = r.Context()
		} else {
			ctx = defaultRenderContext()
		}
	}

	var currentURL *url.URL
	if r != nil {
		currentURL = r.URL
	}

	state := &RenderContext{
		URL:      currentURL,
		BasePath: p.getBasePath(),
		Request:  r,
		Context:  ctx,
		Partial:  p,
		Kind:     kind,
		Values:   make(RenderValues),
		Funcs:    make(template.FuncMap),
		Events:   mergeEventSinks(p.getEvents(), EventSinkFromContext(ctx)),
	}
	state.response = RenderResponse{Headers: make(map[string]string), Status: p.getStatus()}
	state.Response = &state.response
	state.runtime = Runtime{partial: p, state: state}
	state.Runtime = &state.runtime
	return state
}

func defaultRenderContext() context.Context {
	return context.TODO()
}

func renderWithChain(state *RenderContext, stages []RenderStage, terminal RenderNext) (template.HTML, error) {
	result := renderWithChainResult(state, stages, terminal)
	return result.HTML, result.Err
}

func renderWithChainResult(state *RenderContext, stages []RenderStage, terminal RenderNext) renderResult {
	if state == nil {
		return renderResult{Err: fmt.Errorf("render context is not configured")}
	}
	if terminal == nil {
		return renderResult{Response: state.Response, Err: fmt.Errorf("terminal render stage is not configured")}
	}

	active := activeStages(stages)

	var err error
	for _, stage := range active {
		response := state.Response
		state, err = stage.Prepare(state)
		if err != nil {
			if state != nil {
				response = state.Response
			}
			return renderResult{Response: response, Err: err}
		}
		if state == nil {
			return renderResult{Err: fmt.Errorf("render stage prepare returned nil context")}
		}
		if state.Response == nil {
			state.Response = &RenderResponse{Headers: make(map[string]string)}
		}
	}

	// Templates ask the stages for the functions they call; see
	// TemplateFuncResolver.
	state.stages = active
	state.stageResolvers = slices.ContainsFunc(active, resolvesFuncs)

	next := renderChain(active, terminal)

	state.Emit(Event{
		Kind:    EventRenderStart,
		Level:   EventDebug,
		Message: "render started",
	})
	out, renderErr := next(state)
	for i := len(active) - 1; i >= 0; i-- {
		out, renderErr = active[i].Finalize(state, out, renderErr)
	}
	if renderErr != nil {
		state.Emit(Event{
			Kind:    EventRenderError,
			Level:   EventError,
			Message: "render failed",
			Error:   renderErr,
		})
	} else {
		state.Emit(Event{
			Kind:    EventRenderFinish,
			Level:   EventDebug,
			Message: "render finished",
			Fields:  map[string]any{"size": len([]byte(out))},
		})
	}

	return renderResult{HTML: out, Response: state.Response, Err: renderErr}
}

// activeStages returns stages without the nil ones; stages itself when it
// has none, as is usual.
func activeStages(stages []RenderStage) []RenderStage {
	if !slices.Contains(stages, nil) {
		return stages
	}

	active := make([]RenderStage, 0, len(stages))
	for _, stage := range stages {
		if stage != nil {
			active = append(active, stage)
		}
	}

	return active
}

// renderChain returns the Render hooks of stages as one function that ends
// with terminal, the first stage's hook first. Stages without a Render hook
// only call the next one: the chain leaves them out.
func renderChain(stages []RenderStage, terminal RenderNext) RenderNext {
	next := terminal

	for i := len(stages) - 1; i >= 0; i-- {
		stage := stages[i]
		if rendersThrough(stage) {
			continue
		}

		previous := next
		next = func(ctx *RenderContext) (template.HTML, error) {
			return stage.Render(ctx, previous)
		}
	}

	return next
}
