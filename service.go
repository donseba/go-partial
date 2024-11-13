package partial

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
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
	var renderTarget = r.Header.Get(l.service.config.PartialHeader)

	// safeguard against directly calling a parent which is also the wrapper
	if l.wrapper != nil {
		if l.wrapper.id == l.content.id {
			return "", fmt.Errorf("partial %s is a wrapper for itself, cannot render directly", l.content.id)
		}

		for k, v := range l.content.children {
			if l.wrapper.id == v.id {
				return "", fmt.Errorf("partial %s is a wrapper for %s, cannot render directly", v.id, k)
			}
		}
	}

	// set basics for rendering
	l.applyConfigToPartial(l.content)
	if l.wrapper != nil {
		l.applyConfigToPartial(l.wrapper)
	}

	if renderTarget == "" && l.wrapper != nil {
		return l.renderWrapped(ctx, r)
	}

	if renderTarget != "" {
		if l.content.id == renderTarget {
			out, err := l.content.render(ctx, r)

			// we want to update the content with the wrapper's oob children
			if l.wrapper != nil {
				out += l.renderOOBChildren(ctx, r.URL, l.wrapper, true)
			}

			return out, err
		}

		return l.renderPartial(ctx, l.content, r.URL, renderTarget)
	}

	return l.content.render(ctx, r)
}

// renderWrapped renders the partial with the wrapper.
func (l *Layout) renderWrapped(ctx context.Context, r *http.Request) (template.HTML, error) {
	l.wrapper.With(l.content)

	return l.wrapper.render(ctx, r)
}

// renderPartial renders the partial with the target.
func (l *Layout) renderPartial(ctx context.Context, p *Partial, currentURL *url.URL, target string) (template.HTML, error) {
	c := l.recursiveChildLookup(p, target, make(map[string]bool))
	if c == nil {
		return "", fmt.Errorf("requested partial %s not found", target)
	}

	out, err := c.renderNamed(ctx, currentURL, path.Base(c.templates[0]), c.templates)
	if err != nil {
		return "", err
	}

	// find all the oob children and add them to the output
	if c.parent != nil {
		out += l.renderOOBChildren(ctx, currentURL, c.parent, true)
	}

	return out, nil
}

// renderOOBChildren renders the children of the partial add sets the isOOB flag if attachOOB is true.
func (l *Layout) renderOOBChildren(ctx context.Context, currentURL *url.URL, p *Partial, attachOOB bool) (out template.HTML) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for id := range p.oobChildren {
		if child, cok := p.children[id]; cok {
			child.isOOB = attachOOB
			if childData, childErr := child.renderNamed(ctx, currentURL, path.Base(child.templates[0]), child.templates); childErr == nil {
				out += childData
			} else {
				out += template.HTML(childErr.Error())
			}
		}
	}

	return out
}

// recursiveChildLookup looks up a child recursively.
func (l *Layout) recursiveChildLookup(p *Partial, id string, visited map[string]bool) *Partial {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if visited[p.id] {
		return nil
	}
	visited[p.id] = true

	if c, ok := p.children[id]; ok {
		return c
	}

	for _, child := range p.children {
		if c := l.recursiveChildLookup(child, id, visited); c != nil {
			return c
		}
	}

	return nil
}

func (l *Layout) applyConfigToPartial(p *Partial) {
	p.fs = l.filesystem
	p.functions = l.service.config.FuncMap
	p.useCache = l.service.config.UseCache
	p.globalData = l.service.data
	p.layoutData = l.data
}
