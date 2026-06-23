package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

type Row struct {
	ID     int
	Name   string
	Price  string
	Status string
	Owner  string
}

type NavItem struct {
	Path  string
	Label string
	Group string
}

type App struct {
	service      *partial.Service
	rows         []Row
	counter      int
	flowSessions map[string]*partial.FlowSessionData
}

type showcaseLocalizer struct {
	locale string
}

type showcaseCsrf struct {
	key   string
	token string
}

func (c showcaseCsrf) Token(ctx context.Context) string {
	return c.token
}

func (c showcaseCsrf) Key() string {
	return c.key
}

func (l showcaseLocalizer) GetLocale() string {
	return l.locale
}

type showcaseTranslator struct {
	messages map[string]map[string]string
}

func (t showcaseTranslator) FuncMap() template.FuncMap {
	return template.FuncMap{
		"tl":  t.tl,
		"tn":  t.tn,
		"ctl": t.ctl,
		"ctn": t.ctn,
	}
}

func (t showcaseTranslator) tl(loc partial.Localizer, key string, args ...any) string {
	return t.translate(loc.GetLocale(), key, args...)
}

func (t showcaseTranslator) tn(loc partial.Localizer, singular string, plural string, n int, args ...any) string {
	key := plural
	if n == 1 {
		key = singular
	}
	return t.translate(loc.GetLocale(), key, args...)
}

func (t showcaseTranslator) ctl(loc partial.Localizer, context string, key string, args ...any) string {
	return t.translate(loc.GetLocale(), context+"."+key, args...)
}

func (t showcaseTranslator) ctn(loc partial.Localizer, context string, singular string, plural string, n int, args ...any) string {
	key := plural
	if n == 1 {
		key = singular
	}
	return t.translate(loc.GetLocale(), context+"."+key, args...)
}

func (t showcaseTranslator) translate(locale string, key string, args ...any) string {
	values, ok := t.messages[locale]
	if !ok {
		values = t.messages["en_US"]
	}
	value, ok := values[key]
	if !ok {
		value = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(value, args...)
	}
	return value
}

func main() {
	app := &App{
		service: partial.NewService(&partial.Config{
			Connector:        connector.NewHTMX(nil),
			FS:               os.DirFS("examples/showcase"),
			ErrorMode:        partial.ErrorModeDetailed,
			UseTemplateCache: false,
		}),
		rows: []Row{
			{ID: 1, Name: "Coffee", Price: "12.50", Status: "Ready", Owner: "Ada"},
			{ID: 2, Name: "Tea", Price: "4.25", Status: "Brewing", Owner: "Ada"},
			{ID: 3, Name: "Cake", Price: "6.75", Status: "Queued", Owner: "Ada"},
		},
		flowSessions: make(map[string]*partial.FlowSessionData),
	}
	app.service.UseFuncs(showcaseTranslationFunctions())

	app.service.SetData(map[string]any{
		"AppName": "go-partial showcase",
		"Now":     time.Now().Format(time.RFC822),
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.home)
	mux.HandleFunc("/scoped", app.scoped)
	mux.HandleFunc("/scoped/refresh-row", app.refreshRow)
	mux.HandleFunc("/selection", app.selection)
	mux.HandleFunc("/tabs", app.tabs)
	mux.HandleFunc("/action", app.action)
	mux.HandleFunc("/oob", app.oob)
	mux.HandleFunc("/oob/ping", app.oobPing)
	mux.HandleFunc("/context", app.contextPage)
	mux.HandleFunc("/debug", app.debugPage)
	mux.HandleFunc("/localization", app.localization)
	mux.HandleFunc("/headers", app.headers)
	mux.HandleFunc("/headers/notify", app.headersNotify)
	mux.HandleFunc("/flow", app.flow)
	mux.HandleFunc("/sse", app.sse)
	mux.HandleFunc("/sse/stream", app.sseStream)
	mux.HandleFunc("/infinite", app.infinite)
	mux.HandleFunc("/infinite/load", app.infiniteLoad)
	mux.HandleFunc("/error", app.errorPage)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("examples/showcase/static"))))

	log.Println("showcase running on http://localhost:8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}

func (app *App) home(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/home.gohtml", map[string]any{
		"Title": "Server-rendered partials",
	})
}

