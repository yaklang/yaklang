package python2ssa

import (
	"strings"

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
	if right.COLON() != nil {
		return b.VisitAnnassign(left, right)
	}

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
		if testCtx == nil {
			return
		}
		logicalTests := testCtx.AllLogical_test()
		if len(logicalTests) == 0 {
			return
		}
		ltCtx, ok := logicalTests[0].(*pythonparser.Logical_testContext)
		if !ok || ltCtx == nil || ltCtx.Comparison() == nil {
			return
		}
		compCtx, ok := ltCtx.Comparison().(*pythonparser.ComparisonContext)
		if !ok || compCtx == nil || compCtx.Expr() == nil {
			return
		}
		exprCtx, ok := compCtx.Expr().(*pythonparser.ExprContext)
		if !ok || exprCtx == nil {
			return
		}
		target := b.extractAssignTargetFromExpr(exprCtx)
		if target.String() != "" {
			targets = append(targets, target)
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

func (b *singleFileBuilder) emitCallablePlaceholder(name string) ssa.Value {
	if name == "" {
		return nil
	}
	if value := b.PeekValueInThisFunction(name); value != nil {
		return value
	}
	return b.EmitValueOnlyDeclare(name)
}

func (b *singleFileBuilder) collectExprlistTargets(raw pythonparser.IExprlistContext) []assignTarget {
	exprlistCtx, ok := raw.(*pythonparser.ExprlistContext)
	if !ok || exprlistCtx == nil {
		return nil
	}

	targets := make([]assignTarget, 0, len(exprlistCtx.AllExpr()))
	for _, expr := range exprlistCtx.AllExpr() {
		exprCtx, ok := expr.(*pythonparser.ExprContext)
		if !ok || exprCtx == nil {
			continue
		}
		target := b.extractAssignTargetFromExpr(exprCtx)
		if target.String() != "" {
			targets = append(targets, target)
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
	if b == nil || left == nil || assignPart == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(assignPart)
	defer recoverRange()

	targets := b.extractLeftTargets(left)
	if len(targets) == 0 {
		return nil
	}

	rightValues := make([]ssa.Value, 0, 1)
	if testlist := assignPart.Testlist(); testlist != nil {
		if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok {
			for _, test := range testlistCtx.AllTest() {
				if testCtx, ok := test.(*pythonparser.TestContext); ok {
					if value, ok := b.VisitTest(testCtx).(ssa.Value); ok {
						rightValues = append(rightValues, value)
					}
				}
			}
		}
	}

	var assignedValue ssa.Value
	if len(rightValues) > 0 {
		assignedValue = rightValues[0]
	} else {
		assignedValue = b.EmitUndefined(targets[0].String())
	}

	for _, target := range targets {
		b.assignToTarget(target, assignedValue)
	}
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

	if b.Break() {
		return nil
	}
	if control := b.currentStaticLoopControl(); control != nil {
		control.state = staticLoopControlBreak
	}
	return nil
}

// VisitContinueStmt visits a continue_stmt node.
func (b *singleFileBuilder) VisitContinueStmt(raw *pythonparser.Continue_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	if b.Continue() {
		return nil
	}
	if control := b.currentStaticLoopControl(); control != nil {
		control.state = staticLoopControlContinue
	}
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

// VisitTypeStmt lowers `type Alias = ...` into a lightweight alias type and
// binds it as both an exported type and a type value in the current scope.
func (b *singleFileBuilder) VisitTypeStmt(raw *pythonparser.Type_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	nameCtx := raw.Name()
	if nameCtx == nil {
		return nil
	}
	aliasName := nameCtx.GetText()
	aliasType := ssa.NewAliasType(aliasName, "", b.inferTypeFromTest(raw.Test()))
	b.GetProgram().SetExportType(aliasName, aliasType)

	typeValue := b.EmitTypeValue(aliasType)
	if typeValue == nil {
		return nil
	}

	variable := b.createVar(aliasName)
	b.AssignVariable(variable, typeValue)
	b.GetProgram().SetExportValue(aliasName, typeValue)
	return typeValue
}

func firstImportBinding(path string) string {
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "."); idx >= 0 {
		return path[:idx]
	}
	return path
}

func extractSimpleQualifiedNameFromTest(raw *pythonparser.TestContext) string {
	if raw == nil {
		return ""
	}
	text := strings.TrimSpace(raw.GetText())
	if text == "" {
		return ""
	}
	for _, ch := range text {
		if ch == '_' || ch == '.' ||
			(ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') {
			continue
		}
		return ""
	}
	return text
}

func extractSimpleQualifiedNamesFromTest(raw *pythonparser.TestContext) []string {
	if raw == nil {
		return nil
	}
	if name := extractSimpleQualifiedNameFromTest(raw); name != "" {
		return []string{name}
	}

	text := strings.TrimSpace(raw.GetText())
	if len(text) < 2 || text[0] != '(' || text[len(text)-1] != ')' {
		return nil
	}

	inner := strings.TrimSpace(text[1 : len(text)-1])
	if inner == "" {
		return nil
	}

	parts := strings.Split(inner, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		for _, ch := range part {
			if ch == '_' || ch == '.' ||
				(ch >= 'a' && ch <= 'z') ||
				(ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') {
				continue
			}
			return nil
		}
		names = append(names, part)
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func inferRaisedTypeName(raw *pythonparser.TestContext) string {
	if raw == nil {
		return ""
	}
	text := strings.TrimSpace(raw.GetText())
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, "("); idx > 0 && strings.HasSuffix(text, ")") {
		callee := strings.TrimSpace(text[:idx])
		for _, ch := range callee {
			if ch == '_' || ch == '.' ||
				(ch >= 'a' && ch <= 'z') ||
				(ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') {
				continue
			}
			return ""
		}
		return callee
	}
	return extractSimpleQualifiedNameFromTest(raw)
}

func joinImportPath(base, name string) string {
	switch {
	case base == "":
		return name
	case name == "":
		return base
	case strings.HasSuffix(base, "."):
		return base + name
	default:
		return base + "." + name
	}
}

func (b *singleFileBuilder) bindImportedName(bindingName, sourceName, packagePath string) ssa.Value {
	if bindingName == "" {
		return nil
	}
	if sourceName == "" {
		sourceName = bindingName
	}
	if packagePath == "" {
		packagePath = sourceName
	}

	prog := b.GetProgram()
	if prog == nil {
		return nil
	}

	if value, ok := prog.ReadImportValue(bindingName); ok && value != nil {
		b.AssignVariable(b.createVar(bindingName), value)
		return value
	}

	if prog.GetCurrentEditor() == nil {
		return b.bindImportedPlaceholder(bindingName, sourceName)
	}

	lib, err := prog.GetOrCreateLibrary(packagePath)
	if err != nil || lib == nil {
		return b.bindImportedPlaceholder(bindingName, sourceName)
	}

	value := lib.GetExportValue(bindingName)
	if value == nil {
		libBuilder := lib.GetAndCreateFunctionBuilder(lib.PkgName, string(ssa.VirtualFunctionName))
		if libBuilder == nil {
			return b.bindImportedPlaceholder(bindingName, sourceName)
		}
		value = b.newDynamicPlaceholder(sourceName)
		if value == nil {
			return b.bindImportedPlaceholder(bindingName, sourceName)
		}
		lib.SetExportValue(bindingName, value)
	}

	if err := prog.ImportValueFromLib(lib, bindingName); err != nil {
		return b.bindImportedPlaceholder(bindingName, sourceName)
	}
	if imported, ok := prog.ReadImportValue(bindingName); ok && imported != nil {
		return imported
	}
	return value
}

// VisitImportStmt visits an import_stmt node.
func (b *singleFileBuilder) VisitImportStmt(raw *pythonparser.Import_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	names, ok := raw.Dotted_as_names().(*pythonparser.Dotted_as_namesContext)
	if !ok || names == nil {
		return nil
	}

	for _, dottedName := range names.AllDotted_as_name() {
		entry, ok := dottedName.(*pythonparser.Dotted_as_nameContext)
		if !ok || entry == nil || entry.Dotted_name() == nil {
			continue
		}

		sourceName := entry.Dotted_name().GetText()
		bindingName := firstImportBinding(sourceName)
		if alias := entry.Name(); alias != nil {
			bindingName = alias.GetText()
		} else {
			// `import pkg.sub` binds `pkg`; keep the bound value rooted at the package
			// name until we add richer module-object lowering.
			sourceName = bindingName
		}

		b.bindImportedName(bindingName, sourceName, entry.Dotted_name().GetText())
	}
	return nil
}

// VisitFromStmt visits a from_stmt node.
func (b *singleFileBuilder) VisitFromStmt(raw *pythonparser.From_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	modulePath := strings.Repeat(".", len(raw.AllDOT())) + strings.Repeat("...", len(raw.AllELLIPSIS()))
	if dotted := raw.Dotted_name(); dotted != nil {
		modulePath = joinImportPath(modulePath, dotted.GetText())
	}

	if raw.STAR() != nil {
		b.addWildcardImportPackage(modulePath)
		if prog := b.GetProgram(); prog != nil {
			if lib, err := prog.GetOrCreateLibrary(modulePath); err == nil && lib != nil {
				_ = prog.ImportAll(lib)
			}
		}
		return nil
	}

	importNames, ok := raw.Import_as_names().(*pythonparser.Import_as_namesContext)
	if !ok || importNames == nil {
		return nil
	}

	for _, imported := range importNames.AllImport_as_name() {
		entry, ok := imported.(*pythonparser.Import_as_nameContext)
		if !ok || entry == nil {
			continue
		}

		names := entry.AllName()
		if len(names) == 0 {
			continue
		}

		importedName := names[0].GetText()
		bindingName := importedName
		if len(names) > 1 && names[1] != nil {
			bindingName = names[1].GetText()
		}

		b.bindImportedName(bindingName, joinImportPath(modulePath, importedName), modulePath)
	}
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

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	tests := raw.AllTest()
	if len(tests) == 0 {
		return nil
	}

	cond, ok := b.VisitTest(tests[0].(*pythonparser.TestContext)).(ssa.Value)
	if !ok || cond == nil {
		return nil
	}

	var msgValue ssa.Value
	if len(tests) > 1 {
		if value, ok := b.VisitTest(tests[1].(*pythonparser.TestContext)).(ssa.Value); ok {
			msgValue = value
		}
	}

	b.EmitAssert(cond, msgValue, raw.GetText())
	return nil
}

// VisitRaiseStmt visits a raise_stmt node.
func (b *singleFileBuilder) VisitRaiseStmt(raw *pythonparser.Raise_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	var raised ssa.Value
	for i, test := range raw.AllTest() {
		testCtx, ok := test.(*pythonparser.TestContext)
		if !ok || testCtx == nil {
			continue
		}
		value, ok := b.VisitTest(testCtx).(ssa.Value)
		if !ok || value == nil {
			continue
		}
		if i == 0 && raised == nil {
			raised = value
		}
	}

	if raised == nil {
		raised = b.EmitUndefined("python.raise")
	}
	if control := b.currentTryControl(); control != nil {
		control.raised = true
		control.lastRaised = raised
		if len(raw.AllTest()) > 0 {
			if testCtx, ok := raw.AllTest()[0].(*pythonparser.TestContext); ok {
				control.lastRaisedType = inferRaisedTypeName(testCtx)
			}
		}
	}
	b.EmitPanic(raised)
	return raised
}

// VisitDelStmt visits a del_stmt node.
func (b *singleFileBuilder) VisitDelStmt(raw *pythonparser.Del_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	for _, target := range b.collectExprlistTargets(raw.Exprlist()) {
		undefined := b.EmitUndefined(target.String())
		if undefined == nil {
			continue
		}
		undefined.Kind = ssa.UndefinedValueValid
		b.assignToTarget(target, undefined)
	}
	return nil
}

// VisitPrintStmt visits a print_stmt node.
func (b *singleFileBuilder) VisitPrintStmt(raw *pythonparser.Print_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	printlnValue := b.emitCallablePlaceholder("println")
	if printlnValue == nil {
		return nil
	}

	tests := raw.AllTest()
	start := 0
	if raw.RIGHT_SHIFT() != nil && len(tests) > 0 {
		// `print >>dest, value` uses the first test as output redirection target.
		start = 1
	}

	for _, test := range tests[start:] {
		testCtx, ok := test.(*pythonparser.TestContext)
		if !ok || testCtx == nil {
			continue
		}
		value, ok := b.VisitTest(testCtx).(ssa.Value)
		if !ok || value == nil {
			continue
		}
		call := b.NewCall(printlnValue, []ssa.Value{value})
		b.EmitCall(call)
	}
	return nil
}

// VisitExecStmt visits an exec_stmt node.
func (b *singleFileBuilder) VisitExecStmt(raw *pythonparser.Exec_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	args := make([]ssa.Value, 0, 3)
	if expr, ok := raw.Expr().(*pythonparser.ExprContext); ok && expr != nil {
		if value, ok := b.VisitExpr(expr).(ssa.Value); ok && value != nil {
			args = append(args, value)
		}
	}
	for _, test := range raw.AllTest() {
		testCtx, ok := test.(*pythonparser.TestContext)
		if !ok || testCtx == nil {
			continue
		}
		if value, ok := b.VisitTest(testCtx).(ssa.Value); ok && value != nil {
			args = append(args, value)
		}
	}

	execValue := b.emitCallablePlaceholder("exec")
	if execValue == nil {
		return nil
	}
	call := b.NewCall(execValue, args)
	return b.EmitCall(call)
}

// VisitYieldStmt visits a yield_stmt node.
func (b *singleFileBuilder) VisitYieldStmt(raw *pythonparser.Yield_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	args := make([]ssa.Value, 0, 2)
	if yieldExpr, ok := raw.Yield_expr().(*pythonparser.Yield_exprContext); ok && yieldExpr != nil {
		if yieldArg, ok := yieldExpr.Yield_arg().(*pythonparser.Yield_argContext); ok && yieldArg != nil {
			if test := yieldArg.Test(); test != nil {
				if testCtx, ok := test.(*pythonparser.TestContext); ok {
					if value, ok := b.VisitTest(testCtx).(ssa.Value); ok && value != nil {
						args = append(args, value)
					}
				}
			} else if testlist := yieldArg.Testlist(); testlist != nil {
				if testlistCtx, ok := testlist.(*pythonparser.TestlistContext); ok && testlistCtx != nil {
					for _, test := range testlistCtx.AllTest() {
						testCtx, ok := test.(*pythonparser.TestContext)
						if !ok || testCtx == nil {
							continue
						}
						if value, ok := b.VisitTest(testCtx).(ssa.Value); ok && value != nil {
							args = append(args, value)
						}
					}
				}
			}
		}
	}

	yieldValue := b.emitCallablePlaceholder("yield")
	if yieldValue == nil {
		return nil
	}
	call := b.NewCall(yieldValue, args)
	return b.EmitCall(call)
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

	b.ensureBlueprintMember(obj, attrNameStr)
	obj = b.ensureDynamicObjectType(obj)

	return b.CreateMemberCallVariable(obj, key)
}

func (b *singleFileBuilder) extractVariableNameFromLogicalTest(raw pythonparser.ILogical_testContext) string {
	if raw == nil {
		return ""
	}
	logicalTest, ok := raw.(*pythonparser.Logical_testContext)
	if !ok || logicalTest == nil {
		return ""
	}
	comparison := logicalTest.Comparison()
	if comparison == nil {
		return ""
	}
	compCtx, ok := comparison.(*pythonparser.ComparisonContext)
	if !ok || compCtx == nil {
		return ""
	}
	expr := compCtx.Expr()
	if expr == nil {
		return ""
	}
	return b.extractVariableNameFromExpr(expr)
}

func (b *singleFileBuilder) extractVariableNameFromExpr(raw pythonparser.IExprContext) string {
	exprCtx, ok := raw.(*pythonparser.ExprContext)
	if !ok || exprCtx == nil {
		return ""
	}
	if len(exprCtx.AllExpr()) > 0 || len(exprCtx.AllTrailer()) > 0 {
		return ""
	}
	atom := exprCtx.Atom()
	if atom == nil {
		return ""
	}
	atomCtx, ok := atom.(*pythonparser.AtomContext)
	if !ok || atomCtx == nil {
		return ""
	}
	name := atomCtx.Name()
	if name == nil {
		return ""
	}
	return name.GetText()
}

func (b *singleFileBuilder) extractAssignTargetFromExpr(raw pythonparser.IExprContext) assignTarget {
	exprCtx, ok := raw.(*pythonparser.ExprContext)
	if !ok || exprCtx == nil {
		return assignTarget{}
	}

	if varName := b.extractVariableNameFromExpr(exprCtx); varName != "" {
		return assignTarget{varName: varName}
	}

	atom := exprCtx.Atom()
	if atom == nil {
		return assignTarget{}
	}
	atomCtx, ok := atom.(*pythonparser.AtomContext)
	if !ok || atomCtx == nil {
		return assignTarget{}
	}

	name := atomCtx.Name()
	if name == nil {
		return assignTarget{}
	}
	objName := name.GetText()
	obj := b.ReadValue(objName)
	if obj == nil {
		return assignTarget{}
	}

	trailers := exprCtx.AllTrailer()
	if len(trailers) == 0 {
		return assignTarget{}
	}

	currentObj := obj
	for _, trailer := range trailers[:len(trailers)-1] {
		trailerCtx, ok := trailer.(*pythonparser.TrailerContext)
		if !ok || trailerCtx == nil {
			return assignTarget{}
		}
		nextObj := b.VisitTrailer(trailerCtx, currentObj)
		if nextObj == nil {
			return assignTarget{}
		}
		currentObj = nextObj
	}
	obj = currentObj

	lastTrailer, ok := trailers[len(trailers)-1].(*pythonparser.TrailerContext)
	if !ok || lastTrailer == nil {
		return assignTarget{}
	}

	if lastTrailer.DOT() != nil && lastTrailer.Name() != nil {
		attrName := lastTrailer.Name().GetText()
		syntheticName := attrName
		if objName := obj.GetName(); objName != "" {
			syntheticName = objName + "." + attrName
		}
		if obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.FunctionTypeKind {
			return assignTarget{varName: syntheticName}
		}
		if obj.GetType() != nil {
			switch obj.GetType().GetTypeKind() {
			case ssa.SliceTypeKind, ssa.TupleTypeKind:
				return assignTarget{varName: syntheticName}
			}
		}
		if obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.ClassBluePrintTypeKind {
			if blueprint, ok := ssa.ToClassBluePrintType(obj.GetType()); ok && !b.hasBlueprintMemberOrMethod(blueprint, attrName) {
				return assignTarget{varName: syntheticName}
			}
		}
		if b.shouldUseDynamicMemberFallback(obj) {
			return assignTarget{varName: syntheticName}
		}
		b.ensureBlueprintMember(obj, attrName)
		obj = b.ensureDynamicObjectType(obj)
		key := b.EmitConstInst(attrName)
		return assignTarget{memberVar: b.CreateMemberCallVariable(obj, key)}
	}

	if arguments := lastTrailer.Arguments(); arguments != nil {
		if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok && argCtx.OPEN_BRACKET() != nil {
			if subscriptlist := argCtx.Subscriptlist(); subscriptlist != nil {
				if subscriptListCtx, ok := subscriptlist.(*pythonparser.SubscriptlistContext); ok {
					subs := subscriptListCtx.AllSubscript()
					if len(subs) > 0 {
						if subCtx, ok := subs[0].(*pythonparser.SubscriptContext); ok {
							if test := subCtx.Test(0); test != nil {
								if idxValue, ok := b.VisitTest(test.(*pythonparser.TestContext)).(ssa.Value); ok && idxValue != nil {
									if obj.GetType() != nil {
										switch obj.GetType().GetTypeKind() {
										case ssa.SliceTypeKind, ssa.TupleTypeKind:
											if idxValue.GetType() != nil && idxValue.GetType().GetTypeKind() == ssa.StringTypeKind {
												name := obj.GetName()
												if name == "" {
													name = "item"
												}
												return assignTarget{varName: name + "[" + idxValue.String() + "]"}
											}
										}
									}
									obj = b.ensureDynamicObjectType(obj)
									return assignTarget{memberVar: b.CreateMemberCallVariable(obj, idxValue)}
								}
							}
						}
					}
				}
			}
		}
	}

	return assignTarget{}
}

func (a assignTarget) String() string {
	if a.varName != "" {
		return a.varName
	}
	if a.memberVar != nil {
		return a.memberVar.GetName()
	}
	return ""
}

func (b *singleFileBuilder) inferTypeFromTest(raw pythonparser.ITestContext) ssa.Type {
	if raw == nil {
		return ssa.CreateAnyType()
	}

	text := strings.TrimSpace(raw.GetText())
	switch text {
	case "int", "float":
		return ssa.CreateNumberType()
	case "str":
		return ssa.CreateStringType()
	case "bool":
		return ssa.CreateBooleanType()
	case "bytes", "bytearray":
		return ssa.CreateBytesType()
	case "None":
		return ssa.CreateNullType()
	case "Any":
		return ssa.CreateAnyType()
	}

	if strings.Contains(text, "|") {
		parts := strings.Split(text, "|")
		types := make([]ssa.Type, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			types = append(types, b.inferTypeFromTestString(part))
		}
		if len(types) == 1 {
			return types[0]
		}
		if len(types) > 1 {
			return ssa.NewOrType(types...)
		}
	}

	return b.inferTypeFromTestString(text)
}

func (b *singleFileBuilder) inferTypeFromTestString(text string) ssa.Type {
	switch {
	case text == "":
		return ssa.CreateAnyType()
	case strings.HasPrefix(text, "list[") || strings.HasPrefix(text, "tuple[") || strings.HasPrefix(text, "set["):
		return ssa.NewSliceType(ssa.CreateAnyType())
	case strings.HasPrefix(text, "dict[") || strings.HasPrefix(text, "_dict["):
		return ssa.NewMapType(ssa.CreateAnyType(), ssa.CreateAnyType())
	case len(text) == 1 && strings.ToUpper(text) == text:
		return ssa.NewGenericType(text)
	default:
		return ssa.NewAliasType(text, "", ssa.CreateAnyType())
	}
}
