package c2ssa

import (
	"strconv"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	} else if ds := ast.DeclarationSpecifier(); ds != nil {
		b.buildDeclarationSpecifier(ds.(*cparser.DeclarationSpecifierContext))
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

		if a := ast.AssignmentExpression(); a != nil {
			base = b.buildAssignmentExpression(a.(*cparser.AssignmentExpressionContext))
		}
		variable, index, _ := b.buildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), kinds...)
		if c1, ok := ssa.ToConstInst(index); ok {
			if c2, ok := ssa.ToConstInst(base); ok {
				i1, _ := strconv.Atoi(c1.String())
				i2, _ := strconv.Atoi(c2.String())
				base = b.EmitConstInst(i1 * i2)
			}
		}
		return variable, base, nil
	}

	// directDeclarator: directDeclarator '(' parameterTypeList ')'
	if dd := ast.DirectDeclarator(); dd != nil && ast.LeftParen() != nil && ast.ParameterTypeList() != nil {
		switch kind {
		case VARIABLE_KIND:
			_, base, _ = b.buildDirectDeclarator(dd.(*cparser.DirectDeclaratorContext), FUNC_KIND)
			return b.CreateLocalVariable(base.GetName()), nil, nil
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
	return b.CreateVariable(""), b.EmitConstInst(0), nil
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
		if d := ast.Declarator(); d != nil {
			_, param, _ := b.buildDeclarator(d.(*cparser.DeclaratorContext), PARAM_KIND)
			if ssatyp != nil && param != nil {
				param.SetType(ssatyp)
			}
			return param, ssatyp
		} else if a := ast.AbstractDeclarator(); a != nil {
			b.buildAbstractDeclarator(a.(*cparser.AbstractDeclaratorContext))
		}

	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0), ssa.CreateAnyType()
}

