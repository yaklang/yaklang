package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/utils"
)

func NodeDeduplication(nodes []*core.Node) []*core.Node {
	nodeSet := utils.NewSet[*core.Node]()
	nodeSet.AddList(nodes)
	return nodeSet.List()
}

func IsEndNode(node *core.Node) bool {
	if v, ok := node.Statement.(*statements.MiddleStatement); ok {
		return v.Flag == "end"
	}
	return false
}
func WalkNodeToList(node *core.Node) []*core.Node {
	var list []*core.Node
	core.WalkGraph[*core.Node](node, func(node *core.Node) ([]*core.Node, error) {
		list = append(list, node)
		return node.Next, nil
	})
	return list
}