func (app *App) scoped(w http.ResponseWriter, r *http.Request) {
	content := app.tablePartial()
	app.renderPartial(w, r, content)
}

func (app *App) tablePartial() *partial.Partial {
	rowPartial := partial.NewID("row", "templates/row.gohtml")
	content := partial.NewID("content", "templates/scoped.gohtml").SetData(map[string]any{
		"Title": "Scoped rows",
		"Owner": "Ada",
		"Rows":  app.rows,
	})
	content.With(rowPartial)
	content.WithTargetResolver(func(ctx context.Context, r *http.Request, target string) (*partial.Partial, map[string]any, bool) {
		if !strings.HasPrefix(target, "row-") {
			return nil, nil, false
		}
		id, err := strconv.Atoi(strings.TrimPrefix(target, "row-"))
		if err != nil {
			return nil, nil, false
		}
		for _, row := range app.rows {
			if row.ID == id {
				row.Status = "Updated " + time.Now().Format("15:04:05")
				return rowPartial, map[string]any{"Row": row, "Owner": "Ada"}, true
			}
		}
		return nil, nil, false
	})
	return content
}

func (app *App) refreshRow(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("id")
	app.writeContent(w, r, app.tablePartial())
}

func (app *App) selection(w http.ResponseWriter, r *http.Request) {
	summary := partial.NewID("summary", "templates/selection_summary.gohtml").SetData(map[string]any{
		"Title": "Summary",
	})
	details := partial.NewID("details", "templates/selection_details.gohtml").SetData(map[string]any{
		"Title": "Details",
	})
	content := partial.NewID("content", "templates/selection.gohtml").SetData(map[string]any{
		"Title": "Selection partials",
	})
	content.WithSelectMap("summary", map[string]*partial.Partial{
		"summary": summary,
		"details": details,
	})
	app.renderPartial(w, r, content)
}

func (app *App) tabs(w http.ResponseWriter, r *http.Request) {
	overview := partial.NewID("overview", "templates/tabs_overview.gohtml")
	activity := partial.NewID("activity", "templates/tabs_activity.gohtml")
	settings := partial.NewID("settings", "templates/tabs_settings.gohtml")
	failing := partial.NewID("failing", "templates/tabs_failing.gohtml")
	content := partial.NewID("content", "templates/tabs.gohtml").SetData(map[string]any{
		"Title": "Tabs with selection",
		"Tabs": []map[string]string{
			{"Key": "overview", "Label": "Overview"},
			{"Key": "activity", "Label": "Activity"},
			{"Key": "settings", "Label": "Settings"},
			{"Key": "failing", "Label": "Fails"},
		},
	})
	content.WithSelectMap("overview", map[string]*partial.Partial{
		"overview": overview,
		"activity": activity,
		"settings": settings,
		"failing":  failing,
	})
	app.renderPartial(w, r, content)
}

func (app *App) action(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.Header.Get(connector.HeaderAction.String()) == "increment" {
		app.counter++
	}
	app.render(w, r, "content", "templates/action.gohtml", map[string]any{
		"Title":        "Action callbacks",
		"Counter":      app.counter,
		"ActionHeader": connector.HeaderAction.String(),
	})
}

func (app *App) oob(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/oob.gohtml", map[string]any{
		"Title": "Out-of-band updates",
	})
}

func (app *App) oobPing(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/oob.gohtml").SetData(map[string]any{
		"Title": "Out-of-band updates",
		"Ping":  time.Now().Format("15:04:05"),
	})
	wrapper := app.wrapper()
	wrapper.WithOOB(partial.NewID("toast", "templates/toast.gohtml").SetData(map[string]any{
		"Message": "OOB toast rendered at " + time.Now().Format("15:04:05"),
	}).SetAlwaysSwapOOB(true))
	layout := app.service.NewLayout().Set(content).Wrap(wrapper)
	app.writeLayout(w, r, layout)
}

func (app *App) contextPage(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/context.gohtml").SetData(map[string]any{
		"Title": "Context helpers",
	})
	app.renderPartial(w, r, content)
}

