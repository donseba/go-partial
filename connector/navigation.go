package connector

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type (
	// Redirector is implemented by connectors whose client library follows
	// a redirect of its own, such as htmx's HX-Location; see Redirect.
	Redirector interface {
		// Redirect answers r by sending the client to location, and reports
		// whether it did; when it reports false, Redirect answers with a
		// 303 See Other.
		Redirect(w http.ResponseWriter, r *http.Request, location Location) bool
	}

	// Boost says whether the client library takes over the links and forms
	// of an element, such as htmx's hx-boost; see Attr.
	Boost int

	// Attr describes how the client library treats a link or form, in
	// terms that do not depend on the library; see Attrs.
	Attr struct {
		// Confirm is a question the client asks before it follows the link
		// or submits the form, such as htmx's hx-confirm.
		Confirm string
		// Boost turns the library's handling of the element on or off,
		// such as htmx's hx-boost="false" for a link that must load a
		// whole page.
		Boost Boost
		// Target is the selector of the element the response is swapped
		// into; TargetSelf for the element itself.
		Target string
		// Swap is how the response is swapped in, such as outerHTML, for
		// connectors that can choose it.
		Swap string
		// Inherit gives the settings to the element's descendants, for
		// settings on the body or a container, such as boosting every link
		// into #content: htmx 4 writes its attributes with :inherited,
		// and a boost with a target or swap as hx-boost:inherited="target:#content
		// swap:outerHTML", which applies them to boosted links and forms
		// only; htmx 2 and the other libraries inherit them as they are.
		Inherit bool
		// DisableWhileRequest is the selector of the elements disabled
		// while a request of the element is under way, such as
		// "find button[type='submit']": htmx 4's hx-disable, htmx 2's
		// hx-disabled-elt, Unpoly's up-disable. Turbo disables the
		// submit button of a form itself.
		DisableWhileRequest string
		// KeepOnServerError leaves the page as it is when a response has a
		// 5xx status, such as a proxy's error page, without changing the
		// address: htmx 4's hx-status:5xx="swap:none push:false". Out-of-
		// band parts of the response, such as an error flash message, are
		// still swapped. htmx 2 swaps no error response by default.
		KeepOnServerError bool
		// SubmitConfirm is a question asked before a submit button sends
		// its form, for a form whose buttons do different things, such as
		// "Restore" and "Delete for good": data-turbo-confirm with Turbo,
		// which reads it from the button; data-partial-confirm otherwise,
		// which exp/interactions' browser client asks.
		SubmitConfirm string
		// ReloadOnError makes a link or form whose request gets a 5xx
		// status, or no answer at all, load as a whole page instead, so the
		// visitor sees what went wrong rather than nothing: a link is
		// followed and a GET form submitted again by the browser itself; a
		// POST, PUT, PATCH or DELETE is never sent twice, the server's HTML
		// error page replacing the page instead, if it sent one. On
		// a body or container it applies to the links and forms in it. It
		// is data-partial-reload-on-error with every connector, which
		// exp/interactions' browser client reads.
		ReloadOnError bool
	}

	// AttrWriter is implemented by connectors that write Attr as their
	// client library's attributes; see Attrs.
	AttrWriter interface {
		Attrs(attr Attr) map[string]string
	}

	// AttrKind names a setting of Attr, for MergeAttrs.
	AttrKind int

	// OOBWriter is implemented by connectors whose client library swaps an
	// element of a response out of band, outside the target, by an
	// attribute of the element; see OOBAttrs.
	OOBWriter interface {
		OOBAttrs(swap string) map[string]string
	}

	// RequestTargeter is implemented by connectors that can make a request
	// ask for a target the way their client library does; see
	// SetRequestTarget.
	RequestTargeter interface {
		SetRequestTarget(r *http.Request, target string)
	}
)

