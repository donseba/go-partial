// Package flash provides experimental request-scoped flash message helpers.
package flash

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"unicode"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/internal/templateutil"
)

//go:embed *.gohtml
var defaultTemplates embed.FS

type (
	// Level describes the intent of a flash message.
	Level string

	// Message is one request-scoped flash message.
	Message struct {
		Level Level
		Text  string
	}

	// Data is passed to the flash template.
	Data struct {
		Messages []Message
		TargetID string
		// RequestErrorAttrs are the attributes of the flash target that
		// tell the browser client how to show failed requests, as the
		// stage's WithRequestErrors configures them, each after a space;
		// empty without. A flash target template writes them on its
		// element, as the default one does; see also flashTargetAttrs.
		RequestErrorAttrs template.HTMLAttr
	}

	// RequestErrors configures how the browser client (ClientScript) shows
	// requests that fail; see WithRequestErrors.
	RequestErrors struct {
		// Text is the error message shown for a failed request, such as
		// "The request failed. Please try again."; a template gives one of
		// its own, such as a translated one, as the argument of
		// {{ flashTargetAttrs }}.
		Text string
		// Replace keeps one request error at a time: a new one replaces the
		// last, and the next request removes it.
		Replace bool
		// Body shows the text/plain sentence of a refused request (4xx),
		// such as Respond writes for requests of no client library,
		// instead of Text; htmx does not swap that body.
		Body bool
		// SkipFlash shows nothing for a failed response that brings its own
		// flash message, such as those of Respond, RenderOOB and ext/errors'
		// WithFlash.
		SkipFlash bool
	}

	// Store holds flash messages for one request or app-owned session handoff.
	//
	// Store is safe for concurrent Add, Messages, and Drain calls, but most
	// applications should still treat a store as request/session-owned state.
	Store struct {
		mu       sync.Mutex
		messages []Message
	}

	options struct {
		partial       *partial.Partial
		targetPartial *partial.Partial
		targetID      string
		requestErrors *RequestErrors
	}

	// Option configures the flash renderer.
	Option func(*options)
)

const (
	LevelSuccess Level = "success"
	LevelInfo    Level = "info"
	LevelWarn    Level = "warn"
	LevelError   Level = "error"
)

var storeContextKey = contextKey{}

type contextKey struct{}

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[contextKey]().PkgPath()

const defaultTargetID = "flash-messages"

var unsafeTokenChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// FuncMap returns placeholders for flash template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"flash":         Flash,
		"flashTarget":   FlashTarget,
		"flashes":       Flashes,
		"hasFlashes":    HasFlashes,
		"flashTemplate": Template,

		"flashTargetAttrs": TargetAttrs,
	}
}

// Template renders, for each level, the default flash template as a
// <template data-flash-template="level"> for the browser client, which
// clones it for the messages it shows itself; see Stage for the template
// of the application's flash template. It renders nothing in a partial
// rendered out of band, such as a flash target that a response adds its
// messages to: the page has the templates already.
//
// go-doc:sig func(levels ...string) html/template.HTML
func Template(args ...any) template.HTML {
	var ctx *partial.RenderContext

	levels := make([]string, 0, len(args))
	for _, arg := range args {
		switch value := arg.(type) {
		case *partial.RenderContext:
			ctx = value
		case string:
			levels = append(levels, value)
		case Level:
			levels = append(levels, string(value))
		}
	}

	if ctx == nil {
		return ""
	}

	return renderTemplates(ctx, defaultPartial("flash", "default.gohtml"), defaultTargetID, levels)
}

// TargetAttrs returns the attributes of an element of the template's
// own that is the flash target: its id and, with the flash Stage's
// WithRequestErrors, the attributes that tell the browser client how to
// show failed requests, such as data-flash-request-error="..."; a text
// argument replaces the stage's text, such as a translated one:
//
//	<div {{ flashTargetAttrs (t "The request failed.") }} {{ oobAttr "beforeend" }} class="toasts">
//
// Without the stage, the id is the default flash target's and a text
// argument is the only request error setting.
//
// go-doc:sig func(text ...string) html/template.HTMLAttr
func TargetAttrs(args ...any) template.HTMLAttr {
	var text []string

	for _, arg := range args {
		if value, ok := arg.(string); ok {
			text = append(text, value)
		}
	}

	return targetAttrs(defaultTargetID, nil, text)
}

// Flash renders flash messages for a render context with the default template.
//
// go-doc:sig func() html/template.HTML
func Flash(ctx ...*partial.RenderContext) template.HTML {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil {
		return ""
	}
	return renderMessages(renderCtx, defaultPartial("flash", "default.gohtml"), defaultTargetID)
}

