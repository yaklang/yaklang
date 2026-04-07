package python2ssa

import (
	"fmt"
	"sort"
	"strings"

	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func constStringValue(v ssa.Value) (string, bool) {
	inst, ok := v.(ssa.Instruction)
	if !ok {
		return "", false
	}
	constInst, ok := inst.(*ssa.ConstInst)
	if !ok || !constInst.IsString() {
		return "", false
	}
	return constInst.VarString(), true
}

func inferLiteralMapKeyType(values []ssa.Value) ssa.Type {
	if len(values) == 0 {
		return ssa.CreateAnyType()
	}

	type typedKey struct {
		signature string
		typ       ssa.Type
	}

	keys := make([]typedKey, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		if value == nil || value.GetType() == nil {
			continue
		}
		signature := value.GetType().String()
		if _, exists := seen[signature]; exists {
			continue
		}
		seen[signature] = struct{}{}
		keys = append(keys, typedKey{
			signature: signature,
			typ:       value.GetType(),
		})
	}

	if len(keys) == 0 {
		return ssa.CreateAnyType()
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].signature < keys[j].signature
	})
	if len(keys) == 1 {
		return keys[0].typ
	}

	types := make([]ssa.Type, 0, len(keys))
	for _, key := range keys {
		types = append(types, key.typ)
	}
	return ssa.NewOrType(types...)
}

func (b *singleFileBuilder) extractStaticIterableItems(value ssa.Value) ([]ssa.Value, bool) {
	if items, ok := b.extractStaticSequenceValues(value); ok {
		return items, true
	}
	if keys, ok := b.extractStaticMapKeys(value); ok {
		return keys, true
	}
	return nil, false
}

func collectStaticComprehensionIfs(raw pythonparser.IComp_iterContext) ([]*pythonparser.TestContext, bool) {
	if raw == nil {
		return nil, true
	}
	iterCtx, ok := raw.(*pythonparser.Comp_iterContext)
	if !ok || iterCtx == nil {
		return nil, false
	}
	if iterCtx.Comp_for() != nil {
		return nil, false
	}
	testCtx, ok := iterCtx.Test().(*pythonparser.TestContext)
	if !ok || testCtx == nil {
		return nil, false
	}
	rest, ok := collectStaticComprehensionIfs(iterCtx.Comp_iter())
	if !ok {
		return nil, false
	}
	return append([]*pythonparser.TestContext{testCtx}, rest...), true
}

func staticComprehensionFilterPass(value ssa.Value) bool {
	constant, ok := value.(*ssa.ConstInst)
	if !ok || constant == nil {
		return false
	}
	switch {
	case constant.IsBoolean():
		return constant.Boolean()
	case constant.IsNumber():
		return constant.Number() != 0
	case constant.IsString():
		return constant.VarString() != ""
	}
	return false
}

type staticComprehensionPlan struct {
	targets []loopAssignTarget
	items   []ssa.Value
	filters []*pythonparser.TestContext
}

func (b *singleFileBuilder) prepareStaticComprehension(compFor *pythonparser.Comp_forContext) (*staticComprehensionPlan, bool) {
	if compFor == nil {
		return nil, false
	}
	targets := b.extractLoopTargets(compFor.Exprlist())
	if len(targets) == 0 {
		return nil, false
	}
	logicalTest, ok := compFor.Logical_test().(*pythonparser.Logical_testContext)
	if !ok || logicalTest == nil {
		return nil, false
	}
	iterableValue, ok := b.VisitLogicalTest(logicalTest).(ssa.Value)
	if !ok || iterableValue == nil {
		return nil, false
	}
	items, ok := b.extractStaticIterableItems(iterableValue)
	if !ok {
		return nil, false
	}
	filters, ok := collectStaticComprehensionIfs(compFor.Comp_iter())
	if !ok {
		return nil, false
	}
	return &staticComprehensionPlan{targets: targets, items: items, filters: filters}, true
}

func (b *singleFileBuilder) evaluateStaticComprehension(plan *staticComprehensionPlan, build func() (ssa.Value, bool)) ([]ssa.Value, bool) {
	if plan == nil {
		return nil, false
	}
	results := make([]ssa.Value, 0, len(plan.items))
	for _, item := range plan.items {
		b.assignLoopTargets(plan.targets, item)
		include := true
		for _, filter := range plan.filters {
			filterValue, ok := b.VisitTest(filter).(ssa.Value)
			if !ok || filterValue == nil {
				return nil, false
			}
			if !staticComprehensionFilterPass(filterValue) {
				include = false
				break
			}
		}
		if !include {
			continue
		}
		resultValue, ok := build()
		if !ok || resultValue == nil {
			return nil, false
		}
		results = append(results, resultValue)
	}
	return results, true
}

func (b *singleFileBuilder) buildStaticComprehensionValues(raw *pythonparser.Testlist_compContext) ([]ssa.Value, bool) {
	if raw == nil || raw.Comp_for() == nil {
		return nil, false
	}
	tests := raw.AllTest()
	if len(tests) != 1 {
		return nil, false
	}
	bodyTest, ok := tests[0].(*pythonparser.TestContext)
	if !ok || bodyTest == nil {
		return nil, false
	}
	compFor, ok := raw.Comp_for().(*pythonparser.Comp_forContext)
	if !ok || compFor == nil {
		return nil, false
	}
	plan, ok := b.prepareStaticComprehension(compFor)
	if !ok {
		return nil, false
	}
	return b.evaluateStaticComprehension(plan, func() (ssa.Value, bool) {
		resultValue, ok := b.VisitTest(bodyTest).(ssa.Value)
		return resultValue, ok
	})
}

