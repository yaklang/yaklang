package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Type struct {
	t ssa.Type
}

func TypeCompare(t1, t2 *Type) bool {
	return ssa.TypeCompare(t1.t, t2.t)
}

func NewType(t ssa.Type) *Type {
	return &Type{t: t}
}

func (t *Type) String() string {
	return t.t.String()
}

func (t *Type) IsAny() bool {
	if t == nil || t.t == nil {
		return true
	}
	b, ok := t.t.(*ssa.BasicType)
	if !ok {
		return true
	}
	return b.Kind == ssa.AnyTypeKind
}

func (t *Type) Compare(t2 *Type) bool {
	return TypeCompare(t, t2)
}

func SliceOf(t *Type) *Type {
	return NewType(ssa.NewSliceType(t.t))
}

func MapOf(key, value *Type) *Type {
	return NewType(ssa.NewMapType(key.t, value.t))
}

func FuncOf(name string, args, ret []*Type, isVariadic bool) *Type {
	return NewType(
		ssa.NewFunctionTypeDefine(name,
			lo.Map(args, func(t *Type, _ int) ssa.Type { return t.t }),
			lo.Map(ret, func(t *Type, _ int) ssa.Type { return t.t }),
			isVariadic),
	)
}

func GetBareType(typ *Type) ssa.Type {
	return typ.t
}
