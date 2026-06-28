# Go Partial - Partial Page Rendering for Go

This package provides a request-aware rendering layer for Go templates. It lets applications render full pages or targeted partials from the same registered template tree, with layout wrapping, connector headers, OOB output, caching, and typed template-friendly data flow.
## Features

- **Partial Templates**: Define and render partial templates with typed dot data and functions.
- **Native Template Composition**: Use `{{ template "row.gohtml" . }}` with `SetDot` so reusable sections can render inside a page or as HTMX targets.
- **Layouts**: Use layouts to wrap content while keeping page data explicit.
- **Template Caching**: Enable caching of parsed templates for improved performance.
- **Out-of-Band Rendering**: Support for rendering out-of-band (OOB) partials.
- **File System Support**: Use custom fs.FS implementations for template file access.
- **Thread-Safe**: Designed for concurrent use in web applications.

## Installation
To install the package, run:
```bash
go get github.com/donseba/go-partial
```

## Template functions
Template-facing helpers and accessors are documented in [TEMPLATE_FUNCTIONS.md](TEMPLATE_FUNCTIONS.md).

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

Open http://localhost:8090 to see pages for typed rows, selection partials, actions, out-of-band rendering, context helpers, localization, HTMX response headers, server-sent events, infinite scroll with cursor-style `X-Action` values, and the embedded error page.

## Integrations
Several integrations are available, detailed information can be found in the [INTEGRATIONS.md](INTEGRATIONS.md) file.

- htmx
- Turbo
- Unpoly
- Partial, for framework-neutral fetch clients and tests
- SSE writer, for streaming rendered HTML patches

## Embedded Error Page
`WriteWithRequest` renders a built-in HTML error page when template parsing or execution fails. In detailed mode, the page includes the partial ID, template list, request URL, template location, and original error so development and failed htmx requests still return useful output.

Normal requests receive a `500` full HTML page. HTMX/partial requests receive a swappable error fragment with status `200` and `X-Go-Partial-Error: true`, because HTMX does not swap `500` responses by default.

The default renderer does not show a Go stack trace. Template parse and execution errors already carry the useful template location, while a stack captured during fallback rendering mostly describes the application render wrapper path.

You can replace the default renderer globally:

```go
service := partial.NewService(&partial.Config{
    ErrorRenderer: func(ctx context.Context, p *partial.Partial, r *http.Request, err error) (template.HTML, error) {
        return template.HTML(`<div class="error">` + template.HTMLEscapeString(err.Error()) + `</div>`), nil
    },
})
```

Or on one partial:

```go
content.SetErrorRenderer(partial.DefaultErrorRenderer())
```

`RenderWithRequest` still returns the render error directly. The fallback page is written by `WriteWithRequest`.

## Localization
Templates receive a request localizer as `.Loc`. The core interface only requires `GetLocale()`. Translation behavior should come from user-provided template functions registered with `Service.UseFuncs`:

```go
service.UseFuncs(translator.FuncMap())
```

```go
ctx := context.WithValue(r.Context(), partial.LocalizerContextKey, localizer)
_ = content.WriteWithRequest(ctx, w, r)
```

```html
<p>{{ tl .Loc "Hello, World!" }}</p>
<p>{{ tn .Loc "You have one message." "You have %d messages." 5 5 }}</p>
<button>{{ ctl .Loc "button" "save" }}</button>
```

