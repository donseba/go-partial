package partial_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// skippedDirs are left out of TestStagesNameTheirPackage, as are hidden
// directories: the examples are applications.
var skippedDirs = []string{"examples", "testdata", "node_modules"}

// TestStagesNameTheirPackage checks that every render stage the packages of
// this module make with RenderStageHooks sets its Package. Where the
// compiler inlines the function that returns such a stage into an
// application, its hooks are named after the application, and without
// Package the stand-ins of the stage's FuncMap raise false func.protected
// warnings. A test shows that only from another package and only where the
// compiler inlines the function, so this checks the source instead.
func TestStagesNameTheirPackage(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			name := entry.Name()
			if path != "." && (strings.HasPrefix(name, ".") || slices.Contains(skippedDirs, name)) {
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			checkStagePackages(t, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkStagePackages(t *testing.T, path string) {
	t.Helper()

	files := token.NewFileSet()

	file, err := parser.ParseFile(files, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Errorf("%s: %v", path, err)
		return
	}

	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok || !isRenderStageHooks(literal.Type) {
			return true
		}

		if !hasField(literal, "Package") {
			t.Errorf("%s: RenderStageHooks without Package", files.Position(literal.Pos()))
		}

		return true
	})
}

func isRenderStageHooks(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name == "RenderStageHooks"
	case *ast.SelectorExpr:
		return typed.Sel.Name == "RenderStageHooks"
	}

	return false
}

func hasField(literal *ast.CompositeLit, name string) bool {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := pair.Key.(*ast.Ident)
		if ok && key.Name == name {
			return true
		}
	}

	return false
}