func (app *App) debugPage(w http.ResponseWriter, r *http.Request) {
	custom := partial.NewID("custom-debug", "templates/debug_custom.gohtml").
		SetData(map[string]any{"Name": "Ada", "Role": "Editor"}).
		SetDebugRenderer(func(ctx context.Context, p *partial.Partial, data *partial.Data, value any) (template.HTML, error) {
			return template.HTML(customDebugHTML(value)), nil
		})

	content := partial.NewID("content", "templates/debug.gohtml").SetData(map[string]any{
		"Title": "Debug helper",
		"Payload": map[string]any{
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

func (app *App) localization(w http.ResponseWriter, r *http.Request) {
	locale := app.localeFromRequest(r)
	app.render(w, r, "content", "templates/localization.gohtml", map[string]any{
		"Title":   "Localization",
		"Locale":  locale,
		"Locales": []string{"en_US", "nl_NL", "fr_FR"},
		"Count":   5,
	})
}

func (app *App) headers(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/headers.gohtml", map[string]any{
		"Title": "HTMX response helpers",
	})
}

func (app *App) headersNotify(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("notice", "templates/notice.gohtml").SetData(map[string]any{
		"Message": "Response headers changed this element at " + time.Now().Format("15:04:05") + ".",
	})
	content.Response().
		Retarget("#notice").
		ReswapWith(connector.NewSwap().Style(connector.SwapOuterHTML).Swap(120 * time.Millisecond).Transition(true)).
		TriggerWith(connector.NewTrigger().AddEventObject("showcase:notice", map[string]any{
			"message": "Headers set at " + time.Now().Format("15:04:05"),
		}))
	app.writeContent(w, r, content)
}

func (app *App) flow(w http.ResponseWriter, r *http.Request) {
	session := app.flowSession(w, r)
	if session.Current == "" {
		session.Current = "account"
	}

	steps := app.flowSteps(session, "")
	flow := partial.NewPageFlow(steps)
	if flow.FindStep(session.Current) == -1 {
		session.Current = steps[0].Name
	}

	errorMessage := ""
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			errorMessage = err.Error()
		} else {
			switch r.FormValue("direction") {
			case "reset":
				*session = partial.FlowSessionData{Current: "account"}
			case "back":
				flow.Prev(session)
			default:
				step := flow.CurrentStep(session)
				data := flowFormData(r)
				if step != nil && step.Validate != nil {
					if err := step.Validate(r, data); err != nil {
						errorMessage = err.Error()
						session.SetStepValidated(step.Name, false)
						break
					}
				}
				if step != nil {
					session.SetStepValidated(step.Name, true)
					session.SetStepData(step.Name, data)
				}
				flow.Next(session)
			}
		}
	}

	steps = app.flowSteps(session, errorMessage)
	flow = partial.NewPageFlow(steps)
	app.renderPartial(w, r, app.flowPartial(flow, session, errorMessage))
}

func (app *App) flowSteps(session *partial.FlowSessionData, errorMessage string) []partial.FlowStep {
	account := partial.NewID("account", "templates/flow_account.gohtml").SetData(map[string]any{
		"Email": session.GetStepData("account")["email"],
		"Error": errorMessage,
	})
	details := partial.NewID("details", "templates/flow_details.gohtml").SetData(map[string]any{
		"Name":  session.GetStepData("details")["name"],
		"Plan":  session.GetStepData("details")["plan"],
		"Error": errorMessage,
	})
	confirm := partial.NewID("confirm", "templates/flow_confirm.gohtml").SetData(map[string]any{
		"AllData": session.GetAllData(),
	})

	return []partial.FlowStep{
		{
			Name:    "account",
			Partial: account,
			Validate: func(r *http.Request, data map[string]any) error {
				email, _ := data["email"].(string)
				if !strings.Contains(email, "@") {
					return errors.New("enter an email address before continuing")
				}
				return nil
			},
		},
		{
			Name:    "details",
			Partial: details,
			Validate: func(r *http.Request, data map[string]any) error {
				name, _ := data["name"].(string)
				if strings.TrimSpace(name) == "" {
					return errors.New("enter a project name before continuing")
				}
				return nil
			},
		},
		{Name: "confirm", Partial: confirm},
	}
}

func (app *App) flowPartial(flow *partial.PageFlow, session *partial.FlowSessionData, errorMessage string) *partial.Partial {
	current := flow.CurrentStep(session)
	currentName := ""
	if current != nil {
		currentName = current.Name
	}
	content := partial.NewID("content", "templates/flow.gohtml").SetData(map[string]any{
		"Title":       "Page flow",
		"Flow":        flow,
		"Steps":       flow.Steps,
		"CurrentStep": currentName,
		"Validated":   session.Validated,
		"Error":       errorMessage,
	})
	for _, step := range flow.Steps {
		content.With(step.Partial)
	}
	return content
}

