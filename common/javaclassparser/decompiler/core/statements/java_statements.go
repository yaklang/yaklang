package statements

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type ConditionStatement struct {
	Condition values.JavaValue
	Neg       bool
	Callback  func(values.JavaValue)
	// TernaryChainArm mirrors OpCode.TernaryChainArm: this condition supplies a DISTINCT nested
	// ternary arm and therefore must not be folded into a short-circuit &&/|| by MergeIf.
	TernaryChainArm bool
}

// ReplaceVar implements Statement.
func (r *ConditionStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	r.Condition.ReplaceVar(oldId, newId)
}

func (r *ConditionStatement) String(funcCtx *class_context.ClassContext) string {
	// A ConditionStatement is an intermediate structuring placeholder that should be consumed by
	// IfRewriter before reaching output. If one leaks (a structuring gap at a complex merge point),
	// rendering it as bare `if cond` (no brackets, no body) produces INVALID Java that fails syntax
	// validation and stubs the ENTIRE method. As a last-resort safety net, render it as a valid
	// `if (cond){}` — syntactically parseable Java with an empty body — so the method degrades
	// gracefully (one branch is empty) instead of being fully stubbed.
	return fmt.Sprintf("if (%s){}", r.Condition.String(funcCtx))
}

// isBoolPrimer reports whether v carries a non-nil boolean primitive type. It guards against the
// nil Type() that incomplete stack simulation can produce, so the boolean-comparison folding below
// never nil-dereferences (which would panic the whole method into a stub).
func isBoolPrimer(v values.JavaValue) bool {
	if v == nil {
		return false
	}
	t := v.Type()
	if t == nil {
		return false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	return ok && p.Name == types.JavaBoolean
}

func NewConditionStatement(cmp values.JavaValue, op string) *ConditionStatement {
	if t := cmp.Type(); t != nil {
		t.ResetType(types.NewJavaPrimer(types.JavaBoolean))
	}
	if v, ok := cmp.(*values.JavaCompare); ok {
		if op == values.NEQ {
			if literal, ok := v.JavaValue2.(*values.JavaLiteral); ok {
				if isBoolPrimer(v.JavaValue1) {
					if literal.Data == 0 {
						return &ConditionStatement{
							Condition: v.JavaValue1,
						}
					}
					if literal.Data == 1 {
						return &ConditionStatement{
							Condition: values.NewUnaryExpression(v.JavaValue1, values.Not, types.NewJavaPrimer(types.JavaBoolean)),
						}
					}
				}
			}
		}
		if op == values.EQ {
			if literal, ok := v.JavaValue2.(*values.JavaLiteral); ok {
				if isBoolPrimer(v.JavaValue1) {
					if literal.Data == 0 {
						return &ConditionStatement{
							Condition: values.NewUnaryExpression(v.JavaValue1, values.Not, types.NewJavaPrimer(types.JavaBoolean)),
						}
					}
					if literal.Data == 1 {
						return &ConditionStatement{
							Condition: v.JavaValue1,
						}
					}
				}
			}
		}
		return &ConditionStatement{
			Condition: values.NewBinaryExpression(v.JavaValue1, v.JavaValue2, op, types.NewJavaPrimer(types.JavaBoolean)),
		}
	} else {
		return &ConditionStatement{
			Condition: cmp,
		}
	}
}

type ReturnStatement struct {
	JavaValue values.JavaValue
}

// ReplaceVar implements Statement.
func (r *ReturnStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if r.JavaValue != nil {
		r.JavaValue.ReplaceVar(oldId, newId)
	}
}

func (r *ReturnStatement) String(funcCtx *class_context.ClassContext) string {
	if r.JavaValue == nil {
		return "return"
	}
	expr := r.JavaValue.String(funcCtx)
	// Narrowing cast for char/byte/short return types: bytecode stores char/byte/short
	// literals as ints (bipush/sipush/iconst), so a method returning char whose body
	// returns `cond ? 102 : 101` renders int literals that javac rejects ("possible
	// lossy conversion from int to char"). When the declared return type is narrower than
	// int and the returned value is int-typed, wrap it in an explicit cast. This is a
	// pure rendering fix — the recompiled bytecode is behaviorally identical.
	if cast := narrowingReturnCast(funcCtx, r.JavaValue); cast != "" {
		return fmt.Sprintf("return (%s) (%s)", cast, expr)
	}
	// Type-variable return: when the method's recovered return type is a class-scope type
	// variable (e.g. T/K/V) but the returned value's static type is the erased bound/Object,
	// emit an explicit unchecked cast so the source recompiles. `return null` needs no cast
	// (null is assignable to any type variable).
	if expr != "null" {
		if cast := typeVarReturnCast(funcCtx, r.JavaValue); cast != "" {
			// When the cast target carries a NESTED parameterized argument (e.g.
			// `Function<Supplier<T>, T>`) and the returned value is a DIFFERENT class cast to that
			// interface (e.g. an enum singleton `SupplierFunctionImpl.INSTANCE` that concretely
			// implements `Function<Supplier<Object>, Object>`), the direct cast is rejected as
			// inconvertible: javac proves `Supplier<Object>` distinct from `Supplier<T>`. Guava's own
			// source casts via the directly-implemented intermediate interface; the byte-faithful
			// decompiler analogue is a raw-erasure bridge cast `(Target<..>) (RawTarget) (value)`,
			// which is legal (downcast to the raw supertype, then an unchecked widen). Bare type-arg
			// targets (`Predicate<T>`, `Converter<T, T>`) keep the single direct cast.
			if bridge := nestedGenericRawBridge(funcCtx, r.JavaValue, cast); bridge != "" {
				return fmt.Sprintf("return (%s) (%s) (%s)", cast, bridge, expr)
			}
			return fmt.Sprintf("return (%s) (%s)", cast, expr)
		}
	}
	return fmt.Sprintf("return %s", expr)
}

// nestedGenericRawBridge returns the raw-erasure type name to interpose as an intermediate cast
// (`(retStr) (bridge) (value)`) when a type-variable return cast to a target with NESTED generic
// arguments would otherwise be rejected as inconvertible. It triggers only when (1) retStr has a
// nested parameterization (more than one `<`, e.g. `Function<Supplier<T>, T>`) AND (2) the returned
// value's erased type differs from retStr's erasure (the value is a different class being cast to a
// parameterized supertype). Otherwise it returns "" and the single direct cast is used.
func nestedGenericRawBridge(funcCtx *class_context.ClassContext, v values.JavaValue, retStr string) string {
	if strings.Count(retStr, "<") <= 1 {
		return ""
	}
	if v == nil {
		return ""
	}
	vt := v.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	targetErasure := erasureName(retStr)
	if targetErasure == "" || erasureName(raw.String(funcCtx)) == targetErasure {
		return ""
	}
	return targetErasure
}

// typeVarReturnCast returns the type-variable name to cast the returned value to when the
// enclosing method's declared return type is a class-scope type variable (recovered from the
// method Signature, e.g. `()TT;`) but the returned value's static type is a DIFFERENT reference
// type (the erased bound, typically Object or the declared bound such as Comparable). Bytecode
// erases a type-variable return to its bound, so a local typed as that bound, when returned from
// a method whose return type Yak has correctly recovered to `T`, fails to compile ("incompatible
// types: Comparable cannot be converted to T"). An explicit `(T)` cast is an unchecked but
// behavior-preserving rendering fix, matching what CFR/Fernflower emit. Returns "" when the
// return type is not a type variable, the value is a primitive, or the value already renders as
// that type variable. Kill-switch JDEC_TYPEVAR_RET_CAST_OFF disables it.
func typeVarReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	if funcCtx == nil || v == nil {
		return ""
	}
	if os.Getenv("JDEC_TYPEVAR_RET_CAST_OFF") != "" {
		return ""
	}
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(funcCtx)
	// Two distinct triggers:
	//   - bareTypeVar: the return type IS a type variable (T). Bytecode erases a type-variable return
	//     to its bound (Object/Comparable), so a value typed as that bound fails "Comparable cannot be
	//     converted to T"; an unchecked (T) cast fixes it. This is the original behavior.
	//   - a generic return type that MENTIONS a type variable (Converter<T, T>, Map<K, V>). Here a cast
	//     is needed ONLY when the returned value is itself PARAMETERIZED with arguments that differ
	//     from the recovered return type (e.g. `Converter.identity()` returns `IdentityConverter<?>` -
	//     captured to IdentityConverter<CAP> - into `Converter<T, T>`, a hard "cannot convert" error).
	//     A RAW or bare value (`new InnerNode()`, `this.putExisting(...)` whose call type is erased to
	//     the raw class) converts to the parameterized return type by UNCHECKED conversion and needs no
	//     cast; adding one there is gratuitous and was over-casting plain raw-subtype returns.
	bareTypeVar := funcCtx.IsTypeParam(retStr)
	if !bareTypeVar && !mentionsTypeParam(retStr, funcCtx.TypeParams) {
		return ""
	}
	vt := v.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	// A type variable is always a reference type; a primitive value never needs (and cannot take)
	// this cast.
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	if raw.String(funcCtx) == retStr {
		return ""
	}
	if !bareTypeVar {
		// Parameterized return type (Converter<T, T>): only certain returned-value forms carry a
		// RECOMPILED static type that CONFLICTS with the recovered return type and thus need a cast:
		//   - a field-access singleton (`IdentityConverter<?> INSTANCE` captures to
		//     IdentityConverter<CAP>, not assignable to Converter<T, T>);
		//   - `return this` when the declared return type is a DIFFERENT (super)type parameterized
		//     with a type variable -- e.g. enum `ObjectPredicate implements Predicate<Object>` whose
		//     `<T> Predicate<T> withNarrowedType()` does `return this`: `this` is Predicate<Object>,
		//     not Predicate<T>, so guava itself writes the unchecked `(Predicate<T>) this` cast.
		// Every other form is safe without a cast: a method call recompiles to its own recovered
		// generic signature, `return this` whose return type is the class's OWN type (InnerNode<K,V>
		// in class InnerNode) converts by unchecked conversion, and `new Raw(...)` / bare values
		// convert by unchecked conversion. Casting those only adds noise (the InnerNode over-cast
		// regression), so they are excluded.
		switch uv := values.UnpackSoltValue(v).(type) {
		case *values.JavaClassMember, *values.RefMember:
			// field access (static `Class.INSTANCE` or instance `this.field`) - may need the cast
		case *values.JavaRef:
			if !uv.IsThis {
				return ""
			}
			// `return this`: cast only when returning as a DIFFERENT raw type than this's own class
			// (a supertype/interface), i.e. the erasures differ. Same-erasure (the class's own type)
			// converts by unchecked conversion and must not be cast.
			if erasureName(retStr) == erasureName(raw.String(funcCtx)) {
				return ""
			}
		default:
			return ""
		}
	}
	return retStr
}

