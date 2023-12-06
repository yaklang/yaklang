package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitConstant(raw phpparser.IConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ConstantContext)
	if i == nil {
		return nil
	}

	if i.Null() != nil {

	} else if i.LiteralConstant() != nil {
		return y.VisitLiteralConstant(i.LiteralConstant())
	} else if i.MagicConstant() != nil {

	} else if i.ClassConstant() != nil {

	} else if i.QualifiedNamespaceName() != nil {

	} else {

	}
	return nil
}

func (y *builder) VisitLiteralConstant(raw phpparser.ILiteralConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.LiteralConstantContext)
	if i == nil {
		return nil
	}

	if i.Real() != nil {

	} else if i.BooleanConstant() != nil {

	} else if i.NumericConstant() != nil {

	} else if i.StringConstant() != nil {

	}

	return nil
}

func (y *builder) VisitString_(raw phpparser.IStringContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.StringContext)
	if i == nil {
		return nil
	}

	return nil
}
