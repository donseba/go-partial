package interactions

import (
	"strings"
	"testing"
)

func TestClientScriptIsTheInteractionsClient(t *testing.T) {
	if !strings.Contains(string(ClientScript()), "data-partial-confirm") {
		t.Fatal("ClientScript() is not the interactions client")
	}
}

func TestClientAsksInThePagesDialog(t *testing.T) {
	script := string(ClientScript())

	for _, want := range []string{
		`var dialogAttr = "data-partial-confirm-dialog"`,
		`"dialog[" + dialogAttr + "]"`,
		"[data-partial-confirm-question]",
		"showModal()",
		`found.returnValue === "confirm"`,
		// htmx 4 waits for issueRequest or dropRequest, htmx 2 skips its
		// own question with issueRequest(true).
		`"htmx:confirm"`,
		"detail.ctx.confirm",
		"detail.dropRequest()",
		"detail.issueRequest(true)",
		// A submit button's question waits for the dialog, then the click
		// or submission is made again.
		"button.click()",
		"form.requestSubmit(button)",
		// Without a dialog, the browser asks.
		"global.confirm(question)",
		"ask: ask",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("client script lacks %s", want)
		}
	}
}

func TestClientReloadsFailedLinksAndForms(t *testing.T) {
	script := string(ClientScript())

	for _, want := range []string{
		"data-partial-reload-on-error",
		`"htmx:response:error"`,
		`"htmx:error"`,
		`"htmx:responseError"`,
		`"htmx:sendError"`,
		`"up:fragment:offline"`,
		`"turbo:fetch-request-error"`,
		"HTMLFormElement.prototype.submit.call",
		"AbortError",
		// Unsafe methods are never sent twice: the error page is shown,
		// or the failure dispatched.
		`verb === "get" || verb === "head"`,
		"document.write(html)",
		`"partial:reload-on-error:failed"`,
		// An htmx 4 answer that leaves the page as it is, such as an error
		// flash message, is not replaced.
		`headers.get("HX-Reswap")`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("client script lacks %s", want)
		}
	}
}
