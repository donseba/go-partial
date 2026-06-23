# Template Functions And Accessors

This document describes the template-facing helpers and accessors intended for users of `go-partial`.

The goal is to keep templates readable while making the source of each value clear.

## Naming Rules

Template names are split into two groups:

- Reserved underscore accessors: generated accessors for typed struct params, such as `_user` or `_can`.
- Regular helpers: renderer helpers such as `scoped`, `child`, `partial`, `selection`, and `action`. `dict` is a data/map helper, not a composition helper.
- Interaction declarations live on `.Interact`, not `.Data`, when they are configured in Go.

Underscore names are reserved for generated struct/param accessors. User-defined template functions should not start with `_`.

## Quick Reference

| Name | Kind | Purpose |
| --- | --- | --- |
| `_user`, `_can`, etc. | Reserved accessor | Candidate generated accessors for typed render params declared with `@param`. |
| `scoped` | Built-in helper/accessor | Local scope passed by a parent `child` or `partial` call. |
| `child` | Helper | Render a registered child partial by ID. |
| `childIf` | Helper | Render a child only when it has been registered. |
| `partial` | Built-in helper | Render a registered partial from another template using scoped data. |
| `async` | Built-in helper | Render connector-aware deferred loading markup for an endpoint. |
| `reveal` | Built-in helper | Load an endpoint when the region enters the viewport. |
| `poll` | Built-in helper | Refresh an endpoint on an interval. |
| `on` | Built-in helper | Refresh an endpoint when a named browser event is dispatched. |
| `stream` | Built-in helper | Declare a stream-backed region for clients that support it. |
| `prefetch` | Built-in helper | Emit a prefetch hint. |
| `island` | Built-in helper | Create a named lazy server-rendered island. |
| `refresh` | Built-in helper | Render a refresh control for an endpoint or target. |
| `dict` | Built-in data helper | Build a map when a template needs map-style values. |
| `selection` | Helper | Render the selected partial from a `WithSelectMap` selection. |
| `action` | Helper | Render the requested action partial. |
| `oob`, `oobAttr` | Helpers | Detect out-of-band rendering and emit `hx-swap-oob`. |
| `url`, `urlIs`, `urlStarts`, `urlContains`, `urlPath`, `joinPath` | Helpers | Request URL and path helpers. |
| `targetValue`, `selectionValue`, `actionValue` | Helpers | Read current connector target, selection, and action values. |
| `tl`, `tn`, `ctl`, `ctn` | User-owned helpers | Translation helpers supplied by your app through `Service.UseFuncs`. |

## `scoped`

`scoped` exposes the current partial's local data. Today this is the same child-local data available through `.Data`, but `scoped` makes the intent clearer in repeated fragments.

`scoped` returns a shallow copy of the current local data map. This keeps custom template functions from accidentally mutating the partial's `.Data`.

Example parent template:

```gotemplate
{{ range .Rows }}
  {{ partial "users/row" "Row" . }}
{{ end }}
```

Row partial:

```gotemplate
<tr id="row-{{ scoped.Row.ID }}">
  <td>{{ scoped.Row.Name }}</td>
  <td>{{ scoped.Row.Total }}</td>
</tr>
```

`scoped.Row` is local to that row render. The next row gets a different scope.

Rules:

- `scoped` is local, not global.
- `scoped` is bound fresh per render.
- `scoped` returns a shallow copy, not the live `.Data` map.
- `scoped` should not be cached as rendered output.
- `scoped` is best for repeated fragments such as rows, cards, list items, tabs, and inline partials.
- `scoped` should not become a generic dumping ground for application-wide data.

## Generated Param Accessors

Generated param accessors are still being explored.

Contract:

```gotemplate
{{/*
@partial navbar
@param user User
*/}}
```

Candidate template usage:

```gotemplate
<nav>Hello {{ _user.Name }}</nav>
```

The model is:

