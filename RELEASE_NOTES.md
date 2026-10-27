# go-partial release notes

## v0.15.0

### Behaviour Changes

The README's Upgrading section lists every change that a user of v0.14.0
can notice, with what to do.

- Out-of-band partials are sent in a fixed order: those of the partial
  nearest the target first, then those of each ancestor, and each partial's
  own in the order `WithOOB` registered them (`slots.Glob`: the order of the
  file names). Before, the regions of one partial came in map order, which
  changed from render to render and with it the bytes, ETag and cached
  copies of the same response. Registering an ID again keeps its place. A
  target ID found in more than one branch of the tree is now looked up in
  the order of the children's IDs, so it always finds the same partial.
- `SetAlwaysSwapOOB(true)`, which `slots.OOB()` sets, takes effect: such a
  region also goes out of band with a partial request for the partial it is
  registered on, a registered one or one a render stage resolves, after that
  partial's output, unless the output places it already, such as with
  `{{ slot }}`. Before, the setting did nothing: a region went only with
  requests for the partials below the one it is registered on.
- `ext/errors`: in `ModeSafe`, the built-in error page and fragment of a
  failed render name no partial, template or error: they show the status
  text (`Data.Title`), the safe message (`Data.Message`, or the text of
  `WithMessage`) and the language of `WithLang`, as an error of `Respond`
  does; before, they showed "Template render error" with the partial ID and
  template names. `Data.PartialID` and `Data.Templates` are empty in
  `ModeSafe`. In `ModeDetailed` nothing changes.
- `ext/errors`: in `ModeSafe`, `Data.Message` is now `errors.SafeMessage` and
  `Data.Location` is empty, so an error template that prints them no longer
  shows the error; `Data.Error` still holds it. In `ModeDetailed` nothing
  changes.
- With the template cache, a `{{ partial }}` path that does not exist is
  remembered as missing, as one that exists already was, so a file added
  later is not seen by that tree. `UseTemplateCache` applies to the
  partials rendered under a partial that set nothing themselves (see
  Template Cache Inherited).
- New reserved template names: `oobSwap` (core), and with their stages
  `flashTemplate` and `flashTargetAttrs` (`exp/flash`), `requiredSlot` and
  `deferredSlot` (`exp/slots`).
- `{{ oobAttr }}` writes the connector's out-of-band attribute
  (`connector.OOBAttrs`), as `oobSwap` and `SetOOBElements` do: `up-hungry=""`
  for Unpoly, where it wrote `hx-swap-oob`. htmx, Turbo and connectors
  without an attribute of their own keep `hx-swap-oob`; the swap value given
  is now HTML-escaped.
- `ext/errors`: the built-in error fragment has classes instead of `style`
  attributes, which a Content-Security-Policy without `'unsafe-inline'`
  blocks (`style-src-attr`). Before, every element carried a `style`
  attribute; now the section keeps `class="go-partial-error"` and the
  heading, message and list get `go-partial-error__title`,
  `go-partial-error__message` and `go-partial-error__details`. The fragment
  no longer styles itself: serve `errors.StylesheetFS()`, which holds its
  CSS as `partial-errors.css`, and link that file from the application's
  layout, or style the classes in the application's
  CSS. Unstyled, it reads as a heading, a paragraph and a list. The
  built-in error page keeps its inline `<style>`.
- `ext/debug`: the debug box styles its new classes (`go-partial-debug__header`,
  `__label`, `__partial`, `__value`) with one `<style>` element of its own
  instead of `style` attributes. It looks the same; under a strict policy it
  needs `debug.StyleHash()` in `style-src`.
- `connector.NewUnpoly`: an interaction that targets its own element
  (`this`, `connector.TargetSelf`) is written as `up-target=":origin"`, as
  Unpoly names that element; before, `this` was written as it is.
- go-partial requires OpenTelemetry Go v1.45.0 (`exp/otel`), which fixes
  GHSA-8wmf-6v46-5gfg.

### Content-Security-Policy, Error Responses And Other Additions

