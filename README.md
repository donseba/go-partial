# Go Partial

A Go package for rendering partial HTML snippets based on specific HTTP headers. It supports nested partials, out-of-band (OOB) partials, template caching, and more.

## Features

- **Partial Rendering**: Render specific parts of a webpage based on an HTTP header.
- **Nested Partials**: Support for nesting partials within each other.
- **Out-of-Band (OOB) Partials**: Render OOB partials for dynamic content updates without a full page reload.
- **Template Caching**: Optional template caching with concurrency safety.
- **Template Functions**: Support for custom template functions.
- **File System Support**: Use any fs.FS as the template file system.

## Installation
To install the package, run:
```bash
go get github.com/donseba/go-partial
```

## Usage
Below are examples demonstrating how to use the partial package in your Go projects.

### Basic Usage
```go
package main

import (
    "context"
    "net/http"

    "github.com/donseba/go-partial"
)

func main() {
    http.HandleFunc("/", handleRequest)
    http.ListenAndServe(":8080", nil)
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// cstom filesystem with the templates for mini website
	fsys := &partial.InMemoryFS{
		Files: map[string]string{
			"templates/index.html":   "<html><body>{{.Partials.content }}</body></html>",
			"templates/content.html": "<div>{{.Data.Text}}</div>",
		},
	}

    // Create the root partial
    p := partial.New("templates/index.html").ID("root").WithFS(fsys)

    // Create a child partial
    content := partial.New("templates/content.html").ID("content")
    content.SetData(map[string]any{
        "Text": "Welcome to the home page",
    })

    // Add the child partial to the root
    p.With(content)

    // Render the partial based on the request
    out, err := p.RenderWithRequest(context.Background(), r)
    if err != nil {
        http.Error(w, "An error occurred while rendering the page", http.StatusInternalServerError)
        return
    }

    w.Write([]byte(out))
}
```

### Handling Partial Rendering

To render only a specific partial based on an HTTP header:
```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    // ... (setup code as before)

    // Render the partial based on the request
    out, err := p.RenderWithRequest(context.Background(), r)
    if err != nil {
        http.Error(w, "An error occurred while rendering the partial", http.StatusInternalServerError)
        return
    }

    w.Write([]byte(out))
}

// Setting the header to request a specific partial
request, _ := http.NewRequest(http.MethodGet, "/", nil)
request.Header.Set("X-Partial", "content")
```

### Using Out-of-Band (OOB) Partials
Out-of-Band partials allow you to update parts of the page without reloading:

```go
// Create the OOB partial
footer := partial.New("templates/footer.html").ID("footer")
footer.SetData(map[string]any{
    "Text": "This is the footer",
})

// Add the OOB partial
p.WithOOB(footer)
```

### Wrapping Partials
You can wrap a partial with another partial, such as wrapping content with a layout:

```go
// Create the wrapper partial (e.g., layout)
layout := partial.New("templates/layout.html").ID("layout")

// Wrap the content partial with the layout
content.Wrap(layout)
```

### Template Functions
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

## API Reference

### Types
`type Partial`

Represents a renderable component with optional children and data.

`type Data`

The data passed to templates during rendering.
```go
type Data struct {
    Ctx      context.Context
    URL      *url.URL
    Data     map[string]any
    Global   map[string]any
    Partials map[string]template.HTML
}
```

### Functions and Methods
- **New(templates ...string) \*Partial**: Creates a new root partial.
- **NewID(id string, templates ...string) \*Partial**: Creates a new partial with a specific ID.
- **(\*Partial) WithFS(fsys fs.FS) \*Partial**: Sets the file system for template files.
- **(\*Partial) ID(id string) \*Partial**: Sets the ID of the partial.
- **(\*Partial) Templates(templates ...string) \*Partial**: Sets the templates for the partial.
- **(\*Partial) SetData(data map[string]any) \*Partial**: Sets the data for the partial.
- **(\*Partial) AddData(key string, value any) \*Partial**: Adds a data key-value pair to the partial.
- **(\*Partial) SetGlobalData(data map[string]any) \*Partial**: Sets global data available to all partials.
- **(\*Partial) AddGlobalData(key string, value any) \*Partial**: Adds a global data key-value pair.
- **(\*Partial) SetFuncs(funcs template.FuncMap) \*Partial**: Sets template functions for the partial.
- **(\*Partial) AddFunc(name string, fn interface{}) \*Partial**: Adds a single template function.
- **(\*Partial) AppendFuncs(funcs template.FuncMap) \*Partial**: Appends template functions if they don't exist.
- **(\*Partial) AddTemplate(template string) \*Partial**: Adds an additional template to the partial.
- **(\*Partial) With(child \*Partial) \*Partial**: Adds a child partial.
- **(\*Partial) WithOOB(child \*Partial) \*Partial**: Adds an out-of-band child partial.
- **(\*Partial) Wrap(renderer \*Partial) \*Partial**: Wraps the partial with another partial.
- **(\*Partial) RenderWithRequest(ctx context.Context, r \*http.Request) (template.HTML, error)**: Renders the partial based on the HTTP request.


### Template Data
In your templates, you can access the following data:

- **{{.Ctx}}**: The context of the request.
- **{{.URL}}**: The URL of the request.
- **{{.Data}}**: Data specific to this partial.
- **{{.Global}}**: Global data available to all partials.
- **{{.Partials}}**: Rendered HTML of child partials.

### Concurrency and Template Caching
The package includes concurrency safety measures for template caching:

- Templates are cached using a sync.Map.
- Mutexes are used to prevent race conditions during template parsing.
- Set UseTemplateCache to true to enable template caching.

```go
partial.UseTemplateCache = true
```

## Contributing

Contributions are welcome! Please open an issue or submit a pull request with your improvements.

## License

This project is licensed under the [MIT License](LICENSE).
```
