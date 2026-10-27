// Package interactions contains optional client-interaction template helpers.
//
// The core go-partial package does not register these helpers automatically.
// Applications opt in with:
//
//	root.SetFunc(interactions.FuncMap())
package interactions

import (
	"bytes"
	"fmt"
	"html/template"
	"maps"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// RenderKindInteraction is the renderer kind used for interaction markup.
const RenderKindInteraction partial.RenderKind = "interaction"

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[Interaction]().PkgPath()

// Interaction is an optional builder for connector-aware client interactions.
// Register values with partial.SetContract("interaction", ...).
type Interaction struct {
	contractName string
	interaction  connector.Interaction
}

// Data is the render payload for RenderKindInteraction.
type Data struct {
	Runtime     *partial.Runtime
	Interaction connector.Interaction
	Attrs       map[string]string
}

// MarkupRenderer renders connector interaction attributes into final HTML.
type MarkupRenderer func(runtime *partial.Runtime, interaction connector.Interaction, attrs map[string]string) (template.HTML, error)

const (
	// SwapInnerHTML replaces the target element's inner HTML.
	SwapInnerHTML = string(connector.SwapInnerHTML)
	// SwapOuterHTML replaces the target element itself.
	SwapOuterHTML = string(connector.SwapOuterHTML)
)

// FuncMap returns the optional interaction template helpers: those that
// render an element of their own (async, reveal, poll, on, stream, prefetch
// and refresh) and those of AttrsFuncMap. The names of the first are
// generic, so an application that only writes attributes on elements of its
// own registers AttrsFuncMap instead, leaving names such as "poll" free for
// its own functions.
func FuncMap() template.FuncMap {
	funcs := template.FuncMap{
		"async":    Async,
		"reveal":   Reveal,
		"poll":     Poll,
		"on":       On,
		"stream":   Stream,
		"prefetch": Prefetch,
		"refresh":  Refresh,
	}

	maps.Copy(funcs, AttrsFuncMap())

	return funcs
}

// AttrsFuncMap returns the interaction helpers that write attributes on an
// element of the template's own: asyncAttrs, revealAttrs, pollAttrs,
// onAttrs, streamAttrs, refreshAttrs, submitAttrs, liveAttrs, clientAttrs,
// noBoost and confirmAttr. Their names end in Attrs or say what they write,
// so they do not clash with an application's functions, such as a "poll"
// of its own, as FuncMap's async, poll or on may.
//
//	root.SetFunc(interactions.AttrsFuncMap())
func AttrsFuncMap() template.FuncMap {
	return template.FuncMap{
		"asyncAttrs":   AsyncAttrs,
		"revealAttrs":  RevealAttrs,
		"pollAttrs":    PollAttrs,
		"onAttrs":      OnAttrs,
		"streamAttrs":  StreamAttrs,
		"refreshAttrs": RefreshAttrs,
		"submitAttrs":  SubmitAttrs,
		"liveAttrs":    LiveAttrs,

		"clientAttrs": ClientAttrs,
		"noBoost":     NoBoost,
		"confirmAttr": ConfirmAttr,
	}
}

// Stage returns a partial render stage for interaction markup.
func Stage(markup ...MarkupRenderer) partial.RenderStage {
	renderer := DefaultMarkupRenderer()
	if len(markup) > 0 && markup[0] != nil {
		renderer = markup[0]
	}

	return partial.RenderStageHooks{
		Package: packagePath,
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if ctx == nil || ctx.Kind != RenderKindInteraction {
				return next(ctx)
			}
			data, ok := ctx.Data.(Data)
			if !ok {
				return "", fmt.Errorf("interaction renderer expected interactions.Data, got %T", ctx.Data)
			}
			return renderer(data.Runtime, data.Interaction, data.Attrs)
		},
	}
}

// NewAsync creates an async interaction contract value.
func NewAsync(endpoint string) Interaction {
	return newInteraction(connector.InteractionAsync, "", endpoint)
}

