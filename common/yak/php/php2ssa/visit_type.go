package php2ssa

import (
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTypeHint(raw phpparser.ITypeHintContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return ssa.GetAnyType()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TypeHintContext)
	if i == nil {
		return ssa.GetAnyType()
	}
	if r := i.QualifiedStaticTypeRef(); r != nil {
		//这里类型就行修复
		className := y.VisitQualifiedStaticTypeRef(r)
		return className
	} else if i.Callable() != nil {
		_ = i.Callable().GetText()
	} else if i.PrimitiveType() != nil {
		return y.VisitPrimitiveType(i.PrimitiveType())
	} else if i.Pipe() != nil {
		//types := lo.Map(i.AllTypeHint(), func(item phpparser.ITypeHintContext, index int) ssa.Type {
		//	return y.VisitTypeHint(i)
		//})
		//_ = types
		// need a
		// return ssa.NewUnionType(types)
	}
	return ssa.GetAnyType()
}

func (y *builder) VisitTypeRef(raw phpparser.ITypeRefContext) (*ssa.BluePrint, string) {
	if y == nil || raw == nil || y.IsStop() {
		log.Errorf("[BUG]: TypeRef is nil")
		return y.CreateBluePrint(raw.GetText()), raw.GetText()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TypeRefContext)
	if i == nil {
		return y.CreateBluePrint(raw.GetText()), raw.GetText()
	}
	if i.FlexiVariable() != nil {
		//todo: flexivariable
	}
	if i.QualifiedNamespaceName() != nil {
		if bluePrint := y.GetBluePrint(strings.TrimSpace(i.QualifiedNamespaceName().GetText())); bluePrint != nil {
			return bluePrint, i.QualifiedNamespaceName().GetText()
		}
		name, s := y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
		//namespace := y.GetProgram().CurrentNameSpace
		if library, _ := y.GetProgram().GetApplication().GetLibrary(strings.Join(name, ".")); !utils.IsNil(library) {
			if bluePrint := library.GetBluePrint(s); !utils.IsNil(bluePrint) {
				return bluePrint, s
			} else {
				log.Errorf("not found this class: %s in namespace", i.QualifiedNamespaceName().GetText())
			}
		} else {
			log.Errorf("not found this class: %s", i.QualifiedNamespaceName().GetText())
		}

	} else if i.IndirectTypeRef() != nil {

	} else if i.PrimitiveType() != nil {

	} else if i.Static() != nil {
		y.GetBluePrint(i.Static().GetText())
	}
	log.Warnf("[BUG]: fix it")
	return y.CreateBluePrint(raw.GetText()), raw.GetText()
}

func (y *builder) VisitPrimitiveType(raw phpparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

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

func (y *builder) VisitCastOperation(raw phpparser.ICastOperationContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

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
func (y *builder) VisitQualifiedStaticTypeRef(raw phpparser.IQualifiedStaticTypeRefContext) *ssa.BluePrint {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.QualifiedStaticTypeRefContext)
	if i == nil {
		return nil
	}
	if i.QualifiedNamespaceName() != nil {
		path, name := y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
		if library, _ := y.GetProgram().GetLibrary(strings.Join(path, ".")); !utils.IsNil(library) {
			if cls := library.GetBluePrint(name); cls != nil {
				return cls
			}
		} else {
			if bluePrint := y.GetProgram().GetBluePrint(name); !utils.IsNil(bluePrint) {
				return bluePrint
			}
		}
	}
	log.Warnf("classBlue print not found")
	return y.CreateBluePrint(uuid.NewString())
}
