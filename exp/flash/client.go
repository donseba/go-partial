package flash

import (
	"bytes"
	_ "embed"
)

// clientScript is the browser client for flash messages; see
// ClientScript.
//
//go:embed client.js
var clientScript []byte

// ClientScript returns the browser client for flash messages. It hides the
// messages exp/flash's templates write after a time that depends on their
// level (success and info 5s, warn 8s, error never; change it with
// data-flash-autohide-<level>="3s" or data-flash-autohide="3s" on an
// ancestor, or on the message itself, "off" to keep it), keeps a message
// while the pointer or keyboard focus is on it, removes it when an element
// in it with data-flash-dismiss is clicked or Escape is pressed in it, and
// dispatches partial:flash on each message as it appears, with its level
// and text.
//
// The messages it shows itself are clones of the <template
// data-flash-template> that {{ flashTemplate }} renders with the
// application's flash template, so they look like those of the server;
// without one, it writes the default markup.
//
// With data-flash-request-error="Something went wrong" on the flash target
// (see WithRequestErrors, FlashTarget and TargetAttrs), requests of
// htmx 2 and 4, Unpoly or Turbo that fail, and exp/sse's partial:error
// events, show that text as an error message. A response with an error
// status that htmx swaps into its target, such as a form with its
// validation errors (422) with htmx 4, is shown by the swap and does not
// count as failed: with htmx 4 the client decides at htmx:finally:request,
// once htmx has swapped the response or not. More attributes on the
// target:
// data-flash-request-error-replace keeps one request error at a time, the
// next request removing it; data-flash-request-error-body shows the
// text/plain body of a refused request (4xx) when it is a sentence, such as
// flash.Respond writes for requests of no client library, without htmx
// swapping it; data-flash-request-error-skip-flash shows nothing for a
// failed response that brings its own message, such as those of
// ext/errors' WithFlash. Scripts call goPartialFlash.show(level,
// text).
//
// The client needs no build step and no other library; serve it from the
// file system of go-partial's clients package, or include it in your own
// bundle.
func ClientScript() []byte {
	return bytes.Clone(clientScript)
}

// ClientName is the name of the browser client in the file system of
// go-partial's clients package.
const ClientName = "partial-flash.js"