// NewReveal creates a reveal interaction contract value.
func NewReveal(endpoint string) Interaction {
	return newInteraction(connector.InteractionReveal, "", endpoint)
}

// NewPoll creates a polling interaction contract value.
func NewPoll(endpoint string) Interaction {
	return newInteraction(connector.InteractionPoll, "", endpoint)
}

// NewStream creates an SSE-backed interaction contract value.
func NewStream(endpoint string) Interaction {
	return newInteraction(connector.InteractionStream, "", endpoint)
}

// NewPrefetch creates a prefetch interaction contract value.
func NewPrefetch(endpoint string) Interaction {
	return newInteraction(connector.InteractionPrefetch, "", endpoint)
}

// NewRefresh creates a refresh-control interaction contract value.
func NewRefresh(endpoint string) Interaction {
	return newInteraction(connector.InteractionRefresh, "", endpoint)
}

// NewOn creates an event-driven interaction contract value.
func NewOn(event string, endpoint string) Interaction {
	return newInteraction(connector.InteractionOn, event, endpoint).
		Trigger(event).
		From("body").
		Placeholder("")
}

func newInteraction(kind connector.InteractionKind, name string, endpoint string) Interaction {
	return Interaction{interaction: connector.Interaction{
		Kind:    kind,
		Name:    name,
		URL:     endpoint,
		Params:  make(map[string]string),
		Options: make(map[string]string),
	}}
}

// Interaction returns the connector interaction represented by the builder.
func (i Interaction) Interaction() connector.Interaction {
	interaction := i.interaction
	if interaction.Params == nil {
		interaction.Params = make(map[string]string)
	}
	if interaction.Options == nil {
		interaction.Options = make(map[string]string)
	}
	interaction.URL = resolveURL(interaction.URL, interaction.Params)
	return normalize(interaction)
}

// ContractName returns the contract name used when registering the interaction.
func (i Interaction) ContractName() string {
	if i.contractName != "" {
		return i.contractName
	}
	return nameFromEndpoint(i.interaction.URL)
}

// ID sets the element ID used by the rendered interaction.
func (i Interaction) ID(id string) Interaction {
	i.interaction.ID = id
	return i
}

// As sets the contract name for this interaction.
func (i Interaction) As(name string) Interaction {
	i.contractName = strings.TrimSpace(name)
	return i
}

// Target sets the client-side target selector.
func (i Interaction) Target(target string) Interaction {
	i.interaction.Target = target
	return i
}

// Swap sets the connector swap strategy.
func (i Interaction) Swap(swap string) Interaction {
	i.interaction.Swap = swap
	return i
}

// Trigger sets the browser event or connector trigger.
func (i Interaction) Trigger(trigger string) Interaction {
	i.interaction.Trigger = trigger
	return i
}

// From sets the event source option for event-driven interactions.
func (i Interaction) From(source string) Interaction {
	if i.interaction.Options == nil {
		i.interaction.Options = make(map[string]string)
	}
	i.interaction.Options["from"] = source
	return i
}

// Every sets the polling interval.
func (i Interaction) Every(interval time.Duration) Interaction {
	i.interaction.Interval = interval.String()
	return i
}

// EveryString sets the polling interval from a raw connector interval string.
func (i Interaction) EveryString(interval string) Interaction {
	i.interaction.Interval = interval
	return i
}

// Placeholder sets the placeholder markup text rendered before content loads.
func (i Interaction) Placeholder(placeholder string) Interaction {
	i.interaction.Placeholder = placeholder
	if i.interaction.Options == nil {
		i.interaction.Options = make(map[string]string)
	}
	i.interaction.Options["placeholder"] = placeholder
	return i
}

// Param sets a URL/path parameter used when resolving the endpoint.
func (i Interaction) Param(key string, value any) Interaction {
	if i.interaction.Params == nil {
		i.interaction.Params = make(map[string]string)
	}
	i.interaction.Params[key] = fmt.Sprint(value)
	return i
}

