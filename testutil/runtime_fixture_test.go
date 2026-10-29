package testutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/pkg/store/file"
)

// Validate the actual legacy preparation corpus, which build tags otherwise
// hide from ordinary tests. Do not duplicate or weaken its policy assertions.
func TestRuntimePreparationSPDL(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "../pkg/svcs/adsrest/check_prepare_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	fragments := map[string][]string{}
	ast.Inspect(source, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		name, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		list, ok := assignment.Rhs[0].(*ast.CompositeLit)
		if !ok {
			return true
		}
		var lines []string
		for _, element := range list.Elts {
			literal, ok := element.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			line, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, line)
		}
		fragments[name.Name] = lines
		return true
	})
	checked := 0
	ast.Inspect(source, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "CreateServiceWithPDL" {
			return true
		}
		nameLiteral, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			t.Fatal("unexpected service name expression")
		}
		fragment, ok := call.Args[1].(*ast.Ident)
		if !ok {
			t.Fatal("unexpected fixture expression")
		}
		name, _ := strconv.Unquote(nameLiteral.Value)
		lines, exists := fragments[fragment.Name]
		if !exists {
			t.Fatalf("missing corpus %s", fragment.Name)
		}
		_, problems := file.CheckSPDL(strings.NewReader(strings.Join(serviceSPDL(name, lines), "\n")), name+".spdl")
		for _, problem := range problems {
			t.Error(problem)
		}
		checked++
		return true
	})
	if checked == 0 {
		t.Fatal("no legacy runtime fixtures checked")
	}
	t.Logf("validated %d legacy runtime SPDL fixtures", checked)
}
