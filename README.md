# Go Partial - Partial Template Rendering for Go

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

## Basic Usage

Here's a simple example of how to use the package to render a template.

### 1. Create a Service

The Service holds global configurations and data.

```go 
cfg := &partial.Config{
    PartialHeader: "X-Partial",          // Optional: Header to determine which partial to render
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

The Layout manages the overall structure of your templates.
```go
layout := service.NewLayout()
layout.SetData(map[string]any{
    "PageTitle": "Home Page",
})
```

### 3. Define Partials

Create Partial instances for the content and any other components.

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
    {{.Partials.content}}
</body>
</html>
```
templates/content.html
```html 
<h1>{{.Data.Message}}</h1>
```

## Using Global and Layout Data

- **Global Data (ServiceData)**: Set on the Service, accessible via {{.Service}} in templates.
- **Layout Data (LayoutData)**: Set on the Layout, accessible via {{.Layout}} in templates.
- **Partial Data (Data)**: Set on individual Partial instances, accessible via {{.Data}} in templates.

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


### Template Data
In your templates, you can access the following data:

- **{{.Ctx}}**: The context of the request.
- **{{.URL}}**: The URL of the request.
- **{{.Data}}**: Data specific to this partial.
- **{{.Service}}**: Global data available to all partials.
- **{{.Layout}}**: Data specific to the layout.
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
