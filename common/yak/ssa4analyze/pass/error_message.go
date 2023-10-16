package pass

import "fmt"

func BlockUnreachable() string {
	return "this block unreachable!"

}

func FunctionContReturnError() string {
	return "this function con't return error"
}

func ValueIsNull() string {
	return "this value is null"
}

func InvalidField(typ string) string {
	return fmt.Sprintf("Invalid operation: connot get member or index (variable of type %s)", typ)
}
