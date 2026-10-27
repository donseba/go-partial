# Integrations

go-partial integrates with frontend tools through connectors. A connector tells go-partial how to read request intent, such as the DOM target, selected partial, or action, and how to write protocol-specific response headers.

## Core connectors

| Connector | Target header | Use |
| --- | --- | --- |
| `connector.NewHTMX` | `HX-Target` | HTMX requests, boosted navigation, response headers, and out-of-band swaps. |
| `connector.NewHTMX4` | `HX-Target` (`tagName#id`) | htmx 4 requests; see [HTMX 4](README.md#htmx-4) in the README. |
| `connector.NewPartial` | `X-Target` | Framework-neutral fetch clients, tests, and custom JavaScript. |
| `connector.NewTurbo` | `Turbo-Frame` | Turbo Frame requests. |
| `connector.NewUnpoly` | `X-Up-Target` | Unpoly fragment requests. |

Selection and action values use the shared `X-Select` and `X-Action` headers unless a connector defines something else.

## HTMX

```go
root := partial.New("shell.gohtml").
    SetConnector(connector.NewHTMX(nil)).
    SetFileSystem(os.DirFS("web"))
```

```html
<button hx-get="/tabs" hx-target="#content" hx-headers='{"X-Select":"settings"}'>
    Settings
</button>
```

Partials can set response instructions without hard-coding HTMX headers:

```go
notice := partial.NewID("notice", "notice.gohtml")
notice.Response().
    Retarget("#notice").
    ReswapWith(connector.NewSwap().Style(connector.SwapOuterHTML)).
    TriggerWith(connector.NewTrigger().AddEvent("saved"))

_ = partial.Write(r.Context(), w, r, notice)
```

The HTMX connector writes headers such as `HX-Retarget`, `HX-Reswap`, and `HX-Trigger`.

Handlers ask the connector about a request rather than reading `HX-*` headers: `connector.IsRequest(conn, r)` (also true for an htmx 4 request for the whole page, which `RenderPartial` is not), `connector.IsBoosted(conn, r)` and `connector.CurrentURL(conn, r)`. `partial.Write` adds `connector.VaryHeaders(conn)` to the `Vary` header.

## Redirects, Link Attributes And Test Requests

These work the same with every connector, each in its library's terms:

| Helper | htmx 2 / 4 | Unpoly | Turbo | Partial |
| --- | --- | --- | --- | --- |
| `connector.Redirect(c, w, r, connector.Location{...})` | `HX-Location`, status 200, for htmx requests; else 303 | 303 See Other | 303 See Other | 303 See Other |
| `connector.Attrs(c, connector.Attr{Confirm: ...})` | `hx-confirm` | `up-confirm` | `data-turbo-confirm` | `data-confirm` |
| `Attr{Boost: connector.BoostOff}` | `hx-boost="false"` | `up-follow="false"`, `up-submit="false"` | `data-turbo="false"` | `data-boost="false"` |
| `Attr{Target: "#list"}` | `hx-target` | `up-target` | `data-turbo-frame="list"` | `data-target` |
| `Attr{Swap: "outerHTML"}` | `hx-swap` | (none) | (none) | `data-swap` |
| `connector.SetRequestTarget(c, req, "list")` | `HX-Request`, `HX-Target` (htmx 4: `#list`, `HX-Request-Type: partial`) | `X-Up-Target` | `Turbo-Frame` | `X-Target` |

`SetRequestTarget` is the inverse of `GetTargetValue`, for tests and requests a server makes to itself. Templates write the attributes with `clientAttrs`, `noBoost` and `confirmAttr` of `exp/interactions`.

Interaction kinds map as follows; `"include"`, `"pushURL"` and `"replaceURL"` options become `hx-include`, `hx-push-url` and `hx-replace-url` with htmx:

| Kind | htmx 2 / 4 | Unpoly | Turbo |
| --- | --- | --- | --- |
| `submit` (`submitAttrs`) | `hx-post` (or `hx-put`, `hx-patch`, `hx-delete` with `"method"`), `hx-target`, `hx-swap` | `up-submit`, `up-target` | `data-turbo-frame` for a target |
| `live` (`liveAttrs`) | `hx-get`, `hx-trigger="input changed delay:300ms, search"` | `up-autosubmit`, `up-watch-delay`, `up-target` | data attributes for a script |

## Partial connector

Use the neutral connector when your own fetch code sends go-partial headers.

```go
root := partial.New("shell.gohtml").
    SetConnector(connector.NewPartial(&connector.Config{
        UseURLQuery: true,
    })).
    SetFileSystem(os.DirFS("web"))
```

```js
await fetch("/rows", {
  headers: {
    "X-Target": "row-42",
    "X-Action": "refresh"
  }
});
```

When `UseURLQuery` is enabled, `target`, `select`, and `action` query parameters are used as a fallback after headers.

## Turbo

Turbo Frame requests can target a frame through the `Turbo-Frame` header.

```go
root := partial.New("shell.gohtml").
	SetConnector(connector.NewTurbo(nil)).
	SetFileSystem(os.DirFS("templates"))
content := partial.NewID("account-frame", "account.gohtml")
_ = partial.Write(ctx, w, r, root.Clone().SetContent(content))
```

```html
<turbo-frame id="account-frame" src="/account"></turbo-frame>
```

## Unpoly

Unpoly fragment requests use `X-Up-Target`.

```go
root := partial.New("shell.gohtml").
	SetConnector(connector.NewUnpoly(nil)).
	SetFileSystem(os.DirFS("templates"))
content := partial.NewID("content", "content.gohtml")
_ = partial.Write(ctx, w, r, root.Clone().SetContent(content))
```

```html
<a href="/settings" up-target="#content">Settings</a>
```

## Server-sent events

SSE is a writer layer, not a connector. Use it when the server decides to stream rendered patches after the initial request.

```go
events := sse.NewWriter(w)

status := partial.NewID("status", "status.gohtml").
    SetDot(StatusPatch{Step: 2})

_ = events.PatchPartial(r.Context(), r, "#status", status)
_ = events.Signal("progress", map[string]any{"step": 2})
events.Flush()
```

`PatchPartial` renders the partial itself, whatever the request targets. `events.SetParent(root)` renders partials that are not in a tree with the functions, stages and models of the application's root, without changing either.

The browser side is `sse.ClientScript()`, served from `clients.FS()`, a small script that applies `partial:patch` events with every connector, and with htmx processes the patched content. Elements with `data-partial-stream="/events"` open a stream while they are in the page, and a new one when the URL changes; with `data-partial-stream-hidden="30s"` the stream closes while the tab is hidden that long and opens again, with a `partial:stream:reconnect` event, when it shows.

`flash.ClientScript()` of `exp/flash` is the browser side of flash messages: it hides them after a time per level, keeps them under the pointer or focus, and with `data-flash-request-error` on the flash target shows an error for failed requests of htmx, Unpoly and Turbo.

The client is go-partial's own rather than htmx 4's `hx-sse` extension: one stream can patch any number of targets, it works with every connector and without htmx, and `Signal` events stay apart from HTML. `{{ stream }}` with the htmx 4 connector still emits `hx-sse:connect` for applications that use the extension and send it plain HTML.

## Custom connectors

If a frontend library does not define a stable fragment-request protocol, prefer `connector.NewPartial` or implement the connector interface in your application.

```go
type Connector interface {
    RenderPartial(r *http.Request) bool
    GetTargetValue(r *http.Request) string
    GetSelectValue(r *http.Request) string
    GetActionValue(r *http.Request) string

    GetTargetHeader() string
    GetSelectHeader() string
    GetActionHeader() string
    InteractionAttrs(interaction connector.Interaction) map[string]string
    ResponseHeaders(response connector.Response) map[string]string
}
```

`InteractionAttrs` turns an interaction of `exp/interactions` into attributes. The built-in connectors write its `Select` where the client library can select a part of the response, and its `Attrs` as given, after their own; a custom connector does the same.

Optional interfaces add to a custom connector what the built-in ones do: `connector.RequestInspector` (`IsRequest`, `IsBoosted`, `CurrentURL`), `connector.HeaderVarier` (`VaryHeaders`), `connector.CurrentURLHeaderNamer` (`CurrentURLHeader`), `connector.Redirector` (`Redirect`), `connector.AttrWriter` (`Attrs`), `connector.OOBWriter` (`OOBAttrs`) and `connector.RequestTargeter` (`SetRequestTarget`).
