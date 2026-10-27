package connector

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestRequestInspection(t *testing.T) {
	full := httptest.NewRequest(http.MethodGet, "/", nil)
	full.Header.Set("HX-Request", "true")
	full.Header.Set("HX-Request-Type", "full")
	full.Header.Set("HX-Boosted", "true")
	full.Header.Set("HX-Current-URL", "https://example.com/admin/pages?id=4")

	htmx4 := NewHTMX4(nil)
	if htmx4.RenderPartial(full) {
		t.Fatal("RenderPartial() = true for an htmx 4 request for the whole page")
	}

	if !IsRequest(htmx4, full) || !IsBoosted(htmx4, full) {
		t.Fatal("IsRequest() or IsBoosted() = false for a boosted htmx request")
	}

	if current := CurrentURL(htmx4, full); current == nil || current.Path != "/admin/pages" || current.Query().Get("id") != "4" {
		t.Fatalf("CurrentURL() = %v", current)
	}

	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsRequest(htmx4, plain) || IsBoosted(htmx4, plain) || CurrentURL(htmx4, plain) != nil {
		t.Fatal("a plain request is reported as an htmx request")
	}

	partial := NewPartial(nil)
	targeted := httptest.NewRequest(http.MethodGet, "/", nil)
	targeted.Header.Set(HeaderTarget.String(), "content")

	if !IsRequest(partial, targeted) || IsRequest(partial, plain) || IsBoosted(partial, targeted) {
		t.Fatal("partial connector inspection is wrong")
	}

	unpoly := httptest.NewRequest(http.MethodGet, "/", nil)
	unpoly.Header.Set(UnpolyHeaderVersion.String(), "3.10.0")

	if !IsRequest(NewUnpoly(nil), unpoly) {
		t.Fatal("IsRequest() = false for an Unpoly request")
	}
}

func TestVaryHeaders(t *testing.T) {
	tests := map[string]struct {
		connector Connector
		want      []string
	}{
		"htmx": {
			NewHTMX(nil),
			[]string{"HX-Request", "HX-Boosted", "HX-History-Restore-Request", "HX-Target", "X-Select", "X-Action"},
		},
		"htmx4": {
			NewHTMX4(nil),
			[]string{
				"HX-Request",
				"HX-Boosted",
				"HX-History-Restore-Request",
				"HX-Target",
				"X-Select",
				"X-Action",
				"HX-Request-Type",
			},
		},
		"partial": {NewPartial(nil), []string{"X-Target", "X-Select", "X-Action"}},
		"turbo":   {NewTurbo(nil), []string{"Turbo-Frame", "Turbo-Select", "Turbo-Action"}},
	}

	for name, test := range tests {
		if got := VaryHeaders(test.connector); !slices.Equal(got, test.want) {
			t.Fatalf("%s: VaryHeaders() = %v, want %v", name, got, test.want)
		}
	}
}

func TestLocationString(t *testing.T) {
	var response Response
	NewResponseBuilder(&response).Location(Location{Path: "/admin/pages"}.String())

	if response.Location != "/admin/pages" {
		t.Fatalf("Location = %q", response.Location)
	}

	NewResponseBuilder(&response).Location(Location{Path: "/admin/pages", Target: "#content", Swap: "outerHTML"}.String())

	want := `{"path":"/admin/pages","target":"#content","swap":"outerHTML"}`
	if response.Location != want {
		t.Fatalf("Location = %q, want %q", response.Location, want)
	}

	if got := NewHTMX4(nil).ResponseHeaders(response)["HX-Location"]; got != want {
		t.Fatalf("HX-Location = %q", got)
	}
}

func TestUnpolyWritesTheSelfTarget(t *testing.T) {
	interaction := Interaction{Kind: InteractionPoll, URL: "/status", Target: TargetSelf, Interval: "5s"}

	attrs := NewUnpoly(nil).InteractionAttrs(interaction)
	if attrs[UnpolyAttrTarget] != ":origin" {
		t.Fatalf("attrs = %v", attrs)
	}
}

// TestCurrentURLHeader checks that htmx 2 and 4 name HX-Current-URL, and
// connectors whose library sends no current URL name none.
func TestCurrentURLHeader(t *testing.T) {
	tests := []struct {
		name string
		conn Connector
		want string
	}{
		{"htmx", NewHTMX(nil), "HX-Current-URL"},
		{"htmx4", NewHTMX4(nil), "HX-Current-URL"},
		{"unpoly", NewUnpoly(nil), ""},
		{"turbo", NewTurbo(nil), ""},
		{"partial", NewPartial(nil), ""},
		{"nil", nil, ""},
	}

	for _, test := range tests {
		if got := CurrentURLHeader(test.conn); got != test.want {
			t.Errorf("%s: CurrentURLHeader() = %q, want %q", test.name, got, test.want)
		}
	}
}