// FlashTarget renders the default flash target container.
//
// go-doc:sig func() html/template.HTML
func FlashTarget(ctx ...*partial.RenderContext) template.HTML {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil {
		return ""
	}
	return renderTarget(renderCtx, defaultPartial("flash-target", "target.gohtml"), defaultTargetID, nil)
}

// Flashes returns request-scoped flash messages for a render context.
//
// go-doc:sig func() []github.com/donseba/go-partial/exp/flash.Message
func Flashes(ctx ...*partial.RenderContext) []Message {
	renderCtx := firstRenderContext(ctx)
	if renderCtx == nil {
		return nil
	}
	return Messages(renderCtx.Context)
}

// HasFlashes reports whether a render context has request-scoped flash messages.
//
// go-doc:sig func() bool
func HasFlashes(ctx ...*partial.RenderContext) bool {
	return len(Flashes(ctx...)) > 0
}

// Stage gives templates the flash helpers, made only for the templates that
// call them.
func Stage(opts ...Option) partial.RenderStage {
	cfg := options{
		partial:       defaultPartial("flash", "default.gohtml"),
		targetPartial: defaultPartial("flash-target", "target.gohtml"),
		targetID:      defaultTargetID,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.partial == nil {
		cfg.partial = defaultPartial("flash", "default.gohtml")
	}
	if cfg.targetPartial == nil {
		cfg.targetPartial = defaultPartial("flash-target", "target.gohtml")
	}
	if cfg.targetID == "" {
		cfg.targetID = defaultTargetID
	}

	return partial.RenderStageHooks{
		Package:   packagePath,
		FuncNames: []string{"flash", "flashTarget", "flashes", "hasFlashes", "flashTemplate", "flashTargetAttrs"},
		// The helpers are made only for the templates that call them.
		ResolveFunc: func(ctx *partial.RenderContext, name string) (any, bool) {
			switch name {
			case "flashes":
				return func() []Message {
					return Messages(ctx.Context)
				}, true
			case "hasFlashes":
				return func() bool {
					return Has(ctx.Context)
				}, true
			case "flash":
				return func() template.HTML {
					return renderMessages(ctx, cfg.partial, cfg.targetID)
				}, true
			case "flashTarget":
				return func() template.HTML {
					return renderTarget(ctx, cfg.targetPartial, cfg.targetID, cfg.requestErrors)
				}, true
			case "flashTargetAttrs":
				return func(text ...string) template.HTMLAttr {
					return targetAttrs(cfg.targetID, cfg.requestErrors, text)
				}, true
			case "flashTemplate":
				return func(levels ...string) template.HTML {
					return renderTemplates(ctx, cfg.partial, cfg.targetID, levels)
				}, true
			case oobFuncName:
				// For RenderOOB, which code calls: the out-of-band messages
				// with the stage's template and target.
				return func(messages []Message) template.HTML {
					return renderOOB(ctx, cfg.partial, cfg.targetID, messages)
				}, true
			default:
				return nil, false
			}
		},
	}
}

// oobFuncName is the name under which the stage gives RenderOOB its
// messages; templates cannot call it, as no template function starts with
// an underscore.
const oobFuncName = "_flashOOB"

// textMarker stands for the text of a message in the templates of
// flashTemplate; the browser client replaces it.
const textMarker = "__partial_flash_text__"

// renderTemplates renders the message template of each level, with the text
// marker as its text, as a <template data-flash-template> for the browser
// client; without levels, those of the four levels. A partial rendered out
// of band gets none: the page has them.
func renderTemplates(ctx *partial.RenderContext, p *partial.Partial, targetID string, levels []string) template.HTML {
	if ctx == nil || ctx.Runtime == nil || p == nil {
		return ""
	}

	if ctx.Partial != nil && ctx.Partial.IsOOB() {
		return ""
	}

	if len(levels) == 0 {
		levels = []string{string(LevelSuccess), string(LevelInfo), string(LevelWarn), string(LevelError)}
	}

	var b strings.Builder

	for _, level := range levels {
		level := normalizeLevel(Level(level))
		markup := renderPartial(ctx, p, Data{Messages: []Message{{Level: level, Text: textMarker}}, TargetID: targetID})

		b.WriteString(`<template data-flash-template="`)
		b.WriteString(template.HTMLEscapeString(string(level)))
		b.WriteString(`">`)
		b.WriteString(string(markup))
		b.WriteString(`</template>`)
	}

	return template.HTML(b.String())
}

// renderOOB renders messages with p as an out-of-band addition to the flash
// target targetID, in the syntax of the render's connector.
func renderOOB(ctx *partial.RenderContext, p *partial.Partial, targetID string, messages []Message) template.HTML {
	if ctx == nil || ctx.Runtime == nil || p == nil || len(messages) == 0 {
		return ""
	}

	inner := renderPartial(ctx, p, Data{Messages: normalized(messages), TargetID: targetID})

	var b strings.Builder

	b.WriteString(`<div`)
	templateutil.WriteAttr(&b, "id", targetID)
	b.WriteString(string(templateutil.HTMLAttrs(connector.OOBAttrs(ctx.Runtime.Connector(), "beforeend"))))
	b.WriteByte('>')
	b.WriteString(string(inner))
	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

// normalized returns messages with their levels normalized and without
// those without text, as Store.Add keeps them.
func normalized(messages []Message) []Message {
	kept := make([]Message, 0, len(messages))
	for _, message := range messages {
		if strings.TrimSpace(message.Text) == "" {
			continue
		}

		message.Level = normalizeLevel(message.Level)
		kept = append(kept, message)
	}

	return kept
}

// RenderOOB renders messages as an addition to the flash target, sent out
// of band with any response, with the flash template and target of the
// flash Stage of the render: a render stage that answers a request itself,
// such as ext/errors with WithFlash, adds them to the page the way
// the page's own flash messages appear. It reports false when the render
// has no flash stage.
//
//	if out, ok := flash.RenderOOB(ctx, flash.Error("The request failed.")); ok {
//		return out, nil
//	}
func RenderOOB(ctx *partial.RenderContext, messages ...Message) (template.HTML, bool) {
	if ctx == nil {
		return "", false
	}

	fn, ok := ctx.ResolveFunc(oobFuncName)
	if !ok {
		return "", false
	}

	render, ok := fn.(func([]Message) template.HTML)
	if !ok {
		return "", false
	}

	return render(messages), true
}

// WithTemplate renders flash messages with a user template from the active
// partial tree filesystem.
func WithTemplate(path string) Option {
	return func(opts *options) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		opts.partial = partial.NewID("flash", path)
	}
}

// WithPartial renders flash messages with a user-provided partial.
func WithPartial(p *partial.Partial) Option {
	return func(opts *options) {
		if p != nil {
			opts.partial = p
		}
	}
}

// WithTargetID changes the DOM ID used by the default flash target and the data
// passed to flash templates.
func WithTargetID(id string) Option {
	return func(opts *options) {
		if normalized := normalizeTargetID(id); normalized != "" {
			opts.targetID = normalized
		}
	}
}

// WithTargetTemplate renders flashTarget with a user template from the active
// partial tree filesystem.
func WithTargetTemplate(path string) Option {
	return func(opts *options) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		opts.targetPartial = partial.NewID("flash-target", path)
	}
}

