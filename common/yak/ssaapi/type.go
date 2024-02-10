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

func (t *Type) Compare(t2 *Type) bool {
	return TypeCompare(t, t2)
}

var (
	Number        = NewType(ssa.BasicTypes[ssa.NumberTypeKind])
	String        = NewType(ssa.BasicTypes[ssa.StringTypeKind])
	Bytes         = NewType(ssa.BasicTypes[ssa.BytesTypeKind])
	Boolean       = NewType(ssa.BasicTypes[ssa.BooleanTypeKind])
	UndefinedType = NewType(ssa.BasicTypes[ssa.UndefinedTypeKind])
	Null          = NewType(ssa.BasicTypes[ssa.NullTypeKind])
	Any           = NewType(ssa.BasicTypes[ssa.AnyTypeKind])
	ErrorType     = NewType(ssa.BasicTypes[ssa.ErrorTypeKind])
)

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
