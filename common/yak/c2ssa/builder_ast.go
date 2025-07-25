package c2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	cparser "github.com/yaklang/yaklang/common/yak/antlr4c/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type ConstKind string

const (
	VARIABLE_KIND ConstKind = "variable"
	NORMAL_KIND   ConstKind = "normal"
	PARAM_KIND    ConstKind = "param"
	FUNC_KIND     ConstKind = "func"
)

func (b *astbuilder) build(ast *cparser.CompilationUnitContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	exportHandler := func() {
		lib := b.GetProgram()
		for structName, structType := range b.GetStructAll() {
			lib.SetExportType(structName, structType)
		}
		for aliasName, aliasType := range b.GetAliasAll() {
			lib.SetExportType(aliasName, aliasType)
		}
		for globalName, globalValue := range b.GetGlobalVariables() {
			lib.SetExportValue(globalName, globalValue)
		}
	}

	if b.PreHandler() {
		exportHandler()
	} else {
		if unit := ast.TranslationUnit(); unit != nil {
			b.BuildTranslationUnit(unit.(*cparser.TranslationUnitContext))
		}
	}
}

func (b *astbuilder) BuildTranslationUnit(ast *cparser.TranslationUnitContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, e := range ast.AllExternalDeclaration() {
		b.BuildExternalDeclaration(e.(*cparser.ExternalDeclarationContext))
	}
}