func (b *singleFileBuilder) buildStaticListComprehension(raw *pythonparser.Testlist_compContext) ssa.Value {
	results, ok := b.buildStaticComprehensionValues(raw)
	if !ok {
		return nil
	}
	return b.buildSliceValueFromValues(results)
}

func (b *singleFileBuilder) buildStaticDictComprehension(raw *pythonparser.DictorsetmakerContext) ssa.Value {
	if raw == nil || raw.Comp_for() == nil {
		return nil
	}
	tests := raw.AllTest()
	if len(tests) != 2 {
		return nil
	}
	keyTest, ok := tests[0].(*pythonparser.TestContext)
	if !ok || keyTest == nil {
		return nil
	}
	valueTest, ok := tests[1].(*pythonparser.TestContext)
	if !ok || valueTest == nil {
		return nil
	}
	compFor, ok := raw.Comp_for().(*pythonparser.Comp_forContext)
	if !ok || compFor == nil {
		return nil
	}
	plan, ok := b.prepareStaticComprehension(compFor)
	if !ok {
		return nil
	}

	type dictEntry struct {
		key   ssa.Value
		value ssa.Value
	}
	entries := make([]dictEntry, 0, len(plan.items))
	indexByKey := make(map[string]int)
	for _, item := range plan.items {
		b.assignLoopTargets(plan.targets, item)
		include := true
		for _, filter := range plan.filters {
			filterValue, ok := b.VisitTest(filter).(ssa.Value)
			if !ok || filterValue == nil {
				return nil
			}
			if !staticComprehensionFilterPass(filterValue) {
				include = false
				break
			}
		}
		if !include {
			continue
		}
		keyValue, ok := b.VisitTest(keyTest).(ssa.Value)
		if !ok || keyValue == nil {
			return nil
		}
		valueValue, ok := b.VisitTest(valueTest).(ssa.Value)
		if !ok || valueValue == nil {
			return nil
		}
		signature := staticValueSortKey(keyValue)
		if index, exists := indexByKey[signature]; exists {
			entries[index] = dictEntry{key: keyValue, value: valueValue}
			continue
		}
		indexByKey[signature] = len(entries)
		entries = append(entries, dictEntry{key: keyValue, value: valueValue})
	}

	sort.Slice(entries, func(i, j int) bool {
		return staticValueSortKey(entries[i].key) < staticValueSortKey(entries[j].key)
	})

	keyValues := make([]ssa.Value, 0, len(entries))
	for _, entry := range entries {
		keyValues = append(keyValues, entry.key)
	}

	dict := b.EmitMakeBuildWithType(ssa.NewMapType(inferLiteralMapKeyType(keyValues), ssa.CreateAnyType()), nil, nil)
	for _, entry := range entries {
		member := b.CreateMemberCallVariable(dict, entry.key)
		b.AssignVariable(member, entry.value)
	}
	return dict
}

// VisitTest visits a test node.
// This handles expressions.
func (b *singleFileBuilder) VisitTest(raw *pythonparser.TestContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	if raw.COLON_ASSIGN() != nil {
		tests := raw.AllTest()
		if len(tests) > 0 {
			if rhs, ok := tests[0].(*pythonparser.TestContext); ok {
				result := b.VisitTest(rhs)
				if value, ok := result.(ssa.Value); ok {
					if logicalTests := raw.AllLogical_test(); len(logicalTests) > 0 {
						if name := b.extractVariableNameFromLogicalTest(logicalTests[0]); name != "" {
							b.AssignVariable(b.createVar(name), value)
						}
					}
					return value
				}
				return result
			}
		}
	}

	// Visit logical_test if present
	logicalTests := raw.AllLogical_test()
	if len(logicalTests) > 0 {
		if lt, ok := logicalTests[0].(*pythonparser.Logical_testContext); ok {
			return b.VisitLogicalTest(lt)
		}
	}

	return nil
}

// VisitLogicalTest visits a logical_test node.
// This handles logical expressions (and/or).
// logical_test: comparison | NOT logical_test | logical_test AND logical_test | logical_test OR logical_test
func (b *singleFileBuilder) VisitLogicalTest(raw *pythonparser.Logical_testContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Check for binary logical expression (AND/OR)
	logicalTests := raw.AllLogical_test()
	if len(logicalTests) == 2 {
		leftLt, lok := logicalTests[0].(*pythonparser.Logical_testContext)
		rightLt, rok := logicalTests[1].(*pythonparser.Logical_testContext)
		if lok && rok {
			leftVal := b.VisitLogicalTest(leftLt)
			rightVal := b.VisitLogicalTest(rightLt)

			var left, right ssa.Value
			if v, ok := leftVal.(ssa.Value); ok {
				left = v
			}
			if v, ok := rightVal.(ssa.Value); ok {
				right = v
			}

			if left != nil && right != nil {
				// Check which operator
				if raw.AND() != nil {
					return b.EmitBinOp(ssa.OpLogicAnd, left, right)
				}
				if raw.OR() != nil {
					return b.EmitBinOp(ssa.OpLogicOr, left, right)
				}
			}
		}
		return nil
	}

	// Check for NOT expression
	if len(logicalTests) == 1 && raw.NOT() != nil {
		ltCtx, ok := logicalTests[0].(*pythonparser.Logical_testContext)
		if ok {
			val := b.VisitLogicalTest(ltCtx)
			if v, ok := val.(ssa.Value); ok {
				return b.EmitUnOp(ssa.OpNot, v)
			}
		}
		return nil
	}

	// Visit comparison if present (base case)
	if comparison := raw.Comparison(); comparison != nil {
		if comp, ok := comparison.(*pythonparser.ComparisonContext); ok {
			return b.VisitComparison(comp)
		}
	}

	return nil
}

