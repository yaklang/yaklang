package decompiler

const (
	Integer    = "integer"
	Long       = "long"
	Double     = "double"
	Float      = "float"
	String     = "string"
	NullObject = "null"
	Class      = "class"
	//MethodHandle,  // Only used for invokedynamic arguments
	MethodType = "method"
)

type JavaType interface {
	GetStackType() *StackType
}
type JavaArrayType struct {
	JavaType JavaType
}

func NewJavaArrayType(typ JavaType) *JavaArrayType {
	return &JavaArrayType{
		JavaType: typ,
	}
}
func (j *JavaArrayType) GetStackType() *StackType {
	return RT_REFERENCE.GetStackType()
}
