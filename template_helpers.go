package partial

import (
	"fmt"
	"html/template"
	"io/fs"
	"strings"
)

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

			result := child.renderSelfResult(state.Context, p.getRequest())
			if result.Err != nil {
				child.emit(Event{
					Kind:    EventRenderError,
					Level:   EventError,
					Message: "error rendering template partial",
					Error:   result.Err,
					Fields:  map[string]any{"path": templatePath},
				})
				fallback, fallbackErr := child.renderErrorFragment(state.Context, p.getRequest(), result.Err)
				if fallbackErr != nil {
					return template.HTML(fmt.Sprintf("error rendering partial '%s': %v", id, fallbackErr))
				}
				return fallback
			}

			return result.HTML
		}

		p.emit(Event{
			Kind:    EventTemplateMissing,
			Level:   EventWarn,
			Message: "partial template path not found",
			Fields:  map[string]any{"path": id},
		})
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
			p.emit(Event{
				Kind:    EventContentMissingLayout,
				Level:   EventWarn,
				Message: "content helper used outside layout wrapper",
				Fields:  map[string]any{"id": p.id},
			})
			return template.HTML("content is only available on layout wrappers")
		}

		html, err := p.renderChildPartial(state.Context, p.layoutContentID)
		if err != nil {
			p.emit(Event{
				Kind:    EventRenderError,
				Level:   EventError,
				Message: "error rendering layout content",
				Error:   err,
				Fields:  map[string]any{"id": p.layoutContentID},
			})
			return template.HTML(fmt.Sprintf("error rendering content: %v", err))
		}

		return html
	}
}

func partialDotMapArg(p *Partial, id string, args ...any) (map[string]any, bool) {
	if len(args)%2 != 0 {
		p.emit(Event{
			Kind:    EventContractInvalid,
			Level:   EventWarn,
			Message: "invalid dot data for partial, pass key/value pairs",
			Fields:  map[string]any{"id": id},
		})
		return nil, false
	}

	dot := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			p.emit(Event{
				Kind:    EventContractInvalid,
				Level:   EventWarn,
				Message: "invalid dot data key for partial",
				Fields:  map[string]any{"id": id, "type": fmt.Sprintf("%T", args[i])},
			})
			return nil, false
		}
		dot[key] = args[i+1]
	}
	return dot, true
}