// typeVarFieldStoreCast returns the bare class-scope type variable (e.g. "K") to cast the stored
// value to when assigning into a SAME-CLASS field that is declared as that type variable but the
// stored value's static type is a DIFFERENT reference type (Object or the erased bound). Bytecode
// erases a type-variable field to its bound, so `this.key = objExpr` (objExpr typed Object, as a raw
// `keys[]` element read produces) fails "incompatible types: Object cannot be converted to K"; the
// source originally carried an explicit `(K)` cast (guava CompactHashMap.MapEntry). The cast is
// unchecked but behavior-preserving (the field erases to Object at runtime), matching CFR/Fernflower.
// Only same-class fields qualify because their declared type variable is only known for the class
// being rendered (funcCtx.FieldTypeVars). Returns "" for non-field LHS, foreign fields, non-type-var
// fields, primitive/nil values, null literals, or values already rendering as the type variable.
// Kill-switch JDEC_NO_TYPEVAR_FIELD_CAST disables it.
func typeVarFieldStoreCast(funcCtx *class_context.ClassContext, left values.JavaValue, value values.JavaValue) string {
	if funcCtx == nil || left == nil || value == nil {
		return ""
	}
	if os.Getenv("JDEC_NO_TYPEVAR_FIELD_CAST") != "" {
		return ""
	}
	if len(funcCtx.FieldTypeVars) == 0 {
		return ""
	}
	var fieldName string
	switch lv := left.(type) {
	case *values.RefMember:
		// instance field store `this.field` - the receiver must be `this`.
		ref, ok := values.UnpackSoltValue(lv.Object).(*values.JavaRef)
		if !ok || !ref.IsThis {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	case *values.JavaClassMember:
		// static field store `ClassName.field` into the class being rendered.
		if lv.Name != funcCtx.ClassName {
			return ""
		}
		fieldName = class_context.SafeIdentifier(lv.Member)
	default:
		return ""
	}
	tv := funcCtx.FieldTypeVar(fieldName)
	if tv == "" {
		return ""
	}
	if lit, ok := values.UnpackSoltValue(value).(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
		// null is assignable to any type variable without a cast.
		return ""
	}
	vt := value.Type()
	if vt == nil {
		return ""
	}
	raw := vt.RawType()
	if raw == nil {
		return ""
	}
	// A type variable is a reference type; a primitive value never needs (and cannot take) the cast.
	if _, isPrim := raw.(*types.JavaPrimer); isPrim {
		return ""
	}
	// Already rendered as the type variable: no cast needed.
	if raw.String(funcCtx) == tv {
		return ""
	}
	return tv
}

// erasureName strips a generic type string down to its raw/erased name: "Predicate<T>" -> "Predicate",
// "Map<K, V>" -> "Map", "CopyOnWriteHashMap$InnerNode<K, V>" -> "CopyOnWriteHashMap$InnerNode".
func erasureName(s string) string {
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// mentionsTypeParam reports whether retStr contains any of the in-scope type-variable names as a
// whole identifier token (so type param "T" matches "Converter<T, T>" and bare "T" but never the "T"
// inside "String" or a class named "Tree"). Used to extend the type-variable return cast to generic
// return types whose type ARGUMENTS are type variables, not just bare-type-variable returns.
func mentionsTypeParam(retStr string, typeParams []string) bool {
	if len(typeParams) == 0 || retStr == "" {
		return false
	}
	isIdent := func(b byte) bool {
		return b == '_' || b == '$' ||
			(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
	}
	for _, tp := range typeParams {
		if tp == "" {
			continue
		}
		from := 0
		for {
			idx := strings.Index(retStr[from:], tp)
			if idx < 0 {
				break
			}
			start := from + idx
			end := start + len(tp)
			beforeOK := start == 0 || !isIdent(retStr[start-1])
			afterOK := end == len(retStr) || !isIdent(retStr[end])
			if beforeOK && afterOK {
				return true
			}
			from = start + 1
		}
	}
	return false
}

// narrowingReturnCast returns the cast type name ("char"/"byte"/"short") when the enclosing
// method's declared return type is a narrowing-of-int and the returned value is int-typed,
// otherwise "". The cast lets the emitted source recompile without "possible lossy
// conversion from int to char/byte/short" errors.
func narrowingReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(&class_context.ClassContext{})
	valStr := v.Type().RawType().String(&class_context.ClassContext{})
	if valStr != "int" {
		return ""
	}
	switch retStr {
	case "char", "byte", "short":
		return retStr
	}
	return ""
}

// narrowingInitCast returns the cast type name ('byte'/'char'/'short') when a local is declared
// with a narrowing-of-int slot type but its initializer is int-valued. Per JLS the initializer is
// always int-promoted for arithmetic/bitwise/shift expressions, so assigning it to a byte/char/short
// local without a cast is a 'possible lossy conversion' javac error (e.g. commons-codec
// PureJavaCrc32C: `byte x = (arr[i] ^ crc) & 255`). Wrapping the initializer in an explicit cast
// is a pure rendering fix — the recompiled bytecode is behaviorally identical.
// intCategoryWiderThan reports whether slot type a is `int` while initializer type b is one of the
// narrower int-category primitives (byte/char/short). It is used to choose the declared type of a
// local whose slot was unified to int (because a later store assigns an int-valued expression) but
// whose first/initializer value is a narrower type that widens to int implicitly. Only the widening
// to int is recognized (the only widening the slot-merge in AssignVar produces); boolean and the
// non-int categories are excluded by the underlying name checks.
func intCategoryWiderThan(a types.JavaType, b types.JavaType) bool {
	if a == nil || b == nil {
		return false
	}
	pa, oka := a.RawType().(*types.JavaPrimer)
	pb, okb := b.RawType().(*types.JavaPrimer)
	if !oka || !okb {
		return false
	}
	if pa.Name != types.JavaInteger {
		return false
	}
	switch pb.Name {
	case types.JavaByte, types.JavaChar, types.JavaShort:
		return true
	}
	return false
}

func narrowingInitCast(slotType types.JavaType, valueType types.JavaType) string {
	if slotType == nil || valueType == nil {
		return ""
	}
	slotStr := slotType.RawType().String(&class_context.ClassContext{})
	valStr := valueType.RawType().String(&class_context.ClassContext{})
	if valStr != "int" {
		return ""
	}
	switch slotStr {
	case "char", "byte", "short":
		return slotStr
	}
	return ""
}

func NewReturnStatement(value values.JavaValue) *ReturnStatement {
	return &ReturnStatement{
		JavaValue: value,
	}
}

type StackAssignStatement struct {
	Id        int
	JavaValue *values.JavaRef
}

// ReplaceVar implements Statement.
func (a *StackAssignStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.JavaValue.ReplaceVar(oldId, newId)
}

func (a *StackAssignStatement) String(funcCtx *class_context.ClassContext) string {
	return a.JavaValue.String(funcCtx)
}
func NewStackAssignStatement(id int, value *values.JavaRef) *StackAssignStatement {
	return &StackAssignStatement{
		Id:        id,
		JavaValue: value,
	}
}

type AssignStatement struct {
	LeftValue   values.JavaValue
	ArrayMember *values.JavaArrayMember
	JavaValue   values.JavaValue
	IsDeclare   bool
	IsFirst     bool
}

// ReplaceVar implements Statement.
func (a *AssignStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if a.LeftValue != nil {
		a.LeftValue.ReplaceVar(oldId, newId)
	}
	if a.ArrayMember != nil {
		a.ArrayMember.ReplaceVar(oldId, newId)
	}
	if a.JavaValue != nil {
		a.JavaValue.ReplaceVar(oldId, newId)
	}
}

// arrayStoreRHS renders the right-hand side of an array-element store. The JVM has no boolean type on
// the operand stack: `boolean[] a; a[i] = true;` compiles to iconst_1 + bastore, so the stored value
// reaches the decompiler as an int literal (1) whose static type is int, not boolean. Rendering it
// verbatim yields `a[i] = 1`, which javac rejects ("int cannot be converted to boolean"). When the
// array's element type is boolean and the value is an int literal, render it as true/false. bastore is
// shared with byte[] and the remaining primitive array stores all accept a fitting int constant, so
// boolean is the only element type that needs this coercion.
func arrayStoreRHS(member *values.JavaArrayMember, value values.JavaValue, funcCtx *class_context.ClassContext) string {
	if member != nil && value != nil {
		if elem := member.Type(); elem != nil && elem.String(funcCtx) == "boolean" {
			if lit, ok := value.(*values.JavaLiteral); ok {
				if iv, ok := lit.Data.(int); ok {
					if iv == 0 {
						return "false"
					}
					return "true"
				}
			}
		}
		// Narrowing cast for byte[]/char[]/short[] element stores. bastore/castore/sastore implicitly
		// truncate the int on the stack to the element width, but Java source assignment context (JLS
		// 5.2) forbids the implicit int->byte/char/short narrowing, so `arr[i] = intValue` is a
		// "possible lossy conversion" error (commons-codec QCodec `out[i] = b` where b is int-typed).
		// The explicit cast reproduces the truncation the opcode already performs, so it is
		// behaviorally identical; only int-typed values need it (a value already byte/char/short
		// widens to int and back without change, and long/float/double cannot reach these arrays).
		if member.Type() != nil {
			if cast := narrowingInitCast(member.Type(), value.Type()); cast != "" {
				return fmt.Sprintf("(%s) (%s)", cast, value.String(funcCtx))
			}
		}
	}
	return value.String(funcCtx)
}

func (a *AssignStatement) String(funcCtx *class_context.ClassContext) string {
	if a.IsDeclare {
		if a.LeftValue == nil {
			return values.EmptySlotValuePlaceholder
		}
		return fmt.Sprintf("%s %s", a.LeftValue.Type().String(funcCtx), a.LeftValue.String(funcCtx))
	}
	if a.ArrayMember != nil {
		if a.JavaValue == nil {
			return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), values.EmptySlotValuePlaceholder)
		}
		return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), arrayStoreRHS(a.ArrayMember, a.JavaValue, funcCtx))
	}
	if a.LeftValue == nil || a.JavaValue == nil {
		left := values.EmptySlotValuePlaceholder
		right := values.EmptySlotValuePlaceholder
		if a.LeftValue != nil {
			left = a.LeftValue.String(funcCtx)
		}
		if a.JavaValue != nil {
			right = a.JavaValue.String(funcCtx)
		}
		return fmt.Sprintf("%s = %s", left, right)
	}
	assign := fmt.Sprintf("%s = %s", a.LeftValue.String(funcCtx), a.JavaValue.String(funcCtx))
	if a.IsFirst {
		// For `T x = null`, the initializer's static type is java.lang.Object, but the
		// variable's declared type is its (possibly refined) ref type — using the initializer
		// type would emit `Object x = null` even after the slot adopted a concrete type, and
		// `return x` would then mismatch the method's return type. Prefer the variable type
		// for a null initializer; for every other case this is identical to the value type.
		declType := a.JavaValue.Type()
		if lit, ok := a.JavaValue.(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
			declType = a.LeftValue.Type()
		}
		// A class literal initializer (`Foo.class`) is a java.lang.Class object, but its
		// JavaValue.Type() reports the *referenced* class (Foo) to drive bare-name rendering and
		// static-call receivers (`Foo.class`, `Foo.parseInt(...)`). When captured into a local the
		// declared type must be java.lang.Class, not Foo, or later member reads (`c.getName()`,
		// `c.isPrimitive()`) fail to recompile ("cannot find symbol"). Declare it `Class`; raw Class
		// is assignment-compatible with `Foo.class` and always recompiles. Kill-switch:
		// JDEC_NO_CLASSLIT_SLOT_TYPE=1 (shared with the slot-typing guard in stack_simulation.go).
		if _, ok := values.UnpackSoltValue(a.JavaValue).(*values.JavaClassValue); ok && os.Getenv("JDEC_NO_CLASSLIT_SLOT_TYPE") == "" {
			declType = types.NewJavaClass("java.lang.Class")
		}
		// Either side's type can be nil under incomplete simulation; fall back to the other
		// side rather than dereferencing nil (which panicked the whole method into a stub).
		if declType == nil {
			declType = a.LeftValue.Type()
		}
		if declType == nil {
			declType = a.JavaValue.Type()
		}
		if declType == nil {
			// No recoverable declared type. Emit the placeholder so the dumper's safety net
			// degrades this method cleanly instead of crashing.
			return values.EmptySlotValuePlaceholder + " " + assign
		}
		if _, ok := declType.RawType().(*types.JavaMultiCatchType); ok {
			// A multi-catch union type is legal only inside `catch (A | B e)`. If the exception
			// value is hoisted into an ordinary local (`cause = e` after the catch), render it as
			// a common Throwable subtype so the declaration remains valid Java.
			declType = types.NewJavaClass("java.lang.Exception")
		}
		// When the slot's resolved type is a WIDER int-category primitive than the initializer's
		// type, declare with the slot type so the initializer widens implicitly. The slot gets
		// widened to int when a later reassignment stores an int-valued expression into a slot first
		// seen as byte/char/short (commons-codec QuotedPrintableCodec.getUnsignedOctet:
		// `int o = bytes[i]; if (o < 0) o = 256 + o;` — slot is int, the baload initializer is byte).
		// Without this the variable is declared `byte o = bytes[i]` and the `o = 256 + o` reassign is
		// a "possible lossy conversion from int to byte" error; casting the reassign to byte would be
		// SEMANTICALLY WRONG (it truncates 255 back to -1), so the correct fix is the wider int decl.
		if lt := a.LeftValue.Type(); intCategoryWiderThan(lt, declType) {
			declType = lt
		}
		// Narrowing cast for byte/char/short locals: JLS promotes these types to int in any
		// arithmetic/bitwise/shift expression, so `byte x = (arr[i] ^ crc) & 255` is int-valued at
		// the source level even though the slot is byte (commons-codec PureJavaCrc32C). When the
		// slot type is a narrowing-of-int and the initializer is int-typed, keep the slot type as
		// the declaration and wrap the initializer in an explicit cast — mirrors ReturnStatement.
		if cast := narrowingInitCast(a.LeftValue.Type(), declType); cast != "" {
			assign = fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
			declType = a.LeftValue.Type()
		}
		return declType.String(funcCtx) + " " + assign
	} else {
		// Narrowing REASSIGNMENT to a byte/char/short target (field/local) whose RHS is int-typed:
		// `this.quote = cond ? '\'' : '"'` compiles to a ternary pushing the char constants as ints
		// (the JVM stack has no char category) followed by `putfield ... C`, which truncates
		// implicitly, so no i2c opcode is emitted and the value reaches us int-typed. Rendered
		// verbatim that is `this.quote = cond ? 39 : 34`, which javac rejects in assignment context
		// (JLS 5.2: "possible lossy conversion from int to char"; a non-constant conditional is not a
		// constant expression, so constant-narrowing does not apply). The declare branch above and the
		// array-element store (arrayStoreRHS) already insert this cast; mirror them here so plain
		// reassignments to narrow primitive fields/locals recompile (fastjson2 JSONWriter.quote,
		// JSONReaderUTF8 char writes). The explicit cast reproduces exactly the truncation the store
		// opcode performs, so it is behaviorally identical. Values already typed char/byte/short (and
		// those carrying an i2c/i2b/i2s cast) report a non-int type, so narrowingInitCast returns ""
		// and they are untouched. Kill-switch JDEC_NO_NARROW_REASSIGN_CAST=1.
		if os.Getenv("JDEC_NO_NARROW_REASSIGN_CAST") == "" && a.LeftValue != nil && a.JavaValue != nil {
			if cast := narrowingInitCast(a.LeftValue.Type(), a.JavaValue.Type()); cast != "" {
				return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
			}
		}
		// Type-variable field store: `this.key = objExpr` where `key` is declared `K` needs an
		// explicit unchecked `(K)` cast (the field erases to its bound in bytecode). See
		// typeVarFieldStoreCast.
		if cast := typeVarFieldStoreCast(funcCtx, a.LeftValue, a.JavaValue); cast != "" {
			return fmt.Sprintf("%s = (%s) (%s)", a.LeftValue.String(funcCtx), cast, a.JavaValue.String(funcCtx))
		}
		return assign
	}
}

