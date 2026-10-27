// Package slots provides experimental named regions backed by child partials.
//
// A slot is a part of a page that a template places by name with
// {{ slot "name" }}. Slots are looked up from the partial that places them
// through its ancestors, so a content template can place a slot set on the
// layout. With options, a slot can also be answered on its own when a
// partial request targets it (CheckContracts, OmitOOB), or follow every
// partial response out of band (OOB), or load after the page (Deferred).
// Glob sets a slot for every file of a directory.
package slots

import (
	"fmt"
	"html/template"
	"path"
	"reflect"
	"strings"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/internal/templateutil"
)

type (
	// config holds the slots set on a partial by name, and the partial, its
	// owner, under the name "", which no slot has. Set adds to the config
	// of the partial it is given only when that partial owns it; a partial
	// that inherits or shares a config gets a config of its own, so its
	// ancestors and the partial it was cloned from keep theirs.
	config map[string]entry

	entry struct {
		child *partial.Partial
		// options are nil for a slot without options.
		options *options
	}

	options struct {
		oob            bool
		standalone     bool
		checkContracts bool
		omitOOB        bool
		// deferred slots place a placeholder that loads them; see
		// Deferred.
		deferred bool
		// extra holds the options few slots have, nil without.
		extra *extraOptions
	}

	extraOptions struct {
		placeholder *partial.Partial
		setup       func(name string, child *partial.Partial)
		// data is the slot's data for its placeholder; see Data.
		data any
	}

	// Option configures a slot; see Set.
	Option func(*options)

	// PlaceholderData is the dot of the placeholder of a deferred slot;
	// see Placeholder.
	PlaceholderData struct {
		// ID is the ID of the slot's partial, which the placeholder's
		// element carries, so its request targets the slot.
		ID string
		// Name is the slot's name.
		Name string
		// URL is the address the placeholder requests: that of the page.
		URL string
		// Attrs are the connector's attributes that make the element
		// request the slot and swap it in, such as hx-get, hx-trigger,
		// hx-target and hx-swap for htmx.
		Attrs template.HTMLAttr
		// Data is the data of the slot's own that Data gives it, such as
		// the heading of a dashboard panel, so one placeholder template
		// serves several slots; nil without.
		Data any
	}

	extensionKey struct{}
	nameKey      struct{}
)

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[extensionKey]().PkgPath()

// ownerName is the name the owner of a config is kept under.
const ownerName = ""

// Set registers child as the slot name of parent, which templates of
// parent and of the partials rendered under it place with
// {{ slot "name" }}. A slot of the same name set on a nearer partial
// replaces it.
//
// By default the child is registered on parent (see Partial.With), so a
// partial request that targets the child's ID gets it as any registered
// child: with the out-of-band regions of its ancestors. Options change
// that:
//
//	// A layout part sent along with every partial response.
//	slots.Set(root, "cart", cart, slots.OOB())
//
//	// A part of the page answered on its own, when the page can render it.
//	slots.Set(root, "comments", comments, slots.CheckContracts(), slots.OmitOOB())
func Set(parent *partial.Partial, name string, child *partial.Partial, opts ...Option) *partial.Partial {
	if parent == nil || name == "" || child == nil {
		return parent
	}

	var slotOptions *options
	if len(opts) > 0 {
		slotOptions = applyOptions(opts)
		if slotOptions.extra != nil && slotOptions.extra.setup != nil {
			slotOptions.extra.setup(name, child)
		}
	}

	slot := entry{child: child, options: slotOptions}
	if own := ownConfig(parent); own != nil {
		own[name] = slot
	} else {
		next := config{ownerName: {child: parent}, name: slot}
		if shared := sharedConfig(parent); shared != nil {
			// A clone keeps the slots of the partial it was cloned from.
			for key, value := range shared {
				if key != name && key != ownerName {
					next[key] = value
				}
			}
		}

		parent.SetExtension(extensionKey{}, next)
	}

	child.SetExtension(nameKey{}, name)

	switch {
	case slotOptions != nil && slotOptions.oob:
		parent.WithOOB(child)
		child.SetAlwaysSwapOOB(true)
	case slotOptions != nil && slotOptions.standalone:
		// The stage answers requests for it; see Stage.
	default:
		parent.With(child)
	}

	return parent
}

func applyOptions(opts []Option) *options {
	applied := &options{}

	for _, opt := range opts {
		if opt != nil {
			opt(applied)
		}
	}

	return applied
}

