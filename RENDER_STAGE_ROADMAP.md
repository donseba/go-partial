# Render Stage Roadmap

This branch is moving go-partial toward a centralized render lifecycle so core
can stay small and advanced behavior can move into `exp`.

## Common Ground

- A render is a request-scoped task described by `RenderContext`.
- A render stage can prepare context, produce or wrap HTML, and finalize the result.
- The default core render stage executes the current partial template.
- Official optional behavior should live in `ext` as render stages or
  stage-backed helpers.
- Experimental behavior should live in `exp`.
- Specialized behavior should become render stages or stage-backed helpers:
  localization, csrf, pageflow, selection, tabs, actions, debug, errors, target
  resolution, and interactions.

## Target Shape

```go
type RenderStage interface {
    Prepare(*RenderContext) (*RenderContext, error)
    Render(*RenderContext, RenderNext) (template.HTML, error)
    Finalize(*RenderContext, template.HTML, error) (template.HTML, error)
}
```

Render order:

```text
prepare A -> prepare B -> prepare C
render A -> render B -> render C -> template render stage
finalize C -> finalize B -> finalize A
```

## Migration Checklist

- [x] Add core render stage lifecycle types.
- [x] Add render stage chains to `Partial`.
- [x] Move template execution behind the default template render stage.
- [x] Re-express error rendering as `RenderKindError`.
- [x] Re-express debug rendering as `RenderKindDebug`.
- [x] Move default error rendering to `ext/errors`.
- [x] Move default debug rendering to `ext/debug`.
- [x] Remove old `ErrorRenderer` and `DebugRenderer` function types.
- [x] Re-express target resolution as render stage prepare work.
- [x] Move localization helpers to `exp/localization`.
- [x] Move csrf helpers to `exp/csrf`.
- [x] Move selection helper to `exp/selection`.
- [x] Move action/pageflow behavior to `exp/actions` and `exp/pageflow`.
- [x] Move tabs behavior onto `exp/selection` as the shared primitive.
- [x] Refactor `exp/interactions.Stage` to use the centralized render stage lifecycle.
- [x] Remove old specialized render stage types once the exp replacements exist.
- [x] Add slot-backed child partials for lifecycle-aware composition.
- [x] Add metrics sinks, writer output, fanout, request IDs, trace IDs, and
  parent request IDs.
- [x] Keep default error markup in `ext/errors`; core only asks the render stage
  chain for a failure response.
- [x] Make `RenderResponse` real for `Write` so render stages can set
  generic status and headers without templates controlling HTTP response state.

## Design Notes

- `RenderContext.Values` is the extension point for exp packages.
- `RenderContext.Data` carries kind-specific payloads such as debug values,
  interaction data, or error data.
- `RenderContext.Kind` tells generic render stages which task they are handling.
- `Finalize` may transform the HTML result, not only clean up.
- Pre-release API cleanup is complete before v1.
- `RenderResponse` belongs to core as generic render metadata. Templates do not
  set it; Render stages may set it, and only `Write` applies it.
- Slots are structured child partials. Native `{{ template }}` remains the
  right tool for local typed component composition that does not need a partial
  lifecycle.
- `exp/interactions` is positioned as experimental helper sugar for connector
  attributes and wrapper markup, not as core rendering behavior.
- Diagnostic events should become the replacement for core-owned logging. See
  `DIAGNOSTIC_EVENTS.md` for the guideline.

## Later Discussion

- FuncMap and stage registration ergonomics. Current usage is explicit but
  verbose when an application opts into several `exp` and `ext` packages.
- Audit `exp/interactions` after more real application usage. It may remain as
  helper sugar, or shrink further if actions, selection, slots, and SSE cover
  the common cases.
