package sfvm

type SyntaxFlowVirtualMachine[K comparable, V any] struct {
	vars map[K]V

	frames []SFFrame[K, V]
}

func NewSyntaxFlowVirtualMachine[K comparable, V any]() *SyntaxFlowVirtualMachine[K, V] {
	sfv := &SyntaxFlowVirtualMachine[K, V]{
		vars: make(map[K]V),
	}
	return sfv
}
