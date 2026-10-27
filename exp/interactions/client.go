package interactions

import (
	"bytes"
	_ "embed"
)

// clientScript is the browser client of the attributes client libraries
// cannot do themselves; see ClientScript.
//
//go:embed client.js
var clientScript []byte

// ClientScript returns the browser client of the attributes that client
// libraries cannot do themselves: a submit button with
// data-partial-confirm (clientAttrs "submitConfirm", or
// connector.Attr.SubmitConfirm) asks its question before it sends its form,
// with htmx, Unpoly or none. Turbo reads data-turbo-confirm from the button
// itself.
//
// A page with a <dialog data-partial-confirm-dialog> asks these questions,
// and htmx's hx-confirm, in that dialog instead of with window.confirm,
// which some embedded browsers never show. The question goes into the
// dialog's element with data-partial-confirm-question, and a button with
// value="confirm" answers yes; Escape or another button answers no.
//
// A link or form in or with data-partial-reload-on-error (clientAttrs
// "reloadOnError", or connector.Attr.ReloadOnError) whose request gets a
// 5xx status or no answer loads as a whole page instead: a link is
// followed and a GET form submitted again by the browser itself. A POST,
// PUT, PATCH or DELETE is never sent twice: the HTML error page the server
// answered replaces the page; without one, or without an answer, the page
// stays and partial:reload-on-error:failed is dispatched on the link or
// form. It handles the requests of htmx 2 and 4, and those of Unpoly and
// Turbo that get no answer.
//
// The error page written in place of the page keeps the page's
// Content-Security-Policy: under a policy without 'unsafe-inline', the
// page's style-src needs the hash of the error page's inline <style>, such
// as ext/errors' StyleHash for go-partial's built-in error page, or the
// application answers with an error template that has no inline style.
//
// The client needs no build step and no other library; serve it from the
// file system of go-partial's clients package, or include it in your own
// bundle.
func ClientScript() []byte {
	return bytes.Clone(clientScript)
}

// ClientName is the name of the browser client in the file system of
// go-partial's clients package.
const ClientName = "partial-interactions.js"