// VisitComparison visits a comparison node.
// This handles comparison expressions (<, >, ==, etc.).
// comparison: comparison (LESS_THAN | GREATER_THAN | ...) comparison | expr
func (b *singleFileBuilder) VisitComparison(raw *pythonparser.ComparisonContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Check for binary comparison expression first
	comparisons := raw.AllComparison()
	if len(comparisons) == 2 {
		// comparison OP comparison
		leftComp, lok := comparisons[0].(*pythonparser.ComparisonContext)
		rightComp, rok := comparisons[1].(*pythonparser.ComparisonContext)
		if lok && rok {
			leftVal := b.VisitComparison(leftComp)
			rightVal := b.VisitComparison(rightComp)

			var left, right ssa.Value
			if v, ok := leftVal.(ssa.Value); ok {
				left = v
			}
			if v, ok := rightVal.(ssa.Value); ok {
				right = v
			}

			if left != nil && right != nil {
				// Determine the comparison operator
				if raw.LESS_THAN() != nil {
					return b.EmitBinOp(ssa.OpLt, left, right)
				}
				if raw.GREATER_THAN() != nil {
					return b.EmitBinOp(ssa.OpGt, left, right)
				}
				if raw.EQUALS() != nil {
					return b.EmitBinOp(ssa.OpEq, left, right)
				}
				if raw.GT_EQ() != nil {
					return b.EmitBinOp(ssa.OpGtEq, left, right)
				}
				if raw.LT_EQ() != nil {
					return b.EmitBinOp(ssa.OpLtEq, left, right)
				}
				if raw.NOT_EQ_1() != nil || raw.NOT_EQ_2() != nil {
					return b.EmitBinOp(ssa.OpNotEq, left, right)
				}
			}
		}
		return nil
	}

	// Get the expression in the comparison (base case)
	// comparison: comparison (LESS_THAN | ...) comparison | expr
	expr := raw.Expr()
	if expr == nil {
		return nil
	}

	// Type assert to concrete type
	exprCtx, ok := expr.(*pythonparser.ExprContext)
	if !ok {
		return nil
	}

	// Visit the expression
	return b.VisitExpr(exprCtx)
}

// VisitExpr visits an expr node.
// This handles arithmetic expressions and function calls.
// expr: AWAIT? atom trailer*
//
//	| <assoc = right> expr op = POWER expr
//	| op = (ADD | MINUS | NOT_OP) expr
//	| expr op = (STAR | DIV | MOD | IDIV | AT) expr
//	| expr op = (ADD | MINUS) expr
//	| expr op = (LEFT_SHIFT | RIGHT_SHIFT) expr
//	| expr op = AND_OP expr
//	| expr op = XOR expr
//	| expr op = OR_OP expr
func (b *singleFileBuilder) VisitExpr(raw *pythonparser.ExprContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Check for binary expressions first
	exprs := raw.AllExpr()
	if len(exprs) == 2 {
		// Binary expression: expr op expr
		leftExpr, lok := exprs[0].(*pythonparser.ExprContext)
		rightExpr, rok := exprs[1].(*pythonparser.ExprContext)
		if lok && rok {
			leftVal := b.VisitExpr(leftExpr)
			rightVal := b.VisitExpr(rightExpr)

			var left, right ssa.Value
			if v, ok := leftVal.(ssa.Value); ok {
				left = v
			}
			if v, ok := rightVal.(ssa.Value); ok {
				right = v
			}

			if left != nil && right != nil {
				op := raw.GetOp()
				if op != nil {
					return b.emitBinaryOp(op.GetTokenType(), left, right)
				}
			}
		}
		return nil
	}

	// Check for unary expressions
	if len(exprs) == 1 {
		// Unary expression: op expr
		exprCtx, ok := exprs[0].(*pythonparser.ExprContext)
		if ok {
			val := b.VisitExpr(exprCtx)
			if v, ok := val.(ssa.Value); ok {
				op := raw.GetOp()
				if op != nil {
					return b.emitUnaryOp(op.GetTokenType(), v)
				}
			}
		}
		return nil
	}

	// Get the atom in the expression (atom trailer* case)
	atom := raw.Atom()
	if atom == nil {
		return nil
	}

	// Type assert to concrete type
	atomCtx, ok := atom.(*pythonparser.AtomContext)
	if !ok {
		return nil
	}

	// Visit the atom to get the base value
	baseValue := b.VisitAtom(atomCtx)
	if baseValue == nil {
		return nil
	}

	// Convert to ssa.Value if needed
	var obj ssa.Value
	if v, ok := baseValue.(ssa.Value); ok {
		obj = v
	} else {
		return baseValue
	}

	// Process all trailers (function calls, attribute access, etc.)
	trailers := raw.AllTrailer()
	for _, trailer := range trailers {
		if trailerCtx, ok := trailer.(*pythonparser.TrailerContext); ok {
			obj = b.VisitTrailer(trailerCtx, obj)
			if obj == nil {
				return nil
			}
		}
	}

	return obj
}

