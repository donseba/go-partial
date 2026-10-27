<p align="center">
    <a href="https://docs.gowebthings.com/go-partial">
        <img src="./assets/go-partial-logo.png" alt="go-partial" height="70">
    </a>
</p>

# go-partial

[Documentation](https://docs.gowebthings.com/go-partial) · Part of [go-webthings](https://gowebthings.com/components).

This package provides a request-aware rendering layer for Go templates. It lets applications render full pages or targeted partials from the same registered template tree, with wrapper/content composition, connector headers, OOB output, caching, and typed template-friendly data flow.
`go-partial` supersedes `go-htmx`. The rendering core stays small; connectors and optional extensions add the browser integration your application needs.

## Features

- **Partial Templates**: Define and render partial templates with typed dot data and functions.
- **Native Template Composition**: Use `{{ template "row.gohtml" . }}` with `SetDot` so reusable sections can render inside a page or as HTMX targets.
- **Wrapper Content**: Use wrapper partials with content children while keeping page data explicit.
- **Template Caching**: Enable caching of parsed templates for improved performance.
- **Out-of-Band Rendering**: Support for rendering out-of-band (OOB) partials.
- **File System Support**: Use custom fs.FS implementations for template file access.
- **Concurrent Rendering**: Configure partial trees up front, then reuse them safely across concurrent requests.
- **Checks**: Check typed contracts with `CheckContracts` and every template file with `CheckTemplates` before the first request.
- **Browser Clients**: Small scripts for flash messages, server-sent events and interactions, with no build step (`clients`).
- **Strict Content-Security-Policy**: Output that works without `'unsafe-inline'` and `'unsafe-eval'`; see Content-Security-Policy.

## Installation
To install the package, run:
```bash
go get github.com/donseba/go-partial
```

## Template functions
Template-facing helpers and accessors are documented in [TEMPLATE_FUNCTIONS.md](TEMPLATE_FUNCTIONS.md).

## Package Shape
The root package is intentionally small: rendering lifecycle, partial trees, runtime context, connectors, Root partial configuration, and core template helpers. Core does not import optional packages.

Optional packages are split by stability:

- `ext/...` contains extension packages that are useful but not required by core, such as `ext/errors` and `ext/debug`.
- `exp/...` contains experimental opt-in features, such as localization, CSRF, flash messages, selection, actions, pageflow, interactions, metrics, OpenTelemetry, slots, head regions, target resolvers, template helpers, and SSE.

Applications choose the pieces they want with `SetFunc(...)`, `Use(...)`, or package-specific setup helpers. A render stage follows the same lifecycle everywhere: `Prepare` can add request-scoped context, `Render` wraps or replaces template rendering, and `Finalize` observes or transforms the result.

A stage gives templates functions bound to the render with `ResolveFunc`: go-partial asks it, by name, only for the functions a template calls, so a page whose templates call none of them pays nothing. Templates are parsed with stand-ins for the names a stage declares in `FuncNames`, so `Use(stage)` is all a stage needs; the `FuncMap()` of the stages of `exp/...` and `ext/...` stays for go-clue's signatures and for templates go-partial does not execute.

```go
partial.RenderStageHooks{
    FuncNames: []string{"tenant"},
    ResolveFunc: func(ctx *partial.RenderContext, name string) (any, bool) {
        if name != "tenant" {
            return nil, false
        }
        return func() string { return tenantFrom(ctx.Context) }, true
    },
}
```

`RenderContext.SetFunc` in `Prepare` still works and comes first, but makes the function for every partial the stage prepares.

Render stages may set generic response metadata through `ctx.Response`. `partial.Write` applies that status and those headers after rendering. Templates do not receive helpers for setting headers or status; templates produce HTML.

## Rendering
For partials, prefer the package-level rendering functions:

```go
html, err := partial.Render(ctx, content)
html, err = partial.RenderWithRequest(ctx, r, content)
err = partial.Write(ctx, w, r, content)
```

`partial.Write` owns HTTP response behavior: configured headers, connector response headers, render-stage response metadata, error fragments, and out-of-band regions. `Partial` itself does not render or write responses; pass it to the package functions.

`partial.RenderSelf(ctx, r, p)` renders the partial itself with the request, whatever target the request asks for. Use it for HTML that reaches the browser another way, such as a server-sent event.

`partial.Write` adds the request headers the connector reads to the `Vary` response header (see `connector.VaryHeaders`), so caches keep a page and a fragment of it apart. Unless a header sets it, `Write` sends `Content-Type: text/html; charset=utf-8`.

### Template Cache
`UseTemplateCache(true)` on the root turns the template cache on for the whole tree: children, content, the partials `{{ partial }}` renders and those rendered under the root with `SetParent` use the setting of their nearest ancestor that sets one, unless they set it themselves. One switch on the root so covers development and production:

```go
root.UseTemplateCache(!development)
```

What the cache learns of a file system, its parses, its scans, which files exist and what include patterns match, is kept by the root of the tree and shared by its clones, the partials rendered under it and those that name it with `SetParent`. Another root, also one over the same directory or map file system, such as a second application in one test process, learns it again: it sees a file added after the first root rendered, which the first root, with its cache on, does not. Give partials built per request a configured parent with `SetParent` rather than a root of their own, so they share its cache.

### Shared Includes
`SetIncludes` gives every template of a partial and of the partials rendered under it the templates its patterns match, such as a theme's partials, as if each partial listed the files itself:

```go
root := partial.NewID("shell", "layout.gohtml").
    SetFileSystem(theme).
    SetIncludes("partials/*.gohtml")
```

Patterns are those of `fs.Glob`, resolved against the file system of the partial being rendered; a file is parsed once, also when several patterns, or the partial's own templates, name it. A partial's patterns come after those of its ancestors. With the template cache a pattern is resolved once per file system; without, at every render, so new files show.

`SetChildIncludes` gives the patterns to the partials rendered under a partial only, not to the partial itself: a root gives the content, regions, fragments and error templates of every page the theme's partials without adding them to the parse of its layout.

```go
root.SetChildIncludes("partials/*.gohtml")
```

`root.Glob(pattern)` returns the files a pattern matches, found as includes are: once per file system with the template cache.

### Data Computed When Needed
`partial.DotFuncOf(fn)` is a dot computed by a function only when the partial renders, on the page or as the target of a partial request, which `SetDotSource` sets. A response that leaves the partial out, such as one for another panel of a dashboard, does not load its data:

```go
stats := partial.DotFuncOf(func(ctx context.Context) (Stats, error) {
    return store.Stats(ctx)
})
panel := partial.NewID("stats", "stats.gohtml").SetDotSource(stats)
```

The function receives the render's context and runs once per render tree; an error fails the partial's render. The dot keeps its type, so `CheckContracts` checks it against the template's `@dot` without calling the function. A `partial.Dot` can travel as a value, such as in a field of a struct, before there is a partial.

`partial.ModelFuncOf(fn)` is a typed `@model` value computed only when a template calls it, once per render tree, also when the layout, the header and the sidebar each call it, and not at all for a response that leaves them out. `SetModel` and `SetContract` take it with the other models, also from a list of models an application passes around before there is a partial:

```go
page.SetModel(user, partial.ModelFuncOf(func(ctx context.Context) (Navigation, error) {
    return loadNavigation(ctx)
}))
```

```gotemplate
{{/* @model Nav github.com/acme/app.Navigation */}}
<nav>{{ range Nav.Items }}…{{ end }}</nav>
```

It is matched by its type, as `SetModel` values are, also by `CheckContracts`, without calling the function. `SetModel` ignores nil values.

`Dot.Model()` gives a dot as a model that shares its value: a dot of `DotFuncOf` is computed once per render tree when the layout renders it as its dot and the regions call it with `@model`, rather than once for each. A dot of an interface type, whose values' type is not known, is no model.

```go
view := partial.DotFuncOf(loadView)
root.SetDotSource(view).SetModel(view.Model())
```

### Choosing A Template
`FirstExisting` returns the first of several templates that exists in a partial's file system, such as a page's own template before the one of its type and the default:

```go
file, err := root.FirstExisting("pages/"+slug+".gohtml", "pages/"+kind+".gohtml", "pages/page.gohtml")
```

With the template cache, the answer for each candidate is remembered per file system, also when it is missing. Without a match, the error wraps `fs.ErrNotExist`.

Over an `Overlay(theme, defaults)`, `FirstExisting` finds a default specific template before the theme's general one. `FirstExistingIn` tries every candidate in a layer before the next, the layers of the partial's overlay or those it is given:

```go
file, err := root.FirstExistingIn(nil, "screens/"+key+".gohtml", "screens/"+kind+".gohtml")
```

`root.Defines("mail/welcome.gohtml", "html")` reports whether a file defines a template, from the scan renders use, kept with the template cache; `HasEntry()` whether a partial's file defines its entry.

### Rendering Text
`partial.RenderText(ctx, p)` renders a partial with `text/template`, for text that is not HTML, such as the subject and text part of an e-mail, with the file system, includes, functions, `@model` values, stages and template cache of an HTML render:

```go
mail := partial.NewID("mail", "mail/welcome.gohtml").SetParent(root).SetDot(user)

subject, err := partial.RenderText(ctx, mail.SetEntry("subject"))
text, err := partial.RenderText(ctx, mail.SetEntry("text"))
html, err := partial.RenderSelf(ctx, nil, mail.SetEntry("html"))
```

Nothing is escaped. Stages see `partial.RenderKindText`. Text templates call others with `{{ template }}`, from the partial's files and includes; `{{ partial }}` and `{{ content }}` render HTML.

### Rendering Under A Parent
`SetParent` renders a partial under a configured partial without adding it to that partial's children: it inherits the parent's functions, render stages, connector, file system, template cache, failure policy and `@model` values, and the parent is not changed. One configured root can so be the parent of every part an application renders on its own, such as a component, an e-mail or a server-sent event, without cloning it:

```go
card := partial.NewID("card", "card.gohtml").SetDot(data).SetParent(root)
html, err := partial.RenderSelf(ctx, r, card)
```

Requests cannot target such a partial; use `With` for children that can be targeted. `Parent()` returns the partial a partial renders under.

### Rendering A Defined Template
`SetEntry(name)` renders the template a file defines with `{{ define name }}` instead of the file itself, for files that define several parts, such as an e-mail:

```go
html, err := partial.Render(ctx, partial.New("mail/welcome.gohtml").SetEntry("html").SetDot(user))
```

### Failure Policy
By default a failing part of a page, such as the content child or a `{{ partial }}` call, renders as an error fragment in place and the rest of the page renders. Where a partly broken page is worse than an error page, fail the whole response instead:

```go
root.SetFailurePolicy(partial.FailResponse)
```

With `FailResponse`, any failing part, and any template, child or content partial that is missing, fails the render. `Render`, `RenderWithRequest` and `RenderSelf` return the first failure and no HTML. `Write` writes nothing of the page: it writes the failure response of a registered error stage, such as `ext/errors`, or nothing at all, and returns the error so the handler can answer it.

When `Write` wrote a failure response, it returns a `*partial.FailureResponseError` with the status it wrote, so the handler knows not to write another one. A failure response is sent with `Cache-Control: no-store`. With `FailResponse` it keeps an error status for partial requests too, and comes without out-of-band regions; with `FailInline` a partial request gets the error fragment with status 200, so the client swaps it in place.

```go
err := partial.Write(ctx, w, r, page)
var written *partial.FailureResponseError
if err != nil && !errors.As(err, &written) {
    http.Error(w, "Something went wrong", http.StatusInternalServerError)
}
```

### Target Fallback
A partial request for a target the tree cannot resolve, such as a link in another page's layout that targets an element this page lacks, fails by default. A target fallback answers it with a partial of the tree instead, and tells the client to swap it where that partial lives, as a navigation:

```go
root.SetContent(content).
    SetTargetFallback(partial.TargetFallback{ID: "content", PushURL: true})
```

The fallback renders as if the client had asked for it, with the out-of-band regions of its parents. A slot of `exp/slots` that is requested on its own, such as a list that loads more rows, answers without those regions with `slots.OmitOOB()`. `Write` sends `Retarget` (`#content` by default), `Reswap` (`outerHTML` by default) and, for GET requests with `PushURL`, the request URL through the connector, such as `HX-Retarget`, `HX-Reswap` and `HX-Push-Url` for htmx. Known children and targets a resolver of `exp/target` answers are not affected.

Each answer of the fallback emits a `target.fallback` event at info level, with the `target` asked for and the `fallback` ID; a request the tree cannot answer without a fallback emits a `target.missing` warning. See [OBSERVABILITY.md](OBSERVABILITY.md).

## Metrics Output
`exp/metrics` records render lifecycle data through a small `Sink` interface. Use your own sink for storage, or write JSON lines to any `io.Writer`:

```go
sink := metrics.Fanout(
    inMemoryStore,
    metrics.NewWriterSink(os.Stdout),
)

root.Use(metrics.Stage(sink, metrics.WithTag("chain", "web")))
```

The writer sink is intentionally plain: it works with stdout, files, buffers, pipes, or app-owned adapters that forward records to SSE, queues, or databases.

`metrics.Outermost()` records only the outermost render of a context, such as a page's layout with everything rendered inside it, instead of every nested partial; the renders inside it then cost no more than a context lookup, which suits metrics that are always on:

```go
root.Use(metrics.Stage(sink, metrics.Outermost()))
```

Metrics records distinguish request identity from flow identity. `WithRequestID` labels the current HTTP request. `WithTraceID` can group related browser work such as a page request and later async or SSE requests. `WithParentRequestID` can link a spawned request back to the request that caused it.

## Diagnostic Events
Core and extensions can emit diagnostic events without owning logging, tracing, or queues. Attach shared consumers to the reusable root partial:

```go
events := partial.NewAsyncEvents(
    partial.EventsConfig{Buffer: 256, DropPolicy: partial.DropNewest},
    logger.Sink(slog.Default(), logger.WithMinLevel(partial.EventWarn)),
    mySink,
)

root := partial.New("templates/shell.gohtml").
    SetEvents(events)
```

`ext/logger` adapts events to `log/slog`. `logger.WithLevels(...)` writes only the levels given, such as `partial.EventWarn` without the errors an application logs where it answers them, and `logger.Sink(nil)` writes to `slog.Default()` as it is when each event is written. App-owned sinks can forward the same events to DynamoDB, Kafka, OpenTelemetry, SSE debug pages, files, or any other collector. See [OBSERVABILITY.md](OBSERVABILITY.md) for concrete sink examples.

For request-owned collectors, attach a sink with `partial.WithEventSink(r.Context(), sink)` and close request-owned async dispatchers in middleware.

## Example Applications
A documentation-style site built with `go-partial` is available in [examples/docs](examples/docs).

```bash
go run ./examples/docs
```

Open http://localhost:8091.

An htmx-backed feature showcase with real template files is available in [examples/showcase](examples/showcase).

```bash
go run ./examples/showcase
```

Open http://localhost:8090 to see pages for typed rows, selection partials, actions, flash messages, out-of-band rendering, context helpers, localization, HTMX response headers, server-sent events, live metrics, infinite scroll with cursor-style `X-Action` values, and the optional error page.

## Integrations
Several integrations are available, detailed information can be found in the [INTEGRATIONS.md](INTEGRATIONS.md) file.

- htmx 2 (`connector.NewHTMX`) and htmx 4 (`connector.NewHTMX4`)
- Turbo
- Unpoly
- Partial, for framework-neutral fetch clients and tests
- SSE writer, for streaming rendered HTML patches

### HTMX 4

Choose `connector.NewHTMX4(nil)` on the root partial. The connector reads
`HX-Request-Type` and decodes `HX-Target: main#content` into the partial ID
`content`. Body/select requests and history restoration render the full page.
Use IDs on fragment targets; the tag name alone cannot identify a partial.

Continue to build responses with `ReswapWith(connector.NewSwap()...)`. The
response builder snapshots the options, and the selected connector serializes
scroll/show targets in its own protocol. `NewHTMX` keeps the HTMX 2 syntax;
`NewHTMX4` emits separate `scrollTarget`/`showTarget` modifiers. Raw `Reswap`
strings are sent as supplied, so write those in the selected protocol's syntax.

HTMX 4 removed `HX-Trigger-After-Swap` and `HX-Trigger-After-Settle`; its
connector omits these fields. Use `Trigger` for server events or the browser's
`htmx:after:swap` / `htmx:after:settle` events when timing matters. Stream
interactions emit `hx-sse:connect`; load the pinned **hx-sse extension** and send
unnamed SSE data messages for HTML swaps. Named SSE messages dispatch events.

Browser migration also requires explicit `:inherited` attributes and the new
fetch-based event context. See the [official migration guide](https://four.htmx.org/docs/whats-new-in-htmx-4).

### HTMX 4 protocol audit

The connector's complete header/attribute output was checked against the
[HTMX 4 header reference](https://four.htmx.org/reference/headers/),
[migration reference](https://four.htmx.org/docs/whats-new-in-htmx-4), and the
[swap reference](https://four.htmx.org/reference/attributes/hx-swap).

| Definitions | HTMX 4 behavior |
| --- | --- |
| `HX-Request`, `HX-Boosted`, `HX-Current-URL`, `HX-History-Restore-Request` | Names unchanged; history restores render the document |
| `HX-Request-Type` | `full` renders the document; `partial` renders the target |
| `HX-Target`, `HX-Source` | Browser sends `tagName#id`; target lookup decodes the URI-encoded ID |
| Request `HX-Trigger`, `HX-Trigger-Name` | Replaced by `HX-Source`; legacy constants remain for HTMX 2 |
| `HX-Prompt` | Available with the HTMX 4 prompt extension; absent from core |
| `HX-Location`, `HX-Push-Url`, `HX-Redirect`, `HX-Refresh`, `HX-Replace-Url`, `HX-Reswap`, `HX-Retarget`, `HX-Reselect`, response `HX-Trigger` | Header names unchanged; supported response fields are covered together by a protocol test |
| `HX-Trigger-After-Swap`, `HX-Trigger-After-Settle` | Omitted by the HTMX 4 connector; retained by HTMX 2 |
| `hx-get`, `hx-trigger`, `hx-target`, `hx-swap` | Still valid on the request element; async/load, reveal, poll, refresh and custom-event helpers are covered |
| `hx-ext`, `sse-connect`, `sse-swap` | Legacy only; streams emit `hx-sse:connect`, `hx-target`, `hx-swap` and require the HTMX 4 SSE script |
| `Swap.FocusScroll()` | `focusScroll` in HTMX 4, `focus-scroll` in HTMX 2 |
| `Swap.Show()` / `Swap.Scroll()` | HTMX 4 separates direction from quoted `showTarget` / `scrollTarget` |

The attribute constants cover output owned by the interaction helpers, rather
than every possible template attribute. Application templates must also migrate
inheritance, renamed attributes, event handlers and extension loading.

HTMX 4 also introduces built-in morph swaps and `outerSync`; typed swap styles
are available as `SwapInnerMorph`, `SwapOuterMorph`, `SwapOuterSync` and
`SwapTextContent`. These use the same `NewSwap().Style(...)` builder.

Other new browser features belong in application templates: `hx-action` and
`hx-method`, body-based `hx-query`, per-element `hx-config`, `hx-ignore`,
`hx-validate`, status-specific `hx-status:CODE`, explicit inheritance and
multi-target `<hx-partial>` responses. Existing OOB output remains supported.
These do not require new connector request/response headers. The rewritten SSE
integration is covered; WebSockets, streaming multipart responses, reactive
`hx-live`, download/head/pending/preload and other extensions require their own
scripts and application endpoints. They are not enabled by `NewHTMX4`.


## Error Output
`partial.Write` can render an HTML error page when template parsing or execution fails, but only when an error stage is registered. Register `ext/errors` to choose the failure markup. In detailed mode, the page includes the partial ID, template list, request URL, template location, and original error so development and failed htmx requests still return useful output. In safe mode (the default, for production) it names no partial, template or error: it shows the status text (`Data.Title`), the safe message (`Data.Message`) and the request URL, in the language of `Data.Lang`.

Normal requests receive a `500` full HTML page. HTMX/partial requests receive a swappable error fragment with status `200`, because HTMX does not swap `500` responses by default.

The built-in page styles itself with one inline `<style>` element, so it looks right when the application's own assets are what failed; `exterrors.StyleHash()` is its hash for a Content-Security-Policy. The built-in fragment, which lands in the application's page, has classes only (`go-partial-error`, `go-partial-error__title`, `go-partial-error__message`, `go-partial-error__details`): serve `exterrors.StylesheetFS()`, which holds the CSS that styles them as `partial-errors.css`, and link that file from the layout, or style the classes in the application's own CSS. See Content-Security-Policy.

With `exterrors.WithFlash(fn)`, a partial request that fails is answered with an error flash message instead, added out of band to the flash target with the template of the tree's `exp/flash` stage, as the page's own messages are, while the request's target stays as it is (`HX-Reswap: none`); its text is the one `fn` returns, or `Data.Message` (see `WithMessage`) for a nil `fn` or an empty text. Refusals that are not render failures use `flash.Respond` (see Flash Messages In The Browser).

Register the error stage on the reusable root partial:

```go
root := partial.New("templates/shell.gohtml").
    Use(exterrors.Stage(exterrors.WithMode(exterrors.ModeDetailed)))
```

Or on one partial:

```go
content.Use(exterrors.Stage())
```

The failure response can be a template of the application's own, such as a theme's 500 page or a toast for failed partial requests:

```go
root.Use(exterrors.Stage(
    exterrors.WithTemplate("500.gohtml"),
    exterrors.WithFragmentTemplate("partials/error-toast.gohtml"),
))
```

The template is read from the file system of the partial that failed and renders as its child, with its functions, stages, includes (also the root's `SetChildIncludes`) and `@model` values; its dot is an `exterrors.Data`. `Data.Message` is `exterrors.SafeMessage` in the safe mode and the error in the detailed mode, so a template can show it; `WithMessage(fn)` sets it, such as to a translation of `SafeMessage` in the request's language (an empty text keeps it), for the error page, the error fragment and the error flash message alike. `WithLang(fn)` sets `Data.Lang`, the language of the error page, such as that of the page that failed: the built-in page writes it as its `lang` attribute (`en` without it), and a template can too. `WithTitle(fn)` sets `Data.Title`, the heading of the built-in page and fragment, such as a translation of the status text (`http.StatusText`, the default; an empty text keeps it); a detailed failed render keeps its "Template render error" heading. `Data.PartialID` and `Data.Templates` name the partial that failed in detailed mode only. `Data.Error` holds the error in both modes, and `Data.Location` its template location in detailed mode: show them only when `.Detailed`. When the template fails itself, the built-in markup answers and an `ext.errors.template_failed` warning is emitted. While an error template renders, `exterrors.FromContext(ctx)` returns its `Data` and true for the context a template function with a `context.Context` parameter gets, in the template and the partials it includes, so a function that writes something of the page that failed, such as its structured data, can leave it out. A stage with only a page template answers partial requests with it too, and one with only a fragment template full pages.

`partial.RenderWithRequest` still returns the render error directly. `partial.Write` asks the render stage chain for a failure response; without `ext/errors`, it returns the original render error.

An error that happens outside a render, such as a form post whose handler fails to save it, gets the same answer with `exterrors.Respond(w, r, root, status, err)`: the error page, fragment or flash message of the stage registered on `root`'s tree, with its templates, `WithMessage` and `WithFlash`, written with `status` and never cached. `Data.Status` is that status (500 for a render that failed), and `Data.PartialID` and `Data.Templates` are empty, as no partial failed:

```go
if err := comments.Add(r.Context(), form); err != nil {
    log.Error("add comment", "error", err)
    exterrors.Respond(w, r, root, http.StatusInternalServerError, err)
    return
}
```

A full-page request gets the error page. A partial request gets the error flash message with `WithFlash`, out of band with `HX-Reswap: none`; without it, the error page for a 5xx status, which the interactions client's `reloadOnError` shows as the whole page for a form post, and the error fragment for another status, such as 429, which htmx 4 swaps into the target. A tree without the stage gets the built-in markup in the safe mode. `partial.WriteError(w, r, p, status, err)` is the core of it, for any error stage: it returns the error, writing nothing, when no stage answers; error stages read the status with `RenderContext.FailureStatus()`.

## Localization
Templates receive a request localizer through the `localizer` and `locale` helpers from `exp/localization`. The interface only requires `GetLocale()`. Translation behavior should come from user-provided template functions registered with `Partial.SetFunc`:

```go
// tr gives tl, tn, ctl and ctn, such as a *translator.Translator of
// github.com/donseba/go-translator/v2.
root.SetFunc(localization.FuncMap(), tr.FuncMap())
root.Use(localization.Stage())
```

```go
ctx := localization.WithLocalizer(r.Context(), localizer)
_ = partial.Write(ctx, w, r, content)
```

```html
<p>{{ tl localizer "Hello, World!" }}</p>
<p>{{ tn localizer "You have one message." "You have %d messages." 5 5 }}</p>
<button>{{ ctl localizer "button" "save" }}</button>
```

For templates go-partial does not execute, such as a `text/template`, `localization.Funcs(localization.New("nl"))` returns the same `locale` and `localizer` helpers, bound to a localizer.

`tl`, `tn`, `ctl`, and `ctn` are not built into go-partial. For a fuller translation backend, [github.com/donseba/go-translator/v2](https://github.com/donseba/go-translator) (package `translator`) fits this pattern well: `Translator.FuncMap()` gives these four functions, which take the localizer as their first argument.

## CSRF
`exp/csrf` gives templates the request's CSRF token through the `csrf` helper. go-partial does not make or check tokens: the application's CSRF middleware puts the token on the request context, as a `csrf.Token` (`Token(ctx)` and `Key()`) or as a plain string:

```go
root.Use(csrf.Stage())

ctx := csrf.WithTokenString(r.Context(), token) // or csrf.WithToken(ctx, myToken)
_ = partial.Write(ctx, w, r, page)
```

```gotemplate
<input type="hidden" name="{{ csrf.Key }}" value="{{ csrf.Token ctx.Context }}">
```

With `WithTokenString` the key is `csrf.DefaultTokenKey` (`X-CSRF-Token`). Without a token on the context, `csrf` gives a placeholder token that no check accepts.

## HTMX Response Helpers
The configured connector turns partial response instructions into protocol-specific response headers:

```go
content := partial.NewID("notice", "notice.gohtml")

content.Response().
    Retarget("#notice").
    ReswapWith(connector.NewSwap().Style(connector.SwapOuterHTML).Transition(true)).
    TriggerWith(connector.NewTrigger().AddEventObject("notice", map[string]any{
        "message": "Saved",
    }))
```

You can also set the response instructions as data:

```go
content.SetResponse(connector.Response{
    Retarget: "#notice",
    Trigger: connector.NewTrigger().AddEventObject("notice", map[string]any{
        "message": "Saved",
    }).String(),
})
```

With the HTMX connector, these become `HX-Retarget`, `HX-Reswap`, and `HX-Trigger` headers during `partial.Write`.

After a form is posted, `connector.Redirect` sends the client on in the way its library follows: `HX-Location` with status 200 for a request htmx made, so htmx swaps the new page into the target without a full load, and a `303 See Other` otherwise, which browsers, Turbo and Unpoly follow:

```go
connector.Redirect(root.Connector(), w, r, connector.Location{Path: "/admin/pages", Target: "#content"})
```

`connector.Attrs(conn, connector.Attr{Confirm: "Delete?", Boost: connector.BoostOff, Target: "#list", Swap: "outerHTML"})` returns the attributes of a link or form in the connector's syntax, such as `hx-confirm` and `hx-boost="false"` for htmx, `up-confirm` and `up-follow="false" up-submit="false"` for Unpoly and `data-turbo-confirm` and `data-turbo="false"` for Turbo; templates use `clientAttrs`, `noBoost` and `confirmAttr` of `exp/interactions`.

`Attr` also writes the settings of a whole page, on the body: `Inherit` gives them to the descendants (htmx 4's `:inherited`; an inherited boost carries its target and swap, quoted when they hold whitespace or a comma, such as `swap:'outerHTML showTarget:body'`), `DisableWhileRequest` names the elements disabled while a request is under way (htmx 4's `hx-disable`, htmx 2's `hx-disabled-elt`), `KeepOnServerError` leaves the page as it is on a 5xx response (htmx 4's `hx-status:5xx="swap:none push:false"`), and `ReloadOnError` loads a link or form whose request gets a 5xx status or no answer as a whole page (`data-partial-reload-on-error` with every connector, which the client of `exp/interactions` reads): A link is followed and a GET form submitted again by the browser itself. A request that is not safe to send twice (POST, PUT, PATCH, DELETE), such as a comment or a vote, is never sent again: if the server answered with an HTML error page, that page replaces the document, as a whole-page load would show it; without one, or without an answer, the page stays as it is and `partial:reload-on-error:failed` is dispatched on the form (`detail.method`, `detail.status`, 0 without an answer), so the application can react. An htmx 4 answer with a 5xx status and `HX-Reswap: none` leaves the page as it is, also for a POST: it has only out-of-band parts, such as the error flash message of `exterrors.WithFlash(fn)`, which htmx 4 swaps.

```go
connector.Attrs(conn, connector.Attr{Inherit: true, Boost: connector.BoostOn, Target: "#content", Swap: "outerHTML",
    DisableWhileRequest: "find button[type='submit']", KeepOnServerError: true})
// hx-boost:inherited="target:#content swap:outerHTML" hx-disable:inherited="find button[type='submit']"
// hx-status:5xx:inherited="swap:none push:false"
```

`SubmitConfirm` asks a question before a submit button sends its form, for a form whose buttons do different things: `data-turbo-confirm` with Turbo, else `data-partial-confirm`, which `exp/interactions`' browser client asks (`interactions.ClientScript()`). `connector.MergeAttrs(conn, attrs, attr, kinds...)` puts `attr` into a map of attributes another library built, replacing the connector's attributes of the same settings, and removing those of `kinds`. `connector.Apply(conn, w.Header(), connector.Response{PushURL: "false"})` sets response headers in one call. `connector.OOBAttrs(conn, "beforeend")` returns the attributes that swap an element out of band, which templates write with `{{ oobSwap "beforeend" }}`.

The `kinds` of `MergeAttrs` are `connector.AttrKind` values, one for each setting of `Attr`: `AttrConfirm`, `AttrBoost`, `AttrTarget`, `AttrSwap`, `AttrDisableWhileRequest`, `AttrKeepOnServerError`, `AttrSubmitConfirm` and `AttrReloadOnError`. This asks another question, or none when `question` is empty:

```go
attrs = connector.MergeAttrs(conn, attrs, connector.Attr{Confirm: question}, connector.AttrConfirm)
```

`connector.SetRequestTarget(conn, req, "comments")` makes a request ask for a target as the connector's client library would, the inverse of `GetTargetValue`, for tests and requests a server makes to itself.

Handlers that need to know more about a request ask the connector instead of reading protocol headers. `p.Connector()` returns the connector a partial uses:

```go
conn := root.Connector()
connector.IsRequest(conn, r)   // made by the client library, also for a whole page
connector.IsBoosted(conn, r)   // a boosted link or form
connector.CurrentURL(conn, r)  // the page the client shows, such as HX-Current-URL
connector.VaryHeaders(conn)    // the request headers that change responses
```

## Debug Helper
The `debug` template helper renders a styled diagnostic box using an embedded template. The box styles its classes with one `<style>` element of its own; under a Content-Security-Policy without `'unsafe-inline'`, add `debug.StyleHash()` to `style-src` in development (see Content-Security-Policy):

```gotemplate
{{ debug runtime . }}
```

Register the debug helper and stage globally, per wrapper, or per partial:

```go
root.SetFunc(debug.FuncMap())
root.Use(debug.Stage())
```

With `debug.WithFunc()`, the stage gives templates the helper bound to the render, so they write `{{ debug . }}`; `{{ debug runtime . }}` keeps working. `debug.WithFunc(debug.UnlessDefined())` leaves a `debug` function the application registers itself in place (other than `debug.Disabled()`), without a warning. In a text render, such as an e-mail subject, `debug` writes plain text. In production, `debug.Disabled()` is a helper that renders nothing, so templates keep their calls:

```go
if development {
    root.Use(debug.Stage(debug.WithFunc()))
} else {
    root.SetFunc(debug.Disabled())
}
```

## Server-Sent Events
SSE is a writer layer, not a connector. Use it after deciding which partials changed:

```go
events := sse.NewWriter(w)

notice := partial.NewID("notice", "notice.gohtml").
    SetDot(Notice{Message: "Saved"})

_ = events.PatchPartial(r.Context(), r, "#notice", notice)
_ = events.Signal("saved", true)
events.Flush()
```

`PatchPartial` renders the partial itself, whatever the request targets. A partial that is not in a tree renders with the functions, stages, file system and models of an application's root through `events.SetParent(root)`, which leaves both the partial and the root unchanged, so a part of a page streams without a render helper of its own:

```go
events := sse.NewWriter(w).SetParent(root)
viewers := partial.NewID("viewers", "viewers.gohtml").SetDot(view)
_ = events.PatchPartial(r.Context(), r, "#viewers", viewers)
```

`Flush` follows middleware wrappers that expose `Unwrap() http.ResponseWriter` and honors their `FlushError` methods.

The writer declares constants for expected headers and event names, such as `HeaderContentType`, `ContentTypeEventStream`, `EventPatch`, `EventSignal`, and `EventError`.

`exp/sse` ships a small browser client for these events, with no build step and no other library. It replaces the elements each patch's target selects with its HTML (outerHTML), dispatches `partial:signal` and `partial:error` as DOM events, and opens a stream for every element with `data-partial-stream="/url"` while it is in the page. Serve it from `clients.FS()` (see Serving The Browser Clients):

```go
mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(clients.FS())))
```

```html
<script src="/assets/partial-sse.js"></script>
<div data-partial-stream="/events"></div>
```

With htmx loaded, patched content is processed by htmx. The client works with every connector; `{{ stream }}` of `exp/interactions` with a connector that leaves the client to the page also opens a stream through it.

A stream follows its element: when the URL in `data-partial-stream` changes, the client opens a stream to the new one. With `data-partial-stream-hidden="30s"`, it closes the stream once the browser tab has been hidden that long (`partial:stream:paused`) and opens it again when the tab shows, dispatching `partial:stream:reconnect` on the element, so the page can fetch what it missed:

```html
<aside id="presence" data-partial-stream="/events/presence" data-partial-stream-hidden="30s"></aside>
```

With `data-partial-stream-client-id="tab"`, the client adds an id of the browser tab to the stream URL as that parameter, the same for the life of the page, so the server tells the tabs of one browser apart, such as for presence; scripts read it as `goPartialSSE.clientId`. A page restored from the back/forward cache opens its streams again, dispatching `partial:stream:reconnect`.

With `data-partial-stream-beacon="/presence"`, the client tells that address about the stream, so the server can tell a hidden tab, whose stream is closed, from one that is gone. It POSTs `clientId`, `params` (the query of the stream URL, with the client id) and `reason` as a form: `beat` every `data-partial-stream-beat="15s"` while the element is in the page, also while the tab is hidden (a 4xx answer stops them); `hidden` when the stream closes as the tab is hidden; `removed` when the element leaves the page and no element left reports to the same address, such as after a navigation to a page without the stream; `pagehide` when the page is left. The page-leaving reports go with `navigator.sendBeacon`. `sse.ReadBeacon(r)` reads one:

```go
beacon, err := sse.ReadBeacon(r)
if err != nil {
    http.Error(w, "invalid_beacon", http.StatusBadRequest)
    return
}
if beacon.Reason.Leaving() { // removed or pagehide
    presence.Leave(beacon.Params.Get("tab"))
} else { // beat or hidden
    presence.Visit(beacon.Params.Get("tab"), beacon.Params.Get("page"))
}
```

`beacon.Reason` is one of the `sse.BeaconReason` constants `sse.BeaconBeat`, `sse.BeaconHidden`, `sse.BeaconRemoved` and `sse.BeaconPagehide`; `Leaving()` is true for the last two.

Templates write the client's attributes with `{{ sseAttrs }}` (`sse.FuncMap()`), which writes nothing without a URL; in Go, `sse.ClientStream{...}.Attrs()`:

```gotemplate
<div{{ sseAttrs .PresenceStream "clientId" "tab" "hidden" "30s" "beat" "15s" "beacon" "/presence" }}>
```

### Streams Many Clients Follow
A `Broker` fans messages out to the streams that follow a topic, such as everyone looking at the same page. It keeps its state in memory, so it serves the streams of one process, and is safe for concurrent use:

```go
broker := sse.NewBroker(sse.BrokerOptions{MaxSubscribers: 1000, MaxPerKey: 10, Buffer: 1})
defer broker.Close()

// When something changes:
broker.Publish("page:/about", sse.Message{Event: "changed", Data: "/about"})
```

`BrokerOptions` limit it: `MaxSubscribers` caps the open subscriptions and `MaxPerKey` those that share a key, such as the streams of one user (zero is unlimited); `Subscribe` then fails with `sse.ErrTooManySubscribers`, and after `Close` with `sse.ErrBrokerClosed`. `Buffer` is how many messages a subscription holds while its stream is busy (default 16). `Publish` never blocks and reports how many subscriptions received the message; a full subscription misses it, so a buffer of 1 coalesces "something changed" notifications into one.

A stream handler subscribes, and `Writer.Serve` streams the subscription's messages until the request ends, the subscription closes or a write fails, flushing after each:

```go
func stream(w http.ResponseWriter, r *http.Request) {
    subscription, err := broker.Subscribe(userID(r), "page:"+r.URL.Query().Get("page"))
    if err != nil {
        http.Error(w, "too_many_streams", http.StatusTooManyRequests)
        return
    }
    defer subscription.Close()

    events := sse.NewWriter(w).SetParent(root)
    _ = events.Serve(r.Context(), subscription, sse.ServeOptions{
        Heartbeat:       25 * time.Second,
        BeforeHeartbeat: func() error { return checkSession(r) },
        Handle: func(stream *sse.Writer, message sse.Message) error {
            return stream.PatchPartial(r.Context(), r, "#viewers", viewersPartial(r))
        },
    })
}
```

`ServeOptions.Heartbeat` writes a comment at that interval (`Writer.Heartbeat` writes one by hand), which keeps proxies from closing an idle stream and notices a client that went away; `BeforeHeartbeat` runs first, and its error ends the stream, such as when the session ended. `Handle` writes a message, such as a partial rendered for this client; without it the message's `ID`, `Event` and `Data` are written. `Serve` returns nil when the subscription closed, and clears the connection's write deadline, which an `http.Server`'s `WriteTimeout` would otherwise use to end every stream. `Subscription.Messages()` is the queue for code that streams on its own.

### Serving The Browser Clients
The clients of `exp/flash`, `exp/sse` and `exp/interactions` are files of one file system, `clients.FS()` (`partial-flash.js`, `partial-sse.js`, `partial-interactions.js`), for applications that serve, fingerprint and version them with their other assets; each package's `ClientScript()` returns its client for a bundle of the application's own. `clients.All()` lists them with the names scripts import them by, for an import map:

```go
for _, client := range clients.All() {
    importMap.WithFile(client.Import, client.File) // "partial-flash", "partial-flash.js", …
}
```

### A Confirmation Dialog Of The Page
The client of `exp/interactions` asks the question of a submit button with `"submitConfirm"`, and htmx's `hx-confirm` (htmx 2 and 4), in a `<dialog data-partial-confirm-dialog>` of the page when there is one, instead of with the browser's `window.confirm`: some embedded browsers never show that and answer no, and the page's dialog has the application's styles and translated buttons. The question goes into the element with `data-partial-confirm-question`, as text. A button in the dialog with a `value` closes it with that value, also without a `<form method="dialog">`; `confirm` is yes, and Escape or any other value is no:

```html
<dialog data-partial-confirm-dialog aria-labelledby="confirm-question">
    <p id="confirm-question" data-partial-confirm-question></p>
    <button type="button" value="cancel" autofocus>Cancel</button>
    <button type="button" value="confirm">Confirm</button>
</dialog>
```

Without the dialog, nothing changes: the questions use `window.confirm`. The questions of Unpoly and Turbo stay theirs, and so does an htmx 4 question that starts with `js:`. `window.goPartialInteractions.ask(question)` asks a question of the application's own the same way and resolves to `true` for yes.

## Content-Security-Policy
go-partial's output works under a strict policy, without `'unsafe-inline'` for styles or scripts and without `'unsafe-eval'`, such as `default-src 'self'; script-src 'self'; style-src 'self'`. Its browser clients are files (see Serving The Browser Clients), which `'self'` covers, and none of them uses `eval` or `new Function`. What it writes inline:

| Output | Inline | Under a strict policy |
| --- | --- | --- |
| `ext/errors` built-in error page | one `<style>` element | add `exterrors.StyleHash()` to `style-src` |
| `ext/errors` built-in error fragment | nothing: classes only | serve `exterrors.StylesheetFS()` and link its `partial-errors.css`, or style it with the application's CSS; unstyled it reads as a heading, a paragraph and a list |
| `ext/debug` box (development) | one `<style>` element per box, the same each time | add `debug.StyleHash()` to `style-src` in development; without it the value shows unstyled |
| `exp/head` region markers | empty `<template>` elements | nothing to do |

The hashes are `'sha256-…'` sources with their quotes, ready for a policy. They change only when go-partial changes the style, which RELEASE_NOTES.md then says; take them from the functions rather than copying them:

```go
policy := "default-src 'self'; script-src 'self'; style-src 'self' " + exterrors.StyleHash()
if development {
    policy += " " + debug.StyleHash()
}
```

`style-src` covers `<style>` elements; a policy that sets `style-src-elem` needs the hash there instead. An application that answers with an error template of its own (`WithTemplate`, `WithFragmentTemplate`) without inline style needs no hash.

Things that run under the page's policy rather than the response's:

- **The interactions client's `reloadOnError`** writes the HTML error page of a failed POST into the page that sent it (`document.open`/`write`). That page keeps the policy of the page it replaces, so every page with `data-partial-reload-on-error` needs the error page's hash in its own `style-src`: `exterrors.StyleHash()` for the built-in page, or the hash of the application's own error page's inline style, if it has any.
- **Partial responses and SSE patches** are inserted into the page, so their `style="…"` attributes are blocked (`style-src-attr`) and an inline `<style>` or `<script>` in them applies only when the page's policy has its hash. Style swapped and streamed content with classes, and keep inline scripts out of fragments.
- **Out-of-band `<script type="application/ld+json">`** and other data blocks are never executed, so the policy does not affect them.

With htmx, avoid what htmx evaluates as JavaScript, which `'unsafe-eval'` would have to allow:

- `hx-on*` attributes, such as `hx-on:click`;
- trigger filters, such as `hx-trigger="click[ctrlKey]"`;
- `js:` and `javascript:` values, such as in `hx-vals`, `hx-headers` and `hx-confirm`. Plain JSON in `hx-vals` and `hx-headers`, and a plain question in `hx-confirm`, are fine.

htmx 4's indicator CSS (`includeIndicatorCSS`) is a constructable stylesheet (`adoptedStyleSheets`), which CSP does not block. htmx 2 inserts a `<style>` for its indicators: set `"includeIndicatorStyles": false` in its `htmx-config` meta element and style `.htmx-indicator` and `.htmx-request` in the application's CSS, or give it `inlineStyleNonce` on pages that are never cached.

## Flash Messages Across Redirects
`exp/flash` keeps messages for the request after a redirect in a cookie, with their level:

```go
cookie := flash.Cookie{Key: key}

cookie.Persist(w, flash.Success("Saved"))
http.Redirect(w, r, "/items", http.StatusSeeOther)
```

```go
// For every request, such as in middleware:
r = cookie.Restore(w, r)
```

`Restore` moves the messages into the request's flash store, where `{{ flash }}` shows them, and clears the cookie. The fields of `flash.Cookie` configure the cookie; its `Key` signs it, so only messages the application persisted are shown, and the zero `Cookie` is not signed.

## Flash Messages In The Browser
`exp/flash` ships a browser client for its messages, with no build step and no other library, which `clients.FS()` serves (see Serving The Browser Clients):

```go
mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(clients.FS())))
```

```html
<script src="/assets/partial-flash.js"></script>
```

It hides each message after a time that depends on its level (success and info 5s, warn 8s, error never), which `data-flash-autohide-<level>="3s"` or `data-flash-autohide="3s"` on an ancestor, or `data-flash-autohide` on a message, changes (`"off"` keeps it). A message stays while the pointer or keyboard focus is on it, goes when an element in it with `data-flash-dismiss` is clicked or Escape is pressed in it, and dispatches `partial:flash` as it appears. With `data-flash-request-error="Something went wrong"` on the flash target, failed requests of htmx (2 and 4), Unpoly and Turbo, and `partial:error` events of `exp/sse`, show that text as an error; scripts call `goPartialFlash.show(level, text)`. A response with an error status that htmx swaps into its target, such as a form re-rendered with its validation errors (422) with htmx 4, shows itself and is no failed request: with htmx 4 the client decides at `htmx:finally:request`, once htmx swapped the response or not, and with htmx 2 by the `shouldSwap` of `htmx:beforeSwap`.

The messages the client shows itself look like those of the server: `{{ flashTemplate }}` in the flash target renders the application's flash template (`flash.WithTemplate`) as a `<template data-flash-template="error">` per level, which the client clones. More attributes on the flash target:

```html
<div id="flash-messages"
     data-flash-request-error="The request failed. Please try again."
     data-flash-request-error-replace
     data-flash-request-error-body
     data-flash-request-error-skip-flash>
    {{ flashTemplate }}
    {{ flash }}
</div>
```

`-replace` keeps one request error at a time, the next request removing it; `-body` shows the `text/plain` body of a refused request (4xx) when it is a sentence, without htmx swapping it; `-skip-flash` shows nothing more for a failed response that brings its own message.

The flash stage writes these attributes itself with `flash.WithRequestErrors`, in `{{ flashTarget }}` and in `{{ flashTargetAttrs }}`, which also writes the target's id and takes a text of the template's own, such as a translated one. `{{ flashTemplate }}` writes nothing when its partial renders out of band, so a flash target that responses add their messages to needs no condition:

```go
root.Use(flash.Stage(flash.WithRequestErrors(flash.RequestErrors{
    Text: "The request failed. Please try again.", Replace: true, Body: true, SkipFlash: true,
})))
```

```gotemplate
<div {{ flashTargetAttrs (t "The request failed. Please try again.") }} {{ oobAttr "beforeend" }} class="toasts">
    {{ flash }}
    {{ flashTemplate }}
</div>
```

A handler refuses a request with messages through `flash.Respond`: a response that adds them to the flash target out of band, with the template of the flash stage, for requests of the client library, and their text as `text/plain` otherwise:

```go
_ = flash.Respond(w, r, root, http.StatusForbidden, flash.Error("You may not do this."))
```

`flash.RenderOOB(ctx, messages...)` renders such an addition from a render stage.

## Slots, Regions And Fragments
`exp/slots` places named parts of a page with `{{ slot "name" }}`. Slots are looked up from the partial that places them through its ancestors, so a content template can place a slot of the layout. Options decide how partial requests treat a slot:

```go
root.Use(slots.Stage())

// A region: placed on the page, and sent out of band with every partial response.
slots.Set(root, "cart", cart, slots.OOB())

// A fragment: answered on its own when a request targets its ID and the page
// can render it, else left to the target fallback.
slots.Set(root, "comments", comments, slots.CheckContracts(), slots.OmitOOB())
```

`CheckContracts`, `OmitOOB` and `Deferred` make a slot standalone: the stage answers requests for its ID. A slot without options is registered as a child of the partial it is set on, as before.

A deferred slot loads after the page: `{{ slot }}` renders a placeholder that requests the slot from the page's address through the connector, such as `hx-get` with `hx-trigger="load"`, and the request renders the slot alone, with its lazy dot. `slots.Placeholder(p)` renders a placeholder of the application's own with a `slots.PlaceholderData` dot, and `slots.Data(v)` gives that dot data of the slot's own as `.Data`, such as a panel's heading, so one placeholder template serves several slots:

```go
slots.Set(root, "recent", recent.SetDotSource(partial.DotFuncOf(loadRecent)), slots.Deferred(), slots.OmitOOB())
slots.Set(root, "attention", attention, slots.Placeholder(panel), slots.Data(Heading{Title: "Needs attention"}), slots.OmitOOB())
```

The template that places a slot can defer it too, whatever its options, with `{{ deferredSlot "name" }}`, or `{{ deferredSlot "name" "reveal" }}` to load it when it scrolls into view, such as one fragment of a directory that `slots.Glob` set alike:

```gotemplate
<aside>{{ deferredSlot "popular" "reveal" }}</aside>
```

In Go, `slots.DeferredSlot(name, ctx, "reveal")` does what `{{ deferredSlot }}` does from a render context, such as in a template function of the application's own.

A region can send only the elements of it that a request changes: with `SetOOBElements(ids...)` on its partial, a partial response sends the elements of its output with those IDs, each out of band on its own with the connector's attribute, instead of the whole region; `SetOOBElementsFunc(fn)` chooses them per request, such as the menu items whose state changes from the page the visitor comes from. See Using Out-of-Band (OOB) Partials. A region placed in `<head>`, such as the page's canonical and alternate links, is made one with `head.Region(p)` of `exp/head`; see A Region In The Head.

`slots.Glob(root, "regions/*.gohtml", opts...)` sets a slot for every file of a directory, named after the file (`menu` for `regions/menu.gohtml`), found once per file system with the template cache; `slots.Setup(fn)` configures each slot's partial, such as its dot. `{{ requiredSlot "name" }}` fails the template for a slot that is not set; `{{ hasSlot }}` checks optional ones.

## Basic Usage
Here's a simple typed example. The template owns the contract, the handler supplies the model.

### 1. Create A Root Blueprint
The root partial holds shared render configuration and is cloned per request.

```go 
root := partial.NewID("shell", "templates/shell.html").
    SetConnector(connector.NewHTMX(nil)).
    SetFileSystem(os.DirFS("web")).
    UseTemplateCache(true).
    SetEvents(events).
    SetFunc(template.FuncMap{
        "money": formatMoney,
    })

```

### 2. Compose The Root Partial Per Request
A wrapper partial can render one configured content child through `{{ content }}`.
Create the tree inside the handler when the content, wrapper, dot data, or
registered children are request-specific. Clone the configured root blueprint
per request; per-request wiring should stay on the clone.

### 3. Define Partials
Create `Partial` instances for the content and any other components.

```go 
type ContentPage struct {
    PageTitle string
    Message string
}

type ShellPage struct {
    AppName string
}

func handler(w http.ResponseWriter, r *http.Request) {
    // Create the main content partial
    content := partial.NewID("content", "templates/content.html").
        SetDot(ContentPage{
            PageTitle: "Home Page",
            Message:   "Welcome to our website!",
        })
    
    // Clone the shared shell and attach request-specific content.
    page := root.Clone().
        SetDot(ShellPage{AppName: "My Application"})
    page.SetContent(content)
    
    output, err := partial.RenderWithRequest(r.Context(), r, page)
    if err != nil {
        http.Error(w, "An error occurred while rendering the page.", http.StatusInternalServerError)
        return
    }
    w.Write([]byte(output))
}
```

## Template Files
templates/shell.html
```html
<!DOCTYPE html>
<html>
<head>
    <title>{{.AppName}}</title>
</head>
<body>
    {{ content }}
</body>
</html>
```
templates/content.html
```html 
<h1>{{ .PageTitle }}</h1>
<p>{{ .Message }}</p>
```

Note: When a wrapper partial wraps content, it renders the configured route partial by calling `{{ content }}`.


### Accessing Data in Templates

Use `SetDot` for application data:

```gotemplate
{{ .PageTitle }}
{{ .Message }}
```

For shared application values, put them on a typed model and declare them with go-clue `@model`:

```gotemplate
{{/*
@model App github.com/example/app.AppInfo
*/}}

{{ App.AppName }}
```

```go
wrapper.SetModel(appInfo)
```

When `SetDot` is used, request-specific values are available through helpers instead of fields on dot (`locale` with the stage of `exp/localization`, `csrf` with that of `exp/csrf`):

```gotemplate
{{ ctx.URL.Path }}
{{ url.Path }}
{{ request.Method }}
{{ locale }}
{{ csrf.Key }}
{{ basePath }}
```

## Typed Contracts
go-partial can register go-clue typed root declarations before parsing templates. The declaration owns the template name, while the controller supplies the matching Go value:

```gotemplate
{{/*
@model Page github.com/example/app.DashboardPage
*/}}

<h1>{{ Page.Title }}</h1>
```

```go
content := partial.NewID("content", "templates/dashboard.gohtml").
    SetModel(page)
```

`SetModel` appends values to the typed roots already inherited through the partial tree. Use it when a partial adds local models on top of parent models. `SetContract(annotation, values...)` does the same thing for custom annotation names.

When more than one root has the same Go type, bind by name:

```gotemplate
{{/*
@interaction LikesPoll github.com/donseba/go-partial/exp/interactions.Interaction
@interaction LikeButton github.com/donseba/go-partial/exp/interactions.Interaction
*/}}

{{ poll runtime LikesPoll }}
{{ refresh runtime LikeButton }}
```

```go
content.SetContract("interaction",
    interactions.NewPoll("/posts/42/likes").As("LikesPoll").Every(5*time.Second),
    interactions.NewRefresh("/posts/42/likes").As("LikeButton").Target("#likes"),
)
```

Interaction roots are normal go-clue contracts registered with `SetContract("interaction", values...)`. Use `As(name)` when the endpoint does not naturally produce the contract name; otherwise the last endpoint segment is capitalized, so `/stats` becomes `Stats`.

Contract names cannot collide with go-partial's own helpers, such as `partial`, `content`, `ctx`, or `url` (see Template Functions): a template that declares such a root fails to render.

### Checking Contracts
`CheckContracts` tells whether a partial can render with the values it has, without rendering it: every typed root its templates declare finds one value on the partial or its parents, and its dot has the `@dot` type of its first template.

```go
if err := fragment.CheckContracts(); err != nil {
    // this page cannot answer for the fragment
}
```

`runtime.CheckContracts(p)` checks a partial that is not attached yet, as a child of the partial being rendered; `exp/slots` uses it with `slots.CheckContracts()`. `partial.ParseDeclarations(sources...)` returns the `@model`, other typed and `@dot` declarations of template sources, for tools and templates rendered outside go-partial.

### Checking Templates
`CheckTemplates` parses every template file of a partial's file system, in every folder, with the functions its renders have: those of `SetFunc` on the partial and its ancestors, go-partial's own helpers, those its render stages give (`Use`) and the typed roots the files declare. It reports every problem, not only the first: a file that does not parse, every call of a function the partial does not have, and every `{{ template }}` that names a template no file defines. Run it when the application starts, so a template that would fail its renders is known before the first request:

```go
problems, err := root.CheckTemplates()
if err != nil {
    return err // reading the file system failed
}

for _, problem := range problems {
    log.Print(problem)
}
```

```text
layout.gohtml:2:4: function "newsleterForm" not defined; did you mean "newsletterForm"?
layout.gohtml:3:13: no such template "footer"
regions/menu.gohtml:1: unexpected EOF: an {{if}}, {{range}}, {{with}}, {{block}} or {{define}} lacks its {{end}}
```

Each `partial.TemplateProblem` has the `File`, `Line` and `Column` (0 when the parser does not say), the `Func` or `Template` it names, a `Suggestion` with the closest defined name when one is close, and the `Message`; `Position()` gives `file:line:col`, and `Error()` the line above. For an `Overlay`, `Layer` is the index of the layer the file comes from, the first layer 0. Without patterns every `*.gohtml` file is checked; `CheckTemplates("*.tmpl")` chooses files by their base name, as `path.Match` does. A template named in any checked file counts as defined, as templates render with the templates of other files, such as those of `SetIncludes`. The problems are not errors: the error is only that of reading the file system.

go-partial keeps interaction template helpers in `github.com/donseba/go-partial/exp/interactions`.
They are opt-in:

```go
import "github.com/donseba/go-partial/exp/interactions"

root.SetFunc(interactions.FuncMap())
```

`interactions.AttrsFuncMap()` holds only the helpers that write attributes on an element of the template's own (`revealAttrs`, `submitAttrs`, `clientAttrs`, `noBoost` and the like), for applications that leave generic names such as `poll`, `on` or `async` to functions of their own.

Those functions include go-clue signature metadata for the overloads that normal
Go function declarations cannot express. The repository `.go-doc/config.json`
wires them into editor tooling for this repo; go-clue, formerly go-doc, still
reads that file and the `go-doc:` comments. Helper calls such as
`{{ async runtime "/stats" }}` and `{{ async runtime Stats }}` can both be
completed and validated. The `runtime` argument is a per-render value injected
by go-partial; it exposes request context, the active connector, partial tree,
and diagnostic stages to helpers without package globals.

Use the inline endpoint form when the endpoint is naturally local to the
template, especially inside loops:

```gotemplate
{{ range .Rows }}
    {{ async runtime "/async/row/:row" "row" .ID }}
{{ end }}
```

Use a named interaction when Go should own stable IDs, targets, intervals,
events, placeholders, or reuse:

```gotemplate
{{/*
@interaction LikesPoll github.com/donseba/go-partial/exp/interactions.Interaction
*/}}
{{ poll runtime LikesPoll }}
```

```go
content.SetContract("interaction",
    interactions.NewPoll("/posts/42/likes").As("LikesPoll").Every(5*time.Second),
)
```

To put an interaction on an element of the template's own, such as one that only polls while a job runs, use the attribute variants `asyncAttrs`, `revealAttrs`, `pollAttrs`, `onAttrs`, `streamAttrs` and `refreshAttrs`; `submitAttrs` and `liveAttrs` make a form post and an input request as the user types (see [TEMPLATE_FUNCTIONS.md](TEMPLATE_FUNCTIONS.md)). Without a target or id they target the element itself:

```gotemplate
<ul id="jobs"{{ if .Running }}{{ pollAttrs runtime "/jobs" "3s" }}{{ end }}>…</ul>
```

`"select"` picks the part of the response to swap (`hx-select` for htmx), and options whose names start with `hx-`, `up-`, `data-` or `aria-` are written on the element as given, replacing the connector's attribute of the same name:

```gotemplate
<div {{ pollAttrs runtime "/inbox" "every" "10s" "select" "#messages" "hx-indicator" "#spinner" }}></div>
```

## Template Composition
For repeated sections that should be understood by go-clue and also render as HTMX targets, prefer native templates plus `SetDot`. The parent owns the loop, and the nested template receives the row as normal dot data:

```gotemplate
{{/*
@dot github.com/example/app.TablePage
*/}}
{{ range .Rows }}
    {{ template "row.html" . }}
    {{/* or: {{ template "/templates/row.html" . }} */}}
{{ end }}
```

```html
<tr id="row-{{ .ID }}">
    <td>{{ .Name }}</td>
</tr>
```

Register the row partial on the parent so the parent parse can see the row template and so an HTMX request can still target a row by ID; the resolver answers through the stage of `exp/target`:

```go
rowPartial := partial.NewID("row", "templates/row.html")
table.With(rowPartial).Use(target.Stage())
target.WithResolver(table, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
    row := findRowForTarget(target)
    return partial.NewID(target, "templates/row.html").SetDot(row), true
})
```

For wrapper partials, render the route content through `content`:

```gotemplate
<main>{{ content }}</main>
```

Avoid using `partial` as a general row-composition helper in new code. Native `template` calls keep the template idiomatic and give go-clue the strongest type information. Use `partial` when you intentionally want to render a template path through go-partial's render path:

```gotemplate
{{ partial runtime "templates/notice.gohtml" .Notice }}
```

Keep registered partials for HTMX targets, OOB output, selection/action rendering, and places where the browser can request a stable partial ID.

## Using Out-of-Band (OOB) Partials
Out-of-Band partials allow you to update parts of the page without reloading:

### Defining an OOB Partial
```go
type Footer struct {
    Text string
}

// Create the OOB partial
footer := partial.New("templates/footer.html").ID("footer")
footer.SetDot(Footer{
    Text: "This is the footer",
})

// Add the OOB partial
p.WithOOB(footer)
```

A partial response sends the out-of-band partials in a fixed order, so the same page gives the same bytes on every render, which keeps ETags and response caches stable: those of the partial nearest the target first, then those of each ancestor up to the root, and each partial's own in the order `WithOOB` registered them. Registering an ID again keeps its place. `slots.Set` with `slots.OOB()` registers in the order it is called, and `slots.Glob` in the order of the file names, such as `head`, `menu`, `title` for a `regions` directory.

A region goes with partial requests for the partials below the one it is registered on, such as the content of a layout. With `SetAlwaysSwapOOB(true)`, which `slots.OOB()` sets, it also goes with a request for that partial itself, such as a notice a page registers for a toast container of the layout: out of band after the partial's output, unless the output renders the region partial already, as a layout does with `{{ slot }}`. Markup of the region written another way, such as with `{{ template }}`, is not seen, so the region then comes twice.

```go
page.WithOOB(partial.NewID("notice", "templates/notice.gohtml").SetAlwaysSwapOOB(true))
```

### Using OOB Partials in Templates
In your templates, you can use the `oobAttr` function to conditionally render OOB attributes: the connector's, `hx-swap-oob` for htmx and `up-hungry` for Unpoly (see `connector.OOBAttrs`).

templates/footer.html
```html
<div{{ oobAttr }} id="footer">{{ .Text }}</div>
```

### Sending Only Some Elements Of A Region
`SetOOBElements(ids...)` makes a region that goes out of band send only the elements of its output with those IDs, each carrying the connector's out-of-band attribute, rather than its whole output, such as the items of a menu that mark the page being shown:

```go
menu := partial.NewID("menu", "templates/menu.gohtml").SetOOBElements("menu-about", "menu-contact")
p.WithOOB(menu)
```

An element inside another one it sends goes along with that one. The region's own element, the one with `{{ oobAttr }}`, is not sent unless its ID is one of them. When none of the IDs is in the output, or the end of an element cannot be told, such as an `<li>` without its `</li>`, the whole region goes, as without IDs. On a full page the region renders whole.

`SetOOBElementsFunc(fn)` chooses the IDs per request. `fn` is called only when the region goes out of band, once per response; the request headers it names in `Vary` go to the response's `Vary` header then, and only then, so a full page or a fragment request does not vary with them. `connector.CurrentURLHeader(conn)` names the header in which the client library sends the URL of the page it shows, `HX-Current-URL` for htmx, which `connector.CurrentURL(conn, r)` reads:

```go
menu.SetOOBElementsFunc(func(r *http.Request) partial.OOBElements {
    return partial.OOBElements{
        IDs:  changedItems(connector.CurrentURL(conn, r), r.URL),
        Vary: []string{connector.CurrentURLHeader(conn)},
    }
})
```

The two replace each other. `partial.AddVary(w.Header(), names...)` adds request headers to a response's `Vary` header, unless it lists them already.

A connector of your own whose client library sends such a header implements `connector.CurrentURLHeaderNamer`, `CurrentURLHeader() string`, as the htmx connectors do; `connector.CurrentURLHeader` returns "" for a connector without it, or a nil one.

### A Region In The Head
A region of a layout placed in `<head>`, with the page's canonical link, description, alternate and feed links or structured data, cannot go out of band as other regions do: `<head>` holds no element to wrap it, and the number of its elements changes from page to page. `head.Region(p)` of `exp/head` makes a partial such a region. On a full page its output sits between two empty `<template>` markers (`data-partial-head` and `data-partial-head-end`, with the partial's ID); out of band it replaces whatever is between them, as a group, so the elements of the page left are removed rather than kept next to the new ones:

```go
region := head.Region(partial.NewID("head", "regions/head.gohtml").SetDot(page))
slots.Set(root, "head", region, slots.OOB())
```

```gotemplate
<head>
    <meta charset="utf-8">
    {{ slot "head" }}
    <link rel="stylesheet" href="/site.css">
</head>
```

With htmx 2 and 4 the region goes as two out-of-band swaps that need no extension: `hx-swap-oob="delete:…"` of the elements between the markers, and `hx-swap-oob="beforebegin:…"` of a `<div>` with the new elements before the end marker. A `<title>` in the region's output goes on its own, before the markers, where htmx sets the document's title from it. The region needs no ID and no `{{ oobAttr }}`; partials it renders are not affected. With a connector whose out-of-band attribute is not `hx-swap-oob`, such as Unpoly's, the region's output goes as it is.

A back or forward navigation that htmx restores from its history replaces the body and the title only: the head keeps the region's elements of the page left. With htmx 2, `"historyCacheSize": 0` and `"refreshOnHistoryMiss": true` in its `htmx-config` meta element make such a navigation load the whole page instead.

## Template Functions
You can add custom functions to be used within your templates:

```go
import "strings"

// Define custom functions
funcs := template.FuncMap{
    "upper": strings.ToUpper,
}

// Merge the functions into this partial tree
p.SetFunc(funcs)
```

`SetFunc` registers helpers in the current scope. Function names inherited from the Root partial or parent partial remain available, and protected go-partial helper names cannot be overwritten.

Functions whose first parameter is a `context.Context` receive the context of the render; templates call them without it:

```go
root.SetFunc(template.FuncMap{
    "posts": func(ctx context.Context, section string, limit int) ([]Post, error) {
        return store.Latest(ctx, section, limit)
    },
})
```

```gotemplate
{{ range posts "news" 3 }}<li>{{ .Title }}</li>{{ end }}
```

The context is the one passed to `Render`, `RenderWithRequest` or `Write`, with what render stages added. Each function is bound once per render tree, however many partials call it. For templates go-partial does not execute, such as the text part of an e-mail, `partial.BindContext(ctx, funcs...)` binds such functions the same way.

`partial.RuntimeFrom(ctx)` gives such a function the runtime of the partial whose template calls it, so it can render a component as a true child of that partial, with its models, functions, stages and failure policy:

```go
"card": func(ctx context.Context, card Card) (template.HTML, error) {
    runtime, ok := partial.RuntimeFrom(ctx)
    if !ok {
        return "", errors.New("card: not rendering")
    }
    return runtime.RenderPartial(partial.NewID("card", "card.gohtml").SetDot(card))
},
```

`partial.ReservedFuncNames(stages...)` lists the names `SetFunc` cannot give templates: go-partial's helpers and the functions the given stages give templates, such as `locale` for `localization.Stage()`. `partial.IsReservedFuncName(name, stages...)` also reports names starting with an underscore.

`SetFunc` warns with a `func.protected` event when it is given a function templates never call: one named like a go-partial helper, which it ignores, or like a function a stage of the partial or its ancestors gives templates; `Use` warns about registered functions a new stage replaces. A stage's own stand-ins, such as `slots.FuncMap()` for `slots.Stage()`, raise no warning: they come from the package the stage names in `RenderStageHooks.Package`, which a package that offers stand-ins for its stage's functions sets, such as to `reflect.TypeFor[T]().PkgPath()` for a type `T` of its own. Set the event sink before the functions to receive these warnings.

go-partial always reserves the helpers it injects for rendering and request state:
`runtime`, `partial`, `content`, `ctx`, `request`, `url`, `basePath`, the URL
helpers `urlIs`, `urlStarts`, `urlContains`, `urlPath` and `joinPath`, and the
OOB helpers `oob`, `oobAttr` and `oobSwap`. The helpers of a stage, such as
`locale` (`exp/localization`), `csrf` (`exp/csrf`), `targetIs`, `selectionIs`
and `actionIs` (`exp/target`, `exp/selection`, `exp/actions`), `slot` or
`flash`, are reserved while the stage is in use. Generic helpers such as `dict`,
string helpers, and date helpers are ordinary template functions and may be
replaced.

Optional helper providers live under `exp/`:

```go
import (
    "github.com/donseba/go-partial/exp/flash"
    "github.com/donseba/go-partial/exp/interactions"
    "github.com/donseba/go-partial/exp/templatehelpers"
)

root.SetFunc(
    flash.FuncMap(),
    interactions.FuncMap(),
    templatehelpers.StringFuncMap(),
    templatehelpers.CollectionFuncMap(),
)
root.Use(flash.Stage())
```

The helpers of `interactions` and `templatehelpers` work with `SetFunc` alone. Those of `flash.FuncMap()` render nothing without `flash.Stage()`, which gives templates the real ones; the map gives go-clue their signatures.

For go-clue, point provider discovery at the same packages:

```json
{
  "providers": [
    "github.com/donseba/go-partial/exp/flash",
    "github.com/donseba/go-partial/exp/interactions",
    "github.com/donseba/go-partial/exp/templatehelpers"
  ]
}
```

### Usage in Template:
```html
{{ upper .Message }}
```

### Using a Custom File System
If your templates are stored in a custom file system, set it with `SetFileSystem`:

```go
import (
    "embed"
)

//go:embed templates/*
var templatesFS embed.FS

p.SetFileSystem(templatesFS)
```

If you do not use a custom file system, the package will use the default file system and look for templates relative to the current working directory.

### Templates From Memory And Overlays
`partial.Sources` is a file system of template sources held in memory, for markup a package ships, and `partial.Overlay` looks files up in each layer in turn. Together they let applications replace a default template with a file of their own:

```go
var defaults = partial.Sources{
    "site/share.gohtml": `<nav class="share">{{ range .Links }}<a href="{{ .URL }}">{{ .Label }}</a>{{ end }}</nav>`,
}

share := partial.NewID("share", "site/share.gohtml").
    SetFileSystem(partial.Overlay(themeFiles, defaults)).
    SetDot(bar)
```

Partials, `{{ partial }}`, `{{ template }}`, typed models and the template cache work as with any file system. Directories list the files of every layer. The cache tells file systems apart by identity: create `Sources` once rather than per render. Overlays are identified by their layers, so an overlay may be built per render.

`partial.Layers(fsys)` returns the layers of an overlay, in the order it looks files up in, and any other file system alone. Use it to add a layer below an overlay without nesting one overlay in another:

```go
fsys := partial.Overlay(append(partial.Layers(theme), defaults)...)
```

To render a template a file defines, rather than the file itself, such as the `html` block of an e-mail template, add a source that calls it and list the file after it:

```go
entry := partial.Sources{"mail-html.gohtml": `{{ template "html" . }}`}
html := partial.NewID("mail", "mail-html.gohtml", "mail/confirmation.gohtml").
    SetFileSystem(partial.Overlay(entry, themeFiles)).
    SetDot(data)
```

## Rendering Tables and Dynamic Content
For tables and repeated fragments, prefer native Go templates plus `SetDot`. The parent receives a typed page model, ranges over rows, and calls the row template with `{{ template "row.html" . }}`. That keeps the template readable for go-clue while go-partial still knows the row partial ID for HTMX target requests.

Example: Rendering a Table with Dynamic Rows

templates/table.html
```html
<table>
    {{ range .Rows }}
        {{ template "row.html" . }}
    {{ end }}
</table>
```

templates/row.html
```html
<tr id="row-{{ .ID }}">
    <td>{{ .Name }}</td>
</tr>
```

Go Code:
```go
type Row struct {
    ID   int
    Name string
}

type TablePage struct {
    Rows []Row
}

rowPartial := partial.NewID("row", "templates/row.html")
tablePartial := partial.NewID("table", "templates/table.html").
    SetDot(TablePage{Rows: rows})
tablePartial.With(rowPartial).Use(target.Stage())

target.WithResolver(tablePartial, func(ctx context.Context, r *http.Request, target string) (*partial.Partial, bool) {
    row, ok := findRowForTarget(target)
    if !ok {
        return nil, false
    }
    return partial.NewID(target, "templates/row.html").SetDot(row), true
})

out, err := partial.RenderWithRequest(r.Context(), r, tablePartial)
```

If the child does not need custom configuration, use `WithTemplate` as shorthand:

```go
tablePartial := partial.NewID("table", "templates/table.html").
    WithTemplate("templates/row.html").
    SetDot(TablePage{Rows: rows})
```

This is equivalent to `tablePartial.With(partial.NewID("row", "templates/row.html"))`.

## Template Data
In your templates, prefer this model:

- **{{.}}**: Your app model when the partial uses `SetDot`.
- **Typed roots**: Additional typed values registered with `SetModel` or `SetContract`.
- **{{ctx}}**, **{{request}}**, **{{url}}**, **{{locale}}**, **{{csrf}}**, **{{basePath}}**: request-aware helpers that stay available when `SetDot` changes `.` (`locale` and `csrf` with the stages of `exp/localization` and `exp/csrf`).

go-partial does not wrap your model in `.Data`, `.App`, `.Shell`, or `.Global`. Shared application values should be explicit typed roots, for example `SetModel(appInfo)` with a matching go-clue declaration. Request-scoped values live behind helper functions so changing dot never hides them.

## Concurrency and Template Caching
Configure reusable root partials, functions, render stages, headers, and filesystems before serving requests. Clone before adding request-specific content or dot data. After configuration, `partial.RenderWithRequest` and `partial.Write` can be called concurrently on cloned partial trees. Request-specific values such as `request`, `url`, `ctx`, `runtime`, stage values, selected targets, and template helper bindings are scoped to the active render and are not stored on the reusable partial configuration.

In HTTP handlers, pass the request context, normally `r.Context()` or a context derived from it. That lets cancellation, deadlines, localization, CSRF state, event sinks, metrics IDs, and other middleware values follow the render. `partial.Render(ctx, p)` without a request is for tests, offline rendering, and small utilities.

Do not call configuration methods such as `SetFunc`, `Use`, `With`, `SetDot`, `SetFileSystem`, `SetConnector`, or `SetResponseHeaders` while the same tree is being rendered. Build or clone a separate tree when configuration needs to change at runtime.

Mutable builder and stream/session helpers are intentionally single-owner values. Do not share `connector.ResponseBuilder`, `connector.Trigger`, `connector.Swap`, `sse.Writer`, or `pageflow.SessionData` across goroutines without your own synchronization.

The package also includes concurrency safety measures for template caching:

- Parsed templates are cached by the configured partial tree.
- Template files are scanned once per file system and tree root, so partials built per request under a configured parent (`SetParent`) read no files after the first.
- Clones share the registered functions until either registers more.
- Mutexes prevent duplicate parsing for the same Root partial/template/function shape.
- Cached templates are rebound with request-specific functions per render.
- Rendered HTML is not cached.
- Set `UseTemplateCache` to `true` to enable parsed template caching; the partials rendered under a partial inherit its setting.

```go
root.UseTemplateCache(true)
```

## Handling Partial Rendering via HTTP Headers
You can render specific partials based on the target header the connector reads: `X-Target` with the default connector (`connector.NewPartial`), `HX-Target` with htmx, which also sends `HX-Request: true`.

Example:
```go
func handler(w http.ResponseWriter, r *http.Request) {
    page := root.Clone().SetContent(content)
    output, err := partial.RenderWithRequest(r.Context(), r, page)
    if err != nil {
        http.Error(w, "An error occurred while rendering the page.", http.StatusInternalServerError)
        return
    }
    w.Write([]byte(output))
}
```

To request a specific partial:
```bash
curl -H "X-Target: sidebar" http://localhost:8080

# With connector.NewHTMX:
curl -H "HX-Request: true" -H "HX-Target: sidebar" http://localhost:8080
```

## Useless benchmark results

with caching enabled 
```bash
goos: darwin
goarch: arm64
pkg: github.com/donseba/go-partial
cpu: Apple M2 Pro
BenchmarkRenderWithRequest
BenchmarkRenderWithRequest-12    	  526102	      2254 ns/op
PASS
```

with caching disabled
```bash
goos: darwin
goarch: arm64
pkg: github.com/donseba/go-partial
cpu: Apple M2 Pro
BenchmarkRenderWithRequest
BenchmarkRenderWithRequest-12    	   57529	     19891 ns/op
PASS
```

which would mean that caching is roughly 9-10 times faster than without caching


## Upgrading

### From v0.14.0

**Compile breaks**

- `slots.Set` and `debug.Stage` take options as a last, variadic parameter (`slots.Option`, `debug.Option`). Calls compile as before; only a function value of the old type, such as `var stage func() partial.RenderStage = debug.Stage`, does not.
- New fields in `partial.RenderStageHooks` (`ResolveFunc`, `FuncNames`, `FallbackFuncNames`), `connector.Interaction` (`Select`, `Attrs`), `flash.Data` (`RequestErrorAttrs`) and `exterrors.Data` (`Title`, `Lang` and `Status`, after `Message`): struct literals of these types that list the fields without their names no longer compile. Name the fields.

**Responses of `Write`**

- `Write` adds the request headers the connector reads to the `Vary` header of every response: for htmx `HX-Request`, `HX-Boosted`, `HX-History-Restore-Request`, `HX-Target`, `X-Select` and `X-Action`, for htmx 4 also `HX-Request-Type`; before, it set no `Vary`. Caches and CDNs so keep a page and its fragments apart; check caching rules that ignore or rewrite `Vary`.
- `Write` sends `Content-Type: text/html; charset=utf-8` unless a header already sets it; before, only failure responses had it, and `net/http` guessed the type of the others from their first bytes.
- When an error stage, such as `ext/errors`, answers a failed render, `Write` sends the failure response with `Cache-Control: no-store` and returns a `*partial.FailureResponseError` that wraps the render error; before, it returned the render error itself. A comparison such as `err == renderErr` no longer matches: use `errors.Is`, and `errors.As` to tell that the response has been written.

**Targets and out-of-band regions**

- Out-of-band partials go in the order they were registered with `WithOOB` (`slots.Glob`: the order of the file names), nearest partial first; before, the regions of one partial came in a different order from one render to the next. A request that targets an ID found in more than one branch of the tree now always finds the same partial, searching the children in the order of their IDs.
- A target that a render stage resolves, such as a partial an `exp/target` resolver returns, also gets the out-of-band regions of the partial that resolved it, as a registered child does; before, only those of that partial's ancestors were added. Such responses can carry more regions.
- `SetAlwaysSwapOOB(true)`, which `slots.OOB()` sets, takes effect: such a region also goes out of band with a partial request for the partial it is registered on, after that partial's output, unless the output places it already, as a layout does with `{{ slot }}`. Before, the setting did nothing, and a region went only with requests for the partials below the one it is registered on. Responses to requests for a layout or page with such regions that it does not place now carry them; register a region without the setting where it should not go.
- `{{ oobAttr }}` writes the connector's out-of-band attribute, as `oobSwap` and `SetOOBElements` do: `up-hungry=""` for Unpoly, where it wrote `hx-swap-oob`. htmx, Turbo and connectors without an attribute of their own keep `hx-swap-oob`; the swap value given is now HTML-escaped.
- `oobSwap` is a go-partial helper now: `SetFunc` ignores a function of that name, and a template that declares a typed root named `oobSwap` fails to render. Rename them.

**Template cache**

- `UseTemplateCache` applies to the partials rendered under a partial that do not set it themselves, such as its children, its content and the partials of `exp/flash`; before, each partial used its own setting, so with only the root cached the others rendered without the cache. Edits to their template files no longer show until the root is built again: turn the cache off in development, for example with `root.UseTemplateCache(!development)`.
- With the cache on, a file that `{{ partial "path" }}` does not find is remembered as missing, as a file it found already was; a file added later is not seen by that tree.
- A file system the cache cannot tell apart from others of its type, such as a struct that holds a map, no longer shares cached parses: its templates are parsed on every render, so renders over it are slower. Pass a pointer to such a struct instead.

**Template functions**

- A function registered with `SetFunc` whose first parameter is a `context.Context` receives the render's context: templates call it without that argument. A template that passed a context itself now gives one argument too many and fails; drop the argument.
- `SetFunc` and `Use` emit the `func.protected` warning, which was declared but never sent: for a function named like a go-partial helper, which `SetFunc` ignores as before, or like a function a stage gives templates, and for a registered function a new stage replaces. Expect new warnings; rename the functions they name.
- The stages of `exp/target`, `exp/slots`, `exp/localization`, `exp/flash`, `exp/csrf`, `exp/selection`, `exp/actions` and `ext/logger` give their helpers through `RenderStageHooks.ResolveFunc`, only to the templates that call them; they are no longer in `RenderContext.Funcs`. A stage of the application's own that read them there gets them with `RenderContext.ResolveFunc(name)` in its `Render` hook.
- While their stage is in use, `flashTemplate` and `flashTargetAttrs` (`flash.Stage`), and `requiredSlot` and `deferredSlot` (`slots.Stage`), are reserved names: the stage gives templates these functions, so a function the application registers under such a name is no longer called.
- `{{ partial }}` runs the render stages of the calling partial once for the template it renders; before, they ran twice. Stages that count or record renders see one render per call.

**Per package**

- `exp/slots`: `{{ slot }}` and `{{ hasSlot }}` look a slot up through every ancestor of the partial that places it, the nearest first; before, only on the nearest partial that had slots, so a slot set on the layout rendered `""` in a content partial with slots of its own. `slots.Set` on a partial that inherits slots, or on a clone, no longer changes the slots of its ancestor or of the original.
- `exp/sse`: `Writer.PatchPartial` renders the partial itself (`partial.RenderSelf`); before, it used `RenderWithRequest`, which for a partial request, such as one of htmx, looked up the target the request names and added out-of-band regions.
- `exp/interactions`: in the endpoint form, `"select"` becomes `hx-select` with the htmx connectors and `data-select` with `connector.NewPartial`, and options whose names start with `hx-`, `up-`, `data-` or `aria-` are written on the element as given, after the connector's own attributes, which they replace; before, both were ignored. With htmx 2 and 4, the options `include`, `pushURL` and `replaceURL` of an interaction become `hx-include`, `hx-push-url` and `hx-replace-url`; with Unpoly, the target `this` is written as `:origin`.
- `interactions.FuncMap()` holds 11 more helpers (`asyncAttrs`, `revealAttrs`, `pollAttrs`, `onAttrs`, `streamAttrs`, `refreshAttrs`, `submitAttrs`, `liveAttrs`, `clientAttrs`, `noBoost`, `confirmAttr`), which replace functions of those names registered before it. Register such a function after `FuncMap()`.
- `flash.FuncMap()` holds `flashTemplate` and `flashTargetAttrs`, and `slots.FuncMap()` holds `requiredSlot` and `deferredSlot`, which replace functions of those names registered before them, as above.
- `exp/target`: `target.Stage()` leaves a target that an earlier stage resolved alone; before, its resolver replaced it.
- `ext/errors` in `ModeSafe`, the default: the built-in error page and fragment of a failed render show the status text (`Data.Title`), also as the page's `<title>`, and `exterrors.SafeMessage`; before, they showed "Template render error" with the partial ID and template names. `BuildData` in `ModeSafe` sets `Message` to `SafeMessage` and leaves `Location`, `PartialID` and `Templates` empty, so markup of the application's own that prints them no longer shows the error; `Data.Error` still holds it. `ModeDetailed` is unchanged.
- `ext/errors`: the built-in error fragment has classes instead of `style` attributes, which a Content-Security-Policy without `'unsafe-inline'` blocks: `go-partial-error` (as before), and the new `go-partial-error__title` (`h1`), `go-partial-error__message` (`p`) and `go-partial-error__details` (`dl`). It no longer styles itself: serve and link the stylesheet of `exterrors.StylesheetFS()` (`partial-errors.css`) or style the classes in the application's CSS; unstyled it reads as a heading, a paragraph and a list. The built-in error page keeps its inline `<style>`, whose hash is `exterrors.StyleHash()`.
- `ext/debug`: the debug box has classes (`go-partial-debug__header`, `__label`, `__partial`, `__value`) and one `<style>` element of its own instead of `style` attributes; it looks the same. Under a strict policy, add `debug.StyleHash()` to `style-src`.
- `ext/logger`: `logger.Sink(nil)` writes to `slog.Default()` as it is when each event is written; before, to the default logger of the moment `Sink` was called, so a later `slog.SetDefault` now takes effect.
- `exp/otel`: go-partial requires OpenTelemetry Go v1.45.0, which fixes [GHSA-8wmf-6v46-5gfg](https://github.com/advisories/GHSA-8wmf-6v46-5gfg) (exporter endpoints in verbose internal logs). A module that requires an older version moves up to it.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your improvements.

## License

This project is licensed under the [MIT License](LICENSE).
