package connector

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestInteractionAttrsOfEachConnector checks the attributes each connector
// writes for the kinds of interaction it handles itself.
func TestInteractionAttrsOfEachConnector(t *testing.T) {
	tests := []struct {
		name        string
		connector   Connector
		interaction Interaction
		want        map[string]string
	}{
		{
			name:        "partial with target, interval and trigger",
			connector:   NewPartial(nil),
			interaction: Interaction{Kind: InteractionPoll, URL: "/feed", Target: "#feed", Interval: "2s", Trigger: "load", Select: "#items"},
			want: map[string]string{
				"data-partial-interaction": "poll",
				"data-url":                 "/feed",
				"data-target":              "#feed",
				"data-interval":            "2s",
				"data-trigger":             "load",
				"data-select":              "#items",
			},
		},
		{
			name:        "partial with only a URL",
			connector:   NewPartial(nil),
			interaction: Interaction{Kind: InteractionAsync, URL: "/later", Attrs: map[string]string{"": "skipped", "class": "lazy"}},
			want: map[string]string{
				"data-partial-interaction": "async",
				"data-url":                 "/later",
				"class":                    "lazy",
			},
		},
		{
			name:        "htmx submit with a method",
			connector:   NewHTMX(nil),
			interaction: Interaction{Kind: InteractionSubmit, ID: "form", URL: "/save", Trigger: "submit", Options: map[string]string{OptionMethod: " PUT "}},
			want: map[string]string{
				"hx-put":     "/save",
				"hx-target":  "#form",
				"hx-swap":    "innerHTML",
				"hx-trigger": "submit",
			},
		},
		{
			name:        "htmx submit with an unknown method",
			connector:   NewHTMX(nil),
			interaction: Interaction{Kind: InteractionSubmit, ID: "form", URL: "/save", Target: "#out", Options: map[string]string{OptionMethod: "teleport"}},
			want: map[string]string{
				"hx-post":   "/save",
				"hx-target": "#out",
				"hx-swap":   "innerHTML",
			},
		},
		{
			name:        "htmx live with the default delay",
			connector:   NewHTMX(nil),
			interaction: Interaction{Kind: InteractionLive, ID: "results", URL: "/search", Swap: "outerHTML"},
			want: map[string]string{
				"hx-get":     "/search",
				"hx-trigger": "input changed delay:300ms, search",
				"hx-target":  "#results",
				"hx-swap":    "outerHTML",
			},
		},
		{
			name:        "htmx live with its own delay and trigger",
			connector:   NewHTMX(nil),
			interaction: Interaction{Kind: InteractionLive, ID: "results", URL: "/search", Trigger: "keyup", Options: map[string]string{OptionDelay: "1s"}},
			want: map[string]string{
				"hx-get":     "/search",
				"hx-trigger": "keyup",
				"hx-target":  "#results",
				"hx-swap":    "innerHTML",
			},
		},
		{
			name:        "turbo prefetch",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionPrefetch, URL: "/next"},
			want:        map[string]string{"rel": "prefetch", "href": "/next"},
		},
		{
			name:        "turbo refresh",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionRefresh, URL: "/stats"},
			want:        map[string]string{"data-turbo-refresh-url": "/stats"},
		},
		{
			name:        "turbo submit into a frame",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionSubmit, URL: "/save", Target: "#editor"},
			want:        map[string]string{"data-turbo-frame": "editor"},
		},
		{
			name:        "turbo submit to itself",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionSubmit, URL: "/save", Target: TargetSelf},
			want:        map[string]string{},
		},
		{
			name:        "turbo live",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionLive, URL: "/search", Target: "#results", Options: map[string]string{OptionDelay: "500ms"}},
			want: map[string]string{
				"data-partial-interaction": "live",
				"data-url":                 "/search",
				"data-delay":               "500ms",
				"data-target":              "#results",
			},
		},
		{
			name:        "turbo live without a target",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionLive, URL: "/search"},
			want: map[string]string{
				"data-partial-interaction": "live",
				"data-url":                 "/search",
				"data-delay":               "300ms",
			},
		},
		{
			name:        "turbo on",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionOn, URL: "/cart", Trigger: "cart:changed", Target: "#cart"},
			want: map[string]string{
				"data-partial-interaction": "on",
				"data-trigger":             "cart:changed",
				"data-url":                 "/cart",
				"data-target":              "#cart",
			},
		},
		{
			name:        "turbo on without a target",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionOn, URL: "/cart", Trigger: "cart:changed"},
			want: map[string]string{
				"data-partial-interaction": "on",
				"data-trigger":             "cart:changed",
				"data-url":                 "/cart",
			},
		},
		{
			name:        "turbo poll is a frame",
			connector:   NewTurbo(nil),
			interaction: Interaction{Kind: InteractionPoll, ID: "feed", URL: "/feed"},
			want:        map[string]string{"id": "feed", "src": "/feed"},
		},
		{
			name:        "unpoly prefetch",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionPrefetch, ID: "next", URL: "/next"},
			want:        map[string]string{"rel": "prefetch", "href": "/next"},
		},
		{
			name:        "unpoly submit",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionSubmit, ID: "form", URL: "/save"},
			want:        map[string]string{"up-submit": "", "up-target": "#form"},
		},
		{
			name:        "unpoly live with a duration",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionLive, ID: "results", URL: "/search", Options: map[string]string{OptionDelay: "1.5s"}},
			want:        map[string]string{"up-autosubmit": "", "up-watch-delay": "1500", "up-target": "#results"},
		},
		{
			name:        "unpoly live with a delay that is not a duration",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionLive, ID: "results", URL: "/search", Options: map[string]string{OptionDelay: "slow"}},
			want:        map[string]string{"up-autosubmit": "", "up-watch-delay": "slow", "up-target": "#results"},
		},
		{
			name:        "unpoly reveal",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionReveal, ID: "more", URL: "/more"},
			want:        map[string]string{"up-get": "/more", "up-target": "#more", "up-trigger": "reveal"},
		},
		{
			name:        "unpoly poll",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionPoll, ID: "feed", URL: "/feed", Interval: "10s"},
			want:        map[string]string{"up-get": "/feed", "up-target": "#feed", "up-trigger": "every 10s"},
		},
		{
			name:        "unpoly with a trigger",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionRefresh, ID: "stats", URL: "/stats", Trigger: "click", Target: "#panel"},
			want:        map[string]string{"up-get": "/stats", "up-target": "#panel", "up-trigger": "click"},
		},
		{
			name:        "unpoly loads by default",
			connector:   NewUnpoly(nil),
			interaction: Interaction{Kind: InteractionAsync, ID: "later", URL: "/later"},
			want:        map[string]string{"up-get": "/later", "up-target": "#later", "up-trigger": "load"},
		},
		{
			name:        "htmx 4 stream with a target",
			connector:   NewHTMX4(nil),
			interaction: Interaction{Kind: InteractionStream, ID: "feed", URL: "/events", Target: "#log", Swap: "beforeend"},
			want:        map[string]string{"hx-sse:connect": "/events", "hx-target": "#log", "hx-swap": "beforeend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.connector.InteractionAttrs(tt.interaction); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("InteractionAttrs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConnectorHeaderNames(t *testing.T) {
	tests := []struct {
		connector Connector
		target    string
		selected  string
		action    string
	}{
		{NewPartial(nil), "X-Target", "X-Select", "X-Action"},
		{NewHTMX(nil), "HX-Target", "X-Select", "X-Action"},
		{NewTurbo(nil), "Turbo-Frame", "Turbo-Select", "Turbo-Action"},
		{NewUnpoly(nil), "X-Up-Target", "X-Up-Select", "X-Up-Action"},
	}

	for _, tt := range tests {
		if got := tt.connector.GetTargetHeader(); got != tt.target {
			t.Errorf("%T target header = %q, want %q", tt.connector, got, tt.target)
		}

		if got := tt.connector.GetSelectHeader(); got != tt.selected {
			t.Errorf("%T select header = %q, want %q", tt.connector, got, tt.selected)
		}

		if got := tt.connector.GetActionHeader(); got != tt.action {
			t.Errorf("%T action header = %q, want %q", tt.connector, got, tt.action)
		}
	}
}

// TestConnectorReadsTheRequest checks that the select and action values come
// from their headers first, then from the URL query when the connector is
// configured to read it, and not from the query otherwise.
func TestConnectorReadsTheRequest(t *testing.T) {
	queried := NewPartial(&Config{UseURLQuery: true})
	plain := NewPartial(nil)

	fromHeaders := httptest.NewRequest(http.MethodGet, "/?select=q-select&action=q-action", nil)
	fromHeaders.Header.Set("X-Select", "h-select")
	fromHeaders.Header.Set("X-Action", "h-action")

	if got := queried.GetSelectValue(fromHeaders); got != "h-select" {
		t.Errorf("select from header = %q", got)
	}

	if got := queried.GetActionValue(fromHeaders); got != "h-action" {
		t.Errorf("action from header = %q", got)
	}

	fromQuery := httptest.NewRequest(http.MethodGet, "/?target=q-target&select=q-select&action=q-action", nil)

	if got := queried.GetTargetValue(fromQuery); got != "q-target" {
		t.Errorf("target from query = %q", got)
	}

	if got := queried.GetSelectValue(fromQuery); got != "q-select" {
		t.Errorf("select from query = %q", got)
	}

	if got := queried.GetActionValue(fromQuery); got != "q-action" {
		t.Errorf("action from query = %q", got)
	}

	if got := plain.GetSelectValue(fromQuery); got != "" {
		t.Errorf("select without the query option = %q", got)
	}

	if got := plain.GetActionValue(fromQuery); got != "" {
		t.Errorf("action without the query option = %q", got)
	}
}