`tl`, `tn`, `ctl`, and `ctn` are not built into go-partial. For a fuller translation backend, [github.com/donseba/go-translator](https://github.com/donseba/go-translator) fits this pattern well because it exposes a compatible `FuncMap()`.

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

With the HTMX connector, these become `HX-Retarget`, `HX-Reswap`, and `HX-Trigger` headers during `WriteWithRequest`.

## Debug Helper
The `debug` template helper renders a styled diagnostic box using an embedded template:

```gotemplate
{{ debug . }}
```

Override debug output globally, per layout, or per partial:

```go
service.SetDebugRenderer(func(ctx context.Context, p *partial.Partial, data *partial.Data, value any) (template.HTML, error) {
    return template.HTML(`<pre class="debug">` + template.HTMLEscapeString(fmt.Sprintf("%#v", value)) + `</pre>`), nil
})
```

## Server-Sent Events
SSE is a writer layer, not a connector. Use it after deciding which partials changed:

```go
events := partial.NewSSEWriter(w)

notice := partial.NewID("notice", "notice.gohtml").
    SetDot(Notice{Message: "Saved"})

_ = events.PatchPartial(r.Context(), r, "#notice", notice)
_ = events.Signal("saved", true)
events.Flush()
```

The writer declares constants for expected headers and event names, such as `SSEHeaderContentType`, `SSEContentTypeEventStream`, `SSEEventPatch`, `SSEEventSignal`, and `SSEEventError`.

## Basic Usage
Here's a simple typed example. The template owns the contract, the handler supplies the model.

### 1. Create a Service
The `Service` holds shared render configuration.

```go 
cfg := &partial.Config{
    Connector:        connector.NewHTMX(nil), // Choose how request headers are read
    FS:               os.DirFS("web"),        // Template filesystem
    UseTemplateCache: true,                   // Enable parsed template caching
    Logger:           myLogger,               // Implement the Logger interface or use nil
}

service := partial.NewService(cfg)
service.UseFuncs(template.FuncMap{
    "money": formatMoney,
})

```

## 2. Create a Layout
The `Layout` manages the overall structure of your templates.
```go
layout := service.NewLayout()
```

### 3. Define Partials
Create `Partial` instances for the content and any other components.

```go 
type ContentPage struct {
    PageTitle string
    Message string
}

type LayoutPage struct {
    AppName string
}

func handler(w http.ResponseWriter, r *http.Request) {
    // Create the main content partial
    content := partial.NewID("content", "templates/content.html").
        SetDot(ContentPage{
            PageTitle: "Home Page",
            Message:   "Welcome to our website!",
        })
    
    // Optionally, create a wrapper partial (layout)
    wrapper := partial.NewID("wrapper", "templates/layout.html").
        SetDot(LayoutPage{AppName: "My Application"})
    
    layout.Set(content)
    layout.Wrap(wrapper)
    
    output, err := layout.RenderWithRequest(r.Context(), r)
    if err != nil {
        http.Error(w, "An error occurred while rendering the page.", http.StatusInternalServerError)
        return
    }
    w.Write([]byte(output))
}
```

## Template Files
templates/layout.html
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

Note: When a layout wraps content, the wrapper renders the configured route partial by calling `{{ content }}`.


### Accessing Data in Templates

Use `SetDot` for application data:

```gotemplate
{{ .PageTitle }}
{{ .Message }}
```

For shared application values, put them on a typed model and declare them with go-doc `@model`:

```gotemplate
{{/*
@model Service github.com/example/app.ServiceInfo
*/}}

{{ Service.AppName }}
```

```go
wrapper.UseModels(serviceInfo)
```

When `SetDot` is used, request-specific values are available through helpers instead of fields on dot:

```gotemplate
{{ ctx.Locale }}
{{ ctx.URL.Path }}
{{ request.Method }}
{{ locale }}
{{ csrf.Key }}
{{ basePath }}
```

## Contract Models
go-partial can register go-doc `@model` declarations before parsing templates. The declaration owns the template name, while the controller only supplies typed values:

```gotemplate
{{/*
@model Page github.com/example/app.DashboardPage
*/}}

<h1>{{ Page.Title }}</h1>
```

```go
content := partial.NewID("content", "templates/dashboard.gohtml").
    UseModels(page)
```

`UseModels` appends values to the models already inherited through the partial tree. Use it when a partial adds local models on top of parent models.

Model names cannot collide with go-partial helpers such as `partial`, `locale`, `ctx`, or `url`.

## Template Composition
For repeated sections that should be understood by go-doc and also render as HTMX targets, prefer native templates plus `SetDot`. The parent owns the loop, and the nested template receives the row as normal dot data:

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

Register the row partial on the parent so the parent parse can see the row template and so an HTMX request can still target a row by ID:

```go
rowPartial := partial.NewID("row", "templates/row.html")
table.With(rowPartial)
table.WithTargetResolver(func(ctx context.Context, r *http.Request, target string) (*partial.Partial, map[string]any, bool) {
    row := findRowForTarget(target)
    return partial.NewID(target, "templates/row.html").SetDot(row), nil, true
})
```

For layout wrappers, render the route content through `content`:

```gotemplate
<main>{{ content }}</main>
```

Avoid using `partial` as a general row-composition helper in new code. Native `template` calls keep the template idiomatic and give go-doc the strongest type information. Use `partial` when you intentionally want to render a template path through go-partial's render path:

```gotemplate
{{ partial "templates/notice.gohtml" .Notice }}
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

### Using OOB Partials in Templates
In your templates, you can use the `oobAttr` function to conditionally render OOB attributes.

templates/footer.html
```html
<div{{ oobAttr }} id="footer">{{ .Text }}</div>
```

## Template Functions
You can add custom functions to be used within your templates:

```go
import "strings"

// Define custom functions
funcs := template.FuncMap{
    "upper": strings.ToUpper,
}

// Merge the functions into this partial tree
p.UseFuncs(funcs)
```

`UseFuncs` merges helpers into the current scope. Function names inherited from the service or layout remain available, and protected go-partial helper names cannot be overwritten.

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
var content embed.FS

p.SetFileSystem(content)
```

If you do not use a custom file system, the package will use the default file system and look for templates relative to the current working directory.

## Rendering Tables and Dynamic Content
For tables and repeated fragments, prefer native Go templates plus `SetDot`. The parent receives a typed page model, ranges over rows, and calls the row template with `{{ template "row.html" . }}`. That keeps the template readable for go-doc while go-partial still knows the row partial ID for HTMX target requests.

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
tablePartial.With(rowPartial)

tablePartial.WithTargetResolver(func(ctx context.Context, r *http.Request, target string) (*partial.Partial, map[string]any, bool) {
    row, ok := findRowForTarget(target)
    if !ok {
        return nil, nil, false
    }
    return partial.NewID(target, "templates/row.html").SetDot(row), nil, true
})

out, err := layout.Set(tablePartial).RenderWithRequest(r.Context(), r)
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
- **@model values**: Additional typed values registered with `UseModels`.
- **{{ctx}}**, **{{request}}**, **{{url}}**, **{{locale}}**, **{{csrf}}**, **{{basePath}}**: request-aware helpers that stay available when `SetDot` changes `.`.

The older map-wrapper APIs (`SetData`, `AddData`, and `MergeData`) are still available for tests and compatibility, but they are not the recommended v1 data model. They expose values through `.Data`, which go-doc cannot type as precisely as a real model.

## Concurrency and Template Caching
The package includes concurrency safety measures for template caching:

- Parsed templates are cached per service.
- Mutexes prevent duplicate parsing for the same service/template/function shape.
- Cached templates are rebound with request-specific functions per render.
- Rendered HTML is not cached.
- Set `UseTemplateCache` to `true` to enable parsed template caching.

```go
cfg := &partial.Config{
    UseTemplateCache: true,
}
```

## Handling Partial Rendering via HTTP Headers
You can render specific partials based on the X-Target header (or your custom header).

Example:
```go
func handler(w http.ResponseWriter, r *http.Request) {
    output, err := layout.RenderWithRequest(r.Context(), r)
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

which would mean that caching is rougly 9-10 times faster than without caching


## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your improvements.

## License

This project is licensed under the [MIT License](LICENSE).
```
