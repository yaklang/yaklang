package python2ssa

import (
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) createVar(name string) *ssa.Variable {
	if b.globalNames != nil {
		if b.globalNames[name] {
			return b.CreateVariableCross(name)
		}
	}
	return b.CreateVariable(name)
}

// VisitExprStmt visits an expr_stmt node.
// This handles assignments and expression statements.
func (b *singleFileBuilder) VisitExprStmt(raw *pythonparser.Expr_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Get the testlist_star_expr (left side)
	testlistStarExpr := raw.Testlist_star_expr()
	if testlistStarExpr == nil {
		return nil
	}

	// Get the assign_part (right side, if any)
	assignPart := raw.Assign_part()
	if assignPart == nil {
		// This is just an expression statement (e.g., function call)
		// Visit the expression to generate the call
		if testlistStarExprCtx, ok := testlistStarExpr.(*pythonparser.Testlist_star_exprContext); ok {
			result := b.VisitTestlistStarExpr(testlistStarExprCtx)
			// Ensure the result is processed (for side effects like function calls)
			_ = result
		}
		return nil
	}

	// Type assert to concrete types
	left, leftOk := testlistStarExpr.(*pythonparser.Testlist_star_exprContext)
	right, rightOk := assignPart.(*pythonparser.Assign_partContext)
	if !leftOk || !rightOk {
		return nil
	}

	// This is an assignment
	return b.VisitAssignPart(left, right)
}

// VisitAssignPart visits an assign_part node.
// This handles assignment operations.
func (b *singleFileBuilder) VisitAssignPart(left *pythonparser.Testlist_star_exprContext, right *pythonparser.Assign_partContext) interface{} {
	if b == nil || left == nil || right == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(right)
	defer recoverRange()

	// Handle different types of assignments
	if len(right.AllASSIGN()) > 0 {
		// Simple assignment
		return b.VisitSimpleAssignment(left, right)
	}

	// Check for augmented assignment operators
	if right.ADD_ASSIGN() != nil || right.SUB_ASSIGN() != nil || right.MULT_ASSIGN() != nil ||
		right.DIV_ASSIGN() != nil || right.MOD_ASSIGN() != nil || right.IDIV_ASSIGN() != nil ||
		right.AND_ASSIGN() != nil || right.OR_ASSIGN() != nil || right.XOR_ASSIGN() != nil ||
		right.LEFT_SHIFT_ASSIGN() != nil || right.RIGHT_SHIFT_ASSIGN() != nil ||
		right.POWER_ASSIGN() != nil || right.AT_ASSIGN() != nil {
		// Augmented assignment
		return b.VisitAugassign(left, right)
	}

	return nil
}

// collectRightValues collects all right-hand side values from an assign_part.
func (b *singleFileBuilder) collectRightValues(assignPart *pythonparser.Assign_partContext) []ssa.Value {
	var rightValues []ssa.Value
	for _, rightExpr := range assignPart.AllTestlist_star_expr() {
		rightExprCtx, ok := rightExpr.(*pythonparser.Testlist_star_exprContext)
		if !ok {
			continue
		}
		if testlist := rightExprCtx.Testlist(); testlist != nil {
			if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
				for _, test := range testlistCtx.AllTest() {
					if testCtx, ok := test.(*pythonparser.TestContext); ok {
						if v, ok := b.VisitTest(testCtx).(ssa.Value); ok {
							rightValues = append(rightValues, v)
						}
					}
				}
			}
		} else {
			tests := rightExprCtx.AllTest()
			if len(tests) > 0 {
				for _, test := range tests {
					if testCtx, ok := test.(*pythonparser.TestContext); ok {
						if v, ok := b.VisitTest(testCtx).(ssa.Value); ok {
							rightValues = append(rightValues, v)
						}
					}
				}
			} else {
				if v, ok := b.VisitTestlistStarExpr(rightExprCtx).(ssa.Value); ok {
					rightValues = append(rightValues, v)
				}
			}
		}
	}
	return rightValues
}

// extractLeftTargets extracts left-hand side assignment targets from a testlist_star_expr.
// Returns two slices: member variables (e.g., self.x) and plain variable names.
// Each entry in memberVars corresponds to the matching index in the leftTests slice.
type assignTarget struct {
	memberVar *ssa.Variable // non-nil when this is a member access (e.g. self.x)
	varName   string        // plain variable name (e.g. x)
}