// emitBinaryOp emits a binary operation instruction.
func (b *singleFileBuilder) emitBinaryOp(opType int, left, right ssa.Value) ssa.Value {
	var op ssa.BinaryOpcode
	switch opType {
	case pythonparser.PythonParserADD:
		op = ssa.OpAdd
	case pythonparser.PythonParserMINUS:
		op = ssa.OpSub
	case pythonparser.PythonParserSTAR:
		op = ssa.OpMul
	case pythonparser.PythonParserDIV:
		op = ssa.OpDiv
	case pythonparser.PythonParserMOD:
		op = ssa.OpMod
	case pythonparser.PythonParserIDIV:
		op = ssa.OpDiv // Integer division
	case pythonparser.PythonParserPOWER:
		op = ssa.OpPow
	case pythonparser.PythonParserLEFT_SHIFT:
		op = ssa.OpShl
	case pythonparser.PythonParserRIGHT_SHIFT:
		op = ssa.OpShr
	case pythonparser.PythonParserAND_OP:
		op = ssa.OpAnd
	case pythonparser.PythonParserOR_OP:
		op = ssa.OpOr
	case pythonparser.PythonParserXOR:
		op = ssa.OpXor
	default:
		return nil
	}
	return b.EmitBinOp(op, left, right)
}

// emitUnaryOp emits a unary operation instruction.
func (b *singleFileBuilder) emitUnaryOp(opType int, val ssa.Value) ssa.Value {
	var op ssa.UnaryOpcode
	switch opType {
	case pythonparser.PythonParserMINUS:
		op = ssa.OpNeg
	case pythonparser.PythonParserADD:
		op = ssa.OpPlus
	case pythonparser.PythonParserNOT_OP:
		op = ssa.OpBitwiseNot
	default:
		return nil
	}
	return b.EmitUnOp(op, val)
}

