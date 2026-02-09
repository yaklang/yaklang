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
	} else if mce := ast.MacroCallExpression(); mce != nil {
		// 处理宏调用表达式（如 FUN(fmin, double, <)）
		b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
	} else if mcs := ast.MacroCallStatement(); mcs != nil {
		// 处理宏调用语句（如 FF_DISABLE_DEPRECATION_WARNINGS）
		b.buildMacroCallStatement(mcs.(*cparser.MacroCallStatementContext))
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
				_, _, paramTypes = b.buildDeclarator(de.(*cparser.DeclaratorContext), FUNC_KIND)

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

	var ssatypes ssa.Types

	kind := NORMAL_KIND
	if len(kinds) > 0 {
		kind = kinds[0]
	}

	// directDeclarator: Identifier declaratorSuffix*
	if id := ast.Identifier(); id != nil {
		// Identifier 本身只是一个名字，不根据 kind 创建值
		// 根据 declaratorSuffix 的类型来决定如何构建
		identifierName := id.GetText()
		var variable *ssa.Variable
		var value ssa.Value

		// 先处理基础 Identifier，根据是否有 suffix 来决定初始值
		suffixes := ast.AllDeclaratorSuffix()
		if len(suffixes) == 0 {
			// 没有 suffix，直接根据 kind 创建
			return b.buildIdentifierDeclarator(identifierName, kind)
		}

		// 有 suffix，先创建基础标识符，然后逐个处理 suffix
		// 第一个 suffix 决定基础类型
		firstSuffix := suffixes[0].(*cparser.DeclaratorSuffixContext)
		if firstSuffix.ArraySuffix() != nil {
			// 数组类型：先创建变量名，然后处理数组维度
			if kind == VARIABLE_KIND {
				variable = b.CreateLocalVariable(identifierName)
			}
			value = b.EmitConstInst(identifierName)
		} else if firstSuffix.FunctionSuffix() != nil {
			// 函数类型：根据 kind 创建函数
			// 重用 buildIdentifierDeclarator 的逻辑，只取 value 部分
			_, value, _ = b.buildIdentifierDeclarator(identifierName, kind)
		} else {
			// 未知类型，使用默认处理
			value = b.EmitConstInst(identifierName)
		}

		// 逐个处理 declaratorSuffix*
		for _, suffix := range suffixes {
			variable, value, ssatypes = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, ssatypes, PARAM_KIND)
		}
		return variable, value, ssatypes
	}

	// directDeclarator: macroCallExpression declaratorSuffix*
	if mce := ast.MacroCallExpression(); mce != nil {
		// 处理宏调用表达式（暂时作为普通标识符处理）
		value := b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		var variable *ssa.Variable
		if kind == VARIABLE_KIND {
			variable = b.CreateLocalVariable(value.GetName())
		}
		// 逐个处理 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, ssatypes = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, ssatypes, kind)
		}
		return variable, value, ssatypes
	}

	// directDeclarator: '(' declarator ')' declaratorSuffix*
	if d := ast.Declarator(); d != nil {
		declCtx := d.(*cparser.DeclaratorContext)
		// 先处理内部的 directDeclarator
		var variable *ssa.Variable
		var value ssa.Value
		var types ssa.Types
		if innerDirect := declCtx.DirectDeclarator(); innerDirect != nil {
			variable, value, types = b.buildDirectDeclarator(innerDirect.(*cparser.DirectDeclaratorContext), kind)
		}
		// 然后应用指针修饰符（如果有）
		if p := declCtx.Pointer(); p != nil {
			b.applyPointerModifiers(p.(*cparser.PointerContext), value)
		}
		// 处理 gccDeclaratorExtension
		for _, g := range declCtx.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		// 处理 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, types = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, types, kind)
		}
		return variable, value, types
	}

	// directDeclarator: Identifier ':' DigitSequence
	if id := ast.Identifier(); id != nil && ast.DigitSequence() != nil {
		return nil, b.EmitConstInst("bitfield"), nil
	}

	// directDeclarator: vcSpecificModifer Identifier declaratorSuffix*
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Identifier() != nil {
		b.buildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		var value ssa.Value = b.EmitConstInst("vcSpecific")
		var variable *ssa.Variable
		// 逐个处理 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, ssatypes = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, ssatypes, kind)
		}
		return variable, value, ssatypes
	}

	// directDeclarator: '(' vcSpecificModifer declarator ')' declaratorSuffix*
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Declarator() != nil {
		b.buildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		declCtx := ast.Declarator().(*cparser.DeclaratorContext)
		// 先处理内部的 directDeclarator
		var variable *ssa.Variable
		var value ssa.Value
		var types ssa.Types
		if innerDirect := declCtx.DirectDeclarator(); innerDirect != nil {
			variable, value, types = b.buildDirectDeclarator(innerDirect.(*cparser.DirectDeclaratorContext), kind)
		}
		// 然后应用指针修饰符（如果有）
		if p := declCtx.Pointer(); p != nil {
			b.applyPointerModifiers(p.(*cparser.PointerContext), value)
		}
		// 处理 gccDeclaratorExtension
		for _, g := range declCtx.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		// 处理 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, types = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, types, kind)
		}
		return variable, value, types
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.CreateVariable(""), b.EmitConstInst(0), nil
}