// more returns the options few slots have, made when first set.
func (o *options) more() *extraOptions {
	if o.extra == nil {
		o.extra = &extraOptions{}
	}

	return o.extra
}

// opts returns the slot's options, the zero options for a slot without.
func (e entry) opts() options {
	if e.options == nil {
		return options{}
	}

	return *e.options
}

// OOB makes the slot a region that follows every partial response: placed
// by {{ slot }} on a full page, and sent out of band along with the target
// of every partial request, such as a cart count in the header (see
// Partial.WithOOB and SetAlwaysSwapOOB). A request for the partial the slot
// is set on gets it where that partial's template places it, else out of
// band. Its template marks its element as out of band with {{ oobAttr }}.
func OOB() Option {
	return func(opts *options) {
		opts.oob = true
	}
}

// CheckContracts makes the slot standalone, answering only when it can
// render in the tree: its typed roots find their values and its dot has its
// @dot type (see partial.Runtime.CheckContracts). Otherwise the request is
// treated as one for an unknown target, which the tree's target fallback
// answers, if it has one (see partial.Partial.SetTargetFallback).
//
// A standalone slot answers partial requests that target its ID, rendered
// as a child of the partial being written, through the slots Stage, which
// must be in use. It is not registered as a child of the partial it is set
// on: it is found by its ID from the partial being written and its
// ancestors. Use it for parts of a page that are requested on their own,
// such as a comment thread a form updates.
func CheckContracts() Option {
	return func(opts *options) {
		opts.standalone = true
		opts.checkContracts = true
	}
}

// OmitOOB makes the slot standalone (see CheckContracts), and answers
// partial requests for it with the slot alone, without the out-of-band
// regions of the tree (see partial.RenderContext.OmitOOB).
func OmitOOB() Option {
	return func(opts *options) {
		opts.standalone = true
		opts.omitOOB = true
	}
}

// Deferred makes the slot load after the page: on a full page {{ slot }}
// renders a placeholder that requests the slot from the page's own address,
// through the connector, such as htmx's hx-get with hx-trigger="load", and
// the request for the slot's ID renders the slot alone, as a standalone
// slot answers it (see CheckContracts). A slot whose data is expensive,
// such as a panel of a dashboard with a lazy dot (see partial.DotFuncOf),
// then costs nothing for the page itself. It makes the slot standalone;
// add CheckContracts or OmitOOB as for any standalone slot. Without a
// request, such as in a render for an e-mail, the slot renders in place.
//
// The default placeholder is an empty <div> with the slot's ID, the
// connector's attributes and aria-busy="true" (a <turbo-frame> with
// Turbo); Placeholder renders one of the application's own.
//
//	slots.Set(root, "recent", recent, slots.Deferred(), slots.OmitOOB())
func Deferred() Option {
	return func(opts *options) {
		opts.standalone = true
		opts.deferred = true
	}
}

// Placeholder renders the placeholder of a deferred slot with p, with a
// PlaceholderData dot, such as a panel that shows it is loading:
//
//	<section id="{{ .ID }}" class="panel"{{ .Attrs }} aria-busy="true">Loading…</section>
//
// Its element must carry the ID and the attributes. p renders as a child of
// the partial that places the slot, from its file system; p is not
// changed. It implies Deferred.
func Placeholder(p *partial.Partial) Option {
	return func(opts *options) {
		Deferred()(opts)

		if p != nil {
			opts.more().placeholder = p
		}
	}
}

// Data gives the slot data of its own for its placeholder, as
// PlaceholderData.Data, such as the heading of a dashboard panel, so one
// placeholder template serves several deferred slots:
//
//	slots.Set(root, "recent", recent, slots.Placeholder(panel), slots.Data(Heading{Title: "Recent"}))
//
// The data is the placeholder's only; the slot's partial keeps its own dot.
func Data(data any) Option {
	return func(opts *options) {
		opts.more().data = data
	}
}

// Setup calls fn with the name and partial of the slot before Set
// registers it, such as to give the slots of Glob their dot:
//
//	slots.Glob(root, "regions/*.gohtml", slots.OOB(), slots.Setup(func(name string, region *partial.Partial) {
//		region.SetDot(view)
//	}))
func Setup(fn func(name string, child *partial.Partial)) Option {
	return func(opts *options) {
		if fn != nil {
			opts.more().setup = fn
		}
	}
}

