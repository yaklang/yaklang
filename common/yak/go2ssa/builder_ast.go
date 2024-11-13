package go2ssa

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// entry point
func (b *astbuilder) build(ast *gol.SourceFileContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	var pkgNameCurrent string

	var exportHandler = func() {
		lib := b.GetProgram()
		for structName, structType := range b.GetStructAll() {
			lib.SetExportType(structName, structType)
		}
		for aliasName, aliasType := range b.GetAliasAll() {
			lib.SetExportType(aliasName, aliasType)
		}
		for funcName, funcValue := range b.GetProgram().Funcs {
			if !funcValue.IsMethod() && funcValue.GetName() != "@init" {
				lib.SetExportValue(funcName, funcValue)
			}
		}
		for globalName, globalValue := range b.GetGlobalVariables() {
			lib.SetExportValue(globalName, globalValue)
		}
	}
	_ = exportHandler

	if b.PreHandler() {
		if packag, ok := ast.PackageClause().(*gol.PackageClauseContext); ok {
			pkgPath := b.buildPackage(packag)
			if b.GetProgram().ExtraFile["go.mod"] != "" {
				pkgNameCurrent = b.GetProgram().ExtraFile["go.mod"] + "/" + pkgPath[0]
			} else {
				pkgNameCurrent = pkgPath[0]
			}
			prog := b.GetProgram()
			application := prog.Application
			global := application.GlobalScope

			initHandler := func(name string) {
				variable := b.CreateMemberCallVariable(global, b.EmitConstInst(name))
				emptyContainer := b.EmitEmptyContainer()
				b.AssignVariable(variable, emptyContainer)
			}
			initHandler(pkgNameCurrent)

			b.pkgNameCurrent = pkgNameCurrent

			lib, skip := prog.GetLibrary(pkgPath[0])
			if skip {
				return
			}
			if lib == nil {
				lib = prog.NewLibrary(pkgPath[0], pkgPath)
			}
			lib.PushEditor(prog.GetCurrentEditor())
			lib.GlobalScope = b.ReadMemberCallValue(global, b.EmitConstInst(pkgNameCurrent))

			init := lib.GetAndCreateFunction(pkgNameCurrent, "@init")
			init.SetType(ssa.NewFunctionType("", []ssa.Type{ssa.CreateAnyType()}, ssa.CreateAnyType(), false))
			builder := lib.GetAndCreateFunctionBuilder(pkgNameCurrent, "@init")

			if builder != nil {
				builder.SetBuildSupport(b.FunctionBuilder)
				builder.SetEditor(prog.GetApplication().GetCurrentEditor())
				currentBuilder := b.FunctionBuilder
				b.FunctionBuilder = builder
				defer func() {
					for _, e := range builder.GetProgram().GetErrors() {
						currentBuilder.GetProgram().AddError(e)
					}
					b.FunctionBuilder = currentBuilder
				}()
			}
		}

		for _, decl := range ast.AllDeclaration() {
			if decl, ok := decl.(*gol.DeclarationContext); ok {
				b.buildDeclaration(decl, true)
			}
		}

		for _, meth := range ast.AllMethodDecl() {
			if meth, ok := meth.(*gol.MethodDeclContext); ok {
				b.buildMethodDeclFront(meth)
			}
		}

		for _, fun := range ast.AllFunctionDecl() {
			if fun, ok := fun.(*gol.FunctionDeclContext); ok {
				b.buildFunctionDeclFront(fun)
			}
		}

		for _, impo := range ast.AllImportDecl() {
			names, paths := b.buildImportDecl(impo.(*gol.ImportDeclContext))

			for i, name := range names {
				pathl := strings.Split(paths[i], "/")
				b.SetImportPackage(name, pathl[len(pathl)-1], paths[i])
				if lib, _ := b.GetImportPackage(name); lib != nil {
					b.GetProgram().ImportAll(lib)
				}
			}
		}

		exportHandler()
	} else {
		if packag, ok := ast.PackageClause().(*gol.PackageClauseContext); ok {
			pkgPath := b.buildPackage(packag)
			prog := b.GetProgram()
			lib, _ := prog.GetLibrary(pkgPath[0])

			if lib == nil {
				b.NewError(ssa.Error, TAG, "no library found for package %s", pkgPath[0])
			}

			builder := lib.GetAndCreateFunctionBuilder(pkgNameCurrent, "@init")

			if builder != nil {
				builder.SetBuildSupport(b.FunctionBuilder)
				builder.SetEditor(prog.GetApplication().GetCurrentEditor())
				currentBuilder := b.FunctionBuilder
				b.FunctionBuilder = builder
				defer func() {
					for _, e := range builder.GetProgram().GetErrors() {
						currentBuilder.GetProgram().AddError(e)
					}
					b.FunctionBuilder = currentBuilder
				}()
			}
		}

		for _, f := range b.GetProgram().Funcs {
			f.Build()
		}
	}
}

func (b *astbuilder) buildPackage(p *gol.PackageClauseContext) []string {
	recoverRange := b.SetRange(p.BaseParserRuleContext)
	defer recoverRange()

	if n := p.PackageName(); n != nil {
		re := b.buildPackageName(n.(*gol.PackageNameContext))
		return []string{re}
	}
	return []string{""}
}

func (b *astbuilder) buildPackageName(packageName *gol.PackageNameContext) string {
	recoverRange := b.SetRange(packageName.BaseParserRuleContext)
	defer recoverRange()

	if id := packageName.IDENTIFIER(); id != nil {
		return id.GetText()
	}
	return ""
}

func (b *astbuilder) buildImportDecl(importDecl *gol.ImportDeclContext) ([]string, []string) {
	recoverRange := b.SetRange(importDecl.BaseParserRuleContext)
	defer recoverRange()
	var namel, pathl []string

	for _, i := range importDecl.AllImportSpec() {
		name, path := b.buildImportSpec(i.(*gol.ImportSpecContext))
		namel = append(namel, name)
		pathl = append(pathl, path)
	}
	return namel, pathl
}

func (b *astbuilder) buildImportSpec(importSpec *gol.ImportSpecContext) (string, string) {
	recoverRange := b.SetRange(importSpec.BaseParserRuleContext)
	defer recoverRange()
	var name string
	var path string

	if p := importSpec.ImportPath(); p != nil {
		path = b.buildImportPath(p.(*gol.ImportPathContext))
		namel := strings.Split(path, "/")
		name = namel[len(namel)-1]
	}

	if id := importSpec.IDENTIFIER(); id != nil {
		name = id.GetText()
	}
	if dot := importSpec.DOT(); dot != nil {
		name = "."
	}
	return name, path
}

func (b *astbuilder) buildImportPath(importPath *gol.ImportPathContext) string {
	recoverRange := b.SetRange(importPath.BaseParserRuleContext)
	defer recoverRange()

	if s := importPath.String_(); s != nil {
		name := s.GetText()
		name = name[1 : len(name)-1]
		return name
	}

	return ""
}

func (b *astbuilder) handleImportPackage() (values []ssa.Value) {
	for n, ft := range b.importMap {
		value := b.ReadValue(n)
		values = append(values, value)
		typ := value.GetType()

		if b, ok := ssa.ToBasicType(typ); ok {
			typ = ssa.NewBasicType(b.Kind, b.GetName())
			typ.SetFullTypeNames([]string{ft.Path})
		}
		value.SetType(typ)
	}

	return
}

