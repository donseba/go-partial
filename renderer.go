package partial

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
)

type (
	// RenderKind identifies the current render task for renderer hooks.
	RenderKind string

	// RenderValues stores request-scoped values shared by renderers and helpers.
	RenderValues map[any]any

	// RenderResponse stores generic response metadata set by renderers.
	RenderResponse struct {
		Headers map[string]string
		Status  int
	}

	// RenderNext calls the next renderer in the chain.
	RenderNext func(*RenderContext) (template.HTML, error)

	renderResult struct {
		HTML     template.HTML
		Response *RenderResponse
		Headers  map[string]string
		Err      error
	}

	// Renderer observes or changes a render lifecycle.
	//
	// Prepare runs before the terminal template render and is the right place to
	// add request-scoped template funcs, resolve context values, or start timing.
	// Render wraps the terminal render and can replace or decorate the produced
	// HTML. Finalize runs after Render, even when Render returned an error, and is
	// the right place for cleanup, metrics, or error response shaping.
	Renderer interface {
		Prepare(*RenderContext) (*RenderContext, error)
		Render(*RenderContext, RenderNext) (template.HTML, error)
		Finalize(*RenderContext, template.HTML, error) (template.HTML, error)
	}

	// RendererHooks adapts individual lifecycle functions to Renderer.
	RendererHooks struct {
		PrepareFunc  func(*RenderContext) (*RenderContext, error)
		RenderFunc   func(*RenderContext, RenderNext) (template.HTML, error)
		FinalizeFunc func(*RenderContext, template.HTML, error) (template.HTML, error)
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

func (h RendererHooks) Prepare(ctx *RenderContext) (*RenderContext, error) {
	if h.PrepareFunc == nil {
		return ctx, nil
	}
	return h.PrepareFunc(ctx)
}

func (h RendererHooks) Render(ctx *RenderContext, next RenderNext) (template.HTML, error) {
	if h.RenderFunc == nil {
		return next(ctx)
	}
	return h.RenderFunc(ctx, next)
}

func (h RendererHooks) Finalize(ctx *RenderContext, out template.HTML, renderErr error) (template.HTML, error) {
	if h.FinalizeFunc == nil {
		return out, renderErr
	}
	return h.FinalizeFunc(ctx, out, renderErr)
}

func templateRenderer() Renderer {
	return RendererHooks{
		RenderFunc: func(ctx *RenderContext, next RenderNext) (template.HTML, error) {
			if ctx == nil || ctx.Partial == nil {
				return "", fmt.Errorf("template renderer requires a partial")
			}
			return ctx.Partial.renderTemplate(ctx)
		},
	}
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

func (ctx *RenderContext) SetFunc(name string, fn any) {
	if ctx == nil || name == "" || fn == nil {
		return
	}
	if ctx.Funcs == nil {
		ctx.Funcs = make(template.FuncMap)
	}
	ctx.Funcs[name] = fn
}

func newRenderContext(ctx context.Context, p *Partial, r *http.Request, kind RenderKind) *RenderContext {
	if ctx == nil {
		ctx = context.Background()
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
		Response: &RenderResponse{Headers: make(map[string]string)},
		Funcs:    make(template.FuncMap),
		Events:   mergeEventSinks(p.getEventSink(), EventSinkFromContext(ctx)),
	}
	state.Runtime = newRuntime(p, state)
	return state
}

func renderWithChain(state *RenderContext, renderers []Renderer, terminal RenderNext) (template.HTML, error) {
	result := renderWithChainResult(state, renderers, terminal)
	return result.HTML, result.Err
}

func renderWithChainResult(state *RenderContext, renderers []Renderer, terminal RenderNext) renderResult {
	if state == nil {
		return renderResult{Err: fmt.Errorf("render context is not configured")}
	}
	if terminal == nil {
		return renderResult{Response: state.Response, Err: fmt.Errorf("terminal renderer is not configured")}
	}

	active := make([]Renderer, 0, len(renderers))
	for _, renderer := range renderers {
		if renderer != nil {
			active = append(active, renderer)
		}
	}

	var err error
	for _, renderer := range active {
		response := state.Response
		state, err = renderer.Prepare(state)
		if err != nil {
			if state != nil {
				response = state.Response
			}
			return renderResult{Response: response, Err: err}
		}
		if state == nil {
			return renderResult{Err: fmt.Errorf("renderer prepare returned nil context")}
		}
		if state.Response == nil {
			state.Response = &RenderResponse{Headers: make(map[string]string)}
		}
	}

	next := terminal
	for i := len(active) - 1; i >= 0; i-- {
		renderer := active[i]
		previous := next
		next = func(ctx *RenderContext) (template.HTML, error) {
			return renderer.Render(ctx, previous)
		}
	}

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
