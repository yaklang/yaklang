package python2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
)

// VisitRoot visits the root node of the Python AST.
// This is the entry point for converting a Python file to SSA.
func (b *singleFileBuilder) VisitRoot(raw pythonparser.IRootContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	root, ok := raw.(*pythonparser.RootContext)
	if !ok || root == nil {
		return nil
	}

	// Python files can have file_input, single_input, or eval_input
	// For now, we'll handle file_input which is the most common case
	if fileInput := root.File_input(); fileInput != nil {
		b.VisitFileInput(fileInput)
	} else if singleInput := root.Single_input(); singleInput != nil {
		b.VisitSingleInput(singleInput)
	} else if evalInput := root.Eval_input(); evalInput != nil {
		b.VisitEvalInput(evalInput)
	}

	return nil
}

// VisitFileInput visits a file_input node (a complete Python file).
func (b *singleFileBuilder) VisitFileInput(raw pythonparser.IFile_inputContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	fileInput, ok := raw.(*pythonparser.File_inputContext)
	if !ok || fileInput == nil {
		log.Debugf("python2ssa: VisitFileInput: fileInput is nil or type assertion failed")
		return nil
	}

	// Visit all statements in the file
	stmts := fileInput.AllStmt()
	for _, stmt := range stmts {
		b.VisitStmt(stmt)
	}

	return nil
}

// VisitSingleInput visits a single_input node (a single statement).
func (b *singleFileBuilder) VisitSingleInput(raw pythonparser.ISingle_inputContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	singleInput, ok := raw.(*pythonparser.Single_inputContext)
	if !ok || singleInput == nil {
		return nil
	}

	// Single_input can have either Simple_stmt or Compound_stmt
	if simpleStmt := singleInput.Simple_stmt(); simpleStmt != nil {
		b.VisitSimpleStmt(simpleStmt)
	} else if compoundStmt := singleInput.Compound_stmt(); compoundStmt != nil {
		b.VisitCompoundStmt(compoundStmt)
	}

	return nil
}

// VisitEvalInput visits an eval_input node (an expression for evaluation).
func (b *singleFileBuilder) VisitEvalInput(raw pythonparser.IEval_inputContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	evalInput, ok := raw.(*pythonparser.Eval_inputContext)
	if !ok || evalInput == nil {
		return nil
	}

	// For eval_input, we visit the testlist
	if testlist := evalInput.Testlist(); testlist != nil {
		b.VisitTestlist(testlist)
	}

	return nil
}

// VisitStmt visits a statement node.
// This dispatches to either simple_stmt or compound_stmt handlers.
func (b *singleFileBuilder) VisitStmt(raw pythonparser.IStmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	stmt, ok := raw.(*pythonparser.StmtContext)
	if !ok || stmt == nil {
		return nil
	}

	// Handle simple_stmt or compound_stmt
	if simpleStmt := stmt.Simple_stmt(); simpleStmt != nil {
		b.VisitSimpleStmt(simpleStmt)
	} else if compoundStmt := stmt.Compound_stmt(); compoundStmt != nil {
		b.VisitCompoundStmt(compoundStmt)
	}

	return nil
}

// VisitSimpleStmt visits a simple_stmt node.
func (b *singleFileBuilder) VisitSimpleStmt(raw pythonparser.ISimple_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	simpleStmt, ok := raw.(*pythonparser.Simple_stmtContext)
	if !ok || simpleStmt == nil {
		return nil
	}

	// Visit all small_stmt nodes
	for _, smallStmt := range simpleStmt.AllSmall_stmt() {
		b.VisitSmallStmt(smallStmt)
	}

	return nil
}

