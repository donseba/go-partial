package connector

import (
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestHTMXResponseHeaders(t *testing.T) {
	conn := NewHTMX(nil)
	response := Response{}
	NewResponseBuilder(&response).
		Retarget("#toast").
		ReswapWith(NewSwap().Style(SwapOuterHTML).Swap(150 * time.Millisecond).Transition(true)).
		TriggerWith(NewTrigger().AddEventObject("notice", map[string]any{"message": "Saved"}))

	headers := conn.ResponseHeaders(response)
	if got := headers[HTMXHeaderRetarget.String()]; got != "#toast" {
		t.Fatalf("expected retarget header, got %q", got)
	}
	if got := headers[HTMXHeaderReswap.String()]; got != "outerHTML swap:150ms transition:true" {
		t.Fatalf("expected reswap header, got %q", got)
	}
	if got := headers[HTMXHeaderTrigger.String()]; got != `{"notice":{"message":"Saved"}}` {
		t.Fatalf("expected trigger header, got %q", got)
	}
}

func TestConnectorsHandleNilRequest(t *testing.T) {
	connectors := []Connector{
		NewPartial(&Config{UseURLQuery: true}),
		NewHTMX(&Config{UseURLQuery: true}),
		NewTurbo(&Config{UseURLQuery: true}),
		NewUnpoly(&Config{UseURLQuery: true}),
	}

	for _, conn := range connectors {
		if conn.RenderPartial(nil) {
			t.Fatalf("%T RenderPartial(nil) = true, want false", conn)
		}
		if got := conn.GetTargetValue(nil); got != "" {
			t.Fatalf("%T GetTargetValue(nil) = %q, want empty", conn, got)
		}
		if got := conn.GetSelectValue(nil); got != "" {
			t.Fatalf("%T GetSelectValue(nil) = %q, want empty", conn, got)
		}
		if got := conn.GetActionValue(nil); got != "" {
			t.Fatalf("%T GetActionValue(nil) = %q, want empty", conn, got)
		}
	}
}

func TestConnectorURLQueryFallbackHandlesNilURL(t *testing.T) {
	conn := NewPartial(&Config{UseURLQuery: true})
	req := &http.Request{Header: make(http.Header)}

	if got := conn.GetTargetValue(req); got != "" {
		t.Fatalf("GetTargetValue(nil URL) = %q, want empty", got)
	}
	if got := conn.GetSelectValue(req); got != "" {
		t.Fatalf("GetSelectValue(nil URL) = %q, want empty", got)
	}
	if got := conn.GetActionValue(req); got != "" {
		t.Fatalf("GetActionValue(nil URL) = %q, want empty", got)
	}
}

// TestResponseBuilderSetsEveryHeader checks that each builder method sets its
// field, and that the generic connectors write each one in its X- header.
func TestResponseBuilderSetsEveryHeader(t *testing.T) {
	builder := NewResponseBuilder(nil).
		Location("/next").
		PushURL("/pushed").
		Redirect("/elsewhere").
		Refresh(false).
		ReplaceURL("/replaced").
		Reswap("outerHTML").
		Retarget("#main").
		Reselect("#part").
		Trigger("saved").
		TriggerAfterSettle("settled").
		TriggerAfterSwap("swapped")

	response := builder.Value()

	want := map[string]string{
		"X-Location":             "/next",
		"X-Push-Url":             "/pushed",
		"X-Redirect":             "/elsewhere",
		"X-Refresh":              "false",
		"X-Replace-Url":          "/replaced",
		"X-Reswap":               "outerHTML",
		"X-Retarget":             "#main",
		"X-Reselect":             "#part",
		"X-Trigger":              "saved",
		"X-Trigger-After-Settle": "settled",
		"X-Trigger-After-Swap":   "swapped",
	}

	for _, conn := range []Connector{NewPartial(nil), NewTurbo(nil), NewUnpoly(nil)} {
		if got := conn.ResponseHeaders(response); !reflect.DeepEqual(got, want) {
			t.Fatalf("%T ResponseHeaders() = %#v, want %#v", conn, got, want)
		}
	}

	if got := NewPartial(nil).ResponseHeaders(Response{}); len(got) != 0 {
		t.Fatalf("headers of an empty response = %v", got)
	}

	htmx := NewHTMX(nil).ResponseHeaders(NewResponseBuilder(nil).Refresh(true).Value())
	if !reflect.DeepEqual(htmx, map[string]string{"HX-Refresh": "true"}) {
		t.Fatalf("htmx refresh headers = %v", htmx)
	}
}

// TestResponseBuilderTriggersAndSwaps checks the triggers and swaps built
// with their builders, and that nil builders leave the response as it was.
func TestResponseBuilderTriggersAndSwaps(t *testing.T) {
	var response Response

	NewResponseBuilder(&response).
		Trigger("kept").
		TriggerWith(nil).
		TriggerAfterSettle("kept").
		TriggerAfterSettleWith(nil).
		TriggerAfterSwap("kept").
		TriggerAfterSwapWith(nil).
		Reswap("kept").
		ReswapWith(nil)

	if response.Trigger != "kept" || response.TriggerAfterSettle != "kept" || response.TriggerAfterSwap != "kept" || response.Reswap != "kept" {
		t.Fatalf("nil builders changed the response: %+v", response)
	}

	NewResponseBuilder(&response).
		TriggerAfterSettleWith(NewTrigger().AddEvent("settled")).
		TriggerAfterSwapWith(NewTrigger().AddEventDetailed("swapped", "row 4")).
		TriggerWith(NewTrigger())

	if response.TriggerAfterSettle != `{"settled":null}` {
		t.Fatalf("TriggerAfterSettle = %q", response.TriggerAfterSettle)
	}

	if response.TriggerAfterSwap != `{"swapped":"row 4"}` {
		t.Fatalf("TriggerAfterSwap = %q", response.TriggerAfterSwap)
	}

	if response.Trigger != "" {
		t.Fatalf("Trigger of an empty trigger = %q, want empty", response.Trigger)
	}

	broken := NewTrigger().AddEventObject("broken", map[string]any{"ch": make(chan int)})
	if got := broken.String(); got != "" {
		t.Fatalf("trigger that cannot be encoded = %q, want empty", got)
	}
}

func TestSwapString(t *testing.T) {
	tests := []struct {
		name string
		swap *Swap
		want string
	}{
		{name: "nil", swap: nil, want: ""},
		{name: "default", swap: NewSwap(), want: "innerHTML"},
		{
			name: "every modifier",
			swap: NewSwap().
				Style(SwapBeforeEnd).
				Swap(2*time.Second).
				Settle(1500*time.Millisecond).
				Transition(false).
				IgnoreTitle(true).
				FocusScroll(true).
				Scroll("#list", "bottom").
				Show("", "top"),
			want: "beforeend swap:2s settle:1.5s transition:false ignoreTitle:true focus-scroll:true scroll:#list:bottom show:top",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.swap.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLocationStringFallsBackToThePath checks that a location whose values
// cannot be encoded is written as its path.
func TestLocationStringFallsBackToThePath(t *testing.T) {
	location := Location{Path: "/next", Values: map[string]any{"ch": make(chan int)}}

	if got := location.String(); got != "/next" {
		t.Fatalf("String() = %q, want the path", got)
	}
}

func TestApplyHeadersSetsEachValue(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Trigger", "old")

	ApplyHeaders(headers, map[string]string{"X-Trigger": "new", "X-Redirect": "/next"})

	if got := headers.Get("X-Trigger"); got != "new" {
		t.Fatalf("X-Trigger = %q", got)
	}

	if got := headers.Get("X-Redirect"); got != "/next" {
		t.Fatalf("X-Redirect = %q", got)
	}
}
