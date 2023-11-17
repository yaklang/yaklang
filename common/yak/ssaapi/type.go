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

func newType(t ssa.Type) *Type {
	return &Type{t: t}
}

func (t *Type) String() string {
	return t.t.String()
}

func (t *Type) Compare(t2 *Type) bool {
	return TypeCompare(t, t2)
}

var (
	Number        = newType(ssa.BasicTypes[ssa.Number])
	String        = newType(ssa.BasicTypes[ssa.String])
	Bytes         = newType(ssa.BasicTypes[ssa.Bytes])
	Boolean       = newType(ssa.BasicTypes[ssa.Boolean])
	UndefinedType = newType(ssa.BasicTypes[ssa.UndefinedType])
	Null          = newType(ssa.BasicTypes[ssa.Null])
	Any           = newType(ssa.BasicTypes[ssa.Any])
	ErrorType     = newType(ssa.BasicTypes[ssa.ErrorType])
)

func SliceOf(t *Type) *Type {
	return newType(ssa.NewSliceType(t.t))
}

func MapOf(key, value *Type) *Type {
	return newType(ssa.NewMapType(key.t, value.t))
}

func FuncOf(name string, args, ret []*Type, isVariadic bool) *Type {
	return newType(
		ssa.NewFunctionTypeDefine(name,
			lo.Map(args, func(t *Type, _ int) ssa.Type { return t.t }),
			lo.Map(ret, func(t *Type, _ int) ssa.Type { return t.t }),
			isVariadic),
	)
}
