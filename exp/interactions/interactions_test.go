package interactions

import (
	"context"
	"strings"
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

	out, err := p.Render(context.Background())
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

	out, err := p.Render(context.Background())
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

	out, err := p.Render(context.Background())
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

	out, err := p.Render(context.Background())
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
