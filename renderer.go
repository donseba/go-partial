package partial

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
)

type (
	RenderKind string

	RenderValues map[any]any

	RenderResponse struct {
		Headers map[string]string
		Status  int
	}

	RenderNext func(*RenderContext) (template.HTML, error)

	Renderer interface {
		Prepare(*RenderContext) (*RenderContext, error)
		Render(*RenderContext, RenderNext) (template.HTML, error)
		Finalize(*RenderContext, template.HTML, error) (template.HTML, error)
	}

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

func TemplateRenderer() Renderer {
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
	}
	state.Runtime = newRuntime(p, state)
	return state
}

func renderWithChain(state *RenderContext, renderers []Renderer, terminal RenderNext) (template.HTML, error) {
	if state == nil {
		return "", fmt.Errorf("render context is not configured")
	}
	if terminal == nil {
		return "", fmt.Errorf("terminal renderer is not configured")
	}

	active := make([]Renderer, 0, len(renderers))
	for _, renderer := range renderers {
		if renderer != nil {
			active = append(active, renderer)
		}
	}

	var err error
	for _, renderer := range active {
		state, err = renderer.Prepare(state)
		if err != nil {
			return "", err
		}
		if state == nil {
			return "", fmt.Errorf("renderer prepare returned nil context")
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

	out, renderErr := next(state)
	for i := len(active) - 1; i >= 0; i-- {
		out, renderErr = active[i].Finalize(state, out, renderErr)
	}

	return out, renderErr
}