func (b *astbuilder) buildDeclaration(decl *gol.DeclarationContext, isglobal bool) {
	recoverRange := b.SetRange(decl.BaseParserRuleContext)
	defer recoverRange()

	if constDecl := decl.ConstDecl(); constDecl != nil {
		b.buildConstDecl(constDecl.(*gol.ConstDeclContext))
	}
	if varDecl := decl.VarDecl(); varDecl != nil {
		b.buildVarDecl(varDecl.(*gol.VarDeclContext), isglobal)
	}
	if typeDecl := decl.TypeDecl(); typeDecl != nil {
		b.buildTypeDecl(typeDecl.(*gol.TypeDeclContext))
	}
}

func (b *astbuilder) buildConstDecl(constDecl *gol.ConstDeclContext) {
	recoverRange := b.SetRange(constDecl.BaseParserRuleContext)
	defer recoverRange()
	var defaul ssa.Value = nil
	var index int
	var isiota bool = false

	for _, v := range constDecl.AllConstSpec() {
		rightv, isiotat := b.buildConstSpec(v.(*gol.ConstSpecContext), defaul)
		if isiotat { // 每个 const 块中的 iota 是独立的
			isiota = true
			index = 1
		}

		if isiota {
			rightv = b.EmitConstInst(index)
			index++
		}
		defaul = rightv
	}
}

func (b *astbuilder) buildConstSpec(constSpec *gol.ConstSpecContext, defaul ssa.Value) (ssa.Value, bool) {
	recoverRange := b.SetRange(constSpec.BaseParserRuleContext)
	defer recoverRange()

	var leftvl []*ssa.Variable
	var rightvl []ssa.Value
	var isiota bool = false

	leftList := constSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER()
	for _, value := range leftList {
		leftv := b.CreateLocalVariable(value.GetText())
		leftvl = append(leftvl, leftv)
		b.AddToCmap(value.GetText())
	}

	expList := constSpec.ExpressionList()
	if expList != nil {
		rightList := expList.(*gol.ExpressionListContext).AllExpression()
		for _, value := range rightList {
			rightv, _ := b.buildExpression(value.(*gol.ExpressionContext), false)
			rightvl = append(rightvl, rightv)
		}
	} else {
		if defaul != nil && len(leftList) == 1 {
			rightvl = append(rightvl, defaul)
		} else {
			b.NewError(ssa.Error, TAG, MissInitExpr(leftList[0].GetText()))
		}
	}
	if rightvl[0].String() == "iota" {
		rightvl[0] = b.EmitConstInst(0)
		isiota = true
	}

	b.AssignList(leftvl, rightvl)
	return rightvl[0], isiota
}

func (b *astbuilder) buildVarDecl(varDecl *gol.VarDeclContext, isglobal bool) {
	recoverRange := b.SetRange(varDecl.BaseParserRuleContext)
	defer recoverRange()
	for _, v := range varDecl.AllVarSpec() {
		b.buildVarSpec(v.(*gol.VarSpecContext), isglobal)
	}
}

func (b *astbuilder) buildVarSpec(varSpec *gol.VarSpecContext, isglobal bool) {
	recoverRange := b.SetRange(varSpec.BaseParserRuleContext)
	defer recoverRange()

	var leftvl []*ssa.Variable
	var rightvl []ssa.Value
	var ssaTyp ssa.Type

	if typ := varSpec.Type_(); typ != nil {
		ssaTyp = b.buildType(typ.(*gol.Type_Context))
	}

	a := varSpec.ASSIGN()

	if isglobal {
		if a == nil {
			leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER()
			for _, value := range leftList {
				recoverRange := b.SetRangeFromTerminalNode(value)
				id := value.GetText()
				if b.GetFromCmap(id) {
					b.NewError(ssa.Warn, TAG, CannotAssign())
				}
				b.AddGlobalVariable(id, b.GetDefaultValue(ssaTyp))
				recoverRange()
			}
		} else {
			leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER()
			rightList := varSpec.ExpressionList().(*gol.ExpressionListContext).AllExpression()
			for _, value := range leftList {
				if b.GetFromCmap(value.GetText()) {
					b.NewError(ssa.Warn, TAG, CannotAssign())
				}
			}
			for i, value := range rightList {
				rightv, _ := b.buildExpression(value.(*gol.ExpressionContext), false)
				rightvl = append(rightvl, rightv)
				b.AddGlobalVariable(leftList[i].GetText(), rightv)
			}
		}
	} else {
		if a == nil {
			leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER()
			for _, value := range leftList {
				recoverRange := b.SetRangeFromTerminalNode(value)
				id := value.GetText()
				if b.GetFromCmap(id) {
					b.NewError(ssa.Warn, TAG, CannotAssign())
				}

				leftv := b.CreateLocalVariable(id)
				b.AssignVariable(leftv, b.GetDefaultValue(ssaTyp))
				leftvl = append(leftvl, leftv)
				recoverRange()
			}
		} else {
			leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER()
			rightList := varSpec.ExpressionList().(*gol.ExpressionListContext).AllExpression()
			for _, value := range leftList {
				if b.GetFromCmap(value.GetText()) {
					b.NewError(ssa.Warn, TAG, CannotAssign())
				}

				leftv := b.CreateLocalVariable(value.GetText())
				leftvl = append(leftvl, leftv)
			}
			for _, value := range rightList {
				rightv, _ := b.buildExpression(value.(*gol.ExpressionContext), false)
				rightvl = append(rightvl, rightv)
			}
			b.AssignList(leftvl, rightvl)
		}
	}
}

func (b *astbuilder) AssignList(leftVariables []*ssa.Variable, rightVariables []ssa.Value) {
	leftlen := len(leftVariables)
	rightlen := len(rightVariables)

	GetCallField := func(c *ssa.Call, lvs []*ssa.Variable) {
		length := 1
		c.SetName(uuid.NewString())
		c.Unpack = true
		if it, ok := ssa.ToObjectType(c.GetType()); c.GetType().GetTypeKind() == ssa.TupleTypeKind && ok {
			length = it.Len
			if len(leftVariables) == length {
				for i := range leftVariables {
					value := b.ReadMemberCallValue(c, b.EmitConstInst(i))
					b.AssignVariable(leftVariables[i], value)
				}
				return
			}
		}
		if c.GetType().GetTypeKind() == ssa.AnyTypeKind {
			for i := range leftVariables {
				b.AssignVariable(
					leftVariables[i],
					b.ReadMemberCallValue(c, b.EmitConstInst(i)),
				)
			}
			return
		}

		if c.IsDropError {
			c.NewError(ssa.Error, TAG,
				ssa.CallAssignmentMismatchDropError(len(leftVariables), c.GetType().String()),
			)
		} else {
			b.NewError(ssa.Error, TAG,
				ssa.CallAssignmentMismatch(len(leftVariables), c.GetType().String()),
			)
		}

		for i := range leftVariables {
			if i >= length {
				value := b.ReadValue(leftVariables[i].GetName())
				b.AssignVariable(leftVariables[i], value)
				continue
			}

			if length == 1 {
				b.AssignVariable(leftVariables[i], c)
				continue
			}
			value := b.ReadMemberCallValue(c, b.EmitConstInst(i))
			b.AssignVariable(leftVariables[i], value)
		}
	}

	if leftlen == rightlen {
		for i, _ := range leftVariables {
			b.AssignVariable(leftVariables[i], rightVariables[i])
		}
	} else if rightlen == 1 { /* 大概率是函数调用 */
		inter := rightVariables[0]
		if c, ok := inter.(*ssa.Call); ok {
			GetCallField(c, leftVariables)
		} else {
			for i, _ := range leftVariables {
				b.AssignVariable(leftVariables[i], inter)
			}
		}
	} else {
		b.NewError(ssa.Error, TAG, MultipleAssignFailed(leftlen, rightlen))
		return
	}
}

