package flash

import (
	"strings"
	"testing"
)

func TestClientScriptIsTheFlashClient(t *testing.T) {
	script := string(ClientScript())
	if !strings.Contains(script, `"partial:flash"`) || !strings.Contains(script, "data-flash-request-error") {
		t.Fatal("ClientScript() is not the flash client")
	}
}

func TestClientCountsOnlyUnswappedErrorResponses(t *testing.T) {
	script := string(ClientScript())

	for _, want := range []string{
		// htmx 4: kept at htmx:response:error, decided once htmx swapped it.
		`"htmx:response:error"`,
		`"htmx:finally:request"`,
		`ctx.status !== "swapped"`,
		`style === "none"`,
		// htmx 2: the swap decision of htmx:beforeSwap.
		`"htmx:beforeSwap"`,
		"decision.shouldSwap",
		// The text/plain sentence of a refusal is not swapped.
		`ctx.swap = "none"`,
		"detail.shouldSwap = false",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("client script lacks %s", want)
		}
	}
}
