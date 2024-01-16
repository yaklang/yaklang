package ssaapi

import "github.com/yaklang/yaklang/common/utils/omap"

type DominatorForest struct {
	Trees []*DominatorTree
}

type DominatorTree *omap.OrderedMap[int, *Value]
