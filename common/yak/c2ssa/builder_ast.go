package c2ssa

import (
	"strconv"
	"strings"

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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	exportHandler := func() {
		// Types are already written to ExportType during prebuild (SetExportType).
		// Re-copying via GetStructAll/GetAliasAll is a no-op; only sync globals.
		lib := b.GetProgram()
		if lib.GlobalVariablesBlueprint == nil {
			return
		}
		container := lib.GlobalVariablesBlueprint.Container()
		if container == nil {
			return
		}
		for key, val := range container.GetAllMember() {
			lib.SetExportValue(key.String(), val)
		}
	}

	if b.PreHandler() {
		if unit := ast.TranslationUnit(); unit != nil {
			b.prebuildTranslationUnit(unit.(*cparser.TranslationUnitContext))
		}
		exportHandler()
		ssa.ReleaseASTRoot(ast)
	} else {
		if unit := ast.TranslationUnit(); unit != nil {
			b.buildTranslationUnit(unit.(*cparser.TranslationUnitContext))
		}
	}
}

func (b *astbuilder) prebuildTranslationUnit(ast *cparser.TranslationUnitContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	for _, e := range ast.AllExternalDeclaration() {
		b.prebuildExternalDeclaration(e.(*cparser.ExternalDeclarationContext))
	}
}

func (b *astbuilder) prebuildExternalDeclaration(ast *cparser.ExternalDeclarationContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if fod := ast.FunctionOrDeclaration(); fod != nil {
		b.buildFunctionOrDeclaration(fod.(*cparser.FunctionOrDeclarationContext))
		return
	}
	b.buildExternalDeclaration(ast)
}

func (b *astbuilder) buildTranslationUnit(ast *cparser.TranslationUnitContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	for _, e := range ast.AllExternalDeclaration() {
		b.buildExternalDeclaration(e.(*cparser.ExternalDeclarationContext))
	}
}

func (b *astbuilder) buildExternalDeclaration(ast *cparser.ExternalDeclarationContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if fod := ast.FunctionOrDeclaration(); fod != nil {
		b.buildFunctionOrDeclaration(fod.(*cparser.FunctionOrDeclarationContext))
	} else if e := ast.Expression(); e != nil {
		b.buildExpression(e.(*cparser.ExpressionContext), false)
	} else if ds := ast.DeclarationSpecifier(); ds != nil {
		b.buildDeclarationSpecifier(ds.(*cparser.DeclarationSpecifierContext))
	} else if mce := ast.MacroCallExpression(); mce != nil {
		b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
	} else if mcs := ast.MacroCallStatement(); mcs != nil {
		b.buildMacroCallStatement(mcs.(*cparser.MacroCallStatementContext))
	}
}

func (b *astbuilder) buildFunctionOrDeclaration(ast *cparser.FunctionOrDeclarationContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	rest, _ := ast.FunctionOrDeclarationRest().(*cparser.FunctionOrDeclarationRestContext)
	if rest != nil && rest.CompoundStatement() != nil {
		b.buildFunctionFromDeclarator(ast.DeclarationSpecifier(), ast.Declarator(), rest.DeclarationList(), rest.CompoundStatement())
		return
	}
	if ds := ast.DeclarationSpecifier(); ds != nil {
		ssatype := b.buildDeclarationSpecifier(ds.(*cparser.DeclarationSpecifierContext))
		if de := ast.Declarator(); de != nil {
			decl := de.(*cparser.DeclaratorContext)
			kind := VARIABLE_KIND
			if cIsFunctionPrototypeDeclarator(decl) {
				kind = FUNC_KIND
			}
			left, right, _ := b.buildDeclarator(decl, kind)
			if kind == FUNC_KIND {
				if fn, ok := ssa.ToFunction(right); ok {
					if name := fn.GetName(); name != "" {
						variable := b.CreateLocalVariable(name)
						b.AssignVariable(variable, fn)
					}
				}
			} else if left != nil && rest != nil {
				if e := rest.Initializer(); e != nil {
					initial := b.buildInitializer(e.(*cparser.InitializerContext), ssatype)
					b.AssignVariable(left, initial)
				} else if left.GetValue() == nil {
					b.AssignVariable(left, b.GetDefaultValue(ssatype))
				}
			} else if left != nil && left.GetValue() == nil {
				b.AssignVariable(left, b.GetDefaultValue(ssatype))
			}
			if rest != nil {
				for _, extra := range rest.AllInitDeclarator() {
					b.buildInitDeclarator(extra.(*cparser.InitDeclaratorContext), ssatype)
				}
			}
		}
	}
}

func (b *astbuilder) buildFunctionFromDeclarator(
	ds cparser.IDeclarationSpecifierContext,
	de cparser.IDeclaratorContext,
	dl cparser.IDeclarationListContext,
	body cparser.ICompoundStatementContext,
) {
	var retType ssa.Type
	var paramTypes ssa.Types
	if ds != nil {
		if spec, ok := ds.(*cparser.DeclarationSpecifierContext); ok {
			retType = b.buildDeclarationSpecifier(spec)
		}
	}
	decl, ok := de.(*cparser.DeclaratorContext)
	if !ok || decl == nil {
		return
	}
	_, base, _ := b.buildDeclarator(decl, FUNC_KIND)
	newFunc, isFn := ssa.ToFunction(base)
	if !isFn {
		return
	}
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
	capturedDecl := decl
	capturedDL := dl
	capturedBody := body
	log.Debugf("add function funcName = %s", funcName)
	newFunc.AddLazyBuilder(func() {
		log.Debugf("build function funcName = %s", funcName)
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
		_, _, paramTypes = b.buildDeclarator(capturedDecl, FUNC_KIND)
		handleFunctionType(b.Function)
		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}
		if capturedDL != nil {
			if list, ok := capturedDL.(*cparser.DeclarationListContext); ok {
				b.buildDeclarationList(list)
			}
		}
		if capturedBody != nil {
			if c, ok := capturedBody.(*cparser.CompoundStatementContext); ok {
				b.buildCompoundStatement(c)
			}
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	}, false)
}

func (b *astbuilder) buildFunctionDefinition(ast *cparser.FunctionDefinitionContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
	b.buildFunctionFromDeclarator(ast.DeclarationSpecifier(), ast.Declarator(), ast.DeclarationList(), ast.CompoundStatement())
}

