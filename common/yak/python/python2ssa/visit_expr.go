package python2ssa

import (
	"fmt"
	"strings"

	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// VisitTest visits a test node.
// This handles expressions.
func (b *singleFileBuilder) VisitTest(raw *pythonparser.TestContext) interface{} {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.SetRange(raw)
	defer recoverRange()

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
		elemValues := make([]ssa.Value, 0)
		if testlist := raw.Testlist_comp(); testlist != nil {
			if tlc, ok := testlist.(*pythonparser.Testlist_compContext); ok {
				for _, t := range tlc.AllTest() {
					if tc, ok := t.(*pythonparser.TestContext); ok {
						if val, ok := b.VisitTest(tc).(ssa.Value); ok {
							elemValues = append(elemValues, val)
						}
					}
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
	} else if raw.OPEN_BRACE() != nil && raw.CLOSE_BRACE() != nil {
		// dict literal: {"a":1}
		if dsm := raw.Dictorsetmaker(); dsm != nil {
			if dsCtx, ok := dsm.(*pythonparser.DictorsetmakerContext); ok {
				tests := dsCtx.AllTest()
				if len(tests) >= 2 && strings.Contains(dsCtx.GetText(), ":") {
					mapType := ssa.NewMapType(ssa.CreateStringType(), ssa.CreateAnyType())
					dict := b.EmitMakeBuildWithType(mapType, nil, nil)
					for i := 0; i+1 < len(tests); i += 2 {
						keyTest, valTest := tests[i], tests[i+1]
						keyValRaw := b.VisitTest(keyTest.(*pythonparser.TestContext))
						valValRaw := b.VisitTest(valTest.(*pythonparser.TestContext))
						keyVal, kok := keyValRaw.(ssa.Value)
						valVal, vok := valValRaw.(ssa.Value)
						if !kok || !vok {
							continue
						}
						member := b.CreateMemberCallVariable(dict, keyVal)
						b.AssignVariable(member, valVal)
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
	}

	// Try to read as constant first
	if constVal, ok := b.ReadConst(name); ok {
		return constVal
	}

	// Try to read as variable or builtin function
	// For builtins like println, this will return an extern function
	if varVal := b.ReadValue(name); varVal != nil {
		return varVal
	}

	// Variable doesn't exist yet, emit undefined
	return b.EmitUndefined(name)
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
			memberKey := b.EmitConstInst(attrName)
			methodVal := b.ReadMemberCallMethod(obj, memberKey)
			if arguments := raw.Arguments(); arguments != nil {
				if argCtx, ok := arguments.(*pythonparser.ArgumentsContext); ok {
					return b.VisitArguments(argCtx, methodVal)
				}
			}
			return methodVal
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
								return b.ReadMemberCallValue(obj, idxVal)
							}
						}
					}
				}
			}
		}
		return obj
	}

	// Handle function or method call
	if arglist := raw.Arglist(); arglist != nil {
		args := b.VisitArglist(arglist)
		call := b.NewCall(obj, args)
		return b.EmitCall(call)
	} else {
		// Function call with no arguments
		call := b.NewCall(obj, []ssa.Value{})
		return b.EmitCall(call)
	}
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

	// Handle keyword argument: test ASSIGN test
	if raw.ASSIGN() != nil {
		// TODO: Handle keyword arguments
		// For now, just return the value
		if test := raw.Test(0); test != nil {
			if testCtx, ok := test.(*pythonparser.TestContext); ok {
				return b.VisitTest(testCtx)
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
