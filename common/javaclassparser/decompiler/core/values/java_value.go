package values

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type JavaRef struct {
	Id       int
	StackVar JavaValue

	JavaType types.JavaType
}

func (j *JavaRef) Type() types.JavaType {
	return j.JavaType
}

func (j *JavaRef) String(funcCtx *class_context.FunctionContext) string {
	if j.StackVar != nil {
		return j.StackVar.String(funcCtx)
	}
	return fmt.Sprintf("var%d", j.Id)
}

func NewJavaRef(id int, typ types.JavaType) *JavaRef {
	return &JavaRef{
		Id:       id,
		JavaType: typ,
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

func (j *JavaArray) String(funcCtx *class_context.FunctionContext) string {
	return fmt.Sprintf("%s[%d]", j.Class.String(funcCtx), j.Length)
}

func NewJavaArray(class *types.JavaClass, length JavaValue) *JavaArray {
	return &JavaArray{
		Class:    class,
		Length:   length,
		JavaType: types.NewJavaArrayType(class, length),
	}
}

type JavaLiteral struct {
	JavaType types.JavaType
	Data     any
}

func (j *JavaLiteral) Type() types.JavaType {
	return j.JavaType
}

func (j *JavaLiteral) String(funcCtx *class_context.FunctionContext) string {
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

type JavaClassMember struct {
	Name        string
	Member      string
	Description string
	JavaType    *types.JavaFuncType
}

func (j *JavaClassMember) Type() types.JavaType {
	return j.JavaType
}

func (j *JavaClassMember) String(funcCtx *class_context.FunctionContext) string {
	if j.Name == funcCtx.ClassName {
		return j.Member
	}
	name := funcCtx.ShortTypeName(j.Name)
	return fmt.Sprintf("%s.%s", name, j.Member)
}
func NewJavaClassMember(typeName, member, desc string) *JavaClassMember {
	return &JavaClassMember{
		Name:        typeName,
		Member:      member,
		Description: desc,
	}
}

type RefMember struct {
	Member   string
	Id       int
	JavaType types.JavaType
}

func (j *RefMember) Type() types.JavaType {
	return j.JavaType
}

func NewRefMember(id int, member string, typ types.JavaType) *RefMember {
	return &RefMember{
		Member:   member,
		Id:       id,
		JavaType: typ,
	}
}

type JavaArrayMember struct {
	Ref   *JavaRef
	Index JavaValue
}

func (j *JavaArrayMember) Type() types.JavaType {
	return j.Ref.Type().(*types.JavaArrayType).JavaType
}
func (j *JavaArrayMember) String(funcCtx *class_context.FunctionContext) string {
	return fmt.Sprintf("var%d[%v]", j.Ref.Id, j.Index.String(funcCtx))
}

func NewJavaArrayMember(ref *JavaRef, index JavaValue) *JavaArrayMember {
	return &JavaArrayMember{
		Ref:   ref,
		Index: index,
	}
}

func (j *RefMember) String(funcCtx *class_context.FunctionContext) string {
	if j.Id == 0 {
		return j.Member
	}
	return fmt.Sprintf("var%d.%s", j.Id, j.Member)
}
