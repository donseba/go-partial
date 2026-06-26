# go-partial todo example

This example exists to test typed template models and IDE typeahead.

Run:

```bash
go run ./examples/todo
```

Open:

```text
http://localhost:8091/todos
```

go-doc can inspect the model contracts for editor support:

```bash
go-doc index .
```

The templates declare:

```gotemplate
@model Page github.com/donseba/go-partial/examples/todo.TodoPage
@model Todo github.com/donseba/go-partial/examples/todo.Todo
```

Those map to runtime model functions:

```gotemplate
{{ Page.Title }}
{{ Todo.Title }}
```

And the application binds them with:

```go
content.SetModels(partial.Model("Page", page))
detail.SetModels(partial.Model("Todo", todo))
```
