package values

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type JavaRef struct {
	Id          *utils.VariableId
	StackVar    JavaValue
	CustomValue *CustomValue
	IsThis      bool
	Val         JavaValue
}

func (j *JavaRef) Type() types.JavaType {
	return j.Val.Type()
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

func NewJavaRef(id *utils.VariableId, val JavaValue) *JavaRef {
	return &JavaRef{
		Id:  id,
		Val: val,
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
		return fmt.Sprintf(`"%s"`, j.Data)
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
	Condition  JavaValue
	TrueValue  JavaValue
	FalseValue JavaValue
}

func (j *TernaryExpression) Type() types.JavaType {
	return j.TrueValue.Type()
}
func (j *TernaryExpression) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("(%s) ? (%s) : (%s)", j.Condition.String(funcCtx), j.TrueValue.String(funcCtx), j.FalseValue.String(funcCtx))
}

func NewTernaryExpression(condition, v1, v2 JavaValue) *TernaryExpression {
	return &TernaryExpression{
		Condition:  condition,
		TrueValue:  v1,
		FalseValue: v2,
	}
}

type SlotValue struct {
	Value   JavaValue
	TmpType types.JavaType
}

func (s *SlotValue) Type() types.JavaType {
	if s.Value == nil {
		return s.TmpType
	}
	return s.Value.Type()
}
func (s *SlotValue) String(funcCtx *class_context.ClassContext) string {
	return s.Value.String(funcCtx)
}

func NewSlotValue(val JavaValue) *SlotValue {
	return &SlotValue{
		Value: val,
	}
}