- Content-Security-Policy: `ext/errors.StyleHash()` returns the hash source
  (`'sha256-…'`, with its quotes) of the built-in error page's inline
  `<style>`, for `style-src`; it is
  `'sha256-5WNh79OqlbaM4rZ/DlkOnop5aYED/JCWp9ChTZ5ZOSQ='` and will be named
  here whenever it changes. `ext/errors.StylesheetFS()` and
  `StylesheetName` (`partial-errors.css`) give the CSS of the built-in error
  fragment. `ext/debug.StyleHash()` is the hash of the debug box's
  `<style>`. The README's new Content-Security-Policy section lists what
  go-partial writes inline, where the interactions client's
  `reloadOnError` and SSE patches run under the page's policy, and which
  htmx features need `'unsafe-eval'`.
- `ext/errors`: `FromContext(ctx) (Data, bool)` tells a template function
  with a `context.Context` parameter that it is called while a custom error
  template (`WithTemplate`, `WithFragmentTemplate`) renders, with that
  render's `Data`, so it can leave out what belongs to the page that failed,
  such as its structured data.
- `exp/head`: `head.Region(p)` makes a region of the layout placed in
  `<head>` replace the elements it wrote before when it goes out of band:
  on a full page its output sits between two empty `<template>` markers,
  and with htmx 2 and 4 a partial response removes the elements between
  them and adds the new ones before the end marker, with its `<title>` on
  its own.
- `partial.ModelFuncOf(fn)`: a lazy typed model as a value, which `SetModel`
  and `SetContract` take with other models.
- `exp/flash`: `WithRequestErrors(RequestErrors{Text, Replace, Body,
  SkipFlash})`, written by `{{ flashTarget }}`, the new
  `{{ flashTargetAttrs }}` (the target's id and those attributes, with an
  optional text) and `Data.RequestErrorAttrs`.
- `exp/sse`: `{{ sseAttrs url "clientId" "tab" "hidden" "30s" "beat" "15s"
  "beacon" "/presence" }}` (`sse.FuncMap()`, `sse.Attrs`,
  `sse.ClientStream`) writes the client's attributes. The client reports a
  stream to `data-partial-stream-beacon` (beat, hidden, removed, pagehide),
  which `sse.ReadBeacon` reads. A nil context of `Writer.PatchPartial` is
  the request's.
- `exp/interactions`: `AttrsFuncMap()` with only the attribute helpers;
  `clientAttrs "reloadOnError"` (`connector.Attr.ReloadOnError`,
  `data-partial-reload-on-error`) loads a link or form whose request gets a
  5xx status or no answer as a whole page, through the interactions client:
  links and GET forms are loaded again by the browser; a POST, PUT, PATCH or
  DELETE is never sent twice: the server's HTML error page replaces the
  document, and without one (or without an answer) the page stays and
  `partial:reload-on-error:failed` is dispatched on the form. An htmx 4
  answer with a 5xx status and `HX-Reswap: none`, such as the error flash
  message of `ext/errors`, leaves the page as it is.
- `exp/slots`: `{{ deferredSlot "name" ["reveal"] }}` defers a slot from the
  template that places it; `slots.Data(v)` gives a placeholder
  `PlaceholderData.Data`.
- `clients.All()` lists the browser clients with their import names.
- `exp/sse`: `NewBroker(BrokerOptions{MaxSubscribers, MaxPerKey, Buffer})`
  fans `Message`s out to `Subscription`s by topic (`Subscribe`, `Publish`,
  `Close`; `Subscription.Messages`, `Close`; `ErrTooManySubscribers`,
  `ErrBrokerClosed`).
  `Writer.Serve(ctx, subscription, ServeOptions{Heartbeat, BeforeHeartbeat,
  Handle})` streams a subscription with heartbeats (`Writer.Heartbeat`).
- `exp/metrics`: `Outermost()` records only the outermost render of a
  context.
- `connector.CurrentURLHeaderNamer` is the interface behind
  `connector.CurrentURLHeader`, for connectors of an application's own.
- `ext/errors`: `WithTitle(fn)` sets the new `Data.Title`, the heading of the
  built-in error page and fragment, such as a translation of the status text;
  it is the status text (`http.StatusText`) by default. A detailed failed
  render keeps its "Template render error" heading.
- `ext/errors`: `WithLang(fn)` sets the new `Data.Lang`, the language of the
  error page, such as that of the page that failed; the built-in page writes
  it as its `lang` attribute, `en` when it is empty, as before.
- `ext/errors`: `WithMessage(fn)` sets `Data.Message`, which the error page,
  the error fragment and the error flash message show, such as a
  translation of `SafeMessage` in the request's language; an empty text
  keeps it. `WithFlash(fn)` still overrides the flash text alone.
