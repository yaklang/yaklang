package go2ssa

import (
	"fmt"

	"github.com/google/uuid"
	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// entry point
func (b *astbuilder) build(ast *gol.SourceFileContext) {
	recoverRange := b.SetRange(ast.BaseParserRuleContext)
	defer recoverRange()

	if packag, ok := ast.PackageClause().(*gol.PackageClauseContext); ok {
		b.buildPackage(packag)
	}

	for _, impo := range ast.AllImportDecl() {
		if impo,ok := impo.(*gol.ImportDeclContext); ok {
			b.buildImportDecl(impo)
		}
	}

	for _, decl := range ast.AllDeclaration() {
		if decl,ok := decl.(*gol.DeclarationContext); ok {
			b.buildDeclaration(decl)
		}
	}

	for _, meth := range ast.AllMethodDecl() {
		if meth,ok := meth.(*gol.MethodDeclContext); ok {
		    b.buildMethodDecl(meth)
		}
	}

	for _, fun := range ast.AllFunctionDecl() {
		if fun,ok := fun.(*gol.FunctionDeclContext); ok {
			b.buildFunctionDecl(fun)
		}
	}	
}

func (b *astbuilder) buildPackage(packag *gol.PackageClauseContext) {
    recoverRange := b.SetRange(packag.BaseParserRuleContext)
	defer recoverRange()

	// TODO
}

func (b *astbuilder) buildImportDecl(importDecl *gol.ImportDeclContext) {
	recoverRange := b.SetRange(importDecl.BaseParserRuleContext)
	defer recoverRange()
}

func (b *astbuilder) buildDeclaration(decl *gol.DeclarationContext) {
	recoverRange := b.SetRange(decl.BaseParserRuleContext)
	defer recoverRange()

	if constDecl := decl.ConstDecl(); constDecl != nil {
		b.buildConstDecl(constDecl.(*gol.ConstDeclContext))
	}
	if varDecl := decl.VarDecl(); varDecl != nil {
		b.buildVarDecl(varDecl.(*gol.VarDeclContext))
	}
	if typeDecl := decl.TypeDecl(); typeDecl != nil {
		b.buildTypeDecl(typeDecl.(*gol.TypeDeclContext))
	}
}

func (b *astbuilder) buildConstDecl(constDecl *gol.ConstDeclContext) {
	recoverRange := b.SetRange(constDecl.BaseParserRuleContext)
	defer recoverRange()
	for _,v := range constDecl.AllConstSpec() {
	    b.buildConstSpec(v.(*gol.ConstSpecContext))
	}
}

func (b *astbuilder) buildConstSpec(constSpec *gol.ConstSpecContext) {
	recoverRange := b.SetRange(constSpec.BaseParserRuleContext)
	defer recoverRange()

	var leftvl []*ssa.Variable
	var rightvl []ssa.Value

	leftList := constSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER();
	rightList := constSpec.ExpressionList().(*gol.ExpressionListContext).AllExpression();
	for _, value := range leftList {
		leftv := b.CreateLocalVariable(value.GetText())
		leftvl = append(leftvl,leftv)
		b.AddToCmap(value.GetText())
	}
	for _, value := range rightList {
		rightv, _ := b.buildExpression(value.(*gol.ExpressionContext),false)
		rightvl = append(rightvl, rightv)
	}
	b.AssignList(leftvl, rightvl)
}

func (b *astbuilder) buildVarDecl(varDecl *gol.VarDeclContext) {
	recoverRange := b.SetRange(varDecl.BaseParserRuleContext)
	defer recoverRange()
	for _,v := range varDecl.AllVarSpec() {
	    b.buildVarSpec(v.(*gol.VarSpecContext))
	}
}

func (b *astbuilder) buildVarSpec(varSpec *gol.VarSpecContext){
    recoverRange := b.SetRange(varSpec.BaseParserRuleContext)
	defer recoverRange()

	var leftvl []*ssa.Variable
	var rightvl []ssa.Value
	var ssaTyp ssa.Type

	if typ := varSpec.Type_(); typ != nil {
	    ssaTyp = b.buildType(typ.(*gol.Type_Context))
	}

	_ = ssaTyp

	a := varSpec.ASSIGN()
	if a == nil { 
		leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER();
		for _, value := range leftList {
			recoverRange := b.SetRangeFromTerminalNode(value)
			id := value.GetText()
			if b.GetFromCmap(id) {
				b.NewError(ssa.Warn, TAG, "cannot assign to const value")
			}
		
			leftv := b.CreateLocalVariable(id)
			b.AssignVariable(leftv, b.EmitValueOnlyDeclare(id))
			leftvl = append(leftvl,leftv)
			recoverRange()
		}
	}else{
		leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER();
		rightList := varSpec.ExpressionList().(*gol.ExpressionListContext).AllExpression();
		for _, value := range leftList {
			if b.GetFromCmap(value.GetText()) {
				b.NewError(ssa.Warn, TAG, "cannot assign to const value")
			}

			leftv := b.CreateLocalVariable(value.GetText())
			leftvl = append(leftvl,leftv)
		}
		for _, value := range rightList {
			rightv, _ := b.buildExpression(value.(*gol.ExpressionContext),false)
			rightvl = append(rightvl, rightv)
		}
		b.AssignList(leftvl, rightvl)
	}
}

func (b *astbuilder) AssignList(leftVariables []*ssa.Variable, rightVariables []ssa.Value)  {
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
					value := b.ReadMemberCallVariable(c, b.EmitConstInst(i))
					b.AssignVariable(leftVariables[i], value)
				}
				return
			}
		}
		if c.GetType().GetTypeKind() == ssa.AnyTypeKind {
			for i := range leftVariables {
				b.AssignVariable(
					leftVariables[i],
					b.ReadMemberCallVariable(c, b.EmitConstInst(i)),
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
				value := b.EmitUndefined(leftVariables[i].GetName())
				b.AssignVariable(leftVariables[i], value)
				continue
			}

			if length == 1 {
				b.AssignVariable(leftVariables[i], c)
				continue
			}
			value := b.ReadMemberCallVariable(c, b.EmitConstInst(i))
			b.AssignVariable(leftVariables[i], value)
		}
		return
	}

	if leftlen == rightlen {
		for i, _ := range leftVariables {
			b.AssignVariable(leftVariables[i], rightVariables[i])
		}
	}else if(rightlen == 1){
		inter := rightVariables[0]
		if c, ok := inter.(*ssa.Call); ok {
			GetCallField(c, leftVariables)
		}
	}else{
		b.NewError(ssa.Error, TAG, MultipleAssignFailed(leftlen, rightlen))
		return
	}
}


