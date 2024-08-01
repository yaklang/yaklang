package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"strings"
)

func (y *builder) VisitTypeHint(raw phpparser.ITypeHintContext) ssa.Type {
	if y == nil || raw == nil {
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
		return y.GetClassBluePrint(className)
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

func (y *builder) VisitTypeRef(raw phpparser.ITypeRefContext) (*ssa.ClassBluePrint, string) {
	if y == nil || raw == nil {
		log.Errorf("[BUG]: TypeRef is nil")
		return y.CreateClassBluePrint(raw.GetText()), raw.GetText()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TypeRefContext)
	if i == nil {
		return y.CreateClassBluePrint(raw.GetText()), raw.GetText()
	}
	if i.FlexiVariable() != nil {
		//todo: flexivariable
	}
	getLib := func(path string) (*ssa.Program, bool) {
		program := y.GetProgram()
		return program.GetLibrary(path)
	}
	if i.QualifiedNamespaceName() != nil {
		name := y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
		path := strings.Split(name, "\\")
		class := path[len(path)-1]
		lib := strings.Join(path[:len(path)-1], ".")
		if lib == "" {
			return y.GetClassBluePrint(class), class
		}
		program, b := getLib(lib)
		if b {
			if bluePrint := program.GetClassBluePrint(class); bluePrint != nil {
				return bluePrint, class
			}
		}
	} else if i.IndirectTypeRef() != nil {

	} else if i.PrimitiveType() != nil {

	} else if i.Static() != nil {
		y.GetClassBluePrint(i.Static().GetText())
	}
	log.Warnf("[BUG]: fix it")
	return y.CreateClassBluePrint(raw.GetText()), raw.GetText()
}

func (y *builder) VisitPrimitiveType(raw phpparser.IPrimitiveTypeContext) ssa.Type {
	if y == nil || raw == nil {
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
	if y == nil || raw == nil {
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
func (y *builder) VisitQualifiedStaticTypeRef(raw phpparser.IQualifiedStaticTypeRefContext) string {
	if y == nil || raw == nil {
		return ""
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.QualifiedStaticTypeRefContext)
	if i == nil {
		return ""
	}

	if i.Static() != nil {
		return i.Static().GetText()
	} else if i.QualifiedNamespaceName() != nil {
		return y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	}

	return ""
}
