// Package flash provides experimental request-scoped flash message helpers.
package flash

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"unicode"

	partial "github.com/donseba/go-partial"
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

const defaultTargetID = "flash-messages"

var unsafeTokenChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// FuncMap returns placeholders for flash template helpers.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"flash":       Flash,
		"flashTarget": FlashTarget,
		"flashes":     Flashes,
		"hasFlashes":  HasFlashes,
	}
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
	return renderTarget(renderCtx, defaultPartial("flash-target", "target.gohtml"), defaultTargetID)
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

// Stage installs flash template helpers.
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
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			ctx.SetFunc("flashes", func() []Message { return Messages(ctx.Context) })
			ctx.SetFunc("hasFlashes", func() bool { return Has(ctx.Context) })
			ctx.SetFunc("flash", func() template.HTML { return renderMessages(ctx, cfg.partial, cfg.targetID) })
			ctx.SetFunc("flashTarget", func() template.HTML { return renderTarget(ctx, cfg.targetPartial, cfg.targetID) })
			return ctx, nil
		},
	}
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

func renderTarget(ctx *partial.RenderContext, p *partial.Partial, targetID string) template.HTML {
	if ctx == nil || ctx.Runtime == nil || p == nil {
		return ""
	}
	return renderPartial(ctx, p, Data{TargetID: targetID})
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
