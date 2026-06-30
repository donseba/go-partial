package partial

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/donseba/go-partial/connector"
)

func TestTargetResolverRendersDynamicRowTarget(t *testing.T) {
	type row struct {
		ID   int
		Name string
	}

	rows := []row{
		{ID: 1, Name: "Coffee"},
		{ID: 2, Name: "Tea"},
	}

	fsys := &inMemoryFS{}
	fsys.AddFile("table.gohtml", `<table><tbody>{{ range .Rows }}{{ template "row.gohtml" . }}{{ end }}</tbody></table>`)
	fsys.AddFile("row.gohtml", `<tr id="row-{{ .ID }}"><td>{{ .Name }}</td></tr>`)

	table := NewID("content", "table.gohtml").
		SetFileSystem(fsys).
		SetDot(map[string]any{"Rows": rows}).
		SetFunc(testTargetFuncMap()).
		Use(testTargetRenderer())
	rowPartial := NewID("row", "row.gohtml").SetFileSystem(fsys)
	table.With(rowPartial)
	testUseTargetResolver(table, func(ctx context.Context, r *http.Request, target string) (*Partial, bool) {
		if !strings.HasPrefix(target, "row-") {
			return nil, false
		}
		id, err := strconv.Atoi(strings.TrimPrefix(target, "row-"))
		if err != nil {
			return nil, false
		}
		for _, candidate := range rows {
			if candidate.ID == id {
				return NewID(target, "row.gohtml").SetFileSystem(fsys).SetDot(candidate), true
			}
		}
		return nil, false
	})

	req := httptest.NewRequest(http.MethodGet, "/rows", nil)
	req.Header.Set(connector.HeaderTarget.String(), "row-2")

	out, err := table.RenderWithRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("render with dynamic target: %v", err)
	}

	if string(out) != `<tr id="row-2"><td>Tea</td></tr>` {
		t.Fatalf("unexpected row output: %q", out)
	}
}
