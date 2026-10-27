package partial

import (
	"context"
	"fmt"
	"html/template"
	"maps"
	"net/url"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

type (
	// TemplateFuncNamer is implemented by render stages that give templates
	// functions, with RenderContext.SetFunc or as a TemplateFuncResolver, so
	// ReservedFuncNames can list them. RenderStageHooks implements it with
	// its FuncNames.
	TemplateFuncNamer interface {
		TemplateFuncNames() []string
	}

	// contextFunc is a registered function whose first parameter is a
	// context.Context, with the type templates call it with: the same
	// without the context.
	contextFunc struct {
		fn       reflect.Value
		template reflect.Type
	}

	// contextBindings are the context functions bound during one render
	// tree, so a page binds each function once rather than in every partial
	// it renders.
	contextBindings struct {
		mu    sync.Mutex
		funcs map[contextBindingKey]contextBinding
		// runtimes are those of the partials whose templates are executing,
		// the innermost last; see RuntimeFrom.
		runtimes []*Runtime
	}

	// contextBindingKey identifies a function by its name and code; closures
	// of one function literal share their code, so the name tells them
	// apart.
	contextBindingKey struct {
		name string
		code uintptr
	}

	contextBinding struct {
		ctx context.Context
		fn  any
	}

	contextBindingsKey struct{}

	// funcTable holds the functions registered on a partial. A table is not
	// changed once made: SetFunc makes a new one, so clones of a partial
	// share it, and the identifier of its names is computed once.
	funcTable struct {
		// static are the functions by name; for functions that take a
		// context, stand-ins until a render binds them.
		static template.FuncMap
		// context are the registered functions whose first parameter is a
		// context.Context, by name.
		context map[string]contextFunc
		// set identifies the names of static for cache keys; zero until
		// computed. See funcSets.
		set atomic.Uint64
	}
)

var contextType = reflect.TypeFor[context.Context]()

// ReservedFuncNames returns, sorted, the template function names that
// SetFunc cannot give templates: go-partial's own helpers, such as partial,
// content, ctx and url, and the names the given render stages give
// templates, which replace registered functions of the same name. Names that
// start with an underscore are reserved as well; see IsReservedFuncName.
func ReservedFuncNames(stages ...RenderStage) []string {
	names := slices.Collect(maps.Keys(coreFunctionNames))

	for _, stage := range stages {
		namer, ok := stage.(TemplateFuncNamer)
		if !ok {
			continue
		}

		for _, name := range namer.TemplateFuncNames() {
			if !fallsBack(stage, name) {
				names = append(names, name)
			}
		}
	}

	slices.Sort(names)

	return slices.Compact(names)
}

// IsReservedFuncName reports whether templates cannot be given a function
// named name: one of ReservedFuncNames(stages...), or a name that starts with
// an underscore.
func IsReservedFuncName(name string, stages ...RenderStage) bool {
	if isProtectedFunctionName(name) {
		return true
	}

	for _, stage := range stages {
		if givesFunc(stage, name) && !fallsBack(stage, name) {
			return true
		}
	}

	return false
}

// givesFunc reports whether the stage gives templates a function named
// name; see TemplateFuncNamer.
func givesFunc(stage RenderStage, name string) bool {
	namer, ok := stage.(TemplateFuncNamer)

	return ok && slices.Contains(namer.TemplateFuncNames(), name)
}

// fallsBack reports whether the stage gives name only where no function of
// that name is registered; see RenderStageHooks.FallbackFuncNames.
func fallsBack(stage RenderStage, name string) bool {
	fallback, ok := stage.(TemplateFuncFallback)

	return ok && slices.Contains(fallback.FallbackTemplateFuncNames(), name)
}

// Func returns the function registered as name with SetFunc on the partial
// or, failing that, on its nearest ancestor, as SetFunc was given it.
func (p *Partial) Func(name string) (any, bool) {
	for current := p; current != nil; {
		current.mu.RLock()
		funcs := current.funcs
		parent := current.parent
		current.mu.RUnlock()

		if fn, ok := funcs.given(name); ok {
			return fn, true
		}

		current = parent
	}

	return nil, false
}

// warnReservedFuncs emits an EventFuncProtected warning for each function
// SetFunc is given that templates will never call: one named like a
// go-partial helper, which is ignored, or like a function a render stage of
// the partial or its ancestors gives templates, which replaces it.
func (p *Partial) warnReservedFuncs(funcMaps []template.FuncMap) {
	if p.getEvents() == nil {
		return
	}

	stages := p.getRenderStages()

	for _, funcMap := range funcMaps {
		for name, fn := range funcMap {
			p.warnReservedFunc(name, fn, stages)
		}
	}
}

// warnShadowedFuncs emits an EventFuncProtected warning for each function
// registered on the partial or its ancestors that the stages Use adds
// replace.
func (p *Partial) warnShadowedFuncs(stages []RenderStage) {
	if p.getEvents() == nil {
		return
	}

	registered := p.getRegisteredFuncs()

	for _, stage := range stages {
		namer, ok := stage.(TemplateFuncNamer)
		if !ok {
			continue
		}

		for _, name := range namer.TemplateFuncNames() {
			if fn, ok := registered[name]; ok {
				p.warnReservedFunc(name, fn, []RenderStage{stage})
			}
		}
	}
}

// getRegisteredFuncs returns the functions registered on the partial and
// its ancestors as SetFunc was given them, the nearest first.
func (p *Partial) getRegisteredFuncs() map[string]any {
	registered := make(map[string]any)

	for current := p; current != nil; current = current.parentPartial() {
		current.mu.RLock()
		funcs := current.funcs
		current.mu.RUnlock()

		for name := range funcs.staticFuncs() {
			if _, nearer := registered[name]; nearer {
				continue
			}

			fn, _ := funcs.given(name)
			registered[name] = fn
		}
	}

	return registered
}

// warnReservedFunc warns that the function fn registered as name is never
// called, unless it is the stand-in of the stage that gives the name: a
// function of the stage's own package, such as those of slots.FuncMap for
// slots.Stage.
func (p *Partial) warnReservedFunc(name string, fn any, stages []RenderStage) {
	message := ""

	switch {
	case isProtectedFunctionName(name):
		message = "template function ignored: the name is a go-partial helper"
	case IsReservedFuncName(name, stages...):
		if standsInFor(name, fn, stages) {
			return
		}

		message = "template function never called: a render stage gives templates this name"
	default:
		return
	}

	p.emitWithContext(context.Background(), nil, Event{
		Kind:    EventFuncProtected,
		Level:   EventWarn,
		Message: message,
		Fields:  map[string]any{"function": name},
	})
}

// standsInFor reports whether fn comes from the package of a stage that
// gives templates name: the stand-in a stage's package offers for it.
func standsInFor(name string, fn any, stages []RenderStage) bool {
	pkg := funcPackage(fn)
	if pkg == "" {
		return false
	}

	for _, stage := range stages {
		if givesFunc(stage, name) && stagePackage(stage) == pkg {
			return true
		}
	}

	return false
}

// funcPackage returns the import path of the package that defines the
// function fn, or "".
func funcPackage(fn any) string {
	value := reflect.ValueOf(fn)
	if value.Kind() != reflect.Func || value.IsNil() {
		return ""
	}

	info := runtime.FuncForPC(value.Pointer())
	if info == nil {
		return ""
	}

	return packageOf(info.Name())
}

// stagePackage returns the import path of the package that made the stage:
// RenderStageHooks.Package or, without it, that of the hook functions; or
// the package of the stage's type.
func stagePackage(stage RenderStage) string {
	var hooks *RenderStageHooks

	switch typed := stage.(type) {
	case RenderStageHooks:
		hooks = &typed
	case *RenderStageHooks:
		hooks = typed
	}

	if hooks == nil {
		typ := reflect.TypeOf(stage)
		for typ != nil && typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}

		if typ == nil {
			return ""
		}

		return typ.PkgPath()
	}

	if hooks.Package != "" {
		return hooks.Package
	}

	// The compiler names the hooks after the caller where it inlines the
	// function that returns the stage.
	for _, fn := range []any{hooks.ResolveFunc, hooks.PrepareFunc, hooks.RenderFunc, hooks.FinalizeFunc} {
		if pkg := funcPackage(fn); pkg != "" {
			return pkg
		}
	}

	return ""
}