func (b *astbuilder) buildTypeDecl(typeDecl *gol.TypeDeclContext) {
	recoverRange := b.SetRange(typeDecl.BaseParserRuleContext)
	defer recoverRange()

	for _, t := range typeDecl.AllTypeSpec() {
		if ts, ok := t.(*gol.TypeSpecContext); ok {
			b.buildTypeSpec(ts)
		}
	}
}

func (b *astbuilder) buildTypeSpec(ts *gol.TypeSpecContext) {
	recoverRange := b.SetRange(ts.BaseParserRuleContext)
	defer recoverRange()

	if alias := ts.AliasDecl(); alias != nil {
		b.buildAliasDecl(alias.(*gol.AliasDeclContext))
	}
	if typ := ts.TypeDef(); typ != nil {
		b.buildTypeDef(typ.(*gol.TypeDefContext))
	}
}

func (b *astbuilder) buildAliasDecl(alias *gol.AliasDeclContext) {
	recoverRange := b.SetRange(alias.BaseParserRuleContext)
	defer recoverRange()

	name := alias.IDENTIFIER().GetText()
	ssatyp := b.buildType(alias.Type_().(*gol.Type_Context))

	aliast := ssa.NewAliasType(name, ssatyp.PkgPathString(), ssatyp)
	b.AddAlias(name, aliast)
}

func (b *astbuilder) buildTypeDef(typedef *gol.TypeDefContext) {
	recoverRange := b.SetRange(typedef.BaseParserRuleContext)
	defer recoverRange()

	if param := typedef.TypeParameters(); param != nil {
		tpHandler := b.buildTypeParameters(param.(*gol.TypeParametersContext))
		defer tpHandler()
	}

	name := typedef.IDENTIFIER().GetText()
	ssatyp := b.buildType(typedef.Type_().(*gol.Type_Context))

	switch ssatyp.GetTypeKind() {
	case ssa.StructTypeKind:
		if it, ok := ssa.ToObjectType(ssatyp); ok {
			b.AddStruct(name, it)
		}
	default:
		aliast := ssa.NewAliasType(name, ssatyp.PkgPathString(), ssatyp)
		b.AddAlias(name, aliast)
	}
}

func (b *astbuilder) buildTypeParameters(typ *gol.TypeParametersContext) func() {
	recoverRange := b.SetRange(typ.BaseParserRuleContext)
	defer recoverRange()
	var alias []*ssa.AliasType

	for _, t := range typ.AllTypeParameterDecl() {
		aliast := b.buildTypeParameterDecl(t.(*gol.TypeParameterDeclContext))
		alias = append(alias, aliast...)
	}
	for _, a := range alias {
		b.AddAlias(a.Name, a)
	}

	return func() {
		for _, a := range alias {
			b.DelAliasByStr(a.Name)
		}
	}
}

func (b *astbuilder) buildTypeParameterDecl(typ *gol.TypeParameterDeclContext) []*ssa.AliasType {
	recoverRange := b.SetRange(typ.BaseParserRuleContext)
	defer recoverRange()

	var ssatyp ssa.Type
	var alias []*ssa.AliasType

	if te, ok := typ.TypeElement().(*gol.TypeElementContext); ok {
		ssatyp = b.buildTypeElement(te)
	}

	if idl, ok := typ.IdentifierList().(*gol.IdentifierListContext); ok {
		for _, id := range idl.AllIDENTIFIER() {
			name := id.GetText()
			aliast := ssa.NewAliasType(name, ssatyp.PkgPathString(), ssatyp)
			alias = append(alias, aliast)
		}
	}
	return alias
}

func (b *astbuilder) buildFunctionDeclFront(fun *gol.FunctionDeclContext) {
	var params []ssa.Type
	var result ssa.Type

	funcName := ""
	if Name := fun.IDENTIFIER(); Name != nil {
		funcName = Name.GetText()
	}

	newFunc := b.NewFunc(funcName)

	hitDefinedFunction := false
	MarkedFunctionType := b.GetMarkedFunction()
	handleFunctionType := func(fun *ssa.Function) {
		fun.ParamLength = len(fun.Params)
		fun.SetType(ssa.NewFunctionType("", params, result, false))
		fun.Type.IsMethod = false
		if MarkedFunctionType == nil {
			return
		}
		if len(fun.Params) != len(MarkedFunctionType.Parameter) {
			return
		}

		for i, p := range fun.Params {
			p.SetType(MarkedFunctionType.Parameter[i])
		}
		hitDefinedFunction = true
	}

	if funcName != "" {
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		defer recoverRange()

		variable := b.CreateLocalVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}

	PreHandlerBlock := b.CurrentBlock
	newFunc.SetLazyBuilder(func() {
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		CurrentBlock := b.CurrentBlock
		b.CurrentBlock = PreHandlerBlock

		defer func() {
			recoverRange()
			b.CurrentBlock = CurrentBlock
			if tph := b.tpHandler[newFunc.GetName()]; tph != nil {
				tph()
				delete(b.tpHandler, newFunc.GetName())
			}
		}()
		b.FunctionBuilder = b.PushFunction(newFunc)
		b.SupportClosure = false

		if para, ok := fun.Signature().(*gol.SignatureContext); ok {
			params, result = b.buildSignature(para)
		}

		if typeps := fun.TypeParameters(); typeps != nil {
			b.tpHandler[funcName] = b.buildTypeParameters(typeps.(*gol.TypeParametersContext))
		}

		handleFunctionType(b.Function)

		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}

		b.handleImportPackage()
		if block, ok := fun.Block().(*gol.BlockContext); ok {
			b.buildBlock(block)
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()

	}, false)
}

func (b *astbuilder) getReceiver(stmt *gol.ReceiverContext) []string {
	if parameters := stmt.Parameters(); parameters != nil {
		return b.getReceiverParameter(parameters.(*gol.ParametersContext))
	}
	return []string{}
}

func (b *astbuilder) getReceiverParameter(parms *gol.ParametersContext) []string {
	var types []string

	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			types = append(types, b.getReceiverDecl(i.(*gol.ParameterDeclContext)))
		}
	}

	return types
}

func (b *astbuilder) getReceiverDecl(para *gol.ParameterDeclContext) string {
	if typ := para.Type_(); typ != nil {
		if lit := typ.(*gol.Type_Context).TypeLit(); lit != nil {
		}
		if typ.GetText()[0] == '*' {
			return typ.GetText()[1:]
		}
		return typ.GetText()
	}
	return ""
}

func (b *astbuilder) buildMethodDeclFront(fun *gol.MethodDeclContext) {
	var params []ssa.Type
	var result ssa.Type

	funcName := ""
	methodName := ""
	if Name := fun.IDENTIFIER(); Name != nil {
		methodName = Name.GetText()
		if recove := fun.Receiver(); recove != nil {
			ssatypName := b.getReceiver(recove.(*gol.ReceiverContext))
			funcName = fmt.Sprintf("%s_%s", ssatypName[0], methodName)
		}
	}

	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(methodName)

	hitDefinedFunction := false
	MarkedFunctionType := b.GetMarkedFunction()
	handleFunctionType := func(fun *ssa.Function) {
		fun.ParamLength = len(fun.Params)
		fun.SetType(ssa.NewFunctionType("", params, result, false))
		fun.Type.IsMethod = true
		if MarkedFunctionType == nil {
			return
		}
		if len(fun.Params) != len(MarkedFunctionType.Parameter) {
			return
		}

		for i, p := range fun.Params {
			p.SetType(MarkedFunctionType.Parameter[i])
		}
		hitDefinedFunction = true
	}

	if funcName != "" {
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		defer recoverRange()

		variable := b.CreateLocalVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}

	{
		b.FunctionBuilder = b.PushFunction(newFunc)
		b.SupportClosure = false

		if recove := fun.Receiver(); recove != nil {
			ssatyp := b.buildReceiver(recove.(*gol.ReceiverContext))
			for _, t := range ssatyp {
				if it, ok := ssa.ToObjectType(t); ok {
					it.AddMethod(methodName, newFunc)
				}
			}
		}
		b.FunctionBuilder = b.PopFunction()
	}

	PreHandlerBlock := b.CurrentBlock
	newFunc.SetLazyBuilder(func() {
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		CurrentBlock := b.CurrentBlock
		b.CurrentBlock = PreHandlerBlock

		defer func() {
			recoverRange()
			b.CurrentBlock = CurrentBlock
			if tph := b.tpHandler[newFunc.GetName()]; tph != nil {
				tph()
				delete(b.tpHandler, newFunc.GetName())
			}
		}()
		b.FunctionBuilder = b.PushFunction(newFunc)
		b.SupportClosure = false

		if para, ok := fun.Signature().(*gol.SignatureContext); ok {
			params, result = b.buildSignature(para)
		}

		handleFunctionType(b.Function)

		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}

		b.handleImportPackage()
		if block, ok := fun.Block().(*gol.BlockContext); ok {
			b.buildBlock(block)
		}
		b.Finish()
		b.FunctionBuilder = b.PopFunction()

	}, false)
}

