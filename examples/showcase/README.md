# go-partial showcase

Run from the repository root:

```bash
go run ./examples/showcase
```

Open http://localhost:8090.

The app uses actual template files from `examples/showcase/templates`, htmx 4 on the frontend with `connector.NewHTMX4`, and `examples/showcase/static/site.css` for styling. The live metrics feed and the stream interaction use htmx 4's `hx-sse` extension, which swaps SSE messages without an event name. The infinite-scroll page uses cursor-style values such as `X-Action: current-25`, appends flash messages for loaded chunks, and swaps in a YouTube embed after row 150. The SSE page opens an `EventSource` and patches server-rendered partials into the page.
