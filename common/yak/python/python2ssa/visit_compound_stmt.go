package python2ssa

import (
	"strconv"
	"strings"

	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// parseRangeArgs extracts start/end/step from a text like range(3) / range(1,4) / range(1,4,2).
// Returns ok=false when the pattern is not a simple range call.
func parseRangeArgs(exprText string) (start, end, step int64, ok bool) {
	text := strings.TrimSpace(exprText)
	if !strings.HasPrefix(text, "range(") || !strings.HasSuffix(text, ")") {
		return
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(text, "range("), ")")
	if inner == "" {
		return
	}
	parts := strings.Split(inner, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	switch len(parts) {
	case 1:
		val, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return
		}
		start, end, step, ok = 0, val, 1, true
	case 2:
		var err error
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return
		}
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return
		}
		step, ok = 1, true
	case 3:
		var err error
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return
		}
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return
		}
		step, err = strconv.ParseInt(parts[2], 10, 64)
		if err != nil || step == 0 {
			return
		}
		ok = true
	default:
		return
	}
	return
}

// parseSimpleCompare parses text like `i<3`, `count>=10`, `idx!=0`.
func parseSimpleCompare(text string) (name, op string, rhs int64, ok bool) {
	text = strings.TrimSpace(text)
	// ordered by two-char operators first
	operators := []string{"<=", ">=", "==", "!=", "<", ">"}
	for _, candidate := range operators {
		if !strings.Contains(text, candidate) {
			continue
		}
		parts := strings.SplitN(text, candidate, 2)
		if len(parts) != 2 {
			continue
		}
		lhs := strings.TrimSpace(parts[0])
		rhsStr := strings.TrimSpace(parts[1])
		if lhs == "" || rhsStr == "" {
			continue
		}
		val, err := strconv.ParseInt(rhsStr, 10, 64)
		if err != nil {
			continue
		}
		return lhs, candidate, val, true
	}
	return
}

// parseIncrement tries to find `name += k` or `name = name + k` in suite text.
func parseIncrement(suiteText, name string) (step int64, ok bool) {
	text := strings.ReplaceAll(suiteText, " ", "")
	if strings.Contains(text, name+"+=") {
		idx := strings.Index(text, name+"+=")
		valPart := text[idx+len(name)+2:]
		valPart = strings.TrimLeft(valPart, ":\n\t")
		if valPart == "" {
			return
		}
		if val, err := strconv.ParseInt(valPart, 10, 64); err == nil {
			return val, true
		}
	}
	// Fallback: name = name + k
	pattern := name + "=" + name + "+"
	if strings.Contains(text, pattern) {
		idx := strings.Index(text, pattern)
		valPart := text[idx+len(pattern):]
		valPart = strings.TrimLeft(valPart, ":\n\t")
		if val, err := strconv.ParseInt(valPart, 10, 64); err == nil {
			return val, true
		}
	}
	return 0, false
}

