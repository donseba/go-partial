package connector

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRedirect(t *testing.T) {
	location := Location{Path: "/orders", Target: "#content"}

	htmxRequest := httptest.NewRequest(http.MethodPost, "/orders", nil)
	htmxRequest.Header.Set("HX-Request", "true")

	rec := httptest.NewRecorder()
	Redirect(NewHTMX4(nil), rec, htmxRequest, location)

	if rec.Code != http.StatusOK || rec.Header().Get("HX-Location") != `{"path":"/orders","target":"#content"}` {
		t.Fatalf("htmx response = %d %v", rec.Code, rec.Header())
	}

	for name, conn := range map[string]Connector{
		"htmx":    NewHTMX(nil),
		"unpoly":  NewUnpoly(nil),
		"turbo":   NewTurbo(nil),
		"partial": NewPartial(nil),
		"nil":     nil,
	} {
		rec := httptest.NewRecorder()
		Redirect(conn, rec, httptest.NewRequest(http.MethodPost, "/orders", nil), location)

		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/orders" {
			t.Fatalf("%s: response = %d %v, want a 303", name, rec.Code, rec.Header())
		}
	}
}

func TestAttrs(t *testing.T) {
	attr := Attr{Confirm: "Sure?", Boost: BoostOff, Target: "#list", Swap: "outerHTML"}

	cases := map[string]struct {
		conn Connector
		want map[string]string
	}{
		"htmx": {
			NewHTMX(nil),
			map[string]string{"hx-confirm": "Sure?", "hx-boost": "false", "hx-target": "#list", "hx-swap": "outerHTML"},
		},
		"htmx4": {
			NewHTMX4(nil),
			map[string]string{"hx-confirm": "Sure?", "hx-boost": "false", "hx-target": "#list", "hx-swap": "outerHTML"},
		},
		"unpoly": {
			NewUnpoly(nil),
			map[string]string{"up-confirm": "Sure?", "up-follow": "false", "up-submit": "false", "up-target": "#list"},
		},
		"turbo": {
			NewTurbo(nil),
			map[string]string{"data-turbo-confirm": "Sure?", "data-turbo": "false", "data-turbo-frame": "list"},
		},
		"partial": {
			NewPartial(nil),
			map[string]string{"data-confirm": "Sure?", "data-boost": "false", "data-target": "#list", "data-swap": "outerHTML"},
		},
	}
	for name, tc := range cases {
		if got := Attrs(tc.conn, attr); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: Attrs() = %v, want %v", name, got, tc.want)
		}
	}

	if got := Attrs(NewHTMX(nil), Attr{}); len(got) != 0 {
		t.Fatalf("Attrs() of nothing = %v", got)
	}

	got := Attrs(NewUnpoly(nil), Attr{Target: TargetSelf, Boost: BoostOn})
	if got["up-target"] != ":origin" || got["up-follow"] != "" {
		t.Fatalf("Attrs() = %v", got)
	}
}

func TestSetRequestTargetIsTheInverseOfGetTargetValue(t *testing.T) {
	for name, conn := range map[string]Connector{
		"htmx":    NewHTMX(nil),
		"htmx4":   NewHTMX4(nil),
		"unpoly":  NewUnpoly(nil),
		"turbo":   NewTurbo(nil),
		"partial": NewPartial(nil),
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		SetRequestTarget(conn, req, "comment list")

		if got := conn.GetTargetValue(req); got != "comment list" {
			t.Fatalf("%s: GetTargetValue() = %q", name, got)
		}

		if !conn.RenderPartial(req) || !IsRequest(conn, req) {
			t.Fatalf("%s: the request should be a partial request", name)
		}
	}
}
