// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package pythonparser // PythonParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BasePythonParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BasePythonParserVisitor) VisitRoot(ctx *RootContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitSingle_input(ctx *Single_inputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitFile_input(ctx *File_inputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitEval_input(ctx *Eval_inputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitStmt(ctx *StmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitIf_stmt(ctx *If_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitWhile_stmt(ctx *While_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitFor_stmt(ctx *For_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTry_stmt(ctx *Try_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitWith_stmt(ctx *With_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitClass_or_func_def_stmt(ctx *Class_or_func_def_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitSuite(ctx *SuiteContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDecorator(ctx *DecoratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitElif_clause(ctx *Elif_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitElse_clause(ctx *Else_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitFinally_clause(ctx *Finally_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitWith_item(ctx *With_itemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitExcept_clause(ctx *Except_clauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitClassdef(ctx *ClassdefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitFuncdef(ctx *FuncdefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTypedargslist(ctx *TypedargslistContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitArgs(ctx *ArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitKwargs(ctx *KwargsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDef_parameters(ctx *Def_parametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDef_parameter(ctx *Def_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitNamed_parameter(ctx *Named_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitSimple_stmt(ctx *Simple_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitExpr_stmt(ctx *Expr_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitPrint_stmt(ctx *Print_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDel_stmt(ctx *Del_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitPass_stmt(ctx *Pass_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitBreak_stmt(ctx *Break_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitContinue_stmt(ctx *Continue_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitReturn_stmt(ctx *Return_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitRaise_stmt(ctx *Raise_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitYield_stmt(ctx *Yield_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitImport_stmt(ctx *Import_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitFrom_stmt(ctx *From_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitGlobal_stmt(ctx *Global_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitExec_stmt(ctx *Exec_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitAssert_stmt(ctx *Assert_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitNonlocal_stmt(ctx *Nonlocal_stmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTestlist_star_expr(ctx *Testlist_star_exprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitStar_expr(ctx *Star_exprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitAssign_part(ctx *Assign_partContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitExprlist(ctx *ExprlistContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitImport_as_names(ctx *Import_as_namesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitImport_as_name(ctx *Import_as_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDotted_as_names(ctx *Dotted_as_namesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDotted_as_name(ctx *Dotted_as_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTest(ctx *TestContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitVarargslist(ctx *VarargslistContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitVardef_parameters(ctx *Vardef_parametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitVardef_parameter(ctx *Vardef_parameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitVarargs(ctx *VarargsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitVarkwargs(ctx *VarkwargsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitLogical_test(ctx *Logical_testContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitComparison(ctx *ComparisonContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitExpr(ctx *ExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitAtom(ctx *AtomContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDictorsetmaker(ctx *DictorsetmakerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTestlist_comp(ctx *Testlist_compContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTestlist(ctx *TestlistContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitDotted_name(ctx *Dotted_nameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitName(ctx *NameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitNumber(ctx *NumberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitInteger(ctx *IntegerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitYield_expr(ctx *Yield_exprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitYield_arg(ctx *Yield_argContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitTrailer(ctx *TrailerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitArguments(ctx *ArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitArglist(ctx *ArglistContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitArgument(ctx *ArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitSubscriptlist(ctx *SubscriptlistContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitSubscript(ctx *SubscriptContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitSliceop(ctx *SliceopContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitComp_for(ctx *Comp_forContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePythonParserVisitor) VisitComp_iter(ctx *Comp_iterContext) interface{} {
	return v.VisitChildren(ctx)
}