const (
	// AttrConfirm is Attr.Confirm.
	AttrConfirm AttrKind = iota + 1
	// AttrBoost is Attr.Boost.
	AttrBoost
	// AttrTarget is Attr.Target.
	AttrTarget
	// AttrSwap is Attr.Swap.
	AttrSwap
	// AttrDisableWhileRequest is Attr.DisableWhileRequest.
	AttrDisableWhileRequest
	// AttrKeepOnServerError is Attr.KeepOnServerError.
	AttrKeepOnServerError
	// AttrSubmitConfirm is Attr.SubmitConfirm.
	AttrSubmitConfirm
	// AttrReloadOnError is Attr.ReloadOnError.
	AttrReloadOnError
)

const (
	// BoostDefault leaves the element as the library treats it.
	BoostDefault Boost = iota
	// BoostOn makes the library take over the element.
	BoostOn
	// BoostOff makes the browser load the link or submit the form itself.
	BoostOff
)

// Redirect answers r by sending the client to location, such as after a
// form is posted: for a request the connector's client library made, the
// way the library follows it, such as htmx's HX-Location with status 200,
// so the client swaps location.Path into location.Target without loading a
// whole page; otherwise with a 303 See Other to location.Path, which
// browsers, Turbo and Unpoly follow with a GET.
//
//	connector.Redirect(conn, w, r, connector.Location{Path: "/orders", Target: "#content"})
func Redirect(c Connector, w http.ResponseWriter, r *http.Request, location Location) {
	if redirector, ok := c.(Redirector); ok && redirector.Redirect(w, r, location) {
		return
	}

	http.Redirect(w, r, location.Path, http.StatusSeeOther)
}

// Redirect sends a request htmx made to location with HX-Location; see the
// package function Redirect.
func (h *HTMX) Redirect(w http.ResponseWriter, r *http.Request, location Location) bool {
	if !h.IsRequest(r) {
		return false
	}

	w.Header().Set(HTMXHeaderLocation.String(), location.String())
	w.WriteHeader(http.StatusOK)

	return true
}

// Attrs returns the attributes of attr in the connector's client library,
// such as hx-confirm and hx-boost for htmx, up-confirm for Unpoly and
// data-turbo-confirm for Turbo, for a link or form of the template's own.
// Connectors that do not implement AttrWriter get data-confirm,
// data-boost, data-target and data-swap.
func Attrs(c Connector, attr Attr) map[string]string {
	if writer, ok := c.(AttrWriter); ok {
		return writer.Attrs(attr)
	}

	return dataAttrs(attr)
}

// MergeAttrs returns a copy of existing, such as the attributes of a form a
// library built, with the attributes of attr in the connector's syntax:
// for every setting attr has, and every kind in remove, the attributes the
// connector writes for that setting are taken out first, so attr's replace
// them. existing is not changed.
//
//	// Ask another question, or none when question is empty.
//	attrs = connector.MergeAttrs(c, attrs, connector.Attr{Confirm: question}, connector.AttrConfirm)
func MergeAttrs(c Connector, existing map[string]string, attr Attr, remove ...AttrKind) map[string]string {
	merged := make(map[string]string, len(existing)+2)
	maps.Copy(merged, existing)

	kinds := append(attrKinds(attr), remove...)
	for _, kind := range kinds {
		for _, name := range attrNames(c, kind) {
			delete(merged, name)
		}
	}

	maps.Copy(merged, Attrs(c, attr))

	return merged
}

// attrKinds returns the settings attr has.
func attrKinds(attr Attr) []AttrKind {
	var kinds []AttrKind
	if attr.Confirm != "" {
		kinds = append(kinds, AttrConfirm)
	}

	if attr.Boost != BoostDefault {
		kinds = append(kinds, AttrBoost)
	}

	if attr.Target != "" {
		kinds = append(kinds, AttrTarget)
	}

	if attr.Swap != "" {
		kinds = append(kinds, AttrSwap)
	}

	if attr.DisableWhileRequest != "" {
		kinds = append(kinds, AttrDisableWhileRequest)
	}

	if attr.KeepOnServerError {
		kinds = append(kinds, AttrKeepOnServerError)
	}

	if attr.SubmitConfirm != "" {
		kinds = append(kinds, AttrSubmitConfirm)
	}

	if attr.ReloadOnError {
		kinds = append(kinds, AttrReloadOnError)
	}

	return kinds
}