// WithRequestErrors makes the flash target tell the browser client
// (ClientScript) to show failed requests as errors, as errors configures:
// {{ flashTarget }} and {{ flashTargetAttrs }} write the attributes the
// client reads, such as data-flash-request-error="The request failed.", and
// flash target templates get them as Data.RequestErrorAttrs.
//
//	flash.Stage(flash.WithRequestErrors(flash.RequestErrors{
//		Text:      "The request failed. Please try again.",
//		Replace:   true,
//		Body:      true,
//		SkipFlash: true,
//	}))
func WithRequestErrors(errors RequestErrors) Option {
	return func(opts *options) {
		opts.requestErrors = &errors
	}
}

// requestErrorAttrs writes the attributes of the flash target that
// configure the request errors of the browser client: none without
// errors, and data-flash-request-error with the text, which a non-empty
// text argument replaces, and one attribute for each setting with.
func requestErrorAttrs(errors *RequestErrors, text []string) template.HTMLAttr {
	settings := RequestErrors{}
	if errors != nil {
		settings = *errors
	}

	for _, value := range text {
		if value != "" {
			settings.Text = value
		}
	}

	if errors == nil && settings.Text == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString(` data-flash-request-error="`)
	b.WriteString(template.HTMLEscapeString(settings.Text))
	b.WriteByte('"')

	for _, flag := range []struct {
		set  bool
		name string
	}{
		{settings.Replace, "data-flash-request-error-replace"},
		{settings.Body, "data-flash-request-error-body"},
		{settings.SkipFlash, "data-flash-request-error-skip-flash"},
	} {
		if flag.set {
			b.WriteByte(' ')
			b.WriteString(flag.name)
		}
	}

	return template.HTMLAttr(b.String())
}

// targetAttrs writes the id of the flash target and its request error
// attributes, after a space.
func targetAttrs(targetID string, errors *RequestErrors, text []string) template.HTMLAttr {
	id := ` id="` + template.HTMLEscapeString(targetID) + `"`

	return template.HTMLAttr(id) + requestErrorAttrs(errors, text)
}

// WithTargetPartial renders flashTarget with a user-provided partial.
func WithTargetPartial(p *partial.Partial) Option {
	return func(opts *options) {
		if p != nil {
			opts.targetPartial = p
		}
	}
}

// New creates a flash message with the provided level and text.
func New(level Level, text string) Message {
	return Message{Level: normalizeLevel(level), Text: text}
}

