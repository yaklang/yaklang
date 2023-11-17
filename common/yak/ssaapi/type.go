package ssaapi

import "github.com/yaklang/yaklang/common/yak/ssa"

type Type struct {
	t ssa.Type
}

func TypeCompare(t1, t2 *Type) bool {
	return ssa.TypeCompare(t1.t, t2.t)
}

func newType(t ssa.Type) *Type {
	return &Type{t: t}
}

func (t *Type) String() string {
	return t.t.String()
}

func (t *Type) Compare(t2 *Type) bool {
	return TypeCompare(t, t2)
}

func (t *Type) GetKind() ssa.TypeKind {
	return t.t.GetTypeKind()
}