func (b *astbuilder) buildDirectDeclarator(ast *cparser.DirectDeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	var ssatypes ssa.Types

	kind := NORMAL_KIND
	if len(kinds) > 0 {
		kind = kinds[0]
	}

	// '(' Identifier pointer directDeclarator ')' — unexpanded WINAPI *name
	if p := ast.Pointer(); p != nil {
		if inner := ast.DirectDeclarator(); inner != nil {
			variable, value, types := b.buildDirectDeclarator(inner.(*cparser.DirectDeclaratorContext), kind)
			b.applyPointerModifiers(p.(*cparser.PointerContext), value)
			for _, suffix := range ast.AllDeclaratorSuffix() {
				variable, value, types = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, types, kind)
			}
			return variable, value, types
		}
	}

	// directDeclarator: Identifier ('.' Identifier)* declaratorSuffix*
	if id := ast.Identifier(0); id != nil {
		// Identifier 鏈韩鍙槸涓€涓悕瀛楋紝涓嶆牴鎹?kind 鍒涘缓鍊?
		// 鏍规嵁 declaratorSuffix 鐨勭被鍨嬫潵鍐冲畾濡備綍鏋勫缓
		identifierName := id.GetText()
		var variable *ssa.Variable
		var value ssa.Value

		// 鍏堝鐞嗗熀纭€ Identifier锛屾牴鎹槸鍚︽湁 suffix 鏉ュ喅瀹氬垵濮嬪€?
		suffixes := ast.AllDeclaratorSuffix()
		if len(suffixes) == 0 {
			// Dotted field-name macros: `ev_.ev_io.ev_io_next`.
			identifierName = ast.GetText()
			return b.buildIdentifierDeclarator(identifierName, kind)
		}

		// 鏈?suffix锛屽厛鍒涘缓鍩虹鏍囪瘑绗︼紝鐒跺悗閫愪釜澶勭悊 suffix
		// 绗竴涓?suffix 鍐冲畾鍩虹绫诲瀷
		firstSuffix := suffixes[0].(*cparser.DeclaratorSuffixContext)
		if firstSuffix.ArraySuffix() != nil {
			// 鏁扮粍绫诲瀷锛氬厛鍒涘缓鍙橀噺鍚嶏紝鐒跺悗澶勭悊鏁扮粍缁村害
			if kind == VARIABLE_KIND {
				variable = b.CreateLocalVariable(identifierName)
			}
			value = b.EmitConstInst(identifierName)
		} else if firstSuffix.FunctionSuffix() != nil {
			// 鍑芥暟绫诲瀷锛氭牴鎹?kind 鍒涘缓鍑芥暟
			// 閲嶇敤 buildIdentifierDeclarator 鐨勯€昏緫锛屽彧鍙?value 閮ㄥ垎
			_, value, _ = b.buildIdentifierDeclarator(identifierName, kind)
			if kind == FUNC_KIND && b.PreHandler() {
				// Prebuild only needs the Function object. Parameter NewParam calls belong
				// in the lazy body (after PushFunction); skipping here avoids orphan params
				// on the file-level builder and a redundant parameter-list walk.
				return variable, value, nil
			}
		} else {
			// 鏈煡绫诲瀷锛屼娇鐢ㄩ粯璁ゅ鐞?
			value = b.EmitConstInst(identifierName)
		}

		// 閫愪釜澶勭悊 declaratorSuffix*
		for _, suffix := range suffixes {
			variable, value, ssatypes = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, ssatypes, PARAM_KIND)
		}
		return variable, value, ssatypes
	}

	// directDeclarator: macroCallExpression declaratorSuffix*
	if mce := ast.MacroCallExpression(); mce != nil {
		// 澶勭悊瀹忚皟鐢ㄨ〃杈惧紡锛堟殏鏃朵綔涓烘櫘閫氭爣璇嗙澶勭悊锛?
		value := b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		var variable *ssa.Variable
		if kind == VARIABLE_KIND {
			variable = b.CreateLocalVariable(value.GetName())
		}
		// 閫愪釜澶勭悊 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, ssatypes = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, ssatypes, kind)
		}
		return variable, value, ssatypes
	}

	// directDeclarator: '(' declarator ')' declaratorSuffix*
	if d := ast.Declarator(); d != nil {
		declCtx := d.(*cparser.DeclaratorContext)
		// 鍏堝鐞嗗唴閮ㄧ殑 directDeclarator
		var variable *ssa.Variable
		var value ssa.Value
		var types ssa.Types
		if innerDirect := declCtx.DirectDeclarator(); innerDirect != nil {
			variable, value, types = b.buildDirectDeclarator(innerDirect.(*cparser.DirectDeclaratorContext), kind)
		}
		// 鐒跺悗搴旂敤鎸囬拡淇グ绗︼紙濡傛灉鏈夛級
		if p := declCtx.Pointer(); p != nil {
			b.applyPointerModifiers(p.(*cparser.PointerContext), value)
		}
		// 澶勭悊 gccDeclaratorExtension
		for _, g := range declCtx.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		// 澶勭悊 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, types = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, types, kind)
		}
		return variable, value, types
	}

	// directDeclarator: Identifier ':' DigitSequence
	if id := ast.Identifier(0); id != nil && ast.DigitSequence() != nil {
		return nil, b.EmitConstInst("bitfield"), nil
	}

	// directDeclarator: vcSpecificModifer Identifier declaratorSuffix*
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Identifier(0) != nil {
		b.buildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		var value ssa.Value = b.EmitConstInst("vcSpecific")
		var variable *ssa.Variable
		// 閫愪釜澶勭悊 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, ssatypes = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, ssatypes, kind)
		}
		return variable, value, ssatypes
	}

	// directDeclarator: '(' vcSpecificModifer declarator ')' declaratorSuffix*
	if vcm := ast.VcSpecificModifer(); vcm != nil && ast.Declarator() != nil {
		b.buildVcSpecificModifer(vcm.(*cparser.VcSpecificModiferContext))
		declCtx := ast.Declarator().(*cparser.DeclaratorContext)
		// 鍏堝鐞嗗唴閮ㄧ殑 directDeclarator
		var variable *ssa.Variable
		var value ssa.Value
		var types ssa.Types
		if innerDirect := declCtx.DirectDeclarator(); innerDirect != nil {
			variable, value, types = b.buildDirectDeclarator(innerDirect.(*cparser.DirectDeclaratorContext), kind)
		}
		// 鐒跺悗搴旂敤鎸囬拡淇グ绗︼紙濡傛灉鏈夛級
		if p := declCtx.Pointer(); p != nil {
			b.applyPointerModifiers(p.(*cparser.PointerContext), value)
		}
		// 澶勭悊 gccDeclaratorExtension
		for _, g := range declCtx.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		// 澶勭悊 declaratorSuffix*
		for _, suffix := range ast.AllDeclaratorSuffix() {
			variable, value, types = b.buildDeclaratorSuffix(suffix.(*cparser.DeclaratorSuffixContext), variable, value, types, kind)
		}
		return variable, value, types
	}

	b.NewError(ssa.Error, TAG, Unreachable())
	return b.CreateVariable(""), b.EmitConstInst(0), nil
}