// buildIdentifierDeclarator 处理没有 suffix 的简单标识符声明
func (b *astbuilder) buildIdentifierDeclarator(name string, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	switch kind {
	case VARIABLE_KIND:
		return b.CreateLocalVariable(name), nil, nil
	case NORMAL_KIND:
		return nil, b.EmitConstInst(name), nil
	case PARAM_KIND:
		return nil, b.NewParam(name), nil
	case FUNC_KIND:
		return nil, b.NewFunc(name), nil
	default:
		return nil, b.EmitConstInst(name), nil
	}
}

// buildDeclaratorSuffix 处理声明符后缀（数组维度或函数参数）
func (b *astbuilder) buildDeclaratorSuffix(ast *cparser.DeclaratorSuffixContext, variable *ssa.Variable, value ssa.Value, ssatypes ssa.Types, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// 处理数组后缀: arraySuffix
	if as := ast.ArraySuffix(); as != nil {
		arraySuffix := as.(*cparser.ArraySuffixContext)
		var base ssa.Value
		if e := arraySuffix.Expression(); e != nil {
			base, _ = b.buildExpression(e.(*cparser.ExpressionContext), false)
		}
		if c1, ok := ssa.ToConstInst(value); ok {
			if c2, ok := ssa.ToConstInst(base); ok {
				i1, _ := strconv.Atoi(c1.String())
				i2, _ := strconv.Atoi(c2.String())
				base = b.EmitConstInst(i1 * i2)
			}
		}
		if utils.IsNil(base) {
			base = b.EmitConstInst(0)
		}
		return variable, base, nil
	}

	// 处理函数后缀: functionSuffix
	if fs := ast.FunctionSuffix(); fs != nil {
		return b.buildFunctionSuffixDeclarator(fs.(*cparser.FunctionSuffixContext), variable, value, ssatypes, kind)
	}

	return variable, value, ssatypes
}

// buildFunctionSuffixDeclarator 处理函数后缀声明符
func (b *astbuilder) buildFunctionSuffixDeclarator(functionSuffix *cparser.FunctionSuffixContext, variable *ssa.Variable, value ssa.Value, ssatypes ssa.Types, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	// 提取参数类型列表（如果有）
	if ptl := functionSuffix.ParameterTypeList(); ptl != nil {
		_, ssatypes = b.buildParameterTypeList(ptl.(*cparser.ParameterTypeListContext))
	} else if idl := functionSuffix.IdentifierList(); idl != nil {
		b.buildIdentifierList(idl.(*cparser.IdentifierListContext))
	}

	// 根据 kind 决定返回值
	switch kind {
	case VARIABLE_KIND:
		// 函数类型的变量：优先使用 variable 的名字，如果没有则使用 value 的名字
		var varName string
		if variable != nil {
			varName = variable.GetName()
		} else if value != nil {
			varName = value.GetName()
		}
		// 如果仍然没有名字，使用默认值
		if varName == "" {
			varName = "unknown"
		}
		return b.CreateLocalVariable(varName), nil, nil
	case FUNC_KIND:
		// 函数定义：保持 value（应该是函数）
		if utils.IsNil(value) {
			value = b.EmitConstInst(0)
		}
		return variable, value, ssatypes
	case PARAM_KIND:
		// 参数声明：已经提取了参数类型，返回 value 和类型
		if utils.IsNil(value) {
			value = b.EmitConstInst(0)
		}
		return variable, value, ssatypes
	default:
		// 其他情况：保持原值
		if utils.IsNil(value) {
			value = b.EmitConstInst(0)
		}
		return variable, value, ssatypes
	}
}

