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

			// These relation names are SSA-internal bookkeeping members. Pre-register
			// them in the blueprint maps so Python class lowering does not rely on
			// SSA-side missing-member suppression.
			if parentContainer := parentBp.Container(); parentContainer != nil {
				blueprint.RegisterNormalMember(string(ssa.BlueprintRelationParents), parentContainer, false)
			}
			if childContainer := blueprint.Container(); childContainer != nil {
				parentBp.RegisterNormalMember(string(ssa.BlueprintRelationInherit), childContainer, false)
			}

			blueprint.AddParentBlueprint(parentBp)
		}
	}
}

func (b *singleFileBuilder) visitClassBody(suite pythonparser.ISuiteContext, blueprint *ssa.Blueprint) {
	suiteCtx, ok := suite.(*pythonparser.SuiteContext)
	if !ok {
		return
	}

	// Python classes are callable even without an explicit __init__. Pre-register
	// the constructor slot so synthesized constructor storage stays Python-local.
	b.ensureBlueprintConstructorSlot(blueprint)

	// Register methods synchronously so the constructor is visible to call sites
	// in the same scope. Method bodies are still compiled lazily (AddLazyBuilder).
	if simpleStmt := suiteCtx.Simple_stmt(); simpleStmt != nil {
		b.visitClassSimpleStmt(simpleStmt, blueprint)
	} else if stmts := suiteCtx.AllStmt(); len(stmts) > 0 {
		for _, stmt := range stmts {
			b.visitClassStmt(stmt, blueprint)
		}
	}
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
					modifier := b.extractMethodModifier(classOrFuncDef)
					b.visitClassMethodWithModifier(funcdefCtx, blueprint, modifier)
				}
			} else if nestedClassdef := classOrFuncDef.Classdef(); nestedClassdef != nil {
				if nestedClassdefCtx, ok := nestedClassdef.(*pythonparser.ClassdefContext); ok {
					b.visitNestedClassdef(nestedClassdefCtx, blueprint)
				}
			}
		}
	}
}

func (b *singleFileBuilder) extractMethodModifier(raw *pythonparser.Class_or_func_def_stmtContext) string {
	if raw == nil {
		return ""
	}
	decorators := raw.AllDecorator()
	for _, dec := range decorators {
		decCtx, ok := dec.(*pythonparser.DecoratorContext)
		if !ok {
			continue
		}
		dottedName := decCtx.Dotted_name()
		if dottedName == nil {
			continue
		}
		decName := dottedName.GetText()
		switch decName {
		case "staticmethod", "classmethod", "property":
			return decName
		}
	}
	return ""
}

func (b *singleFileBuilder) visitNestedClassdef(classdef *pythonparser.ClassdefContext, parentBlueprint *ssa.Blueprint) {
	if classdef == nil {
		return
	}
	nameCtx := classdef.Name()
	if nameCtx == nil {
		return
	}
	className := parentBlueprint.Name + "." + nameCtx.GetText()
	arglist := classdef.Arglist()
	suite := classdef.Suite()
	if suite == nil {
		return
	}
	nestedBp := b.CreateBlueprint(className, classdef)
	nestedBp.SetKind(ssa.BlueprintClass)
	b.GetProgram().SetExportType(className, nestedBp)
	b.handleClassInheritance(nestedBp, arglist)
	b.visitClassBody(suite, nestedBp)
	blueprintValue := nestedBp.Container()
	if blueprintValue == nil {
		return
	}
	parentBlueprint.RegisterNormalMember(nameCtx.GetText(), blueprintValue, false)
	parentBlueprint.RegisterStaticMember(nameCtx.GetText(), blueprintValue, false)
	b.syncBlueprintContainerMember(parentBlueprint, nameCtx.GetText(), blueprintValue)
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
	targets := b.extractLeftTargets(testlistStar)
	if len(targets) == 0 {
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
		value = b.EmitUndefined("class_member")
	}

	for _, target := range targets {
		if target.memberVar != nil || target.varName == "" {
			continue
		}
		blueprint.RegisterNormalMember(target.varName, value)
		blueprint.RegisterStaticMember(target.varName, value)
		b.syncBlueprintContainerMember(blueprint, target.varName, value)
	}
}

func (b *singleFileBuilder) visitClassMethod(funcdef *pythonparser.FuncdefContext, blueprint *ssa.Blueprint) {
	b.visitClassMethodWithModifier(funcdef, blueprint, "")
}

func (b *singleFileBuilder) visitClassMethodWithModifier(funcdef *pythonparser.FuncdefContext, blueprint *ssa.Blueprint, modifier string) {
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
	isStaticMethod := (modifier == "staticmethod")
	isClassMethod := (modifier == "classmethod")

	if !isConstructor {
		// Pre-register method names in the class member map so later blueprint
		// bookkeeping writes do not surface as user-facing invalid-member errors.
		blueprint.RegisterNormalMember(methodName, newFunc, false)
	}

	switch {
	case isConstructor:
		// Pre-register class name as NormalMember (store=false) so storeField inside
		// RegisterMagicMethod can resolve the member without emitting an InvalidField error.
		blueprint.RegisterNormalMember(blueprint.Name, newFunc, false)
		blueprint.RegisterMagicMethod(ssa.Constructor, newFunc)
	case isStaticMethod:
		blueprint.RegisterStaticMethod(methodName, newFunc)
		b.syncBlueprintContainerMember(blueprint, methodName, newFunc)
	case isClassMethod:
		blueprint.RegisterStaticMethod(methodName, newFunc)
		b.syncBlueprintContainerMember(blueprint, methodName, newFunc)
	default:
		blueprint.RegisterNormalMethod(methodName, newFunc)
		b.syncBlueprintContainerMember(blueprint, methodName, newFunc)
	}

	store := b.StoreFunctionBuilder()
	blueprint.AddLazyBuilder(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()

		b.FunctionBuilder = b.PushFunction(newFunc)
		b.MarkedThisClassBlueprint = blueprint
		defer func() {
			b.MarkedThisClassBlueprint = nil
		}()

		if isConstructor {
			// $self is a placeholder matching the call-site's prepended Undefined argument.
			// The real instance is created separately and bound to "self".
			b.NewParam("$self")
			instance := b.EmitMakeWithoutType(nil, nil)
			instance.SetType(blueprint)
			selfVar := b.CreateVariable("self")
			b.AssignVariable(selfVar, instance)

			if params := funcdef.Typedargslist(); params != nil {
				b.buildFuncParamsSkipFirst(params)
			}
		} else if isStaticMethod {
			if params := funcdef.Typedargslist(); params != nil {
				b.buildFuncParams(params)
			}
		} else if isClassMethod {
			if params := funcdef.Typedargslist(); params != nil {
				b.buildFuncParamsSkipFirst(params)
			}
		} else {
			selfParam := b.NewParam("self")
			selfParam.SetType(blueprint)
			selfVar := b.CreateVariable("self")
			b.AssignVariable(selfVar, selfParam)

			if params := funcdef.Typedargslist(); params != nil {
				b.buildFuncParamsSkipFirst(params)
			}
		}

		if suite := funcdef.Suite(); suite != nil {
			b.VisitSuite(suite)
		}

		if isConstructor {
			selfVal := b.ReadValue("self")
			if selfVal != nil {
				b.EmitReturn([]ssa.Value{selfVal})
			}
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}
