package values

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type JavaRef struct {
	VarUid      string
	Id          *utils.VariableId
	StackVar    JavaValue
	CustomValue *CustomValue
	IsThis      bool
	Val         JavaValue
	typ         types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *JavaRef) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if j.Id == oldId {
		j.Id = newId
	}
	if j.StackVar != nil {
		j.StackVar.ReplaceVar(oldId, newId)
	}
}

func (j *JavaRef) Type() types.JavaType {
	if j == nil || j.typ == nil {
		// Fallback for a nil ref or a ref created without a proper type (e.g.
		// from the getVarScope fallback in complex CFG paths). Return Object to
		// avoid nil pointer dereference downstream.
		return types.NewJavaClass("java.lang.Object")
	}
	return j.typ
}

// ResetVarType repoints the variable's declared type. Used when a slot first seen as a
// null initializer (Object-typed) is later assigned a concrete reference type: the slot is
// kept as one variable and adopts the concrete type instead of being split.
func (j *JavaRef) ResetVarType(t types.JavaType) {
	j.typ = t
}

// IsNullInitialized reports whether this variable's stored value is the `null` literal, i.e.
// it was declared as `T x = null` with no committed concrete type yet.
func (j *JavaRef) IsNullInitialized() bool {
	lit, ok := j.Val.(*JavaLiteral)
	return ok && lit != nil && fmt.Sprint(lit.Data) == "null"
}

func IsNullLiteral(v JavaValue) bool {
	lit, ok := v.(*JavaLiteral)
	return ok && lit != nil && fmt.Sprint(lit.Data) == "null"
}

func (j *JavaRef) String(funcCtx *class_context.ClassContext) string {
	if j.IsThis {
		return "this"
	}
	if j.CustomValue != nil {
		return j.CustomValue.String(funcCtx)
	}
	if j.StackVar != nil {
		return j.StackVar.String(funcCtx)
	}
	return j.Id.String()
}

// javaRefUidCounter backs VarUid generation. VarUid only needs to be unique per
// JavaRef instance (it is used as a map key and for instance equality), so a process
// wide monotonic counter is sufficient. This replaces a per-variable uuid.NewString()
// call: under the parallel jdsc workload its crypto/rand getentropy() syscalls
// serialize on a kernel lock and burn a large fraction of CPU (profiled as the top
// cost), which the counter eliminates entirely.
var javaRefUidCounter atomic.Int64

func NewJavaRef(id *utils.VariableId, val JavaValue, typ types.JavaType) *JavaRef {
	return &JavaRef{
		VarUid: "ref-" + strconv.FormatInt(javaRefUidCounter.Add(1), 10),
		Id:     id,
		Val:    val,
		typ:    typ,
	}
}

type JavaArray struct {
	Class    *types.JavaClass
	Length   JavaValue
	JavaType types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *JavaArray) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	j.Length.ReplaceVar(oldId, newId)
}

func (j *JavaArray) Type() types.JavaType {
	return j.JavaType
}

func (j *JavaArray) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s[%d]", j.Class.String(funcCtx), j.Length)
}

func NewJavaArray(class *types.JavaClass, length JavaValue) *JavaArray {
	return &JavaArray{
		Class:    class,
		Length:   length,
		JavaType: types.NewJavaArrayType(class),
	}
}

type JavaLiteral struct {
	JavaType types.JavaType
	Data     any
}

// ReplaceVar implements JavaValue.
func (j *JavaLiteral) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (j *JavaLiteral) Type() types.JavaType {
	return j.JavaType
}

func JavaStringToLiteral(i any) string {
	data := fmt.Sprint(i)
	// MatchMIMEType runs full magic-byte sniffing (allocating a csv/bufio reader) and
	// is only useful to recover a mis-decoded Chinese charset, which by definition needs
	// non-ASCII bytes. Pure-ASCII literals (the overwhelming majority) can never match a
	// Chinese charset, so skip the expensive detection -- it was ~4% of all decompiler
	// allocations. Behavior is unchanged: ASCII already fell through to the quote path.
	if !isPureASCII(data) {
		mimeType, _ := codec.MatchMIMEType(data)
		if mimeType != nil && mimeType.IsChineseCharset() {
			result, ok := mimeType.TryUTF8Convertor([]byte(data))
			if ok {
				return fixJavaStringEscapes(strconv.Quote(string(result)))
			}
		}
	}
	return fixJavaStringEscapes(strconv.Quote(data))
}

