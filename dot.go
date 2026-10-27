package partial

import (
	"context"
	"reflect"
	"sync"
)

type (
	// lazyDot computes a value of a partial when it renders: its dot (see
	// DotFuncOf) or a typed model (see ModelFuncOf).
	lazyDot struct {
		fn func(context.Context) (any, error)
		// typ is the type of the values fn returns, for the @dot check of
		// CheckContracts and for matching typed models; nil when it is not
		// known.
		typ reflect.Type
	}

	// lazyDots are the lazy values computed during one render tree, so a
	// value that several partials use is computed once. A tree has few:
	// they are kept in a slice, the first ones without an allocation.
	lazyDots struct {
		mu     sync.Mutex
		values []lazyDotValue
		inline [4]lazyDotValue
	}

	lazyDotValue struct {
		lazy  *lazyDot
		value any
		err   error
	}

	lazyDotsKey struct{}

	// Dot is the dot of a partial as a value, for code that passes it
	// around before a partial exists, such as in a field of a struct:
	// computed when the partial renders (DotFuncOf), keeping its type for
	// CheckContracts. The zero Dot is no dot. Give it to a partial with
	// SetDotSource.
	//
	//	type Panel struct {
	//		ID   string
	//		Dot  partial.Dot
	//	}
	//
	//	panel := Panel{ID: "stats", Dot: partial.DotFuncOf(loadStats)}
	//	partial.NewID(panel.ID, "stats.gohtml").SetDotSource(panel.Dot)
	Dot struct {
		lazy *lazyDot
	}
)

// DotFuncOf returns a Dot computed by fn only when the partial renders: on
// the page, or on its own as the target of a partial request. A part of a
// page whose data is expensive, such as a panel of a dashboard, then costs
// nothing in responses that leave it out. fn receives the context of the
// render and is called once per render tree, also when several partials
// inherit the dot; an error fails the render of the partial. A nil fn is
// no dot.
//
// As T is known before the dot is computed, CheckContracts checks it
// against the @dot declaration of the partial's template without calling
// fn, so it tells whether the partial suits a request without computing
// its data.
//
//	stats := partial.DotFuncOf(func(ctx context.Context) (Stats, error) {
//		return loadStats(ctx)
//	})
//	panel := partial.NewID("stats", "stats.gohtml").SetDotSource(stats)
func DotFuncOf[T any](fn func(ctx context.Context) (T, error)) Dot {
	if fn == nil {
		return Dot{}
	}

	return Dot{lazy: newLazyOf(fn)}
}

// IsZero reports whether the Dot is no dot.
func (d Dot) IsZero() bool {
	return d.lazy == nil
}

// SetDotSource sets the dot of the partial to d, computed when the partial
// renders. The zero Dot removes the dot, as ClearDot does. It replaces
// SetDot, and SetDot replaces it.
func (p *Partial) SetDotSource(d Dot) *Partial {
	if p == nil {
		return nil
	}

	if d.lazy == nil {
		return p.ClearDot()
	}

	return p.setLazyDot(d.lazy)
}

// newLazyOf makes the lazy value of fn, of the type T unless T is an
// interface, which tells nothing about the values it holds.
func newLazyOf[T any](fn func(ctx context.Context) (T, error)) *lazyDot {
	typ := reflect.TypeFor[T]()
	if typ.Kind() == reflect.Interface {
		typ = nil
	}

	return &lazyDot{
		fn: func(ctx context.Context) (any, error) {
			return fn(ctx)
		},
		typ: typ,
	}
}

func (p *Partial) setLazyDot(lazy *lazyDot) *Partial {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.dot = nil
	p.hasDot = false
	p.dotFunc = lazy
	p.lazy = true

	return p
}

// ModelFunc is a typed model computed only when a template calls it, as a
// value: SetModel and SetContract take it with other models, such as from a
// list of models an application passes around before a partial exists.
// Make one with ModelFuncOf; the zero ModelFunc is no model, which SetModel
// ignores.
type ModelFunc struct {
	lazy *lazyDot
}

