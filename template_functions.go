package partial

import (
	"fmt"
	"html/template"
	"maps"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/donseba/go-partial/connector"
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
	"upperFirst":  upperFirst,
	"compare":     strings.Compare,
	"equalFold":   strings.EqualFold,
	"urlEncode":   url.QueryEscape,
	"urlDecode":   url.QueryUnescape,
	"safeURL":     safeURL,
	// Time functions

	"now":        time.Now,
	"formatDate": formatDate,
	"parseDate":  parseDate,

	// List functions
	"first": first,
	"last":  last,

	// Map functions
	"dict":   dict,
	"hasKey": hasKey,
	"keys":   keys,

	"inc": inc,
	"dec": dec,
}

// AddGlobalFunc adds a function to the package-level default template function map.
// Services created after this call receive the function when they copy the defaults.
func AddGlobalFunc(name string, f any) error {
	if _, ok := protectedFunctionNames[name]; ok {
		return fmt.Errorf("function name [%s] is protected and cannot be overwritten", name)
	}

	DefaultTemplateFuncMap[name] = f
	return nil
}

func copyFuncMap() template.FuncMap {
	return maps.Clone(DefaultTemplateFuncMap)
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

// upperFirst capitalizes the first character of the string.
func upperFirst(s string) string {
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

func safeURL(s string) template.URL {
	if s == "" {
		return template.URL("")
	}
	return template.URL(url.QueryEscape(s))
}

func inc(args ...any) any {
	if len(args) == 0 {
		return 1
	}
	switch v := args[0].(type) {
	case int:
		if len(args) > 1 {
			if by, ok := args[1].(int); ok {
				return v + by
			}
		}
		return v + 1
	case int64:
		if len(args) > 1 {
			if by, ok := args[1].(int64); ok {
				return v + by
			}
		}
		return v + 1
	case float64:
		if len(args) > 1 {
			if by, ok := args[1].(float64); ok {
				return v + by
			}
		}
		return v + 1
	case float32:
		if len(args) > 1 {
			if by, ok := args[1].(float32); ok {
				return v + by
			}
		}
		return v + 1
	case uint:
		if len(args) > 1 {
			if by, ok := args[1].(uint); ok {
				return v + by
			}
		}
		return v + 1
	}
	return args[0]
}

func dec(args ...any) any {
	if len(args) == 0 {
		return -1
	}
	switch v := args[0].(type) {
	case int:
		if len(args) > 1 {
			if by, ok := args[1].(int); ok {
				return v - by
			}
		}
		return v - 1
	case int64:
		if len(args) > 1 {
			if by, ok := args[1].(int64); ok {
				return v - by
			}
		}
		return v - 1
	case float64:
		if len(args) > 1 {
			if by, ok := args[1].(float64); ok {
				return v - by
			}
		}
		return v - 1
	case float32:
		if len(args) > 1 {
			if by, ok := args[1].(float32); ok {
				return v - by
			}
		}
		return v - 1
	case uint:
		if len(args) > 1 {
			if by, ok := args[1].(uint); ok {
				return v - by
			}
		}
		return v - 1
	}
	return args[0]
}

// formatDate formats the time with the layout.
func formatDate(layout string, t time.Time) string {
	return t.Format(layout)
}

// parseDate parses the time with the layout.
func parseDate(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict expects key/value pairs")
	}

	out := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict key must be string, got %T", values[i])
		}
		out[key] = values[i+1]
	}

	return out, nil
}

func selectionFunc(p *Partial, data *Data) func() template.HTML {
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

		html, err := selectedClone.renderSelf(data.Ctx, p.GetRequest())
		if err != nil {
			p.getLogger().Error("error rendering selected partial", "id", selectionValue, "parent", p.id, "error", err)
			fallback, fallbackErr := selectedClone.renderErrorFragment(data.Ctx, p.GetRequest(), err)
			if fallbackErr != nil {
				p.getLogger().Error("error rendering selected partial fallback", "id", selectionValue, "parent", p.id, "error", fallbackErr)
				return template.HTML(fmt.Sprintf("error rendering selected partial '%s': %v", selectionValue, fallbackErr))
			}
			return fallback
		}

		return html
	}
}

