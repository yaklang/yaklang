package values

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type NewExpression struct {
	types.JavaType
	Length          []JavaValue
	ArgumentsGetter func() string
	Initializer     []JavaValue
}

// ReplaceVar implements JavaValue.
func (n *NewExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	for _, length := range n.Length {
		length.ReplaceVar(oldId, newId)
	}
	for _, initializer := range n.Initializer {
		initializer.ReplaceVar(oldId, newId)
	}
}

func NewNewArrayExpression(typ types.JavaType, length ...JavaValue) *NewExpression {
	return &NewExpression{
		JavaType: typ,
		Length:   length,
	}
}

// coerceInitializerLiteral renders an array-initializer element with the array's
// element type when that yields more faithful source. Today this only matters for
// boolean element types: a boolean[] initializer is filled by iconst_0/iconst_1,
// whose values carry an int type, so they must be rendered as false/true.
func coerceInitializerLiteral(v JavaValue, elemType types.JavaType, funcCtx *class_context.ClassContext) string {
	if lit, ok := v.(*JavaLiteral); ok {
		if elemType.String(funcCtx) == types.NewJavaPrimer(types.JavaBoolean).String(funcCtx) {
			if n, ok := lit.Data.(int); ok {
				if n == 0 {
					return "false"
				}
				return "true"
			}
		}
	}
	return v.String(funcCtx)
}

func NewNewExpression(typ types.JavaType) *NewExpression {
	return &NewExpression{
		JavaType: typ,
	}
}
func (n *NewExpression) Type() types.JavaType {
	return n.JavaType
}

func (n *NewExpression) String(funcCtx *class_context.ClassContext) string {
	if n.IsArray() {
		base := n.JavaType
		for base.IsArray() {
			base = base.ElementType()
		}
		s := fmt.Sprintf("new %s", base.String(funcCtx))
		// An explicit initializer (new T[]{...}) is incompatible with a sized dimension
		// (new T[3]{...} is a javac error); the literal supplies the length, so drop the
		// first numeric dimension and emit empty brackets per array dimension instead.
		if len(n.Initializer) != 0 {
			for i := 0; i < n.JavaType.ArrayDim(); i++ {
				s += "[]"
			}
			vsStr := []string{}
			for _, v := range n.Initializer {
				// Coerce int 0/1 literals to boolean false/true when the array element type is
				// boolean: iconst_0/iconst_1 fill a boolean[] but carry an int type, so without
				// this coercion the initializer renders `new boolean[]{1,1,1,1}`, which javac
				// rejects ("int cannot be converted to boolean").
				vsStr = append(vsStr, coerceInitializerLiteral(v, base, funcCtx))
			}
			s += fmt.Sprintf("{%s}", strings.Join(vsStr, ","))
			return s
		}
		for _, l := range n.Length {
			s += fmt.Sprintf("[%v]", l.(JavaValue).String(funcCtx))
		}
		for i := len(n.Length); i < n.JavaType.ArrayDim(); i++ {
			s += "[]"
		}
		return s
	}
	var args string
	if n.ArgumentsGetter != nil {
		args = n.ArgumentsGetter()
	}
	return fmt.Sprintf("new %s(%s)", n.JavaType.String(funcCtx), args)
}

type JavaExpression struct {
	Values []JavaValue
	Op     string
	Typ    types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *JavaExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	for _, value := range j.Values {
		value.ReplaceVar(oldId, newId)
	}
}

func (j *JavaExpression) Type() types.JavaType {
	// A non-short-circuit boolean connective (& | ^) of two boolean operands is boolean-typed even
	// when its operands reach it as int-shaped `cond ? 1 : 0` ternaries (built by a later CFG pass,
	// after NewBinaryExpression already fixed j.Typ to int). Reporting boolean here lets a boolean
	// context (return/assign/condition) accept it; see String() for the matching rendering.
	if _, _, ok := j.boolConnectiveConds(); ok {
		return types.NewJavaPrimer(types.JavaBoolean)
	}
	return j.Typ
}

// boolConnectiveConds reports whether this expression is `a & b`, `a | b` or `a ^ b` where BOTH
// operands are boolean (either already boolean-typed, or the int `cond ? 1 : 0` shape javac emits
// for a comparison feeding an integer bitwise op). It returns the two underlying boolean conditions.
// This recovers the original `cond1 & cond2` boolean connective instead of the int-typed
// `(c1?1:0) & (c2?1:0)`, which fails to compile where a boolean is required.
func (j *JavaExpression) boolConnectiveConds() (JavaValue, JavaValue, bool) {
	if len(j.Values) != 2 || (j.Op != AND && j.Op != OR && j.Op != XOR) {
		return nil, nil, false
	}
	c1, ok1 := boolOperandCondition(j.Values[0])
	c2, ok2 := boolOperandCondition(j.Values[1])
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	return c1, c2, true
}

