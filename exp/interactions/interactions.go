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

// FuncMap returns the optional interaction template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"async":    Async,
		"reveal":   Reveal,
		"poll":     Poll,
		"on":       On,
		"stream":   Stream,
		"prefetch": Prefetch,
		"refresh":  Refresh,
	}
}

// Stage returns a partial render stage for interaction markup.
func Stage(markup ...MarkupRenderer) partial.RenderStage {
	renderer := DefaultMarkupRenderer()
	if len(markup) > 0 && markup[0] != nil {
		renderer = markup[0]
	}

	return partial.RenderStageHooks{
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
	if conn == nil {
		conn = connector.NewPartial(nil)
	}
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
		Name:        name,
		Params:      vals,
		Options:     vals,
	}), nil
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
