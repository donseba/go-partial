package partial

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"sync"

	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/internal/templateutil"
)

type (
	// Logger is the logging interface used by go-partial.
	Logger interface {
		Warn(msg string, args ...any)
		Error(msg string, args ...any)
	}

	// Config configures a Service.
	Config struct {
		Connector        connector.Connector
		UseTemplateCache bool
		Logger           Logger
		FS               fs.FS
		Renderers        []Renderer
	}

	// Service stores shared rendering configuration for layouts.
	Service struct {
		config             *Config
		staticFuncs        template.FuncMap
		customFuncs        template.FuncMap
		hasCustomFunctions bool
		connector          connector.Connector
		templateCache      *templateutil.Store
		renderers          []Renderer
		funcsLock          sync.RWMutex
	}

	// Layout binds a content partial to an optional wrapper partial.
	Layout struct {
		service            *Service
		filesystem         fs.FS
		content            *Partial
		wrapper            *Partial
		request            *http.Request
		staticFuncs        template.FuncMap
		customFuncs        template.FuncMap
		hasCustomFunctions bool
		connector          connector.Connector
		renderers          []Renderer
		funcsLock          sync.RWMutex
	}
)

// NewService returns a new partial service.
func NewService(cfg *Config) *Service {
	if cfg == nil {
		cfg = &Config{}
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default().WithGroup("partial")
	}

	if cfg.Connector == nil {
		cfg.Connector = connector.NewPartial(nil)
	}

	functions := make(template.FuncMap)
	return &Service{
		config:        cfg,
		funcsLock:     sync.RWMutex{},
		staticFuncs:   functions,
		customFuncs:   make(template.FuncMap),
		connector:     cfg.Connector,
		renderers:     append([]Renderer(nil), cfg.Renderers...),
		templateCache: templateutil.NewStore(),
	}
}

// NewLayout returns a new layout.
func (svc *Service) NewLayout() *Layout {
	fsys := svc.config.FS
	functions := svc.getStaticFuncMap()
	customFuncs := svc.getCustomFuncMap()
	return &Layout{
		service:            svc,
		filesystem:         fsys,
		connector:          svc.connector,
		renderers:          append([]Renderer(nil), svc.renderers...),
		staticFuncs:        functions,
		customFuncs:        customFuncs,
		hasCustomFunctions: svc.getHasCustomFunctions(),
	}
}

// Use appends renderers to every layout created by this service.
func (svc *Service) Use(renderers ...Renderer) *Service {
	if svc == nil {
		return nil
	}
	svc.renderers = append(svc.renderers, renderers...)
	svc.config.Renderers = append(svc.config.Renderers, renderers...)
	return svc
}

// SetFunc registers template functions on the Service.
func (svc *Service) SetFunc(funcMaps ...template.FuncMap) *Service {
	if svc == nil {
		return nil
	}
	svc.funcsLock.Lock()
	defer svc.funcsLock.Unlock()

	if mergeFuncMapsInto(svc.staticFuncs, svc.customFuncs, svc.config.Logger, funcMaps...) {
		svc.hasCustomFunctions = true
	}
	return svc
}

func (svc *Service) getStaticFuncMap() template.FuncMap {
	svc.funcsLock.RLock()
	defer svc.funcsLock.RUnlock()
	return maps.Clone(svc.staticFuncs)
}

func (svc *Service) getCustomFuncMap() template.FuncMap {
	svc.funcsLock.RLock()
	defer svc.funcsLock.RUnlock()
	return maps.Clone(svc.customFuncs)
}

func (svc *Service) getHasCustomFunctions() bool {
	svc.funcsLock.RLock()
	defer svc.funcsLock.RUnlock()
	return svc.hasCustomFunctions
}

func mergeStaticFuncMap(dst template.FuncMap, src template.FuncMap, logger Logger) template.FuncMap {
	merged := make(template.FuncMap, len(src))
	for k, v := range src {
		if isProtectedFunctionName(k) {
			if logger != nil {
				logger.Warn("function name is protected and cannot be overwritten", "function", k)
			}
			continue
		}
		dst[k] = v
		merged[k] = v
	}
	return merged
}

