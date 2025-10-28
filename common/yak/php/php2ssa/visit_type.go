//go:build !no_language
// +build !no_language

package php2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTypeHint(raw phpparser.ITypeHintContext) ssa.Type {
	if y == nil || raw == nil || y.IsStop() {
		return ssa.CreateAnyType()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TypeHintContext)
	if i == nil {
		return ssa.CreateAnyType()
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
	return ssa.CreateAnyType()
}

func (y *builder) VisitTypeRef(raw phpparser.ITypeRefContext) (*ssa.Blueprint, string) {
	if y == nil || raw == nil || y.IsStop() {
		log.Errorf("[BUG]: TypeRef is nil")
		return y.CreateBlueprint(raw.GetText()), raw.GetText()
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.TypeRefContext)
	if i == nil {
		return y.CreateBlueprint(raw.GetText()), raw.GetText()
	}
	if i.FlexiVariable() != nil {
		//todo: flexivariable
	}
	if i.QualifiedNamespaceName() != nil {
		if bluePrint := y.GetBluePrint(strings.TrimSpace(i.QualifiedNamespaceName().GetText())); bluePrint != nil {
			return bluePrint, i.QualifiedNamespaceName().GetText()
		}
		name, s := y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
		if len(name) == 1 && name[0] == s {
			name = []string{""}
		}
		if library, _ := y.GetProgram().GetApplication().GetOrCreateLibrary(strings.Join(name, ".")); !utils.IsNil(library) {
			if bluePrint := library.GetBluePrint(s); !utils.IsNil(bluePrint) {
				return bluePrint, s
			} else {
				return y.FakeGetBlueprint(library, s), s
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
	return y.CreateBlueprint(raw.GetText()), raw.GetText()
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
		return ssa.CreateAnyType()
	} else if i.ObjectType() != nil {
		return ssa.CreateAnyType()
	} else if i.Array() != nil {
		return ssa.NewMapType(ssa.CreateAnyType(), ssa.CreateAnyType())
	}
	return ssa.CreateAnyType()
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
		return ssa.CreateBooleanType()
	case i.Int8Cast() != nil, i.IntType() != nil, i.Int16Cast() != nil, i.UintCast() != nil, i.DoubleCast() != nil, i.DoubleType() != nil, i.FloatCast() != nil:
		return ssa.CreateNumberType()
	case i.StringType() != nil:
		return ssa.CreateStringType()
	case i.BinaryCast() != nil:
		return ssa.CreateBytesType()
	case i.UnicodeCast() != nil:
		return ssa.CreateStringType()
	case i.Array() != nil:
		return ssa.NewMapType(ssa.CreateAnyType(), ssa.CreateAnyType())
	case i.ObjectType() != nil:
		return ssa.CreateAnyType()
	case i.Unset() != nil:
		return ssa.CreateNullType()
	default:
		return ssa.CreateAnyType()
	}
	return nil
}
func (y *builder) VisitQualifiedStaticTypeRef(raw phpparser.IQualifiedStaticTypeRefContext) *ssa.Blueprint {
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
	log.Warnf("classBlue print not found: %s", raw.GetText())
	return y.CreateBlueprint(yakunquote.TryUnquote(raw.GetText()), raw)
}