// VisitAtom visits an atom node.
// This handles basic expressions like names, literals, etc.
func (b *singleFileBuilder) VisitAtom(raw *pythonparser.AtomContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Handle different types of atoms
	if name := raw.Name(); name != nil {
		if nameCtx, ok := name.(*pythonparser.NameContext); ok {
			return b.VisitName(nameCtx)
		}
	} else if num := raw.Number(); num != nil {
		if numCtx, ok := num.(*pythonparser.NumberContext); ok {
			return b.VisitNumber(numCtx)
		}
	} else if len(raw.AllSTRING()) > 0 {
		return b.VisitString(raw)
	} else if raw.NONE() != nil {
		return b.EmitConstInst(nil)
	} else if raw.OPEN_BRACKET() != nil && raw.CLOSE_BRACKET() != nil {
		// list literal: [a, b, c]
		// For now treat as slice of numbers/any
		if testlist := raw.Testlist_comp(); testlist != nil {
			if tlc, ok := testlist.(*pythonparser.Testlist_compContext); ok {
				if tlc.Comp_for() != nil {
					if value := b.buildStaticListComprehension(tlc); value != nil {
						return value
					}
				}
				elemValues := make([]ssa.Value, 0)
				for _, t := range tlc.AllTest() {
					if tc, ok := t.(*pythonparser.TestContext); ok {
						if val, ok := b.VisitTest(tc).(ssa.Value); ok {
							elemValues = append(elemValues, val)
						}
					}
				}
				elemType := ssa.CreateAnyType()
				if len(elemValues) > 0 {
					if c, ok := elemValues[0].(*ssa.ConstInst); ok && c.IsNumber() {
						elemType = ssa.CreateNumberType()
					}
				}
				sliceType := ssa.NewSliceType(elemType)
				lst := b.EmitMakeBuildWithType(sliceType, b.EmitConstInst(int64(len(elemValues))), b.EmitConstInst(int64(len(elemValues))))
				for idx, val := range elemValues {
					idxConst := b.EmitConstInst(int64(idx))
					member := b.CreateMemberCallVariable(lst, idxConst)
					b.AssignVariable(member, val)
				}
				return lst
			}
		}
		return b.buildSliceValueFromValues(nil)
	} else if raw.OPEN_BRACE() != nil && raw.CLOSE_BRACE() != nil {
		// dict / set literal
		if dsm := raw.Dictorsetmaker(); dsm != nil {
			if dsCtx, ok := dsm.(*pythonparser.DictorsetmakerContext); ok {
				if !strings.Contains(dsCtx.GetText(), ":") {
					if testlist := dsCtx.Testlist_comp(); testlist != nil {
						if tlc, ok := testlist.(*pythonparser.Testlist_compContext); ok && tlc != nil && tlc.Comp_for() == nil {
							elemValues := make([]ssa.Value, 0, len(tlc.AllTest()))
							seen := make(map[string]struct{})
							for _, test := range tlc.AllTest() {
								testCtx, ok := test.(*pythonparser.TestContext)
								if !ok || testCtx == nil {
									continue
								}
								val, ok := b.VisitTest(testCtx).(ssa.Value)
								if !ok || val == nil {
									continue
								}
								sortKey := staticValueSortKey(val)
								if _, exists := seen[sortKey]; exists {
									continue
								}
								seen[sortKey] = struct{}{}
								elemValues = append(elemValues, val)
							}
							sort.Slice(elemValues, func(i, j int) bool {
								return staticValueSortKey(elemValues[i]) < staticValueSortKey(elemValues[j])
							})
							return b.buildSliceValueFromValues(elemValues)
						}
						if tlc, ok := testlist.(*pythonparser.Testlist_compContext); ok && tlc != nil && tlc.Comp_for() != nil {
							if values, ok := b.buildStaticComprehensionValues(tlc); ok {
								seen := make(map[string]struct{})
								filtered := make([]ssa.Value, 0, len(values))
								for _, value := range values {
									signature := staticValueSortKey(value)
									if _, exists := seen[signature]; exists {
										continue
									}
									seen[signature] = struct{}{}
									filtered = append(filtered, value)
								}
								sort.Slice(filtered, func(i, j int) bool {
									return staticValueSortKey(filtered[i]) < staticValueSortKey(filtered[j])
								})
								return b.buildSliceValueFromValues(filtered)
							}
						}
					}
				}
				if dsCtx.Comp_for() != nil && strings.Contains(dsCtx.GetText(), ":") {
					if value := b.buildStaticDictComprehension(dsCtx); value != nil {
						return value
					}
				}
				tests := dsCtx.AllTest()
				if len(tests) >= 2 && strings.Contains(dsCtx.GetText(), ":") {
					keyValues := make([]ssa.Value, 0, len(tests)/2)
					literalEntries := make([]struct {
						key ssa.Value
						val ssa.Value
					}, 0, len(tests)/2)
					for i := 0; i+1 < len(tests); i += 2 {
						keyTest, valTest := tests[i], tests[i+1]
						keyValRaw := b.VisitTest(keyTest.(*pythonparser.TestContext))
						valValRaw := b.VisitTest(valTest.(*pythonparser.TestContext))
						keyVal, kok := keyValRaw.(ssa.Value)
						valVal, vok := valValRaw.(ssa.Value)
						if !kok || !vok {
							continue
						}
						keyValues = append(keyValues, keyVal)
						literalEntries = append(literalEntries, struct {
							key ssa.Value
							val ssa.Value
						}{
							key: keyVal,
							val: valVal,
						})
					}
					mapType := ssa.NewMapType(inferLiteralMapKeyType(keyValues), ssa.CreateAnyType())
					dict := b.EmitMakeBuildWithType(mapType, nil, nil)
					for _, entry := range literalEntries {
						member := b.CreateMemberCallVariable(dict, entry.key)
						b.AssignVariable(member, entry.val)
					}
					return dict
				}
			}
		}
		// fallback empty map
		return b.EmitMakeBuildWithType(ssa.NewMapType(ssa.CreateAnyType(), ssa.CreateAnyType()), nil, nil)
	} else if raw.OPEN_PAREN() != nil && raw.CLOSE_PAREN() != nil {
		// tuple literal -> treat as immutable slice
		values := make([]ssa.Value, 0)
		if testlist := raw.Testlist_comp(); testlist != nil {
			if tlc, ok := testlist.(*pythonparser.Testlist_compContext); ok {
				for _, t := range tlc.AllTest() {
					if tc, ok := t.(*pythonparser.TestContext); ok {
						if val, ok := b.VisitTest(tc).(ssa.Value); ok {
							values = append(values, val)
						}
					}
				}
			}
		}
		elemType := ssa.CreateAnyType()
		if len(values) > 0 {
			if c, ok := values[0].(*ssa.ConstInst); ok && c.IsNumber() {
				elemType = ssa.CreateNumberType()
			}
		}
		tupleType := ssa.NewSliceType(elemType)
		tupleVal := b.EmitMakeBuildWithType(tupleType, b.EmitConstInst(int64(len(values))), b.EmitConstInst(int64(len(values))))
		for idx, val := range values {
			idxConst := b.EmitConstInst(int64(idx))
			member := b.CreateMemberCallVariable(tupleVal, idxConst)
			b.AssignVariable(member, val)
		}
		return tupleVal
	}

	// Check for True/False via NAME tokens
	if name := raw.Name(); name != nil {
		if nameCtx, ok := name.(*pythonparser.NameContext); ok {
			if nameCtx.TRUE() != nil {
				return b.EmitConstInst(true)
			} else if nameCtx.FALSE() != nil {
				return b.EmitConstInst(false)
			}
		}
	}

	return nil
}