// packageOf returns the package path of a qualified function name, such as
// github.com/acme/app for github.com/acme/app.(*T).Method.func1, and
// gopkg.in/yaml.v3 for gopkg.in/yaml%2ev3.Unmarshal: function names escape
// the dots of the last element of the path, which import paths keep.
func packageOf(qualified string) string {
	slash := strings.LastIndex(qualified, "/")

	dot := strings.Index(qualified[slash+1:], ".")
	if dot < 0 {
		return ""
	}

	pkg := qualified[:slash+1+dot]
	if !strings.Contains(pkg, "%") {
		return pkg
	}

	unescaped, err := url.PathUnescape(pkg)
	if err != nil {
		return pkg
	}

	return unescaped
}

// addStageStandIns gives funcs, the functions templates are parsed with, a
// stand-in for each function the stages declare (see TemplateFuncNamer)
// that funcs lacks: templates must know the names of the functions they
// call when they are parsed, and the stages give the functions themselves
// when they execute. A stand-in that is called reports that no stage of the
// render gave it.
func addStageStandIns(funcs template.FuncMap, stages []RenderStage) {
	for _, stage := range stages {
		namer, ok := stage.(TemplateFuncNamer)
		if !ok {
			continue
		}

		for _, name := range namer.TemplateFuncNames() {
			if _, ok := funcs[name]; ok || name == "" {
				continue
			}

			funcs[name] = stageStandIn(name)
		}
	}
}

