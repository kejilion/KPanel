package panel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Every terminal error event the file transfer stream emits must close the
// audit chain the handler opened with an intent record (§7.2/§12). The
// handler has no lightweight end-to-end harness, so this pins the invariant
// structurally: in each block that writes a State "error" event, a failure
// audit record precedes it.
func TestFileTransferErrorEventsCloseTheAuditChain(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "files.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var handler *ast.FuncDecl
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "handleFileTransfer" {
			handler = function
		}
	}
	if handler == nil {
		t.Fatal("handleFileTransfer not found")
	}
	errorEvents := 0
	ast.Inspect(handler.Body, func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		audited := false
		for _, statement := range block.List {
			if isFailureAudit(statement) {
				audited = true
			}
			if isErrorEvent(statement) {
				errorEvents++
				if !audited {
					t.Errorf("%s: error event without a preceding failure audit", fileSet.Position(statement.Pos()))
				}
			}
		}
		return true
	})
	if errorEvents == 0 {
		t.Fatal("no error events found; the invariant check is not looking at the right code")
	}
}

func isFailureAudit(statement ast.Stmt) bool {
	assign, ok := statement.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "audit" {
		return false
	}
	for _, argument := range call.Args {
		if literal, ok := argument.(*ast.BasicLit); ok && literal.Kind == token.STRING {
			if value, _ := strconv.Unquote(literal.Value); value == "failure" {
				return true
			}
		}
	}
	return false
}

func isErrorEvent(statement ast.Stmt) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	if identifier, ok := call.Fun.(*ast.Ident); !ok || identifier.Name != "writeEvent" {
		return false
	}
	composite, ok := call.Args[0].(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, element := range composite.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, keyOK := pair.Key.(*ast.Ident)
		value, valueOK := pair.Value.(*ast.BasicLit)
		if keyOK && valueOK && key.Name == "State" && value.Value == `"error"` {
			return true
		}
	}
	return false
}