func childFunc(p *Partial, data *Data) func(id string, args ...any) template.HTML {
	return func(id string, args ...any) template.HTML {
		d, ok := scopedDataArg(p, id, args...)
		if !ok {
			return template.HTML(fmt.Sprintf("invalid scoped data for partial '%s'", id))
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

func partialFunc(p *Partial, data *Data) func(id string, args ...any) template.HTML {
	return func(id string, args ...any) template.HTML {
		d, ok := scopedDataArg(p, id, args...)
		if !ok {
			return template.HTML(fmt.Sprintf("invalid scoped data for partial '%s'", id))
		}

		html, err := p.renderChildPartial(data.Ctx, id, d)
		if err != nil {
			p.getLogger().Error("error rendering partial", "id", id, "error", err)
			return template.HTML(fmt.Sprintf("error rendering partial '%s': %v", id, err))
		}

		return html
	}
}

func interactionFunc(p *Partial, data *Data, kind connector.InteractionKind) func(value any, args ...any) template.HTML {
	return func(value any, args ...any) template.HTML {
		interaction, err := interactionFromValue(kind, "", value, args...)
		if err != nil {
			p.getLogger().Warn("invalid interaction arguments", "kind", kind, "error", err)
			return template.HTML(template.HTMLEscapeString(err.Error()))
		}

		return renderInteraction(p, data, interaction)
	}
}

func islandFunc(p *Partial, data *Data) func(value any, args ...any) template.HTML {
	return func(value any, args ...any) template.HTML {
		interaction, err := islandInteractionFromValue(value, args...)
		if err != nil {
			p.getLogger().Warn("invalid island arguments", "error", err)
			return template.HTML(template.HTMLEscapeString(err.Error()))
		}
		return renderInteraction(p, data, interaction)
	}
}

func onFunc(p *Partial, data *Data) func(value any, args ...any) template.HTML {
	return func(value any, args ...any) template.HTML {
		interaction, err := onInteractionFromValue(value, args...)
		if err != nil {
			p.getLogger().Warn("invalid on arguments", "error", err)
			return template.HTML(template.HTMLEscapeString(err.Error()))
		}
		return renderInteraction(p, data, interaction)
	}
}

func interactionFromValue(kind connector.InteractionKind, name string, value any, args ...any) (connector.Interaction, error) {
	switch v := value.(type) {
	case string:
		if kind == connector.InteractionPoll && len(args) == 1 {
			args = []any{"every", args[0]}
		}
		return buildInteraction(kind, name, v, args...)
	case Interaction:
		return normalizeInteraction(v.Interaction()), nil
	case *Interaction:
		if v == nil {
			return connector.Interaction{}, fmt.Errorf("interaction is nil")
		}
		return normalizeInteraction(v.Interaction()), nil
	case connector.Interaction:
		return normalizeInteraction(v), nil
	case InteractionConfig:
		return normalizeInteraction(v.Interaction()), nil
	default:
		return connector.Interaction{}, fmt.Errorf("interaction helper expects an endpoint string or interaction config, got %T", value)
	}
}

func onInteractionFromValue(value any, args ...any) (connector.Interaction, error) {
	event, ok := value.(string)
	if !ok {
		return interactionFromValue(connector.InteractionOn, "", value, args...)
	}
	if len(args) == 0 {
		return connector.Interaction{}, fmt.Errorf("on expects an endpoint when the first argument is an event")
	}
	endpoint, ok := args[0].(string)
	if !ok {
		return connector.Interaction{}, fmt.Errorf("on endpoint must be string, got %T", args[0])
	}

	interaction, err := buildInteraction(connector.InteractionOn, event, endpoint, args[1:]...)
	if err != nil {
		return connector.Interaction{}, err
	}
	interaction.Trigger = event
	if interaction.Options == nil {
		interaction.Options = make(map[string]string)
	}
	if _, ok := interaction.Options["from"]; !ok {
		interaction.Options["from"] = "body"
	}
	if _, ok := interaction.Options["placeholder"]; !ok {
		interaction.Placeholder = ""
	}
	return normalizeInteraction(interaction), nil
}

func islandInteractionFromValue(value any, args ...any) (connector.Interaction, error) {
	if _, ok := value.(string); !ok {
		return interactionFromValue(connector.InteractionIsland, "", value, args...)
	}
	name := value.(string)
	if len(args) == 0 {
		return connector.Interaction{}, fmt.Errorf("island expects an endpoint when the first argument is a name")
	}
	endpoint, ok := args[0].(string)
	if !ok {
		return connector.Interaction{}, fmt.Errorf("island endpoint must be string, got %T", args[0])
	}
	return buildInteraction(connector.InteractionIsland, name, endpoint, args[1:]...)
}

func renderInteraction(p *Partial, data *Data, interaction connector.Interaction) template.HTML {
	conn := p.getConnector()
	if conn == nil {
		conn = connector.NewPartial(nil)
	}

	attrs := conn.InteractionAttrs(interaction)
	renderer := p.getInteractionRenderer()
	out, err := renderer(data.Ctx, p, data, interaction, attrs)
	if err != nil {
		p.getLogger().Error("error rendering interaction", "kind", interaction.Kind, "id", interaction.ID, "error", err)
		return template.HTML(template.HTMLEscapeString(err.Error()))
	}
	return out
}

func buildInteraction(kind connector.InteractionKind, name string, endpoint string, args ...any) (connector.Interaction, error) {
	values, err := interactionValues(args...)
	if err != nil {
		return connector.Interaction{}, err
	}

	resolvedURL := resolveInteractionURL(endpoint, values)
	if name == "" {
		name = values["name"]
	}

	return normalizeInteraction(connector.Interaction{
		Kind:        kind,
		ID:          values["id"],
		URL:         resolvedURL,
		Target:      values["target"],
		Swap:        values["swap"],
		Trigger:     values["trigger"],
		Interval:    values["every"],
		Placeholder: values["placeholder"],
		Name:        name,
		Params:      values,
		Options:     values,
	}), nil
}

func normalizeInteraction(interaction connector.Interaction) connector.Interaction {
	if interaction.Options == nil {
		interaction.Options = make(map[string]string)
	}
	if interaction.Kind == connector.InteractionOn {
		if interaction.Trigger == "" {
			interaction.Trigger = interaction.Name
		}
		if _, ok := interaction.Options["from"]; !ok {
			interaction.Options["from"] = "body"
		}
	}
	if interaction.ID == "" {
		idBase := interaction.URL
		if interaction.Name != "" {
			idBase = interaction.Name
		}
		interaction.ID = string(interaction.Kind) + "-" + sanitizeInteractionID(idBase)
	}
	if interaction.Target == "" {
		interaction.Target = "#" + interaction.ID
	}
	if _, placeholderSet := interaction.Options["placeholder"]; interaction.Placeholder == "" && !placeholderSet {
		if interaction.Kind == connector.InteractionOn {
			interaction.Placeholder = ""
		} else {
			interaction.Placeholder = "Loading..."
		}
	}
	return interaction
}

func interactionValues(args ...any) (map[string]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	if len(args)%2 != 0 {
		return nil, fmt.Errorf("interaction helpers expect key/value pairs")
	}

	values := make(map[string]string, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			return nil, fmt.Errorf("interaction option key must be string, got %T", args[i])
		}
		values[key] = fmt.Sprint(args[i+1])
	}
	return values, nil
}

func resolveInteractionURL(endpoint string, values map[string]string) string {
	resolved := strings.TrimSpace(endpoint)
	if resolved == "" {
		resolved = "/"
	}
	if !strings.HasPrefix(resolved, "/") && !strings.HasPrefix(resolved, "http://") && !strings.HasPrefix(resolved, "https://") {
		resolved = "/" + resolved
	}

	for key, value := range values {
		resolved = strings.ReplaceAll(resolved, ":"+key, url.PathEscape(value))
	}

	return resolved
}

func sanitizeInteractionID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "content"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "content"
	}
	return out
}

