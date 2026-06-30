package partial

import (
	"context"
	"html/template"
	"io/fs"
	"maps"
	"net/http"
	"sync"

	"github.com/donseba/go-partial/connector"
	"github.com/donseba/go-partial/internal/templateutil"
)

type (
	// Config configures a Service.
	Config struct {
		Connector        connector.Connector
		UseTemplateCache bool
		Events           EventSink
		FS               fs.FS
		// Stages configures the render stage chain shared by new layouts.
		Stages []RenderStage
		// Renderers configures the render stage chain.
		//
		// Deprecated: use Stages.
		Renderers []RenderStage
	}

	// Service stores shared rendering configuration for layouts.
	Service struct {
		config        *Config
		staticFuncs   template.FuncMap
		customFuncs   template.FuncMap
		connector     connector.Connector
		events        EventSink
		templateCache *templateutil.Store
		stages        []RenderStage
		funcsLock     sync.RWMutex
	}

	// Layout binds a content partial to an optional wrapper partial.
	Layout struct {
		service     *Service
		filesystem  fs.FS
		content     *Partial
		wrapper     *Partial
		staticFuncs template.FuncMap
		customFuncs template.FuncMap
		connector   connector.Connector
		events      EventSink
		stages      []RenderStage
		funcsLock   sync.RWMutex
	}
)

// NewService returns a new partial service.
func NewService(cfg *Config) *Service {
	if cfg == nil {
		cfg = &Config{}
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
		events:        cfg.Events,
		stages:        appendRenderStages(cfg.Stages, cfg.Renderers),
		templateCache: templateutil.NewStore(),
	}
}

func appendRenderStages(stages []RenderStage, deprecated []RenderStage) []RenderStage {
	out := append([]RenderStage(nil), stages...)
	return append(out, deprecated...)
}

// NewLayout returns a new layout.
func (svc *Service) NewLayout() *Layout {
	fsys := svc.config.FS
	functions := svc.getStaticFuncMap()
	customFuncs := svc.getCustomFuncMap()
	return &Layout{
		service:     svc,
		filesystem:  fsys,
		connector:   svc.connector,
		events:      svc.events,
		stages:      append([]RenderStage(nil), svc.stages...),
		staticFuncs: functions,
		customFuncs: customFuncs,
	}
}

// Use appends render stages to every layout created by this service.
func (svc *Service) Use(stages ...RenderStage) *Service {
	if svc == nil {
		return nil
	}
	svc.stages = append(svc.stages, stages...)
	svc.config.Stages = append(svc.config.Stages, stages...)
	return svc
}

// SetFunc registers template functions on the Service.
func (svc *Service) SetFunc(funcMaps ...template.FuncMap) *Service {
	if svc == nil {
		return nil
	}
	svc.funcsLock.Lock()
	defer svc.funcsLock.Unlock()

	mergeFuncMapsInto(svc.staticFuncs, svc.customFuncs, svc.events, funcMaps...)
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

func mergeStaticFuncMap(dst template.FuncMap, src template.FuncMap, events EventSink) template.FuncMap {
	merged := make(template.FuncMap, len(src))
	for k, v := range src {
		if isProtectedFunctionName(k) {
			emitSafely(events, nil, Event{
				Time:    timeNow(),
				Kind:    EventFuncProtected,
				Level:   EventWarn,
				Message: "function name is protected and cannot be overwritten",
				Fields:  map[string]any{"function": k},
			})
			continue
		}
		dst[k] = v
		merged[k] = v
	}
	return merged
}

// Use appends render stages to the layout render chain.
func (l *Layout) Use(stages ...RenderStage) *Layout {
	if l == nil {
		return nil
	}
	l.stages = append(l.stages, stages...)
	if l.content != nil {
		l.content.Use(stages...)
	}
	if l.wrapper != nil {
		l.wrapper.Use(stages...)
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

	mergeFuncMapsInto(l.staticFuncs, l.customFuncs, l.events, funcMaps...)
	return l
}

func mergeFuncMapsInto(staticFuncs, customFuncs template.FuncMap, events EventSink, funcMaps ...template.FuncMap) {
	for _, funcMap := range funcMaps {
		merged := mergeStaticFuncMap(staticFuncs, funcMap, events)
		if len(merged) == 0 {
			continue
		}
		maps.Copy(customFuncs, merged)
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
	if l.wrapper != nil {
		return l.wrapper.RenderWithRequest(ctx, r)
	} else {
		return l.content.RenderWithRequest(ctx, r)
	}
}

// WriteWithRequest writes the layout to the response writer.
func (l *Layout) WriteWithRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	conn := l.connector
	if conn == nil {
		conn = connector.NewPartial(nil)
	}

	if conn.RenderPartial(r) {
		err := l.content.WriteWithRequest(ctx, w, r)
		if err != nil {
			emitSafely(l.events, nil, Event{
				Time:    timeNow(),
				Kind:    EventRenderError,
				Level:   EventError,
				Message: "error rendering layout",
				Error:   err,
			})
			return err
		}
		return nil
	}

	if l.wrapper != nil {
		err := l.wrapper.WriteWithRequest(ctx, w, r)
		if err != nil {
			emitSafely(l.events, nil, Event{
				Time:    timeNow(),
				Kind:    EventRenderError,
				Level:   EventError,
				Message: "error rendering layout",
				Error:   err,
			})
			return err
		}
		return nil
	}

	if l.content != nil {
		err := l.content.WriteWithRequest(ctx, w, r)
		if err != nil {
			emitSafely(l.events, nil, Event{
				Time:    timeNow(),
				Kind:    EventRenderError,
				Level:   EventError,
				Message: "error rendering layout",
				Error:   err,
			})
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
	p.events = l.events
	if l.filesystem != nil {
		p.fs = l.filesystem
		p.fsSet = true
	}
	p.useCache = l.service.config.UseTemplateCache
	if len(l.stages) > 0 && !p.stagesInherited {
		p.stages = append(append([]RenderStage(nil), l.stages...), p.stages...)
		p.stagesInherited = true
	}
	p.templateCache = l.service.templateCache

	for _, child := range p.children {
		l.applyConfigToPartial(child)
	}
}
