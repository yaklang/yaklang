package python2ssa

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

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

type loopAssignTarget struct {
	assignTarget
	starred bool
}

func (b *singleFileBuilder) extractLoopTargets(raw pythonparser.IExprlistContext) []loopAssignTarget {
	exprlistCtx, ok := raw.(*pythonparser.ExprlistContext)
	if !ok || exprlistCtx == nil {
		return nil
	}

	targets := make([]loopAssignTarget, 0, len(exprlistCtx.GetChildren()))
	for _, child := range exprlistCtx.GetChildren() {
		switch node := child.(type) {
		case *pythonparser.ExprContext:
			target := b.extractAssignTargetFromExpr(node)
			if target.String() != "" {
				targets = append(targets, loopAssignTarget{assignTarget: target})
			}
		case *pythonparser.Star_exprContext:
			exprCtx, ok := node.Expr().(*pythonparser.ExprContext)
			if !ok || exprCtx == nil {
				continue
			}
			target := b.extractAssignTargetFromExpr(exprCtx)
			if target.String() != "" {
				targets = append(targets, loopAssignTarget{assignTarget: target, starred: true})
			}
		}
	}
	return targets
}

func (b *singleFileBuilder) undefinedLoopValue(name string) ssa.Value {
	value := b.EmitUndefined(name)
	if value != nil {
		value.Kind = ssa.UndefinedValueValid
	}
	return value
}

func (b *singleFileBuilder) extractStaticSequenceValues(value ssa.Value) ([]ssa.Value, bool) {
	inst, ok := value.(ssa.Instruction)
	if !ok {
		return nil, false
	}
	makeValue, ok := ssa.ToMake(inst)
	if !ok || makeValue == nil || makeValue.Len == 0 {
		return nil, false
	}
	lenValue, ok := makeValue.GetValueById(makeValue.Len)
	if !ok || lenValue == nil {
		return nil, false
	}
	lengthConst, ok := lenValue.(*ssa.ConstInst)
	if !ok || !lengthConst.IsNumber() {
		return nil, false
	}

	length := int(lengthConst.Number())
	if length < 0 || length > 256 {
		return nil, false
	}

	values := make([]ssa.Value, 0, length)
	for idx := 0; idx < length; idx++ {
		member := b.ReadMemberCallValue(value, b.EmitConstInst(int64(idx)))
		if member == nil {
			return nil, false
		}
		values = append(values, member)
	}
	return values, true
}

func staticValueSortKey(value ssa.Value) string {
	if value == nil {
		return ""
	}
	if constant, ok := value.(*ssa.ConstInst); ok {
		switch {
		case constant.IsString():
			return "s:" + constant.VarString()
		case constant.IsNumber():
			return fmt.Sprintf("n:%020f", constant.Number())
		case constant.IsBoolean():
			if constant.Boolean() {
				return "b:1"
			}
			return "b:0"
		}
	}
	return value.String()
}

