package partial

import (
	"context"
	"html/template"
	"io/fs"
	"log/slog"
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
		Connector connector.Connector
		UseCache  bool
		FuncMap   template.FuncMap
		Logger    Logger
		fs        fs.FS
	}

	Service struct {
		config            *Config
		data              map[string]any
		combinedFunctions template.FuncMap
		connector         connector.Connector
		funcMapLock       sync.RWMutex // Add a read-write mutex
	}

	Layout struct {
		service           *Service
		filesystem        fs.FS
		content           *Partial
		wrapper           *Partial
		data              map[string]any
		request           *http.Request
		combinedFunctions template.FuncMap
		connector         connector.Connector
		funcMapLock       sync.RWMutex // Add a read-write mutex
	}
)

// NewService returns a new partial service.
func NewService(cfg *Config) *Service {
	if cfg.FuncMap == nil {
		cfg.FuncMap = DefaultTemplateFuncMap
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.Default().WithGroup("partial")
	}

	return &Service{
		config:            cfg,
		data:              make(map[string]any),
		funcMapLock:       sync.RWMutex{},
		combinedFunctions: cfg.FuncMap,
		connector:         cfg.Connector,
	}
}

// NewLayout returns a new layout.
func (svc *Service) NewLayout() *Layout {
	return &Layout{
		service:           svc,
		data:              make(map[string]any),
		filesystem:        svc.config.fs,
		connector:         svc.connector,
		combinedFunctions: svc.getFuncMap(),
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

// MergeFuncMap merges the given FuncMap with the existing FuncMap.
func (svc *Service) MergeFuncMap(funcMap template.FuncMap) {
	svc.funcMapLock.Lock()
	defer svc.funcMapLock.Unlock()

	for k, v := range funcMap {
		if _, ok := protectedFunctionNames[k]; ok {
			svc.config.Logger.Warn("function name is protected and cannot be overwritten", "function", k)
			continue
		}
		// Modify the existing map directly
		svc.combinedFunctions[k] = v
	}
}

func (svc *Service) getFuncMap() template.FuncMap {
	svc.funcMapLock.RLock()
	defer svc.funcMapLock.RUnlock()
	return svc.combinedFunctions
}

// FS sets the filesystem for the Layout.
func (l *Layout) FS(fs fs.FS) *Layout {
	l.filesystem = fs
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

// MergeFuncMap merges the given FuncMap with the existing FuncMap in the Layout.
func (l *Layout) MergeFuncMap(funcMap template.FuncMap) {
	l.funcMapLock.Lock()
	defer l.funcMapLock.Unlock()

	for k, v := range funcMap {
		if _, ok := protectedFunctionNames[k]; ok {
			l.service.config.Logger.Warn("function name is protected and cannot be overwritten", "function", k)
			continue
		}
		// Modify the existing map directly
		l.combinedFunctions[k] = v
	}
}

func (l *Layout) getFuncMap() template.FuncMap {
	l.funcMapLock.RLock()
	defer l.funcMapLock.RUnlock()

	return l.combinedFunctions
}

// RenderWithRequest renders the partial with the given http.Request.
func (l *Layout) RenderWithRequest(ctx context.Context, r *http.Request) (template.HTML, error) {
	l.request = r

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
	combinedFunctions := l.getFuncMap()

	p.mergeFuncMapInternal(combinedFunctions)

	p.connector = l.service.connector
	p.fs = l.filesystem
	p.logger = l.service.config.Logger
	p.useCache = l.service.config.UseCache
	p.globalData = l.service.data
	p.layoutData = l.data
	p.request = l.request
}
