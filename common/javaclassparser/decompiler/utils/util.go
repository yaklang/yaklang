package utils

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

func NodesToStatements(nodes []*core.Node) []core.Statement {
	var result []core.Statement
	for _, item := range nodes {
		result = append(result, item.Statement)
	}
	return result
}
func ShowStatementNodes(nodes []*core.Node) {
	funcCtx := &core.FunctionContext{}
	for _, item := range nodes {
		fmt.Printf("%d %s\n", item.Id, item.Statement.String(funcCtx))
	}
}
func InsertBetweenNodes(src, target *core.Node, newNode *core.Node) {
	var ok bool
	for i, n := range src.Next {
		if n == target {
			src.Next[i] = newNode
			newNode.Source = append(newNode.Source, src)
			ok = true
			break
		}
	}
	if !ok {
		src.Next = append(src.Next, newNode)
		newNode.Source = append(newNode.Source, src)
	}
	ok = false
	for i, n := range target.Source {
		if n == src {
			target.Source[i] = newNode
			newNode.AddNext(target)
			ok = true
			break
		}
	}
	if !ok {
		src.Source = append(src.Source, newNode)
		newNode.AddNext(target)
	}
}
func CutNode(src, target *core.Node) func() {
	for i, item := range src.Next {
		if item == target {
			src.Next = append(src.Next[:i], src.Next[i+1:]...)
			break
		}
	}
	for i, item := range target.Source {
		if item == src {
			target.Source = append(target.Source[:i], target.Source[i+1:]...)
			break
		}
	}
	return func() {
		src.Next = append(src.Next, target)
		target.Source = append(target.Source, src)
	}
}
func LinkNode(src, target *core.Node) {
	target.Source = append(target.Source, src)
	src.Next = append(src.Next, target)
}
