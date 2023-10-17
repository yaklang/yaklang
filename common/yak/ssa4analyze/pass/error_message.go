package pass

import "fmt"

func BlockUnreachable() string {
	return "This block unreachable!"

}

func FunctionContReturnError() string {
	return "This function con't return error"
}

func ValueIsNull() string {
	return "This value is null"
}

func InvalidField(typ string) string {
	return fmt.Sprintf("Invalid operation: connot get member or index (variable of type %s)", typ)
}