type getSingleExpr interface {
	Expression(i int) gol.IExpressionContext
}

func (b *astbuilder) buildExpression(exp *gol.ExpressionContext,IslValue bool) (ssa.Value, *ssa.Variable) {
	if exp == nil {
		return nil, nil
	}

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.Expression(i); s != nil {
			rightv, _ := b.buildExpression(s.(*gol.ExpressionContext),IslValue)
			return rightv
		} else {
			return nil
		}
	}

	fmt.Printf("exp = %v\n", exp.GetText())

	if ret := exp.PrimaryExpr();ret != nil{
		return b.buildPrimaryExpression(ret.(*gol.PrimaryExprContext), IslValue)
	}

	if !IslValue { // right
		if op := exp.GetUnary_op(); op != nil {
			var ssaop ssa.UnaryOpcode
	
			switch op.GetText() {
			case "+":
				ssaop = ssa.OpPlus
			case "-":
				ssaop = ssa.OpNeg
			case "!":
				ssaop = ssa.OpNot
			case "^":
				ssaop = ssa.OpBitwiseNot
			case "<-":
				ssaop = ssa.OpChan
			case "*":
				// TODO
			case "&":
			default:
				b.NewError(ssa.Error, TAG, UnaryOperatorNotSupport(op.GetText()))
			}
	
			op1 := getValue(exp, 0)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitUnOp(ssaop, op1),nil
		}
	
		if op := exp.GetAdd_op(); op != nil {
			var ssaop ssa.BinaryOpcode
	
			switch op.GetText() {
			case "+":
				ssaop = ssa.OpAdd
			case "-":
				ssaop = ssa.OpSub
			case "|":
				ssaop = ssa.OpOr
			case "^":
				ssaop = ssa.OpXor
			default:
			}
	
			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitBinOp(ssaop, op1, op2),nil
		}
	
		if op := exp.GetMul_op(); op != nil {
			var ssaop ssa.BinaryOpcode
	
			switch op.GetText() {
			case "*":
				ssaop = ssa.OpMul
			case "/":
				ssaop = ssa.OpDiv
			case "%":
				ssaop = ssa.OpMod
			case "&":
				ssaop = ssa.OpAnd
			case "<<":
				ssaop = ssa.OpShl
			case ">>":
				ssaop = ssa.OpShr
			case "&^":
				ssaop = ssa.OpAndNot
			default:
			}
	
			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitBinOp(ssaop, op1, op2),nil
		}
	
		if op := exp.GetRel_op(); op != nil {
			var ssaop ssa.BinaryOpcode

			switch op.GetText() {
			case "==":
				ssaop = ssa.OpEq
			case "!=":
				ssaop = ssa.OpNotEq
			case "<":
				ssaop = ssa.OpLt
			case ">":
				ssaop = ssa.OpGt
			case "<=":
				ssaop = ssa.OpLtEq
			case ">=":
				ssaop = ssa.OpGtEq
			default:
			}
			
			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitBinOp(ssaop, op1, op2),nil
		}
	}else{ // left

	}

	return nil, nil
}

