# Template Functions And Accessors

This document describes the template-facing helpers intended for users of `go-partial`.

The current model is intentionally close to normal Go templates:

- Use native `{{ template "row.gohtml" . }}` for typed composition.
- Use go-doc annotations such as `@model` and `@dot` so editors understand the values.
- Use `slot` for named regions that are registered on a partial tree and can also be addressed by an HTMX-style target.
- Use interaction helpers such as `async`, `poll`, and `refresh` for client-side delivery behavior.

## Naming Rules

Avoid user-defined helper or model names that collide with Go template actions or go-partial helpers, such as `range`, `if`, `len`, `ctx`, `request`, `url`, `locale`, `csrf`, `slot`, `partial`, `scoped`, `selection`, and `action`.

When a template uses `SetDot`, request-specific values are still available through helper functions instead of fields on dot.

## Quick Reference

| Name | Kind | Purpose |
| --- | --- | --- |
| `slot` | Composition helper | Render a registered region by ID without passing ad-hoc values. |
| `partial` | Composition helper | Render a registered partial with local scope values. Prefer native `template` for typed rows. |
| `scoped` | Accessor | Read the local scope passed by `partial`. |
| `selection` | Helper | Render the selected partial from a `WithSelectMap` registration. |
| `action` | Helper | Render the partial returned by an action callback. |
| `async` | Interaction helper | Render connector-aware deferred loading markup for an endpoint. |
| `reveal` | Interaction helper | Load an endpoint when the region enters the viewport. |
| `poll` | Interaction helper | Refresh an endpoint on an interval. |
| `on` | Interaction helper | Refresh an endpoint when a named browser event is dispatched. |
| `stream` | Interaction helper | Declare a stream-backed region for clients that support it. |
| `prefetch` | Interaction helper | Emit a prefetch hint. |
| `refresh` | Interaction helper | Render a refresh control for an endpoint or target. |
| `dict` | Data helper | Build a map when a template needs map-style values. |
| `oob`, `oobAttr` | Connector helpers | Detect out-of-band rendering and emit `hx-swap-oob`. |
| `ctx`, `request`, `url`, `locale`, `csrf`, `basePath` | Request helpers | Read request-aware values while dot remains your app model. |
| `urlIs`, `urlStarts`, `urlContains`, `urlPath`, `joinPath` | URL helpers | Read and compare request paths. |
| `targetValue`, `selectionValue`, `actionValue` | Connector helpers | Read current connector target, selection, and action values. |

Translation helpers such as `tl`, `tn`, `ctl`, and `ctn` are not built in. Add them through `Service.UseFuncs`, `Layout.UseFuncs`, or `Partial.UseFuncs`.

## Typed Template Composition

For rows, cards, nested sections, and reusable fragments, prefer native Go template composition. This is the path go-doc understands best.

Parent template:

```gotemplate
{{/*
@dot github.com/example/app.TablePage
*/}}

{{ range .Rows }}
    {{ template "row.gohtml" . }}
{{ end }}
```

Row template:

```gotemplate
{{/*
@dot github.com/example/app.Row
*/}}

<tr id="row-{{ .ID }}">
    <td>{{ .Name }}</td>
</tr>
```

The same row template can still be rendered as an HTMX target by registering a partial and resolving dynamic target IDs:

```go
rowPartial := partial.NewID("row", "templates/row.gohtml")

table.With(rowPartial)
table.WithTargetResolver(func(ctx context.Context, r *http.Request, target string) (*partial.Partial, map[string]any, bool) {
    if !strings.HasPrefix(target, "row-") {
        return nil, nil, false
    }

    row := loadCurrentRow(target)
    return partial.NewID(target, "templates/row.gohtml").SetDot(row), nil, true
})
```

That gives you one template for three modes: inside a parent render, as a standalone render, and as an HTMX target response.

## `slot`

`slot` renders a registered region by ID:

```gotemplate
<main>
    {{ slot "content" }}
</main>
```

Use it for layout regions, selected regions, out-of-band regions, and error-boundary sections where the ID is part of the partial tree. `slot` does not accept local key/value data. If a fragment needs local typed data, use native `template` instead.

## go-doc `@model` Contracts With `UseModels`

Use `SetDot` when the whole template root is one app value. Use `@model` plus `UseModels` when you want named model functions in the template:

```gotemplate
{{/*
@model Page github.com/example/app.Page
@model User github.com/example/app.User
*/}}

<h1>{{ Page.Title }}</h1>
<p>{{ User.Name }}</p>
```

