package partial

import (
	"context"
	"html/template"
	"io/fs"
	"maps"
	"slices"

	"github.com/donseba/go-partial/connector"
)

type testBlueprint struct {
	root *Partial
}

func newTestBlueprint(opts ...func(*Partial)) *testBlueprint {
	root := New()
	for _, opt := range opts {
		if opt != nil {
			opt(root)
		}
	}
	return &testBlueprint{root: root}
}

func testBlueprintFS(fsys fs.FS) func(*Partial) {
	return func(p *Partial) {
		p.SetFileSystem(fsys)
	}
}

func testBlueprintConnector(conn connector.Connector) func(*Partial) {
	return func(p *Partial) {
		p.SetConnector(conn)
	}
}

func testBlueprintCache(enabled bool) func(*Partial) {
	return func(p *Partial) {
		p.UseTemplateCache(enabled)
	}
}

func (bp *testBlueprint) Apply(p *Partial) *Partial {
	if bp == nil || bp.root == nil || p == nil {
		return p
	}
	configured := p.clone()
	bp.apply(configured)
	return configured
}

func (bp *testBlueprint) Use(stages ...RenderStage) *testBlueprint {
	if bp != nil && bp.root != nil {
		bp.root.Use(stages...)
	}
	return bp
}

func (bp *testBlueprint) SetFunc(funcs ...template.FuncMap) *testBlueprint {
	if bp != nil && bp.root != nil {
		bp.root.SetFunc(funcs...)
	}
	return bp
}

func (bp *testBlueprint) getStaticFuncMap() template.FuncMap {
	if bp == nil || bp.root == nil {
		return nil
	}
	return bp.root.getStaticFuncMap()
}

func (bp *testBlueprint) getCustomFuncMap() template.FuncMap {
	if bp == nil || bp.root == nil {
		return nil
	}
	return bp.root.getCustomFuncMap()
}

func (bp *testBlueprint) Compose(content *Partial, wrapper *Partial) *Partial {
	if bp == nil || bp.root == nil {
		return wrapper.SetContent(content)
	}
	if wrapper == nil {
		return bp.Apply(content)
	}
	root := wrapper.clone()
	bp.apply(root)
	return root.SetContent(content)
}

func (bp *testBlueprint) apply(p *Partial) {
	if bp == nil || bp.root == nil || p == nil {
		return
	}
	root := bp.root
	root.mu.RLock()
	rootFuncs := maps.Clone(root.staticFuncs)
	rootContracts := slices.Clone(root.contracts)
	rootStages := slices.Clone(root.stages)
	rootFS := root.fs
	rootFSSet := root.fsSet
	rootConnector := root.connector
	rootEvents := root.events
	rootUseCache := root.useCache
	rootCache := root.templateCache
	root.mu.RUnlock()

	p.mu.Lock()
	if len(rootFuncs) > 0 {
		funcs := maps.Clone(rootFuncs)
		maps.Copy(funcs, p.staticFuncs)
		p.staticFuncs = funcs
	}
	if len(rootContracts) > 0 {
		p.contracts = append(rootContracts, p.contracts...)
	}
	if len(rootStages) > 0 {
		p.stages = append(rootStages, p.stages...)
	}
	if rootFSSet {
		p.fs = rootFS
		p.fsSet = true
	}
	if rootConnector != nil {
		p.connector = rootConnector
	}
	if rootEvents != nil {
		p.events = rootEvents
	}
	p.useCache = rootUseCache
	p.templateCache = rootCache
	p.mu.Unlock()

	for _, child := range p.children {
		bp.apply(child)
	}
}

func (bp *testBlueprint) RenderContext(ctx context.Context) context.Context {
	return ctx
}

func (p *Partial) SetDotFrom(other *Partial) *Partial {
	if p == nil || other == nil {
		return p
	}
	if dot, ok := other.getDotContract(); ok {
		p.SetDot(dot)
	}
	return p
}