func (b *astbuilder) buildPrimaryExpression(exp *gol.PrimaryExprContext,IslValue bool) (ssa.Value, *ssa.Variable) {
	if ret := exp.Operand(); ret != nil {
		return b.buildOperandExpression(ret.(*gol.OperandContext), IslValue)
	}

	if IslValue {
		rv,_ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext),false)
		
		if ret := exp.Index(); ret != nil {
		    index := b.buildIndexExpression(ret.(*gol.IndexContext))
			return nil, b.CreateMemberCallVariable(rv, index)
		}

		if ret := exp.DOT(); ret != nil {
			if id := exp.IDENTIFIER(); id != nil {
				return nil, b.CreateMemberCallVariable(rv, b.EmitConstInst(id.GetText()))
			}
		}
	}

	if !IslValue {
		rv,_ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext),false)
		if ret := exp.Arguments(); ret != nil {
			args := b.buildArgumentsExpression(ret.(*gol.ArgumentsContext))
			return b.EmitCall(b.NewCall(rv, args)),nil
		}

		if ret := exp.Index(); ret != nil {
		    index := b.buildIndexExpression(ret.(*gol.IndexContext))
			return b.ReadMemberCallVariable(rv, index), nil
		}

		if ret := exp.Slice_(); ret != nil {
		    values := b.buildSliceExpression(ret.(*gol.Slice_Context))
			return b.EmitMakeSlice(rv, values[0], values[1], values[2]), nil
		}

		if ret := exp.DOT(); ret != nil {
			if id := exp.IDENTIFIER(); id != nil {
				test := id.GetText()
				member :=  b.ReadMemberCallVariable(rv, b.EmitConstInst(test))
				return member, nil
			}
		}
	}

	return nil, nil
}

func (b *astbuilder) buildSliceExpression(exp *gol.Slice_Context) ([3]ssa.Value) {
	var values [3]ssa.Value
	
	if low := exp.GetLow(); low != nil {
		rightv,_ := b.buildExpression(low.(*gol.ExpressionContext), false)
	    values[0] = rightv
	}
	if high := exp.GetHigh(); high != nil {
		rightv,_ := b.buildExpression(high.(*gol.ExpressionContext), false)
	    values[1] = rightv
	}
	if max := exp.GetMax(); max != nil {
		rightv,_ := b.buildExpression(max.(*gol.ExpressionContext), false)
	    values[2] = rightv
	}

    return values
}