// attrNames returns the names of the attributes the connector writes for
// the setting kind, inherited or not.
func attrNames(c Connector, kind AttrKind) []string {
	var samples []Attr

	switch kind {
	case AttrConfirm:
		samples = []Attr{{Confirm: "?"}}
	case AttrBoost:
		samples = []Attr{{Boost: BoostOn}, {Boost: BoostOff}}
	case AttrTarget:
		samples = []Attr{{Target: "#target"}, {Target: TargetSelf}}
	case AttrSwap:
		samples = []Attr{{Swap: "innerHTML"}}
	case AttrDisableWhileRequest:
		samples = []Attr{{DisableWhileRequest: "button"}}
	case AttrKeepOnServerError:
		samples = []Attr{{KeepOnServerError: true}}
	case AttrSubmitConfirm:
		samples = []Attr{{SubmitConfirm: "?"}}
	case AttrReloadOnError:
		samples = []Attr{{ReloadOnError: true}}
	}

	var names []string

	for _, sample := range samples {
		for _, inherit := range []bool{false, true} {
			sample.Inherit = inherit
			names = slices.AppendSeq(names, maps.Keys(Attrs(c, sample)))
		}
	}

	return names
}

// Attrs writes attr as data-confirm, data-boost, data-target, data-swap,
// data-disable, data-keep-on-server-error, data-partial-confirm and
// data-partial-reload-on-error.
func (x *base) Attrs(attr Attr) map[string]string {
	return dataAttrs(attr)
}

func dataAttrs(attr Attr) map[string]string {
	attrs := make(map[string]string)
	setAttr(attrs, "data-confirm", attr.Confirm)
	setAttr(attrs, "data-boost", boostValue(attr.Boost, "true"))
	setAttr(attrs, "data-target", attr.Target)
	setAttr(attrs, "data-swap", attr.Swap)
	setAttr(attrs, "data-disable", attr.DisableWhileRequest)

	if attr.KeepOnServerError {
		attrs["data-keep-on-server-error"] = "true"
	}

	setAttr(attrs, SubmitConfirmAttr, attr.SubmitConfirm)
	setReloadOnError(attrs, attr)

	return attrs
}

// SubmitConfirmAttr is the attribute of a submit button whose question
// exp/interactions' browser client asks before the button sends its form;
// see Attr.SubmitConfirm.
const SubmitConfirmAttr = "data-partial-confirm"

// ReloadOnErrorAttr is the attribute of a link or form, or of an element
// around them, that makes exp/interactions' browser client load it as a
// whole page when its request fails; see Attr.ReloadOnError.
const ReloadOnErrorAttr = "data-partial-reload-on-error"

// setReloadOnError writes Attr.ReloadOnError, the same for every connector.
func setReloadOnError(attrs map[string]string, attr Attr) {
	if attr.ReloadOnError {
		attrs[ReloadOnErrorAttr] = ""
	}
}

// Attrs writes attr as hx-confirm, hx-boost, hx-target, hx-swap and
// hx-disabled-elt; htmx 2 inherits them as they are and keeps the page on
// error responses by default.
func (h *HTMX) Attrs(attr Attr) map[string]string {
	attrs := make(map[string]string)
	setAttr(attrs, "hx-confirm", attr.Confirm)
	setAttr(attrs, "hx-boost", boostValue(attr.Boost, "true"))
	setAttr(attrs, HTMXAttrTarget, attr.Target)
	setAttr(attrs, HTMXAttrSwap, attr.Swap)
	setAttr(attrs, "hx-disabled-elt", attr.DisableWhileRequest)
	setAttr(attrs, SubmitConfirmAttr, attr.SubmitConfirm)
	setReloadOnError(attrs, attr)

	return attrs
}