// VisitName visits a name node.
// This handles variable names and builtins.
func (b *singleFileBuilder) VisitName(raw *pythonparser.NameContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	name := raw.GetText()
	if name == "" {
		return nil
	}

	// Handle special names
	switch name {
	case "True":
		return b.EmitConstInst(true)
	case "False":
		return b.EmitConstInst(false)
	case "None":
		return b.EmitConstInst(nil)
	case "super":
		// Return a super proxy value; actual super() call is handled in VisitArguments
		return b.emitSuperValue()
	}

	// Try to read as constant first
	if constVal, ok := b.ReadConst(name); ok {
		return constVal
	}

	// Prefer values already visible in the current function/scope before checking
	// imports, so local variables continue to shadow imported names.
	if varVal := b.PeekValueInThisFunction(name); varVal != nil {
		return varVal
	}
	if importValue, ok := b.GetProgram().ReadImportValue(name); ok {
		return importValue
	}
	if importType, ok := b.GetProgram().ReadImportType(name); ok {
		if blueprint, ok := ssa.ToBluePrintType(importType); ok {
			return blueprint.Container()
		}
	}
	if wildcardValue := b.resolveWildcardImportName(name); wildcardValue != nil {
		return wildcardValue
	}
	// Fall back to the original full lookup path for closure/freevalue resolution,
	// externs, and undefined placeholders.
	if varVal := b.ReadValue(name); varVal != nil {
		return varVal
	}

	// Variable doesn't exist yet, emit undefined
	return b.EmitUndefined(name)
}

// emitSuperValue emits a value representing the super() proxy.
// super() in Python returns a proxy to the parent class; we model it as an undefined
// with a special tag so callers can recognize it if needed.
func (b *singleFileBuilder) emitSuperValue() ssa.Value {
	return b.EmitUndefined("super")
}

// VisitNumber visits a number node.
// This handles numeric literals.
func (b *singleFileBuilder) VisitNumber(raw *pythonparser.NumberContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Handle integer
	if integer := raw.Integer(); integer != nil {
		if intCtx, ok := integer.(*pythonparser.IntegerContext); ok {
			return b.VisitInteger(intCtx)
		}
	}

	// Handle float
	if floatToken := raw.FLOAT_NUMBER(); floatToken != nil {
		text := floatToken.GetText()
		// Parse float
		var val float64
		if _, err := fmt.Sscanf(text, "%f", &val); err == nil {
			return b.EmitConstInst(val)
		}
	}

	// Handle imaginary number
	if imagToken := raw.IMAG_NUMBER(); imagToken != nil {
		// TODO: Handle imaginary numbers
		return b.EmitConstInst(0)
	}

	return b.EmitConstInst(0)
}

// VisitInteger visits an integer node.
func (b *singleFileBuilder) VisitInteger(raw *pythonparser.IntegerContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Handle decimal integer
	if decToken := raw.DECIMAL_INTEGER(); decToken != nil {
		text := decToken.GetText()
		var val int64
		if _, err := fmt.Sscanf(text, "%d", &val); err == nil {
			return b.EmitConstInst(val)
		}
	}

	// Handle octal integer
	if octToken := raw.OCT_INTEGER(); octToken != nil {
		text := octToken.GetText()
		var val int64
		if _, err := fmt.Sscanf(text, "%o", &val); err == nil {
			return b.EmitConstInst(val)
		}
	}

	// Handle hex integer
	if hexToken := raw.HEX_INTEGER(); hexToken != nil {
		text := hexToken.GetText()
		var val int64
		if _, err := fmt.Sscanf(text, "%x", &val); err == nil {
			return b.EmitConstInst(val)
		}
	}

	// Handle binary integer
	if binToken := raw.BIN_INTEGER(); binToken != nil {
		text := binToken.GetText()
		// Remove '0b' or '0B' prefix
		if len(text) > 2 {
			text = text[2:]
		}
		var val int64
		if _, err := fmt.Sscanf(text, "%b", &val); err == nil {
			return b.EmitConstInst(val)
		}
	}

	return b.EmitConstInst(0)
}

// VisitString visits a string literal.
func (b *singleFileBuilder) VisitString(raw *pythonparser.AtomContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Get all string tokens
	strTokens := raw.AllSTRING()
	if len(strTokens) == 0 {
		return nil
	}

	// Concatenate all string tokens
	var result string
	for _, token := range strTokens {
		text := token.GetText()
		// Remove quotes from string literals
		if len(text) >= 2 {
			text = text[1 : len(text)-1]
		}
		result += text
	}

	return b.EmitConstInst(result)
}

