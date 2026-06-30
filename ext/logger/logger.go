// Package logger adapts go-partial diagnostic events to log/slog.
package logger

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"

	partial "github.com/donseba/go-partial"
)

type (
	config struct {
		minLevel partial.EventLevel
	}

	// Option configures a logger sink.
	Option func(*config)
)

const (
	// EventTemplateLog identifies diagnostic events emitted from templates.
	EventTemplateLog = "ext.logger.template"
)

// WithMinLevel ignores events below level.
func WithMinLevel(level partial.EventLevel) Option {
	return func(cfg *config) {
		if level != "" {
			cfg.minLevel = level
		}
	}
}

// FuncMap returns the optional logger template helper for static parsing and docs.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"logger": Logger,
	}
}

// Stage installs the request-scoped logger template helper.
func Stage() partial.RenderStage {
	return partial.RenderStageHooks{
		PrepareFunc: func(ctx *partial.RenderContext) (*partial.RenderContext, error) {
			ctx.SetFunc("logger", func(message string, pairs ...any) string {
				return Logger(ctx, message, pairs...)
			})
			return ctx, nil
		},
	}
}

// Renderer installs the request-scoped logger template helper.
//
// Deprecated: use Stage.
func Renderer() partial.RenderStage {
	return Stage()
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

// Sink returns an event sink that writes diagnostic events to slog.
func Sink(log *slog.Logger, options ...Option) partial.EventSink {
	cfg := config{minLevel: partial.EventWarn}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if log == nil {
		log = slog.Default()
	}
	log = log.WithGroup("partial")
	return partial.EventSinkFunc(func(ctx *partial.RenderContext, event partial.Event) {
		if levelRank(event.Level) < levelRank(cfg.minLevel) {
			return
		}
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
		log.LogAttrs(logCtx, slogLevel(event.Level), message, attrs...)
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