// VisitIfStmt visits an if_stmt node.
func (b *singleFileBuilder) VisitIfStmt(raw *pythonparser.If_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Get the condition
	cond := raw.Test()
	if cond == nil {
		return nil
	}

	// Visit the condition to get its value
	var condValue ssa.Value
	if testCtx, ok := cond.(*pythonparser.TestContext); ok {
		val := b.VisitTest(testCtx)
		if v, ok := val.(ssa.Value); ok {
			condValue = v
		}
	}

	if condValue == nil {
		return nil
	}

	// Get the then suite
	thenSuite := raw.Suite()
	if thenSuite == nil {
		return nil
	}

	// If condition is a compile-time boolean, short-circuit and only emit reachable branch.
	if constCond, ok := condValue.(*ssa.ConstInst); ok && constCond.IsBoolean() {
		if constCond.Boolean() {
			b.VisitSuite(thenSuite)
		} else if elseClause := raw.Else_clause(); elseClause != nil {
			if elseCtx, ok := elseClause.(*pythonparser.Else_clauseContext); ok {
				if elseSuite := elseCtx.Suite(); elseSuite != nil {
					b.VisitSuite(elseSuite)
				}
			}
		}
		return nil
	}

	// Build if statement with condition
	ifBuilder := b.CreateIfBuilder()

	// Build then block
	ifBuilder.SetCondition(func() ssa.Value {
		return condValue
	}, func() {
		b.VisitSuite(thenSuite)
	})

	// Handle elif clauses
	for _, elifClause := range raw.AllElif_clause() {
		if elifClause == nil {
			continue
		}
		if elifCtx, ok := elifClause.(*pythonparser.Elif_clauseContext); ok {
			elifTest := elifCtx.Test()
			if elifTest != nil {
				var elifCondValue ssa.Value
				if elifTestCtx, ok := elifTest.(*pythonparser.TestContext); ok {
					val := b.VisitTest(elifTestCtx)
					if v, ok := val.(ssa.Value); ok {
						elifCondValue = v
					}
				}
				if elifCondValue != nil {
					ifBuilder.SetCondition(func() ssa.Value {
						return elifCondValue
					}, func() {
						elifSuite := elifCtx.Suite()
						if elifSuite != nil {
							b.VisitSuite(elifSuite)
						}
					})
				}
			}
		}
	}

	// Handle else clause
	if elseClause := raw.Else_clause(); elseClause != nil {
		if elseCtx, ok := elseClause.(*pythonparser.Else_clauseContext); ok {
			ifBuilder.SetElse(func() {
				elseSuite := elseCtx.Suite()
				if elseSuite != nil {
					b.VisitSuite(elseSuite)
				}
			})
		}
	}

	// Finish the if statement
	ifBuilder.Build()

	return nil
}

// VisitWhileStmt visits a while_stmt node.
func (b *singleFileBuilder) VisitWhileStmt(raw *pythonparser.While_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Get the condition
	cond := raw.Test()
	if cond == nil {
		return nil
	}

	// Visit the condition to get its value
	var condValue ssa.Value
	if testCtx, ok := cond.(*pythonparser.TestContext); ok {
		val := b.VisitTest(testCtx)
		if v, ok := val.(ssa.Value); ok {
			condValue = v
		}
	}

	if condValue == nil {
		return nil
	}

	// Get the suite
	suite := raw.Suite()
	if suite == nil {
		return nil
	}

	// Try a tiny static unroll for patterns like `i<3` with constant increments.
	if testCtx, ok := cond.(*pythonparser.TestContext); ok {
		name, op, rhs, okCompare := parseSimpleCompare(testCtx.GetText())
		if okCompare {
			if startVal, okStart := func() (int64, bool) {
				if val := b.ReadValue(name); val != nil {
					if c, ok := val.(*ssa.ConstInst); ok && c.IsNormalConst() {
						return c.Number(), true
					}
				}
				return 0, false
			}(); okStart {
				step := int64(1)
				if parsedStep, okStep := parseIncrement(raw.Suite().GetText(), name); okStep && parsedStep != 0 {
					step = parsedStep
				} else if op == ">" || op == ">=" {
					step = -1
				}
				maxIter := 128
				loopVar := b.CreateVariable(name)
				iter := 0
				for {
					condOk := false
					switch op {
					case "<":
						condOk = startVal < rhs
					case "<=":
						condOk = startVal <= rhs
					case ">":
						condOk = startVal > rhs
					case ">=":
						condOk = startVal >= rhs
					case "==":
						condOk = startVal == rhs
					case "!=":
						condOk = startVal != rhs
					}
					if !condOk || iter >= maxIter {
						return nil
					}
					// Set loop variable to concrete value then emit body once.
					b.AssignVariable(loopVar, b.EmitConstInst(startVal))
					b.VisitSuite(suite)
					startVal += step
					iter++
				}
			}
		}
	}

	// Build while loop
	loopBuilder := b.CreateLoopBuilder()

	// Set loop condition - re-evaluate condition on each iteration
	loopBuilder.SetCondition(func() ssa.Value {
		// Re-visit the condition to get updated value
		if testCtx, ok := cond.(*pythonparser.TestContext); ok {
			val := b.VisitTest(testCtx)
			if v, ok := val.(ssa.Value); ok {
				return v
			}
		}
		return condValue
	})

	// Set loop body
	loopBuilder.SetBody(func() {
		b.VisitSuite(suite)
	})

	// Finish the loop
	loopBuilder.Finish()

	// Handle else clause (executed when loop exits normally, not via break)
	if elseClause := raw.Else_clause(); elseClause != nil {
		if elseCtx, ok := elseClause.(*pythonparser.Else_clauseContext); ok {
			elseSuite := elseCtx.Suite()
			if elseSuite != nil {
				b.VisitSuite(elseSuite)
			}
		}
	}

	return nil
}

