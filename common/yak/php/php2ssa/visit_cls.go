package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitNewExpr(raw phpparser.INewExprContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.NewExprContext)
	if i == nil {
		return nil
	}

	t := y.VisitTypeRef(i.TypeRef())
	_ = t
	if i.Arguments() != nil {
		return nil
	}
	return nil
}

func (y *builder) VisitTypeRef(raw phpparser.ITypeRefContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.TypeRefContext)
	if i == nil {
		return nil
	}

	if i.QualifiedNamespaceName() != nil {
		y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	} else if i.IndirectTypeRef() != nil {

	} else if i.PrimitiveType() != nil {
		return y.VisitPrimitiveType(i.PrimitiveType())
	} else if i.Static() != nil {
		// as class name
	}

	return nil
}

func (y *builder) VisitPrimitiveType(raw phpparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.PrimitiveTypeContext)
	if i == nil {
		return nil
	}

	if i.BoolType() != nil {
		return ssa.GetTypeByStr("bool")
	} else if i.IntType() != nil {
		return ssa.GetTypeByStr("int")
	} else if i.Int64Type() != nil {
		return ssa.GetTypeByStr("int64")
	} else if i.DoubleType() != nil {
		return ssa.GetTypeByStr("float64")
	} else if i.StringType() != nil {
		return ssa.GetTypeByStr("string")
	} else if i.Resource() != nil {
		return ssa.GetTypeByStr("any")
	} else if i.ObjectType() != nil {
		return ssa.GetTypeByStr("any")
	} else if i.Array() != nil {
		return ssa.NewMapType(ssa.GetTypeByStr("any"), ssa.GetTypeByStr("any"))
	}
	return ssa.GetTypeByStr("any")
}