func (b *astbuilder) BuildExternalDeclaration(ast *cparser.ExternalDeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if f := ast.FunctionDefinition(); f != nil {
		b.BuildFunctionDefinition(f.(*cparser.FunctionDefinitionContext))
	} else if d := ast.Declaration(); d != nil {
		b.BuildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) BuildFunctionDefinition(ast *cparser.FunctionDefinitionContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var retType ssa.Type
	var paramTypes ssa.Types

	if de := ast.Declarator(); de != nil {
		var base ssa.Value
		_, base, _ = b.BuildDeclarator(de.(*cparser.DeclaratorContext), FUNC_KIND)
		if newFunc, ok := ssa.ToFunction(base); ok {
			funcName := newFunc.GetName()

			hitDefinedFunction := false
			MarkedFunctionType := b.GetMarkedFunction()
			handleFunctionType := func(fun *ssa.Function) {
				fun.ParamLength = len(fun.Params)
				fun.SetType(ssa.NewFunctionType("", paramTypes, retType, false))
				fun.Type.IsMethod = false
				if MarkedFunctionType == nil {
					return
				}
				if len(fun.Params) != len(MarkedFunctionType.Parameter) {
					return
				}

				for i, p := range fun.Params {
					val, ok := fun.GetValueById(p)
					if !ok {
						continue
					}
					val.SetType(MarkedFunctionType.Parameter[i])
				}
				hitDefinedFunction = true
			}

			if funcName != "" {
				variable := b.CreateLocalVariable(funcName)
				b.AssignVariable(variable, newFunc)
			}

			store := b.StoreFunctionBuilder()
			log.Infof("add function funcName = %s", funcName)
			newFunc.AddLazyBuilder(func() {
				log.Infof("build function funcName = %s", funcName)
				switchHandler := b.SwitchFunctionBuilder(store)
				defer func() {
					switchHandler()
					if tph := b.tpHandler[newFunc.GetName()]; tph != nil {
						tph()
						delete(b.tpHandler, newFunc.GetName())
					}
				}()
				b.FunctionBuilder = b.PushFunction(newFunc)
				b.SupportClosure = false

				if ds := ast.DeclarationSpecifier(); ds != nil {
					retType = b.BuildDeclarationSpecifier(ds.(*cparser.DeclarationSpecifierContext))
				}
				_, _, paramTypes = b.BuildDeclarator(de.(*cparser.DeclaratorContext), PARAM_KIND)

				handleFunctionType(b.Function)

				if hitDefinedFunction {
					b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
				}

				if dl := ast.DeclarationList(); dl != nil {
					b.BuildDeclarationList(dl.(*cparser.DeclarationListContext))
				}
				if c := ast.CompoundStatement(); c != nil {
					b.BuildCompoundStatement(c.(*cparser.CompoundStatementContext))
				}

				b.Finish()
				b.FunctionBuilder = b.PopFunction()

			}, false)
		}
	}
}

func (b *astbuilder) BuildDirectDeclarator(ast *cparser.DirectDeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var base ssa.Value
	var ssatypes ssa.Types

	kind := NORMAL_KIND
	if len(kinds) > 0 {
		kind = kinds[0]
	}

	// directDeclarator: Identifier
	if id := ast.Identifier(); id != nil {
		switch kind {
		case VARIABLE_KIND:
			return b.CreateLocalVariable(id.GetText()), nil, nil
		case NORMAL_KIND:
			return nil, b.EmitConstInst(id.GetText()), nil
		case PARAM_KIND:
			return nil, b.NewParam(id.GetText()), nil
		case FUNC_KIND:
			return nil, b.NewFunc(id.GetText()), nil
		}
	}

	// directDeclarator: '(' declarator ')'
	if d := ast.Declarator(); d != nil {
		return b.BuildDeclarator(d.(*cparser.DeclaratorContext))
	}

	// directDeclarator: directDeclarator '[' ... ']'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftBracket() != nil && ast.RightBracket() != nil {
		// 处理数组声明
		// 1. directDeclarator '[' typeQualifierList? assignmentExpression? ']'
		// 2. directDeclarator '[' 'static' typeQualifierList? assignmentExpression ']'
		// 3. directDeclarator '[' typeQualifierList 'static' assignmentExpression ']'
		// 4. directDeclarator '[' typeQualifierList? '*' ']'
		// 这里只做结构递归，具体类型推断和 SSA 类型生成可后续完善
		_, base, _ = b.BuildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		// 可递归处理 typeQualifierList/assignmentExpression
		return nil, base, nil
	}

	// directDeclarator: directDeclarator '(' parameterTypeList ')'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftParen() != nil && ast.ParameterTypeList() != nil {
		switch kind {
		case FUNC_KIND:
			_, base, _ = b.BuildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		case PARAM_KIND:
			_, ssatypes = b.BuildParameterTypeList(ast.ParameterTypeList().(*cparser.ParameterTypeListContext))
		}

		return nil, base, ssatypes
	}

	// directDeclarator: directDeclarator '(' identifierList? ')'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftParen() != nil && ast.RightParen() != nil {
		_, base, _ = b.BuildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		if idl := ast.IdentifierList(); idl != nil {
			b.BuildIdentifierList(idl.(*cparser.IdentifierListContext))
		}
		return nil, base, nil
	}

	// directDeclarator: Identifier ':' DigitSequence
	if id := ast.Identifier(); id != nil && ast.DigitSequence() != nil {
		return nil, b.EmitConstInst("bitfield"), nil
	}

	// directDeclarator: vcSpecificModifer Identifier
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Identifier() != nil {
		b.BuildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		return nil, b.EmitConstInst("vcSpecific"), nil
	}

	// directDeclarator: '(' vcSpecificModifer declarator ')'
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Declarator() != nil {
		b.BuildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		return b.BuildDeclarator(ast.Declarator().(*cparser.DeclaratorContext))
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return nil, b.EmitConstInst(0), nil
}

func (b *astbuilder) BuildDeclarator(ast *cparser.DeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DirectDeclarator(); d != nil {
		return b.BuildDirectDeclarator(d.(*cparser.DirectDeclaratorContext), kinds...)
	}

	if p := ast.Pointer(); p != nil {
		_ = p
	}
	for _, g := range ast.AllGccDeclaratorExtension() {
		b.BuildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
	}
	return b.CreateVariable(""), b.EmitConstInst(0), nil
}

func (b *astbuilder) BuildVcSpecificModifer(ast *cparser.VcSpecificModiferContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return b.EmitConstInst(0)
}

