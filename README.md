# Go Partial - Partial Page Rendering for Go

This package provides a flexible and efficient way to manage and render partial templates in Go (Golang). It allows you to create reusable, hierarchical templates with support for layouts, global data, caching, and more.
## Features

- **Partial Templates**: Define and render partial templates with their own data and functions.
- **Layouts**: Use layouts to wrap content and share data across multiple partials.
- **Global Data**: Set global data accessible to all partials.
- **Template Caching**: Enable caching of parsed templates for improved performance.
- **Out-of-Band Rendering**: Support for rendering out-of-band (OOB) partials.
- **File System Support**: Use custom fs.FS implementations for template file access.
- **Thread-Safe**: Designed for concurrent use in web applications.

## Installation
To install the package, run:
```bash
go get github.com/donseba/go-partial
```

## Advanced use cases 
Advanced usecases are documented in the [ADVANCED.md](ADVANCED.md) file

## Integrations
Several integrations are available, detailed information can be found in the [INTEGRATIONS.md](INTEGRATIONS.md) file
- htmx
- Turbo
- Stimulus
- Unpoly
- Alpine.js / Alpine Ajax (not great)
- Vue.js (not great)
- Standalone

## Basic Usage
Here's a simple example of how to use the package to render a template.

### 1. Create a Service
The `Service` holds global configurations and data.

```go 
cfg := &partial.Config{
    PartialHeader: "X-Target",          // Optional: Header to determine which partial to render
    UseCache:      true,                 // Enable template caching
    FuncMap:       template.FuncMap{},   // Global template functions
    Logger:        myLogger,             // Implement the Logger interface or use nil
}

service := partial.NewService(cfg)
service.SetData(map[string]any{
    "AppName": "My Application",
})

```

## 2. Create a Layout
The `Layout` manages the overall structure of your templates.
```go
layout := service.NewLayout()
layout.SetData(map[string]any{
    "PageTitle": "Home Page",
})
```

### 3. Define Partials
Create `Partial` instances for the content and any other components.

```go 
func handler(w http.ResponseWriter, r *http.Request) {
    // Create the main content partial
    content := partial.NewID("content", "templates/content.html")
    content.SetData(map[string]any{
        "Message": "Welcome to our website!",
    })
    
    // Optionally, create a wrapper partial (layout)
    wrapper := partial.NewID("wrapper", "templates/layout.html")
    
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
    <title>{{.Layout.PageTitle}} - {{.Service.AppName}}</title>
</head>
<body>
    {{ child "content" }}
</body>
</html>
```
templates/content.html
```html 
<h1>{{.Data.Message}}</h1>
```

Note: In the layout template, we use {{ child "content" }} to render the content partial on demand.


### Using Global and Layout Data
- **Global Data (ServiceData)**: Set on the Service, accessible via {{.Service}} in templates.
- **Layout Data (LayoutData)**: Set on the Layout, accessible via {{.Layout}} in templates.
- **Partial Data (Data)**: Set on individual Partial instances, accessible via {{.Data}} in templates.

### Accessing Data in Templates

You can access data in your templates using dot notation:

- **Partial Data**: `{{ .Data.Key }}`
- **Layout Data**: `{{ .Layout.Key }}`
- **Global Data**: `{{ .Service.Key }}`


### Wrapping Partials
You can wrap a partial with another partial, such as wrapping content with a layout:

```go
// Create the wrapper partial (e.g., layout)
layout := partial.New("templates/layout.html").ID("layout")

// Wrap the content partial with the layout
content.Wrap(layout)
```

## Rendering Child Partials on Demand
Use the child function to render child partials within your templates.

### Syntax
```html
{{ child "partial_id" }}
```
You can also pass data to the child partial using key-value pairs:

```html
{{ child "sidebar" "UserName" .Data.UserName "Notifications" .Data.Notifications }}
```
Child Partial (sidebar):
```html 
<div>
    <p>User: {{ .Data.UserName }}</p>
    <p>Notifications: {{ .Data.Notifications }}</p>
</div>
```

## Using Out-of-Band (OOB) Partials
Out-of-Band partials allow you to update parts of the page without reloading:

### Defining an OOB Partial
```go
// Create the OOB partial
footer := partial.New("templates/footer.html").ID("footer")
footer.SetData(map[string]any{
    "Text": "This is the footer",
})

// Add the OOB partial
p.WithOOB(footer)
```

### Using OOB Partials in Templates
In your templates, you can use the swapOOB function to conditionally render OOB attributes.

templates/footer.html
```html
<div {{ if swapOOB }}hx-swap-oob="true"{{ end }} id="footer">{{ .Data.Text }}</div>
```

## Wrapping Partials
You can wrap a partial with another partial, such as wrapping content with a layout.

```go
// Create the wrapper partial (e.g., layout)
layoutPartial := partial.New("templates/layout.html").ID("layout")

// Wrap the content partial with the layout
content.Wrap(layoutPartial)

```

## Template Functions
You can add custom functions to be used within your templates:

```go
import "strings"

// Define custom functions
funcs := template.FuncMap{
    "upper": strings.ToUpper,
}

// Set the functions for the partial
p.SetFuncs(funcs)
```

### Usage in Template:
```html
{{ upper .Data.Message }}
```

### Using a Custom File System
If your templates are stored in a custom file system, you can set it using WithFS:

```go
import (
    "embed"
)

//go:embed templates/*
var content embed.FS

p.WithFS(content)
```

If you do not use a custom file system, the package will use the default file system and look for templates relative to the current working directory.

## Rendering Tables and Dynamic Content
You can render dynamic content like tables by rendering child partials within loops.

Example: Rendering a Table with Dynamic Rows

templates/table.html
```html
<table>
    {{ range $i := .Data.Rows }}
    {{ child "row" "RowNumber" $i }}
    {{ end }}
</table>
```

templates/row.html
```html
<tr>
    <td>{{ .Data.RowNumber }}</td>
</tr>
```

Go Code:
```go
// Create the row partial
rowPartial := partial.New("templates/row.html").ID("row")

// Create the table partial and set data
tablePartial := partial.New("templates/table.html").ID("table")
tablePartial.SetData(map[string]any{
"Rows": []int{1, 2, 3, 4, 5}, // Generate 5 rows
})
tablePartial.With(rowPartial)

// Render the table partial
out, err := layout.Set(tablePartial).RenderWithRequest(r.Context(), r)
```

## Template Data
In your templates, you can access the following data:

- **{{.Ctx}}**: The context of the request.
- **{{.URL}}**: The URL of the request.
- **{{.Data}}**: Data specific to this partial.
- **{{.Service}}**: Global data available to all partials.
- **{{.Layout}}**: Data specific to the layout.

## Concurrency and Template Caching
The package includes concurrency safety measures for template caching:

- Templates are cached using a sync.Map.
- Mutexes are used to prevent race conditions during template parsing.
- Set UseTemplateCache to true to enable template caching.

```go
cfg := &partial.Config{
    UseCache: true,
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