// buildIdentifierDeclarator 澶勭悊娌℃湁 suffix 鐨勭畝鍗曟爣璇嗙澹版槑
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

// buildDeclaratorSuffix 澶勭悊澹版槑绗﹀悗缂€锛堟暟缁勭淮搴︽垨鍑芥暟鍙傛暟锛?
func (b *astbuilder) buildDeclaratorSuffix(ast *cparser.DeclaratorSuffixContext, variable *ssa.Variable, value ssa.Value, ssatypes ssa.Types, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	// 澶勭悊鏁扮粍鍚庣紑: arraySuffix
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

	// 澶勭悊鍑芥暟鍚庣紑: functionSuffix
	if fs := ast.FunctionSuffix(); fs != nil {
		return b.buildFunctionSuffixDeclarator(fs.(*cparser.FunctionSuffixContext), variable, value, ssatypes, kind)
	}

	return variable, value, ssatypes
}

// buildFunctionSuffixDeclarator 澶勭悊鍑芥暟鍚庣紑澹版槑绗?
func (b *astbuilder) buildFunctionSuffixDeclarator(functionSuffix *cparser.FunctionSuffixContext, variable *ssa.Variable, value ssa.Value, ssatypes ssa.Types, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	// 鎻愬彇鍙傛暟绫诲瀷鍒楄〃锛堝鏋滄湁锛?
	if ptl := functionSuffix.ParameterTypeList(); ptl != nil {
		_, ssatypes = b.buildParameterTypeList(ptl.(*cparser.ParameterTypeListContext))
	} else if idl := functionSuffix.IdentifierList(); idl != nil {
		b.buildIdentifierList(idl.(*cparser.IdentifierListContext))
	}

	// 鏍规嵁 kind 鍐冲畾杩斿洖鍊?
	switch kind {
	case VARIABLE_KIND:
		// 鍑芥暟绫诲瀷鐨勫彉閲忥細浼樺厛浣跨敤 variable 鐨勫悕瀛楋紝濡傛灉娌℃湁鍒欎娇鐢?value 鐨勫悕瀛?
		var varName string
		if variable != nil {
			varName = variable.GetName()
		} else if value != nil {
			varName = value.GetName()
		}
		// 濡傛灉浠嶇劧娌℃湁鍚嶅瓧锛屼娇鐢ㄩ粯璁ゅ€?
		if varName == "" {
			varName = "unknown"
		}
		return b.CreateLocalVariable(varName), nil, nil
	case FUNC_KIND:
		// 鍑芥暟瀹氫箟锛氫繚鎸?value锛堝簲璇ユ槸鍑芥暟锛?
		if utils.IsNil(value) {
			value = b.EmitConstInst(0)
		}
		return variable, value, ssatypes
	case PARAM_KIND:
		// 鍙傛暟澹版槑锛氬凡缁忔彁鍙栦簡鍙傛暟绫诲瀷锛岃繑鍥?value 鍜岀被鍨?
		if utils.IsNil(value) {
			value = b.EmitConstInst(0)
		}
		return variable, value, ssatypes
	default:
		// 鍏朵粬鎯呭喌锛氫繚鎸佸師鍊?
		if utils.IsNil(value) {
			value = b.EmitConstInst(0)
		}
		return variable, value, ssatypes
	}
}

// buildMacroCallExpression 澶勭悊瀹忚皟鐢ㄨ〃杈惧紡
// macroCallExpression: Identifier '(' macroArgumentList? ')' postfixSuffix*
func (b *astbuilder) buildMacroCallExpression(ast *cparser.MacroCallExpressionContext) ssa.Value {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	var right ssa.Value

	// 鑾峰彇瀹忓悕绉?
	if id := ast.Identifier(); id != nil {
		macroName := id.GetText()
		// 澶勭悊瀹忓弬鏁板垪琛?
		var args ssa.Values
		if mal := ast.MacroArgumentList(); mal != nil {
			args = b.buildMacroArgumentList(mal.(*cparser.MacroArgumentListContext))
		}
		// 鏆傛椂浣滀负鍑芥暟璋冪敤澶勭悊锛堝鏋滃畯灞曞紑涓哄嚱鏁拌皟鐢級
		// 鎴栬€呬綔涓烘櫘閫氭爣璇嗙澶勭悊
		if len(args) > 0 {
			// 灏濊瘯浣滀负鍑芥暟璋冪敤澶勭悊
			if fun, ok := b.GetFunc(macroName, ""); ok {
				right = b.EmitCall(b.NewCall(fun, args))
			} else {
				// 鏃犳硶鎵惧埌鍑芥暟锛屼綔涓哄父閲忓鐞?
				right = b.EmitConstInst(macroName)
			}
		} else {
			right = b.EmitConstInst(macroName)
		}
	}

	// 澶勭悊 postfixSuffix*锛堝鏁扮粍涓嬫爣锛?
	for _, suffix := range ast.AllPostfixSuffix() {
		right, _ = b.buildPostfixSuffix(suffix.(*cparser.PostfixSuffixContext), right, nil, false)
	}

	if utils.IsNil(right) {
		right = b.EmitConstInst(0)
	}
	return right
}

