# go-partial upcoming

## go-doc Template Function Support

go-partial now publishes optional interaction template helper functions in:

```text
github.com/donseba/go-partial/exp/interactions
```

These helpers contain the endpoint/interaction parsing logic and are registered
explicitly with `service.SetFunc(interactions.FuncMap())`. Core go-partial no
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

Register it with `service.SetFunc(flash.FuncMap())` and
`service.Use(flash.Stage())`. Templates can render `{{ flashTarget }}` once
in a layout and `{{ flash }}` in request or fragment templates. Message and
target templates are embedded by default and can be replaced with
`flash.WithTemplate`, `flash.WithPartial`, `flash.WithTargetTemplate`,
`flash.WithTargetPartial`, and `flash.WithTargetID`.

## Helper Provider Split

The protected helper set has been reduced to go-partial's actual core helpers:
runtime, layout/content, request/context, OOB, and connector state helpers.
Generic helpers such as `dict`, string helpers, date helpers, and counters are
ordinary template helpers and can be provided through:

```text
github.com/donseba/go-partial/exp/templatehelpers
```

The repository `.go-doc/config.json` now advertises optional providers such as
`exp/flash`, `exp/interactions`, and `exp/templatehelpers`.