func flowFormData(r *http.Request) map[string]any {
	data := make(map[string]any)
	for key, values := range r.PostForm {
		if key == "direction" {
			continue
		}
		if len(values) > 0 {
			data[key] = values[0]
		}
	}
	return data
}

func (app *App) sse(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "content", "templates/sse.gohtml", map[string]any{
		"Title": "Server-sent events",
	})
}

func (app *App) sseStream(w http.ResponseWriter, r *http.Request) {
	events := partial.NewSSEWriter(w)
	_ = events.Comment("go-partial showcase stream")
	events.Flush()

	for i := 1; i <= 5; i++ {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(700 * time.Millisecond):
		}

		status := partial.NewID("sse-status", "templates/sse_status.gohtml").
			SetFileSystem(os.DirFS("examples/showcase")).
			SetData(map[string]any{
				"Step": i,
				"Time": time.Now().Format("15:04:05"),
				"Done": i == 5,
			})
		if err := events.PatchPartial(app.requestContext(r), r, "#sse-status", status); err != nil {
			_ = events.Error(err)
			events.Flush()
			return
		}

		if err := events.Signal("progress", map[string]any{
			"step": i,
			"done": i == 5,
		}); err != nil {
			return
		}
		events.Flush()
	}
}

func (app *App) infinite(w http.ResponseWriter, r *http.Request) {
	content := app.infinitePartial("content", 1, 25)
	app.renderPartial(w, r, content)
}

func (app *App) infiniteLoad(w http.ResponseWriter, r *http.Request) {
	action := r.Header.Get(connector.HeaderAction.String())
	if !strings.HasPrefix(action, "current-") {
		http.Error(w, "missing X-Action: current-<row>", http.StatusBadRequest)
		return
	}

	current, err := strconv.Atoi(strings.TrimPrefix(action, "current-"))
	if err != nil || current < 0 {
		http.Error(w, "invalid X-Action cursor", http.StatusBadRequest)
		return
	}

	if current >= 150 {
		current = 125
	}

	start := current + 1
	content := app.infinitePartial("infinite-chunk", start, 25)
	app.writeStandalone(w, r, content)
}

func (app *App) infinitePartial(id string, start int, count int) *partial.Partial {
	end := start + count - 1
	if end > 150 {
		end = 150
	}

	rows := make([]int, 0, max(0, end-start+1))
	for i := start; i <= end; i++ {
		rows = append(rows, i)
	}

	templateName := "templates/infinite_chunk.gohtml"
	if id == "content" {
		templateName = "templates/infinite.gohtml"
	}

	content := partial.NewID(id, templateName).SetData(map[string]any{
		"Title":        "Infinite scroll with X-Action",
		"Rows":         rows,
		"Next":         end,
		"Done":         end >= 150,
		"Start":        start,
		"Current":      start - 1,
		"ActionHeader": connector.HeaderAction.String(),
	})
	content.With(partial.NewID("infinite-row", "templates/infinite_row.gohtml"))
	content.With(partial.NewID("rickroll", "templates/rickroll.gohtml"))
	content.With(partial.NewID("infinite-toast", "templates/infinite_toast.gohtml"))
	return content
}

func (app *App) errorPage(w http.ResponseWriter, r *http.Request) {
	content := partial.NewID("content", "templates/error.gohtml").SetData(map[string]any{
		"Title": "Template error boundary",
	})
	content.With(partial.NewID("broken-section", "templates/broken.gohtml"))
	app.renderPartial(w, r, content)
}

func (app *App) render(w http.ResponseWriter, r *http.Request, id string, tmpl string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}
	app.renderPartial(w, r, partial.NewID(id, tmpl).SetData(data))
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
	wrapper.WithOOB(partial.NewID("header", "templates/header.gohtml").SetData(map[string]any{
		"Nav": app.navItems(),
	}).SetAlwaysSwapOOB(true))
	return wrapper
}

