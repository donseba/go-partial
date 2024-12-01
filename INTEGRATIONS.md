# Supported Integrations (connectors)

`go-partial` currently supports the following connectors:

- HTMX
- Turbo
- Unpoly
- Alpine.js
- Stimulus
- Partial (Custom Connector)

## HTMX

### Description:
[HTMX](https://htmx.org/) allows you to use AJAX, WebSockets, and Server-Sent Events directly in HTML using attributes.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the HTMX connector
contentPartial.SetConnector(connector.NewHTMX(&connector.Config{
    UseURLQuery: true, // Enable fallback to URL query parameters
}))

// Optionally add actions or selections
contentPartial.WithAction(func(ctx context.Context, p *partial.Partial, data *partial.Data) (*partial.Partial, error) {
    // Action logic here
    return p, nil
})

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<!-- Load content into #content div when clicked -->
<button hx-get="/content" hx-target="#content" hx-headers='{"HX-Select": "tab1"}'>Tab 1</button>
<button hx-get="/content" hx-target="#content" hx-headers='{"HX-Select": "tab2"}'>Tab 2</button>

<!-- Content area -->
<div id="content">
    <!-- Dynamic content will be loaded here -->
</div>
```

### alternative: 
```html
<button hx-get="/content" hx-target="#content" hx-headers='{"X-Select": "tab1"}'>Tab 1</button>
<button hx-get="/content" hx-target="#content" hx-headers='{"X-Select": "tab2"}'>Tab 2</button>
```

### alternative 2:
```html
<button hx-get="/content" hx-target="#content" hx-params="select=tab1">Tab 1</button>
<button hx-get="/content" hx-target="#content" hx-params="select=tab2">Tab 2</button>
```

## Turbo
### Description:
[Turbo](https://turbo.hotwired.dev/) speeds up web applications by reducing the amount of custom JavaScript needed to provide rich, modern user experiences.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the Turbo connector
contentPartial.SetConnector(connector.NewTurbo(&connector.Config{
    UseURLQuery: true,
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<turbo-frame id="content">
    <!-- Dynamic content will be loaded here -->
</turbo-frame>

<!-- Links to load content -->
<a href="/content?select=tab1" data-turbo-frame="content">Tab 1</a>
<a href="/content?select=tab2" data-turbo-frame="content">Tab 2</a>
```

## Unpoly
### Description:
[Unpoly](https://unpoly.com/) enables fast and flexible server-side rendering with minimal custom JavaScript.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the Unpoly connector
contentPartial.SetConnector(connector.NewUnpoly(&connector.Config{
    UseURLQuery: true,
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<!-- Links to load content -->
<a href="/content?select=tab1" up-target="#content" up-headers='{"X-Up-Select": "tab1"}'>Tab 1</a>
<a href="/content?select=tab2" up-target="#content" up-headers='{"X-Up-Select": "tab2"}'>Tab 2</a>

<!-- Content area -->
<div id="content">
    <!-- Dynamic content will be loaded here -->
</div>
```

### Alternative:
```html
<a href="/content?select=tab1" up-target="#content">Tab 1</a>
<a href="/content?select=tab2" up-target="#content">Tab 2</a>
```

## Alpine.js
### Description:
[Alpine.js](https://alpinejs.dev/) offers a minimal and declarative way to render reactive components in the browser.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the Alpine.js connector
contentPartial.SetConnector(connector.NewAlpine(&connector.Config{
    UseURLQuery: true,
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<div>
    <!-- Buttons to load content -->
    <form method="get" action="/page" x-target="content" x-headers="{'X-Alpine-Select': 'tab1'}">
        <button type="submit">Tab 1</button>
    </form>    
    
    <form method="get" action="/page" x-target="content" x-headers="{'X-Alpine-Select': 'tab2'}">
        <button type="submit">Tab 2</button>
    </form>

    <!-- Content area -->
    <div id="content">
        <!-- Dynamic content will be loaded here -->
    </div>
</div>
```

## Alpine Ajax
### Description:
[Alpine Ajax](https://alpine-ajax.js.org) is an Alpine.js plugin that enables your HTML elements to request remote content from your server.
### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the Alpine-AJAX connector
contentPartial.SetConnector(connector.NewAlpineAjax(&connector.Config{
UseURLQuery: true, // Enable fallback to URL query parameters
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<!-- Include Alpine.js and Alpine-AJAX -->
<script src="https://cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js" defer></script>
<script src="https://unpkg.com/alpine-ajax@latest/dist/alpine-ajax.min.js" defer></script>

<!-- Initialize Alpine -->
<div x-data>

    <!-- Buttons to load content -->
    <button x-on:click="click" x-get="/content" x-target="#content" x-headers='{"X-Alpine-Select": "tab1"}'>Tab 1</button>
    <button x-on:click="click" x-get="/content" x-target="#content" x-headers='{"X-Alpine-Select": "tab1"}'>Tab 2</button>

    <!-- Content area -->
    <div id="content">
        <!-- Dynamic content will be loaded here -->
    </div>
</div>

```

## Stimulus
### Description:
[Stimulus](https://stimulus.hotwired.dev/) is a JavaScript framework that enhances static or server-rendered HTML with just enough behavior.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the Stimulus connector
contentPartial.SetConnector(connector.NewStimulus(&connector.Config{
    UseURLQuery: true,
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<div data-controller="ajax">
    <!-- Buttons to load content -->
    <button data-action="click->ajax#load" data-select="tab1">Tab 1</button>
    <button data-action="click->ajax#load" data-select="tab2">Tab 2</button>

    <!-- Content area -->
    <div id="content">
        <!-- Dynamic content will be loaded here -->
    </div>
</div>

<script>
import { Controller } from "stimulus"

export default class extends Controller {
    load(event) {
        event.preventDefault()
        const select = event.target.dataset.select
        fetch('/content', {
            headers: {
                'X-Stimulus-Target': 'content',
                'X-Stimulus-Select': select
            }
        })
        .then(response => response.text())
        .then(html => {
            document.getElementById('content').innerHTML = html
        })
    }
}
</script>
```

## Partial (Custom Connector)
### Description:
The Partial connector is a simple, custom connector provided by go-partial. It can be used when you don't rely on any specific front-end library.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the custom Partial connector
contentPartial.SetConnector(connector.NewPartial(&connector.Config{
    UseURLQuery: true,
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

```
### Client-Side Usage:
```html
<button x-get="/" x-target="#content" x-select="tab1">Tab 1</button>
<button x-get="/" x-target="#content" x-select="tab2 ">Tab 2</button>

<!-- Content area -->
<div id="content">
    <!-- Dynamic content will be loaded here -->
</div>
```

## Vue.js
### Description:
[Vue.js](https://vuejs.org/) is a progressive JavaScript framework for building user interfaces.

### Note:
Integrating go-partial with Vue.js for partial HTML updates is possible but comes with limitations. For small sections of the page or simple content updates, it can work. For larger applications, consider whether server-rendered partials align with your architecture.

### Server-Side Setup:
```go
import (
    "github.com/donseba/go-partial"
    "github.com/donseba/go-partial/connector"
)

// Create a new partial
contentPartial := partial.New("templates/content.gohtml").ID("content")

// Set the Vue connector
contentPartial.SetConnector(connector.NewVue(&connector.Config{
    UseURLQuery: true,
}))

// Handler function
func contentHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

### Client-Side Setup:
```html
<template>
  <div>
    <!-- Buttons to load content -->
    <button @click="loadContent('tab1')">Tab 1</button>
    <button @click="loadContent('tab2')">Tab 2</button>

    <!-- Content area -->
    <div v-html="content"></div>
  </div>
</template>

<script>
export default {
  data() {
    return {
      content: ''
    }
  },
  methods: {
    loadContent(select) {
      fetch('/content', {
        headers: {
          'X-Vue-Target': 'content',
          'X-Vue-Select': select
        }
      })
      .then(response => response.text())
      .then(html => {
        this.content = html;
      });
    }
  }
}
</script>
```

### using axios:
```javascript
methods: {
  loadContent(select) {
    axios.get('/content', {
      headers: {
        'X-Vue-Target': 'content',
        'X-Vue-Select': select
      }
    })
    .then(response => {
      this.content = response.data;
    });
  }
}
```