// Package logger adapts go-partial diagnostic events to log/slog.
package logger

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"reflect"

	partial "github.com/donseba/go-partial"
)

type (
	config struct {
		minLevel partial.EventLevel
		// levels, when set, are the only levels written.
		levels map[partial.EventLevel]bool
	}

	// Option configures a logger sink.
	Option func(*config)
)

const (
	// EventTemplateLog identifies diagnostic events emitted from templates.
	EventTemplateLog = "ext.logger.template"
)

// packagePath is the import path of this package, which its render stages
// name as their partial.RenderStageHooks.Package.
var packagePath = reflect.TypeFor[config]().PkgPath()

// WithMinLevel ignores events below level.
func WithMinLevel(level partial.EventLevel) Option {
	return func(cfg *config) {
		if level != "" {
			cfg.minLevel = level
		}
	}
}

// WithLevels writes only events of the given levels, whatever the minimum
// level.
func WithLevels(levels ...partial.EventLevel) Option {
	return func(cfg *config) {
		cfg.levels = make(map[partial.EventLevel]bool, len(levels))
		for _, level := range levels {
			cfg.levels[level] = true
		}
	}
}

// writes reports whether the sink writes events of level.
func (cfg config) writes(level partial.EventLevel) bool {
	if cfg.levels != nil {
		return cfg.levels[level]
	}

	return levelRank(level) >= levelRank(cfg.minLevel)
}

// FuncMap returns the optional logger template helper for static parsing and docs.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"logger": Logger,
	}
}

// Stage gives templates the request-scoped logger helper. It is made only
// for the templates that call it.
func Stage() partial.RenderStage {
	return partial.RenderStageHooks{
		Package:   packagePath,
		FuncNames: []string{"logger"},
		ResolveFunc: func(ctx *partial.RenderContext, name string) (any, bool) {
			if name != "logger" {
				return nil, false
			}

			return func(message string, pairs ...any) string {
				return Logger(ctx, message, pairs...)
			}, true
		},
	}
}

// Logger emits an info-level diagnostic event from template code.
//
// go-doc:sig func(message string, pairs ...any) string
func Logger(ctx *partial.RenderContext, message string, pairs ...any) string {
	if ctx == nil {
		return ""
	}
	if message == "" {
		message = "template log"
	}
	ctx.Emit(partial.Event{
		Kind:    EventTemplateLog,
		Level:   partial.EventInfo,
		Message: message,
		Fields:  templateLogFields(pairs...),
	})
	return ""
}

// Sink returns an event sink that writes diagnostic events to slog, those
// of level warn and above unless options say otherwise. A nil log writes to
// slog.Default() as it is when each event is written, so a default set
// later is used.
func Sink(log *slog.Logger, options ...Option) partial.EventSink {
	cfg := config{minLevel: partial.EventWarn}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}

	if log != nil {
		log = log.WithGroup("partial")
	}
	return partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
		if !cfg.writes(event.Level) {
			return
		}
		out := orDefault(log)
		attrs := []slog.Attr{
			slog.String("kind", event.Kind),
			slog.String("level", string(event.Level)),
		}
		if event.PartialID != "" {
			attrs = append(attrs, slog.String("partial", event.PartialID))
		}
		if event.ParentID != "" {
			attrs = append(attrs, slog.String("parent", event.ParentID))
		}
		if event.Name != "" {
			attrs = append(attrs, slog.String("name", event.Name))
		}
		if event.TraceID != "" {
			attrs = append(attrs, slog.String("trace", event.TraceID))
		}
		if event.Error != nil {
			attrs = append(attrs, slog.Any("error", event.Error))
		}
		for key, value := range event.Fields {
			attrs = append(attrs, slog.Any(key, value))
		}

		message := event.Message
		if message == "" {
			message = event.Kind
		}
		logCtx := context.Background()
		if ctx != nil && ctx.Context != nil {
			logCtx = ctx.Context
		}
		out.LogAttrs(logCtx, slogLevel(event.Level), message, attrs...)
	})
}

func slogLevel(level partial.EventLevel) slog.Level {
	switch level {
	case partial.EventDebug:
		return slog.LevelDebug
	case partial.EventInfo:
		return slog.LevelInfo
	case partial.EventWarn:
		return slog.LevelWarn
	case partial.EventError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func levelRank(level partial.EventLevel) int {
	switch level {
	case partial.EventDebug:
		return 0
	case partial.EventInfo:
		return 1
	case partial.EventWarn:
		return 2
	case partial.EventError:
		return 3
	default:
		return 1
	}
}

func templateLogFields(pairs ...any) map[string]any {
	fields := map[string]any{"source": "template"}
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok || key == "" {
			key = fmt.Sprintf("arg%d", i)
		}
		if i+1 >= len(pairs) {
			fields[key] = ""
			continue
		}
		fields[key] = pairs[i+1]
	}
	return fields
}

// orDefault returns log, or for a nil log slog.Default() as it is when an
// event is written, with the group of go-partial's attributes.
func orDefault(log *slog.Logger) *slog.Logger {
	if log != nil {
		return log
	}

	return slog.Default().WithGroup("partial")
}