- `Partial.SetOOBElements(ids...)`: a region that goes out of band sends only
  the elements of its output with those IDs, each with the connector's
  out-of-band attribute, instead of its whole output, such as the menu items
  that mark the page shown. The region's own `oobAttr` element is dropped
  unless its ID is one of them; with none of the IDs in the output, or an
  element whose end cannot be told, the whole region goes. A full page is
  not affected.
- `Partial.SetOOBElementsFunc(func(*http.Request) partial.OOBElements)`
  chooses the IDs per request, called only when the region goes out of
  band; the headers in `OOBElements.Vary` are added to the response's Vary
  header then, and only then. `connector.CurrentURLHeader(conn)` names the
  header with the URL of the page the client shows (`HX-Current-URL` for
  htmx; empty for the others), and `partial.AddVary(header, names...)` adds
  names to a Vary header.
- `ext/errors`: `Respond(w, r, root, status, err)` answers an error that
  happened outside a render, such as a failed form post, with the stage's
  error page, fragment or flash message (with `WithMessage` and
  `WithFlash`), written with `status` and never cached: a partial
  request gets the flash message, or the error page for a 5xx status, which
  `reloadOnError` shows as the whole page, or the fragment for another
  status. A tree without the stage gets the built-in markup. `Data.Status`
  is the status (500 for a failed render). Core: `partial.WriteError(w, r,
  p, status, err)` and `RenderContext.FailureStatus()`.
- `Dot.Model()`: a dot as a model sharing its lazy value, so a layout's dot
  and the regions' `@model` compute it once per render tree.
- `placeholderRequestFuncMap` carries `// go-clue:funcmap`, so go-clue learns
  the render-time functions (`content`, `runtime`, `oobAttr`, `partial`,
  `url`…) from it.

### Lazy Models And Dot Values

`partial.ModelFuncOf(func(ctx) (T, error))` is a typed `@model` value
computed only when a template calls it, once per render tree, also when the
layout, header and sidebar each call it, and not for a response that leaves
them out. It is matched by T, also by `CheckContracts`, without calling the
function. `partial.Dot` carries a dot as a value, such as in a struct
field: `DotFuncOf(fn)`, which keeps the type for `CheckContracts`, set with
`SetDotSource`. Lazy values (`DotFuncOf`, `ModelFuncOf`) are kept from
where the render of a tree starts, so a dot or model that the page's target
and its out-of-band regions share is computed once per response.

### Child Includes, Glob, Defines, FirstExistingIn

`SetChildIncludes(patterns...)` gives include patterns to the partials
rendered under a partial only, so a root gives its pages, regions and error
templates the theme's partials without adding them to its layout's parse.
`Partial.Glob(pattern)` returns the files a pattern matches, once per file
system with the template cache. `Partial.Defines(file, name)` reports
whether a file defines a template, and `HasEntry()` whether a partial's file
defines its entry, from the scans renders use. `FirstExistingIn(layers,
candidates...)` tries every candidate in a layer before the next; without
layers, those of the partial's `Overlay`. `Partial.Func(name)` returns a
function registered with `SetFunc`.

### Errors As Flash Messages

`exterrors.WithFlash(fn)` answers a partial request that fails to
render with an error flash message, out of band in the flash target, with
the template of the tree's `exp/flash` stage, leaving the request's target
as it is (`HX-Reswap: none`); `fn` gives the text, `Data.Message` when it is
nil.
`flash.Respond(w, r, parent, status, messages...)` refuses a request with
messages the same way, or with their text as `text/plain` for requests of no
client library. `flash.RenderOOB(ctx, messages...)` renders such an addition
from a render stage. `{{ oobSwap "beforeend" }}` writes the connector's
out-of-band attribute, also outside an out-of-band render
(`connector.OOBAttrs`: `hx-swap-oob` for htmx, `up-hungry` for Unpoly).
`RenderContext.ResolveFunc(name)` gives a stage the functions of the other
stages, and `RenderContext.FailureResponse()` tells an error stage that it
answers the request as a whole.

### Flash Client Templates And Request Errors

`{{ flashTemplate }}` renders the application's flash template as a
`<template data-flash-template="level">` per level, which the browser client
clones for the messages it shows, so they look like those of the server;
it writes nothing in a partial rendered out of band. On the flash target,
`data-flash-request-error-replace` keeps one request error at a time,
`data-flash-request-error-body` shows the `text/plain` sentence of a
refused request (4xx) without htmx swapping it, and
`data-flash-request-error-skip-flash` adds nothing to a failed response
that brings its own message.

