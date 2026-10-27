package errors

import (
	stderrors "errors"
	"net/http"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// Respond answers r for err, an error that happened outside a render, such
// as a form post whose handler failed to save it, the way the Stage of
// parent's tree answers a render that failed: with its error page, its
// error fragment or its error flash message, rendered with parent's
// functions, includes and models, with the text of WithMessage, and written
// with status. parent is the partial whose tree has the Stage, such as the
// root of a site's pages. The response is never cached.
//
//	if err := comments.Add(r.Context(), form); err != nil {
//		log.Error("add comment", "error", err)
//		errors.Respond(w, r, root, http.StatusInternalServerError, err)
//
//		return
//	}
//
// A full-page request gets the error page. A partial request, such as an
// htmx form post, gets the error flash message with WithFlash, out
// of band, leaving the request's target as it is (htmx's HX-Reswap: none).
// Without a flash message it gets the error page for a 5xx status, which
// the interactions client shows as the whole page for a link or form with
// clientAttrs "reloadOnError", and the error fragment for any other
// status, such as 429, which the client library swaps into the target
// where it swaps such answers (htmx 4 does; htmx 2 swaps no error
// response). Data.Status is status, and Data.PartialID and Data.Templates
// are empty: no partial failed. A status of 0 or less is 500.
//
// When parent's tree has no Stage, Respond answers with the built-in
// markup, in ModeSafe. It writes the response itself, as http.Error does:
// the caller writes nothing more.
func Respond(w http.ResponseWriter, r *http.Request, parent *partial.Partial, status int, err error) {
	if w == nil {
		return
	}

	if status <= 0 {
		status = http.StatusInternalServerError
	}

	if err == nil {
		err = stderrors.New(http.StatusText(status))
	}

	if partial.WriteError(w, r, parent, status, err) == nil {
		return
	}

	respondBuiltIn(w, r, parent, status, err)
}

// respondBuiltIn answers an error of Respond with the built-in markup, in
// ModeSafe, for a tree without a Stage.
func respondBuiltIn(w http.ResponseWriter, r *http.Request, parent *partial.Partial, status int, err error) {
	conn := parent.Connector()

	data := Data{
		Error:         err,
		Message:       SafeMessage,
		Title:         http.StatusText(status),
		Status:        status,
		Request:       r,
		TemplateLabel: "Templates",
	}
	if r != nil {
		data.URL = r.URL
	}

	fragment := r != nil && conn.RenderPartial(r) && !pageForPartialRequest(status)

	out, renderErr := renderBuiltIn(data, fragment)
	if renderErr != nil {
		http.Error(w, http.StatusText(status), status)

		return
	}

	header := w.Header()
	partial.AddVary(header, connector.VaryHeaders(conn)...)
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	// The client may have gone; there is no one left to tell.
	_, _ = w.Write([]byte(out))
}
