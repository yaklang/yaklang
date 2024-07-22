package decompiler

import "fmt"

type JavaValue interface {
	String() string
	Type() JavaType
}

var (
	_ JavaValue = &JavaRef{}
	_ JavaValue = &JavaArray{}
	_ JavaValue = &JavaLiteral{}
	_ JavaValue = &JavaClass{}
	_ JavaValue = &JavaClassMember{}
	_ JavaValue = &JavaExpression{}
)

type JavaRef struct {
	Id       int
	JavaType JavaType
}

func (j *JavaRef) Type() JavaType {
	return j.JavaType
}

func (j *JavaRef) String() string {
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

func (j *JavaArray) String() string {
	return fmt.Sprintf("%s[%d]", j.Class.String(), j.Length)
}

func NewJavaArray(class *JavaClass, length int) *JavaArray {
	return &JavaArray{
		Class:    class,
		Length:   length,
		JavaType: NewJavaArrayType(class.Type()),
	}
}

type JavaLiteral struct {
	JavaType JavaType
	Data     any
}

func (j *JavaLiteral) Type() JavaType {
	return j.JavaType
}

func (j *JavaLiteral) String() string {
	switch j.JavaType {
	case RT_REFERENCE:
		if v, ok := j.Data.(string); ok {
			return fmt.Sprintf(`"%s"`, v)
		} else {
			return fmt.Sprint(j.Data)
		}
	default:
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

func (j *JavaClassMember) String() string {
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
	Name     string
	JavaType JavaType
}

func (j *JavaClass) Type() JavaType {
	return j.JavaType
}

func (j *JavaClass) String() string {
	return fmt.Sprintf("%s", j.Name)
}
func NewJavaClass(typeName string, typ JavaType) *JavaClass {
	return &JavaClass{
		Name:     typeName,
		JavaType: typ,
	}
}
