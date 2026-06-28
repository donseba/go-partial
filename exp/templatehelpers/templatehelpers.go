// Package templatehelpers exposes optional convenience helpers for templates.
//
// These helpers are useful in applications, but they are not required for
// go-partial's render tree, request handling, or connector behavior. Register
// them explicitly when an application wants them:
//
//	service.SetFunc(templatehelpers.FuncMap())
package templatehelpers

import (
	"fmt"
	"html/template"
	"maps"
	"net/url"
	"strings"
	"time"
	"unicode"
)

var helperFuncMap = template.FuncMap{
	"safeHTML": safeHTML,

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

	"now":        time.Now,
	"formatDate": formatDate,
	"parseDate":  parseDate,

	"first": first,
	"last":  last,

	"dict":   dict,
	"hasKey": hasKey,
	"keys":   keys,

	"inc": inc,
	"dec": dec,
}

// FuncMap returns a fresh copy of the optional helper function map.
func FuncMap() template.FuncMap {
	return maps.Clone(helperFuncMap)
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

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

func title(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	capitalizeNext := true
	for i := range runes {
		if unicode.IsSpace(runes[i]) {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			runes[i] = unicode.ToUpper(runes[i])
			capitalizeNext = false
			continue
		}
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

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

func first(a []any) any {
	if len(a) == 0 {
		return nil
	}
	return a[0]
}

func last(a []any) any {
	if len(a) == 0 {
		return nil
	}
	return a[len(a)-1]
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

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
		return v + numericArg(args, 1)
	case int64:
		return v + int64(numericArg(args, 1))
	case float64:
		return v + float64(numericArg(args, 1))
	case float32:
		return v + float32(numericArg(args, 1))
	case uint:
		return v + uint(numericArg(args, 1))
	default:
		return args[0]
	}
}

func dec(args ...any) any {
	if len(args) == 0 {
		return -1
	}
	switch v := args[0].(type) {
	case int:
		return v - numericArg(args, 1)
	case int64:
		return v - int64(numericArg(args, 1))
	case float64:
		return v - float64(numericArg(args, 1))
	case float32:
		return v - float32(numericArg(args, 1))
	case uint:
		return v - uint(numericArg(args, 1))
	default:
		return args[0]
	}
}

func numericArg(args []any, fallback int) int {
	if len(args) < 2 {
		return fallback
	}
	switch v := args[1].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case uint:
		return int(v)
	default:
		return fallback
	}
}

func formatDate(layout string, t time.Time) string {
	return t.Format(layout)
}

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
