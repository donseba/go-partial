package partial

import (
	"fmt"
	"html/template"
	"io/fs"
	"strings"
)

func copyFuncMap() template.FuncMap {
	return make(template.FuncMap)
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

			html, err := child.renderSelf(state.Context, p.getRequest())
			if err != nil {
				child.getLogger().Error("error rendering template partial", "path", templatePath, "error", err)
				fallback, fallbackErr := child.renderErrorFragment(state.Context, p.getRequest(), err)
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