func (b *singleFileBuilder) extractLeftTargets(left *pythonparser.Testlist_star_exprContext) []assignTarget {
	var targets []assignTarget
	collectTest := func(testCtx *pythonparser.TestContext) {
		if mv := b.extractMemberCallVariable(testCtx); mv != nil {
			targets = append(targets, assignTarget{memberVar: mv})
		} else if varName := b.extractVariableName(testCtx); varName != "" {
			targets = append(targets, assignTarget{varName: varName})
		}
	}
	if testlist := left.Testlist(); testlist != nil {
		if tlCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
			for _, test := range tlCtx.AllTest() {
				if tc, ok := test.(*pythonparser.TestContext); ok {
					collectTest(tc)
				}
			}
		}
	} else {
		for _, test := range left.AllTest() {
			if tc, ok := test.(*pythonparser.TestContext); ok {
				collectTest(tc)
			}
		}
	}
	return targets
}

// assignToTarget performs the actual SSA variable assignment for a single target.
func (b *singleFileBuilder) assignToTarget(target assignTarget, value ssa.Value) {
	if target.memberVar != nil {
		b.AssignVariable(target.memberVar, value)
	} else if target.varName != "" {
		variable := b.createVar(target.varName)
		b.AssignVariable(variable, value)
	}
}

// VisitSimpleAssignment visits a simple assignment.
func (b *singleFileBuilder) VisitSimpleAssignment(left *pythonparser.Testlist_star_exprContext, assignPart *pythonparser.Assign_partContext) interface{} {
	if b == nil || left == nil || assignPart == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(assignPart)
	defer recoverRange()

	rightExprs := assignPart.AllTestlist_star_expr()
	if len(rightExprs) == 0 {
		return nil
	}

	rightValues := b.collectRightValues(assignPart)
	targets := b.extractLeftTargets(left)

	// Chain assignment: x = y = 10 (multiple ASSIGN tokens in assign_part)
	if len(rightExprs) > 1 {
		if len(rightValues) == 0 {
			return nil
		}
		lastValue := rightValues[len(rightValues)-1]
		// Assign to intermediate targets (indices 0..n-2 of rightExprs map to variables)
		for i := 0; i < len(rightExprs)-1; i++ {
			if rightExprCtx, ok := rightExprs[i].(*pythonparser.Testlist_star_exprContext); ok {
				for _, interTarget := range b.extractLeftTargets(rightExprCtx) {
					b.assignToTarget(interTarget, lastValue)
				}
			}
		}
		for _, target := range targets {
			b.assignToTarget(target, lastValue)
		}
		return nil
	}

	if len(targets) == 0 || len(rightValues) == 0 {
		return nil
	}

	if len(targets) == len(rightValues) {
		for i, target := range targets {
			b.assignToTarget(target, rightValues[i])
		}
	} else {
		// Single RHS or count mismatch: assign first value to all targets
		for _, target := range targets {
			b.assignToTarget(target, rightValues[0])
		}
	}

	return nil
}

// extractVariableName extracts the variable name from a test context.
func (b *singleFileBuilder) extractVariableName(testCtx *pythonparser.TestContext) string {
	if testCtx == nil {
		return ""
	}

	logicalTests := testCtx.AllLogical_test()
	if len(logicalTests) == 0 {
		return ""
	}

	ltCtx, ok := logicalTests[0].(*pythonparser.Logical_testContext)
	if !ok {
		return ""
	}

	comparison := ltCtx.Comparison()
	if comparison == nil {
		return ""
	}

	compCtx, ok := comparison.(*pythonparser.ComparisonContext)
	if !ok {
		return ""
	}

	expr := compCtx.Expr()
	if expr == nil {
		return ""
	}

	exprCtx, ok := expr.(*pythonparser.ExprContext)
	if !ok {
		return ""
	}

	atom := exprCtx.Atom()
	if atom == nil {
		return ""
	}

	atomCtx, ok := atom.(*pythonparser.AtomContext)
	if !ok {
		return ""
	}

	name := atomCtx.Name()
	if name == nil {
		return ""
	}

	nameCtx, ok := name.(*pythonparser.NameContext)
	if !ok {
		return ""
	}

	return nameCtx.GetText()
}

// VisitAnnassign visits an annotated assignment.
func (b *singleFileBuilder) VisitAnnassign(left *pythonparser.Testlist_star_exprContext, assignPart *pythonparser.Assign_partContext) interface{} {
	// TODO: Implement annotated assignment handling
	return nil
}