// ModelFuncOf returns a model of the type T computed by fn only when a
// template of the partial or of a partial rendered under it calls the
// model, such as the navigation that a layout, its header and its sidebar
// each declare with @model Nav github.com/acme/app.Navigation: fn is called
// once per render tree, also when several partials call the model, and
// not at all for a response that leaves the templates that declare it out.
// An error fails the template that calls the model. It is matched by T, as
// SetModel matches values, without calling fn, also by CheckContracts; T
// must be a concrete type. A nil fn is no model.
//
//	models := []any{user, partial.ModelFuncOf(func(ctx context.Context) (SearchBox, error) {
//		return searchBox(ctx)
//	})}
//	page.SetModel(models...)
func ModelFuncOf[T any](fn func(ctx context.Context) (T, error)) ModelFunc {
	if fn == nil {
		return ModelFunc{}
	}

	return ModelFunc{lazy: newLazyOf(fn)}
}

// Model returns the dot as a model, sharing its value: it is computed once
// per render tree, also when a partial renders it as its dot and a
// template calls it as a model, such as a layout's dot and the regions that
// declare it with @model. A Dot of DotFuncOf for an interface type, whose
// values' type is not known before they are computed, and the zero Dot are
// no model, the zero ModelFunc, which SetModel ignores.
//
//	filled := partial.DotFuncOf(fillView)
//	page.SetDotSource(filled).SetModel(filled.Model())
func (d Dot) Model() ModelFunc {
	if d.lazy == nil || d.lazy.typ == nil {
		return ModelFunc{}
	}

	// A Dot and a ModelFunc hold the same lazy value.
	return ModelFunc(d)
}

// IsZero reports whether the ModelFunc is no model.
func (m ModelFunc) IsZero() bool {
	return m.lazy == nil
}

// hasLazyValues reports whether the partial or one of its ancestors has a
// value computed when it renders; see withLazyValues.
func (p *Partial) hasLazyValues() bool {
	for current := p; current != nil; {
		current.mu.RLock()
		lazy := current.lazy
		parent := current.parent
		current.mu.RUnlock()

		if lazy {
			return true
		}

		current = parent
	}

	return false
}

// withLazyValues returns ctx with a store for the lazy values of a render
// tree, unless it has one, so the partials rendered with it share them:
// the store is made where the render of the tree starts, before any partial
// of it computes a value.
func withLazyValues(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = defaultRenderContext()
	}

	if _, ok := ctx.Value(lazyDotsKey{}).(*lazyDots); ok {
		return ctx
	}

	return context.WithValue(ctx, lazyDotsKey{}, &lazyDots{})
}

// find returns the value computed for lazy.
func (d *lazyDots) find(lazy *lazyDot) (lazyDotValue, bool) {
	for _, found := range d.values {
		if found.lazy == lazy {
			return found, true
		}
	}

	return lazyDotValue{}, false
}

// add keeps the value computed for lazy.
func (d *lazyDots) add(found lazyDotValue) {
	if d.values == nil {
		d.values = d.inline[:0]
	}

	d.values = append(d.values, found)
}

// lazyValueFunc returns the function a template calls for a typed root
// whose value is lazy: it computes the value once per render tree.
func lazyValueFunc(state *RenderContext, lazy *lazyDot) func() (any, error) {
	return func() (any, error) {
		return state.lazyDotValue(lazy)
	}
}

// lazyDotValue returns the value of a lazy dot for the render: computed
// once per render tree, which the render's context carries.
func (ctx *RenderContext) lazyDotValue(lazy *lazyDot) (any, error) {
	if ctx.Context == nil {
		ctx.Context = defaultRenderContext()
	}

	computed, ok := ctx.Context.Value(lazyDotsKey{}).(*lazyDots)
	if !ok {
		computed = &lazyDots{}
		ctx.Context = context.WithValue(ctx.Context, lazyDotsKey{}, computed)
	}

	computed.mu.Lock()
	found, ok := computed.find(lazy)
	computed.mu.Unlock()

	if ok {
		return found.value, found.err
	}

	// The function runs unlocked: it may render partials of its own.
	value, err := lazy.fn(ctx.Context)

	computed.mu.Lock()
	defer computed.mu.Unlock()

	if found, ok := computed.find(lazy); ok {
		return found.value, found.err
	}

	computed.add(lazyDotValue{lazy: lazy, value: value, err: err})

	return value, err
}