type ForStatement struct {
	InitVar       Statement
	Condition     *ConditionStatement
	EndExp        Statement
	SubStatements []Statement
}

// ReplaceVar implements Statement.
func (f *ForStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	f.InitVar.ReplaceVar(oldId, newId)
	f.Condition.ReplaceVar(oldId, newId)
	f.EndExp.ReplaceVar(oldId, newId)
	for _, st := range f.SubStatements {
		st.ReplaceVar(oldId, newId)
	}
}

func NewForStatement(subStatements []Statement) *ForStatement {
	return &ForStatement{
		InitVar:       subStatements[0],
		Condition:     subStatements[1].(*ConditionStatement),
		EndExp:        subStatements[len(subStatements)-2],
		SubStatements: subStatements[2 : len(subStatements)-2],
	}
}
func (f *ForStatement) String(funcCtx *class_context.ClassContext) string {
	datas := []string{}
	datas = append(datas, f.InitVar.String(funcCtx))
	datas = append(datas, f.Condition.String(funcCtx))
	datas = append(datas, f.EndExp.String(funcCtx))
	statementStr := []string{}
	for _, statement := range f.SubStatements {
		statementStr = append(statementStr, statement.String(funcCtx))
	}
	s := fmt.Sprintf("for(%s; %s; %s) {\n%s\n}", datas[0], datas[1], datas[2], strings.Join(statementStr, "\n"))
	return s
}

