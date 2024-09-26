package decompiler

import (
	"fmt"
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
	_ JavaValue = &FunctionCallExpression{}
	_ JavaValue = &RefMember{}
	_ JavaValue = &JavaCompare{}
	_ JavaValue = &javaNull{}
)

type JavaCompare struct {
	JavaValue1, JavaValue2 JavaValue
}

func (j *JavaCompare) Type() JavaType {
	return JavaBoolean
}

func (j *JavaCompare) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("%s compare %s", j.JavaValue1.String(funcCtx), j.JavaValue2.String(funcCtx))
}

func NewJavaCompare(v1, v2 JavaValue) *JavaCompare {
	return &JavaCompare{
		JavaValue1: v1,
		JavaValue2: v2,
	}
}

type JavaRef struct {
	Id       int
	StackVar JavaValue

	JavaType JavaType
}

func (j *JavaRef) Type() JavaType {
	return j.JavaType
}

func (j *JavaRef) String(funcCtx *FunctionContext) string {
	if j.StackVar != nil {
		return j.StackVar.String(funcCtx)
	}
	return fmt.Sprintf("var%d", j.Id)
}

func NewJavaRef(id int, typ JavaType) *JavaRef {
	return &JavaRef{
		Id:       id,
		JavaType: typ,
	}
}

type LambdaFuncRef struct {
	Id           int
	JavaType     JavaType
	LambdaRender func(funcCtx *FunctionContext) string
}

func (j *LambdaFuncRef) Type() JavaType {
	return j.JavaType
}

func (j *LambdaFuncRef) String(funcCtx *FunctionContext) string {
	if j.LambdaRender != nil {
		return j.LambdaRender(funcCtx)
	}
	return fmt.Sprintf("getLambda(%d)", j.Id)
}

func NewLambdaFuncRef(id int, typ JavaType) *LambdaFuncRef {
	return &LambdaFuncRef{
		Id:       id,
		JavaType: typ,
	}
}

type JavaArray struct {
	Class    *JavaClass
	Length   JavaValue
	JavaType JavaType
}

func (j *JavaArray) Type() JavaType {
	return j.JavaType
}

func (j *JavaArray) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("%s[%d]", j.Class.String(funcCtx), j.Length)
}

func NewJavaArray(class *JavaClass, length JavaValue) *JavaArray {
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
	if j.Name == funcCtx.ClassName {
		return j.Member
	}
	name := GetShortName(funcCtx, j.Name)
	return fmt.Sprintf("%s.%s", name, j.Member)
}
func NewJavaClassMember(typeName, member, desc string) *JavaClassMember {
	typ, _, _ := parseType(desc)
	return &JavaClassMember{
		Name:        typeName,
		Member:      member,
		Description: desc,
		JavaType:    typ,
	}
}

type RefMember struct {
	Member   string
	Id       int
	JavaType JavaType
}

func (j *RefMember) Type() JavaType {
	return j.JavaType
}

func NewRefMember(id int, member string, typ JavaType) *RefMember {
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

func (j *JavaArrayMember) Type() JavaType {
	return j.Ref.Type().(*JavaArrayType).JavaType
}
func (j *JavaArrayMember) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("var%d[%v]", j.Ref.Id, j.Index.String(funcCtx))
}

func NewJavaArrayMember(ref *JavaRef, index JavaValue) *JavaArrayMember {
	return &JavaArrayMember{
		Ref:   ref,
		Index: index,
	}
}

func (j *RefMember) String(funcCtx *FunctionContext) string {
	if j.Id == 0 {
		return j.Member
	}
	return fmt.Sprintf("var%d.%s", j.Id, j.Member)
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
	name := GetShortName(funcCtx, j.Name)
	return fmt.Sprintf("%s", name)
}
func NewJavaClass(typeName string) *JavaClass {
	return &JavaClass{
		Name: typeName,
	}
}

const TypeCaseByte = "byte"
const TypeCaseLong = "long"
const TypeCaseShort = "short"
const TypeCaseInt = "int"
const TypeCaseChar = "char"
const TypeCaseFloat = "float"
const TypeCaseDouble = "double"

type VirtualFunctionCall struct {
	Name      string
	Arguments []JavaValue
	JavaType  JavaType
}

func (v *VirtualFunctionCall) Type() JavaType {
	return v.JavaType
}
func (v *VirtualFunctionCall) String(funcCtx *FunctionContext) string {
	args := []string{}
	for _, arg := range v.Arguments {
		args = append(args, arg.String(funcCtx))
	}
	return fmt.Sprintf("%s(%s)", v.Name, strings.Join(args, ","))
}
func NewVirtualFunctionCall(name string, arguemnts []JavaValue, javaType JavaType) *VirtualFunctionCall {
	return &VirtualFunctionCall{
		Name:      name,
		Arguments: arguemnts,
		JavaType:  javaType,
	}
}

type CustomValue struct {
	StringFunc func(funcCtx *FunctionContext) string
	TypeFunc   func() JavaType
}

func (v *CustomValue) Type() JavaType {
	return v.TypeFunc()
}
func (v *CustomValue) String(funcCtx *FunctionContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomValue(stringFun func(funcCtx *FunctionContext) string, typeFunc func() JavaType) *CustomValue {
	return &CustomValue{
		StringFunc: stringFun,
		TypeFunc:   typeFunc,
	}
}

type CustomStatement struct {
	Name       string
	Info any
	StringFunc func(funcCtx *FunctionContext) string
}

func (v *CustomStatement) String(funcCtx *FunctionContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomStatement(stringFun func(funcCtx *FunctionContext) string) *CustomStatement {
	return &CustomStatement{
		StringFunc: stringFun,
	}
}

type LazyJavaValue struct {
	GetJavaValue func() JavaValue
}

func NewLazyJavaValue(f func() JavaValue) *LazyJavaValue {
	return &LazyJavaValue{
		GetJavaValue: f,
	}
}
func (l *LazyJavaValue) Type() JavaType {
	return l.GetJavaValue().Type()
}

func (l *LazyJavaValue) String(funcCtx *FunctionContext) string {
	return l.GetJavaValue().String(funcCtx)
}