// buildMacroCallExpression 处理宏调用表达式
// macroCallExpression: Identifier '(' macroArgumentList? ')' postfixSuffix*
func (b *astbuilder) buildMacroCallExpression(ast *cparser.MacroCallExpressionContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value

	// 获取宏名称
	if id := ast.Identifier(); id != nil {
		macroName := id.GetText()
		// 处理宏参数列表
		var args ssa.Values
		if mal := ast.MacroArgumentList(); mal != nil {
			args = b.buildMacroArgumentList(mal.(*cparser.MacroArgumentListContext))
		}
		// 暂时作为函数调用处理（如果宏展开为函数调用）
		// 或者作为普通标识符处理
		if len(args) > 0 {
			// 尝试作为函数调用处理
			if fun, ok := b.GetFunc(macroName, ""); ok {
				right = b.EmitCall(b.NewCall(fun, args))
			} else {
				// 无法找到函数，作为常量处理
				right = b.EmitConstInst(macroName)
			}
		} else {
			right = b.EmitConstInst(macroName)
		}
	}

	// 处理 postfixSuffix*（如数组下标）
	for _, suffix := range ast.AllPostfixSuffix() {
		right, _ = b.buildPostfixSuffix(suffix.(*cparser.PostfixSuffixContext), right, nil, false)
	}

	if utils.IsNil(right) {
		right = b.EmitConstInst(0)
	}
	return right
}

// buildMacroArgumentList 处理宏参数列表
func (b *astbuilder) buildMacroArgumentList(ast *cparser.MacroArgumentListContext) ssa.Values {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Values
	for _, a := range ast.AllMacroArgument() {
		right := b.buildMacroArgument(a.(*cparser.MacroArgumentContext))
		if right != nil {
			ret = append(ret, right)
		}
	}
	return ret
}

// buildMacroCallStatement 处理宏调用语句
// macroCallStatement: Identifier eos*
func (b *astbuilder) buildMacroCallStatement(ast *cparser.MacroCallStatementContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// 宏调用语句通常不产生值，只是用于预处理指令或副作用
	// 这里可以记录宏调用，但不做实际处理
	if id := ast.Identifier(); id != nil {
		_ = id.GetText()
		// 可以在这里添加宏展开逻辑（如果需要）
	}
}

// applyPointerModifiers 应用指针修饰符到类型上
func (b *astbuilder) applyPointerModifiers(pointer *cparser.PointerContext, value ssa.Value) {
	if pointer == nil || value == nil {
		return
	}

	pointerParts := pointer.AllPointerPart()
	// 对每个 pointerPart，应用指针类型
	for _, part := range pointerParts {
		// 处理类型限定符（如果需要）
		if tql := part.(*cparser.PointerPartContext).TypeQualifierList(); tql != nil {
			b.buildTypeQualifierList(tql.(*cparser.TypeQualifierListContext))
		}
		// 应用指针类型到 value 的类型上
		currentType := value.GetType()
		if currentType != nil {
			pointerType := ssa.NewPointerType()
			// 如果当前类型已经是指针，创建多级指针
			if currentType.GetTypeKind() == ssa.PointerKind {
				// 多级指针：保持为指针类型
				value.SetType(pointerType)
			} else {
				// 单级指针：将基础类型包装为指针
				pointerType.FieldType = currentType
				value.SetType(pointerType)
			}
		} else {
			// 如果没有类型，创建指针类型
			value.SetType(ssa.NewPointerType())
		}
	}
}

// buildDeclaratorCore 处理声明符的核心逻辑（不含指针修饰和扩展）
func (b *astbuilder) buildDeclaratorCore(directDeclarator *cparser.DirectDeclaratorContext, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	if directDeclarator == nil {
		return nil, nil, nil
	}
	return b.buildDirectDeclarator(directDeclarator, kind)
}

// buildDeclarator 处理完整的声明符：pointer? directDeclarator gccDeclaratorExtension*
func (b *astbuilder) buildDeclarator(ast *cparser.DeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// 先处理 directDeclarator（核心逻辑）
	var variable *ssa.Variable
	var value ssa.Value
	var types ssa.Types

	kind := NORMAL_KIND
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	if d := ast.DirectDeclarator(); d != nil {
		variable, value, types = b.buildDeclaratorCore(d.(*cparser.DirectDeclaratorContext), kind)
	}

	if p := ast.Pointer(); p != nil {
		b.applyPointerModifiers(p.(*cparser.PointerContext), value)
	}

	for _, g := range ast.AllGccDeclaratorExtension() {
		b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
	}

	return variable, value, types
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
		var param ssa.Value
		ssatyp := b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext))
		if d := ast.Declarator(); d != nil {
			_, param, _ = b.buildDeclarator(d.(*cparser.DeclaratorContext), PARAM_KIND)
			// if ssatyp != nil && param != nil {
			// 	param.SetType(ssatyp)
			// }
		} else if a := ast.AbstractDeclarator(); a != nil {
			ssatyp = b.buildAbstractDeclarator(a.(*cparser.AbstractDeclaratorContext), ssatyp)
		}
		return param, ssatyp
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.EmitConstInst(0), ssa.CreateAnyType()
}