func (b *astbuilder) buildReceiver(stmt *gol.ReceiverContext) []ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var ssatyp []ssa.Type

	if parameters := stmt.Parameters(); parameters != nil {
		ssatyp = b.buildReceiverParameter(parameters.(*gol.ParametersContext))
	}
	return ssatyp
}

func (b *astbuilder) buildReceiverParameter(parms *gol.ParametersContext) []ssa.Type {
	recoverRange := b.SetRange(parms.BaseParserRuleContext)
	defer recoverRange()
	var types []ssa.Type

	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			types = append(types, b.buildReceiverDecl(i.(*gol.ParameterDeclContext)))
		}
	}

	return types
}

func (b *astbuilder) buildReceiverDecl(para *gol.ParameterDeclContext) ssa.Type {
	recoverRange := b.SetRange(para.BaseParserRuleContext)
	defer recoverRange()

	var typeType ssa.Type
	if typ := para.Type_(); typ != nil {
		typeType = b.buildType(typ.(*gol.Type_Context))
	}

	if idlist := para.IdentifierList(); idlist != nil {
		pList := b.buildParamList(idlist.(*gol.IdentifierListContext))
		if typeType != nil {
			for _, p := range pList {
				p.SetType(typeType)
			}
		}
	}

	return typeType
}

func (b *astbuilder) buildSignature(stmt *gol.SignatureContext) ([]ssa.Type, ssa.Type) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var paramt []ssa.Type
	var rett ssa.Type

	if paras := stmt.Parameters(); paras != nil {
		paramt = b.buildParameters(paras.(*gol.ParametersContext))
	}

	if rety := stmt.Result(); rety != nil {
		rett = b.buildResult(rety.(*gol.ResultContext))
	}

	return paramt, rett
}

func (b *astbuilder) buildParameters(parms *gol.ParametersContext) []ssa.Type {
	recoverRange := b.SetRange(parms.BaseParserRuleContext)
	defer recoverRange()

	var paramt []ssa.Type

	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			if a, ok := i.(*gol.ParameterDeclContext); ok {
				paramt = append(paramt, b.buildParameterDecl(a)...)
			}
		}
	} else {
		b.NewError(ssa.Error, TAG, ArrowFunctionNeedExpressionOrBlock())
		paramt = append(paramt, ssa.CreateAnyType())
	}

	return paramt
}

func (b *astbuilder) buildParameterDecl(para *gol.ParameterDeclContext) []ssa.Type {
	recoverRange := b.SetRange(para.BaseParserRuleContext)
	defer recoverRange()

	var typeType ssa.Type
	var typeTypes []ssa.Type
	if typ := para.Type_(); typ != nil {
		typeType = b.buildType(typ.(*gol.Type_Context))
	}

	if idlist := para.IdentifierList(); idlist != nil {
		pList := b.buildParamList(idlist.(*gol.IdentifierListContext))
		if typeType != nil {
			for _, p := range pList {
				typeTypes = append(typeTypes, typeType)
				p.SetType(typeType)
				p.GetProgram().Cache.AddVariable(typeType.String(), p)
			}
		}
		return typeTypes
	}
	return []ssa.Type{typeType}
}

func (b *astbuilder) buildParamList(idList *gol.IdentifierListContext) []*ssa.Parameter {
	recoverRange := b.SetRange(idList.BaseParserRuleContext)
	defer recoverRange()

	var pList []*ssa.Parameter

	for _, id := range idList.AllIDENTIFIER() {
		p := b.NewParam(id.GetText())
		pList = append(pList, p)
	}

	return pList
}

func (b *astbuilder) buildStructList(idList *gol.IdentifierListContext) []ssa.Value {
	recoverRange := b.SetRange(idList.BaseParserRuleContext)
	defer recoverRange()

	var pList []ssa.Value

	for _, id := range idList.AllIDENTIFIER() {
		p := b.EmitConstInst(id.GetText())
		pList = append(pList, p)
	}

	return pList
}

func (b *astbuilder) buildIdentifierList(idList *gol.IdentifierListContext, isLocal bool) []*ssa.Variable {
	recoverRange := b.SetRange(idList.BaseParserRuleContext)
	defer recoverRange()

	var vList []*ssa.Variable

	for _, id := range idList.AllIDENTIFIER() {
		text := id.GetText()
		if isLocal {
			vList = append(vList, b.CreateLocalVariable(text))
		} else {
			vList = append(vList, b.CreateVariable(text))
		}
	}

	return vList
}

func (b *astbuilder) buildResult(rety *gol.ResultContext) ssa.Type {
	recoverRange := b.SetRange(rety.BaseParserRuleContext)
	defer recoverRange()
	var typeType ssa.Type
	if typ := rety.Type_(); typ != nil {
		typeType = b.buildType(typ.(*gol.Type_Context))
	}

	if paras := rety.Parameters(); paras != nil {
		key, field := b.buildResultParameters(paras.(*gol.ParametersContext))
		obj := ssa.NewStructType()
		obj.SetTypeKind(ssa.TupleTypeKind)
		for i := range field {
			obj.AddField(key[i], field[i])
		}
		return obj
	}

	return typeType
}

func (b *astbuilder) buildResultParameters(parms *gol.ParametersContext) ([]ssa.Value, []ssa.Type) {
	recoverRange := b.SetRange(parms.BaseParserRuleContext)
	defer recoverRange()

	var key []ssa.Value
	var field []ssa.Type
	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			if a, ok := i.(*gol.ParameterDeclContext); ok {
				keyt, fieldt := b.buildResultParameterDecl(a)
				key = append(key, keyt...)
				field = append(field, fieldt...)
			}
		}
	} else {
		b.NewError(ssa.Error, TAG, ArrowFunctionNeedExpressionOrBlock())
		key = append(key, b.EmitConstInst(0))
		field = append(field, ssa.CreateAnyType())
	}

	return key, field
}

func (b *astbuilder) buildResultParameterDecl(para *gol.ParameterDeclContext) ([]ssa.Value, []ssa.Type) {
	recoverRange := b.SetRange(para.BaseParserRuleContext)
	defer recoverRange()

	var key []ssa.Value
	var field []ssa.Type
	var ssatyp ssa.Type

	zero := b.EmitConstInst(0)
	if typ := para.Type_(); typ != nil {
		ssatyp = b.buildType(typ.(*gol.Type_Context))
	}

	if idlist := para.IdentifierList(); idlist != nil {
		iList := b.buildIdentifierList(idlist.(*gol.IdentifierListContext), true)
		if ssatyp != nil {
			for _, i := range iList {
				b.AssignVariable(i, zero)
				key = append(key, zero)
				field = append(field, ssatyp)
				b.AddResultDefault(i.GetName())
			}
		}
		return key, field
	}

	return []ssa.Value{zero}, []ssa.Type{ssatyp}
}

