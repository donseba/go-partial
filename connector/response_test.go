package connector

import (
	"net/http"
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
