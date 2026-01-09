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

// VisitSimpleAssignment visits a simple assignment.
func (b *singleFileBuilder) VisitSimpleAssignment(left *pythonparser.Testlist_star_exprContext, assignPart *pythonparser.Assign_partContext) interface{} {
	if b == nil || left == nil || assignPart == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(assignPart)
	defer recoverRange()

	// Get all right-hand side values
	rightExprs := assignPart.AllTestlist_star_expr()
	if len(rightExprs) == 0 {
		return nil
	}

	// Visit all right-hand side expressions
	var rightValues []ssa.Value
	for _, rightExpr := range rightExprs {
		if rightExprCtx, ok := rightExpr.(*pythonparser.Testlist_star_exprContext); ok {
			// Check if it's a testlist (multiple values: 1, 2)
			if testlist := rightExprCtx.Testlist(); testlist != nil {
				if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
					// Extract all values from testlist
					for _, test := range testlistCtx.AllTest() {
						if testCtx, ok := test.(*pythonparser.TestContext); ok {
							val := b.VisitTest(testCtx)
							if v, ok := val.(ssa.Value); ok {
								rightValues = append(rightValues, v)
							}
						}
					}
				}
			} else {
				// Check for multiple tests directly in testlist_star_expr: (test COMMA)+ test?
				tests := rightExprCtx.AllTest()
				if len(tests) > 0 {
					// Multiple values: 1, 2
					for _, test := range tests {
						if testCtx, ok := test.(*pythonparser.TestContext); ok {
							val := b.VisitTest(testCtx)
							if v, ok := val.(ssa.Value); ok {
								rightValues = append(rightValues, v)
							}
						}
					}
				} else {
					// Single value
					val := b.VisitTestlistStarExpr(rightExprCtx)
					if v, ok := val.(ssa.Value); ok {
						rightValues = append(rightValues, v)
					}
				}
			}
		}
	}

	// Get all left-hand side variables
	var leftVars []string
	if testlist := left.Testlist(); testlist != nil {
		// Multiple assignment: a, b = 1, 2
		if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
			for _, test := range testlistCtx.AllTest() {
				if testCtx, ok := test.(*pythonparser.TestContext); ok {
					varName := b.extractVariableName(testCtx)
					if varName != "" {
						leftVars = append(leftVars, varName)
					}
				}
			}
		}
	} else {
		// Single assignment: x = 1
		tests := left.AllTest()
		for _, test := range tests {
			if testCtx, ok := test.(*pythonparser.TestContext); ok {
				varName := b.extractVariableName(testCtx)
				if varName != "" {
					leftVars = append(leftVars, varName)
				}
			}
		}
	}

	// Handle chain assignment: x = y = 10
	// In this case, we have multiple ASSIGN tokens, and we assign the last value to all variables
	if len(rightExprs) > 1 {
		// Chain assignment: assign the last value to all left variables and intermediate variables
		lastValue := rightValues[len(rightValues)-1]

		// First, assign to intermediate variables (like y in x = y = 10)
		for i := 0; i < len(rightExprs)-1; i++ {
			rightExpr := rightExprs[i]
			if rightExprCtx, ok := rightExpr.(*pythonparser.Testlist_star_exprContext); ok {
				// Extract variable name from the intermediate expression
				if testlist := rightExprCtx.Testlist(); testlist != nil {
					if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
						for _, test := range testlistCtx.AllTest() {
							if testCtx, ok := test.(*pythonparser.TestContext); ok {
								varName := b.extractVariableName(testCtx)
								if varName != "" {
									variable := b.createVar(varName)
									b.AssignVariable(variable, lastValue)
								}
							}
						}
					}
				} else {
					// Try to extract variable name from test
					tests := rightExprCtx.AllTest()
					for _, test := range tests {
						if testCtx, ok := test.(*pythonparser.TestContext); ok {
							varName := b.extractVariableName(testCtx)
							if varName != "" {
								variable := b.createVar(varName)
								b.AssignVariable(variable, lastValue)
							}
						}
					}
				}
			}
		}

		// Then assign to left variables
		for _, varName := range leftVars {
			variable := b.createVar(varName)
			b.AssignVariable(variable, lastValue)
		}
	} else if len(leftVars) > 0 && len(rightValues) > 0 {
		// Multiple assignment: a, b = 1, 2
		// Or single assignment: x = 1
		if len(leftVars) == len(rightValues) {
			// Multiple assignment with matching counts
			for i, varName := range leftVars {
				variable := b.createVar(varName)
				b.AssignVariable(variable, rightValues[i])
			}
		} else if len(leftVars) == 1 && len(rightValues) == 1 {
			// Single assignment
			variable := b.createVar(leftVars[0])
			b.AssignVariable(variable, rightValues[0])
		} else {
			// Mismatch - assign the first right value to all left variables
			if len(rightValues) > 0 {
				for _, varName := range leftVars {
					variable := b.createVar(varName)
					b.AssignVariable(variable, rightValues[0])
				}
			}
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
