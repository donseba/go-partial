package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	partial "github.com/donseba/go-partial"
	"github.com/donseba/go-partial/connector"
)

type Todo struct {
	ID       int
	Title    string
	Done     bool
	Priority string
}

type TodoPage struct {
	Title     string
	Items     []Todo
	OpenCount int
	DoneCount int // holds the count of done tasks
}

type App struct {
	service *partial.Service
	todos   []Todo
}

func main() {
	app := &App{
		service: partial.NewService(&partial.Config{
			Connector:        connector.NewHTMX(nil),
			FS:               os.DirFS("examples/todo"),
			UseTemplateCache: true,
			ErrorMode:        partial.ErrorModeDetailed,
		}),
		todos: []Todo{
			{ID: 1, Title: "Map template models to Go structs", Priority: "High"},
			{ID: 2, Title: "Generate a template type index", Priority: "High"},
			{ID: 3, Title: "Prototype GoLand completions", Done: true, Priority: "Medium"},
			{ID: 4, Title: "Keep HTML readable", Priority: "Always"},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.redirect)
	mux.HandleFunc("/todos", app.todosPage)
	mux.HandleFunc("/todos/detail", app.todoDetail)

	log.Println("todo example running on http://localhost:8091")
	log.Fatal(http.ListenAndServe(":8091", mux))
}

func (app *App) redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/todos", http.StatusFound)
}

func (app *App) todosPage(w http.ResponseWriter, r *http.Request) {
	page := TodoPage{
		Title:     "Contract-aware todos",
		Items:     app.todos,
		OpenCount: app.openCount(),
		DoneCount: app.doneCount(),
	}
	content := partial.NewID("content", "templates/todos.gohtml").
		SetModels(partial.Model("Page", page))
	layout := partial.NewID("layout", "templates/layout.gohtml")
	app.write(w, r, app.service.NewLayout().Set(content).Wrap(layout))
}

func (app *App) todoDetail(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	todo, ok := app.findTodo(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	content := partial.NewID("todo-detail", "templates/todo_detail.gohtml").
		SetFileSystem(os.DirFS("examples/todo")).
		SetConnector(connector.NewHTMX(nil)).
		UseTemplateCache(true).
		SetModels(partial.Model("Todo", todo))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := content.WriteWithRequest(context.Background(), w, r); err != nil {
		log.Printf("render detail: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *App) write(w http.ResponseWriter, r *http.Request, layout *partial.Layout) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := layout.WriteWithRequest(context.Background(), w, r); err != nil {
		log.Printf("render page: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (app *App) findTodo(id int) (Todo, bool) {
	for _, todo := range app.todos {
		if todo.ID == id {
			return todo, true
		}
	}
	return Todo{}, false
}

func (app *App) openCount() int {
	var count int
	for _, todo := range app.todos {
		if !todo.Done {
			count++
		}
	}
	return count
}

func (app *App) doneCount() int {
	var count int
	for _, todo := range app.todos {
		if todo.Done {
			count++
		}
	}
	return count
}
