package connector

import (
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