- `@param user User` declares an inherited render-level param.
- `_user` exposes that param to templates.
- The param is shared through the render tree unless a future API says otherwise.
- `_user` is different from `scoped.Row`: `_user` is inherited render context, `scoped.Row` is local call scope.

Alternative syntax remains possible:

```gotemplate
{{ (param "user").Name }}
```

## `dict`

`dict` builds a map for templates that need one. It is still accepted by `child`, `childIf`, and `partial`, but direct key/value pairs are the preferred scoped-data syntax.

```gotemplate
{{ dict "Row" . "CanEdit" true }}
```

Rules:

- arguments come in key/value pairs
- keys must be strings
- odd argument counts are errors
- values are scoped to the callee

## `partial`

`partial` renders a registered named partial from inside another template using scoped data.

```gotemplate
{{ partial "users/row" "Row" . }}
```

Current behavior:

- first argument identifies a registered partial by its full partial ID, such as `users/row`
- second argument is local scope for `scoped`
- inherited params such as `_user` remain available
- dynamic DOM instances use target resolution rather than becoming separate partial IDs

`partial` is related to `child`, but it is meant for reusable named partials and should use full partial IDs:

```gotemplate
{{ partial "users/row" "Row" . }}
```

`child` remains the short region-style helper. It uses the same scoped-data rule: pass no data, direct key/value pairs, or one map from `dict`.

```gotemplate
{{ child "content" }}
{{ child "sidebar" "User" .Data.User }}
{{ childIf "promo" "Title" "Sale" }}
```

`child`, `childIf`, and `partial` all pass local data into the rendered partial, so the callee can read it through `scoped`:

```gotemplate
{{ scoped.Row }}
```

For repeated fragments:

```text
template identity: users/row
DOM target:        row-2
```

An HTMX request with `HX-Target: row-2` should resolve to:

```text
partial: users/row
scope:   Row = current row 2
```

Use `WithTargetResolver` for that mapping:

```go
rowPartial := partial.NewID("users/row", "templates/users/row.gohtml")

table.With(rowPartial)
table.WithTargetResolver(func(ctx context.Context, r *http.Request, target string) (*partial.Partial, map[string]any, bool) {
    if !strings.HasPrefix(target, "row-") {
        return nil, nil, false
    }
    row := loadCurrentRow(target)
    return rowPartial, map[string]any{"Row": row}, true
})
```

Static child IDs are checked first. The target resolver is only used when the requested DOM target does not match a registered child partial ID.

## `child`

`child` renders a registered child partial by ID.

```gotemplate
{{ child "navbar" }}
```

This remains the best fit for named page regions where the partial ID and DOM target are the same:

```html
<nav id="navbar">
```

```text
HX-Target: navbar -> render child "navbar"
```

## Interaction Helpers

Interaction helpers render connector-aware loading or request markup for endpoints. The helper creates an interaction model, the active connector supplies protocol attributes, and the interaction renderer owns the final HTML wrapper.

```gotemplate
{{ async "/stats" }}
{{ reveal "/charts/monthly" }}
{{ on "cart:changed" "/cart/summary" }}
{{ stream "/activity/events" }}
{{ prefetch "/users/42" }}
```

For configured interactions, build the value in Go and pass it through the partial's interaction context. This keeps client behavior declarations separate from content data.

```go
content.SetInteractions(map[string]any{
    "Stats": partial.Async("/stats").
        ID("stats-loader").
        Target("#stats").
        Placeholder(""),

    "Notifications": partial.Poll("/notifications").
        Every(10 * time.Second),

    "CartChanged": partial.On("cart:changed", "/cart/summary").
        Target("#cart"),

    "Cart": partial.Island("cart", "/cart/summary"),

    "CartRefresh": partial.Refresh("/cart/summary").
        Target("#cart").
        Swap(partial.SwapOuterHTML).
        Placeholder("Refresh cart"),
})
```