// Glob sets a slot on parent for every template file pattern matches in
// parent's file system, such as "regions/*.gohtml": named after the file
// without its extension ("menu" for regions/menu.gohtml), rendered by a
// partial with that ID and the file as its template, with opts (see Set).
// The files are found as partial.Partial.Glob finds them: with the
// template cache, once per file system; without, at every call, so new
// files show. The slots are set in the order of the file names, which is
// the order slots set with OOB go out of band. It returns an error for a
// malformed pattern.
//
//	err := slots.Glob(root, "fragments/*.gohtml", slots.CheckContracts(), slots.OmitOOB())
func Glob(parent *partial.Partial, pattern string, opts ...Option) error {
	if parent == nil {
		return nil
	}

	files, err := parent.Glob(pattern)
	if err != nil {
		return fmt.Errorf("slots %q: %w", pattern, err)
	}

	for _, file := range files {
		name := FileName(file)
		if name == "" {
			continue
		}

		Set(parent, name, partial.NewID(name, file), opts...)
	}

	return nil
}

// FileName is the name Glob gives the slot of a file: its base name without
// its extension, such as "menu" for regions/menu.gohtml.
func FileName(file string) string {
	base := path.Base(file)

	return strings.TrimSuffix(base, path.Ext(base))
}

// Name returns the slot name associated with a partial, if it was registered as a slot.
func Name(p *partial.Partial) string {
	if p == nil {
		return ""
	}
	value, ok := p.Extension(nameKey{})
	if !ok {
		return ""
	}
	name, _ := value.(string)
	return name
}

// FuncMap returns template helpers for rendering slots.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"slot":         Slot,
		"hasSlot":      HasSlot,
		"requiredSlot": RequiredSlot,
		"deferredSlot": DeferredSlot,
	}
}

// Stage gives templates the slot helpers, bound to the active render
// context and made only for the templates that call them, and answers
// partial requests for standalone slots (see CheckContracts).
//
// {{ slot "name" }} renders nothing for a slot that is not set;
// {{ requiredSlot "name" }} fails the template for it, such as a theme's
// template that places a fragment the page must have. {{ hasSlot "name" }}
// tells whether it is set. {{ deferredSlot "name" }} places a slot that
// loads after the page, as if it were Deferred, and
// {{ deferredSlot "name" "reveal" }} one that loads when it scrolls into
// view: the template that places a slot decides, such as for one of the
// slots Glob sets for a directory of fragments.
func Stage() partial.RenderStage {
	return partial.RenderStageHooks{
		Package:     packagePath,
		FuncNames:   []string{"slot", "hasSlot", "requiredSlot", "deferredSlot"},
		ResolveFunc: resolveFunc,
		PrepareFunc: prepare,
	}
}

// resolveFunc gives a template that calls them the slot helpers of its
// render.
func resolveFunc(ctx *partial.RenderContext, name string) (any, bool) {
	if ctx == nil {
		return nil, false
	}

	switch name {
	case "slot":
		return func(name string) template.HTML {
			return Slot(name, ctx)
		}, true
	case "requiredSlot":
		// go-doc:sig func(name string) (html/template.HTML, error)
		return func(name string) (template.HTML, error) {
			return requiredSlot(ctx, name)
		}, true
	case "hasSlot":
		return func(name string) bool {
			return HasSlot(name, ctx)
		}, true
	case "deferredSlot":
		// go-doc:sig func(name string, mode ...string) (html/template.HTML, error)
		return func(name string, mode ...string) (template.HTML, error) {
			return deferredSlot(ctx, name, mode)
		}, true
	default:
		return nil, false
	}
}

// DeferredSlot renders a named slot from a render context as one that loads
// after the page, as Deferred does, whatever the slot's options: on a full
// page a placeholder that requests the slot from the page's address, and
// the request for the slot's ID renders the slot, alone when it is
// standalone (such as with OmitOOB). The placeholder is the slot's own (see
// Placeholder), or the default one. "reveal" after the name loads it when
// it scrolls into view, such as htmx's hx-trigger="revealed"; "load", or
// nothing, as soon as the page loads. Without a request, such as in a
// render for an e-mail, the slot renders in place. A slot that is not set
// renders nothing; an out-of-band slot (OOB) cannot be deferred.
//
// It lets a template defer one slot of many set alike, such as a fragment
// of the slots Glob sets for a directory:
//
//	<aside>{{ deferredSlot "popular" "reveal" }}</aside>
//
// go-doc:sig func(name string, mode ...string) (html/template.HTML, error)
func DeferredSlot(name string, args ...any) (template.HTML, error) {
	var (
		ctx  *partial.RenderContext
		mode []string
	)

	for _, arg := range args {
		switch value := arg.(type) {
		case *partial.RenderContext:
			ctx = value
		case string:
			mode = append(mode, value)
		}
	}

	if ctx == nil {
		return "", fmt.Errorf("slot %q: no render context", name)
	}

	return deferredSlot(ctx, name, mode)
}