// boolOperandCondition returns the boolean condition underlying a `cond ? 1 : 0` ternary, or the
// value itself when it is already boolean-typed (a comparison or a nested boolean connective).
func boolOperandCondition(v JavaValue) (JavaValue, bool) {
	u := UnpackSoltValue(v)
	if cond, ok := BoolTernaryCondition(u); ok {
		return cond, true
	}
	if isBooleanTyped(u) {
		return u, true
	}
	return nil, false
}

func (j *JavaExpression) String(funcCtx *class_context.ClassContext) string {
	if c1, c2, ok := j.boolConnectiveConds(); ok {
		return fmt.Sprintf("(%s) %s (%s)",
			SimplifyConditionValue(c1).String(funcCtx), j.Op, SimplifyConditionValue(c2).String(funcCtx))
	}
	vs := []string{}
	for _, value := range j.Values {
		vs = append(vs, value.String(funcCtx))
	}
	if len(vs) == 1 {
		return fmt.Sprintf("%s(%s)", j.Op, vs[0])
	}
	switch j.Op {
	case ADD:
		return fmt.Sprintf("(%s) + (%s)", vs[0], vs[1])
	case INC:
		return fmt.Sprintf("%s++", vs[0])
	case DEC:
		return fmt.Sprintf("%s--", vs[0])
	case GT, SUB:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	default:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	}
}

// UnaryMinusOperand renders v as the operand of a leading unary minus, wrapping it in parentheses
// whenever the bare "-"+v form would re-associate or merge tokens. The JVM emits `... ; ineg` for a
// negated sub-expression, so an arithmetic `-(a + b)` arrives as Neg(Add(a,b)); rendering it as
// "-" + "(a) + (b)" silently re-parses as "(-a) + b" (wrong value). It also guards "-" + "-x" /
// "-" + "+x" from fusing into the predecrement/increment tokens "--"/"-+". Simple operands
// (refs, literals, fields, calls, array loads) are left unparenthesised to keep output readable.
func UnaryMinusOperand(v JavaValue, funcCtx *class_context.ClassContext) string {
	s := v.String(funcCtx)
	needParen := false
	switch uv := UnpackSoltValue(v).(type) {
	case *JavaExpression:
		// A binary expression (two operands) binds looser than unary minus and must be wrapped.
		if len(uv.Values) >= 2 {
			needParen = true
		}
	case *TernaryExpression:
		needParen = true
	}
	if !needParen && (strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+")) {
		needParen = true
	}
	if needParen {
		return "(" + s + ")"
	}
	return s
}