// VisitTrailer visits a trailer node.
// trailer: DOT name arguments? | arguments
// This handles function calls and attribute access.
func (b *singleFileBuilder) VisitTrailer(raw *pythonparser.TrailerContext, obj ssa.Value) ssa.Value {
	if b == nil || raw == nil || b.IsStop() || obj == nil {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// DOT name (with optional call)
	if name := raw.Name(); name != nil {
		if nameCtx, ok := name.(*pythonparser.NameContext); ok {
			attrName := nameCtx.GetText()
			if arguments := raw.Arguments(); arguments != nil {
				if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
					if argCtx.Arglist() == nil && obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.MapTypeKind {
						switch attrName {
						case "keys":
							if keys, ok := b.extractStaticMapKeys(obj); ok {
								return b.buildSliceValueFromValues(keys)
							}
						case "values":
							if keys, ok := b.extractStaticMapKeys(obj); ok {
								values := make([]ssa.Value, 0, len(keys))
								for _, key := range keys {
									value := b.ReadMemberCallValue(obj, key)
									if value == nil {
										value = b.EmitUndefined("dict_value")
									}
									values = append(values, value)
								}
								return b.buildSliceValueFromValues(values)
							}
						case "items":
							if keys, ok := b.extractStaticMapKeys(obj); ok {
								pairs := make([]ssa.Value, 0, len(keys))
								for _, key := range keys {
									value := b.ReadMemberCallValue(obj, key)
									if value == nil {
										value = b.EmitUndefined("dict_item")
									}
									pairs = append(pairs, b.buildSliceValueFromValues([]ssa.Value{key, value}))
								}
								return b.buildSliceValueFromValues(pairs)
							}
						}
					}
					if rawString, ok := constStringValue(obj); ok {
						switch attrName {
						case "partition":
							var separator string
							if arglist := argCtx.Arglist(); arglist != nil {
								args := b.VisitArglist(arglist)
								if len(args) == 1 {
									if argString, ok := constStringValue(args[0]); ok {
										separator = argString
									}
								}
							}
							if separator != "" {
								before, after, found := strings.Cut(rawString, separator)
								if !found {
									return b.buildSliceValueFromValues([]ssa.Value{
										b.EmitConstInst(rawString),
										b.EmitConstInst(""),
										b.EmitConstInst(""),
									})
								}
								return b.buildSliceValueFromValues([]ssa.Value{
									b.EmitConstInst(before),
									b.EmitConstInst(separator),
									b.EmitConstInst(after),
								})
							}
						case "isidentifier":
							if argCtx.Arglist() == nil {
								return b.EmitConstInst(isIdentifierString(rawString))
							}
						}
					}
				}
			}
			if obj.GetType() != nil {
				switch obj.GetType().GetTypeKind() {
				case ssa.StringTypeKind, ssa.BytesTypeKind:
					syntheticName := "string." + attrName
					if obj.GetType().GetTypeKind() == ssa.BytesTypeKind {
						syntheticName = "bytes." + attrName
					}
					if arguments := raw.Arguments(); arguments != nil {
						if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
							return b.VisitArguments(argCtx, b.newDynamicPlaceholder(syntheticName))
						}
					}
					return b.newDynamicPlaceholder(syntheticName)
				}
			}
			syntheticName := attrName
			if objName := obj.GetName(); objName != "" {
				syntheticName = objName + "." + attrName
			}
			if obj.GetType() != nil {
				switch obj.GetType().GetTypeKind() {
				case ssa.SliceTypeKind, ssa.TupleTypeKind:
					if arguments := raw.Arguments(); arguments != nil {
						if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
							return b.VisitArguments(argCtx, b.newDynamicPlaceholder(syntheticName))
						}
					}
					return b.newDynamicPlaceholder(syntheticName)
				}
			}
			memberKey := b.EmitConstInst(attrName)
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.FunctionTypeKind {
				if stored := b.ReadValue(syntheticName); stored != nil {
					return b.ensureDynamicValueType(stored)
				}
				if arguments := raw.Arguments(); arguments != nil {
					if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
						return b.VisitArguments(argCtx, b.newDynamicPlaceholder(syntheticName))
					}
				}
				return b.newDynamicPlaceholder(syntheticName)
			}
			if arguments := raw.Arguments(); arguments != nil {
				if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
					if obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.ClassBluePrintTypeKind {
						if blueprint, ok := ssa.ToBluePrintType(obj.GetType()); ok && !b.hasBlueprintMemberOrMethod(blueprint, attrName) {
							return b.VisitArguments(argCtx, b.newDynamicPlaceholder(syntheticName))
						}
						b.ensureBlueprintCallableMember(obj, attrName)
						obj = b.ensureDynamicObjectType(obj)
						methodVal := b.ensureDynamicValueType(b.ReadMemberCallMethod(obj, memberKey))
						return b.VisitArguments(argCtx, methodVal)
					}
					if b.shouldUseDynamicMemberFallback(obj) {
						return b.VisitArguments(argCtx, b.newDynamicPlaceholder(syntheticName))
					}
					obj = b.ensureDynamicObjectType(obj)
					methodVal := b.ensureDynamicValueType(b.ReadMemberCallMethod(obj, memberKey))
					return b.VisitArguments(argCtx, methodVal)
				}
			}
			if obj.GetType() != nil && obj.GetType().GetTypeKind() == ssa.ClassBluePrintTypeKind {
				if blueprint, ok := ssa.ToBluePrintType(obj.GetType()); ok && !b.hasBlueprintMemberOrMethod(blueprint, attrName) {
					return b.newDynamicPlaceholder(syntheticName)
				}
				b.ensureBlueprintMember(obj, attrName)
				obj = b.ensureDynamicObjectType(obj)
				return b.ensureDynamicValueType(b.ReadMemberCallValue(obj, memberKey))
			}
			if b.shouldUseDynamicMemberFallback(obj) {
				return b.newDynamicPlaceholder(syntheticName)
			}
			obj = b.ensureDynamicObjectType(obj)
			return b.ensureDynamicValueType(b.ReadMemberCallValue(obj, memberKey))
		}
	}

	// Function call with arguments directly
	if arguments := raw.Arguments(); arguments != nil {
		if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
			return b.VisitArguments(argCtx, obj)
		}
	}

	return obj
}

