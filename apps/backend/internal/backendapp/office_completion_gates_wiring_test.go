package backendapp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestCompletionGatesStartAfterConstruction guards that the completion-gate
// service is started in the same function that builds it. Starting it from an
// earlier startup phase reads a nil service and silently never verifies.
func TestCompletionGatesStartAfterConstruction(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	var construct *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "constructOfficeServices" {
			construct = fn
		}
	}
	if construct == nil {
		t.Fatal("constructOfficeServices not found in main.go")
	}
	wiredAt, startedAt := -1, -1
	index := 0
	ast.Inspect(construct, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		index++
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "wireOfficeSvcsDependencies" {
			wiredAt = index
		}
		if isSelectorCall(call, "Start") && wiredAt >= 0 && startedAt < 0 {
			startedAt = index
		}
		return true
	})
	if wiredAt < 0 || startedAt < 0 {
		t.Fatalf("constructOfficeServices must call wireOfficeSvcsDependencies and then start the completion gates (wired=%d started=%d)", wiredAt, startedAt)
	}
}
