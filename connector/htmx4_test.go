package connector

import (
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestHTMX4RequestProtocol(t *testing.T) {
	conn := NewHTMX4(nil)
	for _, tc := range []struct {
		name, requestType, target, history string
		partial                            bool
		id                                 string
	}{
		{"fragment", "partial", "main#content", "", true, "content"},
		{"encoded ID", "partial", "div#price%20EUR", "", true, "price EUR"},
		{"full body", "full", "body", "", false, "body"},
		{"select from document", "full", "main#content", "", false, "content"},
		{"history restore", "partial", "main#content", "true", false, "content"},
		{"unknown request type", "future", "main#content", "", false, "content"},
		{"legacy request", "", "content", "", true, "content"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("HX-Request", "true")
			r.Header.Set("HX-Request-Type", tc.requestType)
			r.Header.Set("HX-Target", tc.target)
			r.Header.Set("HX-History-Restore-Request", tc.history)

			if got := conn.RenderPartial(r); got != tc.partial {
				t.Fatalf("RenderPartial = %v, want %v", got, tc.partial)
			}

			if got := conn.GetTargetValue(r); got != tc.id {
				t.Fatalf("target = %q, want %q", got, tc.id)
			}
		})
	}

	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("HX-Request-Type", "partial")
	if conn.RenderPartial(r) || conn.RenderPartial(nil) || conn.GetTargetValue(nil) != "" {
		t.Fatal("ordinary and nil requests must not become fragments")
	}

	query := NewHTMX4(&Config{UseURLQuery: true})
	if got := query.GetTargetValue(httptest.NewRequest("GET", "/?target=content", nil)); got != "content" {
		t.Fatalf("query fallback = %q", got)
	}
}

func TestHTMX4ResponseAndInteractions(t *testing.T) {
	conn := NewHTMX4(nil)
	response := Response{Location: `{"path":"/next","target":"#content"}`, Trigger: "saved", TriggerAfterSwap: "old-swap", TriggerAfterSettle: "old-settle"}
	headers := conn.ResponseHeaders(response)
	if headers["HX-Location"] != response.Location || headers["HX-Trigger"] != "saved" || headers["HX-Trigger-After-Swap"] != "" || headers["HX-Trigger-After-Settle"] != "" {
		t.Fatalf("v4 response headers: %v", headers)
	}

	if got := NewHTMX(nil).ResponseHeaders(response)["HX-Trigger-After-Swap"]; got != "old-swap" {
		t.Fatal("v2 timing headers changed")
	}

	stream := conn.InteractionAttrs(Interaction{Kind: InteractionStream, ID: "feed", URL: "/events", Swap: "beforeend"})
	want := map[string]string{"hx-sse:connect": "/events", "hx-target": "#feed", "hx-swap": "beforeend"}
	if !reflect.DeepEqual(stream, want) {
		t.Fatalf("stream = %v, want %v", stream, want)
	}

	options := map[string]string{"from": "closest form"}
	on := conn.InteractionAttrs(Interaction{Kind: InteractionOn, Name: "saved", Options: options})
	if on["hx-trigger"] != `saved from:"closest form"` || options["from"] != "closest form" {
		t.Fatalf("event selector escaping or caller options changed: %v / %v", on, options)
	}

	for _, kind := range []InteractionKind{InteractionAsync, InteractionReveal, InteractionPoll, InteractionRefresh, InteractionPrefetch} {
		interaction := Interaction{Kind: kind, ID: "result", URL: "/result"}
		if !reflect.DeepEqual(conn.InteractionAttrs(interaction), NewHTMX(nil).InteractionAttrs(interaction)) {
			t.Fatalf("unexpected change to %s", kind)
		}
	}

	swap := NewSwap().Style(SwapOuterHTML).Show("#main .result", "top").Scroll("#list", "bottom")
	response = Response{}
	builder := NewResponseBuilder(&response).ReswapWith(swap)
	got := conn.ResponseHeaders(response)["HX-Reswap"]
	if got != `outerHTML show:top showTarget:"#main .result" scroll:bottom scrollTarget:"#list"` {
		t.Fatalf("v4 swap = %q", got)
	}

	if got := NewHTMX(nil).ResponseHeaders(response)["HX-Reswap"]; got != `outerHTML show:#main .result:top scroll:#list:bottom` {
		t.Fatalf("v2 swap changed: %q", got)
	}

	swap.Show("#later", "top").Style(SwapInnerHTML)
	if after := conn.ResponseHeaders(response)["HX-Reswap"]; after != got {
		t.Fatalf("response did not snapshot the options: %q", after)
	}

	builder.Reswap("outerMorph")
	if got := conn.ResponseHeaders(response)["HX-Reswap"]; got != "outerMorph" {
		t.Fatalf("raw override = %q", got)
	}
}