func (b *singleFileBuilder) extractStaticMapKeys(value ssa.Value) ([]ssa.Value, bool) {
	if value == nil || value.GetType() == nil || value.GetType().GetTypeKind() != ssa.MapTypeKind {
		return nil, false
	}
	members := value.GetAllMember()
	if len(members) == 0 {
		return nil, false
	}

	keys := make([]ssa.Value, 0, len(members))
	for key := range members {
		if key != nil {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil, false
	}

	sort.Slice(keys, func(i, j int) bool {
		return staticValueSortKey(keys[i]) < staticValueSortKey(keys[j])
	})
	return keys, true
}

func (b *singleFileBuilder) buildSliceValueFromValues(values []ssa.Value) ssa.Value {
	sliceType := ssa.NewSliceType(ssa.CreateAnyType())
	lst := b.EmitMakeBuildWithType(sliceType, b.EmitConstInst(int64(len(values))), b.EmitConstInst(int64(len(values))))
	for idx, val := range values {
		member := b.CreateMemberCallVariable(lst, b.EmitConstInst(int64(idx)))
		b.AssignVariable(member, val)
	}
	return lst
}

func (b *singleFileBuilder) assignLoopTargets(targets []loopAssignTarget, itemValue ssa.Value) {
	if len(targets) == 0 || itemValue == nil {
		return
	}

	if len(targets) == 1 && !targets[0].starred {
		b.assignToTarget(targets[0].assignTarget, itemValue)
		return
	}

	starIndex := -1
	for index, target := range targets {
		if target.starred {
			starIndex = index
			break
		}
	}

	if staticValues, ok := b.extractStaticSequenceValues(itemValue); ok {
		if starIndex < 0 {
			for index, target := range targets {
				if index < len(staticValues) {
					b.assignToTarget(target.assignTarget, staticValues[index])
				} else {
					b.assignToTarget(target.assignTarget, b.undefinedLoopValue(target.String()))
				}
			}
			return
		}

		prefix := starIndex
		suffix := len(targets) - starIndex - 1
		for index := 0; index < prefix; index++ {
			target := targets[index]
			if index < len(staticValues) {
				b.assignToTarget(target.assignTarget, staticValues[index])
			} else {
				b.assignToTarget(target.assignTarget, b.undefinedLoopValue(target.String()))
			}
		}

		restStart := prefix
		restEnd := len(staticValues) - suffix
		if restEnd < restStart {
			restEnd = restStart
		}
		b.assignToTarget(targets[starIndex].assignTarget, b.buildSliceValueFromValues(staticValues[restStart:restEnd]))

		for offset := 0; offset < suffix; offset++ {
			target := targets[starIndex+1+offset]
			sourceIndex := len(staticValues) - suffix + offset
			if sourceIndex >= 0 && sourceIndex < len(staticValues) {
				b.assignToTarget(target.assignTarget, staticValues[sourceIndex])
			} else {
				b.assignToTarget(target.assignTarget, b.undefinedLoopValue(target.String()))
			}
		}
		return
	}

	if starIndex < 0 {
		for index, target := range targets {
			value := b.ReadMemberCallValue(itemValue, b.EmitConstInst(int64(index)))
			if value == nil {
				value = b.undefinedLoopValue(target.String())
			}
			b.assignToTarget(target.assignTarget, value)
		}
		return
	}

	for index := 0; index < starIndex; index++ {
		target := targets[index]
		value := b.ReadMemberCallValue(itemValue, b.EmitConstInst(int64(index)))
		if value == nil {
			value = b.undefinedLoopValue(target.String())
		}
		b.assignToTarget(target.assignTarget, value)
	}

	if starIndex == len(targets)-1 {
		var restValue ssa.Value = b.EmitMakeSlice(itemValue, b.EmitConstInst(int64(starIndex)), nil, nil)
		if restValue == nil {
			restValue = b.undefinedLoopValue(targets[starIndex].String())
		}
		b.assignToTarget(targets[starIndex].assignTarget, restValue)
		return
	}

	// Generic iterator fallback cannot yet recover exact suffix positions from an
	// unknown iterable item shape. Keep compilation moving while leaving a clear
	// TODO for richer symbolic destructuring.
	b.assignToTarget(targets[starIndex].assignTarget, itemValue)
	for index := starIndex + 1; index < len(targets); index++ {
		b.assignToTarget(targets[index].assignTarget, b.undefinedLoopValue(targets[index].String()))
	}
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
				control := b.pushStaticLoopControl()
				defer b.popStaticLoopControl()
				iter := 0
				for {
					control.state = staticLoopControlNone
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
					switch control.state {
					case staticLoopControlBreak:
						return nil
					case staticLoopControlContinue:
						startVal += step
						iter++
						continue
					}
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

	targets := b.extractLoopTargets(exprlist)
	if len(targets) == 0 {
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
		control := b.pushStaticLoopControl()
		defer b.popStaticLoopControl()
		maxIter := 256
		iter := 0
		for val := start; (step > 0 && val < end) || (step < 0 && val > end); val += step {
			control.state = staticLoopControlNone
			if iter >= maxIter {
				break
			}
			b.assignLoopTargets(targets, b.EmitConstInst(val))
			b.VisitSuite(suite)
			switch control.state {
			case staticLoopControlBreak:
				return nil
			case staticLoopControlContinue:
				iter++
				continue
			}
			iter++
		}
		return nil
	}

	if staticItems, ok := b.extractStaticSequenceValues(iterableValue); ok {
		control := b.pushStaticLoopControl()
		defer b.popStaticLoopControl()
		maxIter := 256
		for index, itemValue := range staticItems {
			control.state = staticLoopControlNone
			if index >= maxIter {
				break
			}
			b.assignLoopTargets(targets, itemValue)
			b.VisitSuite(suite)
			switch control.state {
			case staticLoopControlBreak:
				return nil
			case staticLoopControlContinue:
				continue
			}
		}
		return nil
	}

	if staticKeys, ok := b.extractStaticMapKeys(iterableValue); ok {
		control := b.pushStaticLoopControl()
		defer b.popStaticLoopControl()
		maxIter := 256
		for index, itemValue := range staticKeys {
			control.state = staticLoopControlNone
			if index >= maxIter {
				break
			}
			b.assignLoopTargets(targets, itemValue)
			b.VisitSuite(suite)
			switch control.state {
			case staticLoopControlBreak:
				return nil
			case staticLoopControlContinue:
				continue
			}
		}
		return nil
	}

	if iterableValue == nil {
		return nil
	}

	// Build loop
	loopBuilder := b.CreateLoopBuilder()

	loopBuilder.SetFirst(func() []ssa.Value {
		return []ssa.Value{iterableValue}
	})

	loopBuilder.SetCondition(func() ssa.Value {
		itemValue, _, ok := b.EmitNext(iterableValue, true)
		if itemValue != nil {
			b.assignLoopTargets(targets, itemValue)
		}
		if ok == nil {
			ok = b.EmitConstInst(true)
		}
		return ok
	})

	// Set body: visit suite
	loopBuilder.SetBody(func() {
		b.VisitSuite(suite)
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

	trySuite := raw.Suite()
	if trySuite == nil {
		return nil
	}

	var elseSuite pythonparser.ISuiteContext
	if elseClause := raw.Else_clause(); elseClause != nil {
		if elseCtx, ok := elseClause.(*pythonparser.Else_clauseContext); ok && elseCtx != nil {
			elseSuite = elseCtx.Suite()
		}
	}

	control := b.pushTryControl()
	defer b.popTryControl()
	staticRaisedType := inferStaticRaisedTypeFromSuite(trySuite)

	tryBuilder := b.BuildTry()
	tryBuilder.BuildTryBlock(func() {
		control.raised = false
		control.lastRaised = nil
		control.lastRaisedType = ""
		b.VisitSuite(trySuite)
		if elseSuite != nil && !control.raised && !b.IsBlockFinish() {
			b.VisitSuite(elseSuite)
		}
	})

	staticCatchSelected := false
	staticSelector := &tryControl{lastRaisedType: staticRaisedType}
	for index, exceptClause := range raw.AllExcept_clause() {
		exceptCtx, ok := exceptClause.(*pythonparser.Except_clauseContext)
		if !ok || exceptCtx == nil {
			continue
		}
		if !shouldBuildStaticCatch(staticSelector, exceptCtx, &staticCatchSelected) {
			continue
		}

		exceptionName := fmt.Sprintf("python_exception_%d", index)
		if name := exceptCtx.Name(); name != nil {
			exceptionName = name.GetText()
		}

		tryBuilder.BuildErrorCatch(func() string {
			return exceptionName
		}, func() {
			if exceptCtx.Name() != nil && control.lastRaised != nil {
				catchVar := b.CreateLocalVariable(exceptionName)
				b.AssignVariable(catchVar, control.lastRaised)
			}
			if suite := exceptCtx.Suite(); suite != nil {
				b.VisitSuite(suite)
			}
		})
	}

	if finallyClause := raw.Finally_clause(); finallyClause != nil {
		if finallyCtx, ok := finallyClause.(*pythonparser.Finally_clauseContext); ok && finallyCtx != nil {
			tryBuilder.BuildFinally(func() {
				if suite := finallyCtx.Suite(); suite != nil {
					b.VisitSuite(suite)
				}
			})
		}
	}

	tryBuilder.Finish()

	return nil
}

func (b *singleFileBuilder) isBareCapturePattern(testCtx *pythonparser.TestContext) bool {
	if testCtx == nil {
		return false
	}
	name := b.extractVariableName(testCtx)
	if name == "" || testCtx.GetText() != name {
		return false
	}
	switch name {
	case "_", "True", "False", "None":
		return false
	default:
		return true
	}
}

func isSimpleIdentifierText(text string) bool {
	if text == "" {
		return false
	}
	for i, ch := range text {
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			continue
		}
		if i > 0 && ch >= '0' && ch <= '9' {
			continue
		}
		return false
	}
	return true
}

func isIdentifierString(text string) bool {
	if text == "" {
		return false
	}
	for index, ch := range text {
		if ch == '_' || unicode.IsLetter(ch) {
			continue
		}
		if index > 0 && unicode.IsDigit(ch) {
			continue
		}
		return false
	}
	return true
}

func splitTopLevelMatchAlternatives(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" || !strings.Contains(text, "|") {
		return nil
	}

	parts := make([]string, 0, 2)
	start := 0
	depth := 0
	var quote rune

	for index, ch := range text {
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == 0 {
				part := strings.TrimSpace(text[start:index])
				if part != "" {
					parts = append(parts, part)
				}
				start = index + 1
			}
		}
	}

	if len(parts) == 0 {
		return nil
	}

	last := strings.TrimSpace(text[start:])
	if last != "" {
		parts = append(parts, last)
	}
	return parts
}

func (b *singleFileBuilder) buildMatchPatternValueFromText(text string) (ssa.Value, bool) {
	text = strings.TrimSpace(text)
	switch text {
	case "":
		return nil, false
	case "_":
		return nil, false
	case "True":
		return b.EmitConstInst(true), true
	case "False":
		return b.EmitConstInst(false), true
	case "None":
		return b.EmitConstInst(nil), true
	}

	if isSimpleIdentifierText(text) {
		return nil, false
	}

	if unquoted, err := strconv.Unquote(text); err == nil {
		return b.EmitConstInst(unquoted), true
	}

	cleanNumeric := strings.ReplaceAll(text, "_", "")
	if integer, err := strconv.ParseInt(cleanNumeric, 10, 64); err == nil {
		return b.EmitConstInst(integer), true
	}
	if floatValue, err := strconv.ParseFloat(cleanNumeric, 64); err == nil {
		return b.EmitConstInst(floatValue), true
	}

	if value, ok := b.GetProgram().ReadImportValue(text); ok {
		return value, true
	}
	if value := b.resolveWildcardImportName(text); value != nil {
		return value, true
	}
	if value := b.PeekValueInThisFunction(text); value != nil {
		return value, true
	}

	return nil, false
}

func staticValuesEqual(left, right ssa.Value) bool {
	if left == nil || right == nil {
		return false
	}
	if leftInst, lok := left.(ssa.Instruction); lok {
		if rightInst, rok := right.(ssa.Instruction); rok {
			if leftConst, lok := leftInst.(*ssa.ConstInst); lok {
				if rightConst, rok := rightInst.(*ssa.ConstInst); rok {
					return leftConst.String() == rightConst.String()
				}
			}
		}
	}
	return left.String() == right.String()
}

func evaluateStaticMatchGuard(guard *pythonparser.TestContext, captures map[string]ssa.Value) (bool, bool) {
	if guard == nil {
		return true, true
	}
	text := strings.TrimSpace(guard.GetText())
	if text == "" {
		return true, true
	}

	if strings.HasSuffix(text, ".isidentifier()") {
		name := strings.TrimSuffix(text, ".isidentifier()")
		if captured, ok := captures[name]; ok {
			if inst, ok := captured.(ssa.Instruction); ok {
				if constInst, ok := inst.(*ssa.ConstInst); ok && constInst.IsString() {
					return isIdentifierString(constInst.VarString()), true
				}
			}
		}
	}

	if captured, ok := captures[text]; ok {
		if inst, ok := captured.(ssa.Instruction); ok {
			if constInst, ok := inst.(*ssa.ConstInst); ok {
				switch {
				case constInst.IsBoolean():
					return constInst.Boolean(), true
				case constInst.IsNumber():
					return constInst.Number() != 0, true
				case constInst.IsString():
					return constInst.VarString() != "", true
				}
			}
		}
	}

	return false, false
}

func (b *singleFileBuilder) matchStaticSequenceCase(subjectValues []ssa.Value, raw *pythonparser.Case_clauseContext) (bool, map[string]ssa.Value, bool) {
	if raw == nil {
		return false, nil, false
	}
	patterns, guardTest, ok := extractCasePatternItems(raw)
	if !ok || len(patterns) == 0 || len(raw.AllCOMMA()) == 0 {
		return false, nil, false
	}
	starIndex := -1
	for index, pattern := range patterns {
		if pattern.star != nil {
			starIndex = index
			break
		}
	}

	if starIndex < 0 && len(patterns) != len(subjectValues) {
		return false, nil, true
	}
	if starIndex >= 0 && len(subjectValues) < len(patterns)-1 {
		return false, nil, true
	}

	captures := make(map[string]ssa.Value)
	matchItem := func(pattern matchCasePatternItem, subjectValue ssa.Value) (bool, bool) {
		if pattern.test != nil {
			text := strings.TrimSpace(pattern.test.GetText())
			switch {
			case text == "_":
				return true, true
			case b.isBareCapturePattern(pattern.test):
				captures[text] = subjectValue
				return true, true
			case len(splitTopLevelMatchAlternatives(text)) > 1:
				for _, alternative := range splitTopLevelMatchAlternatives(text) {
					patternValue, ok := b.buildMatchPatternValueFromText(alternative)
					if !ok || patternValue == nil {
						return false, false
					}
					if staticValuesEqual(subjectValue, patternValue) {
						return true, true
					}
				}
				return false, true
			default:
				patternValue, ok := b.buildMatchPatternValueFromText(text)
				if !ok || patternValue == nil {
					return false, false
				}
				return staticValuesEqual(subjectValue, patternValue), true
			}
		}
		if pattern.star != nil {
			exprCtx, ok := pattern.star.Expr().(*pythonparser.ExprContext)
			if !ok || exprCtx == nil {
				return false, false
			}
			target := b.extractAssignTargetFromExpr(exprCtx)
			if target.String() == "" || target.String() == "_" {
				return true, true
			}
			captures[target.String()] = subjectValue
			return true, true
		}
		return false, false
	}

	if starIndex < 0 {
		for index, pattern := range patterns {
			matched, handled := matchItem(pattern, subjectValues[index])
			if !handled {
				return false, nil, false
			}
			if !matched {
				return false, nil, true
			}
		}
	} else {
		for index := 0; index < starIndex; index++ {
			matched, handled := matchItem(patterns[index], subjectValues[index])
			if !handled {
				return false, nil, false
			}
			if !matched {
				return false, nil, true
			}
		}
		starSubject := b.buildSliceValueFromValues(subjectValues[starIndex : len(subjectValues)-(len(patterns)-starIndex-1)])
		matched, handled := matchItem(patterns[starIndex], starSubject)
		if !handled {
			return false, nil, false
		}
		if !matched {
			return false, nil, true
		}
		suffix := len(patterns) - starIndex - 1
		for offset := 0; offset < suffix; offset++ {
			subjectIndex := len(subjectValues) - suffix + offset
			matched, handled := matchItem(patterns[starIndex+1+offset], subjectValues[subjectIndex])
			if !handled {
				return false, nil, false
			}
			if !matched {
				return false, nil, true
			}
		}
	}

	if guardTest != nil {
		if matched, ok := evaluateStaticMatchGuard(guardTest, captures); ok {
			if !matched {
				return false, nil, true
			}
		} else {
			return false, nil, false
		}
	}

	return true, captures, true
}

func (b *singleFileBuilder) bindMatchCaptures(captures map[string]ssa.Value) {
	for name, value := range captures {
		if name == "" || value == nil {
			continue
		}
		b.AssignVariable(b.createVar(name), value)
	}
}

type matchCasePatternItem struct {
	test *pythonparser.TestContext
	star *pythonparser.Star_exprContext
}

func collectCaseClauseTests(raw *pythonparser.Case_clauseContext) []pythonparser.ITestContext {
	if raw == nil {
		return nil
	}
	children := raw.GetChildren()
	tests := make([]pythonparser.ITestContext, 0, len(children))
	for _, child := range children {
		if test, ok := child.(pythonparser.ITestContext); ok {
			tests = append(tests, test)
		}
	}
	return tests
}

func extractCasePatternItems(raw *pythonparser.Case_clauseContext) ([]matchCasePatternItem, *pythonparser.TestContext, bool) {
	if raw == nil {
		return nil, nil, false
	}
	patterns := raw.AllCase_pattern()
	if len(patterns) == 0 {
		return nil, nil, false
	}

	var guardTest *pythonparser.TestContext
	allTests := collectCaseClauseTests(raw)
	if raw.IF() != nil {
		last, ok := allTests[len(allTests)-1].(*pythonparser.TestContext)
		if !ok || last == nil {
			return nil, nil, false
		}
		guardTest = last
	}

	results := make([]matchCasePatternItem, 0, len(patterns))
	for _, pattern := range patterns {
		patternCtx, ok := pattern.(*pythonparser.Case_patternContext)
		if !ok || patternCtx == nil {
			return nil, nil, false
		}
		if test := patternCtx.Test(); test != nil {
			testCtx, ok := test.(*pythonparser.TestContext)
			if !ok || testCtx == nil {
				return nil, nil, false
			}
			results = append(results, matchCasePatternItem{test: testCtx})
			continue
		}
		if star := patternCtx.Star_expr(); star != nil {
			starCtx, ok := star.(*pythonparser.Star_exprContext)
			if !ok || starCtx == nil {
				return nil, nil, false
			}
			results = append(results, matchCasePatternItem{star: starCtx})
			continue
		}
		return nil, nil, false
	}
	return results, guardTest, true
}

func (b *singleFileBuilder) buildDynamicSequenceCasePlan(subject ssa.Value, raw *pythonparser.Case_clauseContext) (ssa.Value, map[string]ssa.Value, *pythonparser.TestContext, bool) {
	if subject == nil || raw == nil || len(raw.AllCOMMA()) == 0 {
		return nil, nil, nil, false
	}
	patternItems, guardTest, ok := extractCasePatternItems(raw)
	if !ok {
		return nil, nil, nil, false
	}

	captures := make(map[string]ssa.Value)
	var cond ssa.Value
	appendCondition := func(nextCond ssa.Value) bool {
		if nextCond == nil {
			return false
		}
		if cond == nil {
			cond = nextCond
		} else {
			cond = b.EmitBinOp(ssa.OpLogicAnd, cond, nextCond)
		}
		return cond != nil
	}

	starIndex := -1
	for index, pattern := range patternItems {
		if pattern.star != nil {
			if starIndex >= 0 {
				return nil, nil, nil, false
			}
			starIndex = index
		}
	}
	if starIndex >= 0 && starIndex != len(patternItems)-1 {
		// TODO(python): support dynamic sequence star-patterns with suffix items once
		// we have a reliable dynamic length/index path for sequence subjects.
		return nil, nil, nil, false
	}

	for index, pattern := range patternItems {
		if pattern.star != nil {
			exprCtx, ok := pattern.star.Expr().(*pythonparser.ExprContext)
			if !ok || exprCtx == nil {
				return nil, nil, nil, false
			}
			target := b.extractAssignTargetFromExpr(exprCtx)
			if target.String() == "" || target.String() == "_" {
				continue
			}
			restValue := b.EmitMakeSlice(subject, b.EmitConstInst(int64(index)), nil, nil)
			if restValue == nil {
				return nil, nil, nil, false
			}
			captures[target.String()] = restValue
			continue
		}
		text := strings.TrimSpace(pattern.test.GetText())
		subjectItem := b.ReadMemberCallValue(subject, b.EmitConstInst(int64(index)))
		if subjectItem == nil {
			subjectItem = b.undefinedLoopValue(fmt.Sprintf("match_item_%d", index))
		}

		switch {
		case text == "_":
			continue
		case b.isBareCapturePattern(pattern.test):
			captures[text] = subjectItem
			continue
		case len(splitTopLevelMatchAlternatives(text)) > 1:
			var altCond ssa.Value
			for _, alternative := range splitTopLevelMatchAlternatives(text) {
				patternValue, ok := b.buildMatchPatternValueFromText(alternative)
				if !ok || patternValue == nil {
					return nil, nil, nil, false
				}
				candidate := b.EmitBinOp(ssa.OpEq, subjectItem, patternValue)
				if candidate == nil {
					return nil, nil, nil, false
				}
				if altCond == nil {
					altCond = candidate
				} else {
					altCond = b.EmitBinOp(ssa.OpLogicOr, altCond, candidate)
				}
			}
			if !appendCondition(altCond) {
				return nil, nil, nil, false
			}
		default:
			patternValue, ok := b.buildMatchPatternValueFromText(text)
			if !ok || patternValue == nil {
				return nil, nil, nil, false
			}
			if !appendCondition(b.EmitBinOp(ssa.OpEq, subjectItem, patternValue)) {
				return nil, nil, nil, false
			}
		}
	}

	if cond == nil {
		cond = b.EmitConstInst(true)
	}
	return cond, captures, guardTest, true
}

func (b *singleFileBuilder) buildMatchGuardChain(captures map[string]ssa.Value, guard *pythonparser.TestContext, suite pythonparser.ISuiteContext, fallback func()) {
	b.bindMatchCaptures(captures)
	if guard == nil {
		if suite != nil {
			b.VisitSuite(suite)
		}
		return
	}

	guardValue, ok := b.VisitTest(guard).(ssa.Value)
	if !ok || guardValue == nil {
		if fallback != nil {
			fallback()
		}
		return
	}
	if constGuard, ok := guardValue.(*ssa.ConstInst); ok && constGuard.IsBoolean() {
		if constGuard.Boolean() {
			if suite != nil {
				b.VisitSuite(suite)
			}
		} else if fallback != nil {
			fallback()
		}
		return
	}

	guardBuilder := b.CreateIfBuilder()
	guardBuilder.SetCondition(func() ssa.Value {
		return guardValue
	}, func() {
		if suite != nil {
			b.VisitSuite(suite)
		}
	})
	if fallback != nil {
		guardBuilder.SetElse(func() {
			fallback()
		})
	}
	guardBuilder.Build()
}

func (b *singleFileBuilder) buildDynamicMatchSequenceCases(subject ssa.Value, cases []*pythonparser.Case_clauseContext, index int) {
	if index >= len(cases) {
		return
	}
	caseCtx := cases[index]
	if caseCtx == nil {
		b.buildDynamicMatchSequenceCases(subject, cases, index+1)
		return
	}
	suite := caseCtx.Suite()

	patterns, guardTest, ok := extractCasePatternItems(caseCtx)
	if ok && len(patterns) == 1 && patterns[0].test != nil && strings.TrimSpace(patterns[0].test.GetText()) == "_" {
		b.buildMatchGuardChain(nil, guardTest, suite, func() {
			b.buildDynamicMatchSequenceCases(subject, cases, index+1)
		})
		return
	}

	cond, captures, guard, handled := b.buildDynamicSequenceCasePlan(subject, caseCtx)
	if !handled || cond == nil {
		// Fallback for patterns we still do not model precisely: keep compiling the
		// remaining branch bodies so the match statement does not disappear entirely.
		for _, fallbackCase := range cases[index:] {
			if fallbackCase == nil || fallbackCase.Suite() == nil {
				continue
			}
			b.VisitSuite(fallbackCase.Suite())
			if b.shouldStopStatementWalk() {
				break
			}
		}
		return
	}

	if constCond, ok := cond.(*ssa.ConstInst); ok && constCond.IsBoolean() {
		if constCond.Boolean() {
			b.buildMatchGuardChain(captures, guard, suite, func() {
				b.buildDynamicMatchSequenceCases(subject, cases, index+1)
			})
		} else {
			b.buildDynamicMatchSequenceCases(subject, cases, index+1)
		}
		return
	}

	ifBuilder := b.CreateIfBuilder()
	condValue := cond
	ifBuilder.SetCondition(func() ssa.Value {
		return condValue
	}, func() {
		b.buildMatchGuardChain(captures, guard, suite, func() {
			b.buildDynamicMatchSequenceCases(subject, cases, index+1)
		})
	})
	ifBuilder.SetElse(func() {
		b.buildDynamicMatchSequenceCases(subject, cases, index+1)
	})
	ifBuilder.Build()
}

func (b *singleFileBuilder) buildMatchCaseCondition(subject ssa.Value, raw *pythonparser.Case_clauseContext) (ssa.Value, bool, bool) {
	if subject == nil || raw == nil {
		return nil, false, false
	}

	tests := collectCaseClauseTests(raw)
	if len(tests) == 0 {
		return nil, false, false
	}

	patternTests := tests
	var guardTest *pythonparser.TestContext
	if raw.IF() != nil {
		last, ok := tests[len(tests)-1].(*pythonparser.TestContext)
		if !ok || last == nil {
			return nil, false, false
		}
		guardTest = last
		patternTests = tests[:len(tests)-1]
	}

	if len(raw.AllCOMMA()) > 0 {
		return nil, false, false
	}

	var cond ssa.Value
	wildcardOnly := false

	appendPatternValue := func(patternValue ssa.Value) bool {
		caseCond := b.EmitBinOp(ssa.OpEq, subject, patternValue)
		if caseCond == nil {
			return false
		}
		if cond == nil {
			cond = caseCond
		} else {
			cond = b.EmitBinOp(ssa.OpLogicOr, cond, caseCond)
		}
		return cond != nil
	}

	for _, pattern := range patternTests {
		testCtx, ok := pattern.(*pythonparser.TestContext)
		if !ok || testCtx == nil {
			return nil, false, false
		}

		text := testCtx.GetText()
		if text == "_" {
			wildcardOnly = true
			cond = b.EmitConstInst(true)
			break
		}

		if alternatives := splitTopLevelMatchAlternatives(text); len(alternatives) > 1 {
			for _, alternative := range alternatives {
				if alternative == "_" {
					wildcardOnly = true
					cond = b.EmitConstInst(true)
					break
				}
				patternValue, ok := b.buildMatchPatternValueFromText(alternative)
				if !ok || patternValue == nil {
					return nil, false, false
				}
				if !appendPatternValue(patternValue) {
					return nil, false, false
				}
			}
			if wildcardOnly {
				break
			}
			continue
		}

		if b.isBareCapturePattern(testCtx) {
			return nil, false, false
		}

		patternValue, ok := b.VisitTest(testCtx).(ssa.Value)
		if !ok || patternValue == nil {
			return nil, false, false
		}
		if !appendPatternValue(patternValue) {
			return nil, false, false
		}
	}

	if cond == nil {
		return nil, false, false
	}

	if guardTest != nil {
		guardValue, ok := b.VisitTest(guardTest).(ssa.Value)
		if !ok || guardValue == nil {
			return nil, false, false
		}
		cond = b.EmitBinOp(ssa.OpLogicAnd, cond, guardValue)
		wildcardOnly = false
	}

	return cond, wildcardOnly, true
}

func shouldBuildStaticCatch(control *tryControl, exceptCtx *pythonparser.Except_clauseContext, matched *bool) bool {
	if control == nil || exceptCtx == nil || matched == nil {
		return true
	}
	if control.lastRaisedType == "" {
		return true
	}
	if *matched {
		return false
	}
	if exceptCtx.Test() == nil {
		*matched = true
		return true
	}
	testCtx, ok := exceptCtx.Test().(*pythonparser.TestContext)
	if !ok || testCtx == nil {
		return true
	}
	catchTypes := extractSimpleQualifiedNamesFromTest(testCtx)
	if len(catchTypes) == 0 {
		return true
	}
	for _, catchType := range catchTypes {
		if catchType == control.lastRaisedType {
			*matched = true
			return true
		}
	}
	return false
}

func inferStaticRaisedTypeFromSmallStmt(raw pythonparser.ISmall_stmtContext) string {
	raiseStmt, ok := raw.(*pythonparser.Raise_stmtContext)
	if !ok || raiseStmt == nil {
		return ""
	}
	tests := raiseStmt.AllTest()
	if len(tests) == 0 {
		return ""
	}
	testCtx, ok := tests[0].(*pythonparser.TestContext)
	if !ok || testCtx == nil {
		return ""
	}
	return inferRaisedTypeName(testCtx)
}

func inferStaticRaisedTypeFromSuite(raw pythonparser.ISuiteContext) string {
	suiteCtx, ok := raw.(*pythonparser.SuiteContext)
	if !ok || suiteCtx == nil {
		return ""
	}

	if simpleStmt := suiteCtx.Simple_stmt(); simpleStmt != nil {
		if simpleCtx, ok := simpleStmt.(*pythonparser.Simple_stmtContext); ok && simpleCtx != nil {
			for _, small := range simpleCtx.AllSmall_stmt() {
				if raisedType := inferStaticRaisedTypeFromSmallStmt(small); raisedType != "" {
					return raisedType
				}
			}
		}
		return ""
	}

	for _, stmt := range suiteCtx.AllStmt() {
		stmtCtx, ok := stmt.(*pythonparser.StmtContext)
		if !ok || stmtCtx == nil {
			continue
		}
		if simple := stmtCtx.Simple_stmt(); simple != nil {
			if simpleCtx, ok := simple.(*pythonparser.Simple_stmtContext); ok && simpleCtx != nil {
				for _, small := range simpleCtx.AllSmall_stmt() {
					if raisedType := inferStaticRaisedTypeFromSmallStmt(small); raisedType != "" {
						return raisedType
					}
				}
			}
		} else {
			// Stop once control flow becomes non-trivial; keep the optimization conservative.
			return ""
		}
	}
	return ""
}

// VisitMatchStmt visits a match_stmt node.
func (b *singleFileBuilder) VisitMatchStmt(raw *pythonparser.Match_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	testCtx, ok := raw.Test().(*pythonparser.TestContext)
	if !ok || testCtx == nil {
		return nil
	}
	subject, ok := b.VisitTest(testCtx).(ssa.Value)
	if !ok || subject == nil {
		return nil
	}

	caseClauses := raw.AllCase_clause()
	if len(caseClauses) == 0 {
		return nil
	}

	typedCases := make([]*pythonparser.Case_clauseContext, 0, len(caseClauses))
	hasSequenceCase := false
	for _, clause := range caseClauses {
		caseCtx, ok := clause.(*pythonparser.Case_clauseContext)
		if !ok || caseCtx == nil {
			continue
		}
		typedCases = append(typedCases, caseCtx)
		if len(caseCtx.AllCOMMA()) > 0 {
			hasSequenceCase = true
		}
	}

	if subjectValues, ok := b.extractStaticSequenceValues(subject); ok {
		allHandled := true
		for _, caseCtx := range typedCases {
			if tests := collectCaseClauseTests(caseCtx); len(tests) == 1 {
				if testCtx, ok := tests[0].(*pythonparser.TestContext); ok && testCtx != nil && strings.TrimSpace(testCtx.GetText()) == "_" {
					if suite := caseCtx.Suite(); suite != nil {
						b.VisitSuite(suite)
					}
					return nil
				}
			}
			matched, captures, handled := b.matchStaticSequenceCase(subjectValues, caseCtx)
			if !handled {
				allHandled = false
				continue
			}
			if matched {
				b.bindMatchCaptures(captures)
				if suite := caseCtx.Suite(); suite != nil {
					b.VisitSuite(suite)
				}
				return nil
			}
		}
		if allHandled {
			return nil
		}
	}

	if hasSequenceCase {
		b.buildDynamicMatchSequenceCases(subject, typedCases, 0)
		return nil
	}

	type matchCasePlan struct {
		cond         ssa.Value
		suite        pythonparser.ISuiteContext
		wildcardOnly bool
	}

	simpleCases := make([]*pythonparser.Case_clauseContext, 0, len(typedCases))
	for _, caseCtx := range typedCases {
		if _, _, ok := b.buildMatchCaseCondition(subject, caseCtx); !ok {
			// Fallback for richer sequence/capture patterns: still compile all suites so
			// AST-to-SSA stays live until full structural pattern lowering is implemented.
			for _, fallbackCtx := range typedCases {
				if fallbackCtx == nil {
					continue
				}
				if suite := fallbackCtx.Suite(); suite != nil {
					b.VisitSuite(suite)
					if b.shouldStopStatementWalk() {
						break
					}
				}
			}
			return nil
		}
		simpleCases = append(simpleCases, caseCtx)
	}

	plans := make([]matchCasePlan, 0, len(simpleCases))
	allConstant := true
	for _, caseCtx := range simpleCases {
		suite := caseCtx.Suite()
		if suite == nil {
			continue
		}
		cond, wildcardOnly, ok := b.buildMatchCaseCondition(subject, caseCtx)
		if !ok || cond == nil {
			continue
		}
		plans = append(plans, matchCasePlan{
			cond:         cond,
			suite:        suite,
			wildcardOnly: wildcardOnly,
		})
		if constCond, ok := cond.(*ssa.ConstInst); !ok || !constCond.IsBoolean() {
			allConstant = false
		}
	}

	if allConstant {
		for _, plan := range plans {
			constCond, _ := plan.cond.(*ssa.ConstInst)
			if constCond != nil && constCond.IsBoolean() && constCond.Boolean() {
				b.VisitSuite(plan.suite)
				return nil
			}
		}
		return nil
	}

	ifBuilder := b.CreateIfBuilder()
	for _, plan := range plans {
		if plan.wildcardOnly {
			ifBuilder.SetElse(func() {
				b.VisitSuite(plan.suite)
			})
			break
		}

		condValue := plan.cond
		suite := plan.suite
		ifBuilder.SetCondition(func() ssa.Value {
			return condValue
		}, func() {
			b.VisitSuite(suite)
		})
	}

	ifBuilder.Build()
	return nil
}

// VisitWithStmt visits a with_stmt node.
func (b *singleFileBuilder) VisitWithStmt(raw *pythonparser.With_stmtContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	for _, item := range raw.AllWith_item() {
		itemCtx, ok := item.(*pythonparser.With_itemContext)
		if !ok || itemCtx == nil {
			continue
		}
		testCtx, ok := itemCtx.Test().(*pythonparser.TestContext)
		if !ok || testCtx == nil {
			continue
		}
		value, ok := b.VisitTest(testCtx).(ssa.Value)
		if !ok || value == nil {
			continue
		}
		if expr := itemCtx.Expr(); expr != nil {
			target := b.extractAssignTargetFromExpr(expr)
			if target.String() != "" {
				b.assignToTarget(target, value)
			}
		}
	}
	if suite := raw.Suite(); suite != nil {
		b.VisitSuite(suite)
	}
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
			if b.shouldStopStatementWalk() {
				break
			}
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
	b.ensureBlueprintConstructorSlot(blueprint)

	b.handleClassInheritance(blueprint, arglist)

	b.visitClassBody(suite, blueprint)

	return nil
}

// VisitFuncdef visits a funcdef node and builds SSA representation for function definition.
// It creates a function blueprint, parses parameters, builds function body, and registers it in scope.
// Example Python code: `def foo(a, b): return a + b`
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

	nameCtx := funcdef.Name()
	if nameCtx == nil {
		return nil
	}
	funcName := nameCtx.GetText()

	suite := funcdef.Suite()
	if suite == nil {
		return nil
	}

	newFunc := b.NewFunc(funcName)
	b.FunctionBuilder = b.PushFunction(newFunc)

	if params := funcdef.Typedargslist(); params != nil {
		b.buildFuncParams(params)
	}

	b.VisitSuite(suite)
	b.Finish()
	b.FunctionBuilder = b.PopFunction()

	funcVar := b.CreateVariable(funcName)
	b.AssignVariable(funcVar, newFunc)

	return nil
}

