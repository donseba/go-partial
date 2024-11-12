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
