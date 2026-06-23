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
	fsys.AddFile("table.gohtml", `<table><tbody>{{ range .Data.Rows }}{{ partial "row" (dict "Row" .) }}{{ end }}</tbody></table>`)
	fsys.AddFile("row.gohtml", `<tr id="row-{{ scoped.Row.ID }}"><td>{{ scoped.Row.Name }}</td></tr>`)

	table := NewID("content", "table.gohtml").
		SetFileSystem(fsys).
		SetData(map[string]any{"Rows": rows})
	rowPartial := NewID("row", "row.gohtml").SetFileSystem(fsys)
	table.With(rowPartial)
	table.WithTargetResolver(func(ctx context.Context, r *http.Request, target string) (*Partial, map[string]any, bool) {
		if !strings.HasPrefix(target, "row-") {
			return nil, nil, false
		}
		id, err := strconv.Atoi(strings.TrimPrefix(target, "row-"))
		if err != nil {
			return nil, nil, false
		}
		for _, candidate := range rows {
			if candidate.ID == id {
				return rowPartial, map[string]any{"Row": candidate}, true
			}
		}
		return nil, nil, false
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