func (b *astbuilder) buildIndexExpression(arg *gol.IndexContext) (ssa.Value) {
	if exp := arg.Expression(); exp != nil {
		rv, _ := b.buildExpression(exp.(*gol.ExpressionContext), false)
		return rv
	}
	return nil
}

func (b *astbuilder) buildArgumentsExpression(arg *gol.ArgumentsContext) ([]ssa.Value) {
	var args []ssa.Value

	if typ := arg.Type_(); typ != nil {
	    ssatyp := b.buildType(typ.(*gol.Type_Context))
		args = append(args, b.EmitTypeValue(ssatyp))
	}

	if expl := arg.ExpressionList(); expl != nil {
		for _, exp := range expl.(*gol.ExpressionListContext).AllExpression(){
			rv, _ := b.buildExpression(exp.(*gol.ExpressionContext), false)
			args = append(args, rv)
		}
	}

	return args
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

	aliast := ssa.NewAliasType(name,ssatyp.PkgPathString(),ssatyp)
	b.AddAlias(name, aliast)
}

func (b *astbuilder) buildTypeDef(typedef *gol.TypeDefContext) {
	recoverRange := b.SetRange(typedef.BaseParserRuleContext)
	defer recoverRange()

	name := typedef.IDENTIFIER().GetText()
	ssatyp := b.buildType(typedef.Type_().(*gol.Type_Context))

	switch ssatyp.GetTypeKind() {
	case ssa.StructTypeKind:
		if it,ok := ssa.ToObjectType(ssatyp); ok {
			b.AddStruct(name, it)
		}
	default:
		aliast := ssa.NewAliasType(name,ssatyp.PkgPathString(),ssatyp)
		b.AddAlias(name, aliast)
	}
}

func (b *astbuilder) buildFunctionDecl(fun *gol.FunctionDeclContext) ssa.Value{
	recoverRange := b.SetRange(fun.BaseParserRuleContext)
	defer recoverRange()

	funcName := ""
	if Name := fun.IDENTIFIER(); Name != nil {
		funcName = Name.GetText()
	}
	newFunc := b.NewFunc(funcName)

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
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		b.FunctionBuilder = b.PushFunction(newFunc)
		
		if para, ok := fun.Signature().(*gol.SignatureContext); ok {
			b.buildSignature(para)
		}

		handleFunctionType(b.Function)
		
		if block, ok := fun.Block().(*gol.BlockContext); ok {
			b.buildBlock(block)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}
		recoverRange()
	}

	if funcName != "" {
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		defer recoverRange()

		variable := b.CreateLocalVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}
	return newFunc
}