// Option sets a connector option.
func (i Interaction) Option(key string, value any) Interaction {
	if i.interaction.Options == nil {
		i.interaction.Options = make(map[string]string)
	}
	i.interaction.Options[key] = fmt.Sprint(value)
	return i
}

// Async renders an async interaction. Pass either an endpoint string with
// optional key/value parameters or an Interaction configured in Go.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func Async(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return Render(connector.InteractionAsync, value, runtime, args...)
}

// Reveal renders an interaction that loads when the element enters the viewport.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func Reveal(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return Render(connector.InteractionReveal, value, runtime, args...)
}

// Poll renders an interaction that refreshes on an interval. When an endpoint
// string is used, a single extra argument is treated as the interval.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func Poll(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return Render(connector.InteractionPoll, value, runtime, args...)
}

// On renders an interaction that refreshes when a browser event is dispatched.
// Pass either an Interaction or an event name followed by an endpoint.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, event string, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func On(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return renderOn(runtime, value, args...)
}

// Stream renders an SSE-backed interaction placeholder.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func Stream(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return Render(connector.InteractionStream, value, runtime, args...)
}

// Prefetch renders a non-visual prefetch hint.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func Prefetch(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return Render(connector.InteractionPrefetch, value, runtime, args...)
}

// Refresh renders a control that explicitly refreshes a target.
//
// go-doc:include
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTML
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTML
func Refresh(runtime *partial.Runtime, value any, args ...any) template.HTML {
	return Render(connector.InteractionRefresh, value, runtime, args...)
}

// AsyncAttrs returns the attributes of an async interaction, for an element
// of the template's own, such as <section {{ asyncAttrs runtime "/stats" }}>.
// Without a target or id, the element itself is the target. Arguments are
// those of Async.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func AsyncAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionAsync, value, runtime, args...)
}

// RevealAttrs returns the attributes of a reveal interaction for an element
// of the template's own; see AsyncAttrs.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func RevealAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionReveal, value, runtime, args...)
}

// PollAttrs returns the attributes of a polling interaction for an element
// of the template's own; see AsyncAttrs. A single argument after an
// endpoint string is the interval, as with Poll.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func PollAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionPoll, value, runtime, args...)
}

// OnAttrs returns the attributes of an event-driven interaction for an
// element of the template's own; see AsyncAttrs and On.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, event string, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func OnAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	interaction, err := on(selfTargeted(value), args...)
	if err != nil {
		return attrsError(err)
	}

	// After the event come the endpoint and its options.
	targeted := true
	if _, isEvent := value.(string); isEvent && len(args) > 0 {
		targeted = hasTargetArg(args[0], args[1:]...)
	}

	if !targeted {
		interaction.Target = connector.TargetSelf
	}

	return renderAttrsFor(runtime, interaction)
}

// StreamAttrs returns the attributes of an SSE-backed interaction for an
// element of the template's own; see AsyncAttrs.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func StreamAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionStream, value, runtime, args...)
}

// RefreshAttrs returns the attributes of a refresh control for an element
// of the template's own, such as a link; see AsyncAttrs.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func RefreshAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionRefresh, value, runtime, args...)
}

// SubmitAttrs returns the attributes of a form that submits to the
// endpoint and swaps the response into its target, such as
// <form method="post" action="{{ .Action }}" {{ submitAttrs runtime .Action "target" "#comments" "swap" "outerHTML" }}>:
// hx-post for htmx, up-submit for Unpoly, and a data-turbo-frame for a
// target with Turbo. Without a target or id, the form itself is the
// target. "method" picks another method, such as "delete"; "include",
// "pushURL" and "replaceURL" are written in the connector's syntax.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func SubmitAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionSubmit, value, runtime, args...)
}

