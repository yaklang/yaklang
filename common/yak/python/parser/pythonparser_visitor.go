// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package pythonparser // PythonParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by PythonParser.
type PythonParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by PythonParser#root.
	VisitRoot(ctx *RootContext) interface{}

	// Visit a parse tree produced by PythonParser#single_input.
	VisitSingle_input(ctx *Single_inputContext) interface{}

	// Visit a parse tree produced by PythonParser#file_input.
	VisitFile_input(ctx *File_inputContext) interface{}

	// Visit a parse tree produced by PythonParser#eval_input.
	VisitEval_input(ctx *Eval_inputContext) interface{}

	// Visit a parse tree produced by PythonParser#stmt.
	VisitStmt(ctx *StmtContext) interface{}

	// Visit a parse tree produced by PythonParser#if_stmt.
	VisitIf_stmt(ctx *If_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#while_stmt.
	VisitWhile_stmt(ctx *While_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#for_stmt.
	VisitFor_stmt(ctx *For_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#try_stmt.
	VisitTry_stmt(ctx *Try_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#with_stmt.
	VisitWith_stmt(ctx *With_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#class_or_func_def_stmt.
	VisitClass_or_func_def_stmt(ctx *Class_or_func_def_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#suite.
	VisitSuite(ctx *SuiteContext) interface{}

	// Visit a parse tree produced by PythonParser#decorator.
	VisitDecorator(ctx *DecoratorContext) interface{}

	// Visit a parse tree produced by PythonParser#elif_clause.
	VisitElif_clause(ctx *Elif_clauseContext) interface{}

	// Visit a parse tree produced by PythonParser#else_clause.
	VisitElse_clause(ctx *Else_clauseContext) interface{}

	// Visit a parse tree produced by PythonParser#finally_clause.
	VisitFinally_clause(ctx *Finally_clauseContext) interface{}

	// Visit a parse tree produced by PythonParser#with_item.
	VisitWith_item(ctx *With_itemContext) interface{}

	// Visit a parse tree produced by PythonParser#except_clause.
	VisitExcept_clause(ctx *Except_clauseContext) interface{}

	// Visit a parse tree produced by PythonParser#classdef.
	VisitClassdef(ctx *ClassdefContext) interface{}

	// Visit a parse tree produced by PythonParser#funcdef.
	VisitFuncdef(ctx *FuncdefContext) interface{}

	// Visit a parse tree produced by PythonParser#typedargslist.
	VisitTypedargslist(ctx *TypedargslistContext) interface{}

	// Visit a parse tree produced by PythonParser#args.
	VisitArgs(ctx *ArgsContext) interface{}

	// Visit a parse tree produced by PythonParser#kwargs.
	VisitKwargs(ctx *KwargsContext) interface{}

	// Visit a parse tree produced by PythonParser#def_parameters.
	VisitDef_parameters(ctx *Def_parametersContext) interface{}

	// Visit a parse tree produced by PythonParser#def_parameter.
	VisitDef_parameter(ctx *Def_parameterContext) interface{}

	// Visit a parse tree produced by PythonParser#named_parameter.
	VisitNamed_parameter(ctx *Named_parameterContext) interface{}

	// Visit a parse tree produced by PythonParser#simple_stmt.
	VisitSimple_stmt(ctx *Simple_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#expr_stmt.
	VisitExpr_stmt(ctx *Expr_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#print_stmt.
	VisitPrint_stmt(ctx *Print_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#del_stmt.
	VisitDel_stmt(ctx *Del_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#pass_stmt.
	VisitPass_stmt(ctx *Pass_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#break_stmt.
	VisitBreak_stmt(ctx *Break_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#continue_stmt.
	VisitContinue_stmt(ctx *Continue_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#return_stmt.
	VisitReturn_stmt(ctx *Return_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#raise_stmt.
	VisitRaise_stmt(ctx *Raise_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#yield_stmt.
	VisitYield_stmt(ctx *Yield_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#import_stmt.
	VisitImport_stmt(ctx *Import_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#from_stmt.
	VisitFrom_stmt(ctx *From_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#global_stmt.
	VisitGlobal_stmt(ctx *Global_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#exec_stmt.
	VisitExec_stmt(ctx *Exec_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#assert_stmt.
	VisitAssert_stmt(ctx *Assert_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#nonlocal_stmt.
	VisitNonlocal_stmt(ctx *Nonlocal_stmtContext) interface{}

	// Visit a parse tree produced by PythonParser#testlist_star_expr.
	VisitTestlist_star_expr(ctx *Testlist_star_exprContext) interface{}

	// Visit a parse tree produced by PythonParser#star_expr.
	VisitStar_expr(ctx *Star_exprContext) interface{}

	// Visit a parse tree produced by PythonParser#assign_part.
	VisitAssign_part(ctx *Assign_partContext) interface{}

	// Visit a parse tree produced by PythonParser#exprlist.
	VisitExprlist(ctx *ExprlistContext) interface{}

	// Visit a parse tree produced by PythonParser#import_as_names.
	VisitImport_as_names(ctx *Import_as_namesContext) interface{}

	// Visit a parse tree produced by PythonParser#import_as_name.
	VisitImport_as_name(ctx *Import_as_nameContext) interface{}

	// Visit a parse tree produced by PythonParser#dotted_as_names.
	VisitDotted_as_names(ctx *Dotted_as_namesContext) interface{}

	// Visit a parse tree produced by PythonParser#dotted_as_name.
	VisitDotted_as_name(ctx *Dotted_as_nameContext) interface{}

	// Visit a parse tree produced by PythonParser#test.
	VisitTest(ctx *TestContext) interface{}

	// Visit a parse tree produced by PythonParser#varargslist.
	VisitVarargslist(ctx *VarargslistContext) interface{}

	// Visit a parse tree produced by PythonParser#vardef_parameters.
	VisitVardef_parameters(ctx *Vardef_parametersContext) interface{}

	// Visit a parse tree produced by PythonParser#vardef_parameter.
	VisitVardef_parameter(ctx *Vardef_parameterContext) interface{}

	// Visit a parse tree produced by PythonParser#varargs.
	VisitVarargs(ctx *VarargsContext) interface{}

	// Visit a parse tree produced by PythonParser#varkwargs.
	VisitVarkwargs(ctx *VarkwargsContext) interface{}

	// Visit a parse tree produced by PythonParser#logical_test.
	VisitLogical_test(ctx *Logical_testContext) interface{}

	// Visit a parse tree produced by PythonParser#comparison.
	VisitComparison(ctx *ComparisonContext) interface{}

	// Visit a parse tree produced by PythonParser#expr.
	VisitExpr(ctx *ExprContext) interface{}

	// Visit a parse tree produced by PythonParser#atom.
	VisitAtom(ctx *AtomContext) interface{}

	// Visit a parse tree produced by PythonParser#dictorsetmaker.
	VisitDictorsetmaker(ctx *DictorsetmakerContext) interface{}

	// Visit a parse tree produced by PythonParser#testlist_comp.
	VisitTestlist_comp(ctx *Testlist_compContext) interface{}

	// Visit a parse tree produced by PythonParser#testlist.
	VisitTestlist(ctx *TestlistContext) interface{}

	// Visit a parse tree produced by PythonParser#dotted_name.
	VisitDotted_name(ctx *Dotted_nameContext) interface{}

	// Visit a parse tree produced by PythonParser#name.
	VisitName(ctx *NameContext) interface{}

	// Visit a parse tree produced by PythonParser#number.
	VisitNumber(ctx *NumberContext) interface{}

	// Visit a parse tree produced by PythonParser#integer.
	VisitInteger(ctx *IntegerContext) interface{}

	// Visit a parse tree produced by PythonParser#yield_expr.
	VisitYield_expr(ctx *Yield_exprContext) interface{}

	// Visit a parse tree produced by PythonParser#yield_arg.
	VisitYield_arg(ctx *Yield_argContext) interface{}

	// Visit a parse tree produced by PythonParser#trailer.
	VisitTrailer(ctx *TrailerContext) interface{}

	// Visit a parse tree produced by PythonParser#arguments.
	VisitArguments(ctx *ArgumentsContext) interface{}

	// Visit a parse tree produced by PythonParser#arglist.
	VisitArglist(ctx *ArglistContext) interface{}

	// Visit a parse tree produced by PythonParser#argument.
	VisitArgument(ctx *ArgumentContext) interface{}

	// Visit a parse tree produced by PythonParser#subscriptlist.
	VisitSubscriptlist(ctx *SubscriptlistContext) interface{}

	// Visit a parse tree produced by PythonParser#subscript.
	VisitSubscript(ctx *SubscriptContext) interface{}

	// Visit a parse tree produced by PythonParser#sliceop.
	VisitSliceop(ctx *SliceopContext) interface{}

	// Visit a parse tree produced by PythonParser#comp_for.
	VisitComp_for(ctx *Comp_forContext) interface{}

	// Visit a parse tree produced by PythonParser#comp_iter.
	VisitComp_iter(ctx *Comp_iterContext) interface{}
}
