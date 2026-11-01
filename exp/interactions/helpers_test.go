package interactions

import (
	"context"
	"errors"
	"html/template"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// renderPage renders src as the template of a partial with the interaction
// helpers, conn and dot.
func renderPage(t *testing.T, conn connector.Connector, src string, dot any) string {
	t.Helper()

	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(src)},
	}

	p := partial.NewID("page", "page.gohtml").
		SetConnector(conn).
		SetFileSystem(fsys).
		SetFunc(FuncMap())

	if dot != nil {
		p.SetDot(dot)
	}

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	return string(out)
}

// TestElementHelpersRenderTheirMarkup checks the element each helper that
// renders one writes, by connector.
func TestElementHelpersRenderTheirMarkup(t *testing.T) {
	tests := []struct {
		name      string
		connector connector.Connector
		template  string
		dot       any
		want      string
	}{
		{
			name:      "reveal",
			connector: connector.NewHTMX(nil),
			template:  `{{ reveal runtime "/more" }}`,
			want:      `<div id="reveal-more" hx-get="/more" hx-swap="innerHTML" hx-target="#reveal-more" hx-trigger="revealed">Loading...</div>`,
		},
		{
			name:      "stream",
			connector: connector.NewHTMX(nil),
			template:  `{{ stream runtime "/events" }}`,
			want:      `<div id="stream-events" hx-ext="sse" sse-connect="/events" sse-swap="message">Loading...</div>`,
		},
		{
			name:      "prefetch",
			connector: connector.NewHTMX(nil),
			template:  `{{ prefetch runtime "/next" }}`,
			want:      `<link href="/next" rel="prefetch">`,
		},
		{
			name:      "refresh",
			connector: connector.NewHTMX(nil),
			template:  `{{ refresh runtime "/stats" "placeholder" "Reload" }}`,
			want:      `<button type="button" id="refresh-stats" hx-get="/stats" hx-swap="innerHTML" hx-target="#refresh-stats" hx-trigger="click">Reload</button>`,
		},
		{
			name:      "turbo lazy frame",
			connector: connector.NewTurbo(nil),
			template:  `{{ async runtime "/later" "placeholder" "" }}`,
			want:      `<turbo-frame id="async-later" loading="lazy" src="/later"></turbo-frame>`,
		},
		{
			name:      "on an event",
			connector: connector.NewHTMX(nil),
			template:  `{{ on runtime "saved" "/list" "from" "window" }}`,
			want:      `<div id="on-saved" hx-get="/list" hx-swap="innerHTML" hx-target="#on-saved" hx-trigger="saved from:window"></div>`,
		},
		{
			name:      "a connector interaction",
			connector: connector.NewHTMX(nil),
			template:  `{{ async runtime . }}`,
			dot:       connector.Interaction{Kind: connector.InteractionAsync, URL: "/raw"},
			want:      `<div id="async-raw" hx-get="/raw" hx-swap="innerHTML" hx-target="#async-raw" hx-trigger="load">Loading...</div>`,
		},
		{
			name:      "attributes of a connector interaction",
			connector: connector.NewHTMX(nil),
			template:  `<p{{ asyncAttrs runtime . }}></p>`,
			dot:       connector.Interaction{Kind: connector.InteractionAsync, URL: "/raw"},
			want:      `<p hx-get="/raw" hx-swap="innerHTML" hx-target="this" hx-trigger="load"></p>`,
		},
		{
			name:      "attributes of a targeted connector interaction",
			connector: connector.NewHTMX(nil),
			template:  `<p{{ asyncAttrs runtime . }}></p>`,
			dot:       connector.Interaction{Kind: connector.InteractionAsync, URL: "/raw", Target: "#box"},
			want:      `<p hx-get="/raw" hx-swap="innerHTML" hx-target="#box" hx-trigger="load"></p>`,
		},
		{
			name:      "a relative endpoint",
			connector: connector.NewHTMX(nil),
			template:  `{{ async runtime "stats" }}`,
			want:      `<div id="async-stats" hx-get="/stats" hx-swap="innerHTML" hx-target="#async-stats" hx-trigger="load">Loading...</div>`,
		},
		{
			name:      "an empty endpoint",
			connector: connector.NewHTMX(nil),
			template:  `{{ async runtime "" }}`,
			want:      `<div id="async-content" hx-get="/" hx-swap="innerHTML" hx-target="#async-content" hx-trigger="load">Loading...</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderPage(t, tt.connector, tt.template, tt.dot); got != tt.want {
				t.Fatalf("Render() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

// TestHelpersReportInvalidArguments checks the error each helper writes for
// arguments it cannot use, escaped in place of the markup.
func TestHelpersReportInvalidArguments(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			name:     "odd pairs",
			template: `{{ async runtime "/stats" "target" }}`,
			want:     "interaction helpers expect key/value pairs",
		},
		{
			name:     "key that is not a string",
			template: `{{ async runtime "/stats" 1 2 }}`,
			want:     "interaction option key must be string, got int",
		},
		{
			name:     "on without an endpoint",
			template: `{{ on runtime "saved" }}`,
			want:     "on expects an endpoint when the first argument is an event",
		},
		{
			name:     "on with an endpoint that is not a string",
			template: `{{ on runtime "saved" 4 }}`,
			want:     "on endpoint must be string, got int",
		},
		{
			name:     "on with odd pairs",
			template: `{{ on runtime "saved" "/list" "from" }}`,
			want:     "interaction helpers expect key/value pairs",
		},
		{
			name:     "refresh attributes with odd pairs",
			template: `<a{{ refreshAttrs runtime "/stats" "swap" }}></a>`,
			want:     `data-partial-error="interaction helpers expect key/value pairs"`,
		},
		{
			name:     "on attributes without an endpoint",
			template: `<a{{ onAttrs runtime "saved" }}></a>`,
			want:     `data-partial-error="on expects an endpoint when the first argument is an event"`,
		},
		{
			name:     "client attributes with odd pairs",
			template: `<a{{ clientAttrs runtime "confirm" }}></a>`,
			want:     `data-partial-error="interaction helpers expect key/value pairs"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderPage(t, connector.NewHTMX(nil), tt.template, nil)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("Render() = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestHelpersWithoutARuntime(t *testing.T) {
	const want = "go-partial interaction runtime is not configured"

	if got := Async(nil, "/stats"); string(got) != want {
		t.Fatalf("Async(nil) = %q", got)
	}

	if got := On(nil, "saved", "/list"); string(got) != want {
		t.Fatalf("On(nil) = %q", got)
	}

	for name, attrs := range map[string]template.HTMLAttr{
		"AsyncAttrs": AsyncAttrs(nil, "/stats"),
		"NoBoost":    NoBoost(nil),
	} {
		if !strings.Contains(string(attrs), `data-partial-error="`+want+`"`) {
			t.Errorf("%s(nil) = %q", name, attrs)
		}
	}

	if got := Async(nil, nil); !strings.HasSuffix(string(got), "got &lt;nil&gt;") {
		t.Fatalf("Async(nil, nil) = %q, want the type error", got)
	}

	var config *Interaction
	if got := Async(nil, config); string(got) != "interaction is nil" {
		t.Fatalf("Async(nil, (*Interaction)(nil)) = %q", got)
	}
}

// TestBuildersOfZeroValues checks that the builder methods work on a zero
// Interaction, whose maps are nil, and on the constructors' values.
func TestBuildersOfZeroValues(t *testing.T) {
	interaction := Interaction{}.
		From("window").
		Option("delay", 250*time.Millisecond).
		Param("id", 7).
		Placeholder("Wait").
		Every(2 * time.Second).
		Interaction()

	if interaction.Options["from"] != "window" || interaction.Options["delay"] != "250ms" {
		t.Fatalf("Options = %v", interaction.Options)
	}

	if interaction.Params["id"] != "7" {
		t.Fatalf("Params = %v", interaction.Params)
	}

	if interaction.Interval != "2s" || interaction.Placeholder != "Wait" {
		t.Fatalf("Interval = %q, Placeholder = %q", interaction.Interval, interaction.Placeholder)
	}

	if got := (Interaction{}).Interaction(); got.Params == nil || got.Options == nil || got.URL != "/" {
		t.Fatalf("zero Interaction() = %+v", got)
	}

	if got := NewPoll("/feed").EveryString("every 3s").Interaction().Interval; got != "every 3s" {
		t.Fatalf("EveryString() interval = %q", got)
	}

	kinds := map[connector.InteractionKind]Interaction{
		connector.InteractionReveal:   NewReveal("/more"),
		connector.InteractionStream:   NewStream("/events"),
		connector.InteractionPrefetch: NewPrefetch("/next"),
		connector.InteractionRefresh:  NewRefresh("/stats"),
	}
	for kind, built := range kinds {
		if got := built.Interaction().Kind; got != kind {
			t.Errorf("kind = %q, want %q", got, kind)
		}
	}

	if got := NewRefresh("/admin/stats?x=1").ContractName(); got != "Stats" {
		t.Fatalf("ContractName() = %q, want Stats", got)
	}
}

// TestDurationsParseInHTMX checks that polling intervals and duration
// options are written as htmx parses them, not as Go's "1m0s".
func TestDurationsParseInHTMX(t *testing.T) {
	interaction := NewPoll("/feed").
		Every(time.Minute).
		Option("delay", 90*time.Second).
		Interaction()

	if interaction.Interval != "60s" || interaction.Options["delay"] != "90s" {
		t.Fatalf("Interval = %q, delay = %q", interaction.Interval, interaction.Options["delay"])
	}

	if got := connector.NewHTMX4(nil).InteractionAttrs(interaction)["hx-trigger"]; got != "every 60s" {
		t.Fatalf("hx-trigger = %q, want every 60s", got)
	}

	got, err := values("every", 500*time.Microsecond)
	if err != nil {
		t.Fatal(err)
	}

	if got["every"] != "0.5ms" {
		t.Fatalf("template value = %q, want 0.5ms", got["every"])
	}
}

// TestStageRejectsOtherData checks that the interaction stage reports data
// of another type than Data, and leaves other render kinds alone.
func TestStageRejectsOtherData(t *testing.T) {
	inspect := func(runtime *partial.Runtime) template.HTML {
		out, err := runtime.RenderWith(RenderKindInteraction, "async", "not data", func(*partial.RenderContext) (template.HTML, error) {
			return "terminal", nil
		})
		if err != nil {
			return template.HTML(template.HTMLEscapeString(err.Error()))
		}

		return out
	}

	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`<p>{{ inspect runtime }}</p>`)},
	}

	p := partial.NewID("page", "page.gohtml").
		SetFileSystem(fsys).
		SetFunc(template.FuncMap{"inspect": inspect}).
		Use(Stage(nil))

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if want := "<p>interaction renderer expected interactions.Data, got string</p>"; string(out) != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}
}