// Expectations follow the HTMX 4.0 header and attribute references, rather than
// inheriting expected output from the legacy connector.
func TestHTMX4CompleteResponseHeaders(t *testing.T) {
	refresh := true
	response := Response{Location: "/next", PushURL: "/push", Redirect: "/redirect", Refresh: &refresh, ReplaceURL: "/replace", Reswap: "outerHTML", Retarget: "#next", Reselect: ".selected", Trigger: "saved", TriggerAfterSwap: "legacy-swap", TriggerAfterSettle: "legacy-settle"}
	want := map[string]string{"HX-Location": "/next", "HX-Push-Url": "/push", "HX-Redirect": "/redirect", "HX-Refresh": "true", "HX-Replace-Url": "/replace", "HX-Reswap": "outerHTML", "HX-Retarget": "#next", "HX-Reselect": ".selected", "HX-Trigger": "saved"}
	if got := NewHTMX4(nil).ResponseHeaders(response); !reflect.DeepEqual(got, want) {
		t.Fatalf("headers = %v, want %v", got, want)
	}
	if got := NewHTMX4(nil).ResponseHeaders(Response{}); len(got) != 0 {
		t.Fatalf("empty response emitted headers: %v", got)
	}
}

func TestHTMX4InteractionAttributeCatalogue(t *testing.T) {
	for _, tc := range []struct {
		kind    InteractionKind
		trigger string
	}{
		{InteractionAsync, "load"}, {InteractionReveal, "revealed"}, {InteractionPoll, "every 3s"}, {InteractionRefresh, "click"}, {InteractionOn, "saved from:body"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			interaction := Interaction{Kind: tc.kind, ID: "result", URL: "/result", Interval: "3s", Name: "saved", Options: map[string]string{"from": "body"}}
			want := map[string]string{"hx-get": "/result", "hx-trigger": tc.trigger, "hx-target": "#result", "hx-swap": "innerHTML"}
			if got := NewHTMX4(nil).InteractionAttrs(interaction); !reflect.DeepEqual(got, want) {
				t.Fatalf("attributes = %v, want %v", got, want)
			}
		})
	}
}

func TestSwapFocusScrollUsesConnectorProtocol(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		response := Response{}
		NewResponseBuilder(&response).ReswapWith(NewSwap().Style(SwapOuterHTML).Swap(120 * time.Millisecond).Settle(20 * time.Millisecond).Transition(true).IgnoreTitle(false).FocusScroll(enabled))
		suffix := boolString(enabled)
		want4 := "outerHTML swap:120ms settle:20ms transition:true ignoreTitle:false focusScroll:" + suffix
		want2 := "outerHTML swap:120ms settle:20ms transition:true ignoreTitle:false focus-scroll:" + suffix
		if got := NewHTMX4(nil).ResponseHeaders(response)["HX-Reswap"]; got != want4 {
			t.Fatalf("HTMX 4 = %q, want %q", got, want4)
		}
		if got := NewHTMX(nil).ResponseHeaders(response)["HX-Reswap"]; got != want2 {
			t.Fatalf("HTMX 2 = %q, want %q", got, want2)
		}
	}
}

func TestHTMX4NewSwapStyles(t *testing.T) {
	for style, want := range map[SwapStyle]string{SwapInnerMorph: "innerMorph", SwapOuterMorph: "outerMorph", SwapOuterSync: "outerSync", SwapTextContent: "textContent"} {
		response := Response{}
		NewResponseBuilder(&response).ReswapWith(NewSwap().Style(style))
		if got := NewHTMX4(nil).ResponseHeaders(response)["HX-Reswap"]; got != want {
			t.Fatalf("style = %q, want %q", got, want)
		}
	}
}