// Success creates a success flash message.
func Success(text string) Message {
	return New(LevelSuccess, text)
}

// Info creates an informational flash message.
func Info(text string) Message {
	return New(LevelInfo, text)
}

// Warn creates a warning flash message.
func Warn(text string) Message {
	return New(LevelWarn, text)
}

// Error creates an error flash message.
func Error(text string) Message {
	return New(LevelError, text)
}

// Add stores messages on the context, creating a request store when needed.
func Add(ctx context.Context, messages ...Message) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(messages) == 0 {
		return ctx
	}
	store, ok := ctx.Value(storeContextKey).(*Store)
	if !ok || store == nil {
		store = NewStore()
		ctx = context.WithValue(ctx, storeContextKey, store)
	}
	store.Add(messages...)
	return ctx
}

// WithStore stores an app-owned flash store on the context.
func WithStore(ctx context.Context, store *Store) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil {
		store = NewStore()
	}
	return context.WithValue(ctx, storeContextKey, store)
}

// FromContext returns the flash store from ctx, if present.
func FromContext(ctx context.Context) *Store {
	if ctx == nil {
		return nil
	}
	store, _ := ctx.Value(storeContextKey).(*Store)
	return store
}

// Messages returns the current context flash messages.
func Messages(ctx context.Context) []Message {
	store := FromContext(ctx)
	if store == nil {
		return nil
	}
	return store.Messages()
}

// Has reports whether the context has flash messages.
func Has(ctx context.Context) bool {
	return len(Messages(ctx)) > 0
}

// Drain removes and returns the current context flash messages.
func Drain(ctx context.Context) []Message {
	store := FromContext(ctx)
	if store == nil {
		return nil
	}
	return store.Drain()
}

// NewStore creates an empty flash store.
func NewStore(messages ...Message) *Store {
	store := &Store{}
	store.Add(messages...)
	return store
}

// Add appends messages to the store.
func (s *Store) Add(messages ...Message) {
	if s == nil || len(messages) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, message := range messages {
		message.Level = normalizeLevel(message.Level)
		if strings.TrimSpace(message.Text) == "" {
			continue
		}
		s.messages = append(s.messages, message)
	}
}

// Messages returns a snapshot of the current messages.
func (s *Store) Messages() []Message {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), s.messages...)
}

// Drain removes and returns all messages.
func (s *Store) Drain() []Message {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := append([]Message(nil), s.messages...)
	s.messages = nil
	return messages
}

func renderMessages(ctx *partial.RenderContext, p *partial.Partial, targetID string) template.HTML {
	if ctx == nil || ctx.Runtime == nil || p == nil {
		return ""
	}
	messages := Messages(ctx.Context)
	if len(messages) == 0 {
		return ""
	}
	return renderPartial(ctx, p, Data{Messages: messages, TargetID: targetID})
}

func renderTarget(ctx *partial.RenderContext, p *partial.Partial, targetID string, errors *RequestErrors) template.HTML {
	if ctx == nil || ctx.Runtime == nil || p == nil {
		return ""
	}

	return renderPartial(ctx, p, Data{TargetID: targetID, RequestErrorAttrs: requestErrorAttrs(errors, nil)})
}

func renderPartial(ctx *partial.RenderContext, p *partial.Partial, data Data) template.HTML {
	view := p.Clone().SetDot(data)
	out, err := ctx.Runtime.RenderPartial(view)
	if err != nil {
		return template.HTML(template.HTMLEscapeString(fmt.Sprintf("error rendering flash: %v", err)))
	}
	return out
}

func defaultPartial(id string, templatePath string) *partial.Partial {
	fsys, err := fs.Sub(defaultTemplates, ".")
	if err != nil {
		fsys = defaultTemplates
	}
	return partial.NewID(id, templatePath).SetFileSystem(fsys)
}

func firstRenderContext(ctx []*partial.RenderContext) *partial.RenderContext {
	if len(ctx) == 0 {
		return nil
	}
	return ctx[0]
}

func normalizeLevel(level Level) Level {
	normalized := normalizeToken(string(level))
	switch Level(normalized) {
	case LevelSuccess, LevelInfo, LevelWarn, LevelError:
		return Level(normalized)
	case "":
		return LevelInfo
	default:
		return Level(normalized)
	}
}

func normalizeTargetID(id string) string {
	id = strings.TrimSpace(strings.TrimPrefix(id, "#"))
	id = normalizeToken(id)
	if id == "" {
		return ""
	}
	first := firstRune(id)
	if first == '_' || unicode.IsLetter(first) {
		return id
	}
	return "flash-" + id
}

func normalizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = unsafeTokenChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return value
}

func firstRune(value string) rune {
	for _, r := range value {
		return r
	}
	return 0
}
