package sfvm

import "github.com/yaklang/yaklang/common/utils/omap"

type SyntaxFlowVirtualMachine[K comparable, V any] struct {
	vars *omap.OrderedMap[K, V]

	frames []SFFrame[K, V]
}

func NewSyntaxFlowVirtualMachine[K comparable, V any]() *SyntaxFlowVirtualMachine[K, V] {
	sfv := &SyntaxFlowVirtualMachine[K, V]{
		vars: omap.NewEmptyOrderedMap[K, V](),
	}
	return sfv
}
