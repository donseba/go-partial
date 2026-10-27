package connector

import (
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
)

type (
	// RequestInspector is implemented by connectors that can tell more about
	// a request than which partial it asks for. The connectors of this
	// package implement it; use the package functions IsRequest, IsBoosted
	// and CurrentURL, which also accept connectors that do not.
	RequestInspector interface {
		// IsRequest reports whether the client library made the request,
		// also when it asks for a whole page, such as an htmx 4 request for
		// the body.
		IsRequest(r *http.Request) bool
		// IsBoosted reports whether the request follows a boosted link or
		// form.
		IsBoosted(r *http.Request) bool
		// CurrentURL returns the URL of the page the client shows, or nil
		// when the request does not tell.
		CurrentURL(r *http.Request) *url.URL
	}

	// HeaderVarier is implemented by connectors that know which request
	// headers change the response; see VaryHeaders.
	HeaderVarier interface {
		VaryHeaders() []string
	}

	// CurrentURLHeaderNamer is implemented by connectors whose client
	// library sends the URL of the page it shows in a request header; see
	// CurrentURLHeader.
	CurrentURLHeaderNamer interface {
		CurrentURLHeader() string
	}
)

const (
	// TargetSelf targets the element an interaction is placed on, for
	// interaction attributes on an existing element. Connectors write it in
	// their own syntax.
	TargetSelf = "this"

	// UnpolyHeaderVersion is sent by Unpoly with every request it makes.
	UnpolyHeaderVersion HeaderKey = "X-Up-Version"
)

// IsRequest reports whether the connector's client library made the
// request. Connectors that do not implement RequestInspector answer whether
// the request asks for a partial.
func IsRequest(c Connector, r *http.Request) bool {
	if c == nil || r == nil {
		return false
	}

	if inspector, ok := c.(RequestInspector); ok {
		return inspector.IsRequest(r)
	}

	return c.RenderPartial(r)
}

// IsBoosted reports whether the request follows a boosted link or form;
// false for connectors without boosting.
func IsBoosted(c Connector, r *http.Request) bool {
	if c == nil || r == nil {
		return false
	}

	if inspector, ok := c.(RequestInspector); ok {
		return inspector.IsBoosted(r)
	}

	return false
}

// CurrentURL returns the URL of the page the client shows when it made the
// request, such as htmx's HX-Current-URL, or nil when the request does not
// tell. For a form posted with htmx, it is the page with the form.
func CurrentURL(c Connector, r *http.Request) *url.URL {
	if c == nil || r == nil {
		return nil
	}

	if inspector, ok := c.(RequestInspector); ok {
		return inspector.CurrentURL(r)
	}

	return nil
}

// CurrentURLHeader names the request header in which the connector's
// client library sends the URL of the page it shows (see CurrentURL), such
// as htmx's HX-Current-URL, for the Vary header of a response that depends
// on it; "" when the library sends none, or for a nil connector.
//
//	partial.OOBElements{IDs: ids, Vary: []string{connector.CurrentURLHeader(conn)}}
func CurrentURLHeader(c Connector) string {
	if namer, ok := c.(CurrentURLHeaderNamer); ok {
		return namer.CurrentURLHeader()
	}

	return ""
}

// VaryHeaders returns the request headers that change the connector's
// responses, for the Vary response header, so caches keep a page and a
// fragment of it apart. partial.Write adds them.
func VaryHeaders(c Connector) []string {
	if c == nil {
		return nil
	}

	if varier, ok := c.(HeaderVarier); ok {
		return varier.VaryHeaders()
	}

	return nonEmpty(c.GetTargetHeader(), c.GetSelectHeader(), c.GetActionHeader())
}

// IsRequest reports whether the request names a target.
func (x *base) IsRequest(r *http.Request) bool {
	return x.RenderPartial(r)
}

// IsBoosted is false: the connector has no boosting.
func (x *base) IsBoosted(*http.Request) bool {
	return false
}

// CurrentURL is nil: the protocol does not send it.
func (x *base) CurrentURL(*http.Request) *url.URL {
	return nil
}

// VaryHeaders returns the target, select and action headers.
func (x *base) VaryHeaders() []string {
	return nonEmpty(x.targetHeader, x.selectHeader, x.actionHeader)
}

// IsRequest reports whether htmx made the request.
func (h *HTMX) IsRequest(r *http.Request) bool {
	return r != nil && headerValue(r.Header, h.requestHeader) == "true"
}

// IsBoosted reports whether htmx made the request for a boosted link or
// form.
func (h *HTMX) IsBoosted(r *http.Request) bool {
	return r != nil && headerValue(r.Header, h.boostedHeader) == "true"
}

// CurrentURL returns the URL htmx sends as HX-Current-URL, or nil.
func (h *HTMX) CurrentURL(r *http.Request) *url.URL {
	if r == nil {
		return nil
	}

	return parseURL(headerValue(r.Header, HTMXHeaderCurrentURL.String()))
}

// CurrentURLHeader names HX-Current-URL, in which htmx sends the URL of
// the page it shows.
func (h *HTMX) CurrentURLHeader() string {
	return HTMXHeaderCurrentURL.String()
}

// VaryHeaders returns the htmx request headers that decide what is
// rendered, and the select and action headers.
func (h *HTMX) VaryHeaders() []string {
	return nonEmpty(
		h.requestHeader,
		h.boostedHeader,
		h.historyRestoreRequestHeader,
		h.targetHeader,
		h.selectHeader,
		h.actionHeader,
	)
}

// VaryHeaders adds HX-Request-Type, with which htmx 4 asks for a whole
// page, to the headers of htmx 2.
func (h *HTMX4) VaryHeaders() []string {
	return nonEmpty(
		h.requestHeader,
		h.boostedHeader,
		h.historyRestoreRequestHeader,
		h.targetHeader,
		h.selectHeader,
		h.actionHeader,
		HTMXHeaderRequestType.String(),
	)
}

// IsRequest reports whether Unpoly made the request.
func (u *Unpoly) IsRequest(r *http.Request) bool {
	return r != nil && (headerValue(r.Header, UnpolyHeaderVersion.String()) != "" || u.RenderPartial(r))
}

func parseURL(value string) *url.URL {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil
	}

	return parsed
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}

	return out
}

// canonicalHeaderKeys are the canonical forms of the header names
// connectors read, such as Hx-Target for HX-Target, made once each.
var canonicalHeaderKeys sync.Map

// headerValue is header.Get(key) without canonicalizing key, which
// allocates for names such as HX-Target, on every call.
func headerValue(header http.Header, key string) string {
	canonical, ok := canonicalHeaderKeys.Load(key)
	if !ok {
		canonical, _ = canonicalHeaderKeys.LoadOrStore(key, textproto.CanonicalMIMEHeaderKey(key))
	}

	values := header[canonical.(string)]
	if len(values) == 0 {
		return ""
	}

	return values[0]
}
