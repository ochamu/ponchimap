package render_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// The model and the display rules must not depend on the drawing engine, so
// that the engine can be replaced behind the Renderer boundary.
func TestOnlyTheAdapterImportsTheDrawingEngine(t *testing.T) {
	root := filepath.Join("..", "..")
	adapter := filepath.Join("internal", "render", "d2render")
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		files++
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for _, imp := range parsed.Imports {
			if strings.Contains(imp.Path.Value, "github.com/d2lang/") && filepath.Dir(rel) != adapter {
				t.Errorf("%s imports %s; only %s may", rel, imp.Path.Value, adapter)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("found no Go files to inspect")
	}
}