// VisitArguments visits an arguments node.
// arguments: OPEN_PAREN arglist? CLOSE_PAREN | OPEN_BRACKET subscriptlist CLOSE_BRACKET
// This handles function call arguments.
func (b *singleFileBuilder) VisitArguments(raw *pythonparser.ArgumentsContext, obj ssa.Value) ssa.Value {
	if b == nil || raw == nil || b.IsStop() || obj == nil {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Subscript form: obj[...]
	if raw.OPEN_BRACKET() != nil || raw.Subscriptlist() != nil {
		if sl, ok := raw.Subscriptlist().(*pythonparser.SubscriptlistContext); ok && sl != nil {
			subs := sl.AllSubscript()
			if len(subs) > 0 {
				if sub, ok := subs[0].(*pythonparser.SubscriptContext); ok {
					if test := sub.Test(0); test != nil {
						if testCtx, ok := test.(*pythonparser.TestContext); ok {
							if idxVal, ok := b.VisitTest(testCtx).(ssa.Value); ok {
								if b.shouldUseDynamicMemberFallback(obj) {
									syntheticName := obj.GetName()
									if syntheticName == "" {
										syntheticName = "item"
									}
									return b.newDynamicPlaceholder(syntheticName + "[" + idxVal.String() + "]")
								}
								if obj.GetType() != nil {
									switch obj.GetType().GetTypeKind() {
									case ssa.SliceTypeKind, ssa.TupleTypeKind:
										if idxVal.GetType() != nil && idxVal.GetType().GetTypeKind() == ssa.StringTypeKind {
											syntheticName := obj.GetName()
											if syntheticName == "" {
												syntheticName = "item"
											}
											return b.newDynamicPlaceholder(syntheticName + "[" + idxVal.String() + "]")
										}
									}
								}
								obj = b.ensureDynamicObjectType(obj)
								return b.ensureDynamicValueType(b.ReadMemberCallValue(obj, idxVal))
							}
						}
					}
				}
			}
		}
		return obj
	}

	// Collect call arguments
	var args []ssa.Value
	if arglist := raw.Arglist(); arglist != nil {
		args = b.VisitArglist(arglist)
	}

	// hasattr(self, "__setup__") is a common Python guard before dynamic method
	// dispatch. Register the guarded member on blueprint-backed receivers so later
	// intra-project calls don't fail purely on missing compile-time class metadata.
	if obj.GetName() == "hasattr" && len(args) == 2 {
		if attrName, ok := constStringValue(args[1]); ok {
			if strings.HasPrefix(attrName, "__") && strings.HasSuffix(attrName, "__") {
				b.ensureBlueprintCallableMember(args[0], attrName)
			} else {
				b.ensureBlueprintMember(args[0], attrName)
			}
		}
	}

	// Class instantiation: prepend an Undefined $self placeholder so constructor
	// formal-parameter indices align with call-site arguments.
	// Avoid ClassConstructor to prevent spurious destructor generation (Python has no __del__).
	if blueprint, ok := ssa.ToBluePrintType(obj.GetType()); ok {
		b.ensureBlueprintConstructorSlot(blueprint)
		selfPlaceholder := b.EmitUndefined(blueprint.Name)
		selfPlaceholder.SetType(blueprint)
		callArgs := append([]ssa.Value{selfPlaceholder}, args...)
		return b.ClassConstructorWithoutDeferDestructor(blueprint, callArgs)
	}

	for index := range args {
		args[index] = b.normalizePythonCallArgument(args[index])
	}

	// Regular function or method call
	call := b.NewCall(obj, args)
	return b.ensureDynamicValueType(b.EmitCall(call))
}

// VisitArglist visits an arglist node.
// arglist: argument (COMMA argument)* COMMA?
func (b *singleFileBuilder) VisitArglist(raw pythonparser.IArglistContext) []ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	arglist, ok := raw.(*pythonparser.ArglistContext)
	if !ok || arglist == nil {
		return nil
	}

	var args []ssa.Value
	for _, argument := range arglist.AllArgument() {
		if argCtx, ok := argument.(*pythonparser.ArgumentContext); ok {
			if argValue := b.VisitArgument(argCtx); argValue != nil {
				if v, ok := argValue.(ssa.Value); ok {
					args = append(args, v)
				}
			}
		}
	}

	return args
}

// VisitArgument visits an argument node.
// argument: test (comp_for | ASSIGN test)? | (POWER | STAR) test
func (b *singleFileBuilder) VisitArgument(raw *pythonparser.ArgumentContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

	// Handle keyword argument: name=value
	// Test(0) is the key (identifier), Test(1) is the value expression.
	// For SSA dataflow purposes we care about the value flowing into the function,
	// not the key name, so we visit and return the value (Test(1)).
	if raw.ASSIGN() != nil {
		tests := raw.AllTest()
		if len(tests) >= 2 {
			if valCtx, ok := tests[1].(*pythonparser.TestContext); ok {
				return b.VisitTest(valCtx)
			}
		}
		return nil
	}

	// Handle positional argument: test
	if test := raw.Test(0); test != nil {
		if testCtx, ok := test.(*pythonparser.TestContext); ok {
			return b.VisitTest(testCtx)
		}
	}

	// Handle *args or **kwargs: (POWER | STAR) test
	if star := raw.STAR(); star != nil {
		// *args
		if test := raw.Test(0); test != nil {
			if testCtx, ok := test.(*pythonparser.TestContext); ok {
				return b.VisitTest(testCtx)
			}
		}
	} else if power := raw.POWER(); power != nil {
		// **kwargs
		if test := raw.Test(0); test != nil {
			if testCtx, ok := test.(*pythonparser.TestContext); ok {
				return b.VisitTest(testCtx)
			}
		}
	}

	return nil
}
