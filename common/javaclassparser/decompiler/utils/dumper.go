package utils

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
)

func DumpNodesToDotExp(code *core.Node) string {
	return ""
	var visitor func(node *core.Node, visited map[*core.Node]bool, sb *strings.Builder)
	visitor = func(node *core.Node, visited map[*core.Node]bool, sb *strings.Builder) {
		if node == nil {
			return
		}
		if visited[node] {
			return
		}
		visited[node] = true
		toString := func(node *core.Node) string {
			//return strconv.Quote(node.Statement.String(&ClassContext{}))
			s := strings.Replace(node.Statement.String(&class_context.ClassContext{}), "\"", "", -1)
			s = strings.Replace(s, "\n", " ", -1)
			return s
		}
		for _, nextNode := range node.Next {
			sb.WriteString(fmt.Sprintf("  \"%d%s\" -> \"%d%s\";\n", node.Id, toString(node), nextNode.Id, toString(nextNode)))
			visitor(nextNode, visited, sb)
		}
	}
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	visited := make(map[*core.Node]bool)
	visitor(code, visited, &sb)
	sb.WriteString("}\n")
	println(sb.String())
	return sb.String()
}