// extractNameFromNamedParameter extracts the identifier from a named_parameter node.
// named_parameter : name (COLON test)?
func extractNameFromNamedParameter(np pythonparser.INamed_parameterContext) string {
	if np == nil {
		return ""
	}
	ctx, ok := np.(*pythonparser.Named_parameterContext)
	if !ok {
		return ""
	}
	if n := ctx.Name(); n != nil {
		return n.GetText()
	}
	return ""
}

// collectParamNames collects parameter names from a typedargslist.
// *args names are prefixed with "*", **kwargs names with "**".
func collectParamNames(paramsCtx *pythonparser.TypedargslistContext) []string {
	var names []string

	addFromDefParams := func(dp pythonparser.IDef_parametersContext) {
		ctx, ok := dp.(*pythonparser.Def_parametersContext)
		if !ok {
			return
		}
		for _, defParam := range ctx.AllDef_parameter() {
			dp2, ok := defParam.(*pythonparser.Def_parameterContext)
			if !ok {
				continue
			}
			if np := dp2.Named_parameter(); np != nil { // bare STAR separator has no name
				if name := extractNameFromNamedParameter(np); name != "" {
					names = append(names, name)
				}
			}
		}
	}

	for _, dp := range paramsCtx.AllDef_parameters() {
		addFromDefParams(dp)
	}

	if argsCtx := paramsCtx.Args(); argsCtx != nil {
		if ac, ok := argsCtx.(*pythonparser.ArgsContext); ok {
			if name := extractNameFromNamedParameter(ac.Named_parameter()); name != "" {
				names = append(names, "*"+name)
			}
		}
	}

	if kwargsCtx := paramsCtx.Kwargs(); kwargsCtx != nil {
		if kc, ok := kwargsCtx.(*pythonparser.KwargsContext); ok {
			if name := extractNameFromNamedParameter(kc.Named_parameter()); name != "" {
				names = append(names, "**"+name)
			}
		}
	}

	return names
}