// Use appends renderers to the layout render chain.
func (l *Layout) Use(renderers ...Renderer) *Layout {
	if l == nil {
		return nil
	}
	l.renderers = append(l.renderers, renderers...)
	if l.content != nil {
		l.content.Use(renderers...)
	}
	if l.wrapper != nil {
		l.wrapper.Use(renderers...)
	}
	return l
}

// Set sets the content for the layout.
func (l *Layout) Set(p *Partial) *Layout {
	l.content = p
	l.applyConfigToPartial(l.content)
	l.attachContentToWrapper()
	return l
}

// Wrap sets the wrapper for the layout.
func (l *Layout) Wrap(p *Partial) *Layout {
	l.wrapper = p
	l.applyConfigToPartial(l.wrapper)
	l.attachContentToWrapper()
	return l
}

func (l *Layout) attachContentToWrapper() {
	if l.wrapper == nil || l.content == nil {
		return
	}
	l.wrapper.With(l.content)
	l.wrapper.layoutContentID = l.content.id
}

// SetFunc registers template functions in the Layout tree.
func (l *Layout) SetFunc(funcMaps ...template.FuncMap) *Layout {
	if l == nil {
		return nil
	}
	l.funcsLock.Lock()
	defer l.funcsLock.Unlock()

	if mergeFuncMapsInto(l.staticFuncs, l.customFuncs, l.service.config.Logger, funcMaps...) {
		l.hasCustomFunctions = true
	}
	return l
}

func mergeFuncMapsInto(staticFuncs, customFuncs template.FuncMap, logger Logger, funcMaps ...template.FuncMap) bool {
	changed := false
	for _, funcMap := range funcMaps {
		merged := mergeStaticFuncMap(staticFuncs, funcMap, logger)
		if len(merged) == 0 {
			continue
		}
		maps.Copy(customFuncs, merged)
		changed = true
	}
	return changed
}

func (l *Layout) getStaticFuncMap() template.FuncMap {
	l.funcsLock.RLock()
	defer l.funcsLock.RUnlock()

	return maps.Clone(l.staticFuncs)
}

func (l *Layout) getCustomFuncMap() template.FuncMap {
	l.funcsLock.RLock()
	defer l.funcsLock.RUnlock()

	return maps.Clone(l.customFuncs)
}

// RenderWithRequest renders the partial with the given http.Request.
func (l *Layout) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	l.request = r
	if l.connector == nil {
		l.connector = connector.NewPartial(nil)
	}

	if l.wrapper != nil {
		return l.wrapper.RenderWithRequest(ctx, r)
	} else {
		return l.content.RenderWithRequest(ctx, r)
	}
}

// WriteWithRequest writes the layout to the response writer.
func (l *Layout) WriteWithRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	l.request = r
	if l.connector == nil {
		l.connector = connector.NewPartial(nil)
	}

	if l.connector.RenderPartial(r) {
		if l.wrapper != nil {
			l.content.parent = l.wrapper
		}
		err := l.content.WriteWithRequest(ctx, w, r)
		if err != nil {
			if l.service.config.Logger != nil {
				l.service.config.Logger.Error("error rendering layout", "error", err)
			}
			return err
		}
		return nil
	}

	if l.wrapper != nil {
		err := l.wrapper.WriteWithRequest(ctx, w, r)
		if err != nil {
			if l.service.config.Logger != nil {
				l.service.config.Logger.Error("error rendering layout", "error", err)
			}
			return err
		}
	}

	return nil
}

func (l *Layout) applyConfigToPartial(p *Partial) {
	if p == nil {
		return
	}

	staticFuncs := l.getStaticFuncMap()
	customFuncs := l.getCustomFuncMap()

	p.mergeFuncMapInternal(staticFuncs, customFuncs)

	p.connector = l.service.connector
	if l.filesystem != nil {
		p.fs = l.filesystem
	}
	if l.service.config.Logger != nil {
		p.logger = l.service.config.Logger
	}
	p.useCache = l.service.config.UseTemplateCache
	if len(l.renderers) > 0 && !p.renderersInherited {
		p.renderers = append(append([]Renderer(nil), l.renderers...), p.renderers...)
		p.renderersInherited = true
	}
	p.templateCache = l.service.templateCache
	p.request = l.request

	for _, child := range p.children {
		l.applyConfigToPartial(child)
	}
}