// primerRawType returns the *types.JavaPrimer raw type of t, guarding against a nil JavaType
// (which incomplete stack simulation can produce) so callers never nil-dereference RawType().
func primerRawType(t types.JavaType) (*types.JavaPrimer, bool) {
	if t == nil {
		return nil, false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	return p, ok
}

func isBooleanTyped(v JavaValue) bool {
	if v == nil {
		return false
	}
	uv := UnpackSoltValue(v)
	if uv == nil {
		return false
	}
	t := uv.Type()
	if t == nil {
		return false
	}
	prim, ok := t.RawType().(*types.JavaPrimer)
	return ok && prim.Name == types.JavaBoolean
}

// resetTypeSafe resets v's type to t, but only when v already carries a non-nil JavaType.
// Incomplete stack simulation can leave a value with a nil Type(); skipping the reset there
// avoids a nil-dereference while leaving correctly-typed values unchanged.
func resetTypeSafe(v JavaValue, t types.JavaType) {
	if v == nil {
		return
	}
	if vt := v.Type(); vt != nil {
		vt.ResetType(t)
	}
}

// nonNilType returns the first non-nil candidate, falling back to int. Expression constructors use
// it so a nil result type (which incomplete stack simulation can yield for an operand) degrades to
// a sensible default instead of panicking at typ.Copy().
func nonNilType(candidates ...types.JavaType) types.JavaType {
	for _, c := range candidates {
		if c != nil {
			return c
		}
	}
	return types.NewJavaPrimer(types.JavaInteger)
}

func NewUnaryExpression(value1 JavaValue, op string, typ types.JavaType) *JavaExpression {
	if IsStrictBooleanOperator(op) {
		resetTypeSafe(value1, types.NewJavaPrimer(types.JavaBoolean))
	}
	return &JavaExpression{
		Values: []JavaValue{value1},
		Op:     op,
		Typ:    nonNilType(typ, value1.Type()).Copy(),
	}
}
func NewBinaryExpression(value1, value2 JavaValue, op string, typ types.JavaType) *JavaExpression {
	if IsStrictBooleanOperator(op) {
		resetTypeSafe(value1, types.NewJavaPrimer(types.JavaBoolean))
		resetTypeSafe(value2, types.NewJavaPrimer(types.JavaBoolean))
	} else if (op == AND || op == OR || op == XOR) && (isBooleanTyped(value1) || isBooleanTyped(value2)) {
		// &, |, ^ are shared between boolean logic and integer bitwise arithmetic. Decide by
		// the operands: if either side is already boolean (e.g. descriptor-typed parameters or
		// a negation), this is boolean logic, so align both sides to boolean. Otherwise leave
		// the operands as their inferred integer type.
		resetTypeSafe(value1, types.NewJavaPrimer(types.JavaBoolean))
		resetTypeSafe(value2, types.NewJavaPrimer(types.JavaBoolean))
		typ = types.NewJavaPrimer(types.JavaBoolean)
	}
	resultType := nonNilType(typ, value1.Type(), value2.Type()).Copy()
	resultType = promoteBinaryNumericResult(op, resultType)
	return &JavaExpression{
		Values: []JavaValue{value1, value2},
		Op:     op,
		Typ:    resultType,
	}
}

// promoteBinaryNumericResult applies JLS 5.6.2 binary numeric promotion to the result type of an
// arithmetic/bitwise/shift binary operator: when both operands are in the int computational category
// (byte/char/short/int) the operation is evaluated in int and yields int — never the narrower operand
// type. The bytecode confirms this (iadd/iand/ishl/... consume and produce the int stack category;
// there is NO byte/short/char arithmetic opcode), so reporting a narrow operand type for the result
// disagrees with javac. That disagreement defeated both the slot-merge in AssignVar (a `byte + 256`
// reassign looked byte-typed, matched the slot type, and never widened the slot to int) and the
// store/return narrowing-cast rendering (which keys off an int-typed value), producing uncompilable
// "possible lossy conversion from int to byte" stores (commons-codec Base32/Base64
// `byte b = in[i++]; if (b < 0) b = b + 256;`). The narrowing cast is reintroduced precisely at the
// byte/short/char-typed use sites (declaration/return/arg/array-store). Only a byte/char/short result
// is promoted: int stays int and long/float/double already carry the wider category via `typ`. The
// boolean &|^ case (handled just above) and comparison operators are left untouched. Kill-switch:
// JDEC_NO_BINNUM_PROMOTE=1.
func promoteBinaryNumericResult(op string, resultType types.JavaType) types.JavaType {
	switch op {
	case ADD, SUB, MUL, DIV, REM, AND, OR, XOR, SHL, SHR, USHR:
	default:
		return resultType
	}
	if os.Getenv("JDEC_NO_BINNUM_PROMOTE") != "" {
		return resultType
	}
	p, ok := resultType.RawType().(*types.JavaPrimer)
	if !ok {
		return resultType
	}
	switch p.Name {
	case types.JavaByte, types.JavaChar, types.JavaShort:
		return types.NewJavaPrimer(types.JavaInteger)
	}
	return resultType
}

type FunctionCallExpression struct {
	IsStatic     bool
	Object       JavaValue
	FunctionName string
	ClassName    string
	Arguments    []JavaValue
	FuncType     *types.JavaFuncType
	// IsSpecialInvoke marks a call decoded from invokespecial. For a non-constructor invokespecial
	// whose receiver is `this` and whose target class is NOT the current class, this is a `super.m()`
	// call (the only other invokespecial forms are constructors and private same-class calls). It must
	// render as `super.m()`, never `this.m()` -- the latter re-dispatches virtually to the overriding
	// method and recurses infinitely (e.g. guava CaseFormat constant-body `convert` -> StackOverflow).
	IsSpecialInvoke bool
}

// ReplaceVar implements JavaValue.
func (f *FunctionCallExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if f.Object != nil {
		f.Object.ReplaceVar(oldId, newId)
	}
	for _, arg := range f.Arguments {
		arg.ReplaceVar(oldId, newId)
	}
}

func (f *FunctionCallExpression) Type() types.JavaType {
	return f.FuncType.ReturnType
}

// isCurrentClass reports whether the call's target class is the class currently being rendered.
// Used to tell a private same-class invokespecial (`this.m()`) from a super call (`super.m()`).
func (f *FunctionCallExpression) isCurrentClass(funcCtx *class_context.ClassContext) bool {
	return funcCtx != nil && f.ClassName == funcCtx.ClassName
}

func (f *FunctionCallExpression) IsSupperConstructorInvoke(funcCtx *class_context.ClassContext) bool {
	if f.FunctionName == "<init>" && f.ClassName == funcCtx.SupperClassName {
		return true
	}
	return false
}
func (f *FunctionCallExpression) ArgumentString(funcCtx *class_context.ClassContext) string {
	return strings.Join(f.ArgumentStrings(funcCtx), ",")
}

func (f *FunctionCallExpression) ArgumentStrings(funcCtx *class_context.ClassContext) []string {
	paramStrs := []string{}
	for i, arg := range f.Arguments {
		argType := f.FuncType.ParamTypes[i]
		// Incomplete stack simulation can leave an argument with a nil Type(); a parameter type
		// can likewise be nil for a malformed descriptor. Guard each RawType() behind a nil check
		// so a missing type degrades the per-argument cast logic to a no-op (rendering the argument
		// as-is) instead of nil-dereferencing and panicking the whole method into a stub.
		var expectClassType *types.JavaClass
		var atcClassType *types.JavaClass
		var ok1, ok2 bool
		if argType != nil {
			expectClassType, ok1 = argType.RawType().(*types.JavaClass)
		}
		if at := arg.Type(); at != nil {
			atcClassType, ok2 = at.RawType().(*types.JavaClass)
		}
		if ok1 && ok2 && expectClassType.Name != atcClassType.Name {
			if expectClassType.Name != "java.lang.Object" {
				argStr := arg.String(funcCtx)
				argTypeStr := argType.String(funcCtx)
				arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
					return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
				}, func() types.JavaType {
					return argType
				})
			}
		} else if expectPrim, okp := primerRawType(argType); okp {
			if expectPrim.Name == types.JavaBoolean {
				// The JVM has no boolean opcodes: a boolean argument is pushed as an int
				// constant (iconst_0/iconst_1). Java forbids int->boolean conversion, so values
				// flowing into a boolean parameter must render with boolean literals, including
				// ternary trees like `cond ? 1 : 0`.
				arg = coerceBooleanArgument(arg)
			} else if actualPrim, oka := primerRawType(arg.Type()); oka &&
				actualPrim.Name != types.JavaBoolean && actualPrim.Name != expectPrim.Name {
				// The JVM descriptor pins the EXACT primitive parameter type, but byte/short/char/int
				// all share the int stack category and convert between each other without an opcode, so
				// the argument's static type frequently disagrees with the parameter type. Two failure
				// modes follow if the cast is dropped:
				//   - narrowing (int -> byte/short/char): illegal in invocation context (JLS 5.3),
				//     "possible lossy conversion from int to char";
				//   - widening that changes overloading (char/byte/short -> int): source picks a
				//     DIFFERENT overload than the bytecode (e.g. StringBuilder.append(char) instead of
				//     append(int)), silently changing behavior.
				// Emitting an explicit cast to the descriptor's parameter type reproduces the original
				// invocation exactly. (long/float/double mismatches already carry an i2l/i2d/... opcode
				// that makes the argument type match, so this fires only for the int-category gap.)
				argStr := arg.String(funcCtx)
				argTypeStr := expectPrim.Name
				arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
					return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
				}, func() types.JavaType {
					return argType
				})
			}
		}
		argStr := arg.String(funcCtx)
		if argStr == "" {
			if ref, ok := arg.(*JavaRef); ok && ref != nil && ref.Id != nil {
				argStr = ref.Id.String()
			}
		}
		paramStrs = append(paramStrs, argStr)
	}
	return paramStrs
}

