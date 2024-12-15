package partial

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var DefaultTemplateFuncMap = template.FuncMap{
	"safeHTML": safeHTML,
	// String functions
	"upper":       strings.ToUpper,
	"lower":       strings.ToLower,
	"trimSpace":   strings.TrimSpace,
	"trim":        strings.Trim,
	"trimSuffix":  strings.TrimSuffix,
	"trimPrefix":  strings.TrimPrefix,
	"contains":    strings.Contains,
	"containsAny": strings.ContainsAny,
	"hasPrefix":   strings.HasPrefix,
	"hasSuffix":   strings.HasSuffix,
	"repeat":      strings.Repeat,
	"replace":     strings.Replace,
	"split":       strings.Split,
	"join":        strings.Join,
	"stringSlice": stringSlice,
	"title":       title,
	"substr":      substr,
	"ucfirst":     ucfirst,
	"compare":     strings.Compare,
	"equalFold":   strings.EqualFold,
	"urlencode":   url.QueryEscape,
	"urldecode":   url.QueryUnescape,
	// Time functions

	"now":        time.Now,
	"formatDate": formatDate,
	"parseDate":  parseDate,

	// List functions
	"first": first,
	"last":  last,

	// Map functions
	"hasKey": hasKey,
	"keys":   keys,

	// Debug functions
	"debug": debug,
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

// ucfirst capitalizes the first character of the string.
func ucfirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func stringSlice(values ...string) []string {
	return values
}

// title capitalizes the first character of each word in the string.
func title(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	length := len(runes)
	capitalizeNext := true
	for i := 0; i < length; i++ {
		if unicode.IsSpace(runes[i]) {
			capitalizeNext = true
		} else if capitalizeNext {
			runes[i] = unicode.ToUpper(runes[i])
			capitalizeNext = false
		} else {
			runes[i] = unicode.ToLower(runes[i])
		}
	}
	return string(runes)
}

// substr returns a substring starting at 'start' position with 'length' characters.
func substr(s string, start, length int) string {
	runes := []rune(s)
	if start >= len(runes) || length <= 0 {
		return ""
	}
	end := start + length
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}

// first returns the first element of the list.
func first(a []any) any {
	if len(a) > 0 {
		return a[0]
	}
	return nil
}

// last returns the last element of the list.
func last(a []any) any {
	if len(a) > 0 {
		return a[len(a)-1]
	}
	return nil
}

// hasKey checks if the map has the key.
func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

// keys returns the keys of the map.
func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// formatDate formats the time with the layout.
func formatDate(t time.Time, layout string) string {
	return t.Format(layout)
}

// parseDate parses the time with the layout.
func parseDate(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

// debug returns the string representation of the value.
func debug(v any) string {
	return fmt.Sprintf("%+v", v)
}

func selectionFunc(p *Partial, data *Data) func() template.HTML {
	return func() template.HTML {
		var selectedPartial *Partial

		partials := p.getSelectionPartials()
		if partials == nil {
			p.getLogger().Error("no selection partials found", "id", p.id)
			return template.HTML(fmt.Sprintf("no selection partials found in parent '%s'", p.id))
		}

		requestedSelect := p.getConnector().GetSelectValue(p.GetRequest())
		if requestedSelect != "" {
			selectedPartial = partials[requestedSelect]
		} else {
			selectedPartial = partials[p.selection.Default]
		}

		if selectedPartial == nil {
			p.getLogger().Error("selected partial not found", "id", requestedSelect, "parent", p.id)
			return template.HTML(fmt.Sprintf("selected partial '%s' not found in parent '%s'", requestedSelect, p.id))
		}

		selectedPartial.fs = p.fs

		html, err := selectedPartial.renderSelf(data.Ctx, p.GetRequest())
		if err != nil {
			p.getLogger().Error("error rendering selected partial", "id", requestedSelect, "parent", p.id, "error", err)
			return template.HTML(fmt.Sprintf("error rendering selected partial '%s'", requestedSelect))
		}

		return html
	}
}

func childFunc(p *Partial, data *Data) func(id string, vals ...any) template.HTML {
	return func(id string, vals ...any) template.HTML {
		if len(vals) > 0 && len(vals)%2 != 0 {
			p.getLogger().Warn("invalid child data for partial, they come in key-value pairs", "id", id)
			return template.HTML(fmt.Sprintf("invalid child data for partial '%s'", id))
		}

		d := make(map[string]any)
		for i := 0; i < len(vals); i += 2 {
			key, ok := vals[i].(string)
			if !ok {
				p.getLogger().Warn("invalid child data key for partial, it must be a string", "id", id, "key", vals[i])
				return template.HTML(fmt.Sprintf("invalid child data key for partial '%s', want string, got %T", id, vals[i]))
			}
			d[key] = vals[i+1]
		}

		html, err := p.renderChildPartial(data.Ctx, id, d)
		if err != nil {
			p.getLogger().Error("error rendering partial", "id", id, "error", err)
			// Handle error: you can log it and return an empty string or an error message
			return template.HTML(fmt.Sprintf("error rendering partial '%s': %v", id, err))
		}

		return html
	}
}

func childIfFunc(p *Partial, data *Data) func(id string, vals ...any) template.HTML {
	return func(id string, vals ...any) template.HTML {
		if len(p.children) == 0 {
			return ""
		}

		if p.children[id] == nil {
			return ""
		}

		return childFunc(p, data)(id, vals...)
	}
}

func actionFunc(p *Partial, data *Data) func() template.HTML {
	return func() template.HTML {
		if p.templateAction == nil {
			p.getLogger().Error("no action callback found", "id", p.id)
			return template.HTML(fmt.Sprintf("no action callback found in partial '%s'", p.id))
		}

		// Use the selector to get the appropriate partial
		actionPartial, err := p.templateAction(data.Ctx, p, data)
		if err != nil {
			p.getLogger().Error("error in selector function", "error", err)
			return template.HTML(fmt.Sprintf("error in action function: %v", err))
		}

		// Render the selected partial instead
		html, err := actionPartial.renderSelf(data.Ctx, p.GetRequest())
		if err != nil {
			p.getLogger().Error("error rendering action partial", "error", err)
			return template.HTML(fmt.Sprintf("error rendering action partial: %v", err))
		}
		return html
	}
}
