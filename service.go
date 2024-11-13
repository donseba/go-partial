package partial

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
)

var (
	// defaultPartialHeader is the default header used to determine which partial to render.
	defaultPartialHeader = "X-Partial"
)

type (
	Logger interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
		Error(msg string, args ...any)

		InfoContext(ctx context.Context, msg string, args ...any)
		WarnContext(ctx context.Context, msg string, args ...any)
		ErrorContext(ctx context.Context, msg string, args ...any)
	}

	Config struct {
		PartialHeader string
		UseCache      bool
		FuncMap       template.FuncMap
		Logger        Logger
		fs            fs.FS
	}

	Service struct {
		config *Config
		data   map[string]any
	}

	Layout struct {
		service    *Service
		filesystem fs.FS
		content    *Partial
		wrapper    *Partial
		data       map[string]any
	}
)

// NewService returns a new partial service.
func NewService(cfg *Config) *Service {
	if cfg.FuncMap == nil {
		cfg.FuncMap = DefaultTemplateFuncMap
	}

	if cfg.PartialHeader == "" {
		cfg.PartialHeader = defaultPartialHeader
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default().WithGroup("partial")
	}

	return &Service{
		config: cfg,
		data:   make(map[string]any),
	}
}

// NewLayout returns a new layout.
func (svc *Service) NewLayout() *Layout {
	return &Layout{
		service:    svc,
		data:       make(map[string]any),
		filesystem: svc.config.fs,
	}
}

// SetData sets the data for the Service.
func (svc *Service) SetData(data map[string]any) *Service {
	svc.data = data
	return svc
}

// AddData adds data to the Service.
func (svc *Service) AddData(key string, value any) *Service {
	svc.data[key] = value
	return svc
}

// FS sets the filesystem for the Layout.
func (l *Layout) FS(fs fs.FS) *Layout {
	l.filesystem = fs
	return l
}

// Set sets the content for the layout.
func (l *Layout) Set(p *Partial) *Layout {
	l.content = p
	return l
}

// Wrap sets the wrapper for the layout.
func (l *Layout) Wrap(p *Partial) *Layout {
	l.wrapper = p
	return l
}

// SetData sets the data for the layout.
func (l *Layout) SetData(data map[string]any) *Layout {
	l.data = data
	return l
}

// AddData adds data to the layout.
func (l *Layout) AddData(key string, value any) *Layout {
	l.data[key] = value
	return l
}

func (l *Layout) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	// Apply configurations to content and wrapper
	l.applyConfigToPartial(l.content)
	if l.wrapper != nil {
		l.applyConfigToPartial(l.wrapper)
		// Set the wrapper as the parent of content
		l.wrapper.With(l.content)
		// Render the wrapper
		return l.wrapper.RenderWithRequest(ctx, r)
	} else {
		// Render the content directly
		return l.content.RenderWithRequest(ctx, r)
	}
}

func (l *Layout) applyConfigToPartial(p *Partial) {
	p.fs = l.filesystem
	p.functions = l.service.config.FuncMap
	p.useCache = l.service.config.UseCache
	p.globalData = l.service.data
	p.layoutData = l.data
	p.partialHeader = l.service.config.PartialHeader
}
