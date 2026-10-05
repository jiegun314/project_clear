package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The import rules that keep this code base from growing in the wrong
// direction. They are tests rather than documentation because a rule that
// nothing checks stops being true the first time someone is in a hurry.

const (
	wailsPrefix   = "github.com/wailsapp/wails"
	excelizePath  = "github.com/xuri/excelize/v2"
	mpsImportPath = "project_clear/internal/mps"
)

// scanGoImports walks the given trees and returns import path -> the files that
// import it, for every .go file including tests.
func scanGoImports(t *testing.T, roots ...string) map[string][]string {
	t.Helper()
	found := map[string][]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			// ImportsOnly parses just the import block, so this does not need the
			// file to compile for any particular platform.
			file, perr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if perr != nil {
				t.Fatalf("parse %s: %v", path, perr)
			}
			for _, spec := range file.Imports {
				p, uerr := strconv.Unquote(spec.Path.Value)
				if uerr != nil {
					t.Fatalf("unquote import in %s: %v", path, uerr)
				}
				found[p] = append(found[p], path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return found
}

// Wails is the desktop integration layer, so it belongs in package main only.
// Keeping it out of every other package is what lets the whole test suite run
// with CGO_ENABLED=0 on a machine with no GTK/WebKit installed, and it is the
// rule that stops business logic from quietly acquiring a UI dependency.
func TestOnlyTheAppPackageMayImportWails(t *testing.T) {
	imports := scanGoImports(t, "internal", "tools")
	for path, files := range imports {
		if path == wailsPrefix || strings.HasPrefix(path, wailsPrefix+"/") {
			t.Errorf("%s is imported by %s, but Wails may only be used from package main",
				path, strings.Join(files, ", "))
		}
	}
}

// internal/mps is the worksheet engine: it reads, merges and re-exports MPS
// workbooks and nothing else. It deliberately knows nothing about the database,
// the configuration, the logger or the application services — degradation is
// reported through FileResult.Warnings / ExportStats.Warnings instead of by
// reaching for a logger. That independence is what makes the engine testable on
// its own fixtures and reusable, so it is pinned here.
func TestMPSEngineDependsOnlyOnStdlibAndExcelize(t *testing.T) {
	imports := scanGoImports(t, filepath.Join("internal", "mps"))
	for path, files := range imports {
		if isStdlibImport(path) || path == excelizePath || path == mpsImportPath {
			continue
		}
		t.Errorf("internal/mps imports %q (from %s); it may only use the standard library and excelize",
			path, strings.Join(files, ", "))
	}
}

// isStdlibImport reports whether an import path looks like a standard library
// package. Every non-stdlib path has a dot in its first element (a domain name).
func isStdlibImport(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}