func (b *astbuilder) buildBlock(block *gol.BlockContext, syntaxBlocks ...bool) {
	syntaxBlock := false
	if len(syntaxBlocks) > 0 {
		syntaxBlock = syntaxBlocks[0]
	}

	recoverRange := b.SetRange(block.BaseParserRuleContext)
	defer recoverRange()

	b.InCmapLevel()
	defer b.OutCmapLevel()

	s, ok := block.StatementList().(*gol.StatementListContext)

	if !ok {
		b.NewError(ssa.Warn, TAG, "empty block")
		return
	}

	if syntaxBlock {
		b.BuildSyntaxBlock(func() {
			b.buildStatementList(s)
		})
	} else {
		b.buildStatementList(s)
	}
}

func (b *astbuilder) buildStatementList(stmt *gol.StatementListContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	allstmt := stmt.AllStatement()
	if len(allstmt) == 0 {
		b.NewError(ssa.Warn, TAG, "empty statement list")
	} else {
		for _, stmt := range allstmt {
			if b.IsStop() {
				return
			}
			if stmt, ok := stmt.(*gol.StatementContext); ok {
				b.buildStatement(stmt)
			}
		}
	}
}

func (b *astbuilder) buildStatement(stmt *gol.StatementContext) {
	if b.IsBlockFinish() {
		return
	}
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	b.AppendBlockRange()

	if s, ok := stmt.Declaration().(*gol.DeclarationContext); ok {
		b.buildDeclaration(s, false)
	}

	if s, ok := stmt.SimpleStmt().(*gol.SimpleStmtContext); ok {
		b.buildSimpleStmt(s)
	}

	if s, ok := stmt.ReturnStmt().(*gol.ReturnStmtContext); ok {
		b.buildReturnStmt(s)
	}

	if s, ok := stmt.IfStmt().(*gol.IfStmtContext); ok {
		b.buildIfStmt(s)
	}

	if s, ok := stmt.ForStmt().(*gol.ForStmtContext); ok {
		b.buildForStmt(s)
	}

	if s, ok := stmt.SwitchStmt().(*gol.SwitchStmtContext); ok {
		b.buildSwitchStmt(s)
	}

	if s, ok := stmt.SelectStmt().(*gol.SelectStmtContext); ok {
		b.buildSelectStmt(s)
	}

	if s, ok := stmt.Block().(*gol.BlockContext); ok {
		b.buildBlock(s, true)
	}

	if s, ok := stmt.BreakStmt().(*gol.BreakStmtContext); ok {
		b.buildBreakStmt(s)
	}

	if s, ok := stmt.ContinueStmt().(*gol.ContinueStmtContext); ok {
		b.buildContinueStmt(s)
	}

	if s, ok := stmt.FallthroughStmt().(*gol.FallthroughStmtContext); ok {
		b.buildFallthroughStmt(s)
	}

	if s, ok := stmt.LabeledStmt().(*gol.LabeledStmtContext); ok {
		b.buildLabeledStmt(s)
	}

	if s, ok := stmt.GotoStmt().(*gol.GotoStmtContext); ok {
		b.buildGotoStmt(s)
	}

	if s, ok := stmt.DeferStmt().(*gol.DeferStmtContext); ok {
		b.buildDeferStmt(s)
	}

	if s, ok := stmt.GoStmt().(*gol.GoStmtContext); ok {
		b.buildGoStmt(s)
	}
}

func (b *astbuilder) buildGoStmt(stmt *gol.GoStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if stmt, ok := stmt.Expression().(*gol.ExpressionContext); ok {
		rightv := b.buildDeferGoExpression(stmt)
		switch t := rightv.(type) {
		case *ssa.Call:
			t.Async = true
			b.EmitCall(t)
		default:
			b.NewError(ssa.Error, TAG, "go statement error")
		}
	}
}

func (b *astbuilder) buildFallthroughStmt(stmt *gol.FallthroughStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if !b.Fallthrough() {
		b.NewError(ssa.Error, TAG, UnexpectedFallthroughStmt())
	}
}

func (b *astbuilder) buildDeferStmt(stmt *gol.DeferStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if stmt, ok := stmt.Expression().(*gol.ExpressionContext); ok {
		rightv := b.buildDeferGoExpression(stmt)
		switch t := rightv.(type) {
		case *ssa.Call:
			b.SetInstructionPosition(t)
			b.EmitDefer(t)
		default:
			b.NewError(ssa.Error, TAG, "defer statement error")
		}
	}
}