func TestNameAndIDHelpers(t *testing.T) {
	names := map[string]string{
		"":              "",
		"/":             "",
		"/rows?page=2":  "Rows",
		"/rows#top":     "Rows",
		"/a/b!c":        "BC",
		"/--":           "",
		"/users/émile":  "Émile",
		"  /trimmed/  ": "Trimmed",
	}
	for endpoint, want := range names {
		if got := nameFromEndpoint(endpoint); got != want {
			t.Errorf("nameFromEndpoint(%q) = %q, want %q", endpoint, got, want)
		}
	}

	ids := map[string]string{
		"":         "content",
		"   ":      "content",
		"---":      "content",
		"/Rows/42": "rows-42",
	}
	for value, want := range ids {
		if got := sanitizeID(value); got != want {
			t.Errorf("sanitizeID(%q) = %q, want %q", value, got, want)
		}
	}

	if got := renderAttrs(map[string]string{"id": "x", "__internal": "y"}); got != "" {
		t.Fatalf("renderAttrs() of skipped keys = %q", got)
	}

	if !isNil(nil) || !isNil((*Interaction)(nil)) || isNil(Interaction{}) || isNil(3) {
		t.Fatal("isNil() is wrong")
	}
}

// TestMarkupRendererErrorsAreWrittenEscaped checks that an error of a custom
// markup renderer takes the place of the interaction, escaped.
func TestMarkupRendererErrorsAreWrittenEscaped(t *testing.T) {
	fsys := fstest.MapFS{
		"page.gohtml": &fstest.MapFile{Data: []byte(`<p>{{ async runtime "/stats" }}</p>`)},
	}

	p := partial.NewID("page", "page.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Stage(func(*partial.Runtime, connector.Interaction, map[string]string) (template.HTML, error) {
			return "", errors.New("no <markup>")
		}))

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if want := "<p>no &lt;markup&gt;</p>"; string(out) != want {
		t.Fatalf("Render() = %q, want %q", out, want)
	}
}
