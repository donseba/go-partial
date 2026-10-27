# Template Functions And Accessors

This document describes the template-facing helpers intended for users of `go-partial`.

The current model is intentionally close to normal Go templates:

- Use native `{{ template "row.gohtml" . }}` for typed composition.
- Use go-clue annotations such as `@model` and `@dot` so editors understand the values.
- Use `content` inside wrapper partials to output the route content child.
- Use `partial` only when you intentionally want to render a template path through go-partial's render path.
- Use interaction helpers such as `async`, `poll`, and `refresh` for client-side delivery behavior.

## Naming Rules

Avoid user-defined helper or model names that collide with Go template actions or go-partial helpers, such as `range`, `if`, `len`, `ctx`, `request`, `url`, `locale`, `csrf`, `content`, `partial`, `selection`, `action`, `flash`, `flashTarget`, `flashes`, `hasFlashes`, `flashTemplate`, `flashTargetAttrs`, `slot`, `hasSlot`, `requiredSlot`, `deferredSlot`, `runtime`, `oobAttr`, and `oobSwap`.

When a template uses `SetDot`, request-specific values are still available through helper functions instead of fields on dot.

`partial.ReservedFuncNames(stages...)` lists the names templates cannot be given with `SetFunc`: go-partial's own helpers and the functions the given stages give templates. Stages give them with `RenderStageHooks.ResolveFunc`, made only for the templates that call them. `partial.IsReservedFuncName(name, stages...)` also covers names starting with an underscore.

go-partial's own helpers are reserved always: `runtime`, `partial`, `content`, `ctx`, `request`, `url`, `basePath`, `urlIs`, `urlStarts`, `urlContains`, `urlPath`, `joinPath`, `oob`, `oobAttr` and `oobSwap`; a template that declares a typed root with one of these names fails to render. The helpers of a stage, such as `locale`, `csrf`, `slot`, `flash` or `targetIs`, are reserved while the stage is in use.

## Functions With A Context

A registered function whose first parameter is a `context.Context` receives the context of the render, and templates call it without that parameter:

```go
root.SetFunc(template.FuncMap{
    "posts": func(ctx context.Context, section string, limit int) ([]Post, error) {
        return store.Latest(ctx, section, limit)
    },
})
```

```gotemplate
{{ range posts "news" 3 }}{{ .Title }}{{ end }}
```

The function is bound once per render tree, also when many partials call it. A function of the same name without a context, registered on a nearer partial, replaces it. Use `partial.BindContext(ctx, funcs...)` to bind such functions for templates go-partial does not execute.

`partial.RuntimeFrom(ctx)` returns the runtime of the partial whose template calls the function, so a function that renders a component renders it as a child of that partial, with its models, functions and stages:

```go
"card": func(ctx context.Context, card Card) (template.HTML, error) {
    runtime, ok := partial.RuntimeFrom(ctx)
    if !ok {
        return "", errors.New("card: not rendering")
    }
    return runtime.RenderPartial(partial.NewID("card", "card.gohtml").SetDot(card))
},
```

```gotemplate
{{ card .Featured }}
```

## Quick Reference