// buildAbstractDeclarator 应用抽象声明符到基础类型上
// abstractDeclarator 用于描述类型而不包含变量名（如 int *, int (*)(), int [10] 等）
func (b *astbuilder) buildAbstractDeclarator(ast *cparser.AbstractDeclaratorContext, baseType ssa.Type) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if utils.IsNil(baseType) {
		baseType = ssa.CreateAnyType()
	}

	resultType := baseType

	if p := ast.Pointer(); p != nil {
		pointerParts := p.(*cparser.PointerContext).AllPointerPart()
		for _, part := range pointerParts {
			if tql := part.(*cparser.PointerPartContext).TypeQualifierList(); tql != nil {
				b.buildTypeQualifierList(tql.(*cparser.TypeQualifierListContext))
			}
			pointerType := ssa.NewPointerType()
			pointerType.FieldType = resultType
			resultType = pointerType
		}
	}

	// 处理 directAbstractDeclarator（数组、函数等修饰）
	if d := ast.DirectAbstractDeclarator(); d != nil {
		resultType = b.buildDirectAbstractDeclarator(d.(*cparser.DirectAbstractDeclaratorContext), resultType)
	}

	// 处理 gccDeclaratorExtension
	for _, g := range ast.AllGccDeclaratorExtension() {
		b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
	}

	return resultType
}

// buildDirectAbstractDeclarator 处理直接抽象声明符（数组、函数等）
func (b *astbuilder) buildDirectAbstractDeclarator(ast *cparser.DirectAbstractDeclaratorContext, baseType ssa.Type) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	resultType := baseType

	// 1. '(' abstractDeclarator ')' abstractDeclaratorSuffix*
	if ast.LeftParen() != nil && ast.RightParen() != nil {
		if a := ast.AbstractDeclarator(); a != nil {
			resultType = b.buildAbstractDeclarator(a.(*cparser.AbstractDeclaratorContext), resultType)
		}
		// 处理 abstractDeclaratorSuffix*
		for _, suffix := range ast.AllAbstractDeclaratorSuffix() {
			resultType = b.buildAbstractDeclaratorSuffix(suffix.(*cparser.AbstractDeclaratorSuffixContext), resultType)
		}
		// 处理 gccDeclaratorExtension
		for _, g := range ast.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		return resultType
	}

	// 2. abstractDeclaratorSuffix+ - 必须以至少一个后缀开始
	suffixes := ast.AllAbstractDeclaratorSuffix()
	if len(suffixes) > 0 {
		for _, suffix := range suffixes {
			resultType = b.buildAbstractDeclaratorSuffix(suffix.(*cparser.AbstractDeclaratorSuffixContext), resultType)
		}
		// 处理 gccDeclaratorExtension
		for _, g := range ast.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		return resultType
	}

	return resultType
}

