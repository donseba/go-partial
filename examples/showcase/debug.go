package main

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	partial "github.com/donseba/go-partial"
)

func (app *App) debugPage(w http.ResponseWriter, r *http.Request) {
	custom := partial.NewID("custom-debug", "templates/debug_custom.gohtml").
		SetDot(DebugCustomPage{Name: "Ada", Role: "Editor"}).
		SetDebugRenderer(func(ctx context.Context, p *partial.Partial, data *partial.Data, value any) (template.HTML, error) {
			return template.HTML(customDebugHTML(value)), nil
		})

	content := partial.NewID("content", "templates/debug.gohtml").SetDot(DebugPage{
		Title: "Debug helper",
		Payload: map[string]any{
			"User":  "Ada",
			"Role":  "Editor",
			"Flags": []string{"beta", "preview"},
		},
	})
	content.With(custom)
	app.renderPartial(w, r, content)
}

func customDebugHTML(value any) string {
	var b strings.Builder
	b.WriteString(`<aside class="custom-debug"><header><strong>Custom debug</strong><span>key/value view</span></header>`)
	if values, ok := value.(map[string]any); ok {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString(`<dl>`)
		for _, key := range keys {
			b.WriteString(`<dt>`)
			b.WriteString(template.HTMLEscapeString(key))
			b.WriteString(`</dt><dd>`)
			b.WriteString(template.HTMLEscapeString(fmt.Sprint(values[key])))
			b.WriteString(`</dd>`)
		}
		b.WriteString(`</dl></aside>`)
		return b.String()
	}
	b.WriteString(`<pre>`)
	b.WriteString(template.HTMLEscapeString(fmt.Sprintf("%#v", value)))
	b.WriteString(`</pre></aside>`)
	return b.String()
}
