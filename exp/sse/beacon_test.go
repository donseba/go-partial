package sse

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func beaconRequest(form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/presence", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	return req
}

func TestReadBeacon(t *testing.T) {
	req := beaconRequest(url.Values{
		"clientId": {"3f2a-tab"},
		"reason":   {"removed"},
		"params":   {"page=%2Fadmin%2Fpages&tab=3f2a-tab"},
	})

	beacon, err := ReadBeacon(req)
	if err != nil {
		t.Fatalf("ReadBeacon() error = %v", err)
	}

	if beacon.ClientID != "3f2a-tab" || beacon.Reason != BeaconRemoved || beacon.Params.Get("page") != "/admin/pages" ||
		beacon.Params.Get("tab") != "3f2a-tab" {
		t.Fatalf("ReadBeacon() = %+v", beacon)
	}

	if !beacon.Reason.Leaving() || !BeaconPagehide.Leaving() || BeaconBeat.Leaving() || BeaconHidden.Leaving() {
		t.Fatal("Leaving() is wrong")
	}
}

func TestReadBeaconRefusesOtherRequests(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
	}{
		{"no client id", beaconRequest(url.Values{"reason": {"beat"}})},
		{"unknown reason", beaconRequest(url.Values{"clientId": {"x"}, "reason": {"bye"}})},
		{"no reason", beaconRequest(url.Values{"clientId": {"x"}})},
		{"bad params", beaconRequest(url.Values{"clientId": {"x"}, "reason": {"beat"}, "params": {"a=%zz"}})},
		{"get", httptest.NewRequest(http.MethodGet, "/presence?clientId=x&reason=beat", nil)},
		{
			"too large",
			beaconRequest(url.Values{"clientId": {"x"}, "reason": {"beat"}, "params": {strings.Repeat("a", maxBeaconSize)}}),
		},
	}

	for _, test := range tests {
		if _, err := ReadBeacon(test.req); !errors.Is(err, ErrInvalidBeacon) {
			t.Errorf("%s: ReadBeacon() error = %v, want ErrInvalidBeacon", test.name, err)
		}
	}

	if _, err := ReadBeacon(nil); !errors.Is(err, ErrInvalidBeacon) {
		t.Errorf("ReadBeacon(nil) error = %v", err)
	}
}