### Deferred, Globbed And Required Slots

`slots.Deferred()` makes `{{ slot }}` render a placeholder that requests the
slot from the page's address through the connector; the request renders
the slot alone, with its lazy dot. `slots.Placeholder(p)` renders the
application's placeholder with a `slots.PlaceholderData` dot.
`slots.Glob(parent, pattern, opts...)` sets a slot per file, named after it
(`slots.FileName`), and `slots.Setup(fn)` configures each. `{{ requiredSlot "name" }}` fails the
template for a slot that is not set.

### Page Settings, Merged Attributes, Submit Confirm

`connector.Attr` has `Inherit` (htmx 4's `:inherited`, with a boost that
carries its target and swap, quoted when they hold whitespace or a comma),
`DisableWhileRequest` (`hx-disable`, `hx-disabled-elt`, `up-disable`),
`KeepOnServerError` (`hx-status:5xx="swap:none push:false"`) and
`SubmitConfirm`, a question a submit button asks (`data-turbo-confirm`,
else `data-partial-confirm`, which the new `exp/interactions` browser
client asks). `clientAttrs` takes
`"inherit"`, `"disableWhileRequest"`, `"keepOnServerError"` and
`"submitConfirm"`. `connector.MergeAttrs(c, existing, attr, remove...)`
replaces the connector's attributes of a setting in a map of attributes;
`connector.Apply(c, header, response)` sets response headers in one call.

### Confirmation Dialog

A page with a `<dialog data-partial-confirm-dialog>` gets the questions of
`"submitConfirm"` buttons and htmx's `hx-confirm` (htmx 2 and 4) asked in
that dialog by the `exp/interactions` client, instead of with
`window.confirm`, which some embedded browsers never show. The question
goes into its `data-partial-confirm-question` element; a button with
`value="confirm"` answers yes. Without the dialog nothing changes.
`goPartialInteractions.ask(question)` asks a question of the application's
own the same way.

### SSE Client Ids And Back/Forward Cache

With `data-partial-stream-client-id="tab"`, the SSE client adds an id of
the tab, the same for the life of the page, to the stream URL as that
parameter (`goPartialSSE.clientId`). A page restored from the back/forward
cache opens its streams again, dispatching `partial:stream:reconnect`.

### Browser Clients As Files

`clients.FS()` holds the clients of `exp/flash`, `exp/sse` and
`exp/interactions`, each under its package's `ClientName`, for applications
that serve, fingerprint and version them with their other assets.

### Debug Unless Defined, Fallback Stage Functions

`debug.Stage(debug.WithFunc(debug.UnlessDefined()))` leaves a `debug`
function the application registers itself in place, without a warning;
`debug.Disabled()` does not count as one. Stages do the same for any
function with `RenderStageHooks.FallbackFuncNames`.

### Template Cache And Map File Systems

The documentation of `UseTemplateCache` now says what the cache learns of a
file system and who shares it: the partials of one tree root.

### Shared Includes

`Partial.SetIncludes(patterns...)` gives every template of a partial and of
the partials rendered under it, its children, content, `{{ partial }}` calls
and partials that name it with `SetParent`, the templates its `fs.Glob`
patterns match, such as a theme's `partials/*.gohtml`. Patterns resolve
against the file system of the partial being rendered; each file is parsed
once, also when several patterns or the partial's own templates name it.
With the template cache a pattern is resolved once per file system; without,
at every render.

### Template Cache Inherited

`UseTemplateCache` now applies to the partials rendered under a partial,
unless they set it themselves: one `root.UseTemplateCache(!development)`
covers the children, content, `{{ partial }}` calls and partials rendered
with `SetParent`. Children that set nothing
used to render without the cache; they now use the nearest setting above
them. Partials go-partial builds itself, such as those of `exp/flash`, now use the
cache of the tree.

### Data Computed When Needed

`partial.DotFuncOf(func(ctx) (T, error))`, set with `SetDotSource`, computes
a partial's dot only when the partial renders, on the page or as the target
of a partial request, once per render tree; an error fails the partial's
render. It keeps the type, so `CheckContracts` checks it against `@dot`
without calling the function. `SetDot` and `SetDotSource` replace each
other.

