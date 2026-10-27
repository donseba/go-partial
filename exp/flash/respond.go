package flash

import (
	"html/template"
	"net/http"
	"strings"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

// Respond answers a request that is refused, rather than failed to render,
// such as one without the right permission or with an expired form, with
// messages: for a request of the client library of parent's connector, a
// response that adds them to the flash target out of band, with the flash
// template and target of parent's flash Stage, and leaves the request's own
// target as it is (htmx's HX-Reswap: none); for any other request, the
// texts of the messages as text/plain. Either is sent with status and never
// cached. Without a flash stage on parent, exp/flash's default template
// and target are used.
//
//	if !allowed {
//		_ = flash.Respond(w, r, root, http.StatusForbidden, flash.Error("You may not do this."))
//		return
//	}
//
// htmx 2 swaps no response with an error status: send its requests status
// 200, or let its browser client show the text/plain body (see
// ClientScript). parent is not changed.
func Respond(w http.ResponseWriter, r *http.Request, parent *partial.Partial, status int, messages ...Message) error {
	if status == 0 {
		status = http.StatusOK
	}

	conn := parent.Connector()
	if r == nil || !connector.IsRequest(conn, r) {
		return respondText(w, status, messages)
	}

	response := partial.NewID("flash-response").SetParent(parent).Use(respondStage(messages))

	out, err := partial.RenderSelf(r.Context(), r, response)
	if err != nil {
		return err
	}

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	connector.Apply(conn, header, connector.Response{Reswap: "none"})
	w.WriteHeader(status)

	_, err = w.Write([]byte(out))

	return err
}

// respondStage renders messages out of band in place of the response's
// template.
func respondStage(messages []Message) partial.RenderStage {
	return partial.RenderStageHooks{
		Package: packagePath,
		RenderFunc: func(ctx *partial.RenderContext, next partial.RenderNext) (template.HTML, error) {
			if ctx == nil || ctx.Kind != partial.RenderKindPartial || ctx.Partial == nil ||
				ctx.Partial.PartialID() != "flash-response" {
				return next(ctx)
			}

			if out, ok := RenderOOB(ctx, messages...); ok {
				return out, nil
			}

			return renderOOB(ctx, defaultPartial("flash", "default.gohtml"), defaultTargetID, messages), nil
		},
	}
}

func respondText(w http.ResponseWriter, status int, messages []Message) error {
	texts := make([]string, 0, len(messages))
	for _, message := range normalized(messages) {
		texts = append(texts, message.Text)
	}

	header := w.Header()
	header.Set("Content-Type", "text/plain; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	_, err := w.Write([]byte(strings.Join(texts, "\n")))

	return err
}
