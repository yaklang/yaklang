package sfvm

import "github.com/yaklang/yaklang/common/utils/omap"

type SFVMOpCode int

const (
	OpPass SFVMOpCode = iota
	OpPush
)

type SFI[T comparable, V any] struct {
	OpCode SFVMOpCode
	Unary  int
	Values []*omap.OrderedMap[T, V]
}

func (s *SFI[T, V]) Show() {
}
