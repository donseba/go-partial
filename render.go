package partial

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/donseba/go-partial/connector"
)

// Render renders a partial without an http.Request.
//
// Use Render for tests, offline rendering, and jobs that do not have request
// data. HTTP handlers should usually use Write, or RenderWithRequest when the
// caller needs the HTML string instead of a written response.
func Render(ctx context.Context, p *Partial) (template.HTML, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	ctx, failures := failingRender(ctx, nil, p)
	return renderOutput(renderSelfResult(ctx, nil, p), failures)
}

// RenderSelf renders the partial itself with request data, whatever target
// the request asks for: for a part of a page that reaches the browser
// another way, such as a server-sent event, or a page rendered for a
// request that was made for another one. Templates see the request as they
// do with RenderWithRequest; no out-of-band regions are added.
func RenderSelf(ctx context.Context, r *http.Request, p *Partial) (template.HTML, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	ctx, failures := failingRender(ctx, r, p)
	return renderOutput(renderSelfResult(ctx, r, p), failures)
}

// renderOutput is the result of a render that returns HTML: with the
// FailResponse policy, nothing when a part of it failed.
func renderOutput(result renderResult, failures *renderFailures) (template.HTML, error) {
	if result.Err != nil {
		return result.HTML, result.Err
	}

	if err := failures.err(); err != nil {
		return "", err
	}

	return result.HTML, nil
}

// RenderWithRequest renders a partial using request-aware connector behavior.
//
// When the connector identifies the request as a partial request, this renders
// the requested target and appends eligible out-of-band regions. For normal
// requests it renders the partial itself.
func RenderWithRequest(ctx context.Context, r *http.Request, p *Partial) (template.HTML, error) {
	if p == nil {
		return "", errors.New("partial is not initialized")
	}

	ctx, failures := failingRender(ctx, r, p)
	return renderOutput(renderWithRequestResult(ctx, r, p), failures)
}

func renderWithRequestResult(ctx context.Context, r *http.Request, p *Partial) renderResult {
	if p == nil {
		return renderResult{Err: errors.New("partial is not initialized")}
	}

	if p.getConnectorOrDefault().RenderPartial(r) {
		return renderWithTargetResult(ctx, r, p)
	}

	return renderSelfResult(ctx, r, p)
}

// Write renders a partial and writes the HTTP response.
//
// Write owns the response side of rendering: configured response headers,
// connector response headers, render-stage response metadata, error fragments,
// and out-of-band regions are applied here. The response varies with the
// request headers the connector reads, which Write adds to the Vary header.
// Unless a header sets it, the content type is text/html; charset=utf-8.
//
// The page is rendered before anything is written. With the FailResponse
// policy (see SetFailurePolicy), a failing part of the page fails the
// whole response. When the render fails and an error stage, such as
// ext/errors, renders a failure response, Write writes it, never cached,
// and returns a *FailureResponseError; otherwise it writes nothing and
// returns the error.
func Write(ctx context.Context, w http.ResponseWriter, r *http.Request, p *Partial) error {
	if w == nil {
		return errors.New("response writer is not configured")
	}
	if p == nil {
		_, err := fmt.Fprint(w, "partial is not initialized")
		return err
	}

	addVary(w.Header(), connector.VaryHeaders(p.getConnectorOrDefault()))

	ctx, failures := failingRender(ctx, r, p)
	result := renderWithRequestResult(ctx, r, p)
	if result.Err == nil {
		result.Err = failures.err()
	}

	if result.Err != nil {
		p.emitWithContext(ctx, r, Event{
			Kind:    EventRenderError,
			Level:   EventError,
			Message: "error rendering partial",
			Error:   result.Err,
		})
		return writeRenderFailure(ctx, w, r, p, result.Err)
	}

	headers := result.Headers
	if headers == nil {
		headers = p.getResponseHeaders()
	}
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	for k, v := range p.getConnectorResponseHeaders() {
		w.Header().Set(k, v)
	}
	applyRenderResponseHeaders(w, result.Response)
	addVary(w.Header(), result.Vary)

	if _, set := w.Header()["Content-Type"]; !set {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}

	if result.Response != nil && result.Response.Status > 0 {
		w.WriteHeader(result.Response.Status)
	}

	_, err := writeHTML(w, result.HTML)
	if err != nil {
		p.emitWithContext(ctx, r, Event{
			Kind:    EventRenderWriteError,
			Level:   EventError,
			Message: "error writing partial to response",
			Error:   err,
		})
		return err
	}

	return nil
}

// FailureResponseError is the error Write returns when a render failed and
// Write answered the request with the failure response of an error stage,
// such as ext/errors: the response is written, and the caller must not
// write another. Without an error stage, Write writes nothing and returns
// the render error itself. Err is the render error and Status the status
// written.
//
//	err := partial.Write(ctx, w, r, page)
//	var written *partial.FailureResponseError
//	if err != nil && !errors.As(err, &written) {
//		http.Error(w, "Something went wrong", http.StatusInternalServerError)
//	}
type FailureResponseError struct {
	Status int
	Err    error
}

func (e *FailureResponseError) Error() string {
	return e.Err.Error()
}

func (e *FailureResponseError) Unwrap() error {
	return e.Err
}