// VisitSmallStmt visits a small_stmt node.
// This handles various small statements like assignments, expressions, etc.
func (b *singleFileBuilder) VisitSmallStmt(raw pythonparser.ISmall_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Use type switch to handle different statement types
	switch stmt := raw.(type) {
	case *pythonparser.Expr_stmtContext:
		return b.VisitExprStmt(stmt)
	case *pythonparser.Return_stmtContext:
		return b.VisitReturnStmt(stmt)
	case *pythonparser.Break_stmtContext:
		return b.VisitBreakStmt(stmt)
	case *pythonparser.Continue_stmtContext:
		return b.VisitContinueStmt(stmt)
	case *pythonparser.Pass_stmtContext:
		return b.VisitPassStmt(stmt)
	case *pythonparser.Import_stmtContext:
		return b.VisitImportStmt(stmt)
	case *pythonparser.From_stmtContext:
		return b.VisitFromStmt(stmt)
	case *pythonparser.Global_stmtContext:
		return b.VisitGlobalStmt(stmt)
	case *pythonparser.Nonlocal_stmtContext:
		return b.VisitNonlocalStmt(stmt)
	case *pythonparser.Assert_stmtContext:
		return b.VisitAssertStmt(stmt)
	case *pythonparser.Raise_stmtContext:
		return b.VisitRaiseStmt(stmt)
	case *pythonparser.Del_stmtContext:
		return b.VisitDelStmt(stmt)
	case *pythonparser.Print_stmtContext:
		return b.VisitPrintStmt(stmt)
	case *pythonparser.Exec_stmtContext:
		return b.VisitExecStmt(stmt)
	case *pythonparser.Yield_stmtContext:
		return b.VisitYieldStmt(stmt)
	}

	return nil
}

// VisitCompoundStmt visits a compound_stmt node.
// This handles compound statements like if, for, while, def, class, etc.
func (b *singleFileBuilder) VisitCompoundStmt(raw pythonparser.ICompound_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Use type switch to handle different compound statement types
	switch stmt := raw.(type) {
	case *pythonparser.If_stmtContext:
		return b.VisitIfStmt(stmt)
	case *pythonparser.While_stmtContext:
		return b.VisitWhileStmt(stmt)
	case *pythonparser.For_stmtContext:
		return b.VisitForStmt(stmt)
	case *pythonparser.Try_stmtContext:
		return b.VisitTryStmt(stmt)
	case *pythonparser.With_stmtContext:
		return b.VisitWithStmt(stmt)
	case *pythonparser.Class_or_func_def_stmtContext:
		return b.VisitClassOrFuncDefStmt(stmt)
	}

	return nil
}

// VisitTestlist visits a testlist node.
// This is used for expression lists in eval_input.
func (b *singleFileBuilder) VisitTestlist(raw pythonparser.ITestlistContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	testlist, ok := raw.(*pythonparser.TestlistContext)
	if !ok || testlist == nil {
		return nil
	}

	// Visit all tests in the testlist
	// Return the first value (for single expressions)
	var result interface{}
	for _, test := range testlist.AllTest() {
		if testCtx, ok := test.(*pythonparser.TestContext); ok {
			result = b.VisitTest(testCtx)
			// For now, just process the first test
			break
		}
	}

	return result
}

// VisitTestlistStarExpr visits a testlist_star_expr node.
// This is used for expressions and expression lists.
func (b *singleFileBuilder) VisitTestlistStarExpr(raw pythonparser.ITestlist_star_exprContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	testlistStarExpr, ok := raw.(*pythonparser.Testlist_star_exprContext)
	if !ok || testlistStarExpr == nil {
		return nil
	}

	// First check for testlist (for simple expressions like `x = 1` or `println(x)`)
	if testlist := testlistStarExpr.Testlist(); testlist != nil {
		return b.VisitTestlist(testlist)
	}

	// Visit all tests in the testlist_star_expr
	// Return the first value (for single expressions)
	tests := testlistStarExpr.AllTest()
	var result interface{}
	for _, test := range tests {
		if testCtx, ok := test.(*pythonparser.TestContext); ok {
			result = b.VisitTest(testCtx)
			// For now, just process the first test
			break
		}
	}

	return result
}
