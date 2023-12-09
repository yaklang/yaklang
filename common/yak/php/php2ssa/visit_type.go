package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitCastOperation(raw phpparser.ICastOperationContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.CastOperationContext)
	if i == nil {
		return nil
	}

	switch {
	case i.BoolType() != nil:
		return ssa.GetBooleanType()
	case i.Int8Cast() != nil, i.IntType() != nil, i.Int16Cast() != nil, i.UintCast() != nil, i.DoubleCast() != nil, i.DoubleType() != nil, i.FloatCast() != nil:
		return ssa.GetNumberType()
	case i.StringType() != nil:
		return ssa.GetStringType()
	case i.BinaryCast() != nil:
		return ssa.GetBytesType()
	case i.UnicodeCast() != nil:
		return ssa.GetStringType()
	case i.Array() != nil:
		return ssa.NewMapType(ssa.GetAnyType(), ssa.GetAnyType())
	case i.ObjectType() != nil:
		return ssa.GetAnyType()
	case i.Unset() != nil:
		return ssa.GetNullType()
	default:
		return ssa.GetAnyType()
	}

	return nil
}
