package partial

import (
	"fmt"
	"html/template"
	"strings"
)

func partialFunc(p *Partial, state *RenderContext) func(id string, args ...any) template.HTML {
	return func(id string, args ...any) template.HTML {
		if templatePath, ok := partialTemplatePath(p, id); ok {
			child := p.clone()
			child.id = templatePath
			child.parent = p
			child.templates = []string{templatePath}
			// The child inherits p's values through its parent; copies would
			// match every declaration twice.
			child.mu.Lock()
			child.removeContractsLocked(func(existing contractInformation) bool {
				return existing.Kind == contractRoot
			})
			child.mu.Unlock()

			if ok := applyPartialTemplateArgs(state, child, id, args...); !ok {
				return template.HTML(fmt.Sprintf("invalid data for partial '%s'", id))
			}

			result := renderSelfResult(state.Context, state.Request, child)
			if result.Err != nil {
				state.EmitForPartial(child, Event{
					Kind:    EventRenderError,
					Level:   EventError,
					Message: "error rendering template partial",
					Error:   result.Err,
					Fields:  map[string]any{"path": templatePath},
				})
				fallback, fallbackErr := renderErrorFragment(state.Context, state.Request, child, result.Err)
				if fallbackErr != nil {
					return template.HTML(fmt.Sprintf("error rendering partial '%s': %v", id, fallbackErr))
				}
				return fallback
			}

			return result.HTML
		}

		state.EmitForPartial(p, Event{
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

	if !p.scanner().IsFile(templatePath) {
		return "", false
	}

	return templatePath, true
}

func applyPartialTemplateArgs(state *RenderContext, p *Partial, id string, args ...any) bool {
	switch len(args) {
	case 0:
		return true
	case 1:
		p.SetDot(args[0])
		return true
	}

	dot, ok := partialDotMapArg(state, p, id, args...)
	if !ok {
		return false
	}
	p.SetDot(dot)
	bindPartialArgs(p, dot)
	return true
}

// bindPartialArgs binds the key/value pairs a partial is called with to the
// typed roots its template declares by those names, so
// {{partial runtime "card.gohtml" "Event" .}} gives a card that declares
// "@model Event example.com/app.Event" its Event. The pairs stay in the dot as
// well.
func bindPartialArgs(p *Partial, args map[string]any) {
	contracts, err := p.scanner().RootContracts(p.templates)
	if err != nil {
		// Rendering the partial reports the error.
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for name, value := range args {
		contract, ok := contracts[name]
		if !ok {
			continue
		}
		p.contracts = append(p.contracts, contractInformation{
			Kind:       contractRoot,
			Annotation: contract.Annotation,
			Name:       name,
			Value:      value,
		})
	}
}

func contentFunc(p *Partial, state *RenderContext) func() template.HTML {
	return func() template.HTML {
		if p.contentID == "" {
			state.EmitForPartial(p, Event{
				Kind:    EventContentMissing,
				Level:   EventWarn,
				Message: "content helper used without a content child",
				Fields:  map[string]any{"id": p.id},
			})
			return template.HTML("content is only available when a content child is configured")
		}

		html, err := renderChildPartial(state.Context, state.Request, p, p.contentID)
		if err != nil {
			state.EmitForPartial(p, Event{
				Kind:    EventRenderError,
				Level:   EventError,
				Message: "error rendering content child",
				Error:   err,
				Fields:  map[string]any{"id": p.contentID},
			})
			return template.HTML(fmt.Sprintf("error rendering content: %v", err))
		}

		return html
	}
}

func partialDotMapArg(state *RenderContext, p *Partial, id string, args ...any) (map[string]any, bool) {
	if len(args)%2 != 0 {
		state.EmitForPartial(p, Event{
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
			state.EmitForPartial(p, Event{
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
