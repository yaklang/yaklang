package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/utils"
)

func NodeDeduplication(nodes []*core.Node) []*core.Node {
	nodeSet := utils.NewSet[*core.Node]()
	nodeSet.AddList(nodes)
	return nodeSet.List()
}