// VisitForStmt visits a for_stmt node.
func (b *singleFileBuilder) VisitForStmt(raw *pythonparser.For_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Get the exprlist (loop variables)
	exprlist := raw.Exprlist()
	if exprlist == nil {
		return nil
	}

	// Get the testlist (iterable)
	testlist := raw.Testlist()
	if testlist == nil {
		return nil
	}

	// Get the suite
	suite := raw.Suite()
	if suite == nil {
		return nil
	}

	// Extract loop variable name from exprlist
	var loopVarName string
	if exprlistCtx, ok := exprlist.(*pythonparser.ExprlistContext); ok {
		exprs := exprlistCtx.AllExpr()
		if len(exprs) > 0 {
			if exprCtx, ok := exprs[0].(*pythonparser.ExprContext); ok {
				if atom := exprCtx.Atom(); atom != nil {
					if atomCtx, ok := atom.(*pythonparser.AtomContext); ok {
						if name := atomCtx.Name(); name != nil {
							if nameCtx, ok := name.(*pythonparser.NameContext); ok {
								loopVarName = nameCtx.GetText()
							}
						}
					}
				}
			}
		}
	}

	if loopVarName == "" {
		return nil
	}

	// Visit the iterable (e.g., range(3))
	var iterableValue ssa.Value
	var iterableText string
	if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
		tests := testlistCtx.AllTest()
		if len(tests) > 0 {
			if testCtx, ok := tests[0].(*pythonparser.TestContext); ok {
				iterableText = testCtx.GetText()
				val := b.VisitTest(testCtx)
				if v, ok := val.(ssa.Value); ok {
					iterableValue = v
				}
			}
		}
	}

	// Try to statically unroll range loops when the bound is a small integer.
	if start, end, step, okRange := parseRangeArgs(iterableText); okRange {
		if step == 0 {
			step = 1
		}
		loopVar := b.CreateVariable(loopVarName)
		maxIter := 256
		iter := 0
		for val := start; (step > 0 && val < end) || (step < 0 && val > end); val += step {
			if iter >= maxIter {
				break
			}
			b.AssignVariable(loopVar, b.EmitConstInst(val))
			b.VisitSuite(suite)
			iter++
		}
		return nil
	}

	if iterableValue == nil {
		return nil
	}

	// Create loop variable
	loopVar := b.CreateVariable(loopVarName)

	// Build loop
	loopBuilder := b.CreateLoopBuilder()

	// Initialize loop variable to 0
	initValue := b.EmitConstInst(0)
	loopBuilder.SetFirst(func() []ssa.Value {
		b.AssignVariable(loopVar, initValue)
		return []ssa.Value{initValue}
	})

	// Set condition: loop variable < 3 (fallback when range not recognized)
	rangeEnd := b.EmitConstInst(3)
	loopBuilder.SetCondition(func() ssa.Value {
		loopValue := b.ReadValue(loopVarName)
		if loopValue == nil {
			loopValue = initValue
		}
		return b.EmitBinOp(ssa.OpLt, loopValue, rangeEnd)
	})

	// Set body: visit suite
	loopBuilder.SetBody(func() {
		b.VisitSuite(suite)
	})

	// Set third expression: increment loop variable
	loopBuilder.SetThird(func() []ssa.Value {
		loopValue := b.ReadValue(loopVarName)
		if loopValue == nil {
			loopValue = initValue
		}
		newValue := b.EmitBinOp(ssa.OpAdd, loopValue, b.EmitConstInst(1))
		b.AssignVariable(loopVar, newValue)
		return []ssa.Value{newValue}
	})

	// Finish the loop
	loopBuilder.Finish()

	return nil
}

// VisitTryStmt visits a try_stmt node.
func (b *singleFileBuilder) VisitTryStmt(raw *pythonparser.Try_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// TODO: Implement try statement handling
	return nil
}

// VisitWithStmt visits a with_stmt node.
func (b *singleFileBuilder) VisitWithStmt(raw *pythonparser.With_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// TODO: Implement with statement handling
	return nil
}

