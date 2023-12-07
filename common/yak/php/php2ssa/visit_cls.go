package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"strings"
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

func (y *builder) VisitClassDeclaration(raw phpparser.IClassDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ClassDeclarationContext)
	if i == nil {
		return nil
	}

	// notes #[...] for dec
	if i.Attributes() != nil {

	}

	// access / private?
	if i.Private() != nil {
		// handle priv
		// not right for class, u can save as an abnormal!
	}

	// modifier: final / abstract
	if i.Modifier() != nil {
		// handle modifier
	}

	if i.Partial() != nil {
		// not in PHP, as abnormal
	}

	var objectTemplate string
	var mergedTemplate []string
	if i.ClassEntryType() != nil {
		switch strings.ToLower(i.ClassEntryType().GetText()) {
		case "trait":
			// trait class is not allowed to be inherited / extend / impl
			// as class alias is right as compiler! XD
			fallthrough
		case "class":
			objectTemplate = i.Identifier().GetText()

			if i.Extends() != nil {
				mergedTemplate = append(mergedTemplate, i.QualifiedStaticTypeRef().GetText())
			}

			if i.Implements() != nil {
				for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
					mergedTemplate = append(mergedTemplate, impl.GetText())
				}
			}
		}
	} else {
		// as interface
		objectTemplate = i.Identifier().GetText()
		if i.Extends() != nil {
			for _, impl := range i.InterfaceList().(*phpparser.InterfaceListContext).AllQualifiedStaticTypeRef() {
				mergedTemplate = append(mergedTemplate, impl.GetText())
			}
		}
	}
	_ = objectTemplate
	_ = mergedTemplate

	for _, field := range i.AllClassStatement() {
		y.VisitClassStatement(field)
	}

	//// how to build a template?
	//// y.main is a SSA.Function
	//template := y.main.BuildObjectTemplate(objectTemplate)    // 注册一个对象模版（有构造和析构方法的对象）
	//template.SetDecorationVerbose(...)                        // 记录一下修饰词
	//for _, i := range mergedTemplate {
	//	y.main.FindObjectTemplate(i).MergeTo(template)        // 合并模版（inherit / trait / extend 都一样）
	//}
	//template.BuildField(func() {                              // 编译字段
	//	for _, field := range i.AllClassStatement() {
	//		y.VisitClassStatement(field)
	//	}
	//})
	//template.Finish()                                         // 宣告完成

	return nil
}

func (y *builder) VisitClassStatement(raw phpparser.IClassStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ClassStatementContext)
	if i == nil {
		return nil
	}

	// note: PHP8 #[...] attributes
	if i.Attributes() != nil {
		// handle php8
	}

	var memberDecorationVerbose string
	if i.PropertyModifiers() != nil {
		memberDecorationVerbose = i.PropertyModifiers().GetText()
	}
	_ = memberDecorationVerbose

	if i.TypeHint() != nil {
		// handle type hint

	}

	return nil
}
