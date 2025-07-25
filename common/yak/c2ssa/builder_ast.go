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
			b.buildTranslationUnit(unit.(*cparser.TranslationUnitContext))
		}
	}
}

func (b *astbuilder) buildTranslationUnit(ast *cparser.TranslationUnitContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, e := range ast.AllExternalDeclaration() {
		b.buildExternalDeclaration(e.(*cparser.ExternalDeclarationContext))
	}
}

func (b *astbuilder) buildExternalDeclaration(ast *cparser.ExternalDeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if f := ast.FunctionDefinition(); f != nil {
		b.buildFunctionDefinition(f.(*cparser.FunctionDefinitionContext))
	} else if d := ast.Declaration(); d != nil {
		b.buildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) buildFunctionDefinition(ast *cparser.FunctionDefinitionContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var retType ssa.Type
	var paramTypes ssa.Types

	if de := ast.Declarator(); de != nil {
		var base ssa.Value
		_, base, _ = b.buildDeclarator(de.(*cparser.DeclaratorContext), FUNC_KIND)
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
					retType = b.buildDeclarationSpecifier(ds.(*cparser.DeclarationSpecifierContext))
				}
				_, _, paramTypes = b.buildDeclarator(de.(*cparser.DeclaratorContext), PARAM_KIND)

				handleFunctionType(b.Function)

				if hitDefinedFunction {
					b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
				}

				if dl := ast.DeclarationList(); dl != nil {
					b.buildDeclarationList(dl.(*cparser.DeclarationListContext))
				}
				if c := ast.CompoundStatement(); c != nil {
					b.buildCompoundStatement(c.(*cparser.CompoundStatementContext))
				}

				b.Finish()
				b.FunctionBuilder = b.PopFunction()

			}, false)
		}
	}
}

func (b *astbuilder) buildDirectDeclarator(ast *cparser.DirectDeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
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
		return b.buildDeclarator(d.(*cparser.DeclaratorContext))
	}

	// directDeclarator: directDeclarator '[' ... ']'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftBracket() != nil && ast.RightBracket() != nil {
		// 处理数组声明
		// 1. directDeclarator '[' typeQualifierList? assignmentExpression? ']'
		// 2. directDeclarator '[' 'static' typeQualifierList? assignmentExpression ']'
		// 3. directDeclarator '[' typeQualifierList 'static' assignmentExpression ']'
		// 4. directDeclarator '[' typeQualifierList? '*' ']'
		// 这里只做结构递归，具体类型推断和 SSA 类型生成可后续完善
		_, base, _ = b.buildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		// 可递归处理 typeQualifierList/assignmentExpression
		return nil, base, nil
	}

	// directDeclarator: directDeclarator '(' parameterTypeList ')'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftParen() != nil && ast.ParameterTypeList() != nil {
		switch kind {
		case FUNC_KIND:
			_, base, _ = b.buildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		case PARAM_KIND:
			_, ssatypes = b.buildParameterTypeList(ast.ParameterTypeList().(*cparser.ParameterTypeListContext))
		}

		return nil, base, ssatypes
	}

	// directDeclarator: directDeclarator '(' identifierList? ')'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftParen() != nil && ast.RightParen() != nil {
		_, base, _ = b.buildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		if idl := ast.IdentifierList(); idl != nil {
			b.buildIdentifierList(idl.(*cparser.IdentifierListContext))
		}
		return nil, base, nil
	}

	// directDeclarator: Identifier ':' DigitSequence
	if id := ast.Identifier(); id != nil && ast.DigitSequence() != nil {
		return nil, b.EmitConstInst("bitfield"), nil
	}

	// directDeclarator: vcSpecificModifer Identifier
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Identifier() != nil {
		b.buildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		return nil, b.EmitConstInst("vcSpecific"), nil
	}

	// directDeclarator: '(' vcSpecificModifer declarator ')'
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Declarator() != nil {
		b.buildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		return b.buildDeclarator(ast.Declarator().(*cparser.DeclaratorContext))
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return nil, b.EmitConstInst(0), nil
}

func (b *astbuilder) buildDeclarator(ast *cparser.DeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DirectDeclarator(); d != nil {
		return b.buildDirectDeclarator(d.(*cparser.DirectDeclaratorContext), kinds...)
	}

	if p := ast.Pointer(); p != nil {
		_ = p
	}
	for _, g := range ast.AllGccDeclaratorExtension() {
		b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
	}
	return b.CreateVariable(""), b.EmitConstInst(0), nil
}

func (b *astbuilder) buildVcSpecificModifer(ast *cparser.VcSpecificModiferContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return b.EmitConstInst(0)
}