func (b *astbuilder) buildMethodDecl(fun *gol.MethodDeclContext) ssa.Value {
	recoverRange := b.SetRange(fun.BaseParserRuleContext)
	defer recoverRange()

	funcName := ""
	if Name := fun.IDENTIFIER(); Name != nil {
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
		recoverRange := b.SetRange(fun.BaseParserRuleContext)
		b.FunctionBuilder = b.PushFunction(newFunc)
		
		if recove := fun.Receiver(); recove != nil {
			ssatyp := b.buildReceiver(recove.(*gol.ReceiverContext))
			for _,t := range ssatyp {
				if it,ok := ssa.ToObjectType(t); ok {
					it.AddMethod(funcName, newFunc)
				}
			}
		}

		if para, ok := fun.Signature().(*gol.SignatureContext); ok {
			b.buildSignature(para)
		}

		handleFunctionType(b.Function)
		
		if block, ok := fun.Block().(*gol.BlockContext); ok {
			b.buildBlock(block)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
		if hitDefinedFunction {
			b.MarkedFunctions = append(b.MarkedFunctions, newFunc)
		}
		recoverRange()
	}

	return newFunc
}

func (b *astbuilder) buildReceiver(stmt *gol.ReceiverContext) ([]ssa.Type){
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if parameters := stmt.Parameters(); parameters != nil {
	    return b.buildReceiverParameter(parameters.(*gol.ParametersContext))
	}
	return nil
}

func (b *astbuilder) buildReceiverParameter(parms *gol.ParametersContext) []ssa.Type {
	recoverRange := b.SetRange(parms.BaseParserRuleContext)
	defer recoverRange()
	var types []ssa.Type

	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			types = append(types,b.buildReceiverDecl(i.(*gol.ParameterDeclContext)) )
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

func (b *astbuilder) buildSignature(stmt *gol.SignatureContext) ([]ssa.Type,ssa.Type) {
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

	return paramt,rett
}


func (b *astbuilder) buildParameters(parms *gol.ParametersContext) []ssa.Type {
    recoverRange := b.SetRange(parms.BaseParserRuleContext)
	defer recoverRange()

	var paramt []ssa.Type

	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			if a, ok := i.(*gol.ParameterDeclContext); ok {
				paramt = append(paramt,  b.buildParameterDecl(a)...)
			}
		} 
	}else{
		b.NewError(ssa.Error, TAG, ArrowFunctionNeedExpressionOrBlock())
	}

	return paramt
}

func (b *astbuilder) buildParameterDecl(para *gol.ParameterDeclContext) []ssa.Type {
	recoverRange := b.SetRange(para.BaseParserRuleContext)
	defer recoverRange()

	var typeType ssa.Type 
	var paramt []ssa.Type
	if typ := para.Type_(); typ != nil {
		typeType = b.buildType(typ.(*gol.Type_Context))
	}

	if idlist := para.IdentifierList(); idlist != nil {
	    pList := b.buildParamList(idlist.(*gol.IdentifierListContext))
		if typeType != nil {
			for _, p := range pList {
				paramt = append(paramt, typeType)
				p.SetType(typeType)
			}
		}
		return paramt
	}
	return []ssa.Type{typeType}
}

func (b *astbuilder) buildParamList(idList *gol.IdentifierListContext) []*ssa.Parameter{
	recoverRange := b.SetRange(idList.BaseParserRuleContext)
	defer recoverRange()

	var pList []*ssa.Parameter

	for _, id := range idList.AllIDENTIFIER() {
		p := b.NewParam(id.GetText())
		pList = append(pList, p)
	}
	
	return pList
}

func (b *astbuilder) buildStructList(idList *gol.IdentifierListContext) []ssa.Value{
	recoverRange := b.SetRange(idList.BaseParserRuleContext)
	defer recoverRange()

	var pList []ssa.Value

	for _, id := range idList.AllIDENTIFIER() {
	    p := b.EmitConstInst(id.GetText())
		pList = append(pList, p)
	}

	return pList
}

func (b *astbuilder) buildIdentifierList(idList *gol.IdentifierListContext,isLocal bool) []*ssa.Variable { 
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
	    b.buildParameters(paras.(*gol.ParametersContext))
	}

	return typeType
}


func (b *astbuilder) buildBlock(block *gol.BlockContext,syntaxBlocks ...bool) {
	syntaxBlock := false
	if len(syntaxBlocks) > 0 {
		syntaxBlock = syntaxBlocks[0]
	}


	recoverRange := b.SetRange(block.BaseParserRuleContext)
	defer recoverRange()

	b.InCmapLevel()
	defer b.OutCmapLevel()

	s, ok := block.StatementList().(*gol.StatementListContext);
		
	if (!ok) {
		b.NewError(ssa.Warn, TAG, "empty block")
		return
	}

	if syntaxBlock {
		b.BuildSyntaxBlock(func(){
			b.buildStatementList(s)
		})
	} else {
		b.buildStatementList(s)
	}
}

