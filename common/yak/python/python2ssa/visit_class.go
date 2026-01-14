package python2ssa

import (
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) handleClassInheritance(blueprint *ssa.Blueprint, arglist pythonparser.IArglistContext) {
	if arglist == nil {
		return
	}

	arglistCtx, ok := arglist.(*pythonparser.ArglistContext)
	if !ok {
		return
	}

	for _, argument := range arglistCtx.AllArgument() {
		if argCtx, ok := argument.(*pythonparser.ArgumentContext); ok {
			tests := argCtx.AllTest()
			if len(tests) == 0 {
				continue
			}
			test := tests[0]
			parentName := test.GetText()

			parentBp := b.GetBluePrint(parentName)
			if parentBp == nil {
				parentBp = b.CreateBlueprint(parentName, test)
				b.GetProgram().SetExportType(parentName, parentBp)
			}
			parentBp.SetKind(ssa.BlueprintClass)
			blueprint.AddParentBlueprint(parentBp)
		}
	}
}

func (b *singleFileBuilder) visitClassBody(suite pythonparser.ISuiteContext, blueprint *ssa.Blueprint) {
	suiteCtx, ok := suite.(*pythonparser.SuiteContext)
	if !ok {
		return
	}

	blueprint.AddLazyBuilder(func() {
		if simpleStmt := suiteCtx.Simple_stmt(); simpleStmt != nil {
			b.visitClassSimpleStmt(simpleStmt, blueprint)
		} else if stmts := suiteCtx.AllStmt(); len(stmts) > 0 {
			for _, stmt := range stmts {
				b.visitClassStmt(stmt, blueprint)
			}
		}
	})
}

func (b *singleFileBuilder) visitClassSimpleStmt(simpleStmt pythonparser.ISimple_stmtContext, blueprint *ssa.Blueprint) {
	if simpleStmt == nil {
		return
	}

	simpleStmtCtx, ok := simpleStmt.(*pythonparser.Simple_stmtContext)
	if !ok {
		return
	}

	for _, smallStmt := range simpleStmtCtx.AllSmall_stmt() {
		b.visitClassSmallStmt(smallStmt, blueprint)
	}
}

func (b *singleFileBuilder) visitClassSmallStmt(smallStmt pythonparser.ISmall_stmtContext, blueprint *ssa.Blueprint) {
	if smallStmt == nil {
		return
	}

	switch stmt := smallStmt.(type) {
	case *pythonparser.Expr_stmtContext:
		b.visitClassConstant(stmt, blueprint)
	}
}

func (b *singleFileBuilder) visitClassStmt(stmt pythonparser.IStmtContext, blueprint *ssa.Blueprint) {
	if stmt == nil {
		return
	}

	stmtCtx, ok := stmt.(*pythonparser.StmtContext)
	if !ok {
		return
	}

	if simpleStmt := stmtCtx.Simple_stmt(); simpleStmt != nil {
		b.visitClassSimpleStmt(simpleStmt, blueprint)
	} else if compoundStmt := stmtCtx.Compound_stmt(); compoundStmt != nil {
		if classOrFuncDef, ok := compoundStmt.(*pythonparser.Class_or_func_def_stmtContext); ok {
			if funcdef := classOrFuncDef.Funcdef(); funcdef != nil {
				if funcdefCtx, ok := funcdef.(*pythonparser.FuncdefContext); ok {
					b.visitClassMethod(funcdefCtx, blueprint)
				}
			}
		}
	}
}

func (b *singleFileBuilder) visitClassConstant(exprStmt *pythonparser.Expr_stmtContext, blueprint *ssa.Blueprint) {
	if exprStmt == nil || blueprint == nil {
		return
	}

	testlistStarExpr := exprStmt.Testlist_star_expr()
	if testlistStarExpr == nil {
		return
	}

	testlistStar, ok := testlistStarExpr.(*pythonparser.Testlist_star_exprContext)
	if !ok {
		return
	}

	tests := testlistStar.AllTest()
	if len(tests) == 0 {
		return
	}

	test := tests[0]

	testCtx, ok := test.(*pythonparser.TestContext)
	if !ok {
		return
	}

	varName := b.extractVariableName(testCtx)
	if varName == "" {
		return
	}

	var value ssa.Value
	if assignPart := exprStmt.Assign_part(); assignPart != nil {
		assignPartCtx, ok := assignPart.(*pythonparser.Assign_partContext)
		if ok {
			rhsExprs := assignPartCtx.AllTestlist_star_expr()
			if len(rhsExprs) > 0 {
				rhs := b.VisitTestlistStarExpr(rhsExprs[0])
				if v, ok := rhs.(ssa.Value); ok {
					value = v
				}
			}
		}
	}

	if value == nil {
		value = b.EmitUndefined(varName)
	}

	blueprint.RegisterNormalMember(varName, value)
}

func (b *singleFileBuilder) visitClassMethod(funcdef *pythonparser.FuncdefContext, blueprint *ssa.Blueprint) {
	if funcdef == nil || blueprint == nil {
		return
	}

	nameCtx := funcdef.Name()
	if nameCtx == nil {
		return
	}
	methodName := nameCtx.GetText()

	funcName := blueprint.Name + "_" + methodName
	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(methodName)

	isConstructor := (methodName == "__init__")

	if isConstructor {
		blueprint.RegisterMagicMethod(ssa.Constructor, newFunc)
	} else {
		blueprint.RegisterNormalMethod(methodName, newFunc)
	}

	newFunc.AddLazyBuilder(func() {
		b.FunctionBuilder = b.PushFunction(newFunc)

		selfParam := b.NewParam("self")
		selfParam.SetType(blueprint)

		if params := funcdef.Typedargslist(); params != nil {
			b.buildFuncParams(params)
		}

		if isConstructor {
			instance := b.EmitEmptyContainer()
			instance.SetType(blueprint)
			selfVar := b.CreateVariable("self")
			b.AssignVariable(selfVar, instance)
		}

		if suite := funcdef.Suite(); suite != nil {
			b.VisitSuite(suite)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}
