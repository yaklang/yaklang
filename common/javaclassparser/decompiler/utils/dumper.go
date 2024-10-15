package utils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"strings"
)

func DumpNodesToDotExp(code *core.Node) string {
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
			//return strconv.Quote(node.Statement.String(&FunctionContext{}))
			s := strings.Replace(node.Statement.String(&class_context.FunctionContext{}), "\"", "", -1)
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
	return sb.String()
}

func DumpOpcodesToDotExp(code *core.OpCode) string {
	var visitor func(node *core.OpCode, visited map[*core.OpCode]bool, sb *strings.Builder)
	visitor = func(node *core.OpCode, visited map[*core.OpCode]bool, sb *strings.Builder) {
		if node == nil {
			return
		}
		if visited[node] {
			return
		}
		visited[node] = true
		for _, nextNode := range node.Target {
			sb.WriteString(fmt.Sprintf("  \"%d%s\" -> \"%d%s\";\n", node.Id, node.Instr.Name, nextNode.Id, nextNode.Instr.Name))
			visitor(nextNode, visited, sb)
		}
	}
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	visited := make(map[*core.OpCode]bool)
	visitor(code, visited, &sb)
	sb.WriteString("}\n")
	return sb.String()
}