func (b *astbuilder) BuildTypeQualifierList(ast *cparser.TypeQualifierListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) BuildIdentifierList(ast *cparser.IdentifierListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) BuildParameterTypeList(ast *cparser.ParameterTypeListContext) (ssa.Values, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if p := ast.ParameterList(); p != nil {
		return b.BuildParameterList(p.(*cparser.ParameterListContext))
	}
	return ssa.Values{}, ssa.Types{}
}

func (b *astbuilder) BuildParameterList(ast *cparser.ParameterListContext) (ssa.Values, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var params ssa.Values
	var ssatypes ssa.Types
	for _, p := range ast.AllParameterDeclaration() {
		param, ssatype := b.BuildParameterDeclaration(p.(*cparser.ParameterDeclarationContext))
		params = append(params, param)
		ssatypes = append(ssatypes, ssatype)
	}
	return params, ssatypes
}

func (b *astbuilder) BuildParameterDeclaration(ast *cparser.ParameterDeclarationContext) (ssa.Value, ssa.Type) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifier(); d != nil {
		ssatyp := b.BuildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		_, param, _ := b.BuildDeclarator(ast.Declarator().(*cparser.DeclaratorContext), PARAM_KIND)
		if ssatyp != nil {
			param.SetType(ssatyp)
		}
		return param, ssatyp
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0), ssa.CreateAnyType()
}

func (b *astbuilder) BuildGccDeclaratorExtension(ast *cparser.GccDeclaratorExtensionContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) CompoundStatement(ast *cparser.CompilationUnitContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) DeclarationList(ast *cparser.DeclarationListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) BuildDeclaration(ast *cparser.DeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifier(); d != nil {
		ssatype := b.BuildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		if init := ast.InitDeclaratorList(); init != nil {
			lefts := b.BuildInitDeclaratorList(init.(*cparser.InitDeclaratorListContext))
			for _, l := range lefts {
				if ssatype == nil {
					break
				}
				if ssatype.String() != l.GetValue().GetType().String() {
					b.NewError(ssa.Error, TAG, TypeMismatch(ssatype.String(), l.GetValue().GetType().String()))
					break
				}
			}
		}
	} else if s := ast.StaticAssertDeclaration(); s != nil {
		b.BuildStaticAssertDeclaration(s.(*cparser.StaticAssertDeclarationContext))
	}
}

func (b *astbuilder) BuildStaticAssertDeclaration(ast *cparser.StaticAssertDeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if c := ast.Expression(); c != nil {
		right := b.BuildExpression(c.(*cparser.ExpressionContext))
		_ = right
	}
}

func (b *astbuilder) BuildInitDeclaratorList(ast *cparser.InitDeclaratorListContext) []*ssa.Variable {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ret []*ssa.Variable
	for _, i := range ast.AllInitDeclarator() {
		ret = append(ret, b.BuildInitDeclarator(i.(*cparser.InitDeclaratorContext)))
	}
	return ret
}

func (b *astbuilder) BuildInitDeclarator(ast *cparser.InitDeclaratorContext) *ssa.Variable {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.Declarator(); d != nil {
		left, _, _ := b.BuildDeclarator(d.(*cparser.DeclaratorContext), VARIABLE_KIND)
		if e := ast.Expression(); e != nil {
			right := b.BuildExpression(e.(*cparser.ExpressionContext))
			b.AssignVariable(left, right)
		}
		return left
	}
	return nil
}

func (b *astbuilder) BuildDeclarationSpecifiers(ast *cparser.DeclarationSpecifiersContext) ssa.Types {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var rets ssa.Types

	for _, d := range ast.AllDeclarationSpecifier() {
		rets = append(rets, b.BuildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext)))
	}
	return rets
}