func (b *astbuilder) buildDeferGoExpression(stmt *gol.ExpressionContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var rv ssa.Value
	var args []ssa.Value
	if p := stmt.PrimaryExpr(); p != nil {
		if p := p.(*gol.PrimaryExprContext).PrimaryExpr(); p != nil {
			rv, _ = b.buildPrimaryExpression(p.(*gol.PrimaryExprContext), false)
		}
		if a := p.(*gol.PrimaryExprContext).Arguments(); a != nil {
			args = b.buildArgumentsExpression(a.(*gol.ArgumentsContext))
		}
	}
	return b.NewCall(rv, args)
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

func (b *astbuilder) buildGotoStmt(stmt *gol.GotoStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	if id := stmt.IDENTIFIER(); id != nil {
		b.handlerGoto(id.GetText())
	}
}

func (b *astbuilder) buildContinueStmt(stmt *gol.ContinueStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// if exist label, goto label
	if id := stmt.IDENTIFIER(); id != nil {
		b.handlerGoto(id.GetText())
		return
	}

	if !b.Continue() {
		b.NewError(ssa.Error, TAG, UnexpectedContinueStmt())
	}
}

func (b *astbuilder) buildBreakStmt(stmt *gol.BreakStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	// if exist label, goto label
	if id := stmt.IDENTIFIER(); id != nil {
		b.handlerGoto(id.GetText(), true)
		return
	}

	if !b.Break() {
		b.NewError(ssa.Error, TAG, UnexpectedBreakStmt())
	}
}

func (b *astbuilder) buildLabeledStmt(stmt *gol.LabeledStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	text := ""
	if id := stmt.IDENTIFIER(); id != nil {
		text = id.GetText()
	}

	LabelBuilder := b.GetLabelByName(text)
	if LabelBuilder == nil {
		LabelBuilder = b.BuildLabel(text)
		b.labels[text] = LabelBuilder
	}

	block := LabelBuilder.GetBlock()
	LabelBuilder.Build()
	b.AddLabel(text, block)
	for _, f := range LabelBuilder.GetGotoHandlers() {
		f(block)
	}

	b.EmitJump(block)
	b.CurrentBlock = block
	if s, ok := stmt.ForStmt().(*gol.ForStmtContext); ok {
		b.buildForStmt(s)
	}

	LabelBuilder.Finish()
}

func (b *astbuilder) buildSelectStmt(stmt *gol.SelectStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	SwitchBuilder := b.BuildSwitch()
	SwitchBuilder.AutoBreak = true

	var values []ssa.Value
	var casepList []*gol.CommClauseContext
	var defaultp *gol.CommClauseContext

	for _, commCase := range stmt.AllCommClause() {
		if commSwitchCase := commCase.(*gol.CommClauseContext).CommCase(); commSwitchCase != nil {
			if commSwitchCase.(*gol.CommCaseContext).DEFAULT() != nil {
				defaultp = commCase.(*gol.CommClauseContext)
			}
			if commSwitchCase.(*gol.CommCaseContext).CASE() != nil {
				casepList = append(casepList, commCase.(*gol.CommClauseContext))
			}
		}
	}

	SwitchBuilder.BuildCaseSize(len(casepList))
	SwitchBuilder.SetCase(func(i int) []ssa.Value {
		if commcList := casepList[i].CommCase(); commcList != nil {
			if sendList := commcList.(*gol.CommCaseContext).SendStmt(); sendList != nil {
				values = b.buildSendStmt(sendList.(*gol.SendStmtContext))
			} else if recvList := commcList.(*gol.CommCaseContext).RecvStmt(); recvList != nil {
				values = b.buildRecvStmt(recvList.(*gol.RecvStmtContext))
			}
		}
		return values
	})

	SwitchBuilder.BuildBody(func(i int) {
		if stmtList := casepList[i].StatementList(); stmtList != nil {
			b.buildStatementList(stmtList.(*gol.StatementListContext))
		}
	})

	// default
	if defaultp != nil {
		SwitchBuilder.BuildDefault(func() {
			if stmtList := defaultp.StatementList(); stmtList != nil {
				b.buildStatementList(stmtList.(*gol.StatementListContext))
			}
		})
	}

	SwitchBuilder.Finish()
}

func (b *astbuilder) buildSendStmt(stmt *gol.SendStmtContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var channv ssa.Value
	var datav ssa.Value

	if chann := stmt.GetChannel(); chann != nil {
		channv, _ = b.buildExpression(chann.(*gol.ExpressionContext), false)
	}

	if data := stmt.GetData(); data != nil {
		datav, _ = b.buildExpression(data.(*gol.ExpressionContext), false)
	}

	// TODO handler "<-"
	_ = channv
	_ = datav
	b.NewError(ssa.Error, TAG, ToDo())
	return []ssa.Value{b.EmitConstInst(0)}
}

func (b *astbuilder) buildRecvStmt(stmt *gol.RecvStmtContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var recvv ssa.Value

	if exp := stmt.GetRecvExpr(); exp != nil {
		recvv, _ = b.buildExpression(exp.(*gol.ExpressionContext), false)
	}

	if expl := stmt.ExpressionList(); expl != nil {
		for _, exp := range expl.(*gol.ExpressionListContext).AllExpression() {
			_, leftv := b.buildExpression(exp.(*gol.ExpressionContext), true)
			b.AssignVariable(leftv, recvv)
		}
	}

	if idl := stmt.IdentifierList(); idl != nil {
		for _, id := range idl.(*gol.IdentifierListContext).AllIDENTIFIER() {
			leftv := b.CreateLocalVariable(id.GetText())
			b.AssignVariable(leftv, recvv)
		}
	}

	return []ssa.Value{recvv}
}

func (b *astbuilder) buildSwitchStmt(stmt *gol.SwitchStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.ExprSwitchStmt().(*gol.ExprSwitchStmtContext); ok {
		b.buildExprSwitchStmt(s)
	}
	if s, ok := stmt.TypeSwitchStmt().(*gol.TypeSwitchStmtContext); ok {
		b.buildTypeSwitchStmt(s)
	}
}

func (b *astbuilder) buildExprSwitchStmt(stmt *gol.ExprSwitchStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	SwitchBuilder := b.BuildSwitch()
	SwitchBuilder.AutoBreak = true

	//  parse expression
	var cond ssa.Value
	if expr, ok := stmt.Expression().(*gol.ExpressionContext); ok {
		SwitchBuilder.BuildCondition(func() ssa.Value {
			if expr, ok := stmt.SimpleStmt().(*gol.SimpleStmtContext); ok {
				b.buildSimpleStmt(expr)
			}
			cond, _ = b.buildExpression(expr, false)
			return cond
		})
	}

	var casepList []*gol.ExprCaseClauseContext
	var defaultp *gol.ExprCaseClauseContext

	for _, exprCase := range stmt.AllExprCaseClause() {
		if exprSwitchCase := exprCase.(*gol.ExprCaseClauseContext).ExprSwitchCase(); exprSwitchCase != nil {
			if exprSwitchCase.(*gol.ExprSwitchCaseContext).DEFAULT() != nil {
				defaultp = exprCase.(*gol.ExprCaseClauseContext)
			}
			if exprSwitchCase.(*gol.ExprSwitchCaseContext).CASE() != nil {
				casepList = append(casepList, exprCase.(*gol.ExprCaseClauseContext))
			}
		}
	}

	SwitchBuilder.BuildCaseSize(len(casepList))
	SwitchBuilder.SetCase(func(i int) []ssa.Value {
		var values []ssa.Value
		if exprcList := casepList[i].ExprSwitchCase(); exprcList != nil {
			if exprList := exprcList.(*gol.ExprSwitchCaseContext).ExpressionList(); exprList != nil {
				for _, expr := range exprList.(*gol.ExpressionListContext).AllExpression() {
					rightv, _ := b.buildExpression(expr.(*gol.ExpressionContext), false)
					values = append(values, rightv)
				}
			}
		}
		return values
	})

	SwitchBuilder.BuildBody(func(i int) {
		if stmtList := casepList[i].StatementList(); stmtList != nil {
			b.buildStatementList(stmtList.(*gol.StatementListContext))
		}
	})

	// default
	if defaultp != nil {
		SwitchBuilder.BuildDefault(func() {
			if stmtList := defaultp.StatementList(); stmtList != nil {
				b.buildStatementList(stmtList.(*gol.StatementListContext))
			}
		})
	}

	SwitchBuilder.Finish()
}

func (b *astbuilder) buildTypeSwitchStmt(stmt *gol.TypeSwitchStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	SwitchBuilder := b.BuildSwitch()
	SwitchBuilder.AutoBreak = true

	if expr, ok := stmt.SimpleStmt().(*gol.SimpleStmtContext); ok {
		b.buildSimpleStmt(expr)
	}

	//  parse expression
	var cond ssa.Value

	if sg, ok := stmt.TypeSwitchGuard().(*gol.TypeSwitchGuardContext); ok {
		if expr, ok := sg.PrimaryExpr().(*gol.PrimaryExprContext); ok {
			SwitchBuilder.BuildCondition(func() ssa.Value {
				cond, _ = b.buildPrimaryExpression(expr, false)
				return b.EmitTypeValue(cond.GetType())
			})
		}

		var values []ssa.Value
		var casepList []*gol.TypeCaseClauseContext
		var defaultp *gol.TypeCaseClauseContext

		for _, typeCase := range stmt.AllTypeCaseClause() {
			if typeSwitchCase := typeCase.(*gol.TypeCaseClauseContext).TypeSwitchCase(); typeSwitchCase != nil {
				if typeSwitchCase.(*gol.TypeSwitchCaseContext).DEFAULT() != nil {
					defaultp = typeCase.(*gol.TypeCaseClauseContext)
				}
				if typeSwitchCase.(*gol.TypeSwitchCaseContext).CASE() != nil {
					casepList = append(casepList, typeCase.(*gol.TypeCaseClauseContext))
				}
			}
		}

		SwitchBuilder.BuildCaseSize(len(casepList))
		SwitchBuilder.SetCase(func(i int) []ssa.Value {
			if typecList := casepList[i].TypeSwitchCase(); typecList != nil {
				if typeList := typecList.(*gol.TypeSwitchCaseContext).TypeList(); typeList != nil {
					for _, typ := range typeList.(*gol.TypeListContext).AllType_() {
						ssatyp := b.buildType(typ.(*gol.Type_Context))
						values = append(values, b.EmitTypeValue(ssatyp))
					}
				}
			}
			return values
		})

		SwitchBuilder.BuildBody(func(i int) {
			if stmtList := casepList[i].StatementList(); stmtList != nil {
				b.buildStatementList(stmtList.(*gol.StatementListContext))
			}
		})

		// default
		if defaultp != nil {
			SwitchBuilder.BuildDefault(func() {
				if stmtList := defaultp.StatementList(); stmtList != nil {
					b.buildStatementList(stmtList.(*gol.StatementListContext))
				}
			})
		}
	}

	SwitchBuilder.Finish()
}

func (b *astbuilder) buildForStmt(stmt *gol.ForStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	// current := f.currentBlock
	loop := b.CreateLoopBuilder()

	// var cond ssa.Value
	var cond *gol.ExpressionContext
	if e, ok := stmt.Expression().(*gol.ExpressionContext); ok {
		// if only expression; just build expression in header;
		cond = e
	} else if condition, ok := stmt.ForClause().(*gol.ForClauseContext); ok {
		if first, ok := condition.GetInitStmt().(*gol.SimpleStmtContext); ok {
			// first expression is initialization, in enter block
			loop.SetFirst(func() []ssa.Value {
				recoverRange := b.SetRange(first.BaseParserRuleContext)
				defer recoverRange()
				return b.buildSimpleStmt(first)
			})
		}
		if expr, ok := condition.Expression().(*gol.ExpressionContext); ok {
			// build expression in header
			cond = expr
		}

		if third, ok := condition.GetPostStmt().(*gol.SimpleStmtContext); ok {
			// build latch
			loop.SetThird(func() []ssa.Value {
				// build third expression in loop.latch
				recoverRange := b.SetRange(third.BaseParserRuleContext)
				defer recoverRange()
				return b.buildSimpleStmt(third)
			})
		}

		loop.SetCondition(func() ssa.Value {
			var condition ssa.Value
			if cond == nil {
				condition = b.EmitConstInst(true)
			} else {
				// recoverRange := b.SetRange(cond.BaseParserRuleContext)
				// defer recoverRange()
				condition, _ = b.buildExpression(cond, false)
				if condition == nil {
					condition = b.EmitConstInst(true)
					// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
				}
			}
			return condition
		})
	} else if rangec, ok := stmt.RangeClause().(*gol.RangeClauseContext); ok {
		b.buildForRangeStmt(rangec, loop)
	}

	//  build body
	loop.SetBody(func() {
		if block, ok := stmt.Block().(*gol.BlockContext); ok {
			b.buildBlock(block)
		}
	})

	loop.Finish()
}

func (b *astbuilder) buildForRangeStmt(stmt *gol.RangeClauseContext, loop *ssa.LoopBuilder) {
	var value ssa.Value
	loop.SetFirst(func() []ssa.Value {
		value, _ = b.buildExpression(stmt.Expression().(*gol.ExpressionContext), false)
		return []ssa.Value{value}
	})

	loop.SetCondition(func() ssa.Value {
		var lefts []*ssa.Variable

		if leftList, ok := stmt.IdentifierList().(*gol.IdentifierListContext); ok {
			lefts = b.buildIdentifierList(leftList, true)
		}

		key, field, ok := b.EmitNext(value, stmt.ASSIGN() != nil)
		if len(lefts) == 1 {
			b.AssignVariable(lefts[0], key)
			ssa.DeleteInst(field)
		} else if len(lefts) >= 2 {
			if value.GetType().GetTypeKind() == ssa.ChanTypeKind {
				b.NewError(ssa.Error, TAG, InvalidChanType(value.GetType().String()))

				b.AssignVariable(lefts[0], key)
				ssa.DeleteInst(field)
			} else {
				b.AssignVariable(lefts[0], key)
				b.AssignVariable(lefts[1], field)
			}
		}
		return ok
	})
}

func (b *astbuilder) buildIfStmt(stmt *gol.IfStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	builder := b.CreateIfBuilder()

	var build func(stmt *gol.IfStmtContext) func()
	build = func(stmt *gol.IfStmtContext) func() {
		if expression := stmt.Expression(); expression != nil {
			builder.AppendItem(
				func() ssa.Value {
					if s, ok := stmt.SimpleStmt().(*gol.SimpleStmtContext); ok {
						b.buildSimpleStmt(s)
					}

					expressionStmt, ok := expression.(*gol.ExpressionContext)
					if !ok {
						return nil
					}

					recoverRange := b.SetRange(expressionStmt.BaseParserRuleContext)
					b.AppendBlockRange()
					recoverRange()

					rvalue, _ := b.buildExpression(expressionStmt, false)
					return rvalue
				},
				func() {
					b.buildBlock(stmt.Block(0).(*gol.BlockContext))
				},
			)
		}

		if stmt.ELSE() != nil {
			if elseBlock, ok := stmt.Block(1).(*gol.BlockContext); ok {
				return func() {
					b.buildBlock(elseBlock)
				}
			} else if elifstmt, ok := stmt.IfStmt().(*gol.IfStmtContext); ok {
				build := build(elifstmt)
				return build
			} else {
				return nil
			}
		}
		return nil
	}

	elseBlock := build(stmt)
	builder.SetElse(elseBlock)
	builder.Build()
}

func (b *astbuilder) buildReturnStmt(stmt *gol.ReturnStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var values []ssa.Value

	if expl, ok := stmt.ExpressionList().(*gol.ExpressionListContext); ok {
		for s := range expl.AllExpression() {
			rightv, _ := b.buildExpression(expl.Expression(s).(*gol.ExpressionContext), false)
			values = append(values, rightv)
		}
		if len(values) == 0 {
			b.NewError(ssa.Warn, TAG, "cannot return nil")
			b.EmitReturn([]ssa.Value{b.EmitConstInstNil()})
		} else {
			b.EmitReturn(values)
		}
	} else { /* 如果return没有设置expr则查找是否有默认返回值 */
		results := b.GetResultDefault()
		if results != nil {
			for _, result := range results {
				values = append(values, b.PeekValue(result))
			}
			b.EmitReturn(values)
		} else {
			b.NewError(ssa.Warn, TAG, "cannot return nil")
			b.EmitReturn([]ssa.Value{b.EmitConstInstNil()})
		}
	}
}

func (b *astbuilder) buildSimpleStmt(stmt *gol.SimpleStmtContext) []ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var rightv []ssa.Value

	if s, ok := stmt.ExpressionStmt().(*gol.ExpressionStmtContext); ok {
		rightv = b.buildExpressionStmt(s)
	}

	if s, ok := stmt.ShortVarDecl().(*gol.ShortVarDeclContext); ok {
		rightv = b.buildShortVarDecl(s)
	}

	if s, ok := stmt.Assignment().(*gol.AssignmentContext); ok {
		rightv = b.buildAssignment(s)
	}

	if s, ok := stmt.IncDecStmt().(*gol.IncDecStmtContext); ok {
		rightv = b.buildIncDecStmt(s)
	}

	if s, ok := stmt.SendStmt().(*gol.SendStmtContext); ok {
		rightv = b.buildSendStmt(s)
	}

	return rightv
}

