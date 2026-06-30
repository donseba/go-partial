# Integrations

go-partial integrates with frontend tools through connectors. A connector tells go-partial how to read request intent, such as the DOM target, selected partial, or action, and how to write protocol-specific response headers.

## Core connectors

| Connector | Target header | Use |
| --- | --- | --- |
| `connector.NewHTMX` | `HX-Target` | HTMX requests, boosted navigation, response headers, and out-of-band swaps. |
| `connector.NewPartial` | `X-Target` | Framework-neutral fetch clients, tests, and custom JavaScript. |
| `connector.NewTurbo` | `Turbo-Frame` | Turbo Frame requests. |
| `connector.NewUnpoly` | `X-Up-Target` | Unpoly fragment requests. |

Selection and action values use the shared `X-Select` and `X-Action` headers unless a connector defines something else.

## HTMX

```go
svc := partial.NewService(&partial.Config{
    Connector: connector.NewHTMX(nil),
    FS:        os.DirFS("web"),
})
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

_ = notice.WriteWithRequest(r.Context(), w, r)
```

The HTMX connector writes headers such as `HX-Retarget`, `HX-Reswap`, and `HX-Trigger`.

## Partial connector

Use the neutral connector when your own fetch code sends go-partial headers.

```go
svc := partial.NewService(&partial.Config{
    Connector: connector.NewPartial(&connector.Config{
        UseURLQuery: true,
    }),
    FS: os.DirFS("web"),
})
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
content := partial.NewID("account-frame", "account.gohtml")
content.SetConnector(connector.NewTurbo(nil))
```

```html
<turbo-frame id="account-frame" src="/account"></turbo-frame>
```

## Unpoly

Unpoly fragment requests use `X-Up-Target`.

```go
content := partial.NewID("content", "content.gohtml")
content.SetConnector(connector.NewUnpoly(nil))
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
    ResponseHeaders(response connector.Response) map[string]string
}
```