// writeRenderFailure answers a failed render with the failure response of
// an error stage, if one renders it. The response is never cached. With
// FailInline, a partial request gets the error fragment with status 200 and
// the out-of-band regions of the ancestors, so the client swaps it in place;
// with FailResponse, the failure response stands alone and keeps an error
// status.
func writeRenderFailure(ctx context.Context, w http.ResponseWriter, r *http.Request, p *Partial, renderErr error) error {
	isPartialRequest := p.isPartialRequest(r)
	failResponse := p.getFailurePolicy() == FailResponse

	result := renderErrorResponse(ctx, r, p, renderErr, errorRender{fragment: isPartialRequest, response: true})
	if result.Err != nil {
		if errors.Is(result.Err, renderErr) {
			return renderErr
		}
		return fmt.Errorf("error rendering failure response: %w; original render error: %v", result.Err, renderErr)
	}

	status := http.StatusInternalServerError

	if isPartialRequest && !failResponse {
		oobOut, vary, oobErr := renderAllAncestorOOBChildren(ctx, r, p, true)
		if oobErr != nil {
			p.emitWithContext(ctx, r, Event{
				Kind:    EventRenderOOBError,
				Level:   EventError,
				Message: "error rendering OOB regions for failure response",
				Error:   oobErr,
			})
			return fmt.Errorf("error rendering OOB regions for failure response: %w; original render error: %v", oobErr, renderErr)
		}
		result.HTML += oobOut
		addVary(w.Header(), vary)
		status = http.StatusOK
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	applyRenderResponseHeaders(w, result.Response)
	w.Header().Set("Cache-Control", "no-store")
	if result.Response != nil && result.Response.Status > 0 {
		if !failResponse || result.Response.Status >= http.StatusBadRequest {
			status = result.Response.Status
		}
	}
	w.WriteHeader(status)

	if _, err := writeHTML(w, result.HTML); err != nil {
		return fmt.Errorf("error writing failure response: %w; original render error: %v", err, renderErr)
	}

	return &FailureResponseError{Status: status, Err: renderErr}
}

// WriteError answers r with the failure response of the error stage of p's
// tree, such as ext/errors, for err, an error that happened outside a
// render, such as a form post whose handler failed to save it: the error
// page, or for a partial request the error fragment or flash message, as
// Write answers a render that failed, written with status. The error
// stage renders with p's functions, includes and models, so an error
// template looks as it does for a render that failed. A status of 0 or
// less is 500, and a nil err the text of the status.
//
//	if err := save(r); err != nil {
//		if werr := partial.WriteError(w, r, root, http.StatusInternalServerError, err); werr != nil {
//			http.Error(w, "Something went wrong", http.StatusInternalServerError)
//		}
//	}
//
// The response is never cached, and varies with the request headers the
// connector reads. WriteError returns nil when it answered the request.
// Otherwise it wrote nothing and returns why: err itself when the tree
// has no error stage that answers it, so the caller answers it another
// way. A failed write of the answer, as to a client that has gone, is
// emitted as an EventRenderWriteError event. ext/errors' Respond answers
// with its built-in markup when no error stage does. The error stage
// renders with the context of r.
func WriteError(w http.ResponseWriter, r *http.Request, p *Partial, status int, err error) error {
	if status <= 0 {
		status = http.StatusInternalServerError
	}

	if err == nil {
		err = errors.New(http.StatusText(status))
	}

	if w == nil {
		return fmt.Errorf("response writer is not configured: %w", err)
	}

	if p == nil {
		return fmt.Errorf("partial is not initialized: %w", err)
	}

	var ctx context.Context
	if r != nil {
		ctx = r.Context()
	}

	ctx, _ = failingRender(ctx, r, p)

	result := renderErrorResponse(ctx, r, p, err, errorRender{
		fragment: p.isPartialRequest(r),
		response: true,
		status:   status,
	})
	if result.Err != nil {
		if errors.Is(result.Err, err) {
			return err
		}

		return fmt.Errorf("error rendering failure response: %w; original error: %v", result.Err, err)
	}

	header := w.Header()
	addVary(header, connector.VaryHeaders(p.getConnectorOrDefault()))
	header.Set("Content-Type", "text/html; charset=utf-8")
	applyRenderResponseHeaders(w, result.Response)
	header.Set("Cache-Control", "no-store")

	if result.Response != nil && result.Response.Status > 0 {
		status = result.Response.Status
	}

	w.WriteHeader(status)

	if _, writeErr := writeHTML(w, result.HTML); writeErr != nil {
		p.emitWithContext(ctx, r, Event{
			Kind:    EventRenderWriteError,
			Level:   EventError,
			Message: "error writing error response",
			Error:   writeErr,
		})
	}

	return nil
}

// writeHTML writes html to w without copying it into a byte slice when w
// takes strings, as net/http's ResponseWriter does.
func writeHTML(w http.ResponseWriter, html template.HTML) (int, error) {
	return io.WriteString(w, string(html))
}

// AddVary adds names to the Vary header, unless it lists them, or "*",
// already, for a response that depends on those request headers. Names
// that are empty are left out.
//
//	partial.AddVary(w.Header(), connector.CurrentURLHeader(conn))
func AddVary(header http.Header, names ...string) {
	if header == nil {
		return
	}

	addVary(header, names)
}

// addVary adds names to the Vary header, unless it lists them already.
func addVary(header http.Header, names []string) {
	if len(names) == 0 {
		return
	}

	vary := header.Values("Vary")
	start := len(vary)
	values := vary[:start:start]

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || varyLists(values, name) {
			continue
		}

		if len(values) == start {
			values = slices.Grow(values, len(names))
		}

		values = append(values, name)
	}

	if len(values) > start {
		header["Vary"] = values
	}
}

// varyLists reports whether the Vary values list name, or "*".
func varyLists(values []string, name string) bool {
	for _, value := range values {
		for value != "" {
			var listed string
			listed, value, _ = strings.Cut(value, ",")

			listed = strings.TrimSpace(listed)
			if listed == "*" || strings.EqualFold(listed, name) {
				return true
			}
		}
	}

	return false
}