func (b *astbuilder) buildIncDecStmt(stmt *gol.IncDecStmtContext) []ssa.Value {
	var values []ssa.Value

	if exp := stmt.Expression(); exp != nil {
		_, leftv := b.buildExpression(exp.(*gol.ExpressionContext), true)

		if stmt.PLUS_PLUS() != nil {
			value := b.EmitBinOp(ssa.OpAdd, b.ReadValueByVariable(leftv), b.EmitConstInst(1))
			b.AssignVariable(leftv, value)
			values = []ssa.Value{value}
		} else if stmt.MINUS_MINUS() != nil {
			value := b.EmitBinOp(ssa.OpSub, b.ReadValueByVariable(leftv), b.EmitConstInst(1))
			b.AssignVariable(leftv, value)
			values = []ssa.Value{value}
		}
	}

	return values
}

func (b *astbuilder) buildShortVarDecl(stmt *gol.ShortVarDeclContext) []ssa.Value {
	leftList := stmt.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER()
	rightList := stmt.ExpressionList().(*gol.ExpressionListContext).AllExpression()

	var leftvl []*ssa.Variable
	var rightvl []ssa.Value

	for _, value := range leftList {
		if b.GetFromCmap(value.GetText()) {
			b.NewError(ssa.Error, TAG, CannotAssign())
		}
		leftv := b.CreateLocalVariable(value.GetText())
		leftvl = append(leftvl, leftv)
	}
	for _, value := range rightList {
		rightv, _ := b.buildExpression(value.(*gol.ExpressionContext), false)
		rightvl = append(rightvl, rightv)
	}

	b.AssignList(leftvl, rightvl)

	return rightvl
}