### Choosing A Template, Parent

`Partial.FirstExisting(candidates...)` returns the first template that exists
in the partial's file system; with the template cache, the answer for each
candidate, also a missing one, is remembered per file system. Without a match
the error wraps `fs.ErrNotExist`. `Partial.Parent()` returns the partial a
partial renders under.

### Rendering Text

`partial.RenderText(ctx, p)` renders a partial with `text/template`, unescaped,
for text such as the subject and text part of an e-mail, with the file
system, includes, entry, dot, `@model` values, functions, render stages and
template cache of an HTML render. Stages see `partial.RenderKindText`.
`{{ debug }}` of `ext/debug` writes plain text there, without its HTML box.

### Stage Functions And Reserved Names

Templates are parsed with stand-ins for the functions render stages declare
in `FuncNames`, so `Use(stage)` no longer needs the stage's `FuncMap()`
registered with `SetFunc`; a stand-in that is called because no stage of the
render gives the function fails with an error that names it. `SetFunc` emits
the
`func.protected` warning, declared before but never sent, for a function
named like a go-partial helper or like a function a stage gives templates,
and `Use` for a registered function a new stage replaces; a stage's own
stand-ins, from the stage's package, raise none.
`RenderStageHooks.Package` names the package that made a stage, so its
stand-ins are known as its own also where the compiler inlines the function
that returns the stage into the application, which names the stage's hooks
after the application; the stages of go-partial's packages set it. A
package that makes stages with `RenderStageHooks` and offers stand-ins for
their functions sets it too, such as to `reflect.TypeFor[T]().PkgPath()`
for a type `T` of its own.
`RenderContext.TargetResolved()` tells a stage that resolves targets that an
earlier stage did; `exp/target` leaves such a target alone.

### Write Content Type

`partial.Write` sends `Content-Type: text/html; charset=utf-8` unless a
header sets it; before, only failure responses had it.

### Slots: Inherited, Regions And Fragments

`exp/slots` looks a slot up from the partial that places it through its
ancestors, so a content template places a slot set on the layout; a nearer
slot of the same name wins. `slots.Set` takes options: `slots.OOB()` makes
the slot a region sent out of band with every partial response;
`slots.CheckContracts()` and `slots.OmitOOB()` answer partial requests for
the slot's ID through `slots.Stage()`: only when the slot can render in the
tree, and without the tree's out-of-band regions. `Set` on a partial that inherits slots, or on a clone, no longer
changes the slots of its ancestors or of the original.

### Error Templates

`ext/errors` renders the failure response with a template of the
application: `WithTemplate(path)` for the error page,
`WithFragmentTemplate(path)` for the error fragment. The template renders as a child of the partial that failed, with an
`errors.Data` dot; when it fails itself, the built-in markup answers and an
`ext.errors.template_failed` warning is emitted. A stage with only an error
page template answers partial requests with it too, and one with only a
fragment template full pages.

### Debug And Logger

`debug.Stage(debug.WithFunc())` gives templates `{{ debug . }}` bound to the
render; `{{ debug runtime . }}` keeps working. `debug.Disabled()` is a helper
that renders nothing, for production. `logger.WithLevels(levels...)`
filters events, and `logger.Sink(nil)` writes
to `slog.Default()` as it is when each event is written.

### Connector Redirects, Attributes And Test Requests

`connector.Redirect(c, w, r, connector.Location{...})` sends the client on
after a request: `HX-Location` with status 200 for htmx requests, a `303 See
Other` otherwise. `connector.Attrs(c, connector.Attr{Confirm, Boost, Target,
Swap})` returns a link's or form's attributes in the connector's syntax.
`connector.SetRequestTarget(c, req, target)` makes a request ask for a target
as the client library would, the inverse of `GetTargetValue`. Custom
connectors join through the optional `Redirector`, `AttrWriter` and
`RequestTargeter` interfaces.

### Forms, Live Search And Link Attributes

`exp/interactions` adds `submitAttrs` (`connector.InteractionSubmit`: htmx
`hx-post` or another method, Unpoly `up-submit`, a Turbo frame) and
`liveAttrs` (`connector.InteractionLive`: an input that requests as the user
types), the neutral options `include`, `pushURL` and `replaceURL` for every
interaction (`hx-include`, `hx-push-url`, `hx-replace-url` with htmx), and
`clientAttrs`, `noBoost` and `confirmAttr` for links and forms.

