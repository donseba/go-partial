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

// TestMergeAttrsReplacesEverySetting checks that each setting of attr takes
// out the attributes the connector wrote for it before.
func TestMergeAttrsReplacesEverySetting(t *testing.T) {
	existing := map[string]string{
		"hx-boost":             "false",
		"hx-swap":              "outerHTML",
		"hx-disabled-elt":      "input",
		"data-partial-confirm": "Old?",
		ReloadOnErrorAttr:      "",
		"class":                "form",
	}

	attr := Attr{
		Boost:               BoostOn,
		Swap:                "innerHTML",
		DisableWhileRequest: "button",
		KeepOnServerError:   true,
		SubmitConfirm:       "Sure?",
		ReloadOnError:       true,
	}

	merged := MergeAttrs(NewHTMX(nil), existing, attr)

	want := map[string]string{
		"hx-boost":             "true",
		"hx-swap":              "innerHTML",
		"hx-disabled-elt":      "button",
		"data-partial-confirm": "Sure?",
		ReloadOnErrorAttr:      "",
		"class":                "form",
	}
	if !reflect.DeepEqual(merged, want) {
		t.Fatalf("MergeAttrs() = %v, want %v", merged, want)
	}

	cleared := MergeAttrs(NewHTMX(nil), merged, Attr{}, AttrBoost, AttrSwap, AttrDisableWhileRequest, AttrSubmitConfirm, AttrReloadOnError)
	if !reflect.DeepEqual(cleared, map[string]string{"class": "form"}) {
		t.Fatalf("MergeAttrs(remove all) = %v", cleared)
	}
}

// TestAttrsOfPlainConnectorsAreDataAttributes checks that a connector that
// does not write attributes itself gets the data- attributes.
func TestAttrsOfPlainConnectorsAreDataAttributes(t *testing.T) {
	got := Attrs(plainConnector{Connector: NewPartial(nil)}, Attr{Confirm: "Sure?", Boost: BoostOff, Target: "#main", Swap: "outerHTML"})

	want := map[string]string{
		"data-confirm": "Sure?",
		"data-boost":   "false",
		"data-target":  "#main",
		"data-swap":    "outerHTML",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Attrs() = %v, want %v", got, want)
	}
}

// TestSetRequestTargetOfPlainConnectors checks that a connector without
// RequestTargeter gets the target in its target header, that a request
// without headers gets them, and that nil connectors and requests are
// ignored.
func TestSetRequestTargetOfPlainConnectors(t *testing.T) {
	conn := plainConnector{Connector: NewPartial(nil)}
	req := &http.Request{}

	SetRequestTarget(conn, req, "comments")

	if got := req.Header.Get(HeaderTarget.String()); got != "comments" {
		t.Fatalf("target header = %q", got)
	}

	SetRequestTarget(nil, req, "other")
	SetRequestTarget(conn, nil, "other")

	if got := conn.GetTargetValue(req); got != "comments" {
		t.Fatalf("GetTargetValue() = %q after nil calls", got)
	}
}

func TestHTMX4ConfigValueKeepsShortQuotedValues(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "#content", want: "#content"},
		{in: "outerHTML show:top", want: "'outerHTML show:top'"},
		{in: "a 'b'", want: `"a 'b'"`},
		{in: "'already quoted'", want: "'already quoted'"},
		{in: " ", want: "' '"},
	}

	for _, tt := range tests {
		if got := htmx4ConfigValue(tt.in); got != tt.want {
			t.Errorf("htmx4ConfigValue(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
