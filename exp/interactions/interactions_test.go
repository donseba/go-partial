package interactions

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

func TestInteractionContractNameFromEndpoint(t *testing.T) {
	tests := map[string]string{
		"/stats":                  "Stats",
		"/cart-summary":           "CartSummary",
		"/interactions/async":     "Async",
		"/users/:id/profile_card": "ProfileCard",
		"/":                       "",
	}
	for endpoint, want := range tests {
		if got := nameFromEndpoint(endpoint); got != want {
			t.Fatalf("nameFromEndpoint(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

func TestRendererCustomizesInteractionMarkup(t *testing.T) {
	fsys := fstest.MapFS{
		"async.gohtml": &fstest.MapFile{Data: []byte(`{{ async runtime "/stats" }}`)},
	}

	p := partial.NewID("async", "async.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		Use(Stage(func(runtime *partial.Runtime, interaction connector.Interaction, attrs map[string]string) (template.HTML, error) {
			return template.HTML(`<section data-kind="` + string(interaction.Kind) + `" ` + renderAttrs(attrs) + `></section>`), nil
		}))

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	if !strings.Contains(body, `<section data-kind="async"`) {
		t.Fatalf("expected custom interaction wrapper, got %q", body)
	}
	if !strings.Contains(body, `hx-get="/stats"`) {
		t.Fatalf("expected connector attributes, got %q", body)
	}
}

func TestInteractionContractNameUsesExplicitName(t *testing.T) {
	interaction := NewAsync("/interactions/async").As("Stats")
	if got := interaction.ContractName(); got != "Stats" {
		t.Fatalf("ContractName() = %q, want Stats", got)
	}
}

func TestInteractionResolvesParams(t *testing.T) {
	interaction := NewAsync("/rows/:row").Param("row", 42).Interaction()
	if interaction.URL != "/rows/42" {
		t.Fatalf("URL = %q, want /rows/42", interaction.URL)
	}
	if interaction.ID != "async-rows-42" {
		t.Fatalf("ID = %q, want async-rows-42", interaction.ID)
	}
}

func TestAsyncRendersHTMXDeferredMarkup(t *testing.T) {
	fsys := fstest.MapFS{
		"async.gohtml": &fstest.MapFile{Data: []byte(`{{ async runtime "/table/:row" "row" .ID }}`)},
	}

	p := partial.NewID("async", "async.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		SetDot(map[string]any{"ID": 7})

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	for _, expected := range []string{
		`id="async-table-7"`,
		`hx-get="/table/7"`,
		`hx-trigger="load"`,
		`hx-target="#async-table-7"`,
		`hx-swap="innerHTML"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in %q", expected, body)
		}
	}
}

func TestAsyncAcceptsInteractionConfig(t *testing.T) {
	type pageData struct {
		Stats Interaction
	}
	page := pageData{
		Stats: NewAsync("/stats").
			ID("stats-loader").
			Target("#stats").
			Swap(SwapOuterHTML).
			Placeholder(""),
	}

	fsys := fstest.MapFS{
		"async.gohtml": &fstest.MapFile{Data: []byte(`{{ async runtime .Stats }}`)},
	}

	p := partial.NewID("async", "async.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		SetDot(page)

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	for _, expected := range []string{
		`id="stats-loader"`,
		`hx-get="/stats"`,
		`hx-target="#stats"`,
		`hx-swap="outerHTML"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in %q", expected, body)
		}
	}
	if strings.Contains(body, "Loading...") {
		t.Fatalf("expected empty placeholder, got %q", body)
	}
}

func TestPollRendersHTMXIntervalMarkup(t *testing.T) {
	fsys := fstest.MapFS{
		"poll.gohtml": &fstest.MapFile{Data: []byte(`{{ poll runtime "/notifications" "every" "10s" }}`)},
	}

	p := partial.NewID("poll", "poll.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap())

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	for _, expected := range []string{
		`id="poll-notifications"`,
		`hx-get="/notifications"`,
		`hx-swap="innerHTML"`,
		`hx-trigger="every 10s"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in %q", expected, body)
		}
	}
}

func TestOnAcceptsInteractionConfig(t *testing.T) {
	type pageData struct {
		CartChanged Interaction
	}
	page := pageData{
		CartChanged: NewOn("cart:changed", "/cart/summary").
			ID("cart-listener").
			Target("#cart").
			From("window"),
	}

	fsys := fstest.MapFS{
		"on.gohtml": &fstest.MapFile{Data: []byte(`{{ on runtime .CartChanged }}`)},
	}

	p := partial.NewID("on", "on.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap()).
		SetDot(page)

	out, err := partial.Render(context.Background(), p)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	body := string(out)
	for _, expected := range []string{
		`id="cart-listener"`,
		`hx-get="/cart/summary"`,
		`hx-target="#cart"`,
		`hx-trigger="cart:changed from:window"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected %q in %q", expected, body)
		}
	}
}

func TestInteractionHelpersRenderConcurrently(t *testing.T) {
	fsys := fstest.MapFS{
		"async.gohtml": &fstest.MapFile{Data: []byte(`{{ async runtime "/table/:row" "row" (url).RawQuery }}`)},
	}
	p := partial.NewID("async", "async.gohtml").
		SetConnector(connector.NewHTMX(nil)).
		SetFileSystem(fsys).
		SetFunc(FuncMap())

	const renders = 64
	var wg sync.WaitGroup
	errs := make(chan string, renders)
	for i := range renders {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value := strconv.Itoa(i)
			req := httptest.NewRequest(http.MethodGet, "/?row="+value, nil)
			out, err := partial.RenderWithRequest(req.Context(), req, p)
			if err != nil {
				errs <- err.Error()
				return
			}
			body := string(out)
			want := `/table/row=` + value
			if !strings.Contains(body, want) {
				errs <- "interaction " + value + " missing " + want + " in " + body
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestAttrsVariantsDecorateExistingElements(t *testing.T) {
	tests := []struct {
		name      string
		connector connector.Connector
		template  string
		want      string
	}{
		{
			name:      "poll targets its own element",
			connector: connector.NewHTMX4(nil),
			template:  `<ul {{ pollAttrs runtime "/status" "3s" }}></ul>`,
			want:      `<ul  hx-get="/status" hx-swap="innerHTML" hx-target="this" hx-trigger="every 3s"></ul>`,
		},
		{
			name:      "async with a target",
			connector: connector.NewHTMX(nil),
			template:  `<section {{ asyncAttrs runtime "/stats" "target" "#stats" "swap" "outerHTML" }}></section>`,
			want:      `<section  hx-get="/stats" hx-swap="outerHTML" hx-target="#stats" hx-trigger="load"></section>`,
		},
		{
			name:      "reveal conditionally",
			connector: connector.NewHTMX4(nil),
			template:  `<tr{{ if true }}{{ revealAttrs runtime "/rows" "page" 2 "swap" "afterend" }}{{ end }}></tr>`,
			want:      `<tr hx-get="/rows" hx-swap="afterend" hx-target="this" hx-trigger="revealed"></tr>`,
		},
		{
			name:      "on an event",
			connector: connector.NewHTMX4(nil),
			template:  `<div {{ onAttrs runtime "cart:changed" "/cart" }}></div>`,
			want:      `<div  hx-get="/cart" hx-swap="innerHTML" hx-target="this" hx-trigger="cart:changed from:body"></div>`,
		},
		{
			name:      "an interaction value",
			connector: connector.NewHTMX4(nil),
			template:  `<div {{ pollAttrs runtime Status }}></div>`,
			want:      `<div  hx-get="/status" hx-swap="innerHTML" hx-target="this" hx-trigger="every 5s"></div>`,
		},
		{
			name:      "select and attributes",
			connector: connector.NewHTMX4(nil),
			template:  `<div {{ pollAttrs runtime "/inbox" "every" "10s" "select" "#messages" "hx-indicator" "#spinner" "data-section" "inbox" }}></div>`,
			want:      `<div  data-section="inbox" hx-get="/inbox" hx-indicator="#spinner" hx-select="#messages" hx-swap="innerHTML" hx-target="this" hx-trigger="every 10s"></div>`,
		},
		{
			name:      "a base connector",
			connector: connector.NewPartial(nil),
			template:  `<div {{ streamAttrs runtime "/events" }}></div>`,
			want:      `<div  data-partial-interaction="stream" data-target="this" data-url="/events"></div>`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"page.gohtml": &fstest.MapFile{
					Data: []byte(
						`{{/* @interaction Status github.com/donseba/go-partial/exp/interactions.Interaction */}}` + test.template,
					),
				},
			}
			p := partial.NewID("page", "page.gohtml").
				SetConnector(test.connector).
				SetFileSystem(fsys).
				SetFunc(FuncMap()).
				SetContract("interaction", NewPoll("/status").As("Status"))

			out, err := partial.Render(context.Background(), p)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			if string(out) != test.want {
				t.Fatalf("Render() =\n%s\nwant\n%s", out, test.want)
			}
		})
	}
}

func TestAttrsReportInvalidInteractions(t *testing.T) {
	out := AsyncAttrs(nil, 42)
	if !strings.Contains(string(out), `data-partial-error="interaction helper expects`) {
		t.Fatalf("AsyncAttrs() = %q", out)
	}
}