// LiveAttrs returns the attributes of an input that requests the endpoint
// as the user types, such as a search box:
// <input type="search" name="q" {{ liveAttrs runtime "/search" "target" "#results" "delay" "300ms" "pushURL" "true" }}>.
// htmx gets hx-get with an input trigger, Unpoly up-autosubmit with
// up-watch-delay; Turbo, which has none, data attributes for a script.
// "include" sends the values of other elements, such as "closest form".
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, endpoint string, params ...any) html/template.HTMLAttr
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, interaction github.com/donseba/go-partial/exp/interactions.Interaction) html/template.HTMLAttr
func LiveAttrs(runtime *partial.Runtime, value any, args ...any) template.HTMLAttr {
	return Attrs(connector.InteractionLive, value, runtime, args...)
}

// ClientAttrs returns the connector's attributes for how its client library
// treats a link or form of the template's own, or the elements in a body or
// container (see connector.Attr), from key/value pairs: "confirm" asks a
// question first, "boost" is "on" or "off", "target" and "swap" say where
// and how the response is swapped in:
// <a href="/logout" {{ clientAttrs runtime "confirm" "Log out?" "boost" "off" }}>.
//
// For settings of a whole page, "inherit" "true" gives them to the
// element's descendants, "disableWhileRequest" names the elements disabled
// while a request is under way, and "keepOnServerError" "true" leaves the
// page as it is on a 5xx response:
//
//	<body {{ clientAttrs runtime "inherit" "true" "boost" "on" "target" "#content" "swap" "outerHTML"
//	        "disableWhileRequest" "find button[type='submit']" "keepOnServerError" "true" }}>
//
// "submitConfirm" asks a question before a submit button sends its form,
// with the browser client of this package (ClientScript) where the client
// library cannot: <button type="submit" {{ clientAttrs runtime "submitConfirm" "Delete for good?" }}>.
//
// "reloadOnError" "true" makes a link or form whose request gets a 5xx
// status, or no answer, load as a whole page, so the visitor sees what went
// wrong rather than nothing, also with "keepOnServerError": a link is
// followed and a GET form submitted again by the browser itself. A POST,
// PUT, PATCH or DELETE form is never sent twice: the server's HTML error
// page replaces the page; without one the page stays and
// partial:reload-on-error:failed is dispatched on the form. It is the same
// attribute with every connector (connector.ReloadOnErrorAttr), which the
// browser client of this package reads; on the body it applies to every
// link and form.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, pairs ...any) html/template.HTMLAttr
func ClientAttrs(runtime *partial.Runtime, pairs ...any) template.HTMLAttr {
	vals, err := values(pairs...)
	if err != nil {
		return attrsError(err)
	}

	attr := connector.Attr{
		Confirm:             vals["confirm"],
		Target:              vals["target"],
		Swap:                vals["swap"],
		DisableWhileRequest: vals["disableWhileRequest"],
		SubmitConfirm:       vals["submitConfirm"],
	}

	for _, flag := range []struct {
		name  string
		value *bool
	}{
		{"inherit", &attr.Inherit},
		{"keepOnServerError", &attr.KeepOnServerError},
		{"reloadOnError", &attr.ReloadOnError},
	} {
		switch strings.ToLower(vals[flag.name]) {
		case "", "false", "off":
		case "true", "on":
			*flag.value = true
		default:
			return attrsError(fmt.Errorf("clientAttrs: %s must be true or false, got %q", flag.name, vals[flag.name]))
		}
	}

	switch strings.ToLower(vals["boost"]) {
	case "":
	case "on", "true":
		attr.Boost = connector.BoostOn
	case "off", "false":
		attr.Boost = connector.BoostOff
	default:
		return attrsError(fmt.Errorf("clientAttrs: boost must be on or off, got %q", vals["boost"]))
	}

	return clientAttrs(runtime, attr)
}