func (b *astbuilder) buildAssignment(stmt *gol.AssignmentContext) []ssa.Value {
	var leftvl []*ssa.Variable
	var rightvl []ssa.Value
	var ssaop ssa.BinaryOpcode

	leftList := stmt.ExpressionList(0).(*gol.ExpressionListContext).AllExpression()
	rightList := stmt.ExpressionList(1).(*gol.ExpressionListContext).AllExpression()

	for _, value := range leftList {
		_, leftv := b.buildExpression(value.(*gol.ExpressionContext), true)
		leftvl = append(leftvl, leftv)
	}
	for _, value := range rightList {
		rightv, _ := b.buildExpression(value.(*gol.ExpressionContext), false)
		rightvl = append(rightvl, rightv)
	}

	if stmt.Assign_op().GetText() == "=" {
		b.AssignList(leftvl, rightvl)
	} else {
		op := stmt.Assign_op()
		switch op.GetText() {
		case "+=":
			ssaop = ssa.OpAdd
		case "-=":
			ssaop = ssa.OpSub
		case "*=":
			ssaop = ssa.OpMul
		case "/=":
			ssaop = ssa.OpDiv
		case "%=":
			ssaop = ssa.OpMod
		case "&=":
			ssaop = ssa.OpAnd
		case "|=":
			ssaop = ssa.OpOr
		case "^=":
			ssaop = ssa.OpXor
		case "<<=":
			ssaop = ssa.OpShl
		case ">>=":
			ssaop = ssa.OpShr
		case "&^=":
			ssaop = ssa.OpAndNot
		}
		retv := b.EmitBinOp(ssaop, b.ReadValueByVariable(leftvl[0]), rightvl[0])
		b.AssignList(leftvl, []ssa.Value{retv})
	}

	return rightvl
}

func (b *astbuilder) buildTypeArgs(stmt *gol.TypeArgsContext) func() {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var alias []*ssa.AliasType
	var ssatyp ssa.Type

	if tl := stmt.TypeList(); tl != nil {
		ssatyp = ssa.CreateAnyType()
		for _, typ := range tl.(*gol.TypeListContext).AllType_() {
			aliast := ssa.NewAliasType(typ.(*gol.Type_Context).GetText(), ssatyp.PkgPathString(), ssatyp)
			alias = append(alias, aliast)
		}
	}
	for _, a := range alias {
		b.AddAlias(a.Name, a)
	}

	return func() {
		for _, a := range alias {
			b.DelAliasByStr(a.Name)
		}
	}
}

func (b *astbuilder) buildType(typ *gol.Type_Context) ssa.Type {
	recoverRange := b.SetRange(typ.BaseParserRuleContext)
	defer recoverRange()
	var ssatyp ssa.Type

	if lit := typ.Type_(); lit != nil {
		ssatyp = b.buildType(lit.(*gol.Type_Context))
	}

	if tname := typ.TypeName(); tname != nil {
		ssatyp = b.buildTypeName(tname.(*gol.TypeNameContext))
		if a := typ.TypeArgs(); a != nil {
			b.tpHandler[b.Function.GetName()] = b.buildTypeArgs(a.(*gol.TypeArgsContext))
		}
	}

	if lit := typ.TypeLit(); lit != nil {
		ssatyp = b.buildTypeLit(lit.(*gol.TypeLitContext))
	}

	return ssatyp
}

func (b *astbuilder) buildTypeName(tname *gol.TypeNameContext) ssa.Type {
	recoverRange := b.SetRange(tname.BaseParserRuleContext)
	defer recoverRange()
	var ssatyp ssa.Type

	if qul := tname.QualifiedIdent(); qul != nil {
		if qul, ok := qul.(*gol.QualifiedIdentContext); ok {
			libName := qul.IDENTIFIER(0).GetText()
			typName := qul.IDENTIFIER(1).GetText()
			lib, path := b.GetImportPackage(libName)
			if lib == nil && path != "" { // 没有找到包，可能是golang库,也可能是package名称和导入名称不同
				b.NewError(ssa.Warn, TAG, PackageNotFind(libName))
				path = path + "/" + typName
			} else if lib != nil {
				obj, ok := lib.GetExportType(typName)

				if ok {
					return obj
				} else { // 没有找到类型，可能来自于golang库
					b.NewError(ssa.Warn, TAG, StructNotFind(typName))
				}
			} else {
				b.NewError(ssa.Warn, TAG, ImportNotFind(typName))
			}
			ssatyp = b.CreateBluePrint(libName)
			ssatyp.AddMethod(typName, b.NewFunc(typName))
			ssatyp.SetFullTypeNames([]string{path})
		}
	} else {
		name := tname.IDENTIFIER().GetText()
		ssatyp = ssa.GetTypeByStr(name)
		if ssatyp == nil {
			ssatyp = b.GetAliasByStr(name)
		}
		if ssatyp == nil {
			ssatyp = b.GetStructByStr(name)
		}
		if ssatyp == nil {
			ssatyp = b.GetSpecialTypeByStr(name)
		}
		if ssatyp == nil {
			b.NewError(ssa.Error, TAG, fmt.Sprintf("Type %v is not defined", name))
			ssatyp = ssa.CreateAnyType()
		}
	}

	return ssatyp
}

func (b *astbuilder) buildTypeElement(stmt *gol.TypeElementContext) ssa.Type {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	var ssatyp ssa.Type

	for _, typt := range stmt.AllTypeTerm() {
		if typ := typt.(*gol.TypeTermContext).Type_(); typ != nil {
			ssatyp = b.buildType(typ.(*gol.Type_Context))
		}
	}
	return ssatyp
}

func (b *astbuilder) buildMethodSpec(stmt *gol.MethodSpecContext, interfacetyp *ssa.InterfaceType) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	funcName := ""
	if Name := stmt.IDENTIFIER(); Name != nil {
		funcName = Name.GetText()
	}
	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(funcName)

	hitDefinedFunction := false
	MarkedFunctionType := b.GetMarkedFunction()
	handleFunctionType := func(fun *ssa.Function) {
		fun.ParamLength = len(fun.Params)
		if MarkedFunctionType == nil {
			return
		}
		if len(fun.Params) != len(MarkedFunctionType.Parameter) {
			return
		}

		for i, p := range fun.Params {
			p.SetType(MarkedFunctionType.Parameter[i])
		}
		hitDefinedFunction = true
	}

	{
		recoverRange := b.SetRange(stmt.BaseParserRuleContext)
		b.FunctionBuilder = b.PushFunction(newFunc)

		if para, ok := stmt.Result().(*gol.ResultContext); ok {
			b.buildResult(para)
		}

		handleFunctionType(b.Function)

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}
		recoverRange()
	}

	interfacetyp.AddMethod(funcName, newFunc)
}