```gotemplate
{{ async .Interact.Stats }}
{{ poll .Interact.Notifications }}
{{ on .Interact.CartChanged }}
{{ island .Interact.Cart }}
{{ refresh .Interact.CartRefresh }}
```

With the HTMX connector, the first call renders markup shaped like:

```html
<div id="async-stats"
     hx-get="/stats"
     hx-trigger="load"
     hx-target="#async-stats"
     hx-swap="innerHTML">
  Loading...
</div>
```

Route parameters use `:name` placeholders. Prefer `Param` in Go for configured interactions:

```go
row := partial.Async("/table/row/:row").Param("row", row.ID)
```

The simple template form still accepts direct key/value pairs for route placeholders when that reads better in a loop:

```gotemplate
{{ async "/table/row/:row" "row" .Data.Row.ID }}
```

The default renderer emits simple unstyled HTML. Applications can replace it with `SetInteractionRenderer` on a service, layout, or partial.

`on` listens for a browser event and refreshes its target when that event is dispatched. With HTMX, custom events default to `from:body`, so application code can dispatch `document.body.dispatchEvent(new CustomEvent("cart:changed"))`.

`stream` only declares the stream region and connector attributes. The client still needs a compatible SSE integration, and the endpoint must emit events that integration understands. `prefetch` is intentionally non-visual; the default renderer emits a `<link rel="prefetch">` hint.

Interaction helpers are deferred client-side loading, not blocking server-side execution. Use `partial` or `child` when the current render should include the child immediately.

## `selection`

`selection` renders one partial from a `WithSelectMap` registration. The selected key comes from the active connector, for example `X-Select` when using the HTMX connector.

```go
content.WithSelectMap("summary", map[string]*partial.Partial{
    "summary": partial.NewID("summary", "summary.gohtml"),
    "details": partial.NewID("details", "details.gohtml"),
})
```

```gotemplate
{{ selection }}
```

## `oob` And `oobAttr`

Use `oob` inside out-of-band partial templates to check whether the partial is being rendered as OOB output. Use `oobAttr` to emit HTMX's `hx-swap-oob` attribute only during OOB rendering.

```gotemplate
<aside id="toast"{{ oobAttr }}>
  {{ .Data.Message }}
</aside>
```

Normal render output:

```html
<aside id="toast">
  Saved
</aside>
```

OOB render output:

```html
<aside id="toast" hx-swap-oob="true">
  Saved
</aside>
```

Pass an explicit HTMX swap value only when you need one:

```gotemplate
<aside id="toast"{{ oobAttr "outerHTML" }}>
```

## URL And Connector Helpers

URL helpers read from the current request:

```gotemplate
{{ if urlIs "/settings" }}active{{ end }}
{{ joinPath .BasePath "users" }}
{{ urlPath }}
```

Connector helpers expose the active target, selection, and action values:

```gotemplate
{{ targetValue }}
{{ selectionValue }}
{{ actionValue }}
```

## Translation Helpers

Translation helpers are not built into go-partial. The renderer exposes `.Loc` from the request context, and your app can add functions such as `tl`, `tn`, `ctl`, and `ctn`.

```go
service.UseFuncs(translator.FuncMap())
```

```gotemplate
{{ tl .Loc "Hello, World!" }}
{{ tn .Loc "You have one message." "You have %d messages." 5 5 }}
{{ ctl .Loc "button" "save" }}
```

`github.com/donseba/go-translator` already exposes `FuncMap()` with this style of helpers.

## Cache Boundary

Template helpers may use cached parsed templates, but they must bind request-specific values fresh per render.

Safe to cache:

- parsed templates
- contracts
- dependency metadata
- target resolver registrations

Not safe to cache by default:

- rendered HTML
- `scoped` values
- `_user` or other param values
- dynamic target lookup results

Safe render flow:

```text
resolve target -> load current data -> clone parsed template -> bind accessors -> execute
```