// NoBoost returns the attribute that makes the browser load a link or
// submit a form itself, such as hx-boost="false" with htmx or
// data-turbo="false" with Turbo: <a href="/download" {{ noBoost runtime }}>.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime) html/template.HTMLAttr
func NoBoost(runtime *partial.Runtime) template.HTMLAttr {
	return clientAttrs(runtime, connector.Attr{Boost: connector.BoostOff})
}

// ConfirmAttr returns the attribute that asks question before a link is
// followed or a form submitted, such as hx-confirm with htmx:
// <button {{ confirmAttr runtime "Delete this page?" }}>.
//
// go-doc:sig func(runtime *github.com/donseba/go-partial.Runtime, question string) html/template.HTMLAttr
func ConfirmAttr(runtime *partial.Runtime, question string) template.HTMLAttr {
	return clientAttrs(runtime, connector.Attr{Confirm: question})
}

func clientAttrs(runtime *partial.Runtime, attr connector.Attr) template.HTMLAttr {
	if runtime == nil {
		return attrsError(fmt.Errorf("go-partial interaction runtime is not configured"))
	}

	attrs := connector.Attrs(runtime.Connector(), attr)
	if len(attrs) == 0 {
		return ""
	}

	return template.HTMLAttr(" " + renderAttrs(attrs))
}

// Attrs returns the connector attributes of an interaction, without the
// wrapper element Render adds, for an element of the template's own; the
// attributes start with a space. Without a target or id, the interaction
// targets the element it is placed on (connector.TargetSelf). An invalid
// interaction returns a data-partial-error attribute with the error.
func Attrs(kind connector.InteractionKind, value any, runtime *partial.Runtime, args ...any) template.HTMLAttr {
	interaction, err := fromValue(kind, selfTargeted(value), args...)
	if err != nil {
		return attrsError(err)
	}

	if !hasTargetArg(value, args...) {
		interaction.Target = connector.TargetSelf
	}

	return renderAttrsFor(runtime, interaction)
}

// selfTargeted is an Interaction value with its own element as the target,
// unless it names a target or id.
func selfTargeted(value any) any {
	interaction, ok := value.(Interaction)
	if !ok || interaction.interaction.Target != "" || interaction.interaction.ID != "" {
		return value
	}

	interaction.interaction.Target = connector.TargetSelf
	return interaction
}

// hasTargetArg reports whether an interaction names its target or id, so
// it does not target its own element.
func hasTargetArg(value any, args ...any) bool {
	switch v := value.(type) {
	case Interaction:
		return true
	case string:
		for i := 0; i+1 < len(args); i += 2 {
			if key, ok := args[i].(string); ok && (key == "target" || key == "id") {
				return true
			}
		}

		return false
	case connector.Interaction:
		return v.Target != "" || v.ID != ""
	default:
		return true
	}
}

func renderAttrsFor(runtime *partial.Runtime, interaction connector.Interaction) template.HTMLAttr {
	if runtime == nil {
		return attrsError(fmt.Errorf("go-partial interaction runtime is not configured"))
	}

	attrs := runtime.Connector().InteractionAttrs(interaction)
	return template.HTMLAttr(" " + renderAttrs(attrs))
}

func attrsError(err error) template.HTMLAttr {
	return template.HTMLAttr(` data-partial-error="` + escapeAttr(err.Error()) + `"`)
}

func Render(kind connector.InteractionKind, value any, runtime *partial.Runtime, args ...any) template.HTML {
	interaction, err := fromValue(kind, value, args...)
	if err != nil {
		return escapedError(err)
	}
	return renderInteraction(runtime, interaction)
}

func renderOn(runtime *partial.Runtime, value any, args ...any) template.HTML {
	interaction, err := on(value, args...)
	if err != nil {
		return escapedError(err)
	}
	return renderInteraction(runtime, interaction)
}