| Name | Kind | Purpose |
| --- | --- | --- |
| `runtime` | Core helper | The render's `*partial.Runtime`, the first argument of `partial`, the interaction helpers and `{{ debug runtime . }}`. |
| `content` | Content helper | Render the content child configured with `root.SetContent(content)`. |
| `partial` | Composition helper | Render a template path through go-partial's render path. Prefer native `template` for typed rows. |
| `selection` | Helper | Render the selected partial from a `selection.WithSelectMap` registration (`exp/selection`). |
| `action` | Helper | Render the partial returned by an action callback (`exp/actions`). |
| `flash` | Helper | Render request-scoped flash messages from `exp/flash`. |
| `flashTarget` | Helper | Render the stable target container used by flash message templates. |
| `flashes`, `hasFlashes` | Helper | Read request-scoped flash messages for custom markup. |
| `flashTemplate` | Helper | Render the flash template as `<template data-flash-template>` per level, which the browser client clones; nothing in an out-of-band render. |
| `flashTargetAttrs` | Helper | The id of the flash target and the request error attributes of `flash.WithRequestErrors`, for an element of the template's own; a text argument replaces the stage's. |
| `async` | Interaction helper | Render connector-aware deferred loading markup for an endpoint. |
| `reveal` | Interaction helper | Load an endpoint when the generated area enters the viewport. |
| `poll` | Interaction helper | Refresh an endpoint on an interval. |
| `on` | Interaction helper | Refresh an endpoint when a named browser event is dispatched. |
| `stream` | Interaction helper | Declare a stream-backed listener for clients that support it. |
| `prefetch` | Interaction helper | Emit a prefetch hint. |
| `refresh` | Interaction helper | Render a refresh control for an endpoint or target. |
| `asyncAttrs`, `revealAttrs`, `pollAttrs`, `onAttrs`, `streamAttrs`, `refreshAttrs` | Interaction helpers | The attributes of an interaction, for an element of the template's own. |
| `submitAttrs`, `liveAttrs` | Interaction helpers | The attributes of a form that posts and swaps the response, and of an input that requests as the user types. |
| `clientAttrs`, `noBoost`, `confirmAttr` | Interaction helpers | Connector-neutral attributes of a link, form, submit button or page: a confirmation, boosting on or off, a target and swap, inherited settings, a reload on server errors. |
| `slot`, `hasSlot`, `requiredSlot`, `deferredSlot` | Slot helpers | Place a named slot of the partial or an ancestor (`exp/slots`); `requiredSlot` fails for a missing one, `deferredSlot` places it to load after the page or when revealed. |
| `sseAttrs` | SSE helper | The attributes of an element whose stream `exp/sse`'s browser client opens: client id, hidden grace, beat, beacon and credentials (`sse.FuncMap()`). |
| `debug` | Debug helper | Show a value (`ext/debug`); `debug.WithFunc()` binds it to the render, `debug.Disabled()` renders nothing. |
| `logger` | Diagnostic helper | Emit an `ext.logger.template` event from a template (`ext/logger`); renders nothing. |
| `dict` | Data helper | Build a map when a template needs map-style values (`templatehelpers.CollectionFuncMap()`). |
| `oob`, `oobAttr` | Connector helpers | Detect out-of-band rendering and emit the connector's out-of-band attribute (`hx-swap-oob` for htmx, `up-hungry` for Unpoly). |
| `oobSwap` | Connector helper | Write the connector's out-of-band attribute, also outside an out-of-band render. |
| `ctx`, `request`, `url`, `basePath` | Request helpers | Read request-aware values while dot remains your app model. |
| `locale`, `localizer` | Localization helpers | The request's locale and localizer (`exp/localization`). |
| `csrf` | CSRF helper | The request's CSRF token (`exp/csrf`). |
| `urlIs`, `urlStarts`, `urlContains`, `urlPath`, `joinPath` | URL helpers | Read and compare request paths. |
| `targetValue`, `selectionValue`, `actionValue` | Connector helpers | Read current connector target, selection, and action values (`exp/target`, `exp/selection`, `exp/actions`). |
| `targetIs`, `selectionIs`, `actionIs` | Connector helpers | Report whether the current target, selection or action is one of the values given. |
| `targetHeader`, `selectionHeader`, `actionHeader` | Connector helpers | The names of the request headers the connector reads them from. |

Translation helpers such as `tl`, `tn`, `ctl`, and `ctn` are not built in. Add them through `Partial.SetFunc`.

## Typed Template Composition

For rows, cards, nested sections, and reusable fragments, prefer native Go template composition. This is the path go-clue understands best.

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

The same row template can still be rendered as an HTMX target by registering a partial and resolving dynamic target IDs with `exp/target`:

```go
rowPartial := partial.NewID("row", "templates/row.gohtml")

table.With(rowPartial).Use(target.Stage())
target.WithResolver(table, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
    if !strings.HasPrefix(target, "row-") {
        return nil, false
    }

    row := loadCurrentRow(target)
    return partial.NewID(target, "templates/row.gohtml").SetDot(row), true
})
```

