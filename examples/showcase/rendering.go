package main

import (
	"bytes"
	"context"
	"html/template"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func (app *App) render(w http.ResponseWriter, r *http.Request, id string, tmpl string, data any) {
	content := partial.NewID(id, tmpl)
	if data != nil {
		if values, ok := data.(map[string]any); ok {
			content.SetData(values)
		} else {
			content.SetDot(data)
		}
	}
	app.renderPartial(w, r, content)
}

func (app *App) renderPartial(w http.ResponseWriter, r *http.Request, content *partial.Partial) {
	layout := app.service.NewLayout().Set(content).Wrap(app.wrapper())
	app.writeLayout(w, r, layout)
}

func (app *App) writeContent(w http.ResponseWriter, r *http.Request, content *partial.Partial) {
	content.SetConnector(connector.NewHTMX(nil))
	content.SetFileSystem(os.DirFS("examples/showcase"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := content.WriteWithRequest(app.requestContext(r), w, r); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (app *App) writeLayout(w http.ResponseWriter, r *http.Request, layout *partial.Layout) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := layout.WriteWithRequest(app.requestContext(r), w, r); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (app *App) writeStandalone(w http.ResponseWriter, r *http.Request, content *partial.Partial) {
	content.SetFileSystem(os.DirFS("examples/showcase"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	out, err := content.Render(app.requestContext(r))
	if err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write([]byte(out))
}

func (app *App) wrapper() *partial.Partial {
	wrapper := partial.NewID("layout", "templates/layout.gohtml")
	wrapper.WithOOB(partial.NewID("header", "templates/header.gohtml").SetDot(HeaderPage{
		AppName: "go-partial showcase",
		Now:     time.Now().Format("02 Jan 06 15:04 MST"),
		Nav:     app.navItems(),
		Joke:    app.programmerJoke(),
	}).SetAlwaysSwapOOB(true))
	return wrapper
}

func showcaseInteractionRenderer() partial.InteractionRenderer {
	return func(ctx context.Context, p *partial.Partial, data *partial.Data, interaction connector.Interaction, attrs map[string]string) (template.HTML, error) {
		attrText := showcaseInteractionAttrs(attrs)
		placeholder := template.HTMLEscapeString(interaction.Placeholder)

		switch interaction.Kind {
		case connector.InteractionPrefetch:
			return template.HTML(`<link ` + attrText + `>`), nil
		case connector.InteractionRefresh:
			return template.HTML(`<button type="button" id="` + template.HTMLEscapeString(interaction.ID) + `" class="inline-flex min-h-9 w-fit cursor-pointer items-center rounded-md border border-stone-300 bg-white px-3 py-2 text-sm font-bold text-stone-950 hover:border-teal-700 hover:text-teal-900" ` + attrText + `>` + placeholder + `</button>`), nil
		case connector.InteractionOn:
			return template.HTML(`<div id="` + template.HTMLEscapeString(interaction.ID) + `" class="hidden" ` + attrText + `></div>`), nil
		default:
			return template.HTML(`<div id="` + template.HTMLEscapeString(interaction.ID) + `" class="grid min-h-24 content-center rounded-lg border border-dashed border-stone-300 bg-stone-50 px-4 py-3 text-sm font-bold text-stone-600" ` + attrText + `>` + placeholder + `</div>`), nil
		}
	}
}

func showcaseInteractionAttrs(attrs map[string]string) string {
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		if key == "id" || key == "class" || strings.HasPrefix(key, "__") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(template.HTMLEscapeString(key))
		b.WriteString(`="`)
		b.WriteString(template.HTMLEscapeString(attrs[key]))
		b.WriteByte('"')
	}
	return b.String()
}

var showcaseProgrammerJokes = []string{
	"There are only two hard things in computer science: cache invalidation, naming things, and off-by-one errors.",
	"I told my HTML it needed therapy. It said it had too many unresolved div issues.",
	"Why do programmers prefer dark mode? Because light attracts bugs.",
	"My code compiled on the first try, so naturally I checked the wrong folder.",
	"A SQL query walks into a bar, walks up to two tables, and asks: can I join you?",
	"Debugging: being the detective in a crime movie where you are also the culprit.",
	"I would tell you a UDP joke, but you might not get it.",
	"TCP jokes are better because you always get an acknowledgement.",
	"The programmer went broke because he used up all his cache.",
	"My password is the last eight digits of pi.",
	"Recursive jokes are funny because recursive jokes are funny.",
	"I named my hard drive Datacenter. Now I can say it is down.",
	"The cloud is just someone else's computer wearing a cape.",
	"I asked Git for commitment. It gave me a detached head.",
	"The frontend developer left the party because there was no class.",
	"The backend developer stayed because there was a queue.",
	"My regex works perfectly, except on text.",
	"Production is where all TODO comments go to become folklore.",
	"I tried to explain DNS, but it took too long to resolve.",
	"Unit tests passed, so the bug moved into integration.",
	"My build pipeline is just optimism with logs.",
	"The API said 200 OK, which was very polite of it.",
	"I have a joke about race conditions, but it already happened.",
	"I have a joke about async, but I will tell you later.",
	"I have a joke about blocking calls, but everyone is waiting.",
	"I have a joke about null, but there is nothing to say.",
	"I have a joke about pointers, but it points somewhere else.",
	"I have a joke about memory leaks, but I forgot where I put it.",
	"I have a joke about indexes, but it starts at one by mistake.",
	"I have a joke about arrays, but it is out of bounds.",
	"I have a joke about interfaces, but it depends on the implementation.",
	"I have a joke about microservices, but it needs twelve services to land.",
	"I have a joke about containers, but it works on my machine.",
	"I have a joke about Kubernetes, but it needs a cluster to explain it.",
	"I have a joke about logs, but it is buried in noise.",
	"I have a joke about observability, but nobody can trace it.",
	"I have a joke about feature flags, but it is disabled in production.",
	"I have a joke about migrations, but it cannot roll back cleanly.",
	"I have a joke about timezones, but you heard it yesterday tomorrow.",
	"I have a joke about date parsing, but the format is wrong.",
	"I have a joke about JSON, but it is missing a comma.",
	"I have a joke about YAML, but the indentation ruined it.",
	"I have a joke about XML, but it closed itself.",
	"I have a joke about CSS, but specificity won.",
	"I have a joke about JavaScript, but it became a string.",
	"I have a joke about Go errors, but I handled it immediately.",
	"I have a joke about goroutines, but now there are 400 of them.",
	"I have a joke about channels, but nobody is receiving.",
	"I have a joke about templates, but the data was nil.",
	"I have a joke about htmx, but it swapped itself out.",
	"I have a joke about partial rendering, but only this section gets it.",
	"I have a joke about server-side rendering, but the client wanted hydration.",
	"I have a joke about cache busting, but you heard the old version.",
	"I have a joke about semver, but it is a breaking patch.",
	"I have a joke about documentation, but it is coming soon.",
}

func (app *App) programmerJoke() string {
	return showcaseProgrammerJokes[rand.N(len(showcaseProgrammerJokes))]
}