// Attrs writes attr as hx-confirm, hx-boost, hx-target, hx-swap,
// hx-disable and hx-status:5xx, with :inherited for Attr.Inherit; an
// inherited boost carries its target and swap, so they apply to boosted
// links and forms only, quoted when they hold whitespace or a comma, such
// as hx-boost:inherited="target:#content swap:'outerHTML showTarget:body'".
func (h *HTMX4) Attrs(attr Attr) map[string]string {
	suffix := ""
	if attr.Inherit {
		suffix = ":inherited"
	}

	attrs := make(map[string]string)
	setAttr(attrs, "hx-confirm"+suffix, attr.Confirm)

	if attr.Inherit && attr.Boost == BoostOn && (attr.Target != "" || attr.Swap != "") {
		var parts []string
		if attr.Target != "" {
			parts = append(parts, "target:"+htmx4ConfigValue(attr.Target))
		}

		if attr.Swap != "" {
			parts = append(parts, "swap:"+htmx4ConfigValue(attr.Swap))
		}

		attrs["hx-boost"+suffix] = strings.Join(parts, " ")
	} else {
		setAttr(attrs, "hx-boost"+suffix, boostValue(attr.Boost, "true"))
		setAttr(attrs, HTMXAttrTarget+suffix, attr.Target)
		setAttr(attrs, HTMXAttrSwap+suffix, attr.Swap)
	}

	setAttr(attrs, "hx-disable"+suffix, attr.DisableWhileRequest)

	if attr.KeepOnServerError {
		attrs["hx-status:5xx"+suffix] = "swap:none push:false"
	}

	setAttr(attrs, SubmitConfirmAttr, attr.SubmitConfirm)
	setReloadOnError(attrs, attr)

	return attrs
}

// htmx4ConfigValue writes value for htmx 4's config syntax, such as the
// "target:#content swap:outerHTML" of an inherited boost, which ends a value
// without quotes at whitespace or a comma: such a value is quoted, with
// single quotes unless it holds one, and a value in quotes is kept as it
// is.
func htmx4ConfigValue(value string) string {
	if !strings.ContainsAny(value, " \t\r\n,") || isQuoted(value) {
		return value
	}

	if strings.Contains(value, "'") {
		return `"` + value + `"`
	}

	return "'" + value + "'"
}

// isQuoted reports whether value is in single or double quotes.
func isQuoted(value string) bool {
	if len(value) < 2 {
		return false
	}

	first := value[0]
	last := value[len(value)-1]

	return (first == '\'' || first == '"') && first == last
}

// Attrs writes attr as up-confirm, up-follow and up-submit, up-target and
// up-disable; Unpoly has no swap of its own choosing.
func (u *Unpoly) Attrs(attr Attr) map[string]string {
	attrs := make(map[string]string)
	setAttr(attrs, "up-confirm", attr.Confirm)
	setAttr(attrs, "up-disable", attr.DisableWhileRequest)
	setAttr(attrs, SubmitConfirmAttr, attr.SubmitConfirm)
	setReloadOnError(attrs, attr)

	if boost := boostValue(attr.Boost, ""); attr.Boost != BoostDefault {
		attrs["up-follow"] = boost
		attrs["up-submit"] = boost
	}

	target := attr.Target
	if target == TargetSelf {
		target = ":origin"
	}

	setAttr(attrs, UnpolyAttrTarget, target)

	return attrs
}

// Attrs writes attr as data-turbo-confirm, also for a submit button,
// data-turbo and, for a target, data-turbo-frame with the frame's ID; Turbo
// has no swap of its own choosing.
func (t *Turbo) Attrs(attr Attr) map[string]string {
	attrs := make(map[string]string)
	setAttr(attrs, "data-turbo-confirm", attr.Confirm)
	setAttr(attrs, "data-turbo-confirm", attr.SubmitConfirm)
	setAttr(attrs, "data-turbo", boostValue(attr.Boost, "true"))
	setReloadOnError(attrs, attr)

	if attr.Target != "" && attr.Target != TargetSelf {
		attrs["data-turbo-frame"] = strings.TrimPrefix(attr.Target, "#")
	}

	return attrs
}