func childIfFunc(p *Partial, data *Data) func(id string, args ...any) template.HTML {
	return func(id string, args ...any) template.HTML {
		if len(p.children) == 0 {
			return ""
		}

		if p.children[id] == nil {
			return ""
		}

		return childFunc(p, data)(id, args...)
	}
}

func scopedDataArg(p *Partial, id string, args ...any) (map[string]any, bool) {
	if len(args) == 0 {
		return nil, true
	}
	if len(args) == 1 {
		if scoped, ok := args[0].(map[string]any); ok {
			return scoped, true
		}
		p.getLogger().Warn("invalid scoped data for partial, pass a map or key/value pairs", "id", id, "type", fmt.Sprintf("%T", args[0]))
		return nil, false
	}
	if len(args)%2 != 0 {
		p.getLogger().Warn("invalid scoped data for partial, pass key/value pairs", "id", id)
		return nil, false
	}

	scoped := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			p.getLogger().Warn("invalid scoped data key for partial", "id", id, "type", fmt.Sprintf("%T", args[i]))
			return nil, false
		}
		scoped[key] = args[i+1]
	}
	return scoped, true
}

func debugFunc(p *Partial, data *Data) func(value any) template.HTML {
	return func(value any) template.HTML {
		renderer := p.getDebugRenderer()
		if renderer == nil {
			return template.HTML(template.HTMLEscapeString(fmt.Sprintf("%#v", value)))
		}

		out, err := renderer(data.Ctx, p, data, value)
		if err != nil {
			p.getLogger().Error("error rendering debug helper", "id", p.id, "error", err)
			return template.HTML(`<pre class="go-partial-debug">` + template.HTMLEscapeString(fmt.Sprintf("%#v", value)) + `</pre>`)
		}
		return out
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
