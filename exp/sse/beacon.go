package sse

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

type (
	// Beacon is a report of the browser client (ClientScript) about a
	// stream of an element with data-partial-stream-beacon (see
	// ClientStream.Beacon); ReadBeacon reads it.
	Beacon struct {
		// ClientID is the id of the browser tab, goPartialSSE.clientId.
		ClientID string
		// Params are the parameters of the stream URL, with the client id
		// under the name of ClientStream.ClientID, such as tab, and those
		// of the application's own, such as the page of a presence stream.
		Params url.Values
		// Reason says why the client reports.
		Reason BeaconReason
	}

	// BeaconReason says why the browser client reports a stream.
	BeaconReason string
)

const (
	// BeaconBeat is a report every ClientStream.Beat, while the element is
	// in the page, also while the tab is hidden and the stream closed.
	BeaconBeat BeaconReason = "beat"
	// BeaconHidden says the stream closed as the tab is hidden (see
	// ClientStream.Hidden); the tab is still there.
	BeaconHidden BeaconReason = "hidden"
	// BeaconRemoved says the element left the page, such as after a
	// navigation to a page without the stream, and no element in the page
	// reports to the same address.
	BeaconRemoved BeaconReason = "removed"
	// BeaconPagehide says the page is being left, or kept in the browser's
	// back/forward cache, which opens the stream again when it is restored.
	BeaconPagehide BeaconReason = "pagehide"
)

// ErrInvalidBeacon is the error of ReadBeacon for a request that is not a
// report of the browser client.
var ErrInvalidBeacon = errors.New("sse: invalid beacon")

// maxBeaconSize is the largest body ReadBeacon reads; a report is a form
// of three short fields.
const maxBeaconSize = 16 << 10

// Leaving reports whether the reason says the tab no longer shows the
// stream's element: BeaconRemoved or BeaconPagehide.
func (r BeaconReason) Leaving() bool {
	return r == BeaconRemoved || r == BeaconPagehide
}

// ReadBeacon reads the report of the browser client from r, a POST to the
// address of data-partial-stream-beacon: the form fields clientId, reason
// and params, the query of the stream URL. It reads at most 16 KiB and
// returns an error wrapping ErrInvalidBeacon for a request without a client
// id or with an unknown reason.
//
//	beacon, err := sse.ReadBeacon(r)
//	if err != nil {
//		http.Error(w, "invalid_beacon", http.StatusBadRequest)
//		return
//	}
//	if beacon.Reason.Leaving() {
//		presence.Leave(beacon.Params.Get("tab"))
//	} else {
//		presence.Visit(beacon.Params.Get("tab"), beacon.Params.Get("page"))
//	}
func ReadBeacon(r *http.Request) (Beacon, error) {
	if r == nil {
		return Beacon{}, fmt.Errorf("%w: no request", ErrInvalidBeacon)
	}

	if r.Method != http.MethodPost {
		return Beacon{}, fmt.Errorf("%w: method %s", ErrInvalidBeacon, r.Method)
	}

	if r.Body != nil && r.PostForm == nil {
		r.Body = http.MaxBytesReader(nil, r.Body, maxBeaconSize)
	}

	if err := r.ParseForm(); err != nil {
		return Beacon{}, fmt.Errorf("%w: %w", ErrInvalidBeacon, err)
	}

	params, err := url.ParseQuery(r.PostForm.Get("params"))
	if err != nil {
		return Beacon{}, fmt.Errorf("%w: params: %w", ErrInvalidBeacon, err)
	}

	beacon := Beacon{
		ClientID: r.PostForm.Get("clientId"),
		Params:   params,
		Reason:   BeaconReason(r.PostForm.Get("reason")),
	}

	if beacon.ClientID == "" {
		return Beacon{}, fmt.Errorf("%w: no client id", ErrInvalidBeacon)
	}

	switch beacon.Reason {
	case BeaconBeat, BeaconHidden, BeaconRemoved, BeaconPagehide:
	default:
		return Beacon{}, fmt.Errorf("%w: reason %q", ErrInvalidBeacon, beacon.Reason)
	}

	return beacon, nil
}