func renderInteraction(runtime *partial.Runtime, interaction connector.Interaction) template.HTML {
	if runtime == nil {
		return escapedError(fmt.Errorf("go-partial interaction runtime is not configured"))
	}
	conn := runtime.Connector()
	attrs := conn.InteractionAttrs(interaction)
	data := Data{Runtime: runtime, Interaction: interaction, Attrs: attrs}
	out, err := runtime.RenderWith(RenderKindInteraction, string(interaction.Kind), data, func(ctx *partial.RenderContext) (template.HTML, error) {
		return DefaultMarkupRenderer()(runtime, interaction, attrs)
	})
	if err != nil {
		return escapedError(err)
	}
	return out
}

// DefaultMarkupRenderer renders small, unstyled wrappers around connector attributes.
func DefaultMarkupRenderer() MarkupRenderer {
	return func(runtime *partial.Runtime, interaction connector.Interaction, attrs map[string]string) (template.HTML, error) {
		switch interaction.Kind {
		case connector.InteractionPrefetch:
			return template.HTML(`<link ` + renderAttrs(attrs) + `>`), nil
		case connector.InteractionRefresh:
			return template.HTML(`<button type="button" id="` + escapeAttr(interaction.ID) + `" ` + renderAttrs(attrs) + `>` + placeholder(interaction) + `</button>`), nil
		default:
			element := "div"
			if _, ok := attrs["src"]; ok {
				if _, lazy := attrs["loading"]; lazy {
					element = "turbo-frame"
				}
			}
			return template.HTML(`<` + element + ` id="` + escapeAttr(interaction.ID) + `" ` + renderAttrs(attrs) + `>` + placeholder(interaction) + `</` + element + `>`), nil
		}
	}
}

func escapedError(err error) template.HTML {
	return template.HTML(template.HTMLEscapeString(err.Error()))
}

type interactionConfig interface {
	Interaction() connector.Interaction
}

func fromValue(kind connector.InteractionKind, value any, args ...any) (connector.Interaction, error) {
	switch v := value.(type) {
	case string:
		if kind == connector.InteractionPoll && len(args) == 1 {
			args = []any{"every", args[0]}
		}
		return build(kind, "", v, args...)
	case interactionConfig:
		if isNil(v) {
			return connector.Interaction{}, fmt.Errorf("interaction is nil")
		}
		return normalize(v.Interaction()), nil
	case connector.Interaction:
		return normalize(v), nil
	default:
		return connector.Interaction{}, fmt.Errorf("interaction helper expects an endpoint string or interaction config, got %T", value)
	}
}

func on(value any, args ...any) (connector.Interaction, error) {
	event, ok := value.(string)
	if !ok {
		return fromValue(connector.InteractionOn, value, args...)
	}
	if len(args) == 0 {
		return connector.Interaction{}, fmt.Errorf("on expects an endpoint when the first argument is an event")
	}
	endpoint, ok := args[0].(string)
	if !ok {
		return connector.Interaction{}, fmt.Errorf("on endpoint must be string, got %T", args[0])
	}

	interaction, err := build(connector.InteractionOn, event, endpoint, args[1:]...)
	if err != nil {
		return connector.Interaction{}, err
	}
	interaction.Trigger = event
	if interaction.Options == nil {
		interaction.Options = make(map[string]string)
	}
	if _, ok := interaction.Options["from"]; !ok {
		interaction.Options["from"] = "body"
	}
	if _, ok := interaction.Options["placeholder"]; !ok {
		interaction.Placeholder = ""
	}
	return normalize(interaction), nil
}

func build(kind connector.InteractionKind, name string, endpoint string, args ...any) (connector.Interaction, error) {
	vals, err := values(args...)
	if err != nil {
		return connector.Interaction{}, err
	}

	resolvedURL := resolveURL(endpoint, vals)
	if name == "" {
		name = vals["name"]
	}

	return normalize(connector.Interaction{
		Kind:        kind,
		ID:          vals["id"],
		URL:         resolvedURL,
		Target:      vals["target"],
		Swap:        vals["swap"],
		Trigger:     vals["trigger"],
		Interval:    vals["every"],
		Placeholder: vals["placeholder"],
		Select:      vals["select"],
		Name:        name,
		Params:      vals,
		Options:     vals,
		Attrs:       attributeOptions(vals),
	}), nil
}