func (b *astbuilder) BuildDeclarationSpecifier(ast *cparser.DeclarationSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Type

	if s := ast.StorageClassSpecifier(); s != nil {
		ret = b.BuildStorageClassSpecifier(s.(*cparser.StorageClassSpecifierContext))
	} else if ts := ast.TypeSpecifier(); ts != nil {
		ret = b.BuildTypeSpecifier(ts.(*cparser.TypeSpecifierContext))
		// if tq := ast.TypeQualifier(); tq != nil {
		// 	ret = b.BuildTypeQualifier(tq.(*cparser.TypeQualifierContext))
		// }
	} else if f := ast.FunctionSpecifier(); f != nil {
		ret = b.BuildFunctionSpecifier(f.(*cparser.FunctionSpecifierContext))
	} else if a := ast.AlignmentSpecifier(); a != nil {
		ret = b.BuildAlignmentSpecifier(a.(*cparser.AlignmentSpecifierContext))
	}
	if ret == nil {
		ret = ssa.CreateAnyType()
	}
	return ret
}

func (b *astbuilder) BuildAlignmentSpecifier(ast *cparser.AlignmentSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if t := ast.TypeName(); t != nil {
		return b.BuildTypeName(t.(*cparser.TypeNameContext))
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) BuildTypeName(ast *cparser.TypeNameContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) BuildFunctionSpecifier(ast *cparser.FunctionSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	return ssa.CreateAnyType()
}

func (b *astbuilder) BuildTypeQualifier(ast *cparser.TypeQualifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) BuildStorageClassSpecifier(ast *cparser.StorageClassSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) BuildTypeSpecifier(ast *cparser.TypeSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	name := ast.GetText()
	if ssatyp := ssa.GetTypeByStr(name); ssatyp != nil {
		return ssatyp
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) BuildDeclarationList(ast *cparser.DeclarationListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, d := range ast.AllDeclaration() {
		b.BuildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) BuildCompoundStatement(ast *cparser.CompoundStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if block := ast.BlockItemList(); block != nil {
		b.BuildBlockItemList(block.(*cparser.BlockItemListContext))
	}
}

func (b *astbuilder) BuildBlockItemList(ast *cparser.BlockItemListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, item := range ast.AllBlockItem() {
		b.BuildBlockItem(item.(*cparser.BlockItemContext))
	}
}

func (b *astbuilder) BuildBlockItem(ast *cparser.BlockItemContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if s := ast.Statement(); s != nil {
		b.BuildStatement(s.(*cparser.StatementContext))
	} else if d := ast.Declaration(); d != nil {
		b.BuildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) BuildStatement(ast *cparser.StatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.ExpressionStatement(); e != nil {
		b.BuildExpressionStatement(e.(*cparser.ExpressionStatementContext))
	} else if j := ast.JumpStatement(); j != nil {
		b.BuildJumpStatement(j.(*cparser.JumpStatementContext))
	} else if c := ast.CompoundStatement(); c != nil {
		b.BuildCompoundStatement(c.(*cparser.CompoundStatementContext))
	} else if s := ast.SelectionStatement(); s != nil {
		b.BuildSelectionStatement(s.(*cparser.SelectionStatementContext))
	} else if i := ast.IterationStatement(); i != nil {
		b.BuildIterationStatement(i.(*cparser.IterationStatementContext))
	} else if l := ast.LabeledStatement(); l != nil {
		b.BuildLabeledStatement(l.(*cparser.LabeledStatementContext))
	} else if a := ast.AsmStatement(); a != nil {
		b.BuildAsmStatement(a.(*cparser.AsmStatementContext))
	}
}

func (b *astbuilder) BuildAsmStatement(ast *cparser.AsmStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) BuildLabeledStatement(ast *cparser.LabeledStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) BuildIterationStatement(ast *cparser.IterationStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.Expression(); e != nil {
		b.BuildExpression(e.(*cparser.ExpressionContext))
	}
}

func (b *astbuilder) BuildJumpStatement(ast *cparser.JumpStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.Expression(); e != nil {
		right := b.BuildExpression(e.(*cparser.ExpressionContext))
		b.EmitReturn(ssa.Values{right})
	}
}

func (b *astbuilder) BuildSelectionStatement(ast *cparser.SelectionStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) BuildExpressionStatement(ast *cparser.ExpressionStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, a := range ast.AllAssignmentExpression() {
		b.BuildAssignmentExpression(a.(*cparser.AssignmentExpressionContext))
	}
}
