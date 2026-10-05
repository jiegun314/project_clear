package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// A panic inside a bound method is recovered by Wails, which then calls the
// webview back with an empty string: that is not valid JSON, so the promise
// never settles, the window stays in its processing state and nothing is
// reported. Every method reachable from the frontend therefore has to convert a
// panic into an ordinary error itself.
func TestRecoverFaultConvertsAPanicIntoAnError(t *testing.T) {
	a := NewApp()

	var err error
	func() {
		defer a.recoverFault("Probe", &err)
		panic("boom")
	}()

	if err == nil {
		t.Fatal("a panic left no error behind: the UI would wait forever")
	}
	for _, want := range []string{"Probe", "boom", "内部错误"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
}

// The guard must be a no-op on the normal path, or every call would report a
// failure.
func TestRecoverFaultLeavesTheErrorUntouched(t *testing.T) {
	a := NewApp()

	var err error
	func() {
		defer a.recoverFault("Probe", &err)
	}()

	if err != nil {
		t.Errorf("recoverFault invented an error: %v", err)
	}
}

// A void binding cannot report an error, but it still must not take the process
// down with an unrecovered panic.
func TestRecoverFaultAcceptsANilErrorTarget(t *testing.T) {
	a := NewApp()

	func() {
		defer a.recoverFault("Probe", nil)
		panic("boom")
	}()
}

// Startup can fail before the logger exists, and Wails fires OnDomReady anyway.
func TestDomReadySurvivesAMissingLogger(t *testing.T) {
	a := NewApp()
	if a.log != nil {
		t.Fatal("a fresh App should have no logger yet")
	}
	a.domReady(context.Background())
}

// The guard is applied per method, so it is easy to add a new bound method and
// forget it. This reads the real source and fails if any exported method on
// *App is missing its deferred guard.
func TestEveryBoundMethodGuardsAgainstPanics(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "app.go", nil, 0)
	if err != nil {
		t.Fatalf("parse app.go: %v", err)
	}

	var checked int
	var missing []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || !fn.Name.IsExported() {
			continue
		}
		if !isAppReceiver(fn.Recv) {
			continue
		}
		checked++
		if !callsRecoverFault(fn.Body) {
			missing = append(missing, fn.Name.Name)
		}
	}

	if checked == 0 {
		t.Fatal("found no bound methods: the check silently passed")
	}
	if len(missing) > 0 {
		t.Errorf("%d bound methods have no panic guard: %v", len(missing), missing)
	}
}

func isAppReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "App"
}

func callsRecoverFault(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "recoverFault" {
			found = true
		}
		return true
	})
	return found
}