### Browser Clients

`exp/flash` ships a browser client, `flash.ClientScript()`: it hides
messages after a time per level, keeps them under the pointer or focus,
removes them with a `data-flash-dismiss` element or Escape, dispatches
`partial:flash`, and with `data-flash-request-error` on the flash target
shows an error for failed requests of htmx, Unpoly and Turbo and for
`partial:error` events. A response with an error status that htmx swaps,
such as a form re-rendered with a 422, is no failed request: with htmx 4
the client decides at `htmx:finally:request`, with htmx 2 by
`htmx:beforeSwap`'s `shouldSwap`; a cancelled htmx 4 request shows no
error.

The `exp/sse` client opens a new stream when an element's URL changes, and
with `data-partial-stream-hidden="30s"` closes a stream while the tab is
hidden that long, opening it again with a `partial:stream:reconnect` event.

### Localization For Other Templates

`localization.Funcs(l)` returns the `locale` and `localizer` helpers bound to
a localizer, for templates go-partial does not execute;
`localization.New(code)` makes a localizer for a locale code.

### Rendering Under A Parent

`Partial.SetParent(parent)` renders a partial under a configured partial
without adding it to its children: it inherits the parent's functions,
render stages, connector, file system, template cache, failure policy and
`@model` values, and the parent is not changed. An application renders
components, e-mail and server-sent events under its root this way, without
cloning the root for each one.

`partial.RuntimeFrom(ctx)` gives a function registered with a
`context.Context` the runtime of the partial whose template calls it, so
`runtime.RenderPartial(component)` renders the component as a true child of
that partial.

`Partial.SetEntry(name)` renders the template a file defines as name, such
as `{{ define "html" }}` of an e-mail, instead of the file itself.

### Failure Responses

When `Write` writes the failure response of an error stage, it returns a
`*partial.FailureResponseError` with the status written, so handlers know
not to answer the request again; without one it returns the render error
and writes nothing, as before. Failure responses are sent with
`Cache-Control: no-store`. Under `FailResponse`, the failure response of a
partial request keeps an error status (500, or the error stage's own 4xx or
5xx) and comes without out-of-band regions; `FailInline` still answers a
partial request with the error fragment and status 200.

### Targets Requested On Their Own

`RenderContext.OmitOOB()` lets a render stage that resolves a target answer
it with the partial alone, without the out-of-band regions of its
ancestors, as `slots.OmitOOB()` does for a standalone slot.

A target resolved by a render stage now gets the out-of-band regions of the
partial that resolved it as well, as a registered child of that partial
does; before, only those of its ancestors were added.

### Interaction Select And Attributes

`connector.Interaction` has `Select`, written as `hx-select` by the htmx
connectors, and `Attrs`, attributes written as given after the connector's
own. In templates, `"select"` and options whose names start with `hx-`,
`up-`, `data-` or `aria-` set them, such as
`{{ pollAttrs runtime "/inbox" "select" "#messages" "hx-indicator" "#spinner" }}`.

### SSE Patches Under A Parent

`sse.Writer.SetParent(root)` renders the partials `PatchPartial` patches
under root, with its functions, stages and models, unless they have a parent
of their own; the partial itself is not changed. `PatchPartial` now renders
the partial itself (`partial.RenderSelf`), also for a request that targets
something else, such as an htmx stream request.

### Templates From Memory And Overlays

`partial.Sources` is a file system of template sources held in memory, and
`partial.Overlay(layers...)` looks files up in each layer in turn, listing
the files of every layer for directories. Together they let a package ship
default markup that applications replace with a file of their own, rendered
with partials, typed models and the template cache like any template.
Overlays of the same layers share their cache entries. `partial.Layers(fsys)`
returns the layers of an overlay, to add one below it without nesting
overlays.

The template cache no longer shares parses between file systems it cannot
tell apart, such as two values of a struct type that holds a map: their
templates are parsed for every render instead of served from each other's
cache entry.

### Contract Checks

`Partial.CheckContracts()` reports whether a partial can render with the
values it has: every typed root finds one value and the dot has the `@dot`
type. `Runtime.CheckContracts(p)` checks a partial as a child of the one
being rendered. `partial.ParseDeclarations(sources...)` returns the typed
and `@dot` declarations of template sources. Typed roots of unnamed types,
such as `[]github.com/acme/app.Row`, now match their values.

