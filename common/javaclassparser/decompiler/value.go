package decompiler

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type JavaValue interface {
	String(funcCtx *FunctionContext) string
	Type() JavaType
}

var (
	_ JavaValue = &JavaRef{}
	_ JavaValue = &JavaArray{}
	_ JavaValue = &JavaLiteral{}
	_ JavaValue = &JavaClass{}
	_ JavaValue = &JavaClassMember{}
	_ JavaValue = &JavaExpression{}
	_ JavaValue = &NewExpression{}
)

type JavaRef struct {
	Id       int
	JavaType JavaType
}

func (j *JavaRef) Type() JavaType {
	return j.JavaType
}

func (j *JavaRef) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("var%d", j.Id)
}

func NewJavaRef(id int, typ JavaType) *JavaRef {
	return &JavaRef{
		Id:       id,
		JavaType: typ,
	}
}

type JavaArray struct {
	Class    *JavaClass
	Length   int
	JavaType JavaType
}

func (j *JavaArray) Type() JavaType {
	return j.JavaType
}

func (j *JavaArray) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("%s[%d]", j.Class.String(funcCtx), j.Length)
}

func NewJavaArray(class *JavaClass, length int) *JavaArray {
	return &JavaArray{
		Class:    class,
		Length:   length,
		JavaType: NewJavaArrayType(class, length),
	}
}

type JavaLiteral struct {
	JavaType JavaType
	Data     any
}

func (j *JavaLiteral) Type() JavaType {
	return j.JavaType
}

func (j *JavaLiteral) String(funcCtx *FunctionContext) string {
	if j.JavaType.String(funcCtx) == "java.lang.String" || j.JavaType.String(funcCtx) == "String" {
		return fmt.Sprintf(`"%s"`, j.Data)
	} else {
		return fmt.Sprint(j.Data)
	}
}

func NewJavaLiteral(data any, typ JavaType) *JavaLiteral {
	return &JavaLiteral{
		JavaType: typ,
		Data:     data,
	}
}

type JavaClassMember struct {
	Name        string
	Member      string
	Description string
	JavaType    JavaType
}

func (j *JavaClassMember) Type() JavaType {
	return j.JavaType
}

func (j *JavaClassMember) String(funcCtx *FunctionContext) string {
	for _, lib := range funcCtx.BuildInLibs {
		pkg, className := SplitPackageClassName(lib)
		fpkg, fclassName := SplitPackageClassName(j.Name)
		if fpkg == pkg && (className == "*" || fclassName == className) {
			return fmt.Sprintf("%s.%s", fclassName, j.Member)
		}
	}
	return fmt.Sprintf("%s.%s", j.Name, j.Member)
}
func NewJavaClassMember(typeName, member, desc string, typ JavaType) *JavaClassMember {
	return &JavaClassMember{
		Name:        typeName,
		Member:      member,
		Description: desc,
		JavaType:    typ,
	}
}

type JavaClass struct {
	Name string
	JavaType
}

func (j *JavaClass) IsJavaType() {

}
func (j *JavaClass) Type() JavaType {
	return j
}

func (j *JavaClass) String(funcCtx *FunctionContext) string {
	if j.Name == funcCtx.ClassName {
		splits := strings.Split(j.Name, ".")
		if len(splits) > 0 {
			return utils.GetLastElement(splits)
		}
		return ""
	}
	return fmt.Sprintf("%s", j.Name)
}
func NewJavaClass(typeName string) *JavaClass {
	return &JavaClass{
		Name: typeName,
	}
}
