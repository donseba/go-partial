package sse

import (
	"bytes"
	_ "embed"
)

// clientScript is the browser client for the writer's events; see
// ClientScript.
//
//go:embed client.js
var clientScript []byte

// ClientScript returns the browser client for the writer's events. It
// applies partial:patch events to the page, replacing the elements each
// patch's target selects with its HTML (outerHTML), and dispatches
// partial:signal (also as partial:signal:<name>) and partial:error as DOM
// events on the document. Elements with a data-partial-stream="/url"
// attribute, or the stream markup of interactions' {{ stream }} with
// connectors that leave the client to the page, open a stream while they
// are in the page, and a new one when their URL changes; scripts can call
// goPartialSSE.connect(url). With htmx loaded, patched content is
// processed by htmx.
//
// An element with data-partial-stream-hidden="30s" closes its stream when
// the browser tab has been hidden that long, dispatching
// partial:stream:paused, and opens it again when the tab shows,
// dispatching partial:stream:reconnect on the element, so the page can
// fetch what it missed. A page restored from the browser's back/forward
// cache opens its streams again the same way.
//
// An element with data-partial-stream-client-id="tab" adds a client id to
// its stream URL as that parameter, the same for the life of the page in
// the tab, so the server tells the tabs of one browser apart, such as for
// presence; scripts read it as goPartialSSE.clientId.
//
// An element with data-partial-stream-beacon="/url" reports its stream to
// that address, which ReadBeacon reads: every data-partial-stream-beat
// (BeaconBeat), also while the tab is hidden and the stream closed, when
// the stream closes as the tab is hidden (BeaconHidden), when the element
// leaves the page and no element left reports to the same address
// (BeaconRemoved), and when the page is left (BeaconPagehide). Templates
// write these attributes with {{ sseAttrs }} (see Attrs and ClientStream).
//
// The client inserts the HTML of an event as the page's own: under a
// Content-Security-Policy without 'unsafe-inline', its style attributes
// are blocked, and an inline <style> or <script> in it applies only when
// the page's policy has its hash. Style patched content with classes.
//
// The client needs no build step and no other library; serve it from the
// file system of go-partial's clients package, or include it in your own
// bundle.
func ClientScript() []byte {
	return bytes.Clone(clientScript)
}

// ClientName is the name of the browser client in the file system of
// go-partial's clients package.
const ClientName = "partial-sse.js"