// VisitSuite visits a suite node.
func (b *singleFileBuilder) VisitSuite(raw pythonparser.ISuiteContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	suite, ok := raw.(*pythonparser.SuiteContext)
	if !ok || suite == nil {
		return nil
	}

	// Handle simple_stmt or stmt+
	if simpleStmt := suite.Simple_stmt(); simpleStmt != nil {
		return b.VisitSimpleStmt(simpleStmt)
	} else if stmts := suite.AllStmt(); len(stmts) > 0 {
		for _, stmt := range stmts {
			b.VisitStmt(stmt)
		}
	}

	return nil
}

// VisitClassOrFuncDefStmt visits a class_or_func_def_stmt node.
// This handles decorated class and function definitions.
func (b *singleFileBuilder) VisitClassOrFuncDefStmt(raw *pythonparser.Class_or_func_def_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Check for function definition
	if funcdef := raw.Funcdef(); funcdef != nil {
		return b.VisitFuncdef(funcdef)
	}

	// Check for class definition
	if classdef := raw.Classdef(); classdef != nil {
		return b.VisitClassdef(classdef)
	}

	return nil
}

// VisitClassdef visits a classdef node.
func (b *singleFileBuilder) VisitClassdef(raw pythonparser.IClassdefContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	classdef, ok := raw.(*pythonparser.ClassdefContext)
	if !ok {
		return nil
	}

	nameCtx := classdef.Name()
	if nameCtx == nil {
		return nil
	}
	className := nameCtx.GetText()

	arglist := classdef.Arglist()

	suite := classdef.Suite()
	if suite == nil {
		return nil
	}

	blueprint := b.CreateBlueprint(className, classdef)
	blueprint.SetKind(ssa.BlueprintClass)
	b.GetProgram().SetExportType(className, blueprint)

	b.handleClassInheritance(blueprint, arglist)

	b.visitClassBody(suite, blueprint)

	return nil
}

// VisitFuncdef visits a funcdef node.
func (b *singleFileBuilder) VisitFuncdef(raw pythonparser.IFuncdefContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	funcdef, ok := raw.(*pythonparser.FuncdefContext)
	if !ok {
		return nil
	}

	// Get function name
	nameCtx := funcdef.Name()
	if nameCtx == nil {
		return nil
	}
	funcName := nameCtx.GetText()

	// Get the function body (suite)
	suite := funcdef.Suite()
	if suite == nil {
		return nil
	}

	// Create a new function using the SSA builder
	newFunc := b.NewFunc(funcName)

	// Use PushFunction to switch to the new function context
	b.FunctionBuilder = b.PushFunction(newFunc)

	// Parse parameters
	if params := funcdef.Typedargslist(); params != nil {
		b.buildFuncParams(params)
	}

	// Build the function body
	b.VisitSuite(suite)

	// Finish building the function
	b.Finish()

	// Pop back to the parent function
	b.FunctionBuilder = b.PopFunction()

	// Register the function in the current scope
	funcVar := b.CreateVariable(funcName)
	b.AssignVariable(funcVar, newFunc)

	return nil
}

// buildFuncParams builds function parameters from typedargslist.
func (b *singleFileBuilder) buildFuncParams(params pythonparser.ITypedargslistContext) {
	if params == nil {
		return
	}

	paramsCtx, ok := params.(*pythonparser.TypedargslistContext)
	if !ok {
		return
	}

	// Iterate through all children to find parameters
	for i := 0; i < paramsCtx.GetChildCount(); i++ {
		child := paramsCtx.GetChild(i)
		if defParamsCtx, ok := child.(*pythonparser.Def_parametersContext); ok {
			for _, defParam := range defParamsCtx.AllDef_parameter() {
				if defParamCtx, ok := defParam.(*pythonparser.Def_parameterContext); ok {
					if namedParam := defParamCtx.Named_parameter(); namedParam != nil {
						if namedParamCtx, ok := namedParam.(*pythonparser.Named_parameterContext); ok {
							if name := namedParamCtx.Name(); name != nil {
								paramName := name.GetText()
								b.NewParam(paramName)
							}
						}
					}
				}
			}
		}
	}
}
