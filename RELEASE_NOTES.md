# go-partial upcoming

## go-doc Template Function Support

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

## Showcase Cleanup

The interaction showcase now uses the typed named interaction form for the
default async example instead of an intentionally-invalid placeholder call.

The showcase also demonstrates `exp/flash` for transient SSR/HTMX messages.
Async rows, infinite-scroll chunks, and webshop cart actions append flash
messages into a stable `flashTarget` container and remove them after a short
delay.

## Flash Messages

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

## Render Stages

The render pipeline vocabulary now uses render stages. New code should use
`partial.RenderStage`, `partial.RenderStageHooks`, `Partial.Use`, and
package constructors such as `metrics.Stage(...)`, `flash.Stage(...)`, and
`errors.Stage(...)`.

## Package Rendering Functions

Partial rendering and HTTP writing now have package-level entry points:
`partial.Render(ctx, p)`, `partial.RenderWithRequest(ctx, r, p)`, and
`partial.Write(ctx, w, r, p)`. `partial.Write` owns response headers, connector
response instructions, render-stage response metadata, error fragments, and
out-of-band output. `Partial` no longer has render or write methods; this is an
intentional pre-v1 break so rendering is owned by package-level functions.

## Helper Provider Split

The protected helper set has been reduced to go-partial's actual core helpers:
runtime, wrapper/content, request/context, OOB, and connector state helpers.
Generic helpers such as `dict`, string helpers, date helpers, and counters are
ordinary template helpers and can be provided through:

```text
github.com/donseba/go-partial/exp/templatehelpers
```

The repository `.go-doc/config.json` now advertises optional providers such as
`exp/flash`, `exp/interactions`, and `exp/templatehelpers`.
