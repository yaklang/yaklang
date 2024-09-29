package core

func NewStackType(computationCategory int, close bool, name string) *StackType {
	return &StackType{
		ComputationCategory: computationCategory,
		Closed:              close,
		Name:                name,
	}
}
func NewRawJavaType(name string, suggestName string, stackType *StackType, usableType bool, boxedName string, number, object bool, min, max int) *RawJavaType {
	return &RawJavaType{
		Name:             name,
		SuggestedVarName: suggestName,
		StackType:        stackType,
		UsableType:       usableType,
		BoxedName:        boxedName,
		IsNumber:         number,
		IsObject:         object,
		IntMin:           min,
		IntMax:           max,
	}
}

var (
	ST_INT                = NewStackType(1, true, "int")
	ST_FLOAT              = NewStackType(1, true, "float")
	ST_REFERENCE          = NewStackType(1, false, "reference")
	ST_RETURNADDRESS      = NewStackType(1, false, "returnaddress")
	ST_RETURNADDRESSORREF = NewStackType(1, false, "returnaddress or ref")
	ST_LONG               = NewStackType(2, true, "long")
	ST_DOUBLE             = NewStackType(2, true, "double")
	ST_VOID               = NewStackType(0, false, "void")
)
var (
	RT_BOOLEAN            = NewRawJavaType("boolean", "bl", ST_INT, true, "java.lang.Boolean", false, false, 0, 1)
	RT_BYTE               = NewRawJavaType("byte", "by", ST_INT, true, "java.lang.Byte", true, false, -128, 127)
	RT_CHAR               = NewRawJavaType("char", "c", ST_INT, true, "java.lang.Character", false, false, 0, 65535)
	RT_SHORT              = NewRawJavaType("short", "s", ST_INT, true, "java.lang.Short", true, false, -32768, 32767)
	RT_INT                = NewRawJavaType("int", "n", ST_INT, true, "java.lang.Integer", true, false, -2147483648, 2147483647)
	RT_LONG               = NewRawJavaType("long", "l", ST_LONG, true, "java.lang.Long", true, false, 2147483647, -2147483648)
	RT_FLOAT              = NewRawJavaType("float", "f", ST_FLOAT, true, "java.lang.Float", true, false, 2147483647, -2147483648)
	RT_DOUBLE             = NewRawJavaType("double", "d", ST_DOUBLE, true, "java.lang.Double", true, false, 2147483647, -2147483648)
	RT_VOID               = NewRawJavaType("void", "", ST_VOID, false, "", false, false, 2147483647, -2147483648)
	RT_REFERENCE          = NewRawJavaType("reference", "", ST_REFERENCE, false, "", false, true, 2147483647, -2147483648)
	RT_RETURNADDRESS      = NewRawJavaType("returnaddress", "", ST_RETURNADDRESS, false, "", false, true, 2147483647, -2147483648)
	RT_RETURNADDRESSORREF = NewRawJavaType("returnaddress or ref", "", ST_RETURNADDRESSORREF, false, "", false, true, 2147483647, -2147483648)
	RT_NULL               = NewRawJavaType("null", "", ST_REFERENCE, false, "", false, true, 2147483647, -2147483648)
)