`Partial.CheckTemplates(patterns...)` parses every template file of the
partial's file system with the functions its renders have, and returns
every `partial.TemplateProblem`: a file that does not parse, a call of a
function the partial does not have, and a `{{ template }}` that names a
template no file defines, each with its file, line, column and the closest
defined name. Run it when the application starts.

### Functions With A Context

Functions registered with `SetFunc` whose first parameter is a
`context.Context` receive the render's context; templates call them without
it. Each is bound once per render tree. `partial.BindContext` binds them for
templates go-partial does not execute.

`partial.ReservedFuncNames(stages...)` and `partial.IsReservedFuncName` list
the names templates cannot be given, including those of render stages, which
declare them with `RenderStageHooks.FuncNames`.

### Target Fallback, Strict Failures, RenderSelf

`SetTargetFallback(partial.TargetFallback{ID: "content", PushURL: true})`
answers partial requests for unknown targets with the content, sending the
retarget, swap and push URL through the connector. Each answer emits a
`target.fallback` event (`partial.EventTargetFallback`) at info level.
`slots.CheckContracts()` leaves standalone slots that cannot render here to
that fallback.

`SetFailurePolicy(partial.FailResponse)` fails the whole render when any part
of it fails or is missing; `Write` then writes only the failure response of
an error stage, or nothing, and returns the error.

`partial.RenderSelf(ctx, r, p)` renders a partial itself, whatever target the
request asks for. `partial.Write` adds the connector's request headers to
`Vary`.

### Connector Helpers

`connector.IsRequest`, `IsBoosted`, `CurrentURL` and `VaryHeaders`, through
the optional `RequestInspector` and `HeaderVarier` interfaces;
`Partial.Connector()`; `connector.TargetSelf`.

### Interaction Attributes, SSE Client, Persistent Flash

`exp/interactions` adds `asyncAttrs`, `revealAttrs`, `pollAttrs`, `onAttrs`,
`streamAttrs` and `refreshAttrs`, which return only the attributes for an
element of the template's own, targeting it unless told otherwise.

`exp/sse` ships a browser client for its events (`sse.ClientScript()`).

`exp/flash` keeps messages across a redirect in a cookie, with their level:
`cookie.Persist(w, messages...)` and `r = cookie.Restore(w, r)` of a
`flash.Cookie`, signed with its `Key`.

### Stage Functions Made When Called

`RenderStageHooks.ResolveFunc`, or any stage that implements
`partial.TemplateFuncResolver`, gives templates functions bound to the render
by name, asked only for those a template calls: a render whose templates call
none of them costs nothing, where functions set with `RenderContext.SetFunc`
in `Prepare` are made for every partial. Functions set with `SetFunc` or
`SetFuncResolver` come first; a later stage is asked before an earlier one.
Without the template cache, the names a stage declares in `FuncNames` are
given to templates as `SetFunc` gave them.

The stages of `exp/target`, `exp/slots`, `exp/localization`, `exp/flash`,
`exp/csrf`, `exp/selection`, `exp/actions` and `ext/logger` now give their
helpers this way; their helpers no longer appear in `RenderContext.Funcs`.

### Lighter Renders And Targets

A render allocates far less, most of all for parts of a page built per
request and for targets a resolver answers:

- clones share the registered functions until either registers more, so
  cloning a root with many functions copies none of them;
- cached templates are parsed with only the functions they call, so each
  copy of one carries a few, and go-partial's helpers are made only for the
  templates that call them;
- `{{ partial }}` no longer copies the calling partial's child tree, nor
  runs its render stages twice;
- render stages without a `Render` hook are left out of the chain, and the
  template render ends it instead of being a stage of its own;
- `New` and `NewID` make children, out-of-band children, extensions and the
  template store only when a partial needs them; a tree's root makes its
  store at its first render or when it is cloned, so clones still share it;
- copies made for a render share the templates, stages and values of the
  partial they copy; the dot is kept apart from the typed values;
- the cached template keeps the typed roots of its files, and is given its
  functions and root values in one lookup;
- file systems, template files and the types of values are identified
  without building keys on every render, and request headers are read
  without canonicalizing their names on every read;
- `Write` adds the connector's headers to `Vary` without allocating for
  each, and the output is built without a copy.