That gives you one template for three modes: inside a parent render, as a standalone render, and as an HTMX target response.

## Wrapper Content

When a wrapper has a content child, the wrapper renders that configured route partial with `content`:

```gotemplate
<main>
    {{ content }}
</main>
```

Use native `template` for typed composition inside the content partial. Register partials in Go when they also need to be renderable as HTMX targets or OOB output.

## go-clue `@model` Contracts With `SetModel`

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

The template owns the names `Page` and `User`. go-partial scans the go-clue contract and matches controller values by type instead of asking you to repeat the names in Go.

A model can be computed only when a template calls it: `partial.ModelFuncOf(fn)` is such a model as a value, which `SetModel` takes with the others, computed once per render tree, also when several templates call it, and not at all for a response that leaves them out. It is matched by the type `fn` returns, also by `CheckContracts`, without calling `fn`.

```go
content.SetModel(page, partial.ModelFuncOf(func(ctx context.Context) (app.User, error) {
    return users.Current(ctx)
}))
```

`Dot.Model()` gives a dot as a model that shares its value, so a layout's lazy dot and the regions that declare it with `@model` compute it once:

```go
view := partial.DotFuncOf(loadView)
root.SetDotSource(view).SetModel(view.Model())
```

## Request Helpers With Dot Templates

When `SetDot` is used, `.` belongs to your app model. Request-specific data remains available through helpers:

```gotemplate
{{ ctx.URL.Path }}
{{ request.Method }}
{{ locale }}
{{ csrf.Key }}
{{ basePath }}
```

`ctx` returns the active `partial.RenderContext`. `request`, `url` and `basePath` are go-partial's own helpers; `locale` and `csrf` are given by the stages of `exp/localization` and `exp/csrf` (`localization.Stage()`, `csrf.Stage()`).

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

A pair whose key names a root the callee declares, such as an `@model`, also binds that root, with its type checked:

```gotemplate
{{ range .Related }}{{ partial runtime "templates/card.gohtml" "Event" . "Note" "new" }}{{ end }}
```

```gotemplate
{{/*
@model Event github.com/example/app.Event
*/}}

<article>{{ Event.Title }} {{ .Note }}</article>
```

The card reads `Event` as a typed root and `Note` from its dot map; the pairs stay in the dot map as well. A value of another type fails the partial. The callee also sees the `SetModel` values of the partials it is rendered in.

For rows and larger fragments, prefer native `template` plus `@dot`, because that gives go-clue the strongest type information. Use `partial` when the nested render should go through go-partial itself.

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
root.SetFunc(flash.FuncMap())
root.Use(flash.Stage())
```

Add messages to the request context before rendering:

```go
ctx := flash.Add(r.Context(), flash.Success("Saved"))
_ = partial.Write(ctx, w, r.WithContext(ctx), root)
```

Render them with the embedded default template:

```gotemplate
{{ flash }}
```

Render a stable destination in a wrapper when fragments should append messages
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

`flash.ClientScript()` is a browser client, served from `clients.FS()`, that hides messages after a time per level, keeps them under the pointer or focus, removes them with a `data-flash-dismiss` element or Escape and shows an error for failed requests; see the README.

`flashTemplate` renders the flash template as a `<template data-flash-template="level">` for each level, or those given, which the browser client clones for the messages it shows itself, such as the error of a failed request:

```gotemplate
<div {{ flashTargetAttrs "The request failed." }}>
    {{ flashTemplate }}
    {{ flash }}