func (app *App) requestContext(r *http.Request) context.Context {
	ctx := context.WithValue(r.Context(), partial.LocalizerContextKey, showcaseLocalizer{locale: app.localeFromRequest(r)})
	return context.WithValue(ctx, partial.CsrfContextKey, showcaseCsrf{
		key:   partial.DefaultCsrfToken,
		token: randomID(),
	})
}

func (app *App) flowSession(w http.ResponseWriter, r *http.Request) *partial.FlowSessionData {
	const cookieName = "go_partial_showcase_flow"
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		cookie = &http.Cookie{
			Name:     cookieName,
			Value:    randomID(),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, cookie)
	}
	session, ok := app.flowSessions[cookie.Value]
	if !ok {
		session = &partial.FlowSessionData{}
		app.flowSessions[cookie.Value] = session
	}
	return session
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

func (app *App) localeFromRequest(r *http.Request) string {
	switch r.URL.Query().Get("locale") {
	case "nl_NL":
		return "nl_NL"
	case "fr_FR":
		return "fr_FR"
	case "en_US":
		return "en_US"
	}

	acceptLanguage := strings.ToLower(r.Header.Get("Accept-Language"))
	if strings.HasPrefix(acceptLanguage, "nl") {
		return "nl_NL"
	}
	if strings.HasPrefix(acceptLanguage, "fr") {
		return "fr_FR"
	}
	return "en_US"
}

func showcaseTranslationFunctions() template.FuncMap {
	return showcaseTranslator{messages: map[string]map[string]string{
		"en_US": {
			"title":       "Localization",
			"intro":       "The localizer is stored in the request context and exposed to every template as .Loc.",
			"checkout":    "Checkout",
			"oneMessage":  "You have one message.",
			"manyMessage": "You have %d messages.",
			"total":       "Total",
			"delivery":    "Delivery",
			"status":      "Ready for pickup",
			"active":      "Active locale",
			"button.save": "Save changes",
			"explanation": "Switch languages with HTMX and the content re-renders from the server without replacing the page shell.",
		},
		"nl_NL": {
			"title":       "Lokalisatie",
			"intro":       "De localizer staat in de request-context en is in elke template beschikbaar als .Loc.",
			"checkout":    "Afrekenen",
			"oneMessage":  "Je hebt een bericht.",
			"manyMessage": "Je hebt %d berichten.",
			"total":       "Totaal",
			"delivery":    "Bezorging",
			"status":      "Klaar om op te halen",
			"active":      "Actieve taal",
			"button.save": "Wijzigingen opslaan",
			"explanation": "Wissel van taal met HTMX en de server rendert de inhoud opnieuw zonder de pagina-shell te vervangen.",
		},
		"fr_FR": {
			"title":       "Localisation",
			"intro":       "Le localizer vit dans le contexte de la requete et chaque template le lit avec .Loc.",
			"checkout":    "Paiement",
			"oneMessage":  "Vous avez un message.",
			"manyMessage": "Vous avez %d messages.",
			"total":       "Total",
			"delivery":    "Livraison",
			"status":      "Pret pour le retrait",
			"active":      "Langue active",
			"button.save": "Enregistrer",
			"explanation": "Changez de langue avec HTMX et le serveur rend le contenu sans remplacer la structure de la page.",
		},
	}}.FuncMap()
}

func (app *App) navItems() []NavItem {
	return []NavItem{
		{Path: "/", Label: "Home", Group: "Start"},
		{Path: "/scoped", Label: "Scoped rows", Group: "Core rendering"},
		{Path: "/context", Label: "Context", Group: "Core rendering"},
		{Path: "/localization", Label: "Localization", Group: "Core rendering"},
		{Path: "/selection", Label: "Selection", Group: "Interactions"},
		{Path: "/tabs", Label: "Tabs", Group: "Interactions"},
		{Path: "/action", Label: "Actions", Group: "Interactions"},
		{Path: "/flow", Label: "Flow", Group: "Interactions"},
		{Path: "/infinite", Label: "Infinite scroll", Group: "Interactions"},
		{Path: "/oob", Label: "OOB", Group: "Integrations"},
		{Path: "/headers", Label: "Headers", Group: "Integrations"},
		{Path: "/sse", Label: "SSE", Group: "Integrations"},
		{Path: "/debug", Label: "Debug", Group: "Diagnostics"},
		{Path: "/error", Label: "Error", Group: "Diagnostics"},
	}
}
