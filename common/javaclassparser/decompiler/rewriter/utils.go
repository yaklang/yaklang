package rewriter

import (
	"sort"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/utils"
)

// NodeDeduplication removes duplicate nodes while returning them in a deterministic id order.
// The result feeds AddNext in the if/switch rewriters, so it directly fixes the .Next ordering of a
// structured node. utils.Set.List() iterates a Go map and would hand back exit nodes in a random order,
// which then randomizes dominator traversal / merge-node selection for any enclosing structure and makes
// the same method intermittently stub with "multiple next". Sorting by node id keeps it stable.
func NodeDeduplication(nodes []*core.Node) []*core.Node {
	nodeSet := utils.NewSet[*core.Node]()
	nodeSet.AddList(nodes)
	list := nodeSet.List()
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].Id < list[j].Id
	})
	return list
}

func IsEndNode(node *core.Node) bool {
	if node == nil {
		return false
	}
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