</div>
```

```gotemplate
{{ flashTemplate "error" "info" }}
```

`flashTemplate` writes nothing when the partial that calls it renders out of band, such as a flash target a response adds its messages to: the page has the templates already, so the template needs no `{{ if not oob }}`.

`flashTargetAttrs` writes the flash target's id and, with `flash.WithRequestErrors`, the attributes that tell the browser client how to show failed requests, so templates write none of the client's `data-flash-request-error*` attributes by hand:

```go
root.Use(flash.Stage(flash.WithRequestErrors(flash.RequestErrors{
	Text:      "The request failed. Please try again.",
	Replace:   true, // one request error at a time
	Body:      true, // a refusal's text/plain sentence replaces the text
	SkipFlash: true, // a failed response with a flash message of its own shows only that
})))
```

```gotemplate
<div {{ flashTargetAttrs (t "The request failed. Please try again.") }} {{ oobAttr "beforeend" }} class="toasts">
```

renders `id="flash-messages" data-flash-request-error="..." data-flash-request-error-replace data-flash-request-error-body data-flash-request-error-skip-flash`; a text argument, such as a translated one, replaces the stage's. `{{ flashTarget }}` writes them too, and flash target templates get them as `.RequestErrorAttrs`.

Applications can override the default message template with `flash.WithTemplate(...)`
or `flash.WithPartial(...)`, and the target with `flash.WithTargetTemplate(...)`,
`flash.WithTargetPartial(...)`, or `flash.WithTargetID(...)`.
Custom levels and target IDs are normalized into lowercase CSS-friendly tokens
before templates receive them.

## Interaction Helpers

Interaction helpers render connector-aware loading or request markup for endpoints. The active connector supplies protocol attributes, and the interaction stage owns the final HTML wrapper.

Interaction helpers live in `github.com/donseba/go-partial/exp/interactions`
and are opt-in. Register them explicitly:

```go
import "github.com/donseba/go-partial/exp/interactions"

root.SetFunc(interactions.FuncMap())
```

`interactions.AttrsFuncMap()` has only the helpers that write attributes on an element of the template's own (`asyncAttrs`, `revealAttrs`, `pollAttrs`, `onAttrs`, `streamAttrs`, `refreshAttrs`, `submitAttrs`, `liveAttrs`, `clientAttrs`, `noBoost`, `confirmAttr`), leaving generic names such as `poll`, `on` or `async` to the application.

Those functions carry `go-doc:sig` comments for overloads such as endpoint
strings and named interactions. go-clue, formerly go-doc, still reads these
comments and `.go-doc/config.json`. Add them through `.go-doc/config.json` as
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

With the HTMX connector, `{{ async runtime "/stats" }}` renders markup shaped like:

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
loader := interactions.NewAsync("/table/row/:row").Param("row", row.ID)
```

```gotemplate
{{ async runtime "/table/row/:row" "row" .ID }}
```

### Attributes On Your Own Elements

Each helper except `prefetch` has an attribute variant that returns only the connector attributes, as `template.HTMLAttr`, for an element the template already has. Without a `target` or `id` the interaction targets that element itself:

```gotemplate
<ul id="jobs"{{ if .Running }}{{ pollAttrs runtime "/jobs" "3s" }}{{ end }}>
<tr {{ revealAttrs runtime "/rows" "page" .Next "swap" "afterend" }}>
<section {{ asyncAttrs runtime Stats }}>
```

With the htmx connectors, `pollAttrs runtime "/jobs" "3s"` renders `hx-get="/jobs" hx-swap="innerHTML" hx-target="this" hx-trigger="every 3s"`.

### Select And Other Attributes

`"select"` picks the part of the response that is swapped in: `hx-select` with the htmx connectors, `data-select` with the plain connector; Unpoly and Turbo select with their target. Options whose names start with `hx-`, `up-`, `data-` or `aria-` are written on the element as given, after the connector's attributes, so they replace one of the same name. Every htmx attribute is so available, such as `hx-indicator`, `hx-push-url`, `hx-include`, `hx-vals`, `hx-sync` or `hx-confirm`:

```gotemplate
<div {{ pollAttrs runtime "/inbox" "every" "10s" "select" "#messages" "hx-indicator" "#spinner" }}></div>
```

renders `hx-get="/inbox" hx-indicator="#spinner" hx-select="#messages" hx-swap="innerHTML" hx-target="this" hx-trigger="every 10s"`.

### Forms, Live Search And Links

`submitAttrs` makes a form post with the connector's client library and swap the response into its target, the form itself unless a `target` or `id` is given; `"method"` picks another method, such as `"delete"`:

