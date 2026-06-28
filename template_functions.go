package partial

import (
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"strings"
)

var DefaultTemplateFuncMap = template.FuncMap{}

// AddGlobalFunc adds a function to the package-level default template function map.
// Services created after this call receive the function when they copy the defaults.
func AddGlobalFunc(name string, f any) error {
	if _, ok := coreFunctionNames[name]; ok {
		return fmt.Errorf("function name [%s] is protected and cannot be overwritten", name)
	}

	DefaultTemplateFuncMap[name] = f
	return nil
}

func copyFuncMap() template.FuncMap {
	return maps.Clone(DefaultTemplateFuncMap)
}

func selectionFunc(p *Partial, state *RenderContext) func() template.HTML {
	return func() template.HTML {
		var selectedPartial *Partial

		partials := p.getSelectionPartials()
		if partials == nil {
			p.getLogger().Error("no selection partials found", "id", p.id)
			return template.HTML(fmt.Sprintf("no selection partials found in parent '%s'", p.id))
		}

		selectionValue := p.getConnector().GetSelectValue(p.GetRequest())
		if selectionValue != "" {
			selectedPartial = partials[selectionValue]
		} else {
			selectedPartial = partials[p.selection.Default]
		}

		if selectedPartial == nil {
			p.getLogger().Error("selected partial not found", "id", selectionValue, "parent", p.id)
			return template.HTML(fmt.Sprintf("selected partial '%s' not found in parent '%s'", selectionValue, p.id))
		}

		selectedPartial.fs = p.fs

		selectedClone := selectedPartial.clone()
		selectedClone.parent = p

		html, err := selectedClone.renderSelf(state.Context, p.GetRequest())
		if err != nil {
			p.getLogger().Error("error rendering selected partial", "id", selectionValue, "parent", p.id, "error", err)
			fallback, fallbackErr := selectedClone.renderErrorFragment(state.Context, p.GetRequest(), err)
			if fallbackErr != nil {
				p.getLogger().Error("error rendering selected partial fallback", "id", selectionValue, "parent", p.id, "error", fallbackErr)
				return template.HTML(fmt.Sprintf("error rendering selected partial '%s': %v", selectionValue, fallbackErr))
			}
			return fallback
		}

		return html
	}
}

func partialFunc(p *Partial, state *RenderContext) func(id string, args ...any) template.HTML {
	return func(id string, args ...any) template.HTML {
		if templatePath, ok := partialTemplatePath(p, id); ok {
			child := p.clone()
			child.id = templatePath
			child.parent = p
			child.templates = []string{templatePath}

			if ok := applyPartialTemplateArgs(child, id, args...); !ok {
				return template.HTML(fmt.Sprintf("invalid data for partial '%s'", id))
			}

			html, err := child.renderSelf(state.Context, p.GetRequest())
			if err != nil {
				child.getLogger().Error("error rendering template partial", "path", templatePath, "error", err)
				fallback, fallbackErr := child.renderErrorFragment(state.Context, p.GetRequest(), err)
				if fallbackErr != nil {
					return template.HTML(fmt.Sprintf("error rendering partial '%s': %v", id, fallbackErr))
				}
				return fallback
			}

			return html
		}

		p.getLogger().Warn("partial template path not found", "path", id)
		return template.HTML(template.HTMLEscapeString(fmt.Sprintf("partial template '%s' not found", id)))
	}
}

func partialTemplatePath(p *Partial, name string) (string, bool) {
	templatePath := strings.TrimSpace(strings.ReplaceAll(name, `\`, `/`))
	templatePath = strings.TrimLeft(templatePath, "/")
	if templatePath == "" {
		return "", false
	}

	info, err := fs.Stat(p.getFS(), templatePath)
	if err != nil || info.IsDir() {
		return "", false
	}

	return templatePath, true
}

func applyPartialTemplateArgs(p *Partial, id string, args ...any) bool {
	switch len(args) {
	case 0:
		return true
	case 1:
		p.SetDot(args[0])
		return true
	}

	dot, ok := partialDotMapArg(p, id, args...)
	if !ok {
		return false
	}
	p.SetDot(dot)
	return true
}

func contentFunc(p *Partial, state *RenderContext) func() template.HTML {
	return func() template.HTML {
		if p.layoutContentID == "" {
			p.getLogger().Warn("content helper used outside layout wrapper", "id", p.id)
			return template.HTML("content is only available on layout wrappers")
		}

		html, err := p.renderChildPartial(state.Context, p.layoutContentID)
		if err != nil {
			p.getLogger().Error("error rendering layout content", "id", p.layoutContentID, "error", err)
			return template.HTML(fmt.Sprintf("error rendering content: %v", err))
		}

		return html
	}
}

func partialDotMapArg(p *Partial, id string, args ...any) (map[string]any, bool) {
	if len(args)%2 != 0 {
		p.getLogger().Warn("invalid dot data for partial, pass key/value pairs", "id", id)
		return nil, false
	}

	dot := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			p.getLogger().Warn("invalid dot data key for partial", "id", id, "type", fmt.Sprintf("%T", args[i]))
			return nil, false
		}
		dot[key] = args[i+1]
	}
	return dot, true
}

func debugFunc(p *Partial, state *RenderContext) func(value any) template.HTML {
	return func(value any) template.HTML {
		renderer := p.getDebugRenderer()
		if renderer == nil {
			return template.HTML(template.HTMLEscapeString(fmt.Sprintf("%#v", value)))
		}

		out, err := renderer(state.Context, p, newRuntime(p, state), value)
		if err != nil {
			p.getLogger().Error("error rendering debug helper", "id", p.id, "error", err)
			return template.HTML(`<pre class="go-partial-debug">` + template.HTMLEscapeString(fmt.Sprintf("%#v", value)) + `</pre>`)
		}
		return out
	}
}

func actionFunc(p *Partial, state *RenderContext) func() template.HTML {
	return func() template.HTML {
		if p.templateAction == nil {
			p.getLogger().Error("no action callback found", "id", p.id)
			return template.HTML(fmt.Sprintf("no action callback found in partial '%s'", p.id))
		}

		// Use the selector to get the appropriate partial
		actionPartial, err := p.templateAction(state.Context, p, newRuntime(p, state))
		if err != nil {
			p.getLogger().Error("error in selector function", "error", err)
			return template.HTML(fmt.Sprintf("error in action function: %v", err))
		}

		// Render the selected partial instead
		html, err := actionPartial.renderSelf(state.Context, p.GetRequest())
		if err != nil {
			p.getLogger().Error("error rendering action partial", "error", err)
			return template.HTML(fmt.Sprintf("error rendering action partial: %v", err))
		}
		return html
	}
}