// boostValue is the value of a boost attribute: on for BoostOn, "false"
// for BoostOff and empty, which writes no attribute, for BoostDefault.
func boostValue(boost Boost, on string) string {
	switch boost {
	case BoostOn:
		return on
	case BoostOff:
		return "false"
	default:
		return ""
	}
}

func setAttr(attrs map[string]string, name, value string) {
	if value != "" {
		attrs[name] = value
	}
}

// SetRequestTarget makes r ask for the partial target, as the connector's
// client library would, so GetTargetValue(r) returns target: for tests and
// for requests a server makes to itself. Connectors that do not implement
// RequestTargeter get target in their target header.
//
//	req := httptest.NewRequest(http.MethodGet, "/", nil)
//	connector.SetRequestTarget(conn, req, "comments")
func SetRequestTarget(c Connector, r *http.Request, target string) {
	if c == nil || r == nil {
		return
	}

	if r.Header == nil {
		r.Header = make(http.Header)
	}

	if targeter, ok := c.(RequestTargeter); ok {
		targeter.SetRequestTarget(r, target)
		return
	}

	if header := c.GetTargetHeader(); header != "" {
		r.Header.Set(header, target)
	}
}

// SetRequestTarget sets the connector's target header to target.
func (x *base) SetRequestTarget(r *http.Request, target string) {
	r.Header.Set(x.targetHeader, target)
}

// SetRequestTarget marks r as an htmx request for target.
func (h *HTMX) SetRequestTarget(r *http.Request, target string) {
	r.Header.Set(h.requestHeader, "true")
	r.Header.Set(h.targetHeader, target)
}

// SetRequestTarget marks r as an htmx 4 partial request for target, which
// htmx 4 sends as tag#id.
func (h *HTMX4) SetRequestTarget(r *http.Request, target string) {
	r.Header.Set(h.requestHeader, "true")
	r.Header.Set(HTMXHeaderRequestType.String(), "partial")
	r.Header.Set(h.targetHeader, "#"+url.PathEscape(target))
}

// Apply sets the response headers of response in the connector's syntax on
// headers, such as HX-Push-Url: false for htmx, in one call:
//
//	connector.Apply(conn, w.Header(), connector.Response{PushURL: "false"})
//
// A ResponseBuilder gives its Response with Value. A nil connector sets
// nothing.
func Apply(c Connector, headers http.Header, response Response) {
	if c == nil || headers == nil {
		return
	}

	ApplyHeaders(headers, c.ResponseHeaders(response))
}

// OOBAttrs returns the attributes that make the client library swap an
// element of a response out of band, into the element of the page with the
// same ID, with swap, such as "beforeend" to add to it ("true" or "" for
// the library's default, which replaces it): hx-swap-oob for htmx, up-hungry
// for Unpoly, whose hungry elements are replaced whatever the swap. Turbo,
// which has no such attribute, and connectors that do not implement
// OOBWriter get hx-swap-oob.
func OOBAttrs(c Connector, swap string) map[string]string {
	if writer, ok := c.(OOBWriter); ok {
		return writer.OOBAttrs(swap)
	}

	return htmxOOBAttrs(swap)
}

func htmxOOBAttrs(swap string) map[string]string {
	if swap == "" {
		swap = "true"
	}

	return map[string]string{"hx-swap-oob": swap}
}

// OOBAttrs writes hx-swap-oob; see the package function OOBAttrs.
func (h *HTMX) OOBAttrs(swap string) map[string]string {
	return htmxOOBAttrs(swap)
}

// OOBAttrs writes up-hungry; Unpoly replaces hungry elements whatever the
// swap. See the package function OOBAttrs.
func (u *Unpoly) OOBAttrs(string) map[string]string {
	return map[string]string{"up-hungry": ""}
}