```gotemplate
<form method="post" action="{{ .Action }}" {{ submitAttrs runtime .Action "target" "#comments" "swap" "outerHTML" }}>
```

renders `hx-post` with the htmx connectors, `up-submit` and `up-target` with Unpoly, and `data-turbo-frame="comments"` with Turbo.

`liveAttrs` requests an endpoint as the user types, after a pause (`"delay"`, 300ms by default):

```gotemplate
<input type="search" name="q" {{ liveAttrs runtime "/search" "target" "#content" "include" "closest form" "pushURL" "true" }}>
```

renders `hx-get="/search" hx-include="closest form" hx-push-url="true" hx-swap="innerHTML" hx-target="#content" hx-trigger="input changed delay:300ms, search"` with htmx, and `up-autosubmit` with `up-watch-delay` with Unpoly. `"include"`, `"pushURL"` and `"replaceURL"` are neutral options every interaction takes, written in the connector's syntax, such as `hx-include`, `hx-push-url` and `hx-replace-url`. Interaction values set them with `Option`, such as `Option("pushURL", "true")`; the connector package names them `connector.OptionInclude`, `OptionPushURL` and `OptionReplaceURL`, with `OptionMethod` (submit) and `OptionDelay` (live).

`clientAttrs`, `noBoost` and `confirmAttr` write how the client library treats a link or form of the template's own (see `connector.Attrs`):

```gotemplate
<a href="/download.zip" {{ noBoost runtime }}>Download</a>
<button {{ confirmAttr runtime "Delete this page?" }}>Delete</button>
<a href="/logout" {{ clientAttrs runtime "confirm" "Log out?" "boost" "off" }}>Log out</a>
```

With htmx, `noBoost` is `hx-boost="false"` and `confirmAttr` is `hx-confirm`; with Turbo, `data-turbo="false"` and `data-turbo-confirm`; with Unpoly, `up-follow="false" up-submit="false"` and `up-confirm`.

The settings of a whole page go on the body: `"inherit" "true"` gives them to the descendants, `"disableWhileRequest"` names the elements disabled during a request and `"keepOnServerError" "true"` leaves the page as it is on a 5xx response. A submit button asks its own question with `"submitConfirm"`:

```gotemplate
<body {{ clientAttrs runtime "inherit" "true" "boost" "on" "target" "#content" "swap" "outerHTML" "disableWhileRequest" "find button[type='submit']" "keepOnServerError" "true" }}>
<button type="submit" name="action" value="purge" {{ clientAttrs runtime "submitConfirm" "Delete for good?" }}>Delete</button>
```

With htmx 4 the body gets `hx-boost:inherited="target:#content swap:outerHTML" hx-disable:inherited="find button[type='submit']" hx-status:5xx:inherited="swap:none push:false"`. A target or swap with whitespace or a comma is quoted there, as htmx 4 ends an unquoted value at either: `"swap" "outerHTML showTarget:body"` gives `swap:'outerHTML showTarget:body'`. The button gets `data-turbo-confirm` with Turbo, else `data-partial-confirm`, which the client of `exp/interactions` asks (`interactions.ClientScript()`).

`"reloadOnError" "true"` makes a link or form whose request gets a 5xx status or no answer load as a whole page, so the visitor sees what went wrong rather than nothing, also with `"keepOnServerError"`: a link is followed and a GET form submitted again by the browser itself. A request that is not safe to send twice (POST, PUT, PATCH, DELETE), such as a comment or a vote, is never sent again: if the server answered with an HTML error page, that page replaces the document, as a whole-page load would show it; without one, or without an answer, the page stays as it is and `partial:reload-on-error:failed` is dispatched on the form (`detail.method`, `detail.status`, 0 without an answer), so the application can react. An htmx 4 answer with a 5xx status and `HX-Reswap: none` leaves the page as it is, also for a POST: it has only out-of-band parts, such as the error flash message of `ext/errors`' `WithFlash(fn)`, which htmx 4 swaps. It is `data-partial-reload-on-error` with every connector, which the client of `exp/interactions` reads for htmx 2 and 4, and for Unpoly and Turbo requests that get no answer; on the body it applies to every link and form.

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