// deferredSlot renders the slot name for {{ deferredSlot }}.
func deferredSlot(ctx *partial.RenderContext, name string, mode []string) (template.HTML, error) {
	reveal := false

	for _, value := range mode {
		switch value {
		case "", "load":
		case "reveal":
			reveal = true
		default:
			return "", fmt.Errorf("slot %q: deferred mode must be load or reveal, got %q", name, value)
		}
	}

	if ctx == nil || ctx.Runtime == nil || name == "" {
		return "", nil
	}

	found, ok := resolveEntry(ctx.Partial, name)
	if !ok {
		return "", nil
	}

	if found.opts().oob {
		return "", fmt.Errorf("slot %q is out of band and cannot be deferred", name)
	}

	if ctx.Request == nil {
		return Slot(name, ctx), nil
	}

	return placeholder(ctx, name, found, reveal), nil
}

// requiredSlot renders the slot name for {{ requiredSlot }}: an error for a
// slot that is not set.
func requiredSlot(ctx *partial.RenderContext, name string) (template.HTML, error) {
	if !HasSlot(name, ctx) {
		return "", fmt.Errorf("slot %q is not set", name)
	}

	return Slot(name, ctx), nil
}

// prepare answers a partial request for a standalone slot with the slot.
func prepare(ctx *partial.RenderContext) (*partial.RenderContext, error) {
	if ctx == nil || ctx.Kind != partial.RenderKindTarget || ctx.Partial == nil || ctx.Runtime == nil || ctx.TargetResolved() {
		return ctx, nil
	}

	found, ok := lookupStandalone(ctx.Partial, ctx.Name)
	if !ok {
		return ctx, nil
	}

	if found.opts().checkContracts {
		if err := ctx.Runtime.CheckContracts(found.child); err != nil {
			ctx.Emit(partial.Event{
				Kind:    partial.EventTargetMissing,
				Level:   partial.EventInfo,
				Message: "slot cannot render here",
				Error:   err,
				Fields:  map[string]any{"target": ctx.Name},
			})
			return ctx, nil
		}
	}

	ctx.Partial = found.child
	if found.opts().omitOOB {
		ctx.OmitOOB()
	}

	return ctx, nil
}

