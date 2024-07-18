package go2ssa

import (
	"fmt"

	"github.com/google/uuid"
	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// entry point
func (b *astbuilder) build(ast *gol.SourceFileContext) {
	if packag, ok := ast.PackageClause().(*gol.PackageClauseContext); ok {
		b.buildPackge(packag)
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

	for _, fun := range ast.AllFunctionDecl() {
		if fun,ok := fun.(*gol.FunctionDeclContext); ok {
			b.buildFunctionDecl(fun)
		}
	}	

	for _, meth := range ast.AllMethodDecl() {
		if meth,ok := meth.(*gol.MethodDeclContext); ok {
		    b.buildMethodDecl(meth)
		}
	}
}

func (b *astbuilder) buildPackge(packag *gol.PackageClauseContext) {
    recoverRange := b.SetRange(packag.BaseParserRuleContext)
	defer recoverRange()
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
		b.buildVarDecl(varDecl.(*gol.VarDeclContext), false)
	}
	if typeDecl := decl.TypeDecl(); typeDecl != nil {
		b.buildTypeDecl(typeDecl.(*gol.TypeDeclContext))
	}
}

func (b *astbuilder) buildConstDecl(constDecl *gol.ConstDeclContext) {
    
}

func (b *astbuilder) buildVarDecl(varDecl *gol.VarDeclContext,left bool) {
	recoverRange := b.SetRange(varDecl.BaseParserRuleContext)
	defer recoverRange()
	for _,v := range varDecl.AllVarSpec() {
	    b.buildVarSpec(v.(*gol.VarSpecContext), left)
	}
}

func (b *astbuilder) buildVarSpec(varSpec *gol.VarSpecContext, left bool){
    recoverRange := b.SetRange(varSpec.BaseParserRuleContext)
	defer recoverRange()

	a := varSpec.ASSIGN()

	if a == nil && !left { // right && not assign
		ssa.NewAny()
	}else{ // left || have assign
		assignValue := func() {	
			var leftvl []*ssa.Variable
			var rightvl []ssa.Value

			leftList := varSpec.IdentifierList().(*gol.IdentifierListContext).AllIDENTIFIER();
			rightList := varSpec.ExpressionList().(*gol.ExpressionListContext).AllExpression();

			for _, value := range leftList {
			    leftvl = append(leftvl, b.CreateLocalVariable(value.GetText()))
			}
			for _, value := range rightList {
				rightv, _ := b.buildExpression(value.(*gol.ExpressionContext),false)
				rightvl = append(rightvl, rightv)
			}
			b.AssignList(leftvl, rightvl)
		}

		assignValue()
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
	fmt.Printf("exp: %v\n", exp.GetText())

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.Expression(i); s != nil {
			rightv, _ := b.buildExpression(s.(*gol.ExpressionContext),IslValue)
			return rightv
		} else {
			return nil
		}
	}

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
		return b.buildOperandExpression(ret, IslValue)
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

	if expl := arg.ExpressionList(); expl != nil {
		for _, exp := range expl.(*gol.ExpressionListContext).AllExpression(){
			rv, _ := b.buildExpression(exp.(*gol.ExpressionContext), false)
			args = append(args, rv)
		}
	}

	return args
}

func (b *astbuilder) buildTypeDecl(typeDecl *gol.TypeDeclContext) {
    
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

		variable := b.CreateVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}
	return newFunc
}

func (b *astbuilder) buildMethodDecl(meth *gol.MethodDeclContext) ssa.Value {
	recoverRange := b.SetRange(meth.BaseParserRuleContext)
	defer recoverRange()

	methName := ""
	if Name := meth.IDENTIFIER(); Name != nil {
		methName = Name.GetText()
		fmt.Printf("methName: %v\n", methName)
	}
	newMeth := b.NewFunc(methName)
	{
		recoverRange := b.SetRange(meth.BaseParserRuleContext)
		b.FunctionBuilder = b.PushFunction(newMeth)
		
		if para, ok := meth.Signature().(*gol.SignatureContext); ok {
			b.buildSignature(para)
		}

		if block, ok := meth.Block().(*gol.BlockContext); ok {
			b.buildBlock(block)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()

		recoverRange()
	}
	if methName != "" {
		variable := b.CreateVariable(methName)
		b.AssignVariable(variable, newMeth)
	}
	return newMeth
}

func (b *astbuilder) buildSignature(stmt *gol.SignatureContext) {
	recoverRange := b.SetRange(stmt.BaseParserRuleContext)
	defer recoverRange()

	if paras := stmt.Parameters(); paras != nil {
	    b.buildParameters(paras.(*gol.ParametersContext))
	}

	if rety := stmt.Result(); rety != nil {
	    b.buildResult(rety.(*gol.ResultContext))
	}
}


func (b *astbuilder) buildParameters(parms *gol.ParametersContext) {
    recoverRange := b.SetRange(parms.BaseParserRuleContext)
	defer recoverRange()

	if f := parms.AllParameterDecl(); f != nil {
		for _, i := range f {
			if a, ok := i.(*gol.ParameterDeclContext); ok {
				b.buildParameterDecl(a)
			}
		}
		return
	}

	b.NewError(ssa.Error, TAG, ArrowFunctionNeedExpressionOrBlock())
}

func (b *astbuilder) buildParameterDecl(para *gol.ParameterDeclContext) {
	recoverRange := b.SetRange(para.BaseParserRuleContext)
	defer recoverRange()

	var typeType ssa.Type = nil
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


func (b *astbuilder) buildResult(rety *gol.ResultContext) {
    recoverRange := b.SetRange(rety.BaseParserRuleContext)
	defer recoverRange()
	var typeType ssa.Type = nil

	if typ := rety.Type_(); typ != nil {
	    typeType = b.buildType(typ.(*gol.Type_Context))
	}

	if paras := rety.Parameters(); paras != nil {
	    b.buildParameters(paras.(*gol.ParametersContext))
	}
	_ = typeType
}


func (b *astbuilder) buildBlock(block *gol.BlockContext) {
	recoverRange := b.SetRange(block.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := block.StatementList().(*gol.StatementListContext); ok {
		b.buildStatementList(s)
	} else {
		b.NewError(ssa.Warn, TAG, "empty block")
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
		if leftList, ok := stmt.ExpressionList().(*gol.ExpressionListContext); ok {
			for _,e := range leftList.AllExpression(){
				_,leftv := b.buildExpression(e.(*gol.ExpressionContext), true)
				lefts = append(lefts,leftv)
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
		}

		if leftList, ok := stmt.IdentifierList().(*gol.IdentifierListContext); ok {
			for _,i := range leftList.AllIDENTIFIER(){
				leftv := b.CreateVariable(i.GetText())
				lefts = append(lefts,leftv)
			}
			key, field, ok := b.EmitNext(value, stmt.DECLARE_ASSIGN() != nil)
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
		}

		return nil
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
		leftvl = append(leftvl, b.CreateLocalVariable(value.GetText()))
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
		leftvl = append(leftvl, b.CreateLocalVariable(value.GetText()))
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