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
)

type (
	Logger interface {
		Warn(msg string, args ...any)
		Error(msg string, args ...any)
	}

	Config struct {
		Connector           connector.Connector
		UseTemplateCache    bool
		Logger              Logger
		FS                  fs.FS
		ErrorRenderer       ErrorRenderer
		DebugRenderer       DebugRenderer
		InteractionRenderer InteractionRenderer
		ErrorMode           ErrorMode
		fs                  fs.FS
	}

	Service struct {
		config             *Config
		data               map[string]any
		staticFuncs        template.FuncMap
		customFuncs        template.FuncMap
		hasCustomFunctions bool
		connector          connector.Connector
		templateCache      *templateStore
		funcsLock          sync.RWMutex // Add a read-write mutex
	}

	Layout struct {
		service            *Service
		filesystem         fs.FS
		content            *Partial
		wrapper            *Partial
		data               map[string]any
		request            *http.Request
		staticFuncs        template.FuncMap
		customFuncs        template.FuncMap
		hasCustomFunctions bool
		connector          connector.Connector
		funcsLock          sync.RWMutex // Add a read-write mutex
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

	functions := copyFuncMap()
	return &Service{
		config:        cfg,
		data:          make(map[string]any),
		funcsLock:     sync.RWMutex{},
		staticFuncs:   functions,
		customFuncs:   make(template.FuncMap),
		connector:     cfg.Connector,
		templateCache: newTemplateStore(),
	}
}

// NewLayout returns a new layout.
func (svc *Service) NewLayout() *Layout {
	fsys := svc.config.FS
	if fsys == nil {
		fsys = svc.config.fs
	}
	functions := svc.getStaticFuncMap()
	customFuncs := svc.getCustomFuncMap()
	return &Layout{
		service:            svc,
		data:               make(map[string]any),
		filesystem:         fsys,
		connector:          svc.connector,
		staticFuncs:        functions,
		customFuncs:        customFuncs,
		hasCustomFunctions: svc.getHasCustomFunctions(),
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

func (svc *Service) SetConnector(conn connector.Connector) *Service {
	svc.connector = conn
	return svc
}

func (svc *Service) SetErrorRenderer(renderer ErrorRenderer) *Service {
	svc.config.ErrorRenderer = renderer
	return svc
}

func (svc *Service) SetDebugRenderer(renderer DebugRenderer) *Service {
	svc.config.DebugRenderer = renderer
	return svc
}

func (svc *Service) SetInteractionRenderer(renderer InteractionRenderer) *Service {
	svc.config.InteractionRenderer = renderer
	return svc
}

func (svc *Service) SetErrorMode(mode ErrorMode) *Service {
	svc.config.ErrorMode = mode
	return svc
}

// UseFuncs adds template functions to the Service.
func (svc *Service) UseFuncs(funcMap template.FuncMap) {
	svc.funcsLock.Lock()
	defer svc.funcsLock.Unlock()

	customFuncs := mergeStaticFuncMap(svc.staticFuncs, funcMap, svc.config.Logger)
	if len(customFuncs) > 0 {
		maps.Copy(svc.customFuncs, customFuncs)
		svc.hasCustomFunctions = true
	}
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

// FS sets the filesystem for the Layout.
func (l *Layout) FS(fs fs.FS) *Layout {
	l.filesystem = fs
	return l
}

func (l *Layout) Connector() connector.Connector {
	return l.connector
}

func (l *Layout) SetErrorRenderer(renderer ErrorRenderer) *Layout {
	l.service.config.ErrorRenderer = renderer
	if l.content != nil {
		l.content.SetErrorRenderer(renderer)
	}
	if l.wrapper != nil {
		l.wrapper.SetErrorRenderer(renderer)
	}
	return l
}

func (l *Layout) SetDebugRenderer(renderer DebugRenderer) *Layout {
	l.service.config.DebugRenderer = renderer
	if l.content != nil {
		l.content.SetDebugRenderer(renderer)
	}
	if l.wrapper != nil {
		l.wrapper.SetDebugRenderer(renderer)
	}
	return l
}

func (l *Layout) SetInteractionRenderer(renderer InteractionRenderer) *Layout {
	l.service.config.InteractionRenderer = renderer
	if l.content != nil {
		l.content.SetInteractionRenderer(renderer)
	}
	if l.wrapper != nil {
		l.wrapper.SetInteractionRenderer(renderer)
	}
	return l
}

func (l *Layout) SetErrorMode(mode ErrorMode) *Layout {
	l.service.config.ErrorMode = mode
	if l.content != nil {
		l.content.SetErrorMode(mode)
	}
	if l.wrapper != nil {
		l.wrapper.SetErrorMode(mode)
	}
	return l
}

// Set sets the content for the layout.
func (l *Layout) Set(p *Partial) *Layout {
	l.content = p
	l.applyConfigToPartial(l.content)
	return l
}

// Wrap sets the wrapper for the layout.
func (l *Layout) Wrap(p *Partial) *Layout {
	l.wrapper = p
	l.applyConfigToPartial(l.wrapper)
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

// UseFuncs adds template functions to the Layout.
func (l *Layout) UseFuncs(funcMap template.FuncMap) {
	l.funcsLock.Lock()
	defer l.funcsLock.Unlock()

	customFuncs := mergeStaticFuncMap(l.staticFuncs, funcMap, l.service.config.Logger)
	if len(customFuncs) > 0 {
		maps.Copy(l.customFuncs, customFuncs)
		l.hasCustomFunctions = true
	}
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
		l.wrapper.With(l.content)
		// Render the wrapper
		return l.wrapper.RenderWithRequest(ctx, r)
	} else {
		// Render the content directly
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
		l.wrapper.With(l.content)

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

	// Combine functions only once
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
	if l.service.config.ErrorRenderer != nil {
		p.errorRenderer = l.service.config.ErrorRenderer
	}
	if l.service.config.DebugRenderer != nil {
		p.debugRenderer = l.service.config.DebugRenderer
	}
	if l.service.config.InteractionRenderer != nil {
		p.interactionRenderer = l.service.config.InteractionRenderer
	}
	p.errorMode = l.service.config.ErrorMode
	p.errorModeSet = true
	p.templateCache = l.service.templateCache
	p.serviceData = l.service.data
	p.layoutData = l.data
	p.request = l.request

	for _, child := range p.children {
		l.applyConfigToPartial(child)
	}
}