// buildMacroArgumentList 澶勭悊瀹忓弬鏁板垪琛?
func (b *astbuilder) buildMacroArgumentList(ast *cparser.MacroArgumentListContext) ssa.Values {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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

// buildMacroCallStatement 澶勭悊瀹忚皟鐢ㄨ鍙?
// macroCallStatement: Identifier eos*
func (b *astbuilder) buildMacroCallStatement(ast *cparser.MacroCallStatementContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	// 瀹忚皟鐢ㄨ鍙ラ€氬父涓嶄骇鐢熷€硷紝鍙槸鐢ㄤ簬棰勫鐞嗘寚浠ゆ垨鍓綔鐢?
	// 杩欓噷鍙互璁板綍瀹忚皟鐢紝浣嗕笉鍋氬疄闄呭鐞?
	if id := ast.Identifier(); id != nil {
		_ = id.GetText()
		// 鍙互鍦ㄨ繖閲屾坊鍔犲畯灞曞紑閫昏緫锛堝鏋滈渶瑕侊級
	}
}

func (b *astbuilder) buildMacroIterationStatement(ast *cparser.MacroIterationStatementContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if mal := ast.MacroArgumentList(); mal != nil {
		_ = b.buildMacroArgumentList(mal.(*cparser.MacroArgumentListContext))
	}
	if c := ast.CompoundStatement(); c != nil {
		b.buildCompoundStatement(c.(*cparser.CompoundStatementContext), true)
		return
	}
	if s := ast.SelectionStatement(); s != nil {
		b.buildSelectionStatement(s.(*cparser.SelectionStatementContext))
		return
	}
	if i := ast.IterationStatement(); i != nil {
		b.buildIterationStatement(i.(*cparser.IterationStatementContext))
		return
	}
	if j := ast.JumpStatement(); j != nil {
		b.buildJumpStatement(j.(*cparser.JumpStatementContext))
		return
	}
	if e := ast.ExpressionStatement(); e != nil {
		b.buildExpressionStatement(e.(*cparser.ExpressionStatementContext))
		return
	}
	if nested := ast.MacroIterationStatement(); nested != nil {
		b.buildMacroIterationStatement(nested.(*cparser.MacroIterationStatementContext))
	}
}

// applyPointerModifiers 搴旂敤鎸囬拡淇グ绗﹀埌绫诲瀷涓?
func (b *astbuilder) applyPointerModifiers(pointer *cparser.PointerContext, value ssa.Value) {
	if pointer == nil || value == nil {
		return
	}

	pointerParts := pointer.AllPointerPart()
	// 瀵规瘡涓?pointerPart锛屽簲鐢ㄦ寚閽堢被鍨?
	for _, part := range pointerParts {
		// 澶勭悊绫诲瀷闄愬畾绗︼紙濡傛灉闇€瑕侊級
		if tql := part.(*cparser.PointerPartContext).TypeQualifierList(); tql != nil {
			b.buildTypeQualifierList(tql.(*cparser.TypeQualifierListContext))
		}
		// 搴旂敤鎸囬拡绫诲瀷鍒?value 鐨勭被鍨嬩笂
		currentType := value.GetType()
		if currentType != nil {
			pointerType := ssa.NewPointerType()
			// 濡傛灉褰撳墠绫诲瀷宸茬粡鏄寚閽堬紝鍒涘缓澶氱骇鎸囬拡
			if currentType.GetTypeKind() == ssa.PointerKind {
				// 澶氱骇鎸囬拡锛氫繚鎸佷负鎸囬拡绫诲瀷
				value.SetType(pointerType)
			} else {
				// 鍗曠骇鎸囬拡锛氬皢鍩虹绫诲瀷鍖呰涓烘寚閽?
				pointerType.FieldType = currentType
				value.SetType(pointerType)
			}
		} else {
			// 濡傛灉娌℃湁绫诲瀷锛屽垱寤烘寚閽堢被鍨?
			value.SetType(ssa.NewPointerType())
		}
	}
}

// buildDeclaratorCore 澶勭悊澹版槑绗︾殑鏍稿績閫昏緫锛堜笉鍚寚閽堜慨楗板拰鎵╁睍锛?
func (b *astbuilder) buildDeclaratorCore(directDeclarator *cparser.DirectDeclaratorContext, kind ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	if directDeclarator == nil {
		return nil, nil, nil
	}
	return b.buildDirectDeclarator(directDeclarator, kind)
}

// buildDeclarator 澶勭悊瀹屾暣鐨勫０鏄庣锛歱ointer? directDeclarator gccDeclaratorExtension*
func (b *astbuilder) buildDeclarator(ast *cparser.DeclaratorContext, kinds ...ConstKind) (*ssa.Variable, ssa.Value, ssa.Types) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	// 鍏堝鐞?directDeclarator锛堟牳蹇冮€昏緫锛?
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
		// Do not assign an Undefined PointerKind placeholder. Undefined is not
		// a pointer object, so later *p hits ObjectError (@pointer on type {})
		// and never writes through to the origin. Uninitialized `int *p;`
		// stays valueless; `p = &x` then supplies a real pointer object.
		b.applyPointerModifiers(p.(*cparser.PointerContext), value)
	}

	for _, g := range ast.AllGccDeclaratorExtension() {
		b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
	}

	return variable, value, types
}

func (b *astbuilder) buildVcSpecificModifer(ast *cparser.VcSpecificModiferContext) ssa.Value {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	return b.EmitConstInst(0)
}

