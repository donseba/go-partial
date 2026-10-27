package partial

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"path"
	"strings"
	texttemplate "text/template"

	"github.com/donseba/go-partial/internal/templateutil"
)

// RenderKindText is the render kind of RenderText.
const RenderKindText RenderKind = "text"

// RenderText renders the partial as plain text with text/template, for text
// that is not HTML, such as the subject and the text part of an e-mail:
// nothing is escaped. Everything else is as for Render: the partial's file
// system, includes, entry, dot, @model values, functions (those that take a
// context.Context receive ctx) and template cache, and the render stages of
// the partial and its ancestors, which give templates their functions and
// see RenderKindText. A file can hold the text and the HTML of a message
// side by side:
//
//	mail := partial.NewID("mail", "mail/welcome.gohtml").SetParent(root).SetDot(data)
//	subject, err := partial.RenderText(ctx, mail.SetEntry("subject"))
//	text, err := partial.RenderText(ctx, mail.SetEntry("text"))
//	html, err := partial.RenderSelf(ctx, nil, mail.SetEntry("html"))
//
// Text templates call other templates with {{ template }}, from the files
// of the partial or of its includes (see SetIncludes); go-partial's
// {{ partial }} and {{ content }} render HTML.
func RenderText(ctx context.Context, p *Partial) (string, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	ctx, failures := failingRender(ctx, nil, p)

	state := newRenderContext(ctx, p, nil, RenderKindText)
	result := renderWithChainResult(state, p.getRenderStages(), renderTextTerminal)

	out, err := renderOutput(result, failures)

	return string(out), err
}

// renderTextTerminal ends the render chain of RenderText: it executes the
// partial's templates as text.
func renderTextTerminal(state *RenderContext) (template.HTML, error) {
	if state == nil || state.Partial == nil {
		return "", errors.New("text render requires a partial")
	}

	p := state.Partial
	if state.Runtime == nil || state.Runtime.partial != p {
		state.Runtime = newRuntime(p, state)
	}

	state.shareTreeValues(p)

	p.mu.RLock()
	templates := p.templates
	entry := p.entry
	p.mu.RUnlock()

	if len(templates) == 0 {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateMissing,
			Level:   EventError,
			Message: "no templates provided for rendering",
		})
		return "", errors.New("no templates provided for rendering")
	}

	tmpl, err := p.textTemplate(state)
	if err != nil {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateParseError,
			Level:   EventError,
			Message: "error getting or parsing text template",
			Error:   err,
		})
		return "", err
	}

	root, err := state.executionDot(p)
	if err != nil {
		return "", err
	}

	if bindings := contextBindingsFrom(state.Context); bindings != nil {
		bindings.push(state.Runtime)
		defer bindings.pop()
	}

	var buf strings.Builder
	if entry != "" {
		err = tmpl.ExecuteTemplate(&buf, entry, root)
	} else {
		err = tmpl.Execute(&buf, root)
	}

	if err != nil {
		state.EmitForPartial(p, Event{
			Kind:    EventTemplateExecuteError,
			Level:   EventError,
			Message: "error executing text template",
			Error:   err,
			Fields:  map[string]any{"template": templates[0], "entry": entry},
		})
		return "", fmt.Errorf("error executing text template '%s': %w", templates[0], err)
	}

	// The text travels through the chain as HTML, unescaped.
	return template.HTML(buf.String()), nil
}

// textTemplate returns the partial's templates parsed as text, with the
// functions of the render: with the template cache, a copy of a parse kept
// in the tree's store.
func (p *Partial) textTemplate(state *RenderContext) (*texttemplate.Template, error) {
	cached := p.usesCache()

	renderTemplates, err := p.renderTemplates(cached)
	if err != nil {
		return nil, err
	}

	scanner := p.scannerFor(cached)

	contracts, err := scanner.RootContracts(renderTemplates)
	if err != nil {
		return nil, fmt.Errorf("error scanning template contracts: %w", err)
	}

	if err := validateRootContracts(contracts); err != nil {
		return nil, err
	}

	funcs := p.renderFuncs(state)

	bindings := p.getContracts()
	for name, contract := range contracts {
		value, err := resolveContractValue(name, contract, bindings)
		if err != nil {
			return nil, err
		}

		funcs[name] = rootValueFunc(state, value)
	}

	_, shared := fileSystemKey(p.getFS())
	if !cached || !shared {
		return parseText(p, renderTemplates, funcs)
	}

	store := p.getTemplateStore()
	key := "text:" + p.generateCacheKey(renderTemplates, p.getFuncSet())

	base, ok := store.Text(key)
	if !ok {
		mu := store.Mutex(key)
		mu.Lock()

		base, ok = store.Text(key)
		if !ok {
			base, err = parseText(p, renderTemplates, funcs)
			if err == nil {
				store.StoreText(key, base)
			}
		}

		mu.Unlock()

		if err != nil {
			return nil, err
		}
	}

	tmpl, err := base.Clone()
	if err != nil {
		return nil, fmt.Errorf("error cloning cached text template: %w", err)
	}

	return tmpl.Funcs(texttemplate.FuncMap(funcs)), nil
}

// parseText parses templates as text with funcs, with the path aliases
// html renders have.
func parseText(p *Partial, templates []string, funcs template.FuncMap) (*texttemplate.Template, error) {
	tmpl, err := texttemplate.New(path.Base(templates[0])).Funcs(texttemplate.FuncMap(funcs)).ParseFS(p.getFS(), templates...)
	if err != nil {
		return nil, fmt.Errorf("error parsing templates: %w", err)
	}

	for _, name := range templates {
		base := templateutil.PathBase(name)
		if name == base || tmpl.Lookup(base) == nil {
			continue
		}

		for _, alias := range templateutil.PathAliases(name) {
			if tmpl.Lookup(alias) != nil {
				continue
			}

			if _, err := tmpl.New(alias).Parse(fmt.Sprintf(`{{ template %q . }}`, base)); err != nil {
				return nil, fmt.Errorf("error adding template path aliases: %w", err)
			}
		}
	}

	return tmpl, nil
}
