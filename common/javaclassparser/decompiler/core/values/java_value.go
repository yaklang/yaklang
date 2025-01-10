package values

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strconv"
	"strings"
)

type JavaRef struct {
	Id          *utils.VariableId
	StackVar    JavaValue
	CustomValue *CustomValue
	IsThis      bool
	Val         JavaValue
	typ         types.JavaType
}

func (j *JavaRef) Type() types.JavaType {
	return j.typ
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

func NewJavaRef(id *utils.VariableId, val JavaValue, typ types.JavaType) *JavaRef {
	return &JavaRef{
		Id:  id,
		Val: val,
		typ: typ,
	}
}

type JavaArray struct {
	Class    *types.JavaClass
	Length   JavaValue
	JavaType types.JavaType
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

func (j *JavaLiteral) Type() types.JavaType {
	return j.JavaType
}

func JavaStringToLiteral(i any) string {
	data := fmt.Sprint(i)
	mimeType, _ := codec.MatchMIMEType(data)
	if mimeType != nil && mimeType.IsChineseCharset() {
		result, ok := mimeType.TryUTF8Convertor([]byte(data))
		if ok {
			return strconv.Quote(string(result))
		}
	}

	raw := strconv.Quote(data)
	results, err := regexp_utils.NewRegexpWrapper(`(\\+)x[0-9a-fA-F]{2}`).ReplaceAllStringFunc(raw, func(s string) string {
		if strings.Count(s, `\`)%2 == 0 {
			return s
		}
		// return \u00xx
		length := len(s)
		pre, after := s[:length-3], "u00"+s[length-2:]
		return pre + after
	})
	if err != nil {
		return raw
	}
	return results
}

func (j *JavaLiteral) String(funcCtx *class_context.ClassContext) string {
	if j.JavaType.String(funcCtx) == types.NewJavaPrimer(types.JavaBoolean).String(funcCtx) {
		if v, ok := j.Data.(int); ok {
			if v == 0 {
				return "false"
			}
			return "true"
		}
	}
	if j.JavaType.String(funcCtx) == "java.lang.String" || j.JavaType.String(funcCtx) == "String" {
		return JavaStringToLiteral(j.Data)
	} else {
		return fmt.Sprint(j.Data)
	}
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

type SlotValue struct {
	val     JavaValue
	TmpType types.JavaType
}

func (s *SlotValue) Type() types.JavaType {
	if s.val == nil {
		return s.TmpType
	}
	return s.val.Type()
}
func (s *SlotValue) String(funcCtx *class_context.ClassContext) string {
	if s.val == nil {
		return "empty slot value"
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