func (b *astbuilder) buildAbstractDeclarator(ast *cparser.AbstractDeclaratorContext) {
	// TODO
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildGccDeclaratorExtension(ast *cparser.GccDeclaratorExtensionContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildDeclaration(ast *cparser.DeclarationContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifier(); d != nil {
		ssatype := b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		if init := ast.InitDeclaratorList(); init != nil {
			lefts, indexs := b.buildInitDeclaratorList(init.(*cparser.InitDeclaratorListContext), ssatype)
			for i, l := range lefts {
				if l.GetValue() == nil {
					right := b.GetDefaultValue(ssatype)
					if indexs[i] != -1 {
						newtype := ssa.NewSliceType(ssatype)
						newtype.Len = indexs[i]
						right = b.GetDefaultValue(newtype)
					}
					b.AssignVariable(l, right)
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
		right, _ := b.buildExpression(c.(*cparser.ExpressionContext), false)
		_ = right
	}
}

func (b *astbuilder) buildInitDeclaratorList(ast *cparser.InitDeclaratorListContext, ssatype ...ssa.Type) ([]*ssa.Variable, []int) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var lefts []*ssa.Variable
	var indexs []int
	for _, i := range ast.AllInitDeclarator() {
		left, index := b.buildInitDeclarator(i.(*cparser.InitDeclaratorContext), ssatype...)
		lefts = append(lefts, left)
		indexs = append(indexs, index)
	}
	return lefts, indexs
}

func (b *astbuilder) buildInitDeclarator(ast *cparser.InitDeclaratorContext, ssatype ...ssa.Type) (*ssa.Variable, int) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.Declarator(); d != nil {
		left, right, _ := b.buildDeclarator(d.(*cparser.DeclaratorContext), VARIABLE_KIND)
		if e := ast.Initializer(); e != nil {
			initial := b.buildInitializer(e.(*cparser.InitializerContext), ssatype...)
			b.AssignVariable(left, initial)
			return left, -1
		}
		if right != nil {
			index, _ := strconv.Atoi(right.String())
			return left, index
		}
		return left, -1
	}
	return nil, -1
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

	if s := ast.StorageClassSpecifier(0); s != nil {
		// TODO
		// ret = b.buildStorageClassSpecifier(s.(*cparser.StorageClassSpecifierContext))
	}
	if t := ast.TypeQualifier(0); t != nil {
		// TODO
		// ret = b.buildTypeQualifier(t.(*cparser.TypeQualifierContext))
	}
	if f := ast.FunctionSpecifier(0); f != nil {
		// TODO
		// ret = b.buildFunctionSpecifier(f.(*cparser.FunctionSpecifierContext))
	}

	if ts := ast.TypeSpecifier(); ts != nil {
		ret = b.buildTypeSpecifier(ts.(*cparser.TypeSpecifierContext))
		// if tq := ast.TypeQualifier(); tq != nil {
		// 	ret = b.buildTypeQualifier(tq.(*cparser.TypeQualifierContext))
		// }
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

	if s := ast.SpecifierQualifierList(); s != nil {
		ssatype := b.buildSpecifierQualifierList(s.(*cparser.SpecifierQualifierListContext))
		if a := ast.AbstractDeclarator(); a != nil {
			_ = a
		}
		return ssatype
	}

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

	if a := ast.AtomicTypeSpecifier(); a != nil {

	} else if s := ast.StructOrUnionSpecifier(); s != nil {
		return b.buildStructOrUnionSpecifier(s.(*cparser.StructOrUnionSpecifierContext))
	} else if e := ast.EnumSpecifier(); e != nil {
		b.buildEnumSpecifier(e.(*cparser.EnumSpecifierContext))
	} else if t := ast.TypedefName(); t != nil {
		return b.buildTypedefName(t.(*cparser.TypedefNameContext))
	} else {
		name := ast.GetText()
		if ssatyp := ssa.GetTypeByStr(name); ssatyp != nil {
			return ssatyp
		}
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypedefName(ast *cparser.TypedefNameContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if id := ast.Identifier(); id != nil {
		name := id.GetText()
		if bp := b.GetBluePrint(name); bp != nil {
			container := bp.Container()
			return container.GetType()
		}
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildEnumSpecifier(ast *cparser.EnumSpecifierContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// TODO
	if e := ast.EnumeratorList(); e != nil {
		b.buildEnumeratorList(e.(*cparser.EnumeratorListContext))
	}
}

func (b *astbuilder) buildEnumeratorList(ast *cparser.EnumeratorListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, e := range ast.AllEnumerator() {
		b.buildEnumerator(e.(*cparser.EnumeratorContext))
	}
}

func (b *astbuilder) buildEnumerator(ast *cparser.EnumeratorContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if id := ast.Identifier(); id != nil {
		if e := ast.Expression(); e != nil {
			right, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
			b.addSpecialValue(id.GetText(), right)
		}
	}
}

func (b *astbuilder) buildStructOrUnionSpecifier(ast *cparser.StructOrUnionSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if id := ast.Identifier(0); id != nil {
		if s := ast.StructDeclarationList(); s != nil {
			structTyp := ssa.NewStructType()
			bp := b.CreateBlueprintAndSetConstruct(id.GetText())
			b.buildStructDeclarationList(s.(*cparser.StructDeclarationListContext), structTyp)
			c := bp.Container()
			c.SetType(structTyp)
		}
		if bp := b.GetBluePrint(id.GetText()); bp != nil {
			container := bp.Container()
			return container.GetType()
		}
	}

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildStructDeclarationList(ast *cparser.StructDeclarationListContext, structTyp *ssa.ObjectType) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, s := range ast.AllStructDeclaration() {
		b.buildStructDeclaration(s.(*cparser.StructDeclarationContext), structTyp)
	}
}

func (b *astbuilder) buildStructDeclaration(ast *cparser.StructDeclarationContext, structTyp *ssa.ObjectType) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if sq := ast.SpecifierQualifierList(); sq != nil {
		ssatype := b.buildSpecifierQualifierList(sq.(*cparser.SpecifierQualifierListContext))
		if sd := ast.StructDeclaratorList(); sd != nil {
			lefts := b.buildStructDeclaratorList(sd.(*cparser.StructDeclaratorListContext))
			for _, l := range lefts {
				structTyp.AddField(b.EmitConstInst(l.GetName()), ssatype)
			}
		}
	} else if sa := ast.StaticAssertDeclaration(); sa != nil {
		b.buildStaticAssertDeclaration(sa.(*cparser.StaticAssertDeclarationContext))
	}
}

func (b *astbuilder) buildSpecifierQualifierList(ast *cparser.SpecifierQualifierListContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ssatype ssa.Type
	if t := ast.TypeSpecifier(); t != nil {
		ssatype = b.buildTypeSpecifier(t.(*cparser.TypeSpecifierContext))
	} else if t := ast.TypeQualifier(); t != nil {
		ssatype = b.buildTypeQualifier(t.(*cparser.TypeQualifierContext))
	}

	if s := ast.SpecifierQualifierList(); s != nil {
		b.buildSpecifierQualifierList(s.(*cparser.SpecifierQualifierListContext))
	}
	return ssatype
}

func (b *astbuilder) buildStructDeclaratorList(ast *cparser.StructDeclaratorListContext) []*ssa.Variable {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ret []*ssa.Variable
	for _, s := range ast.AllStructDeclarator() {
		ret = append(ret, b.buildStructDeclarator(s.(*cparser.StructDeclaratorContext)))
	}
	return ret
}

func (b *astbuilder) buildStructDeclarator(ast *cparser.StructDeclaratorContext) *ssa.Variable {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var ret *ssa.Variable
	if d := ast.Declarator(); d != nil {
		left, _, _ := b.buildDeclarator(d.(*cparser.DeclaratorContext), VARIABLE_KIND)
		ret = left
	}
	if e := ast.Expression(); e != nil {
		right, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
		if ret != nil {
			b.AssignVariable(ret, right)
		}
	}
	return ret
}

func (b *astbuilder) buildDeclarationList(ast *cparser.DeclarationListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	for _, d := range ast.AllDeclaration() {
		b.buildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) buildCompoundStatement(ast *cparser.CompoundStatementContext, isBlock ...bool) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if len(isBlock) > 0 && isBlock[0] {
		b.BuildSyntaxBlock(func() {
			if block := ast.BlockItemList(); block != nil {
				b.buildBlockItemList(block.(*cparser.BlockItemListContext))
			}
		})
	} else {
		if block := ast.BlockItemList(); block != nil {
			b.buildBlockItemList(block.(*cparser.BlockItemListContext))
		}
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
		b.buildCompoundStatement(c.(*cparser.CompoundStatementContext), true)
	} else if s := ast.SelectionStatement(); s != nil {
		b.buildSelectionStatement(s.(*cparser.SelectionStatementContext))
	} else if s := ast.StatementsExpression(); s != nil {
		b.buildStatementsExpression(s.(*cparser.StatementsExpressionContext))
	} else if i := ast.IterationStatement(); i != nil {
		b.buildIterationStatement(i.(*cparser.IterationStatementContext))
	} else if a := ast.AsmStatement(); a != nil {
		b.buildAsmStatement(a.(*cparser.AsmStatementContext))
	} else if id := ast.Identifier(); id != nil {
		b.buildLabeledStatement(ast, id.GetText())
	}
}

func (b *astbuilder) buildLabeledStatement(ast *cparser.StatementContext, text string) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	LabelBuilder := b.GetLabelByName(text)
	block := LabelBuilder.GetBlock()
	LabelBuilder.Build()
	b.AddLabel(text, block)
	for _, f := range LabelBuilder.GetGotoHandlers() {
		f(block)
	}

	b.EmitJump(block)
	b.CurrentBlock = block
	LabelBuilder.Finish()

	if s, ok := ast.Statement().(*cparser.StatementContext); ok {
		b.buildStatement(s)
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
			if utils.IsNil(cond) {
				condition = b.EmitConstInst(true)
			} else {
				// recoverRange := b.SetRange(cond.BaseParserRuleContext)
				// defer recoverRange()
				condition, _ = b.buildExpression(cond.(*cparser.ExpressionContext), false)
			}
			if utils.IsNil(condition) {
				condition = b.EmitConstInst(true)
				// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
			}
			return condition
		})
	} else if condition, ok := ast.ForCondition().(*cparser.ForConditionContext); ok {
		if first, ok := condition.ForDeclarations().(*cparser.ForDeclarationsContext); ok {
			// first expression is initialization, in enter block
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(first.BaseParserRuleContext)
				defer recoverRange()
				return b.buildForDeclarations(first)
			})
		} else if first, ok := condition.AssignmentExpressions().(*cparser.AssignmentExpressionsContext); ok {
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(first.BaseParserRuleContext)
				defer recoverRange()
				return b.buildAssignmentExpressions(first)
			})
		}
		if expr, ok := condition.ForExpression(0).(*cparser.ForExpressionContext); ok {
			// build expression in header
			cond := expr
			loop.SetCondition(func() ssa.Value {
				var condition ssa.Value
				if utils.IsNil(cond) {
					condition = b.EmitConstInst(true)
				} else {
					// recoverRange := b.SetRange(cond.BaseParserRuleContext)
					// defer recoverRange()
					conditions := b.buildForExpression(cond)
					for _, c := range conditions {
						condition = c
					}
				}
				if utils.IsNil(condition) {
					condition = b.EmitConstInst(true)
					// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
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

func (b *astbuilder) buildForDeclarations(ast *cparser.ForDeclarationsContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Values

	for _, f := range ast.AllForDeclaration() {
		ret = append(ret, b.buildForDeclaration(f.(*cparser.ForDeclarationContext))...)
	}
	return ret
}

func (b *astbuilder) buildForDeclaration(ast *cparser.ForDeclarationContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.DeclarationSpecifier(); d != nil {
		ssatype := b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		lefts, indexs := b.buildInitDeclaratorList(ast.InitDeclaratorList().(*cparser.InitDeclaratorListContext))
		for i, l := range lefts {
			if l.GetValue() == nil {
				right := b.GetDefaultValue(ssatype)
				if indexs[i] != -1 {
					newtype := ssa.NewSliceType(ssatype)
					newtype.Len = indexs[i]
					right = b.GetDefaultValue(newtype)
				}
				b.AssignVariable(l, right)
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
		value, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
		ret = append(ret, value)
	}
	return ret
}

func (b *astbuilder) buildJumpStatement(ast *cparser.JumpStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if ast.Return() != nil {
		if e := ast.Expression(); e != nil {
			right, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
			b.EmitReturn(ssa.Values{right})
		}
	} else if ast.Goto() != nil {
		if id := ast.Identifier(); id != nil {
			b.handlerGoto(id.GetText())
		}
	} else if ast.Continue() != nil {
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

					right, _ := b.buildExpression(expression.(*cparser.ExpressionContext), false)
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
			value, _ = b.buildExpression(e.(*cparser.ExpressionContext), false)
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

func (b *astbuilder) buildExpressionStatement(ast *cparser.ExpressionStatementContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if a := ast.AssignmentExpressions(); a != nil {
		return b.buildAssignmentExpressions(a.(*cparser.AssignmentExpressionsContext))
	}
	return nil
}

func (b *astbuilder) buildAssignmentExpressions(ast *cparser.AssignmentExpressionsContext) ssa.Values {
	var ret ssa.Values
	for _, a := range ast.AllAssignmentExpression() {
		ret = append(ret, b.buildAssignmentExpression(a.(*cparser.AssignmentExpressionContext)))
	}
	return ret
}

func (b *astbuilder) handlerGoto(labelName string, isBreak ...bool) {
	gotoBuilder := b.BuildGoto(labelName)
	if len(isBreak) > 0 {
		gotoBuilder.SetBreak(isBreak[0])
	}
	if targetBlock := b.GetLabel(labelName); targetBlock != nil {
		// target label exist, just set it
		LabelBuilder := b.GetLabelByName(labelName)
		gotoBuilder.SetLabel(targetBlock)
		f := gotoBuilder.Finish()
		LabelBuilder.SetGotoFinish(f)
	} else {
		// target label not exist, create it
		LabelBuilder := b.BuildLabel(labelName)
		// use handler function
		LabelBuilder.SetGotoHandler(func(_goto *ssa.BasicBlock) {
			gotoBuilder.SetLabel(_goto)
			f := gotoBuilder.Finish()
			LabelBuilder.SetGotoFinish(f)
		})
		b.labels[labelName] = LabelBuilder
	}
}