func (b* astbuilder) buildStatementList(stmt *gol.StatementListContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	allstmt := stmt.AllStatement()
	if len(allstmt) == 0 {
		b.NewError(ssa.Warn, TAG, "empty statement list")
	} else {
		for _, stmt := range allstmt {
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
		b.buildDeclaration(s)
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
	    b.buildBlock(s,true)
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

func (b* astbuilder) buildGoStmt(stmt *gol.GoStmtContext){
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

func (b* astbuilder) buildFallthroughStmt(stmt *gol.FallthroughStmtContext){
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if !b.Fallthrough() {
		b.NewError(ssa.Error, TAG, UnexpectedFallthroughStmt())
	}
}

func (b* astbuilder) buildDeferStmt(stmt *gol.DeferStmtContext){
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()
	
	if stmt, ok := stmt.Expression().(*gol.ExpressionContext); ok {
	    rightv := b.buildDeferGoExpression(stmt)
		switch t := rightv.(type) {
		case *ssa.Call:
			b.SetInstructionPosition(t)
			b.AddDefer(t)
		default:
			b.NewError(ssa.Error, TAG, "defer statement error")
		}
	}
}

func (b* astbuilder) buildDeferGoExpression(stmt *gol.ExpressionContext) ssa.Value {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var rv ssa.Value
	if p := stmt.PrimaryExpr(); p != nil {
		if p := p.(*gol.PrimaryExprContext).PrimaryExpr(); p != nil {
			rv,_ = b.buildPrimaryExpression(p.(*gol.PrimaryExprContext),false)
		}
		if a := p.(*gol.PrimaryExprContext).Arguments(); a != nil {
			args := b.buildArgumentsExpression(a.(*gol.ArgumentsContext))
			return b.NewCall(rv, args)
		}
	}
	return nil
}

func (b* astbuilder) buildGotoStmt(stmt *gol.GotoStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var _goto *ssa.BasicBlock
	if id := stmt.IDENTIFIER(); id != nil {
		text := id.GetText()
		if _goto = b.GetLabel(text); _goto != nil {
			b.EmitJump(_goto)
		} else {
			b.NewError(ssa.Error, TAG, UndefineLabelstmt())
		}
		return
	}
}

func (b* astbuilder) buildLabeledStmt(stmt *gol.LabeledStmtContext) {
	// TODO: Label not defined
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	text := ""
	if id := stmt.IDENTIFIER(); id != nil {
		text = id.GetText()
	}

	block := b.NewBasicBlockUnSealed(text)
	block.SetScope(b.CurrentBlock.ScopeTable.CreateSubScope())
	b.AddLabel(text, block)
	b.EmitJump(block)
	b.CurrentBlock = block
	if s, ok := stmt.Statement().(*gol.StatementContext); ok {
		b.buildStatement(s)
	}
	b.DeleteLabel(text)
}

func (b* astbuilder) buildContinueStmt(stmt *gol.ContinueStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if !b.Continue() {
		b.NewError(ssa.Error, TAG, UnexpectedContinueStmt())
	}
}


func (b *astbuilder) buildBreakStmt(stmt *gol.BreakStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var _break *ssa.BasicBlock
	if id := stmt.IDENTIFIER(); id != nil {
		text := id.GetText()
		if _break = b.GetLabel(text); _break != nil {
			b.EmitJump(_break)
		} else {
			b.NewError(ssa.Error, TAG, UndefineLabelstmt())
		}
		return
	}

	if !b.Break() {
		b.NewError(ssa.Error, TAG, UnexpectedBreakStmt())
	}
}

func (b *astbuilder) buildSelectStmt(stmt *gol.SelectStmtContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	SwitchBuilder := b.BuildSwitch()
	SwitchBuilder.AutoBreak = true

	var values []ssa.Value
	var casepList []*gol.CommClauseContext
	var defaultp *gol.CommClauseContext

	for _, commCase := range  stmt.AllCommClause() {
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
			}else if recvList := commcList.(*gol.CommCaseContext).RecvStmt(); recvList != nil {
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

func (b* astbuilder) buildSendStmt(stmt *gol.SendStmtContext) []ssa.Value {
    recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var channv ssa.Value
	var datav ssa.Value

	if chann := stmt.GetChannel(); chann != nil {
		channv,_ = b.buildExpression(chann.(*gol.ExpressionContext), false)
	}

	if data := stmt.GetData(); data != nil {
		datav,_ = b.buildExpression(data.(*gol.ExpressionContext), false)
	}

	// TODO handler "<-"
	_ = channv
	_ = datav

	return nil
}

func (b* astbuilder) buildRecvStmt(stmt *gol.RecvStmtContext) []ssa.Value {
    recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	var recvv ssa.Value

	if exp := stmt.GetRecvExpr(); exp != nil {
	    recvv,_ = b.buildExpression(exp.(*gol.ExpressionContext), false)
	}

	if expl := stmt.ExpressionList(); expl != nil {
		for _, exp := range expl.(*gol.ExpressionListContext).AllExpression() {
			_, leftv := b.buildExpression(exp.(*gol.ExpressionContext), false)
			b.AssignVariable(leftv,recvv)
		}
	}

	if idl := stmt.IdentifierList(); idl != nil {
		for _, id := range idl.(*gol.IdentifierListContext).AllIDENTIFIER() {
			leftv := b.CreateLocalVariable(id.GetText())
			b.AssignVariable(leftv,recvv)
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

	if expr, ok := stmt.SimpleStmt().(*gol.SimpleStmtContext); ok {
		b.buildSimpleStmt(expr)
	}

	//  parse expression
	var cond ssa.Value
	if expr, ok := stmt.Expression().(*gol.ExpressionContext); ok {
		SwitchBuilder.BuildCondition(func() ssa.Value {
			cond,_ = b.buildExpression(expr,false)
			return cond
		})
	} 

	var values []ssa.Value
	var casepList []*gol.ExprCaseClauseContext
	var defaultp *gol.ExprCaseClauseContext

	for _, exprCase := range  stmt.AllExprCaseClause() {
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
		if exprcList := casepList[i].ExprSwitchCase(); exprcList != nil {
			if exprList := exprcList.(*gol.ExprSwitchCaseContext).ExpressionList(); exprList != nil {
				for _, expr := range exprList.(*gol.ExpressionListContext).AllExpression() {
				    rightv, _ := b.buildExpression(expr.(*gol.ExpressionContext),false)
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
				cond,_ = b.buildPrimaryExpression(expr,false)
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
				condition,_ = b.buildExpression(cond,false)
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

func (b *astbuilder) buildForRangeStmt(stmt *gol.RangeClauseContext,loop *ssa.LoopBuilder) {
	var value ssa.Value
	loop.SetFirst(func() []ssa.Value {
		value,_ = b.buildExpression(stmt.Expression().(*gol.ExpressionContext), false)
		return []ssa.Value{value}
	})

	loop.SetCondition(func() ssa.Value {
		var lefts []*ssa.Variable

		if leftList, ok := stmt.IdentifierList().(*gol.IdentifierListContext); ok {
			lefts = b.buildIdentifierList(leftList,true)
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

	if s, ok := stmt.SimpleStmt().(*gol.SimpleStmtContext); ok {
		b.buildSimpleStmt(s)
	}

	builder := b.CreateIfBuilder()

	var build func(stmt *gol.IfStmtContext) func()
	build = func(stmt *gol.IfStmtContext) func() {
		if expression := stmt.Expression(); expression != nil {
			builder.AppendItem(
				func() ssa.Value {
					expressionStmt, ok := expression.(*gol.ExpressionContext)
					if !ok {
						return nil
					}

					recoverRange := b.SetRange(expressionStmt.BaseParserRuleContext)
					b.AppendBlockRange()
					recoverRange()

					rvalue,_ := b.buildExpression(expressionStmt,false)
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
		for s := range expl.AllExpression(){
			rightv, _ := b.buildExpression(expl.Expression(s).(*gol.ExpressionContext), false)
			values = append(values, rightv)
		}
		b.EmitReturn(values)
	} else {
		b.EmitReturn(nil)
	}
}

func (b *astbuilder) buildSimpleStmt(stmt *gol.SimpleStmtContext) []ssa.Value {
    
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.ExpressionStmt().(*gol.ExpressionStmtContext); ok {
		return b.buildExpressionStmt(s)
	}
	
	if s, ok := stmt.ShortVarDecl().(*gol.ShortVarDeclContext); ok {
	    return b.buildShortVarDecl(s)
	}

	if s, ok := stmt.Assignment().(*gol.AssignmentContext); ok {
	    return b.buildAssignment(s)
	}

	if s, ok := stmt.IncDecStmt().(*gol.IncDecStmtContext); ok {
	    return b.buildIncDecStmt(s)
	}

	if s, ok := stmt.SendStmt().(*gol.SendStmtContext); ok {
	    return b.buildSendStmt(s)
	}

	return nil
}

func (b *astbuilder) buildIncDecStmt(stmt *gol.IncDecStmtContext) []ssa.Value {
	var values []ssa.Value

    if exp := stmt.Expression(); exp != nil {
        _,leftv := b.buildExpression(exp.(*gol.ExpressionContext), true)

		if stmt.PLUS_PLUS() != nil{
			value := b.EmitBinOp(ssa.OpAdd, b.ReadValueByVariable(leftv), b.EmitConstInst(1))
			b.AssignVariable(leftv, value)
			values = []ssa.Value{value}
		}else if stmt.MINUS_MINUS() != nil{
			value := b.EmitBinOp(ssa.OpSub, b.ReadValueByVariable(leftv), b.EmitConstInst(1))
			b.AssignVariable(leftv, value)
			values = []ssa.Value{value}
		}else{
			return nil
		}
    }

	return values
}

func (b *astbuilder) buildExpressionStmt(stmt *gol.ExpressionStmtContext) []ssa.Value {
    if exp := stmt.Expression(); exp != nil {
        rightv,_ := b.buildExpression(exp.(*gol.ExpressionContext),false)
		return []ssa.Value{rightv}
    }
	return nil
}

func (b *astbuilder) buildShortVarDecl(stmt *gol.ShortVarDeclContext) []ssa.Value {
	leftList := stmt.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER();
	rightList := stmt.ExpressionList().(*gol.ExpressionListContext).AllExpression();

	var leftvl []*ssa.Variable
	var rightvl []ssa.Value

	for _, value := range leftList {
		if b.GetFromCmap(value.GetText()) {
			b.NewError(ssa.Warn, TAG, "cannot assign to const value")
		}
		leftv := b.CreateLocalVariable(value.GetText())
		leftvl = append(leftvl, leftv)
		b.AddVariable(leftv)
	}
	for _, value := range rightList {
		rightv, _ := b.buildExpression(value.(*gol.ExpressionContext),false)
		rightvl = append(rightvl, rightv)
	}

	b.AssignList(leftvl, rightvl)

	return rightvl
}

func (b *astbuilder) buildAssignment(stmt *gol.AssignmentContext) []ssa.Value {
	var leftvl []*ssa.Variable
	var rightvl []ssa.Value
	var ssaop ssa.BinaryOpcode

	leftList := stmt.ExpressionList(0).(*gol.ExpressionListContext).AllExpression();
	rightList := stmt.ExpressionList(1).(*gol.ExpressionListContext).AllExpression();

	for _, value := range leftList {
		_, leftv := b.buildExpression(value.(*gol.ExpressionContext),true)
		leftvl = append(leftvl, leftv)
	}
	for _, value := range rightList {
		rightv, _ := b.buildExpression(value.(*gol.ExpressionContext),false)
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
		retv := b.EmitBinOp(ssaop, b.ReadValueByVariable(leftvl[0]) , rightvl[0])
		b.AssignList(leftvl, []ssa.Value{retv})
	}

	return rightvl
}