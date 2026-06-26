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
	return j.Object.Type().ElementType()
}
func (j *JavaArrayMember) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s[%v]", j.Object.String(funcCtx), j.Index.String(funcCtx))
}

func NewJavaArrayMember(object JavaValue, index JavaValue) *JavaArrayMember {
	if !object.Type().IsArray() {
		if object.Type().String(&class_context.ClassContext{}) == "java.lang.Object" {
			rawObject := object
			newType := types.NewJavaArrayType(object.Type())
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
func (j *TernaryExpression) String(funcCtx *class_context.ClassContext) string {
	condition := SimplifyConditionValue(j.Condition)
	truePrimer, ok1 := j.TrueValue.Type().RawType().(*types.JavaPrimer)
	falsePrimer, ok2 := j.FalseValue.Type().RawType().(*types.JavaPrimer)
	if ok1 && ok2 && truePrimer.Name == types.JavaBoolean && falsePrimer.Name == types.JavaBoolean {
		if j.TrueValue.String(funcCtx) == "true" && j.FalseValue.String(funcCtx) == "false" {
			return condition.String(funcCtx)
		}
		if j.TrueValue.String(funcCtx) == "false" && j.FalseValue.String(funcCtx) == "true" {
			return NewUnaryExpression(condition, Not, types.NewJavaPrimer(types.JavaBoolean)).String(funcCtx)
		}
	}
	return fmt.Sprintf("(%s) ? (%s) : (%s)", condition.String(funcCtx), j.TrueValue.String(funcCtx), j.FalseValue.String(funcCtx))
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
	s.val.Type().ResetTypeRef(s.TmpType)
}
func NewSlotValue(val JavaValue, typ types.JavaType) *SlotValue {
	return &SlotValue{
		val:     val,
		TmpType: typ,
	}
}