func (b *astbuilder) buildTypeQualifierList(ast *cparser.TypeQualifierListContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildIdentifierList(ast *cparser.IdentifierListContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildParameterTypeList(ast *cparser.ParameterTypeListContext) (ssa.Values, ssa.Types) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if p := ast.ParameterList(); p != nil {
		return b.buildParameterList(p.(*cparser.ParameterListContext))
	}
	return ssa.Values{}, ssa.Types{}
}

func (b *astbuilder) buildParameterList(ast *cparser.ParameterListContext) (ssa.Values, ssa.Types) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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

// buildAbstractDeclarator 搴旂敤鎶借薄澹版槑绗﹀埌鍩虹绫诲瀷涓?
// abstractDeclarator 鐢ㄤ簬鎻忚堪绫诲瀷鑰屼笉鍖呭惈鍙橀噺鍚嶏紙濡?int *, int (*)(), int [10] 绛夛級
func (b *astbuilder) buildAbstractDeclarator(ast *cparser.AbstractDeclaratorContext, baseType ssa.Type) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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

	// 澶勭悊 directAbstractDeclarator锛堟暟缁勩€佸嚱鏁扮瓑淇グ锛?
	if d := ast.DirectAbstractDeclarator(); d != nil {
		resultType = b.buildDirectAbstractDeclarator(d.(*cparser.DirectAbstractDeclaratorContext), resultType)
	}

	// 澶勭悊 gccDeclaratorExtension
	for _, g := range ast.AllGccDeclaratorExtension() {
		b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
	}

	return resultType
}

// buildDirectAbstractDeclarator 澶勭悊鐩存帴鎶借薄澹版槑绗︼紙鏁扮粍銆佸嚱鏁扮瓑锛?
func (b *astbuilder) buildDirectAbstractDeclarator(ast *cparser.DirectAbstractDeclaratorContext, baseType ssa.Type) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	resultType := baseType

	// 1. '(' abstractDeclarator ')' abstractDeclaratorSuffix*
	//    or '(' NTAPI '*' ')' for unexpanded calling-convention pointers.
	if ast.LeftParen() != nil && ast.RightParen() != nil {
		if a := ast.AbstractDeclarator(); a != nil {
			resultType = b.buildAbstractDeclarator(a.(*cparser.AbstractDeclaratorContext), resultType)
		} else if p := ast.Pointer(); p != nil {
			for range p.(*cparser.PointerContext).AllPointerPart() {
				pointerType := ssa.NewPointerType()
				pointerType.FieldType = resultType
				resultType = pointerType
			}
		}
		// 澶勭悊 abstractDeclaratorSuffix*
		for _, suffix := range ast.AllAbstractDeclaratorSuffix() {
			resultType = b.buildAbstractDeclaratorSuffix(suffix.(*cparser.AbstractDeclaratorSuffixContext), resultType)
		}
		// 澶勭悊 gccDeclaratorExtension
		for _, g := range ast.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		return resultType
	}

	// 2. abstractDeclaratorSuffix+ - 蹇呴』浠ヨ嚦灏戜竴涓悗缂€寮€濮?
	suffixes := ast.AllAbstractDeclaratorSuffix()
	if len(suffixes) > 0 {
		for _, suffix := range suffixes {
			resultType = b.buildAbstractDeclaratorSuffix(suffix.(*cparser.AbstractDeclaratorSuffixContext), resultType)
		}
		// 澶勭悊 gccDeclaratorExtension
		for _, g := range ast.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		return resultType
	}

	return resultType
}

// buildAbstractDeclaratorSuffix 澶勭悊鎶借薄澹版槑绗﹀悗缂€锛堟暟缁勭淮搴︽垨鍑芥暟鍙傛暟锛?
func (b *astbuilder) buildAbstractDeclaratorSuffix(ast *cparser.AbstractDeclaratorSuffixContext, baseType ssa.Type) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	resultType := baseType

	// 澶勭悊鏁扮粍鍚庣紑: abstractArraySuffix
	if aas := ast.AbstractArraySuffix(); aas != nil {
		abstractArraySuffix := aas.(*cparser.AbstractArraySuffixContext)
		var arraySize int64 = -1 // -1 琛ㄧず鏈寚瀹氬ぇ灏忔垨鍙彉闀垮害鏁扮粍

		// 澶勭悊鏁扮粍澶у皬琛ㄨ揪寮?
		if c := abstractArraySuffix.CoreExpression(); c != nil {
			expr := b.buildCoreExpression(c.(*cparser.CoreExpressionContext))
			if c, ok := ssa.ToConstInst(expr); ok {
				if size, err := strconv.ParseInt(c.String(), 10, 64); err == nil {
					arraySize = size
				}
			}
		} else if abstractArraySuffix.Star() != nil {
			// '[' '*' ']' - 鍙彉闀垮害鏁扮粍
			arraySize = -1
		}

		// 鍒涘缓鏁扮粍绫诲瀷锛堝湪 SSA 涓紝鏁扮粍閫氬父琛ㄧず涓?SliceType锛?
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

	// 澶勭悊鍑芥暟鍚庣紑: abstractFunctionSuffix
	if afs := ast.AbstractFunctionSuffix(); afs != nil {
		abstractFunctionSuffix := afs.(*cparser.AbstractFunctionSuffixContext)
		var paramTypes ssa.Types
		if ptl := abstractFunctionSuffix.ParameterTypeList(); ptl != nil {
			_, paramTypes = b.buildParameterTypeList(ptl.(*cparser.ParameterTypeListContext))
		}
		funcType := ssa.NewFunctionType("", paramTypes, resultType, false)
		resultType = funcType
		// 澶勭悊 gccDeclaratorExtension
		for _, g := range abstractFunctionSuffix.AllGccDeclaratorExtension() {
			b.buildGccDeclaratorExtension(g.(*cparser.GccDeclaratorExtensionContext))
		}
		return resultType
	}

	return resultType
}

func (b *astbuilder) buildGccDeclaratorExtension(ast *cparser.GccDeclaratorExtensionContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildDeclaration(ast *cparser.DeclarationContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
		// 澶勭悊瀹忚皟鐢ㄤ綔涓哄０鏄庯紝濡?DECLARE_ALIGNED(...)[8] = {...}
		_ = b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		// 澶勭悊鍙兘鐨?declaratorSuffix*锛堝鏁扮粍涓嬫爣锛?
		for _, suffix := range ast.AllDeclaratorSuffix() {
			// 杩欓噷闇€瑕佸鐞嗗悗缂€锛屼絾閫氬父瀹忚皟鐢ㄤ綔涓哄０鏄庢椂锛屽悗缂€宸茬粡鍦?macroCallExpression 涓鐞嗕簡
			_ = suffix
		}
		// 澶勭悊鍒濆鍖栧櫒锛堝鏋滄湁锛?
		if init := ast.Initializer(); init != nil {
			_ = b.buildInitializer(init.(*cparser.InitializerContext))
			// 鍙互灏嗗垵濮嬪€艰祴缁欏彉閲忥紙濡傛灉闇€瑕侊級
		}
	} else if s := ast.StaticAssertDeclaration(); s != nil {
		b.buildStaticAssertDeclaration(s.(*cparser.StaticAssertDeclarationContext))
	}
}