func (b *astbuilder) buildTypeQualifierList(ast *cparser.TypeQualifierListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildIdentifierList(ast *cparser.IdentifierListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildParameterTypeList(ast *cparser.ParameterTypeListContext) (ssa.Values, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if p := ast.ParameterList(); p != nil {
		return b.buildParameterList(p.(*cparser.ParameterListContext))
	}
	return ssa.Values{}, ssa.Types{}
}

func (b *astbuilder) buildParameterList(ast *cparser.ParameterListContext) (ssa.Values, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var params ssa.Values
	var ssatypes ssa.Types
	for _, p := range ast.AllParameterDeclaration() {
		param, ssatype := b.buildParameterDeclaration(p.(*cparser.ParameterDeclarationContext))
		params = append(params, param)
		ssatypes = append(ssatypes, ssatype)
	}
	return params, ssatypes
}

func (b *astbuilder) buildParameterDeclaration(ast *cparser.ParameterDeclarationContext) (ssa.Value, ssa.Type) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifier(); d != nil {
		ssatyp := b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		_, param, _ := b.buildDeclarator(ast.Declarator().(*cparser.DeclaratorContext), PARAM_KIND)
		if ssatyp != nil {
			param.SetType(ssatyp)
		}
		return param, ssatyp
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0), ssa.CreateAnyType()
}

func (b *astbuilder) buildGccDeclaratorExtension(ast *cparser.GccDeclaratorExtensionContext) {
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

func (b *astbuilder) buildDeclaration(ast *cparser.DeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifiers(); d != nil {
		ssatypes := b.buildDeclarationSpecifiers(d.(*cparser.DeclarationSpecifiersContext))
		if init := ast.InitDeclaratorList(); init != nil {
			lefts := b.buildInitDeclaratorList(init.(*cparser.InitDeclaratorListContext))
			for i, l := range lefts {
				if l.GetValue() == nil {
					right := b.GetDefaultValue(ssatypes[i])
					b.AssignVariable(l, right)
				}
				if ssatypes == nil {
					break
				}
				if ssatypes.String() != l.GetValue().GetType().String() {
					b.NewError(ssa.Error, TAG, TypeMismatch(ssatypes.String(), l.GetValue().GetType().String()))
					break
				}
			}
		}
	} else if s := ast.StaticAssertDeclaration(); s != nil {
		b.buildStaticAssertDeclaration(s.(*cparser.StaticAssertDeclarationContext))
	}
}

func (b *astbuilder) buildStaticAssertDeclaration(ast *cparser.StaticAssertDeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if c := ast.Expression(); c != nil {
		right := b.buildExpression(c.(*cparser.ExpressionContext))
		_ = right
	}
}

func (b *astbuilder) buildInitDeclaratorList(ast *cparser.InitDeclaratorListContext) []*ssa.Variable {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ret []*ssa.Variable
	for _, i := range ast.AllInitDeclarator() {
		ret = append(ret, b.buildInitDeclarator(i.(*cparser.InitDeclaratorContext)))
	}
	return ret
}

func (b *astbuilder) buildInitDeclarator(ast *cparser.InitDeclaratorContext) *ssa.Variable {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.Declarator(); d != nil {
		left, _, _ := b.buildDeclarator(d.(*cparser.DeclaratorContext), VARIABLE_KIND)
		if e := ast.Expression(); e != nil {
			right := b.buildExpression(e.(*cparser.ExpressionContext))
			b.AssignVariable(left, right)
		}
		return left
	}
	return nil
}

func (b *astbuilder) buildDeclarationSpecifiers(ast *cparser.DeclarationSpecifiersContext) ssa.Types {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var rets ssa.Types

	for _, d := range ast.AllDeclarationSpecifier() {
		rets = append(rets, b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext)))
	}
	return rets
}

func (b *astbuilder) buildDeclarationSpecifier(ast *cparser.DeclarationSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Type

	if s := ast.StorageClassSpecifier(); s != nil {
		ret = b.buildStorageClassSpecifier(s.(*cparser.StorageClassSpecifierContext))
	} else if ts := ast.TypeSpecifier(); ts != nil {
		ret = b.buildTypeSpecifier(ts.(*cparser.TypeSpecifierContext))
		// if tq := ast.TypeQualifier(); tq != nil {
		// 	ret = b.buildTypeQualifier(tq.(*cparser.TypeQualifierContext))
		// }
	} else if f := ast.FunctionSpecifier(); f != nil {
		ret = b.buildFunctionSpecifier(f.(*cparser.FunctionSpecifierContext))
	} else if a := ast.AlignmentSpecifier(); a != nil {
		ret = b.buildAlignmentSpecifier(a.(*cparser.AlignmentSpecifierContext))
	}
	if ret == nil {
		ret = ssa.CreateAnyType()
	}
	return ret
}

