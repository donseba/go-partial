# Renderer Roadmap

This branch is moving go-partial toward a centralized render lifecycle so core
can stay small and advanced behavior can move into `exp`.

## Common Ground

- A render is a request-scoped task described by `RenderContext`.
- A renderer can prepare context, produce or wrap HTML, and finalize the result.
- The default core renderer executes the current partial template.
- Official optional behavior should live in `ext` as renderers or
  renderer-backed helpers.
- Experimental behavior should live in `exp`.
- Specialized behavior should become renderers or renderer-backed helpers:
  localization, csrf, pageflow, selection, tabs, actions, debug, errors, target
  resolution, and interactions.

## Target Shape

```go
type Renderer interface {
    Preflight(*RenderContext) (*RenderContext, error)
    InFlight(*RenderContext, RenderNext) (template.HTML, error)
    Postflight(*RenderContext, template.HTML, error) (template.HTML, error)
}
```

Render order:

```text
preflight A -> preflight B -> preflight C
inflight A -> inflight B -> inflight C -> template renderer
postflight C -> postflight B -> postflight A
```

## Migration Checklist

- [x] Add core renderer lifecycle types.
- [x] Add renderer chains to `Partial`, `Service`, and `Layout`.
- [x] Move template execution behind the default template renderer.
- [x] Re-express error rendering as `RenderKindError`.
- [x] Re-express debug rendering as `RenderKindDebug`.
- [x] Move default error rendering to `ext/errors`.
- [x] Move default debug rendering to `ext/debug`.
- [x] Remove old `ErrorRenderer` and `DebugRenderer` function types.
- [x] Re-express target resolution as renderer preflight.
- [x] Move localization helpers to `exp/localization`.
- [x] Move csrf helpers to `exp/csrf`.
- [x] Move selection helper to `exp/selection`.
- [x] Move action/pageflow behavior to `exp/actions` and `exp/pageflow`.
- [x] Move tabs behavior onto `exp/selection` as the shared primitive.
- [x] Refactor `exp/interactions.Renderer` to use the centralized renderer.
- [x] Remove old specialized renderer types once the exp replacements exist.

## Design Notes

- `RenderContext.Values` is the extension point for exp packages.
- `RenderContext.Data` carries kind-specific payloads such as debug values,
  interaction data, or error data.
- `RenderContext.Kind` tells generic renderers which task they are handling.
- `Postflight` may transform the HTML result, not only clean up.
- Pre-release freedom means compatibility shims can be temporary.
