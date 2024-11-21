# Advanced Use Cases
The go-partial package offers advanced features to handle dynamic content rendering based on user interactions or server-side logic. Below are three advanced use cases:

- **WithSelection**: Selecting partials based on a selection key.
- **WithAction**: Executing server-side actions during request processing.
- **WithTemplateAction**: Invoking actions from within templates.

## WithSelection
### Purpose
WithSelection allows you to select and render one of several predefined partials based on a selection key, such as a header value or query parameter. This is useful for rendering different content based on user interaction, like tabbed interfaces.
### When to Use
Use WithSelection when you have a static set of partials and need to render one of them based on a simple key provided by the client.
### How to Use
#### Step 1: Define the Partials
Create partials for each selectable content.
```go
tab1Partial := partial.New("tab1.gohtml").ID("tab1")
tab2Partial := partial.New("tab2.gohtml").ID("tab2")
defaultPartial := partial.New("default.gohtml").ID("default")
```

#### Step 2: Create a Selection Map
Map selection keys to their corresponding partials.
```go
partialsMap := map[string]*partial.Partial{
"tab1":    tab1Partial,
"tab2":    tab2Partial,
"default": defaultPartial,
}
``` 

#### Step 3: Set Up the Content Partial with Selection
Use WithSelection to associate the selection map with your content partial.
```go
contentPartial := partial.New("content.gohtml").ID("content").WithSelection("default", partialsMap)
```

#### Step 4: Update the Template
In your content.gohtml template, use the {{selection}} function to render the selected partial.
```html
<div class="content">
    {{selection}}
</div>
```

#### Step 5: Handle the Request
In your handler, render the partial as usual.
```go
func yourHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := contentPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```

#### Client-Side Usage
Set the selection key via a header (e.g., X-Select) or another method.
```html
<div hx-get="/your-endpoint" hx-headers='{"X-Select": "tab1"}'>
    <!-- Content will be replaced with tab1 content -->
</div>
```

## WithAction
### Purpose
WithAction allows you to execute server-side logic during request processing, such as handling form submissions or performing business logic, and then render a partial based on the result.
### When to Use
Use WithAction when you need to perform dynamic operations before rendering, such as processing form data, updating a database, or any logic that isn't just selecting a predefined partial.

### How to Use
#### Step 1: Define the Partial with an Action
Attach an action function to the partial using WithAction.
```go
formPartial := partial.New("form.gohtml").ID("contactForm").WithAction(func(ctx context.Context, data *partial.Data) (*partial.Partial, error) {
// Access form values
name := data.Request.FormValue("name")
email := data.Request.FormValue("email")

    // Perform validation and business logic
    if name == "" || email == "" {
        errorPartial := partial.New("error.gohtml").AddData("Message", "Name and email are required.")
        return errorPartial, nil
    }

    // Simulate saving to a database or performing an action
    // ...

    // Decide which partial to render next
    successPartial := partial.New("success.gohtml").AddData("Name", name)
    return successPartial, nil
})
```
#### Step 2: Handle the Request
In your handler, render the partial with the request.
```go
func submitHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := formPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```
#### Step 3: Client-Side Usage
Submit the form with an X-Action header specifying the partial ID.
```html 
<form hx-post="/submit" hx-headers='{"X-Action": "contactForm"}'>
    <input type="text" name="name" placeholder="Your Name">
    <input type="email" name="email" placeholder="Your Email">
    <button type="submit">Submit</button>
</form>
```
#### Explanation

- The form submission triggers the server to execute the action associated with contactForm.
- The action processes the form data and returns a partial to render.
- The server responds with the rendered partial (e.g., a success message).

## WithTemplateAction
### Purpose
WithTemplateAction allows actions to be executed from within the template using a function like {{action "actionName"}}. This provides flexibility to execute actions conditionally or at specific points in your template.
### When to Use
Use WithTemplateAction when you need to invoke actions directly from the template, possibly under certain conditions, or when you have multiple actions within a single template.
### How to Use

#### Step 1: Define the Partial with Template Actions
Attach template actions to your partial.
```go
myPartial := partial.New("mytemplate.gohtml").ID("myPartial").WithTemplateAction("loadDynamicContent", func(ctx context.Context, data *partial.Data) (*partial.Partial, error) {
    // Load dynamic content
    dynamicPartial := partial.New("dynamic.gohtml")
    // Add data or perform operations
    return dynamicPartial, nil
})
```
#### Step 2: Update the Template
In your mytemplate.gohtml template, invoke the action using the action function.
```html 
<div>
    <!-- Some content -->
    {{action "loadDynamicContent"}}
    <!-- More content -->
</div>
```
#### Step 3: Handle the Request
Render the partial as usual.
```go
func yourHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    err := myPartial.WriteWithRequest(ctx, w, r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
```
#### Use Cases

- Conditional Execution
    ```gotemplate
    {{if .Data.ShowSpecialContent}}
        {{action "loadSpecialContent"}}
    {{end}}
    ```
- Lazy Loading
    ```gotemplate
    <div hx-get="/loadContent" hx-trigger="revealed">
        {{action "loadHeavyContent"}}
    </div>
    ```
#### Explanation

- Actions are only executed when the {{action "actionName"}} function is called in the template.
- This allows for conditional or multiple action executions within the same template.
- The server-side action returns a partial to render, which is then included at that point in the template.

## Choosing the Right Approach
- Use `WithSelection` when you have a set of predefined partials and want to select one based on a simple key.
- Use `WithAction` when you need to perform server-side logic during request processing and render a partial based on the result.
- Use `WithTemplateAction` when you want to invoke actions directly from within the template, especially for conditional execution or multiple actions.

## Notes

- **Separation of Concerns**: While WithTemplateAction provides flexibility, be cautious not to overload templates with business logic. Keep templates focused on presentation as much as possible.
- **Error Handling**: Ensure that your actions handle errors gracefully and that your templates can render appropriately even if an action fails.
- **Thread Safety**: If your application is concurrent, ensure that shared data is properly synchronized.