// Slot renders a named slot from a render context.
//
// go-doc:sig func(name string) html/template.HTML
func Slot(name string, ctx ...*partial.RenderContext) template.HTML {
	if len(ctx) == 0 || ctx[0] == nil || ctx[0].Runtime == nil || name == "" {
		return ""
	}
	current := ctx[0]

	found, ok := resolveEntry(current.Partial, name)
	if !ok {
		return ""
	}

	child := found.child
	if found.options != nil && found.options.deferred && current.Request != nil {
		return placeholder(current, name, found, false)
	}

	out, err := current.Runtime.RenderPartialWithFallback(child)
	if err != nil {
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return out
}

// RequiredSlot renders a named slot from a render context, as Slot does,
// and fails for a slot that is not set.
//
// go-doc:sig func(name string) (html/template.HTML, error)
func RequiredSlot(name string, ctx ...*partial.RenderContext) (template.HTML, error) {
	if len(ctx) == 0 || ctx[0] == nil {
		return "", fmt.Errorf("slot %q: no render context", name)
	}

	return requiredSlot(ctx[0], name)
}

// HasSlot reports whether a named slot exists in a render context.
//
// go-doc:sig func(name string) bool
func HasSlot(name string, ctx ...*partial.RenderContext) bool {
	if len(ctx) == 0 || ctx[0] == nil || name == "" {
		return false
	}
	_, ok := resolve(ctx[0].Partial, name)
	return ok
}

// resolve returns the slot name of p or of its nearest ancestor with one.
func resolve(p *partial.Partial, name string) (*partial.Partial, bool) {
	found, ok := resolveEntry(p, name)

	return found.child, ok
}

// resolveEntry returns the slot name of p or of its nearest ancestor with
// one, with its options.
func resolveEntry(p *partial.Partial, name string) (entry, bool) {
	if name == ownerName {
		return entry{}, false
	}

	var slot entry

	eachConfig(p, func(cfg config) bool {
		found, ok := cfg[name]
		if ok {
			slot = found
		}

		return ok
	})

	return slot, slot.child != nil
}

// placeholder renders the placeholder of the deferred slot name, which
// requests the slot from the page's address: when it scrolls into view
// with reveal, else as soon as the page loads.
func placeholder(ctx *partial.RenderContext, name string, slot entry, reveal bool) template.HTML {
	id := slot.child.PartialID()

	url := ""
	if ctx.Request.URL != nil {
		url = ctx.Request.URL.RequestURI()
	}

	kind := connector.InteractionAsync
	if reveal {
		kind = connector.InteractionReveal
	}

	attrs := ctx.Runtime.Connector().InteractionAttrs(connector.Interaction{
		Kind:   kind,
		ID:     id,
		URL:    url,
		Target: connector.TargetSelf,
		Swap:   string(connector.SwapOuterHTML),
	})

	// The element carries the ID itself.
	tag := "div"
	if _, frame := attrs[connector.TurboAttrSource]; frame {
		tag = "turbo-frame"
	}

	delete(attrs, connector.TurboAttrFrameID)

	data := PlaceholderData{ID: id, Name: name, URL: url, Attrs: templateutil.HTMLAttrs(attrs)}

	var custom *partial.Partial

	if slot.options != nil && slot.options.extra != nil {
		data.Data = slot.options.extra.data
		custom = slot.options.extra.placeholder
	}

	if custom != nil {
		out, err := ctx.Runtime.RenderPartialWithFallback(custom.Clone().SetDot(data))
		if err != nil {
			return template.HTML(template.HTMLEscapeString(err.Error()))
		}

		return out
	}

	element := `<` + tag + ` id="` + template.HTMLEscapeString(id) + `"` + string(data.Attrs) + ` aria-busy="true"></` + tag + `>`

	return template.HTML(element)
}

// lookupStandalone returns the standalone slot whose partial has the ID
// target, set on p or its ancestors, the nearest first.
func lookupStandalone(p *partial.Partial, target string) (entry, bool) {
	var standalone entry

	found := false
	eachConfig(p, func(cfg config) bool {
		for name, candidate := range cfg {
			if name != ownerName && candidate.opts().standalone && !candidate.opts().oob &&
				candidate.child.PartialID() == target {
				standalone = candidate
				found = true

				return true
			}
		}

		return false
	})

	return standalone, found
}

// eachConfig calls visit with the slots set on p and on each of its
// ancestors with slots, the nearest first, until visit returns true.
func eachConfig(p *partial.Partial, visit func(config) bool) {
	for current := p; current != nil; {
		cfg := configOf(current)
		if cfg == nil || visit(cfg) {
			return
		}

		// Continue above the partial the slots are set on: its descendants
		// inherit them as extensions.
		owner := cfg.owner()

		current = current.Parent()
		for current != nil && configOf(current).owner() == owner {
			current = current.Parent()
		}
	}
}

// owner returns the partial the config was made for.
func (cfg config) owner() *partial.Partial {
	return cfg[ownerName].child
}

// ownConfig returns the slots p owns, nil when it owns none.
func ownConfig(p *partial.Partial) config {
	if cfg := configOf(p); cfg != nil && cfg.owner() == p {
		return cfg
	}

	return nil
}

// sharedConfig returns the slots p has as a copy of another partial, such
// as the one it was cloned from, rather than from its parent; nil when it
// has none or inherits them.
func sharedConfig(p *partial.Partial) config {
	cfg := configOf(p)
	if cfg == nil {
		return nil
	}

	if parent := p.Parent(); parent != nil && configOf(parent).owner() == cfg.owner() {
		return nil
	}

	return cfg
}

func configOf(p *partial.Partial) config {
	if p == nil {
		return nil
	}
	value, ok := p.Extension(extensionKey{})
	if !ok {
		return nil
	}
	cfg, _ := value.(config)
	return cfg
}
