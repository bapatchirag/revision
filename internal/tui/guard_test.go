package tui_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestFoundationStaysDomainAgnostic is the reusability guard: it asserts that
// no production file under internal/tui (this package and every subpackage,
// including internal/tui/component) imports the SVN domain or the app layer.
// Inner layers must never depend on outer ones.
func TestFoundationStaysDomainAgnostic(t *testing.T) {
	forbidden := []string{
		"github.com/bapatchirag/revision/internal/svn",
		"github.com/bapatchirag/revision/internal/app",
	}

	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if p == bad || strings.HasPrefix(p, bad+"/") {
					t.Errorf("%s imports forbidden package %q (foundation must stay domain-agnostic)", path, p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/tui: %v", err)
	}
}
