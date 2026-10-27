package sse

import (
	"strings"
	"testing"
)

func TestClientScriptIsTheStreamClient(t *testing.T) {
	script := string(ClientScript())
	if !strings.Contains(script, `"partial:patch"`) || !strings.Contains(script, "data-partial-stream-client-id") {
		t.Fatal("ClientScript() is not the stream client")
	}
}

func TestClientReportsToTheBeacon(t *testing.T) {
	script := string(ClientScript())

	for _, want := range []string{
		"data-partial-stream-beacon",
		"data-partial-stream-beat",
		`body.set("clientId", clientId)`,
		`body.set("reason", reason)`,
		`body.set("params", query(stream.url))`,
		"sendBeacon",
		`"` + string(BeaconBeat) + `"`,
		`"` + string(BeaconHidden) + `"`,
		`"` + string(BeaconRemoved) + `"`,
		`"` + string(BeaconPagehide) + `"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("client script lacks %s", want)
		}
	}
}