func NewArrayMemberAssignStatement(m *values.JavaArrayMember, value values.JavaValue) *AssignStatement {
	return &AssignStatement{
		ArrayMember: m,
		JavaValue:   value,
	}
}

func NewDeclareStatement(leftVal values.JavaValue) *AssignStatement {
	return &AssignStatement{
		LeftValue: leftVal,
		IsDeclare: true,
	}
}
func NewAssignStatement(leftVal, value values.JavaValue, isFirst bool) *AssignStatement {
	if value == nil || leftVal == nil || value.Type() == nil || leftVal.Type() == nil {
		// Guard against nil values/types in malformed bytecode: rather than panicking
		// (which forces the whole method into a stub), create the assignment as-is.
		// The type merge is skipped when either side has no type.
	}

	if value.Type() != nil && leftVal.Type() != nil {
		value.Type().ResetType(leftVal.Type())
	}
	return &AssignStatement{
		LeftValue: leftVal,
		JavaValue: value,
		IsFirst:   isFirst,
	}
}

type IfStatement struct {
	Condition values.JavaValue
	IfBody    []Statement
	ElseBody  []Statement
}

func (g *IfStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	g.Condition.ReplaceVar(oldId, newId)
	for _, st := range g.IfBody {
		st.ReplaceVar(oldId, newId)
	}
	for _, st := range g.ElseBody {
		st.ReplaceVar(oldId, newId)
	}
}