// isPureASCII reports whether s contains only bytes < 0x80.
func isPureASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// These regexes are compiled once at package init rather than per call: fixJavaStringEscapes
// runs for every decompiled string literal, and re-creating the wrappers each time made
// regexp compilation one of the top decompiler-core allocators (~5% of all bytes). A
// *regexp.Regexp is safe for concurrent use, so a single shared wrapper serves all (including
// parallel) decompiles.
var (
	reJavaHexEscape  = regexp_utils.NewRegexpWrapper(`(\\+)x[0-9a-fA-F]{2}`)
	reJavaBellEscape = regexp_utils.NewRegexpWrapper(`(\\+)a`)
	reJavaVTabEscape = regexp_utils.NewRegexpWrapper(`(\\+)v`)
)

// fixJavaStringEscapes converts Go-style escapes (emitted by strconv.Quote) that are not
// valid in Java string literals into Java-compatible "\uXXXX" escapes:
//   - "\xHH"  -> "\u00HH"   (Java has no \x hex escape)
//   - "\a"    -> "\u0007"   (Java has no bell escape)
//   - "\v"    -> "\u000b"   (Java has no vertical-tab escape)
func fixJavaStringEscapes(raw string) string {
	results, err := reJavaHexEscape.ReplaceAllStringFunc(raw, func(s string) string {
		if strings.Count(s, `\`)%2 == 0 {
			return s
		}
		// return \u00xx
		length := len(s)
		pre, after := s[:length-3], "u00"+s[length-2:]
		return pre + after
	})
	if err != nil {
		results = raw
	}
	// single-char escapes that Java does not support
	convertSingle := func(input string, re *regexp_utils.RegexpWrapper, replacement string) string {
		out, e := re.ReplaceAllStringFunc(input, func(s string) string {
			if strings.Count(s, `\`)%2 == 0 {
				return s // even number of backslashes => literal, not an escape
			}
			// drop the trailing "\<escChar>" and append the replacement (which carries its own backslash)
			return s[:len(s)-2] + replacement
		})
		if e != nil {
			return input
		}
		return out
	}
	results = convertSingle(results, reJavaBellEscape, `\u0007`)
	results = convertSingle(results, reJavaVTabEscape, `\u000b`)
	return results
}

func (j *JavaLiteral) String(funcCtx *class_context.ClassContext) string {
	typeStr := j.JavaType.String(funcCtx)
	switch typeStr {
	case types.NewJavaPrimer(types.JavaBoolean).String(funcCtx):
		if v, ok := j.Data.(int); ok {
			if v == 0 {
				return "false"
			}
			return "true"
		}
	case types.NewJavaPrimer(types.JavaLong).String(funcCtx):
		// long literals need an explicit L suffix in expression position. The field
		// path adds it separately; without it here, values beyond int range fail to
		// compile ("integer number too large"), e.g. Long.valueOf(9223372036854775807).
		s := fmt.Sprint(j.Data)
		if s != "" && !strings.HasSuffix(s, "L") && !strings.HasSuffix(s, "l") {
			s += "L"
		}
		return s
	case types.NewJavaPrimer(types.JavaFloat).String(funcCtx):
		// A bare decimal literal is a double in Java, so a float value must carry an
		// F suffix or it is a type error (e.g. Float.valueOf(3.14) has no overload).
		return javaFloatLiteralExpr(j.Data)
	case types.NewJavaPrimer(types.JavaDouble).String(funcCtx):
		// The D suffix keeps an integral double (e.g. 1.0 -> "1") from being read as
		// an int, which would break overloads like Double.valueOf(double).
		return javaDoubleLiteralExpr(j.Data)
	}
	if typeStr == "java.lang.String" || typeStr == "String" {
		return JavaStringToLiteral(j.Data)
	}
	return fmt.Sprint(j.Data)
}

// literalToFloat64 normalizes the numeric payload of a float/double literal.
func literalToFloat64(data any) (float64, bool) {
	switch v := data.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	}
	return 0, false
}

// javaFloatLiteralExpr renders a float constant as a valid Java float literal (with
// an F suffix), handling NaN/Infinity. Mirrors the field-path renderer in dumper.go.
func javaFloatLiteralExpr(data any) string {
	f, ok := literalToFloat64(data)
	if !ok {
		return fmt.Sprint(data)
	}
	switch {
	case math.IsNaN(f):
		return "Float.NaN"
	case math.IsInf(f, 1):
		return "Float.POSITIVE_INFINITY"
	case math.IsInf(f, -1):
		return "Float.NEGATIVE_INFINITY"
	}
	return strconv.FormatFloat(f, 'g', -1, 32) + "F"
}

// javaDoubleLiteralExpr renders a double constant as a valid Java double literal
// (with a D suffix), handling NaN/Infinity. Mirrors the field-path renderer.
func javaDoubleLiteralExpr(data any) string {
	f, ok := literalToFloat64(data)
	if !ok {
		return fmt.Sprint(data)
	}
	switch {
	case math.IsNaN(f):
		return "Double.NaN"
	case math.IsInf(f, 1):
		return "Double.POSITIVE_INFINITY"
	case math.IsInf(f, -1):
		return "Double.NEGATIVE_INFINITY"
	}
	return strconv.FormatFloat(f, 'g', -1, 64) + "D"
}

func NewJavaLiteral(data any, typ types.JavaType) *JavaLiteral {
	return &JavaLiteral{
		JavaType: typ,
		Data:     data,
	}
}

type JavaClassValue struct {
	types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *JavaClassValue) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

// String implements JavaValue.
// Subtle: this method shadows the method (JavaType).String of JavaClassValue.JavaType.
func (j *JavaClassValue) String(funcCtx *class_context.ClassContext) string {
	// An array-typed class value can only appear as a class literal (e.g. boolean[].class),
	// never as a cast target or static-call receiver (those bypass this method via Type()).
	// Rendering the bare type "boolean[]" would be invalid in expression position.
	if j.JavaType != nil && j.JavaType.IsArray() {
		return j.JavaType.String(funcCtx) + ".class"
	}
	return j.JavaType.String(funcCtx)
}

func (j *JavaClassValue) Type() types.JavaType {
	return j.JavaType
}
func NewJavaClassValue(typ types.JavaType) *JavaClassValue {
	return &JavaClassValue{
		JavaType: typ,
	}
}

type JavaClassMember struct {
	Name        string
	Member      string
	Description string
	JavaType    types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *JavaClassMember) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (j *JavaClassMember) Type() types.JavaType {
	return j.JavaType
}

func (j *JavaClassMember) String(funcCtx *class_context.ClassContext) string {
	if j.Name == funcCtx.ClassName {
		return j.Member
	}
	//name := funcCtx.ShortTypeName(j.Name)
	name := funcCtx.ShortTypeName(j.Name)
	return fmt.Sprintf("%s.%s", name, j.Member)
}
func NewJavaClassMember(typeName, member string, desc string, typ types.JavaType) *JavaClassMember {
	return &JavaClassMember{
		Name:        typeName,
		Member:      member,
		Description: desc,
		JavaType:    typ,
	}
}

type RefMember struct {
	Member   string
	Object   JavaValue
	JavaType types.JavaType
}

// ReplaceVar implements JavaValue.
func (j *RefMember) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	j.Object.ReplaceVar(oldId, newId)
}

func (j *RefMember) Type() types.JavaType {
	return j.JavaType
}

func NewRefMember(object JavaValue, member string, typ types.JavaType) *RefMember {
	//if object.Type().RawType().(*types.JavaClass){
	//	if object.Type().String(&class_context.ClassContext{}) == "java.lang.Object" {
	//		rawObject := object
	//		newType := types.NewJavaArrayType(object.Type())
	//		object = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
	//			return fmt.Sprintf("(%s)(%s)", newType.String(funcCtx), rawObject.String(funcCtx))
	//		}, func() types.JavaType {
	//			return newType
	//		})
	//	}
	//}
	return &RefMember{
		Member:   member,
		Object:   object,
		JavaType: typ,
	}
}

type JavaArrayMember struct {
	Object JavaValue
	Index  JavaValue
}

// ReplaceVar implements JavaValue.
func (j *JavaArrayMember) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	j.Object.ReplaceVar(oldId, newId)
	j.Index.ReplaceVar(oldId, newId)
}

func (j *JavaArrayMember) Type() types.JavaType {
	ot := j.Object.Type()
	if ot == nil {
		return nil
	}
	return ot.ElementType()
}
func (j *JavaArrayMember) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s[%v]", j.Object.String(funcCtx), j.Index.String(funcCtx))
}

func NewJavaArrayMember(object JavaValue, index JavaValue) *JavaArrayMember {
	// object.Type() can be nil under incomplete stack simulation; guard before the IsArray/String
	// inspection so a typeless array base degrades to a plain member access instead of panicking
	// the whole method into a stub.
	if ot := object.Type(); ot != nil && !ot.IsArray() {
		if ot.String(&class_context.ClassContext{}) == "java.lang.Object" {
			rawObject := object
			newType := types.NewJavaArrayType(ot)
			object = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("(%s)(%s)", newType.String(funcCtx), rawObject.String(funcCtx))
			}, func() types.JavaType {
				return newType
			})
		}
	}
	return &JavaArrayMember{
		Object: object,
		Index:  index,
	}
}

func (j *RefMember) String(funcCtx *class_context.ClassContext) string {
	//if j.Id == 0 {
	//	return j.Member
	//}
	return fmt.Sprintf("%s.%s", j.Object.String(funcCtx), j.Member)
}

type javaNull struct {
}

// ReplaceVar implements JavaValue.
func (j javaNull) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (j javaNull) Type() types.JavaType {
	return types.NewJavaPrimer(types.JavaVoid)
}

func (j javaNull) String(funcCtx *class_context.ClassContext) string {
	return "null"
}

func (j javaNull) IsJavaType() {
}

var JavaNull = javaNull{}

type TernaryExpression struct {
	Condition       JavaValue
	ConditionFromOp int
	TrueValue       JavaValue
	FalseValue      JavaValue
}

// ReplaceVar implements JavaValue.
func (j *TernaryExpression) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	j.Condition.ReplaceVar(oldId, newId)
	j.TrueValue.ReplaceVar(oldId, newId)
	j.FalseValue.ReplaceVar(oldId, newId)
}

func (j *TernaryExpression) Type() types.JavaType {
	return types.MergeTypes(j.TrueValue.Type(), j.FalseValue.Type())
}

// boolLiteralValue reports a boolean literal's value (unwrapping SlotValue). A boolean literal is a
// JavaLiteral of boolean type with integer data (0=false, non-zero=true). Used to classify ternary
// arms structurally - without rendering them to strings - so boolReduce stays linear.
func boolLiteralValue(v JavaValue) (val bool, ok bool) {
	u := UnpackSoltValue(v)
	lit, isLit := u.(*JavaLiteral)
	if !isLit || lit.JavaType == nil {
		return false, false
	}
	p, isP := lit.JavaType.RawType().(*types.JavaPrimer)
	if !isP || p.Name != types.JavaBoolean {
		return false, false
	}
	if d, isInt := lit.Data.(int); isInt {
		return d != 0, true
	}
	return false, false
}

// boolReduce rewrites a boolean-valued ternary tree into an idiomatic &&/||/! connective, recursively.
// javac compiles short-circuit predicates into nested `cond ? T : F` diamonds whose leaves are the
// boolean literals true/false (often shared across arms). Reconstructing the raw tree and then
// reducing it bottom-up - flipping a comparison operator in place instead of stacking textual ! -
// recovers the original flat `a && b || c` form. Arms that are not boolean-typed (a genuine value
// ternary) are returned untouched so only boolean logic is collapsed.
//
// Arms are classified structurally (boolLiteralValue / pointer identity), never by rendering them to
// strings, so the pass is linear in the tree size rather than quadratic.
func boolReduce(v JavaValue, funcCtx *class_context.ClassContext) JavaValue {
	t, ok := v.(*TernaryExpression)
	if !ok || t.Condition == nil || t.TrueValue == nil || t.FalseValue == nil {
		return v
	}
	if !isBooleanTyped(t.TrueValue) || !isBooleanTyped(t.FalseValue) {
		return v // a value ternary, not a boolean connective
	}
	boolType := types.NewJavaPrimer(types.JavaBoolean)
	notCond := func(c JavaValue) JavaValue {
		return SimplifyConditionValue(NewUnaryExpression(c, Not, boolType))
	}
	and := func(a, b JavaValue) JavaValue { return NewBinaryExpression(a, b, LOGICAL_AND, boolType) }
	or := func(a, b JavaValue) JavaValue { return NewBinaryExpression(a, b, LOGICAL_OR, boolType) }
	c := SimplifyConditionValue(t.Condition)
	tv := boolReduce(t.TrueValue, funcCtx)
	fv := boolReduce(t.FalseValue, funcCtx)
	tval, tLit := boolLiteralValue(tv)
	fval, fLit := boolLiteralValue(fv)
	switch {
	case tLit && fLit && tval && !fval: // c ? true : false  =>  c
		return c
	case tLit && fLit && !tval && fval: // c ? false : true  =>  !c
		return notCond(c)
	case tLit && tval: // c ? true : B  =>  c || B
		return or(c, fv)
	case fLit && !fval: // c ? B : false  =>  c && B
		return and(c, tv)
	case tLit && !tval: // c ? false : B  =>  !c && B
		return and(notCond(c), fv)
	case fLit && fval: // c ? B : true  =>  !c || B
		return or(notCond(c), tv)
	}
	// Shared-leaf factoring: both arms are boolean non-literals, but a short-circuit predicate often
	// shares a leaf between the taken arm and the fall-through (the same value appears once as a whole
	// arm and once as a disjunct/conjunct of the other), e.g. `c ? (A || S) : S` is exactly
	// `(c && A) || S`. The leaf is matched by pointer identity first (one DAG node) and, since
	// SimplifyConditionValue may have rebuilt an equivalent value, by rendered equality as a fallback.
	// This branch is only reached when neither arm is a boolean literal (the common short-circuit
	// shape hits the literal switch above), so its rendering is not on the hot path.
	eq := func(a, b JavaValue) bool {
		if a == nil || b == nil {
			return false
		}
		if UnpackSoltValue(a) == UnpackSoltValue(b) {
			return true
		}
		return a.String(funcCtx) == b.String(funcCtx)
	}
	if orE, isOr := tv.(*JavaExpression); isOr && orE.Op == LOGICAL_OR && len(orE.Values) == 2 {
		if eq(orE.Values[0], fv) { // c ? (S || A) : S  =>  S || (c && A)
			return or(fv, and(c, orE.Values[1]))
		}
		if eq(orE.Values[1], fv) { // c ? (A || S) : S  =>  (c && A) || S
			return or(and(c, orE.Values[0]), fv)
		}
	}
	if andE, isAnd := fv.(*JavaExpression); isAnd && andE.Op == LOGICAL_AND && len(andE.Values) == 2 {
		if eq(andE.Values[0], tv) { // c ? T : (T && A)  =>  T && (c || A)
			return and(tv, or(c, andE.Values[1]))
		}
		if eq(andE.Values[1], tv) { // c ? T : (A && T)  =>  (c || A) && T
			return and(or(c, andE.Values[0]), tv)
		}
	}
	return NewTernaryExpression(c, tv, fv) // irreducible: keep a ternary over the reduced arms
}

func (j *TernaryExpression) String(funcCtx *class_context.ClassContext) string {
	reduced := boolReduce(j, funcCtx)
	if rt, ok := reduced.(*TernaryExpression); ok {
		condition := SimplifyConditionValue(rt.Condition)
		return fmt.Sprintf("(%s) ? (%s) : (%s)", condition.String(funcCtx), rt.TrueValue.String(funcCtx), rt.FalseValue.String(funcCtx))
	}
	return reduced.String(funcCtx)
}

func NewTernaryExpression(condition, v1, v2 JavaValue) *TernaryExpression {
	return &TernaryExpression{
		Condition:  condition,
		TrueValue:  v1,
		FalseValue: v2,
	}
}

// EmptySlotValuePlaceholder is rendered when a SlotValue has no underlying value,
// which means the stack simulation produced an incomplete result for the method.
// It is not valid Java; the dumper detects this marker and degrades the affected
// method to a stub instead of emitting un-compilable source.
const EmptySlotValuePlaceholder = "empty slot value"

type SlotValue struct {
	val     JavaValue
	TmpType types.JavaType
}

// ReplaceVar implements JavaValue.
func (s *SlotValue) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	// val may be nil for an empty slot (see Type/String which already guard this);
	// without the guard rewriteVar panics with a nil pointer dereference.
	if s.val == nil {
		return
	}
	s.val.ReplaceVar(oldId, newId)
}

func (s *SlotValue) Type() types.JavaType {
	if s.val == nil {
		return s.TmpType
	}
	return s.val.Type()
}
func (s *SlotValue) String(funcCtx *class_context.ClassContext) string {
	if s.val == nil {
		return EmptySlotValuePlaceholder
	}
	return s.val.String(funcCtx)
}
func (s *SlotValue) GetValue() JavaValue {
	return s.val
}
func (s *SlotValue) ResetValue(val JavaValue) {
	s.val = val
	// val (or its type) can be nil under incomplete stack simulation; guard before
	// propagating the slot's temp type so a typeless value degrades gracefully
	// instead of panicking the whole method into a stub.
	if val == nil {
		return
	}
	// Both the value's type and the slot's temp type can be nil under incomplete stack
	// simulation (e.g. a reused slot whose type was never committed). ResetTypeRef
	// dereferences its argument, so skip the propagation when either side is nil.
	if t := val.Type(); t != nil && s.TmpType != nil {
		t.ResetTypeRef(s.TmpType)
	}
}
func NewSlotValue(val JavaValue, typ types.JavaType) *SlotValue {
	return &SlotValue{
		val:     val,
		TmpType: typ,
	}
}
