package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"

	partial "github.com/donseba/go-partial"
)

func (app *App) debugPage(w http.ResponseWriter, r *http.Request) {
	custom := partial.NewID("custom-debug", "templates/debug_custom.gohtml").
		SetFileSystem(os.DirFS("examples/showcase")).
		SetDot(DebugCustomPage{Name: "Ada", Role: "Editor"}).
		SetDebugRenderer(func(ctx context.Context, p *partial.Partial, data *partial.Data, value any) (template.HTML, error) {
			return template.HTML(customDebugHTML(value)), nil
		})
	customHTML, err := custom.Render(app.requestContext(r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	content := partial.NewID("content", "templates/debug.gohtml").SetDot(DebugPage{
		Title: "Debug helper",
		Payload: map[string]any{
			"User":  "Ada",
			"Role":  "Editor",
			"Flags": []string{"beta", "preview"},
		},
		CustomDebug: customHTML,
	})
	app.renderPartial(w, r, content)
}

func customDebugHTML(value any) string {
	var b strings.Builder
	b.WriteString(`<aside class="grid gap-3 rounded-lg border border-teal-700/40 border-l-4 bg-teal-50/50 p-4 text-stone-950">`)
	b.WriteString(`<header class="flex items-center justify-between gap-3">`)
	b.WriteString(`<strong class="text-[13px] font-extrabold uppercase tracking-normal text-teal-900">Custom debug</strong>`)
	b.WriteString(`<span class="text-sm text-stone-500">key/value view</span>`)
	b.WriteString(`</header>`)
	if values, ok := debugPairs(value); ok {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString(`<dl class="grid gap-2 rounded-md border border-teal-700/20 bg-white p-3 sm:grid-cols-[9rem_1fr]">`)
		for _, key := range keys {
			b.WriteString(`<dt class="font-bold text-teal-950">`)
			b.WriteString(template.HTMLEscapeString(key))
			b.WriteString(`</dt><dd class="min-w-0 break-words font-mono text-sm text-stone-700">`)
			b.WriteString(template.HTMLEscapeString(fmt.Sprint(values[key])))
			b.WriteString(`</dd>`)
		}
		b.WriteString(`</dl></aside>`)
		return b.String()
	}
	b.WriteString(`<pre class="overflow-auto rounded-md border border-teal-700/20 bg-white p-3 font-mono text-sm text-stone-700">`)
	b.WriteString(template.HTMLEscapeString(fmt.Sprintf("%#v", value)))
	b.WriteString(`</pre></aside>`)
	return b.String()
}

func debugPairs(value any) (map[string]any, bool) {
	if values, ok := value.(map[string]any); ok {
		return values, true
	}
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return nil, false
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, false
	}
	rt := rv.Type()
	values := make(map[string]any, rv.NumField())
	for i := 0; i < rv.NumField(); i++ {
		field := rt.Field(i)
		if field.PkgPath != "" {
			continue
		}
		values[field.Name] = rv.Field(i).Interface()
	}
	return values, true
}