// buildAbstractDeclaratorSuffix 处理抽象声明符后缀（数组维度或函数参数）
func (b *astbuilder) buildAbstractDeclaratorSuffix(ast *cparser.AbstractDeclaratorSuffixContext, baseType ssa.Type) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	resultType := baseType

	// 处理数组后缀: abstractArraySuffix
	if aas := ast.AbstractArraySuffix(); aas != nil {
		abstractArraySuffix := aas.(*cparser.AbstractArraySuffixContext)
		var arraySize int64 = -1 // -1 表示未指定大小或可变长度数组

		// 处理数组大小表达式
		if c := abstractArraySuffix.CoreExpression(); c != nil {
			expr := b.buildCoreExpression(c.(*cparser.CoreExpressionContext))
			if c, ok := ssa.ToConstInst(expr); ok {
				if size, err := strconv.ParseInt(c.String(), 10, 64); err == nil {
					arraySize = size
				}
			}
		} else if abstractArraySuffix.Star() != nil {
			// '[' '*' ']' - 可变长度数组
			arraySize = -1
		}

		// 创建数组类型（在 SSA 中，数组通常表示为 SliceType）
		if arraySize >= 0 {
			sliceType := ssa.NewSliceType(resultType)
			sliceType.Len = int(arraySize)
			resultType = sliceType
		} else {
			sliceType := ssa.NewSliceType(resultType)
			resultType = sliceType
		}
		return resultType
	}

	// 处理函数后缀: abstractFunctionSuffix
	if afs := ast.AbstractFunctionSuffix(); afs != nil {
		abstractFunctionSuffix := afs.(*cparser.AbstractFunctionSuffixContext)
		var paramTypes ssa.Types
		if ptl := abstractFunctionSuffix.ParameterTypeList(); ptl != nil {
			_, paramTypes = b.buildParameterTypeList(ptl.(*cparser.ParameterTypeListContext))
		}
		funcType := ssa.NewFunctionType("", paramTypes, resultType, false)
		resultType = funcType
		// 处理 gccDeclaratorExtension
		for _, g := range abstractFunctionSuffix.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		return resultType
	}

	return resultType
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
	} else if mce := ast.MacroCallExpression(); mce != nil {
		// 处理宏调用作为声明，如 DECLARE_ALIGNED(...)[8] = {...}
		_ = b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		// 处理可能的 declaratorSuffix*（如数组下标）
		for _, suffix := range ast.AllDeclaratorSuffix() {
			// 这里需要处理后缀，但通常宏调用作为声明时，后缀已经在 macroCallExpression 中处理了
			_ = suffix
		}
		// 处理初始化器（如果有）
		if init := ast.Initializer(); init != nil {
			_ = b.buildInitializer(init.(*cparser.InitializerContext))
			// 可以将初始值赋给变量（如果需要）
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
		if left != nil {
			lefts = append(lefts, left)
			indexs = append(indexs, index)
		}
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
	return b.CreateVariable(""), -1
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

	// log.Infof("exp = %s\n", ast.GetText())

	// 处理 alignmentSpecifier (优先级最高，如果存在则直接返回)
	if a := ast.AlignmentSpecifier(); a != nil {
		return b.buildAlignmentSpecifier(a.(*cparser.AlignmentSpecifierContext))
	}

	// 处理 storageClassSpecifier (存储类说明符，如 static, extern 等)
	for _, s := range ast.AllStorageClassSpecifier() {
		_ = b.buildStorageClassSpecifier(s.(*cparser.StorageClassSpecifierContext))
		// 存储类说明符不影响类型，只影响存储方式
	}

	// 处理 typeQualifier (类型限定符，如 const, volatile 等)
	for _, tq := range ast.AllTypeQualifier() {
		_ = b.buildTypeQualifier(tq.(*cparser.TypeQualifierContext))
		// 类型限定符不影响基础类型，只影响类型属性
	}

	// 处理 functionSpecifier (函数说明符，如 inline 等)
	for _, f := range ast.AllFunctionSpecifier() {
		_ = b.buildFunctionSpecifier(f.(*cparser.FunctionSpecifierContext))
		// 函数说明符不影响类型
	}

	// 处理 structOrUnion (可选的 struct 或 union 关键字)
	if so := ast.StructOrUnion(); so != nil {
		// structOrUnion 通常与 typeSpecifier 一起使用，这里只记录
		_ = so
	}

	// 处理 typeSpecifier 或 Identifier
	if ts := ast.TypeSpecifier(); ts != nil {
		ret = b.buildTypeSpecifier(ts.(*cparser.TypeSpecifierContext))
	} else if id := ast.Identifier(); id != nil {
		// Identifier 可能是 typedef 定义的类型名
		name := id.GetText()
		if bp := b.GetBluePrint(name); bp != nil {
			container := bp.Container()
			ret = container.GetType()
		} else {
			// 如果找不到 typedef，尝试从特殊类型中查找
			if ssatyp := ssa.GetTypeByStr(name); ssatyp != nil {
				ret = ssatyp
			} else {
				ret = ssa.CreateAnyType()
			}
		}
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

func (b *astbuilder) buildTypeNameByValue(ast *cparser.TypeNameContext) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var value ssa.Value
	if t := ast.TypeName(); t != nil {
		value = b.buildTypeNameByValue(t.(*cparser.TypeNameContext))
	} else {
		text := ast.GetText()
		value = b.PeekValue(text)
	}

	return value
}

func (b *astbuilder) buildTypeName(ast *cparser.TypeNameContext) ssa.Type {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if s := ast.SpecifierQualifierList(); s != nil {
		ssatype := b.buildSpecifierQualifierList(s.(*cparser.SpecifierQualifierListContext))
		if a := ast.AbstractDeclarator(); a != nil {
			// 应用抽象声明符到基础类型（如 int * -> 指针类型）
			ssatype = b.buildAbstractDeclarator(a.(*cparser.AbstractDeclaratorContext), ssatype)
		}
		return ssatype
	}

	// if t := ast.TypeName(); t != nil {
	// }

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

	// 处理枚举标识符（如果有）
	var enumName string
	if id := ast.Identifier(); id != nil {
		enumName = id.GetText()
	}

	// 处理枚举列表
	if e := ast.EnumeratorList(); e != nil {
		// 创建枚举类型（在 C 中，枚举类型通常被视为 int 类型）
		enumType := ssa.CreateNumberType()

		// 如果有枚举名称，创建类型蓝图并注册
		if enumName != "" {
			bp := b.CreateBlueprintAndSetConstruct(enumName)
			c := bp.Container()
			c.SetType(enumType)
			// 注册到导出类型
			b.GetProgram().SetExportType(enumName, enumType)
		}

		// 构建枚举列表，跟踪当前值以便自动递增
		b.buildEnumeratorList(e.(*cparser.EnumeratorListContext))
	} else if enumName != "" {
		// 情况 2: 'enum' Identifier - 引用已存在的枚举类型
		if bp := b.GetBluePrint(enumName); bp != nil {
			container := bp.Container()
			_ = container.GetType()
		} else {
			// 如果找不到，创建一个新的枚举类型
			enumType := ssa.CreateNumberType()
			bp := b.CreateBlueprintAndSetConstruct(enumName)
			c := bp.Container()
			c.SetType(enumType)
			b.GetProgram().SetExportType(enumName, enumType)
		}
	}
}

func (b *astbuilder) buildEnumeratorList(ast *cparser.EnumeratorListContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	// 跟踪当前枚举值，用于自动递增
	currentValue := int64(0)

	for _, e := range ast.AllEnumerator() {
		value := b.buildEnumerator(e.(*cparser.EnumeratorContext), currentValue)
		// 更新当前值：如果有显式值，使用该值；否则使用当前值
		if value != nil {
			if c, ok := ssa.ToConstInst(value); ok {
				if intVal, err := strconv.ParseInt(c.String(), 10, 64); err == nil {
					currentValue = intVal + 1
				} else {
					currentValue++
				}
			} else {
				currentValue++
			}
		} else {
			currentValue++
		}
	}
}

func (b *astbuilder) buildEnumerator(ast *cparser.EnumeratorContext, defaultValue int64) ssa.Value {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if id := ast.Identifier(); id != nil {
		enumName := id.GetText()
		var enumValue ssa.Value

		// 处理显式值或使用默认值
		if e := ast.Expression(); e != nil {
			// 有显式值
			enumValue, _ = b.buildExpression(e.(*cparser.ExpressionContext), false)
			if enumValue == nil {
				enumValue = b.EmitConstInst(defaultValue)
			}
		} else {
			// 没有显式值，使用默认值（自动递增）
			enumValue = b.EmitConstInst(defaultValue)
		}

		// 确保值是整数类型
		if enumValue != nil {
			enumValue.SetType(ssa.CreateNumberType())
			// 将枚举常量添加到特殊值中，使其可以作为常量使用
			b.addSpecialValue(enumName, enumValue)
		}

		return enumValue
	}

	return nil
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
		if v := b.buildStructDeclarator(s.(*cparser.StructDeclaratorContext)); !utils.IsNil(v) {
			ret = append(ret, v)
		}
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
	} else if mcs := ast.MacroCallStatement(); mcs != nil {
		// 处理宏调用语句（如 FF_DISABLE_DEPRECATION_WARNINGS）
		b.buildMacroCallStatement(mcs.(*cparser.MacroCallStatementContext))
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
		if l := ast.InitDeclaratorList(); l != nil {
			lefts, indexs := b.buildInitDeclaratorList(l.(*cparser.InitDeclaratorListContext))
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

	if a := ast.CoreExpressions(); a != nil {
		return b.buildCoreExpressions(a.(*cparser.CoreExpressionsContext))
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

func (b *astbuilder) buildCoreExpressions(ast *cparser.CoreExpressionsContext) ssa.Values {
	var ret ssa.Values
	for _, a := range ast.AllCoreExpression() {
		ret = append(ret, b.buildCoreExpression(a.(*cparser.CoreExpressionContext)))
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