Use `oob` inside out-of-band templates to check whether the partial is being rendered as OOB output. Use `oobAttr` to emit the connector's out-of-band attribute only during OOB rendering: `hx-swap-oob` for htmx (and for Turbo and connectors without one of their own), `up-hungry` for Unpoly, as `connector.OOBAttrs` writes it.

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

Pass an explicit swap value only when you need one; Unpoly, whose hungry elements are replaced whatever the swap, ignores it:

```gotemplate
<aside id="toast"{{ oobAttr "outerHTML" }}>
```

A region with `SetOOBElements(ids...)`, or `SetOOBElementsFunc(fn)` choosing the IDs per request, sends only the elements of its output with those IDs when it goes out of band, each carrying the connector's out-of-band attribute: the region's own element, the one with `{{ oobAttr }}`, is dropped unless its ID is one of them. With none of the IDs in the output, or an element whose end cannot be told (an `<li>` without `</li>`), the whole region goes with its `oobAttr`, as without IDs; on a full page the region renders whole.

```gotemplate
<nav id="menu"{{ oobAttr }}>
  <a id="menu-home" href="/"{{ if urlIs "/" }} aria-current="page"{{ end }}>Home</a>
  <a id="menu-about" href="/about"{{ if urlIs "/about" }} aria-current="page"{{ end }}>About</a>
</nav>
```

With `SetOOBElements("menu-home", "menu-about")`, a partial response sends:

```html
<a hx-swap-oob="true" id="menu-home" href="/">Home</a>
<a hx-swap-oob="true" id="menu-about" href="/about" aria-current="page">About</a>
```

`oobSwap` writes the connector's out-of-band attribute whenever the template renders, such as for an element a response always sends out of band: `hx-swap-oob` for htmx, `up-hungry` for Unpoly.

```gotemplate
<div id="flash-messages"{{ oobSwap "beforeend" }}>…</div>
```

## URL And Connector Helpers

URL helpers read from the current request:

```gotemplate
{{ if urlIs "/settings" }}active{{ end }}
{{ joinPath basePath "users" }}
{{ urlPath basePath "users" "42" }}
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
`exp` render stages when the application opts into those packages.

Generic helpers such as strings, maps, dates, and counters are convenience
helpers. Register them from the optional provider when you want them:

```go
import "github.com/donseba/go-partial/exp/templatehelpers"

root.SetFunc(templatehelpers.FuncMap())
```

Use narrower maps when you want a smaller helper surface:

```go
root.SetFunc(
    templatehelpers.StringFuncMap(),
    templatehelpers.CollectionFuncMap(),
)
```

`templatehelpers.HTMLFuncMap()` is separate because helpers such as `safeHTML`
mark content as trusted HTML.

## Translation Helpers

Translation helpers are user-owned. The localization stage exposes `localizer` and `locale`, and your app can add functions such as `tl`, `tn`, `ctl`, and `ctn`.

```go
// tr gives tl, tn, ctl and ctn, such as a *translator.Translator of
// github.com/donseba/go-translator/v2.
root.SetFunc(localization.FuncMap(), tr.FuncMap())
root.Use(localization.Stage())
```

```gotemplate
{{ tl localizer "Hello, World!" }}
{{ tn localizer "You have one message." "You have %d messages." 5 5 }}
{{ ctl localizer "button" "save" }}
```

`github.com/donseba/go-translator/v2` (package `translator`) already gives these helpers with `Translator.FuncMap()`; they take the localizer as their first argument.

For templates go-partial does not execute, `localization.Funcs(localization.New("nl"))` returns `locale` and `localizer` bound to a localizer.

## Stand-Ins For Stage Functions

Functions a render stage gives templates, such as `slot`, `locale`, `flash` or `targetIs`, need no stand-ins registered with `SetFunc`: templates are parsed with stand-ins for the names the stages of the render declare, which the stages replace when the templates execute. Registering a package's `FuncMap()` still works and gives go-clue the signatures. A stand-in that is called because no stage of the render gives the function fails the template with an error that names it.

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