```go
content := partial.NewID("content", "templates/page.gohtml").
    UseModels(page, user)
```

The template owns the names `Page` and `User`. go-partial delegates type matching to `github.com/donseba/go-doc/renderer`, so the controller passes values by type instead of repeating the names.

## Request Helpers With Dot Templates

When `SetDot` is used, `.` belongs to your app model. Request-specific data remains available through helpers:

```gotemplate
{{ ctx.Locale }}
{{ ctx.URL.Path }}
{{ request.Method }}
{{ locale }}
{{ csrf.Key }}
{{ basePath }}
```

`ctx` returns a `partial.RenderContext` containing `Context`, `Request`, `URL`, `Loc`, `Locale`, `Csrf`, and `BasePath`.

## `partial` And `scoped`

`partial` renders a registered partial and passes a small local scope into it. The callee reads that scope through `scoped`.

```gotemplate
{{ partial "notice" "Message" .FlashMessage }}
```

```gotemplate
<aside>{{ scoped.Message }}</aside>
```

Use this for small component-style fragments where a map handoff is acceptable. For rows and larger fragments, prefer native `template` plus `@dot`, because that gives go-doc a real type contract.

Rules:

- `scoped` is local to the current `partial` call.
- `scoped` returns a shallow copy of the local data map.
- `scoped` should not be used as application-wide data storage.
- `partial` identifies registered partials by full partial ID.

## `dict`

`dict` builds a map for templates that need one.

```gotemplate
{{ partial "notice" (dict "Message" .FlashMessage "Tone" "success") }}
```

Rules:

- arguments come in key/value pairs
- keys must be strings
- odd argument counts are errors
- values are scoped to the callee

## Interaction Helpers

Interaction helpers render connector-aware loading or request markup for endpoints. The active connector supplies protocol attributes, and the interaction renderer owns the final HTML wrapper.

```gotemplate
{{ async "/stats" }}
{{ reveal "/charts/monthly" }}
{{ poll "/notifications" }}
{{ on "cart:changed" "/cart/summary" }}
{{ stream "/activity/events" }}
{{ prefetch "/users/42" }}
{{ refresh "/cart/summary" }}
```

For configured interactions, build the value in Go and pass it through the partial's interaction context:

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

    "Cart": partial.Async("/cart/summary").
        ID("cart").
        Target("#cart"),

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
{{ async .Interact.Cart }}
{{ refresh .Interact.CartRefresh }}
```

With the HTMX connector, `async` renders markup shaped like:

```html
<div id="async-stats"
     hx-get="/stats"
     hx-trigger="load"
     hx-target="#async-stats"
     hx-swap="innerHTML">
  Loading...
</div>
```

Route parameters use `:name` placeholders:

```go
row := partial.Async("/table/row/:row").Param("row", row.ID)
```

```gotemplate
{{ async "/table/row/:row" "row" .ID }}
```

Interaction helpers are deferred client-side loading, not blocking server-side execution. Use native `template`, `slot`, or `partial` when the current server render should include the markup immediately.

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

Use `oob` inside out-of-band templates to check whether the partial is being rendered as OOB output. Use `oobAttr` to emit HTMX's `hx-swap-oob` attribute only during OOB rendering.

```gotemplate
<aside id="toast"{{ oobAttr }}>
  Saved
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
{{ joinPath basePath "users" }}
{{ urlPath }}
```

Connector helpers expose the active target, selection, and action values:

```gotemplate
{{ targetValue }}
{{ selectionValue }}
{{ actionValue }}
```

## Translation Helpers

Translation helpers are user-owned. The renderer exposes `.Loc` from the request context, and your app can add functions such as `tl`, `tn`, `ctl`, and `ctn`.

```go
service.UseFuncs(translator.FuncMap())
```

```gotemplate
{{ tl .Loc "Hello, World!" }}
{{ tn .Loc "You have one message." "You have %d messages." 5 5 }}
{{ ctl .Loc "button" "save" }}
```

`github.com/donseba/go-translator` already exposes `FuncMap()` with this helper style.

## Cache Boundary

Template helpers may use cached parsed templates, but request-specific values are bound fresh per render.

Safe to cache:

- parsed templates
- dependency metadata
- target resolver registrations

Not safe to cache by default:

- rendered HTML
- `scoped` values
- request data and application values
- dynamic target lookup results

Safe render flow:

```text
resolve target -> load current data -> clone parsed template -> bind request helpers -> execute
```