// VisitAugassign visits an augmented assignment.
func (b *singleFileBuilder) VisitAugassign(left *pythonparser.Testlist_star_exprContext, assignPart *pythonparser.Assign_partContext) interface{} {
	if b == nil || left == nil || assignPart == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(assignPart)
	defer recoverRange()

	// Extract variable name from left side
	var varName string
	if testlist := left.Testlist(); testlist != nil {
		if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
			if len(testlistCtx.AllTest()) > 0 {
				if testCtx, ok := testlistCtx.AllTest()[0].(*pythonparser.TestContext); ok {
					varName = b.extractVariableName(testCtx)
				}
			}
		}
	} else {
		tests := left.AllTest()
		if len(tests) > 0 {
			if testCtx, ok := tests[0].(*pythonparser.TestContext); ok {
				varName = b.extractVariableName(testCtx)
			}
		}
	}

	if varName == "" {
		return nil
	}

	// Get the right-hand side value
	var rightValue ssa.Value
	if testlist := assignPart.Testlist(); testlist != nil {
		if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
			if len(testlistCtx.AllTest()) > 0 {
				if testCtx, ok := testlistCtx.AllTest()[0].(*pythonparser.TestContext); ok {
					val := b.VisitTest(testCtx)
					if v, ok := val.(ssa.Value); ok {
						rightValue = v
					}
				}
			}
		}
	}

	if rightValue == nil {
		return nil
	}

	// Read the current value of the variable
	var leftValue ssa.Value
	if varVal := b.ReadValue(varName); varVal != nil {
		leftValue = varVal
	} else {
		// Variable doesn't exist, create it with default value 0
		leftValue = b.EmitConstInst(0)
	}

	// Determine the operation based on the operator
	var op ssa.BinaryOpcode
	if assignPart.ADD_ASSIGN() != nil {
		op = ssa.OpAdd
	} else if assignPart.SUB_ASSIGN() != nil {
		op = ssa.OpSub
	} else if assignPart.MULT_ASSIGN() != nil {
		op = ssa.OpMul
	} else if assignPart.DIV_ASSIGN() != nil {
		op = ssa.OpDiv
	} else if assignPart.MOD_ASSIGN() != nil {
		op = ssa.OpMod
	} else if assignPart.IDIV_ASSIGN() != nil {
		op = ssa.OpDiv // Integer division
	} else if assignPart.POWER_ASSIGN() != nil {
		op = ssa.OpPow
	} else if assignPart.LEFT_SHIFT_ASSIGN() != nil {
		op = ssa.OpShl
	} else if assignPart.RIGHT_SHIFT_ASSIGN() != nil {
		op = ssa.OpShr
	} else if assignPart.AND_ASSIGN() != nil {
		op = ssa.OpAnd
	} else if assignPart.OR_ASSIGN() != nil {
		op = ssa.OpOr
	} else if assignPart.XOR_ASSIGN() != nil {
		op = ssa.OpXor
	} else {
		return nil
	}

	// Perform the binary operation
	result := b.EmitBinOp(op, leftValue, rightValue)

	// Assign the result back to the variable
	variable := b.createVar(varName)
	b.AssignVariable(variable, result)

	return nil
}

// VisitReturnStmt visits a return_stmt node.
func (b *singleFileBuilder) VisitReturnStmt(raw *pythonparser.Return_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Get the return value(s)
	testlist := raw.Testlist()
	if testlist == nil {
		// Return without value
		b.EmitReturn(nil)
		return nil
	}

	// Visit the testlist to get return value(s)
	if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
		var returnValues []ssa.Value
		for _, test := range testlistCtx.AllTest() {
			if testCtx, ok := test.(*pythonparser.TestContext); ok {
				val := b.VisitTest(testCtx)
				if v, ok := val.(ssa.Value); ok {
					returnValues = append(returnValues, v)
				}
			}
		}
		if len(returnValues) > 0 {
			b.EmitReturn(returnValues)
		} else {
			b.EmitReturn(nil)
		}
	}

	return nil
}

// VisitBreakStmt visits a break_stmt node.
func (b *singleFileBuilder) VisitBreakStmt(raw *pythonparser.Break_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// TODO: Implement break statement handling
	return nil
}

// VisitContinueStmt visits a continue_stmt node.
func (b *singleFileBuilder) VisitContinueStmt(raw *pythonparser.Continue_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// TODO: Implement continue statement handling
	return nil
}

// VisitPassStmt visits a pass_stmt node.
func (b *singleFileBuilder) VisitPassStmt(raw *pythonparser.Pass_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Pass statement does nothing
	return nil
}

// VisitImportStmt visits an import_stmt node.
func (b *singleFileBuilder) VisitImportStmt(raw *pythonparser.Import_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement import statement handling
	return nil
}

