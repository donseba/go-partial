# Template Functions And Accessors

This document describes the template-facing helpers intended for users of `go-partial`.

The current model is intentionally close to normal Go templates:

- Use native `{{ template "row.gohtml" . }}` for typed composition.
- Use go-doc annotations such as `@model` and `@dot` so editors understand the values.
- Use `content` inside layout wrappers to output the route content partial.
- Use `partial` only when you intentionally want to render a template path through go-partial's render path.
- Use interaction helpers such as `async`, `poll`, and `refresh` for client-side delivery behavior.

## Naming Rules

Avoid user-defined helper or model names that collide with Go template actions or go-partial helpers, such as `range`, `if`, `len`, `ctx`, `request`, `url`, `locale`, `csrf`, `content`, `partial`, `selection`, `action`, `flash`, `flashTarget`, `flashes`, and `hasFlashes`.

When a template uses `SetDot`, request-specific values are still available through helper functions instead of fields on dot.

## Quick Reference

| Name | Kind | Purpose |
| --- | --- | --- |
| `content` | Layout helper | Render the content partial configured with `layout.Set(content).Wrap(wrapper)`. |
| `partial` | Composition helper | Render a template path through go-partial's render path. Prefer native `template` for typed rows. |
| `selection` | Helper | Render the selected partial from a `selection.WithSelectMap` registration. |
| `action` | Helper | Render the partial returned by an action callback. |
| `flash` | Helper | Render request-scoped flash messages from `exp/flash`. |
| `flashTarget` | Helper | Render the stable target container used by flash message templates. |
| `flashes`, `hasFlashes` | Helper | Read request-scoped flash messages for custom markup. |
| `async` | Interaction helper | Render connector-aware deferred loading markup for an endpoint. |
| `reveal` | Interaction helper | Load an endpoint when the generated area enters the viewport. |
| `poll` | Interaction helper | Refresh an endpoint on an interval. |
| `on` | Interaction helper | Refresh an endpoint when a named browser event is dispatched. |
| `stream` | Interaction helper | Declare a stream-backed listener for clients that support it. |
| `prefetch` | Interaction helper | Emit a prefetch hint. |
| `refresh` | Interaction helper | Render a refresh control for an endpoint or target. |
| `dict` | Data helper | Build a map when a template needs map-style values. |
| `oob`, `oobAttr` | Connector helpers | Detect out-of-band rendering and emit `hx-swap-oob`. |
| `ctx`, `request`, `url`, `locale`, `csrf`, `basePath` | Request helpers | Read request-aware values while dot remains your app model. |
| `urlIs`, `urlStarts`, `urlContains`, `urlPath`, `joinPath` | URL helpers | Read and compare request paths. |
| `targetValue`, `selectionValue`, `actionValue` | Connector helpers | Read current connector target, selection, and action values. |

Translation helpers such as `tl`, `tn`, `ctl`, and `ctn` are not built in. Add them through `Service.SetFunc`, `Layout.SetFunc`, or `Partial.SetFunc`.

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
target.WithResolver(table, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
    if !strings.HasPrefix(target, "row-") {
        return nil, false
    }

    row := loadCurrentRow(target)
    return partial.NewID(target, "templates/row.gohtml").SetDot(row), true
})
```

That gives you one template for three modes: inside a parent render, as a standalone render, and as an HTMX target response.

## Layout Content

When a layout wraps a content partial, the wrapper renders that configured route partial with `content`:

```gotemplate
<main>
    {{ content }}
</main>
```

Use native `template` for typed composition inside the content partial. Register partials in Go when they also need to be renderable as HTMX targets or OOB output.

## go-doc `@model` Contracts With `SetModel`

Use `SetDot` when the whole template root is one app value. Use `@model` plus `SetModel` when you want named root functions in the template:

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
    SetModel(page, user)
```

The template owns the names `Page` and `User`. go-partial scans the go-doc contract and matches controller values by type instead of asking you to repeat the names in Go.

## Request Helpers With Dot Templates

When `SetDot` is used, `.` belongs to your app model. Request-specific data remains available through helpers:

```gotemplate
{{ ctx.URL.Path }}
{{ request.Method }}
{{ locale }}
{{ csrf.Key }}
{{ basePath }}
```

`ctx` returns the active `partial.RenderContext`. Request helpers such as `request`, `url`, `locale`, `csrf`, and `basePath` are installed by the active renderer chain.

## `partial`

`partial` renders a template path through go-partial's render path. This is useful when you want to render another template with request helpers, model registration, extension error handling, and the configured filesystem/cache behavior, but you do not want to make that template part of the native parse tree.

```gotemplate
{{ partial runtime "templates/notice.gohtml" .Notice }}
```

```gotemplate
{{/*
@dot github.com/example/app.Notice
*/}}

<aside>{{ .Message }}</aside>
```

Arguments:

- `{{ partial runtime "templates/card.gohtml" }}` renders the path with the current partial context.
- `{{ partial runtime "templates/card.gohtml" .Card }}` renders the path with `.Card` as dot.
- `{{ partial runtime "templates/card.gohtml" "Title" "Hello" }}` renders the path with a small dot map, so the callee reads `{{ .Title }}`.

For rows and larger fragments, prefer native `template` plus `@dot`, because that gives go-doc the strongest type information. Use `partial` when the nested render should go through go-partial itself.

## `dict`

`dict` builds a map for templates that need one.

```gotemplate
{{ partial runtime "templates/notice.gohtml" (dict "Message" .FlashMessage "Tone" "success") }}
```

Rules:

- arguments come in key/value pairs
- keys must be strings
- odd argument counts are errors
- when passed as one argument to `partial`, the map becomes the callee's dot value

## Flash Helpers

Flash helpers live in `github.com/donseba/go-partial/exp/flash` and are opt-in:

```go
service.SetFunc(flash.FuncMap())
service.Use(flash.Renderer())
```

Add messages to the request context before rendering:

```go
ctx := flash.Add(r.Context(), flash.Success("Saved"))
_ = layout.WriteWithRequest(ctx, w, r.WithContext(ctx))
```

Render them with the embedded default template:

```gotemplate
{{ flash }}
```

Render a stable destination in a layout when fragments should append messages
out-of-band:

```gotemplate
{{ flashTarget }}
```

Use `flashes` and `hasFlashes` when a template wants to own the markup directly:

```gotemplate
{{ if hasFlashes }}
    {{ range flashes }}
        {{ .Level }}: {{ .Text }}
    {{ end }}
{{ end }}
```

Applications can override the default message template with `flash.WithTemplate(...)`
or `flash.WithPartial(...)`, and the target with `flash.WithTargetTemplate(...)`,
`flash.WithTargetPartial(...)`, or `flash.WithTargetID(...)`.
Custom levels and target IDs are normalized into lowercase CSS-friendly tokens
before templates receive them.

## Interaction Helpers

Interaction helpers render connector-aware loading or request markup for endpoints. The active connector supplies protocol attributes, and the interaction renderer owns the final HTML wrapper.

Interaction helpers live in `github.com/donseba/go-partial/exp/interactions`
and are opt-in. Register them explicitly:

```go
import "github.com/donseba/go-partial/exp/interactions"

service.SetFunc(interactions.FuncMap())
```

Those functions carry `go-doc:sig` comments for overloads such as endpoint
strings and named interactions. Add them through `.go-doc/config.json` as
`templateFunctions` so editors understand both helper forms:

```gotemplate
{{ async runtime "/stats" }}
{{ reveal runtime "/charts/monthly" }}
{{ poll runtime "/notifications" }}
{{ on runtime "cart:changed" "/cart/summary" }}
{{ stream runtime "/activity/events" }}
{{ prefetch runtime "/users/42" }}
{{ refresh runtime "/cart/summary" }}
```

The endpoint form is ideal when the route belongs directly to the template. For
example, repeated rows can pass route placeholders from the current dot value:

```gotemplate
{{ range .Rows }}
    {{ async runtime "/async/row/:row" "row" .ID }}
{{ end }}
```

For configured interactions, declare named `@interaction` roots and register matching values from Go:

```gotemplate
{{/*
@interaction Stats github.com/donseba/go-partial/exp/interactions.Interaction
@interaction Notifications github.com/donseba/go-partial/exp/interactions.Interaction
@interaction CartChanged github.com/donseba/go-partial/exp/interactions.Interaction
@interaction CartRefresh github.com/donseba/go-partial/exp/interactions.Interaction
*/}}

{{ async runtime Stats }}
{{ poll runtime Notifications }}
{{ on runtime CartChanged }}
{{ refresh runtime CartRefresh }}
```

```go
content.SetContract("interaction",
    interactions.NewAsync("/stats").As("Stats").ID("stats-loader").Target("#stats"),
    interactions.NewPoll("/notifications").As("Notifications").Every(10*time.Second),
    interactions.NewOn("cart:changed", "/cart/summary").As("CartChanged").Target("#cart"),
    interactions.NewRefresh("/cart/summary").As("CartRefresh").Target("#cart").Swap(interactions.SwapOuterHTML),
)
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
row := interactions.NewAsync("/table/row/:row").Param("row", row.ID)
```

```gotemplate
{{ async runtime "/table/row/:row" "row" .ID }}
```

Interaction helpers are deferred client-side loading, not blocking server-side execution. Use native `template` or `partial` when the current server render should include the markup immediately.

## `selection`

`selection` renders one partial from a `selection.WithSelectMap` registration. The selected key comes from the active connector, for example `X-Select` when using the HTMX connector.

```go
selection.WithSelectMap(content, "summary", map[string]*partial.Partial{
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

URL, runtime, content, request/context, and OOB helpers stay in core because
they are tied directly to the render path. Connector helpers such as
`targetValue`, `selectionValue`, and `actionValue` are installed by their
`exp` renderers when the application opts into those packages.

Generic helpers such as strings, maps, dates, and counters are convenience
helpers. Register them from the optional provider when you want them:

```go
import "github.com/donseba/go-partial/exp/templatehelpers"

service.SetFunc(templatehelpers.FuncMap())
```

Use narrower maps when you want a smaller helper surface:

```go
service.SetFunc(
    templatehelpers.StringFuncMap(),
    templatehelpers.CollectionFuncMap(),
)
```

`templatehelpers.HTMLFuncMap()` is separate because helpers such as `safeHTML`
mark content as trusted HTML.

## Translation Helpers

Translation helpers are user-owned. The localization renderer exposes `localizer` and `locale`, and your app can add functions such as `tl`, `tn`, `ctl`, and `ctn`.

```go
service.SetFunc(localization.FuncMap(), translator.FuncMap())
service.Use(localization.Renderer())
```

```gotemplate
{{ tl localizer "Hello, World!" }}
{{ tn localizer "You have one message." "You have %d messages." 5 5 }}
{{ ctl localizer "button" "save" }}
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
- request data and application values
- dynamic target lookup results

Safe render flow:

```text
resolve target -> load current data -> clone parsed template -> bind request helpers -> execute
```

