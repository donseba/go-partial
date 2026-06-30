package partial

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
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

	result := renderSelfResult(ctx, nil, p)
	return result.HTML, result.Err
}

// RenderWithRequest renders a partial using request-aware connector behavior.
//
// When the connector identifies the request as a partial request, this renders
// the requested target and appends eligible out-of-band regions. For normal
// requests it renders the partial itself.
func RenderWithRequest(ctx context.Context, r *http.Request, p *Partial) (template.HTML, error) {
	result := renderWithRequestResult(ctx, r, p)
	return result.HTML, result.Err
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
// and out-of-band regions are applied here.
func Write(ctx context.Context, w http.ResponseWriter, r *http.Request, p *Partial) error {
	if w == nil {
		return errors.New("response writer is not configured")
	}
	if p == nil {
		_, err := fmt.Fprint(w, "partial is not initialized")
		return err
	}

	result := renderWithRequestResult(ctx, r, p)
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
	if result.Response != nil && result.Response.Status > 0 {
		w.WriteHeader(result.Response.Status)
	}

	_, err := w.Write([]byte(result.HTML))
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

func writeRenderFailure(ctx context.Context, w http.ResponseWriter, r *http.Request, p *Partial, renderErr error) error {
	isPartialRequest := p.isPartialRequest(r)
	result := renderErrorResult(ctx, r, p, renderErr, isPartialRequest)
	if result.Err != nil {
		if errors.Is(result.Err, renderErr) {
			return renderErr
		}
		return fmt.Errorf("error rendering failure response: %w; original render error: %v", result.Err, renderErr)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	status := http.StatusInternalServerError
	if isPartialRequest {
		oobOut, oobErr := renderAllAncestorOOBChildren(ctx, r, p, true)
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
		status = http.StatusOK
	}
	applyRenderResponseHeaders(w, result.Response)
	if result.Response != nil && result.Response.Status > 0 {
		status = result.Response.Status
	}
	w.WriteHeader(status)
	if _, err := w.Write([]byte(result.HTML)); err != nil {
		return fmt.Errorf("error writing failure response: %w; original render error: %v", err, renderErr)
	}

	return renderErr
}