func (f *FunctionCallExpression) String(funcCtx *class_context.ClassContext) string {
	paramStrs := f.ArgumentStrings(funcCtx)
	if f.FunctionName == "<init>" {
		if f.ClassName == funcCtx.ClassName {
			return fmt.Sprintf("%s(%s)", f.Object.String(funcCtx), strings.Join(paramStrs, ","))
		} else if f.ClassName == funcCtx.SupperClassName {
			return fmt.Sprintf("super(%s)", strings.Join(paramStrs, ","))
		}
	}
	functionName := class_context.SafeIdentifier(f.FunctionName)

	// A non-constructor invokespecial whose receiver is `this` and whose target is a DIFFERENT class
	// (the superclass / an ancestor, never the current class which would be a private same-class call)
	// is a `super.method(...)` call. Rendering it as `this.method(...)` re-dispatches virtually to the
	// overriding method and recurses forever (guava CaseFormat constant bodies' `super.convert(...)`).
	if f.IsSpecialInvoke && f.FunctionName != "<init>" && f.ClassName != "" && !f.isCurrentClass(funcCtx) {
		if ref, ok := UnpackSoltValue(f.Object).(*JavaRef); ok && ref != nil && ref.IsThis {
			return fmt.Sprintf("super.%s(%s)", functionName, strings.Join(paramStrs, ","))
		}
	}

	if v, ok := f.Object.(*JavaClassValue); ok {
		if classType, ok2 := v.Type().RawType().(*types.JavaClass); ok2 && classType.Name == funcCtx.ClassName && f.IsStatic {
			// Unqualified static call to a method of the current class (foo() instead of Foo.foo()).
			// Only valid for static dispatch; an instance call on the current class's own class
			// literal (Foo.class.getName()) must keep the `Foo.class` receiver, so it falls through.
			return fmt.Sprintf("%s(%s)", functionName, strings.Join(paramStrs, ","))
		}
		if f.IsStatic {
			// Static method invocation: the receiver is a type reference, so render the bare type
			// name (Integer.parseInt(...)). JavaClassValue.String() now yields the Class-object
			// literal form `Integer.class`, which is correct for value/instance-receiver positions
			// but wrong here, so bypass it via Type().
			return fmt.Sprintf("%s.%s(%s)", v.Type().String(funcCtx), functionName, strings.Join(paramStrs, ","))
		}
	}
	obj := UnpackSoltValue(f.Object)
	if cv, ok := obj.(*CustomValue); ok && cv.Flag == "lambda" {
		// A lambda / method reference inlined directly as a call receiver has no target type of
		// its own - `(() -> x).get()` does not compile. Supply one by casting to the functional
		// interface the value carries: `((Supplier)(() -> x)).get()`.
		return fmt.Sprintf("((%s)(%s)).%s(%s)", cv.Type().String(funcCtx), cv.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	}
	switch obj.(type) {
	case *JavaExpression, *TernaryExpression, *SlotValue:
		return fmt.Sprintf("(%s).%s(%s)", f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	default:
		return fmt.Sprintf("%s.%s(%s)", f.Object.String(funcCtx), functionName, strings.Join(paramStrs, ","))
	}
}

func coerceBooleanArgument(arg JavaValue) JavaValue {
	switch v := UnpackSoltValue(arg).(type) {
	case *JavaLiteral:
		if prim, ok := primerRawType(v.Type()); ok && prim.Name == types.JavaInteger {
			if iv, ok := v.Data.(int); ok && (iv == 0 || iv == 1) {
				return NewJavaLiteral(iv, types.NewJavaPrimer(types.JavaBoolean))
			}
		}
		return arg
	case *TernaryExpression:
		if v == nil {
			return arg
		}
		coerced := NewTernaryExpression(v.Condition, coerceBooleanArgument(v.TrueValue), coerceBooleanArgument(v.FalseValue))
		coerced.ConditionFromOp = v.ConditionFromOp
		return coerced
	}
	// Any OTHER int-typed value reaching a boolean parameter is a boolean held as an int (the JVM has
	// no boolean storage: a boolean local is stored/reloaded with istore/iload, and javac materializes
	// a boolean value via iconst_0/iconst_1). Java forbids the implicit int->boolean conversion, so a
	// plain `int` local/expression flowing into a boolean parameter fails to recompile ("incompatible
	// types: int cannot be converted to boolean"). Render an explicit `(v) != (0)`, which is the exact
	// boolean meaning of the 0/1 int. Values already typed boolean (comparisons, predicate calls,
	// boolean refs) keep their boolean type, so they are left untouched and we never emit an illegal
	// `(a > b) != (0)`.
	if at := arg.Type(); at != nil {
		if prim, ok := primerRawType(at); ok && prim.Name == types.JavaInteger {
			inner := arg
			return NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("(%s) != (0)", inner.String(funcCtx))
			}, func() types.JavaType {
				return types.NewJavaPrimer(types.JavaBoolean)
			})
		}
	}
	return arg
}

func NewFunctionCallExpression(object JavaValue, methodMember *JavaClassMember, funcType *types.JavaFuncType) *FunctionCallExpression {
	return &FunctionCallExpression{
		FuncType:     funcType,
		Object:       object,
		FunctionName: methodMember.Member,
		ClassName:    methodMember.Name,
	}
}