func (g *IfStatement) String(funcCtx *class_context.ClassContext) string {
	getBody := func(sts []Statement) string {
		var res []string
		for _, st := range sts {
			res = append(res, st.String(funcCtx))
		}
		return strings.Join(res, "\n")
	}
	return fmt.Sprintf("if (%s){\n"+
		"%s\n"+
		"}else{\n"+
		"%s\n"+
		"}", g.Condition.String(funcCtx), getBody(g.IfBody), getBody(g.ElseBody))
}
func NewIfStatement(condition values.JavaValue, ifBody, elseBody []Statement) *IfStatement {
	return &IfStatement{
		Condition: condition,
		IfBody:    ifBody,
		ElseBody:  elseBody,
	}
}

type GOTOStatement struct {
	ToStatement int
}

// ReplaceVar implements Statement.
func (g *GOTOStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (g *GOTOStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("goto: %d", g.ToStatement)
}
func NewGOTOStatement() *GOTOStatement {
	return &GOTOStatement{}
}

type NewStatement struct {
	Class *types.JavaClass
}

// ReplaceVar implements Statement.
func (a *NewStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Class.ReplaceVar(oldId, newId)
}

func (a *NewStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("new %s()", a.Class.Name)
}

func NewNewStatement(class *types.JavaClass) *NewStatement {
	return &NewStatement{
		Class: class,
	}
}

type ExpressionStatement struct {
	Expression values.JavaValue
}

// ReplaceVar implements Statement.
func (a *ExpressionStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Expression.ReplaceVar(oldId, newId)
}

func (a *ExpressionStatement) String(funcCtx *class_context.ClassContext) string {
	return a.Expression.String(funcCtx)
}

func NewExpressionStatement(v values.JavaValue) *ExpressionStatement {
	return &ExpressionStatement{
		Expression: v,
	}
}

type CaseItem struct {
	IsDefault bool
	IntValue  int
	Body      []Statement
}

func NewCaseItem(v int, body []Statement) *CaseItem {
	return &CaseItem{
		Body:     body,
		IntValue: v,
	}
}

type SwitchStatement struct {
	Value values.JavaValue
	Cases []*CaseItem
}

// ReplaceVar implements Statement.
func (a *SwitchStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Value.ReplaceVar(oldId, newId)
	for _, c := range a.Cases {
		for _, st := range c.Body {
			st.ReplaceVar(oldId, newId)
		}
	}
}

func (a *SwitchStatement) String(funcCtx *class_context.ClassContext) string {
	casesStrs := []string{}
	for _, c := range a.Cases {
		if c.IsDefault {
			casesStrs = append(casesStrs, fmt.Sprintf("default:\n%s", StatementsString(c.Body, funcCtx)))
			continue
		}
		casesStrs = append(casesStrs, fmt.Sprintf("case %d:\n%s", c.IntValue, StatementsString(c.Body, funcCtx)))
	}
	return fmt.Sprintf("switch(%s) {\n%s\n}", a.Value.String(funcCtx), strings.Join(casesStrs, "\n"))
}

func NewSwitchStatement(value values.JavaValue, cases []*CaseItem) *SwitchStatement {
	return &SwitchStatement{
		Value: value,
		Cases: cases,
	}
}

const (
	MiddleSwitch   = "switch"
	MiddleTryStart = "tryStart"
)

type MiddleStatement struct {
	Data any
	Flag string
}

// ReplaceVar implements Statement.
func (a *MiddleStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (a *MiddleStatement) String(funcCtx *class_context.ClassContext) string {
	return a.Flag
}

func NewMiddleStatement(flag string, d any) *MiddleStatement {
	return &MiddleStatement{
		Flag: flag,
		Data: d,
	}
}

type SynchronizedStatement struct {
	Argument values.JavaValue
	Body     []Statement
}

// ReplaceVar implements Statement.
func (s *SynchronizedStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	s.Argument.ReplaceVar(oldId, newId)
	for _, st := range s.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewSynchronizedStatement(val values.JavaValue, body []Statement) *SynchronizedStatement {
	return &SynchronizedStatement{Argument: val, Body: body}
}

func (s *SynchronizedStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("synchronized(%s) {\n%s\n}", s.Argument.String(funcCtx), StatementsString(s.Body, funcCtx))
}
