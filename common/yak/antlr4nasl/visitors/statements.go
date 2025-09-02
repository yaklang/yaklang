package visitors

import (
	"github.com/yaklang/yaklang/common/log"
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

func (c *Compiler) VisitStatementList(i nasl.IStatementListContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	statements, ok := i.(*nasl.StatementListContext)
	if !ok {
		return
	}
	for _, statement := range statements.AllStatement() {
		c.VisitStatement(statement)
	}
}

func (c *Compiler) VisitStatement(i nasl.IStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	statement, ok := i.(*nasl.StatementContext)
	if !ok {
		return
	}
	if block := statement.Block(); block != nil {
		c.VisitBlock(block)
	}
	if ifStatement := statement.IfStatement(); ifStatement != nil {
		c.VisitIfStatement(ifStatement)
	}
	if iterationStatement := statement.IterationStatement(); iterationStatement != nil {
		c.VisitIterationStatement(iterationStatement)
	}
	if continueStatement := statement.ContinueStatement(); continueStatement != nil {
		c.VisitContinueStatement(continueStatement)
	}
	if breakStatement := statement.BreakStatement(); breakStatement != nil {
		c.VisitBreakStatement(breakStatement)
	}
	if returnStatement := statement.ReturnStatement(); returnStatement != nil {
		c.VisitReturnStatement(returnStatement)
	}
	if expressionStatement := statement.ExpressionStatement(); expressionStatement != nil {
		c.VisitExpressionStatement(expressionStatement)
	}
	if variableDeclarationStatement := statement.VariableDeclarationStatement(); variableDeclarationStatement != nil {
		c.VisitVariableDeclarationStatement(variableDeclarationStatement)
	}
	if variableAssignStatement := statement.VariableAssignStatement(); variableAssignStatement != nil {
		c.VisitVariableAssignStatement(variableAssignStatement)
	}
	if functionDeclarationStatement := statement.FunctionDeclarationStatement(); functionDeclarationStatement != nil {
		c.VisitFunctionDeclarationStatement(functionDeclarationStatement)
	}
	// if exitStatement := statement.ExitStatement(); exitStatement != nil {
	// 	c.VisitExitStatement(exitStatement)
	// }
}

func (c *Compiler) VisitBlock(i nasl.IBlockContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	block, ok := i.(*nasl.BlockContext)
	if !ok {
		return
	}
	//c.pushScope()
	if block.StatementList() != nil {
		c.VisitStatementList(block.StatementList())
	}
	//c.pushScopeEnd()
}

func (c *Compiler) VisitIfStatement(i nasl.IIfStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	ifStatement, ok := i.(*nasl.IfStatementContext)
	if !ok {
		return
	}

	c.VisitSingleExpression(ifStatement.SingleExpression())

	jmpF := c.pushJmpIfFalse()
	//c.pushScope("if")
	c.VisitStatement(ifStatement.Statement(0)) // if body
	//c.pushScopeEnd()
	jmp := c.pushJmp()
	jmpF.Unary = len(c.codes)
	if ifStatement.Else() != nil {
		//c.pushScope("else")
		c.VisitStatement(ifStatement.Statement(1)) // else body
		//c.pushScopeEnd()
	}
	jmp.Unary = len(c.codes)
}

func (c *Compiler) VisitContinueStatement(i nasl.IContinueStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	_, ok := i.(*nasl.ContinueStatementContext)
	if !ok {
		return
	}
	if c.TmpData.Len() != 0 {
		id := c.TmpData.Peek()
		d := id.(*vmstack.Stack)
		jmp := c.pushJmp()
		jmp.Unary = 2
		d.Push(jmp)
	} else {
		//panic("continue can only be used in loops")
	}
}

func (c *Compiler) VisitBreakStatement(i nasl.IBreakStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	_, ok := i.(*nasl.BreakStatementContext)
	if !ok {
		return
	}
	if c.TmpData.Len() != 0 {
		id := c.TmpData.Peek()
		d := id.(*vmstack.Stack)
		jmp := c.pushJmp()
		jmp.Unary = 1
		d.Push(jmp)
	} else {
		//panic("break can only be used in loops")
	}
}

func (c *Compiler) VisitReturnStatement(i nasl.IReturnStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	returnStatement, ok := i.(*nasl.ReturnStatementContext)
	if !ok {
		return
	}
	if exp := returnStatement.SingleExpression(); exp != nil {
		c.VisitSingleExpression(exp)
	}
	c.pushOpcodeFlag(yakvm.OpReturn)
}

func (c *Compiler) VisitExpressionStatement(i nasl.IExpressionStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	expressionStatement, ok := i.(*nasl.ExpressionStatementContext)
	if !ok {
		return
	}
	c.NeedPop(true)
	c.VisitExpressionSequence(expressionStatement.ExpressionSequence())
	if c.needPop {
		c.pushOpcodeFlag(yakvm.OpPop)
	}
	c.NeedPop(false)
}

func (c *Compiler) VisitVariableDeclarationStatement(i nasl.IVariableDeclarationStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	variableDeclarationStatement, ok := i.(*nasl.VariableDeclarationStatementContext)
	if !ok {
		return
	}
	_ = variableDeclarationStatement
	if localVar := variableDeclarationStatement.LocalVar(); localVar != nil {
		for _, identifier := range variableDeclarationStatement.AllIdentifier() {
			text := identifier.GetText()
			c.pushLeftRef(text)
			c.pushDeclare()
		}
	}
	if globalVar := variableDeclarationStatement.GlobalVar(); globalVar != nil {
		for _, identifier := range variableDeclarationStatement.AllIdentifier() {
			text := identifier.GetText()
			c.pushLeftRef(text)
			c.pushGlobalDeclare()
		}
	}
}

func (c *Compiler) VisitVariableAssignStatement(i nasl.IVariableAssignStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	variableAssignStatement, ok := i.(*nasl.VariableAssignStatementContext)
	if !ok {
		return
	}

	var isGlobal = variableAssignStatement.GlobalVar() != nil

	id := variableAssignStatement.Identifier().GetText()

	c.pushLeftRef(id)
	if isGlobal {
		c.pushGlobalDeclare()
	} else {
		c.pushDeclare()
	}

	if exp := variableAssignStatement.SingleExpression(); exp != nil {
		c.pushLeftRef(id)
		c.VisitSingleExpression(exp)
		c.pushAssigin()
	}
}

func (c *Compiler) VisitFunctionDeclarationStatement(i nasl.IFunctionDeclarationStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	functionDeclarationStatement, ok := i.(*nasl.FunctionDeclarationStatementContext)
	if !ok {
		return
	}
	functionName := functionDeclarationStatement.Identifier().GetText()
	c.pushLeftRef("__function__" + functionName)
	iparamList := functionDeclarationStatement.ParameterList()
	symbols := []int{}
	if iparamList != nil {
		paramList := iparamList.(*nasl.ParameterListContext)
		ids := paramList.AllIdentifier()
		for _, id := range ids {
			name := id.GetText()
			if sym, ok := c.symbolTable.GetSymbolByVariableName(name); ok {
				symbols = append(symbols, sym)
			} else {
				sym, err := c.symbolTable.NewSymbolWithReturn(name)
				if err != nil {
					log.Errorf("new symbol error: %v", err)
					continue
				}
				symbols = append(symbols, sym)
			}

		}
	}

	backPackCode := c.codes
	c.codes = []*yakvm.Code{}
	block := functionDeclarationStatement.Block()
	c.VisitBlock(block)

	fun := yakvm.NewFunction(c.codes, c.symbolTable)
	c.codes = backPackCode
	fun.SetName(functionName)
	fun.SetParamSymbols(symbols)
	c.pushValue(&yakvm.Value{
		TypeVerbose: functionName,
		Value:       fun,
	})
	c.pushAssigin()
}

// func (c *Compiler) VisitExitStatement(i nasl.IExitStatementContext) {
// 	if i == nil {
// 		return
// 	}
// 	c.visitHook(c, i)
// 	exitExp := i.(*nasl.ExitStatementContext)
// 	c.VisitSingleExpression(exitExp.SingleExpression())
// 	code := c.pushOpcodeFlag(yakvm.OpExit)
// 	var sourcePath string
// 	if code.SourceCodeFilePath != nil {
// 		sourcePath = *code.SourceCodeFilePath
// 	}
// 	code.Op1 = yakvm.NewAutoValue(map[string]string{
// 		"file name":   sourcePath,
// 		"line number": strconv.Itoa(code.StartLineNumber),
// 	})

// }