// registerParam strips the leading "*"/"**" prefix and registers the name as an SSA parameter.
func (b *singleFileBuilder) registerParam(name string) {
	rawName := name
	clean := strings.TrimLeft(name, "*")
	if clean != "" {
		param := b.NewParam(clean)
		if param != nil && param.GetType() == nil {
			param.SetType(ssa.CreateAnyType())
		}
		if strings.HasPrefix(rawName, "**") {
			// Keep the formal parameter for call-argument alignment, but bind the
			// in-function variable to a dynamic object so kwargs.pop()/kwargs["k"]
			// style real-project code does not collapse into a scalar positional value.
			placeholder := b.newDynamicPlaceholder(clean)
			if placeholder != nil {
				b.AssignVariable(b.CreateVariable(clean), placeholder)
			}
		}
	}
}

// buildFuncParams builds function parameters from typedargslist.
// Handles: positional params, params with defaults, *args, **kwargs, type annotations.
func (b *singleFileBuilder) buildFuncParams(params pythonparser.ITypedargslistContext) {
	if params == nil {
		return
	}
	paramsCtx, ok := params.(*pythonparser.TypedargslistContext)
	if !ok {
		return
	}
	for _, name := range collectParamNames(paramsCtx) {
		b.registerParam(name)
	}
}

// buildFuncParamsSkipFirst builds function parameters, skipping the first one (self/cls).
// Used for class methods where the first parameter is implicitly handled.
func (b *singleFileBuilder) buildFuncParamsSkipFirst(params pythonparser.ITypedargslistContext) {
	if params == nil {
		return
	}
	paramsCtx, ok := params.(*pythonparser.TypedargslistContext)
	if !ok {
		return
	}
	names := collectParamNames(paramsCtx)
	for _, name := range names[1:] { // skip self / cls
		b.registerParam(name)
	}
}
