package parser

import (
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
)

//go:embed testdata/large.js
var largeJS string

func printAllChildren(node *ast.Node, depth int) {
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "----"
	}

	fmt.Printf("%s %s [%d, %d)\n", indent, node.Kind.String(), node.Pos(), node.End())

	children := []*ast.Node{}
	node.VisitEachChild(ast.NewNodeVisitor(func(node *ast.Node) *ast.Node {
		children = append(children, node)
		return node
	}, nil, ast.NodeVisitorHooks{}))
	for _, child := range children {
		printAllChildren(child, depth+1)
	}
}

// helper 生成唯一ID
var nextID int

func genID() int {
	nextID++
	return nextID
}

type dotNode struct {
	ID   int
	Kind string
	Pos  int
	End  int
}

// AST → DOT 格式
func buildGraphvizAST(node *ast.Node, builder *strings.Builder, parentID int) {
	id := genID()
	current := dotNode{
		ID:   id,
		Kind: node.Kind.String(),
		Pos:  node.Pos(),
		End:  node.End(),
	}

	// 打印当前节点
	fmt.Fprintf(builder, `  node%d [label="%s\n[%d,%d)"];`+"\n", current.ID, current.Kind, current.Pos, current.End)

	// 如果有父节点，画一条边
	if parentID != 0 {
		fmt.Fprintf(builder, "  node%d -> node%d;\n", parentID, current.ID)
	}

	// 递归处理子节点
	node.VisitEachChild(ast.NewNodeVisitor(func(child *ast.Node) *ast.Node {
		buildGraphvizAST(child, builder, current.ID)
		return child
	}, nil, ast.NodeVisitorHooks{}))
}

func ExportASTToDotFile(root *ast.Node, filename string) error {
	var builder strings.Builder
	builder.WriteString("digraph AST {\n")
	builder.WriteString("  node [shape=box, style=filled, color=lightgray];\n")

	buildGraphvizAST(root, &builder, 0)

	builder.WriteString("}\n")

	return os.WriteFile(filename, []byte(builder.String()), 0644)
}

func TestParseFile(t *testing.T) {
	start := time.Now()
	sf := ParseSourceFile("", "", largeJS, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	println(time.Now().Sub(start).String())
	printAllChildren(&sf.Node, 0)
}

func TestParseMalformed_FuncMissingRightParen(t *testing.T) {
	bad := `
function greet( {
  console.log("Hi");
}
`
	sf := ParseSourceFile("", "", bad, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	log.Printf("Error Count: %v\n", len(sf.Diagnostics()))
	assert.Equal(t, len(sf.Diagnostics()), 3)
	for _, diag := range sf.Diagnostics() {
		log.Println(diag.Message())
	}
}

func TestParseMalformed_DoubleComma(t *testing.T) {
	bad := `
const person = { name: "Alice", age: 25,, city: "Paris" };
`
	sf := ParseSourceFile("", "", bad, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	log.Printf("Error Count: %v\n", len(sf.Diagnostics()))
	assert.Equal(t, len(sf.Diagnostics()), 1)
	for _, diag := range sf.Diagnostics() {
		log.Println(diag.Message())
	}
}

func TestArrayBinding(t *testing.T) {
	code := `[a, b] = [1, 2]`
	start := time.Now()
	sf := ParseSourceFile("", "", code, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
	println(time.Now().Sub(start).String())
	printAllChildren(&sf.Node, 0)
}

//func TestParseFileToDot(t *testing.T) {
//	sf := ParseSourceFile("", "", packedJS, core.ScriptTargetES5, scanner.JSDocParsingModeParseNone)
//
//	err := ExportASTToDotFile(&sf.Node, "out.dot")
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	fmt.Println("AST exported to out.dot")
//}