In go-webthings' benchmarks, a page went from 1834 to about 1025 allocations
(112 KB to 76 KB), a navigation from 1650 to about 935 (101 KB to 69 KB),
and a fragment answered by a resolver from 266 to 131 (27 KB to 21 KB).

## v0.14.0

### Faster Cached Renders

With the template cache, a render no longer reads template files: each file
is scanned once per file system for the templates it defines and calls, the
functions it uses and its `@model` declarations, and `{{ partial }}` paths
are checked once. Without the cache, files are read on every render as
before, so edits show.

A partial tree shares its root's template store, so partials built for one
render, such as regions, reuse earlier parses. A cached template receives only
the functions it calls instead of a merge of every registered function.

Render stages that offer many functions can use
`RenderContext.SetFuncResolver`, which is asked only for the names a template
calls.

### Partial Pairs Bind Declared Models

`{{ partial runtime "card.gohtml" "Event" . }}` also binds the pair to a root
the card declares by that name, such as `@model Event example.com/app.Event`,
with its type checked. The pairs stay in the card's dot map.

A partial with its own `SetModel` values can now call `{{ partial }}`; the
callee no longer sees those values twice and fails with "multiple matching
values".

## v0.9.0

### go-doc Template Function Support

go-partial now publishes optional interaction template helper functions in:

```text
github.com/donseba/go-partial/exp/interactions
```

These helpers contain the endpoint/interaction parsing logic and are registered
explicitly with `root.SetFunc(interactions.FuncMap())`. Core go-partial no
longer auto-registers `async`, `poll`, `reveal`, `on`, `stream`, `prefetch`, or
`refresh`, which keeps the render core decoupled from optional client
interactions.

They also carry `go-doc:sig` metadata so go-doc can understand the overloads
that Go cannot represent in a single function signature.

The repository includes `.go-doc/config.json` with the built-in helper
signatures wired up as `templateFunctions`. Editors can now understand both
interaction styles:

```gotemplate
{{ async runtime "/async/row/:row" "row" .ID }}
{{ async runtime Async }}
```

Use endpoint strings for simple, local interactions. Use named
`interactions.Interaction` values with `SetContract("interaction", ...)` when
Go should own stable IDs, targets, polling intervals, events, placeholders, or
reuse.

### Showcase Cleanup

The interaction showcase now uses the typed named interaction form for the
default async example instead of an intentionally-invalid placeholder call.

The showcase also demonstrates `exp/flash` for transient SSR/HTMX messages.
Async rows, infinite-scroll chunks, and webshop cart actions append flash
messages into a stable `flashTarget` container and remove them after a short
delay.

### Flash Messages

go-partial now includes an experimental flash helper package:

```text
github.com/donseba/go-partial/exp/flash
```

Register it with `root.SetFunc(flash.FuncMap())` and
`root.Use(flash.Stage())`. Templates can render `{{ flashTarget }}` once
in a wrapper and `{{ flash }}` in request or fragment templates. Message and
target templates are embedded by default and can be replaced with
`flash.WithTemplate`, `flash.WithPartial`, `flash.WithTargetTemplate`,
`flash.WithTargetPartial`, and `flash.WithTargetID`.

### Render Stages

The render pipeline vocabulary now uses render stages. New code should use
`partial.RenderStage`, `partial.RenderStageHooks`, `Partial.Use`, and
package constructors such as `metrics.Stage(...)`, `flash.Stage(...)`, and
`errors.Stage(...)`.

### Package Rendering Functions

Partial rendering and HTTP writing now have package-level entry points:
`partial.Render(ctx, p)`, `partial.RenderWithRequest(ctx, r, p)`, and
`partial.Write(ctx, w, r, p)`. `partial.Write` owns response headers, connector
response instructions, render-stage response metadata, error fragments, and
out-of-band output. `Partial` no longer has render or write methods; this is an
intentional pre-v1 break so rendering is owned by package-level functions.

### Helper Provider Split

The protected helper set has been reduced to go-partial's actual core helpers:
runtime, wrapper/content, request/context, OOB, and connector state helpers.
Generic helpers such as `dict`, string helpers, date helpers, and counters are
ordinary template helpers and can be provided through:

```text
github.com/donseba/go-partial/exp/templatehelpers
```

The repository `.go-doc/config.json` now advertises optional providers such as
`exp/flash`, `exp/interactions`, and `exp/templatehelpers`.