func (b *astbuilder) buildStaticAssertDeclaration(ast *cparser.StaticAssertDeclarationContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if c := ast.Expression(); c != nil {
		right, _ := b.buildExpression(c.(*cparser.ExpressionContext), false)
		_ = right
	}
}

func (b *astbuilder) buildInitDeclaratorList(ast *cparser.InitDeclaratorListContext, ssatype ...ssa.Type) ([]*ssa.Variable, []int) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if d := ast.Declarator(); d != nil {
		decl := d.(*cparser.DeclaratorContext)
		kind := VARIABLE_KIND
		if cIsFunctionPrototypeDeclarator(decl) {
			kind = FUNC_KIND
		}
		left, right, _ := b.buildDeclarator(decl, kind)
		if kind == FUNC_KIND {
			if fn, ok := ssa.ToFunction(right); ok {
				if name := fn.GetName(); name != "" {
					variable := b.CreateLocalVariable(name)
					b.AssignVariable(variable, fn)
					return variable, -1
				}
			}
			return left, -1
		}
		// Only the declarator may contain '*'. Do NOT scan ast.GetText():
		// `int x = *p` would otherwise be treated as a pointer init.
		isPtr := decl.Pointer() != nil || strings.Contains(decl.GetText(), "*")
		if e := ast.Initializer(); e != nil {
			initial := b.buildInitializer(e.(*cparser.InitializerContext), ssatype...)
			// Only wrap NULL/0. Wrapping strings, functions, or aggregates
			// as PointerKind breaks char*, function pointers, and int *arr[].
			if isPtr && cIsNullishConst(initial) {
				initial = b.ensurePointerValue(initial)
			}
			b.AssignVariable(left, initial)
			return left, -1
		}
		if right != nil {
			index, _ := strconv.Atoi(right.String())
			return left, index
		}
		return left, -1
	}
	if inner := ast.DirectDeclarator(); inner != nil {
		left, right, _ := b.buildDirectDeclarator(inner.(*cparser.DirectDeclaratorContext), VARIABLE_KIND)
		isPtr := ast.Pointer() != nil
		if p := ast.Pointer(); p != nil {
			b.applyPointerModifiers(p.(*cparser.PointerContext), right)
		}
		if e := ast.Initializer(); e != nil {
			initial := b.buildInitializer(e.(*cparser.InitializerContext), ssatype...)
			if isPtr && cIsNullishConst(initial) {
				initial = b.ensurePointerValue(initial)
			}
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

func cIsFunctionPrototypeDeclarator(decl *cparser.DeclaratorContext) bool {
	if decl == nil {
		return false
	}
	dd, ok := decl.DirectDeclarator().(*cparser.DirectDeclaratorContext)
	if !ok || dd == nil || dd.Identifier(0) == nil {
		return false
	}
	for _, suffix := range dd.AllDeclaratorSuffix() {
		s, ok := suffix.(*cparser.DeclaratorSuffixContext)
		if ok && s.FunctionSuffix() != nil {
			return true
		}
	}
	return false
}

func (b *astbuilder) buildDeclarationSpecifiers(ast *cparser.DeclarationSpecifiersContext) ssa.Types {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
	var rets ssa.Types

	for _, d := range ast.AllDeclarationSpecifier() {
		rets = append(rets, b.buildDeclarationSpecifier(d.(*cparser.DeclarationSpecifierContext)))
	}
	return rets
}

func (b *astbuilder) buildDeclarationSpecifier(ast *cparser.DeclarationSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Type

	// log.Infof("exp = %s\n", ast.GetText())

	// 澶勭悊 alignmentSpecifier (浼樺厛绾ф渶楂橈紝濡傛灉瀛樺湪鍒欑洿鎺ヨ繑鍥?
	if a := ast.AlignmentSpecifier(); a != nil {
		return b.buildAlignmentSpecifier(a.(*cparser.AlignmentSpecifierContext))
	}

	// 澶勭悊 storageClassSpecifier (瀛樺偍绫昏鏄庣锛屽 static, extern 绛?
	for _, s := range ast.AllStorageClassSpecifier() {
		_ = b.buildStorageClassSpecifier(s.(*cparser.StorageClassSpecifierContext))
		// 瀛樺偍绫昏鏄庣涓嶅奖鍝嶇被鍨嬶紝鍙奖鍝嶅瓨鍌ㄦ柟寮?
	}

	// 澶勭悊 typeQualifier (绫诲瀷闄愬畾绗︼紝濡?const, volatile 绛?
	for _, tq := range ast.AllTypeQualifier() {
		_ = b.buildTypeQualifier(tq.(*cparser.TypeQualifierContext))
		// 绫诲瀷闄愬畾绗︿笉褰卞搷鍩虹绫诲瀷锛屽彧褰卞搷绫诲瀷灞炴€?
	}

	// 澶勭悊 functionSpecifier (鍑芥暟璇存槑绗︼紝濡?inline 绛?
	for _, f := range ast.AllFunctionSpecifier() {
		_ = b.buildFunctionSpecifier(f.(*cparser.FunctionSpecifierContext))
		// 鍑芥暟璇存槑绗︿笉褰卞搷绫诲瀷
	}

	// 澶勭悊 typeSpecifier 鎴?Identifier
	if ts := ast.TypeSpecifier(); ts != nil {
		ret = b.buildTypeSpecifier(ts.(*cparser.TypeSpecifierContext))
	} else if id := ast.Identifier(); id != nil {
		// Identifier may be a typedef name when no typeSpecifier is present.
		name := id.GetText()
		if bp := b.GetBluePrint(name); bp != nil {
			container := bp.Container()
			ret = container.GetType()
		} else {
			// 濡傛灉鎵句笉鍒?typedef锛屽皾璇曚粠鐗规畩绫诲瀷涓煡鎵?
			if ssatyp := ssa.GetTypeByStr(name); ssatyp != nil {
				ret = ssatyp
			} else {
				ret = ssa.CreateAnyType()
			}
		}
	} else if mce := ast.MacroCallExpression(); mce != nil {
		_ = b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		ret = ssa.CreateAnyType()
	}

	if ret == nil {
		ret = ssa.CreateAnyType()
	}
	return ret
}

func (b *astbuilder) buildAlignmentSpecifier(ast *cparser.AlignmentSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if t := ast.TypeName(); t != nil {
		return b.buildTypeName(t.(*cparser.TypeNameContext))
	}
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeNameByValue(ast *cparser.TypeNameContext) ssa.Value {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if s := ast.SpecifierQualifierList(); s != nil {
		ssatype := b.buildSpecifierQualifierList(s.(*cparser.SpecifierQualifierListContext))
		if a := ast.AbstractDeclarator(); a != nil {
			// 搴旂敤鎶借薄澹版槑绗﹀埌鍩虹绫诲瀷锛堝 int * -> 鎸囬拡绫诲瀷锛?
			ssatype = b.buildAbstractDeclarator(a.(*cparser.AbstractDeclaratorContext), ssatype)
		}
		return ssatype
	}

	// if t := ast.TypeName(); t != nil {
	// }

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildFunctionSpecifier(ast *cparser.FunctionSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeQualifier(ast *cparser.TypeQualifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildStorageClassSpecifier(ast *cparser.StorageClassSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	return ssa.CreateAnyType()
}

func (b *astbuilder) buildTypeSpecifier(ast *cparser.TypeSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	// 澶勭悊鏋氫妇鏍囪瘑绗︼紙濡傛灉鏈夛級
	var enumName string
	if id := ast.Identifier(); id != nil {
		enumName = id.GetText()
	}

	// 澶勭悊鏋氫妇鍒楄〃
	if e := ast.EnumeratorList(); e != nil {
		// 鍒涘缓鏋氫妇绫诲瀷锛堝湪 C 涓紝鏋氫妇绫诲瀷閫氬父琚涓?int 绫诲瀷锛?
		enumType := ssa.CreateNumberType()

		// 濡傛灉鏈夋灇涓惧悕绉帮紝鍒涘缓绫诲瀷钃濆浘骞舵敞鍐?
		if enumName != "" {
			bp := b.CreateBlueprintAndSetConstruct(enumName)
			c := bp.Container()
			c.SetType(enumType)
			// 娉ㄥ唽鍒板鍑虹被鍨?
			b.GetProgram().SetExportType(enumName, enumType)
		}

		// 鏋勫缓鏋氫妇鍒楄〃锛岃窡韪綋鍓嶅€间互渚胯嚜鍔ㄩ€掑
		b.buildEnumeratorList(e.(*cparser.EnumeratorListContext))
	} else if enumName != "" {
		// 鎯呭喌 2: 'enum' Identifier - 寮曠敤宸插瓨鍦ㄧ殑鏋氫妇绫诲瀷
		if bp := b.GetBluePrint(enumName); bp != nil {
			container := bp.Container()
			_ = container.GetType()
		} else {
			// 濡傛灉鎵句笉鍒帮紝鍒涘缓涓€涓柊鐨勬灇涓剧被鍨?
			enumType := ssa.CreateNumberType()
			bp := b.CreateBlueprintAndSetConstruct(enumName)
			c := bp.Container()
			c.SetType(enumType)
			b.GetProgram().SetExportType(enumName, enumType)
		}
	}
}

func (b *astbuilder) buildEnumeratorList(ast *cparser.EnumeratorListContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	// 璺熻釜褰撳墠鏋氫妇鍊硷紝鐢ㄤ簬鑷姩閫掑
	currentValue := int64(0)

	for _, e := range ast.AllEnumerator() {
		value := b.buildEnumerator(e.(*cparser.EnumeratorContext), currentValue)
		// 鏇存柊褰撳墠鍊硷細濡傛灉鏈夋樉寮忓€硷紝浣跨敤璇ュ€硷紱鍚﹀垯浣跨敤褰撳墠鍊?
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	finish := func(name string, enumValue ssa.Value) ssa.Value {
		if enumValue == nil {
			enumValue = b.EmitConstInst(defaultValue)
		}
		enumValue.SetType(ssa.CreateNumberType())
		if name != "" {
			b.addSpecialValue(name, enumValue)
		}
		return enumValue
	}

	if id := ast.Identifier(); id != nil {
		var enumValue ssa.Value
		if e := ast.Expression(0); e != nil {
			enumValue, _ = b.buildExpression(e.(*cparser.ExpressionContext), false)
		}
		return finish(id.GetText(), enumValue)
	}

	// Parenthesized enumerator names from expanded macros: `(1U << 0) = (int)(1U << 0)`
	exprs := ast.AllExpression()
	if len(exprs) >= 2 {
		enumValue, _ := b.buildExpression(exprs[1].(*cparser.ExpressionContext), false)
		return finish("", enumValue)
	}
	if len(exprs) == 1 {
		enumValue, _ := b.buildExpression(exprs[0].(*cparser.ExpressionContext), false)
		return finish("", enumValue)
	}
	return finish("", nil)
}

func (b *astbuilder) buildStructOrUnionSpecifier(ast *cparser.StructOrUnionSpecifierContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if id := ast.Identifier(); id != nil {
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	for _, s := range ast.AllStructDeclaration() {
		b.buildStructDeclaration(s.(*cparser.StructDeclarationContext), structTyp)
	}
}

func (b *astbuilder) buildStructDeclaration(ast *cparser.StructDeclarationContext, structTyp *ssa.ObjectType) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if sq := ast.SpecifierQualifierList(); sq != nil {
		ssatype := b.buildSpecifierQualifierList(sq.(*cparser.SpecifierQualifierListContext))
		if sd := ast.StructDeclaratorList(); sd != nil {
			for _, s := range sd.AllStructDeclarator() {
				sc := s.(*cparser.StructDeclaratorContext)
				l := b.buildStructDeclarator(sc)
				if utils.IsNil(l) {
					continue
				}
				fieldType := ssatype
				if d := sc.Declarator(); d != nil {
					if decl := d.(*cparser.DeclaratorContext); decl.Pointer() != nil {
						pt := ssa.NewPointerType()
						pt.SetName("Pointer")
						pt.FieldType = ssatype
						fieldType = pt
					}
				}
				structTyp.AddField(b.EmitConstInst(l.GetName()), fieldType)
			}
		}
	} else if sa := ast.StaticAssertDeclaration(); sa != nil {
		b.buildStaticAssertDeclaration(sa.(*cparser.StaticAssertDeclarationContext))
	} else if mce := ast.MacroCallExpression(); mce != nil {
		_ = b.buildMacroCallExpression(mce.(*cparser.MacroCallExpressionContext))
		if sd := ast.StructDeclaratorList(); sd != nil {
			fieldType := ssa.CreateAnyType()
			for _, s := range sd.AllStructDeclarator() {
				sc := s.(*cparser.StructDeclaratorContext)
				l := b.buildStructDeclarator(sc)
				if utils.IsNil(l) {
					continue
				}
				structTyp.AddField(b.EmitConstInst(l.GetName()), fieldType)
			}
		}
	}
}

func (b *astbuilder) buildSpecifierQualifierList(ast *cparser.SpecifierQualifierListContext) ssa.Type {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	for _, d := range ast.AllDeclaration() {
		b.buildDeclaration(d.(*cparser.DeclarationContext))
	}
}

func (b *astbuilder) buildCompoundStatement(ast *cparser.CompoundStatementContext, isBlock ...bool) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	for _, item := range ast.AllBlockItem() {
		b.buildBlockItem(item.(*cparser.BlockItemContext))
	}
}

func (b *astbuilder) buildBlockItem(ast *cparser.BlockItemContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	// Prefer declaration (matches CParser blockItem order after grammar fix).
	if d := ast.Declaration(); d != nil {
		b.buildDeclaration(d.(*cparser.DeclarationContext))
	} else if e := ast.Expression(); e != nil {
		b.buildExpression(e.(*cparser.ExpressionContext), false)
	} else if s := ast.Statement(); s != nil {
		b.buildStatement(s.(*cparser.StatementContext))
	}
}

func (b *astbuilder) buildStatement(ast *cparser.StatementContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.ExpressionStatement(); e != nil {
		b.buildExpressionStatement(e.(*cparser.ExpressionStatementContext))
	} else if e := ast.Expression(); e != nil {
		b.buildExpression(e.(*cparser.ExpressionContext), false)
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
		// 澶勭悊瀹忚皟鐢ㄨ鍙ワ紙濡?FF_DISABLE_DEPRECATION_WARNINGS锛?
		b.buildMacroCallStatement(mcs.(*cparser.MacroCallStatementContext))
	} else if mi := ast.MacroIterationStatement(); mi != nil {
		b.buildMacroIterationStatement(mi.(*cparser.MacroIterationStatementContext))
	} else if id := ast.Identifier(); id != nil {
		b.buildLabeledStatement(ast, id.GetText())
	}
}

func (b *astbuilder) buildLabeledStatement(ast *cparser.StatementContext, text string) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	return nil
}

func (b *astbuilder) buildAsmStatement(ast *cparser.AsmStatementContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildIterationStatement(ast *cparser.IterationStatementContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	loop := b.CreateLoopBuilder()
	if e := ast.Expression(); e != nil {
		var condition ssa.Value
		cond := e
		loop.SetCondition(func() ssa.Value {
			if utils.IsNil(cond) {
				condition = b.EmitConstInst(true)
			} else {
				// recoverRange := b.SetRange(&cond.BaseParserRuleContext)
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
		if init, ok := condition.ForInitClause().(*cparser.ForInitClauseContext); ok {
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(&init.BaseParserRuleContext)
				defer recoverRange()
				return b.buildForInitClause(init)
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
					// recoverRange := b.SetRange(&cond.BaseParserRuleContext)
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
				recoverRange := b.SetRange(&third.BaseParserRuleContext)
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

func (b *astbuilder) buildForInitClause(ast *cparser.ForInitClauseContext) ssa.Values {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Values
	for _, item := range ast.AllForInitItem() {
		ret = append(ret, b.buildForInitItem(item.(*cparser.ForInitItemContext))...)
	}
	return ret
}

func (b *astbuilder) buildForInitItem(ast *cparser.ForInitItemContext) ssa.Values {
	if d := ast.ForDeclaration(); d != nil {
		return b.buildForDeclaration(d.(*cparser.ForDeclarationContext))
	}
	if c := ast.CastExpression(); c != nil {
		castCtx := c.(*cparser.CastExpressionContext)
		if op := ast.AssignmentOperator(); op != nil && ast.Expression() != nil {
			return ssa.Values{b.applyAssignmentFromCast(castCtx, op.(*cparser.AssignmentOperatorContext), ast.Expression().(*cparser.ExpressionContext))}
		}
		val, _ := b.buildCastExpression(castCtx, false)
		if utils.IsNil(val) {
			val = b.EmitConstInst(0)
		}
		return ssa.Values{val}
	}
	return nil
}

func (b *astbuilder) buildForDeclarations(ast *cparser.ForDeclarationsContext) ssa.Values {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()
	var ret ssa.Values

	for _, f := range ast.AllForDeclaration() {
		ret = append(ret, b.buildForDeclaration(f.(*cparser.ForDeclarationContext))...)
	}
	return ret
}

func (b *astbuilder) buildForDeclaration(ast *cparser.ForDeclarationContext) ssa.Values {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	var ret ssa.Values
	for _, e := range ast.AllExpression() {
		value, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
		ret = append(ret, value)
	}
	return ret
}

func (b *astbuilder) buildJumpStatement(ast *cparser.JumpStatementContext) {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
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
					return b.ensureValue(right)
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
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if ast.If() != nil {
		b.buildIfStatement(ast)
	} else if ast.Switch() != nil {
		b.buildSwitchStatement(ast)
	}
}

func (b *astbuilder) buildExpressionStatement(ast *cparser.ExpressionStatementContext) ssa.Values {
	recoverRange := b.SetRange(&ast.BaseParserRuleContext)
	defer recoverRange()

	if e := ast.Expression(); e != nil && ast.CoreExpressions() == nil {
		right, _ := b.buildExpression(e.(*cparser.ExpressionContext), false)
		return ssa.Values{right}
	}
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