// VisitFromStmt visits a from_stmt node.
func (b *singleFileBuilder) VisitFromStmt(raw *pythonparser.From_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement from statement handling
	return nil
}

// VisitGlobalStmt visits a global_stmt node.
func (b *singleFileBuilder) VisitGlobalStmt(raw *pythonparser.Global_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	if b.globalNames == nil {
		b.globalNames = make(map[string]bool)
	}
	for _, name := range raw.AllName() {
		if name != nil {
			b.globalNames[name.GetText()] = true
		}
	}
	return nil
}

// VisitNonlocalStmt visits a nonlocal_stmt node.
func (b *singleFileBuilder) VisitNonlocalStmt(raw *pythonparser.Nonlocal_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	if b.globalNames == nil {
		b.globalNames = make(map[string]bool)
	}
	for _, name := range raw.AllName() {
		if name != nil {
			b.globalNames[name.GetText()] = true
		}
	}
	return nil
}

// VisitAssertStmt visits an assert_stmt node.
func (b *singleFileBuilder) VisitAssertStmt(raw *pythonparser.Assert_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement assert statement handling
	return nil
}

// VisitRaiseStmt visits a raise_stmt node.
func (b *singleFileBuilder) VisitRaiseStmt(raw *pythonparser.Raise_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement raise statement handling
	return nil
}

// VisitDelStmt visits a del_stmt node.
func (b *singleFileBuilder) VisitDelStmt(raw *pythonparser.Del_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement del statement handling
	return nil
}

// VisitPrintStmt visits a print_stmt node.
func (b *singleFileBuilder) VisitPrintStmt(raw *pythonparser.Print_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement print statement handling
	return nil
}

// VisitExecStmt visits an exec_stmt node.
func (b *singleFileBuilder) VisitExecStmt(raw *pythonparser.Exec_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement exec statement handling
	return nil
}

// VisitYieldStmt visits a yield_stmt node.
func (b *singleFileBuilder) VisitYieldStmt(raw *pythonparser.Yield_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	// TODO: Implement yield statement handling
	return nil
}

// extractMemberCallVariable extracts member call variable from a test context (e.g., self.x).
// Returns nil if the expression is not a member call.
func (b *singleFileBuilder) extractMemberCallVariable(testCtx *pythonparser.TestContext) *ssa.Variable {
	if testCtx == nil {
		return nil
	}

	logicalTests := testCtx.AllLogical_test()
	if len(logicalTests) == 0 {
		return nil
	}

	ltCtx, ok := logicalTests[0].(*pythonparser.Logical_testContext)
	if !ok {
		return nil
	}

	comparison := ltCtx.Comparison()
	if comparison == nil {
		return nil
	}

	compCtx, ok := comparison.(*pythonparser.ComparisonContext)
	if !ok {
		return nil
	}

	expr := compCtx.Expr()
	if expr == nil {
		return nil
	}

	exprCtx, ok := expr.(*pythonparser.ExprContext)
	if !ok {
		return nil
	}

	atom := exprCtx.Atom()
	if atom == nil {
		return nil
	}

	atomCtx, ok := atom.(*pythonparser.AtomContext)
	if !ok {
		return nil
	}

	trailers := exprCtx.AllTrailer()
	if len(trailers) == 0 {
		return nil
	}

	trailer, ok := trailers[0].(*pythonparser.TrailerContext)
	if !ok || trailer.DOT() == nil {
		return nil
	}

	name := atomCtx.Name()
	if name == nil {
		return nil
	}

	nameCtx, ok := name.(*pythonparser.NameContext)
	if !ok {
		return nil
	}

	attrName := trailer.Name()
	if attrName == nil {
		return nil
	}

	attrNameCtx, ok := attrName.(*pythonparser.NameContext)
	if !ok {
		return nil
	}

	objName := nameCtx.GetText()
	obj := b.ReadValue(objName)
	if obj == nil {
		return nil
	}

	attrNameStr := attrNameCtx.GetText()
	key := b.EmitConstInst(attrNameStr)

	// Lazily register unknown attributes on Blueprint types (Python adds attrs at runtime).
	// store=false skips SSA instruction emission in the container's function context.
	if blueprint, ok := ssa.ToClassBluePrintType(obj.GetType()); ok {
		if existing := blueprint.GetNormalMember(attrNameStr); existing == nil {
			blueprint.RegisterNormalMember(attrNameStr, b.EmitUndefined(attrNameStr), false)
		}
	}

	return b.CreateMemberCallVariable(obj, key)
}