func (b *astbuilder) buildAlignmentSpecifier(ast *cparser.AlignmentSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if t := ast.TypeName(); t != nil {
		return b.buildTypeName(t.(*cparser.TypeNameContext))
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeName(ast *cparser.TypeNameContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildFunctionSpecifier(ast *cparser.FunctionSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeQualifier(ast *cparser.TypeQualifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildStorageClassSpecifier(ast *cparser.StorageClassSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeSpecifier(ast *cparser.TypeSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	name := ast.GetText()
	if ssatyp := ssa.GetTypeByStr(name); ssatyp != nil {
		return ssatyp
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildDeclarationList(ast *cparser.DeclarationListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, d := range ast.AllDeclaration() {
		b.buildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) buildCompoundStatement(ast *cparser.CompoundStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if block := ast.BlockItemList(); block != nil {
		b.buildBlockItemList(block.(*cparser.BlockItemListContext))
	}
}

func (b *astbuilder) buildBlockItemList(ast *cparser.BlockItemListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, item := range ast.AllBlockItem() {
		b.buildBlockItem(item.(*cparser.BlockItemContext))
	}
}

func (b *astbuilder) buildBlockItem(ast *cparser.BlockItemContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if s := ast.Statement(); s != nil {
		b.buildStatement(s.(*cparser.StatementContext))
	} else if d := ast.Declaration(); d != nil {
		b.buildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) buildStatement(ast *cparser.StatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.ExpressionStatement(); e != nil {
		b.buildExpressionStatement(e.(*cparser.ExpressionStatementContext))
	} else if j := ast.JumpStatement(); j != nil {
		b.buildJumpStatement(j.(*cparser.JumpStatementContext))
	} else if c := ast.CompoundStatement(); c != nil {
		b.buildCompoundStatement(c.(*cparser.CompoundStatementContext))
	} else if s := ast.SelectionStatement(); s != nil {
		b.buildSelectionStatement(s.(*cparser.SelectionStatementContext))
	} else if s := ast.StatementsExpression(); s != nil {
		b.buildStatementsExpression(s.(*cparser.StatementsExpressionContext))
	} else if i := ast.IterationStatement(); i != nil {
		b.buildIterationStatement(i.(*cparser.IterationStatementContext))
	} else if a := ast.AsmStatement(); a != nil {
		b.buildAsmStatement(a.(*cparser.AsmStatementContext))
	}
}

func (b *astbuilder) buildStatementsExpression(ast *cparser.StatementsExpressionContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	return nil
}

func (b *astbuilder) buildAsmStatement(ast *cparser.AsmStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildIterationStatement(ast *cparser.IterationStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	loop := b.CreateLoopBuilder()
	if e := ast.Expression(); e != nil {
		var condition ssa.Value
		cond := e
		loop.SetCondition(func() ssa.Value {
			if cond == nil {
				condition = b.EmitConstInst(true)
			} else {
				// recoverRange := b.SetRange(cond.BaseParserRuleContext)
				// defer recoverRange()
				condition = b.buildExpression(cond.(*cparser.ExpressionContext))
				if condition == nil {
					condition = b.EmitConstInst(true)
					// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
				}
			}
			return condition
		})
	} else if condition, ok := ast.ForCondition().(*cparser.ForConditionContext); ok {
		if first, ok := condition.ForDeclaration().(*cparser.ForDeclarationContext); ok {
			// first expression is initialization, in enter block
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(first.BaseParserRuleContext)
				defer recoverRange()
				return b.buildForDeclaration(first)
			})
		} else if first, ok := condition.AssignmentExpression().(*cparser.AssignmentExpressionContext); ok {
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(first.BaseParserRuleContext)
				defer recoverRange()
				return ssa.Values{b.buildAssignmentExpression(first)}
			})
		}
		if expr, ok := condition.ForExpression(0).(*cparser.ForExpressionContext); ok {
			// build expression in header
			cond := expr
			loop.SetCondition(func() ssa.Value {
				var condition ssa.Value
				if cond == nil {
					condition = b.EmitConstInst(true)
				} else {
					// recoverRange := b.SetRange(cond.BaseParserRuleContext)
					// defer recoverRange()
					conditions := b.buildForExpression(cond)
					for _, c := range conditions {
						condition = c
					}
					if conditions == nil {
						condition = b.EmitConstInst(true)
						// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
					}
				}
				return condition
			})
		}
		if third, ok := condition.ForExpression(1).(*cparser.ForExpressionContext); ok {
			// build latch
			loop.SetThird(func() []ssa.Value {
				// build third expression in loop.latch
				recoverRange := b.SetRange(third.BaseParserRuleContext)
				defer recoverRange()
				return b.buildForExpression(third)
			})
		}
	}

	loop.SetBody(func() {
		if block, ok := ast.Statement().(*cparser.StatementContext); ok {
			b.buildStatement(block)
		}
	})
	loop.Finish()
}

func (b *astbuilder) buildForDeclaration(ast *cparser.ForDeclarationContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifiers(); d != nil {
		ssatypes := b.buildDeclarationSpecifiers(d.(*cparser.DeclarationSpecifiersContext))
		lefts := b.buildInitDeclaratorList(ast.InitDeclaratorList().(*cparser.InitDeclaratorListContext))
		for i, l := range lefts {
			if l.GetValue() == nil {
				right := b.GetDefaultValue(ssatypes[i])
				b.AssignVariable(l, right)
			}
			if ssatypes == nil {
				break
			}
			if ssatypes.String() != l.GetValue().GetType().String() {
				b.NewError(ssa.Error, TAG, TypeMismatch(ssatypes.String(), l.GetValue().GetType().String()))
				break
			}
		}
	}

	return nil
}

func (b *astbuilder) buildForExpression(ast *cparser.ForExpressionContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ret ssa.Values
	for _, e := range ast.AllExpression() {
		ret = append(ret, b.buildExpression(e.(*cparser.ExpressionContext)))
	}
	return ret
}

func (b *astbuilder) buildJumpStatement(ast *cparser.JumpStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.Expression(); e != nil {
		right := b.buildExpression(e.(*cparser.ExpressionContext))
		b.EmitReturn(ssa.Values{right})
	}
	if ast.Continue() != nil {
		if !b.Continue() {
			b.NewError(ssa.Error, TAG, UnexpectedContinueStmt())
		}
	} else if ast.Break() != nil {
		if !b.Break() {
			b.NewError(ssa.Error, TAG, UnexpectedBreakStmt())
		}
	}
}

func (b *astbuilder) buildIfStatement(ast *cparser.SelectionStatementContext) {
	Ifbuilder := b.CreateIfBuilder()

	build := func() func() {
		if expression := ast.Expression(); expression != nil {
			Ifbuilder.AppendItem(
				func() ssa.Value {
					recoverRange := b.SetRange(ast.Expression())
					b.AppendBlockRange()
					recoverRange()

					right := b.buildExpression(expression.(*cparser.ExpressionContext))
					return right
				},
				func() {
					if s, ok := ast.Statement(0).(*cparser.StatementContext); ok {
						b.buildStatement(s)
					}
				},
			)
		}

		if ast.Else() != nil {
			if elseBlock, ok := ast.Statement(1).(*cparser.StatementContext); ok {
				return func() {
					b.buildStatement(elseBlock)
				}
			} else {
				return nil
			}
		}
		return nil
	}

	elseBlock := build()
	Ifbuilder.SetElse(elseBlock)
	Ifbuilder.Build()
}

func (b *astbuilder) buildSwitchStatement(ast *cparser.SelectionStatementContext) {
	Switchbuilder := b.BuildSwitch()
	Switchbuilder.AutoBreak = false

	var casepList []*cparser.LabeledStatementContext
	var defaultp *cparser.LabeledStatementContext

	for _, commCase := range ast.AllLabeledStatement() {
		if commSwitchCase := commCase.(*cparser.LabeledStatementContext); commSwitchCase != nil {
			if commSwitchCase.Default() != nil {
				defaultp = commSwitchCase
			}
			if commSwitchCase.Case() != nil {
				casepList = append(casepList, commSwitchCase)
			}
		}
	}

	Switchbuilder.BuildCaseSize(len(casepList))
	Switchbuilder.SetCase(func(i int) []ssa.Value {
		var value ssa.Value
		if e := casepList[i].Expression(); e != nil {
			value = b.buildExpression(e.(*cparser.ExpressionContext))
		}
		return ssa.Values{value}
	})

	Switchbuilder.BuildBody(func(i int) {
		for _, statement := range casepList[i].AllStatement() {
			b.buildStatement(statement.(*cparser.StatementContext))
		}
	})

	// default
	if defaultp != nil {
		Switchbuilder.BuildDefault(func() {
			for _, statement := range defaultp.AllStatement() {
				b.buildStatement(statement.(*cparser.StatementContext))
			}
		})
	}

	Switchbuilder.Finish()
}

func (b *astbuilder) buildSelectionStatement(ast *cparser.SelectionStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if ast.If() != nil {
		b.buildIfStatement(ast)
	} else if ast.Switch() != nil {
		b.buildSwitchStatement(ast)
	}
}

func (b *astbuilder) buildExpressionStatement(ast *cparser.ExpressionStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, a := range ast.AllAssignmentExpression() {
		b.buildAssignmentExpression(a.(*cparser.AssignmentExpressionContext))
	}
}