func stageStandIn(name string) any {
	return func(...any) (any, error) {
		return nil, fmt.Errorf("template function %s: no render stage of this render gives it", name)
	}
}

// BindContext returns funcs for templates that go-partial does not execute,
// such as the text part of an e-mail: functions whose first parameter is a
// context.Context are given ctx, the others are returned as they are. Later
// maps replace names of earlier ones. Inside a render, functions the render
// already bound are reused.
func BindContext(ctx context.Context, funcs ...template.FuncMap) template.FuncMap {
	if ctx == nil {
		ctx = defaultRenderContext()
	}

	bindings := contextBindingsFrom(ctx)
	out := make(template.FuncMap)

	for _, set := range funcs {
		for name, fn := range set {
			contextual, ok := asContextFunc(fn)
			if !ok {
				out[name] = fn
				continue
			}

			out[name] = bindings.bind(ctx, name, contextual)
		}
	}

	return out
}

// RuntimeFrom returns the runtime of the partial whose template is
// executing in the render of ctx, for functions registered with SetFunc
// whose first parameter is a context.Context. Such a function can render a
// partial as a true child of the partial that calls it, with its models,
// functions, stages and failure policy:
//
//	func card(ctx context.Context, data Card) (template.HTML, error) {
//		runtime, ok := partial.RuntimeFrom(ctx)
//		if !ok {
//			return "", errors.New("card: not rendering")
//		}
//		return runtime.RenderPartial(partial.NewID("card", "card.gohtml").SetDot(data))
//	}
//
// It reports false outside a template go-partial executes. The partials of
// one render execute their templates one after another; a function that
// renders with ctx on other goroutines passes them the runtime instead.
func RuntimeFrom(ctx context.Context) (*Runtime, bool) {
	bindings := contextBindingsFrom(ctx)
	if bindings == nil {
		return nil, false
	}

	bindings.mu.Lock()
	defer bindings.mu.Unlock()

	if len(bindings.runtimes) == 0 {
		return nil, false
	}

	return bindings.runtimes[len(bindings.runtimes)-1], true
}

// push records that the template of runtime's partial is executing.
func (b *contextBindings) push(runtime *Runtime) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.runtimes = append(b.runtimes, runtime)
}

// pop records that the innermost executing template finished.
func (b *contextBindings) pop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.runtimes) > 0 {
		b.runtimes[len(b.runtimes)-1] = nil
		b.runtimes = b.runtimes[:len(b.runtimes)-1]
	}
}