// attributePrefixes start the option names of the endpoint form that are
// written on the element as given, such as "hx-indicator" or
// "data-section".
var attributePrefixes = []string{"hx-", "up-", "data-", "aria-"}

// attributeOptions returns the options that are attributes; see
// attributePrefixes.
func attributeOptions(vals map[string]string) map[string]string {
	var attrs map[string]string

	for key, value := range vals {
		for _, prefix := range attributePrefixes {
			if !strings.HasPrefix(key, prefix) {
				continue
			}

			if attrs == nil {
				attrs = make(map[string]string)
			}

			attrs[key] = value
			break
		}
	}

	return attrs
}

func normalize(interaction connector.Interaction) connector.Interaction {
	if interaction.Options == nil {
		interaction.Options = make(map[string]string)
	}
	if interaction.Kind == connector.InteractionOn {
		if interaction.Trigger == "" {
			interaction.Trigger = interaction.Name
		}
		if _, ok := interaction.Options["from"]; !ok {
			interaction.Options["from"] = "body"
		}
	}
	if interaction.ID == "" {
		idBase := interaction.URL
		if interaction.Name != "" {
			idBase = interaction.Name
		}
		interaction.ID = string(interaction.Kind) + "-" + sanitizeID(idBase)
	}
	if interaction.Target == "" {
		interaction.Target = "#" + interaction.ID
	}
	if _, placeholderSet := interaction.Options["placeholder"]; interaction.Placeholder == "" && !placeholderSet {
		if interaction.Kind == connector.InteractionOn {
			interaction.Placeholder = ""
		} else {
			interaction.Placeholder = "Loading..."
		}
	}
	return interaction
}

func resolveURL(endpoint string, values map[string]string) string {
	resolved := strings.TrimSpace(endpoint)
	if resolved == "" {
		resolved = "/"
	}
	if !strings.HasPrefix(resolved, "/") && !strings.HasPrefix(resolved, "http://") && !strings.HasPrefix(resolved, "https://") {
		resolved = "/" + resolved
	}

	for key, value := range values {
		resolved = strings.ReplaceAll(resolved, ":"+key, url.PathEscape(value))
	}

	return resolved
}

func values(args ...any) (map[string]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("interaction helpers expect key/value pairs")
	}

	values := make(map[string]string, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("interaction option key must be string, got %T", args[i])
		}
		values[key] = fmt.Sprint(args[i+1])
	}
	return values, nil
}

func renderAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}

	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	wrote := false
	for _, key := range keys {
		if key == "id" || strings.HasPrefix(key, "__") {
			continue
		}
		if wrote {
			b.WriteByte(' ')
		}
		b.WriteString(escapeAttr(key))
		b.WriteString(`="`)
		b.WriteString(escapeAttr(attrs[key]))
		b.WriteByte('"')
		wrote = true
	}
	return b.String()
}

func placeholder(interaction connector.Interaction) string {
	return template.HTMLEscapeString(interaction.Placeholder)
}

func escapeAttr(value string) string {
	return template.HTMLEscapeString(value)
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "content"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "content"
	}
	return out
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func nameFromEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if idx := strings.IndexAny(endpoint, "?#"); idx >= 0 {
		endpoint = endpoint[:idx]
	}
	endpoint = strings.Trim(endpoint, "/")
	if endpoint == "" {
		return ""
	}
	if idx := strings.LastIndex(endpoint, "/"); idx >= 0 {
		endpoint = endpoint[idx+1:]
	}

	parts := strings.FieldsFunc(endpoint, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == ':'
	})
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		upperNext := true
		for _, r := range part {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				upperNext = true
				continue
			}
			if upperNext {
				b.WriteRune(unicode.ToUpper(r))
				upperNext = false
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