// asContextFunc reports whether fn is a function whose first parameter is a
// context.Context.
func asContextFunc(fn any) (contextFunc, bool) {
	if fn == nil {
		return contextFunc{}, false
	}

	value := reflect.ValueOf(fn)

	typ := value.Type()
	if typ.Kind() != reflect.Func || typ.NumIn() == 0 || typ.In(0) != contextType {
		return contextFunc{}, false
	}

	in := make([]reflect.Type, 0, typ.NumIn()-1)
	for i := 1; i < typ.NumIn(); i++ {
		in = append(in, typ.In(i))
	}

	out := make([]reflect.Type, 0, typ.NumOut())
	for i := range typ.NumOut() {
		out = append(out, typ.Out(i))
	}

	return contextFunc{fn: value, template: reflect.FuncOf(in, out, typ.IsVariadic())}, true
}

// placeholder is a function of the template-facing type that returns zero
// values; it stands in while templates are parsed.
func (f contextFunc) placeholder() any {
	return reflect.MakeFunc(f.template, func([]reflect.Value) []reflect.Value {
		out := make([]reflect.Value, f.template.NumOut())
		for i := range out {
			out[i] = reflect.Zero(f.template.Out(i))
		}

		return out
	}).Interface()
}

// bound returns the function with its context parameter filled in.
func (f contextFunc) bound(ctx context.Context) any {
	ctxValue := reflect.ValueOf(&ctx).Elem()
	variadic := f.fn.Type().IsVariadic()

	return reflect.MakeFunc(f.template, func(args []reflect.Value) []reflect.Value {
		call := make([]reflect.Value, 0, len(args)+1)
		call = append(call, ctxValue)

		call = append(call, args...)
		if variadic {
			return f.fn.CallSlice(call)
		}

		return f.fn.Call(call)
	}).Interface()
}

// withContextBindings returns ctx with a place for the functions bound
// during its render tree, unless it has one.
func withContextBindings(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = defaultRenderContext()
	}

	if contextBindingsFrom(ctx) != nil {
		return ctx
	}

	return context.WithValue(ctx, contextBindingsKey{}, &contextBindings{funcs: make(map[contextBindingKey]contextBinding)})
}

func contextBindingsFrom(ctx context.Context) *contextBindings {
	if ctx == nil {
		return nil
	}

	bindings, _ := ctx.Value(contextBindingsKey{}).(*contextBindings)
	return bindings
}

// bind returns f bound to ctx: the function bound before for the same
// context, or a new one.
func (b *contextBindings) bind(ctx context.Context, name string, f contextFunc) any {
	if b == nil {
		return f.bound(ctx)
	}

	key := contextBindingKey{name: name, code: f.fn.Pointer()}

	b.mu.Lock()
	defer b.mu.Unlock()

	if binding, ok := b.funcs[key]; ok && sameContext(binding.ctx, ctx) {
		return binding.fn
	}

	fn := f.bound(ctx)
	b.funcs[key] = contextBinding{ctx: ctx, fn: fn}

	return fn
}

// sameContext compares contexts without panicking on context types that
// cannot be compared.
func sameContext(a, b context.Context) bool {
	if reflect.TypeOf(a) != reflect.TypeOf(b) {
		return false
	}

	if a == nil {
		return true
	}

	if !reflect.ValueOf(a).Comparable() {
		return false
	}

	return a == b
}

// bindContextFunc returns the context function f bound to the render's
// context.
func (ctx *RenderContext) bindContextFunc(name string, f contextFunc) any {
	if ctx.Context == nil {
		ctx.Context = defaultRenderContext()
	}

	return contextBindingsFrom(ctx.Context).bind(ctx.Context, name, f)
}

// with returns a table with the functions of t and funcMaps; later maps
// replace names of earlier ones. Protected names are skipped.
func (t *funcTable) with(funcMaps ...template.FuncMap) *funcTable {
	next := &funcTable{static: make(template.FuncMap)}
	if t != nil {
		next.static = maps.Clone(t.static)
		next.context = maps.Clone(t.context)
	}

	for _, funcMap := range funcMaps {
		for name, fn := range funcMap {
			if isProtectedFunctionName(name) {
				continue
			}

			contextual, ok := asContextFunc(fn)
			if !ok {
				next.static[name] = fn
				delete(next.context, name)
				continue
			}

			// Templates call the function without its context; this
			// stands in until a render binds it.
			if next.context == nil {
				next.context = make(map[string]contextFunc)
			}

			next.context[name] = contextual
			next.static[name] = contextual.placeholder()
		}
	}

	return next
}

// merged returns a table with the functions of t, replaced by those of
// over where both have a name.
func (t *funcTable) merged(over *funcTable) *funcTable {
	if t == nil {
		return over
	}

	if over == nil {
		return t
	}

	next := &funcTable{static: maps.Clone(t.static), context: maps.Clone(t.context)}
	for name, fn := range over.static {
		next.static[name] = fn
		if contextual, ok := over.context[name]; ok {
			if next.context == nil {
				next.context = make(map[string]contextFunc)
			}

			next.context[name] = contextual
			continue
		}

		delete(next.context, name)
	}

	return next
}

func (t *funcTable) staticFuncs() template.FuncMap {
	if t == nil {
		return nil
	}

	return t.static
}

func (t *funcTable) contextFuncs() map[string]contextFunc {
	if t == nil {
		return nil
	}

	return t.context
}

// given returns the function registered as name as SetFunc was given it:
// for a function that takes a context, the function itself rather than
// its stand-in.
func (t *funcTable) given(name string) (any, bool) {
	fn, ok := t.staticFuncs()[name]
	if !ok {
		return nil, false
	}

	if contextual, ok := t.contextFuncs()[name]; ok {
		return contextual.fn.Interface(), true
	}

	return fn, true
}

// setID identifies the names of the table's functions, with go-partial's
// own helpers, for template cache keys.
func (t *funcTable) setID() uint64 {
	if t == nil {
		return coreFuncSet()
	}

	if id := t.set.Load(); id != 0 {
		return id
	}

	id := registeredFuncSets.intern(templateFuncSignature(t.static))
	t.set.Store(id)

	return id
}

// coreFuncSet identifies the names of go-partial's own helpers: the
// function set of partials without functions of their own.
var coreFuncSet = sync.OnceValue(func() uint64 {
	return registeredFuncSets.intern(coreFuncSignature)
})

// lookupTemplateFunc returns the function registered as name on the partial
// or, failing that, on its nearest ancestor, bound to the render's context
// when it takes one.
func (p *Partial) lookupTemplateFunc(state *RenderContext, name string) (any, bool) {
	for current := p; current != nil; {
		current.mu.RLock()
		fn, ok := current.funcs.staticFuncs()[name]
		contextual, takesContext := current.funcs.contextFuncs()[name]
		parent := current.parent
		current.mu.RUnlock()

		if ok {
			if takesContext {
				return state.bindContextFunc(name, contextual), true
			}

			return fn, true
		}

		current = parent
	}

	return nil, false
}

// getContextFuncs returns the context functions templates of the partial
// call: those of the partial and its ancestors, unless a nearer partial
// registered a function without a context under the same name.
func (p *Partial) getContextFuncs() map[string]contextFunc {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var funcs map[string]contextFunc
	if p.parent != nil {
		funcs = p.parent.getContextFuncs()
	}

	own := p.funcs.contextFuncs()
	for name := range p.funcs.staticFuncs() {
		contextual, ok := own[name]
		if ok {
			if funcs == nil {
				funcs = make(map[string]contextFunc)
			}

			funcs[name] = contextual
			continue
		}

		delete(funcs, name)
	}

	return funcs
}

// hasContextFuncs reports whether the partial or an ancestor registered a
// function that takes a context.
func (p *Partial) hasContextFuncs() bool {
	for current := p; current != nil; {
		current.mu.RLock()
		count := len(current.funcs.contextFuncs())
		parent := current.parent
		current.mu.RUnlock()

		if count > 0 {
			return true
		}

		current = parent
	}

	return false
}

// bindContextFuncs replaces the stand-ins of context functions in funcs, a
// map of every function, with the functions bound to the render's context.
func (p *Partial) bindContextFuncs(funcs template.FuncMap, state *RenderContext) {
	for name, contextual := range p.getContextFuncs() {
		funcs[name] = state.bindContextFunc(name, contextual)
	}
}
