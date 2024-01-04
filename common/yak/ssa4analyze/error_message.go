package ssa4analyze

import (
	"fmt"
)

func ErrorUnhandled() string {
	return "Error Unhandled "
}
func ErrorUnhandledWithType(typ string) string {
	return fmt.Sprintf("The value is (%s) type, has unhandled error", typ)
}

func ValueUndefined(v string) string {
	return fmt.Sprintf("Value undefined:%s", v)
}

func ConditionIsConst(control string) string {
	return fmt.Sprintf("The %s condition is constant", control)

}

func ArgumentTypeError(index int, valueType, wantType, funName string) string {
	return fmt.Sprintf(
		`The No.%d argument (%s), cannot use as (%s) in call %s`,
		index, valueType, wantType, funName,
	)
}

func NotEnoughArgument(funName string, have, want string) string {
	return fmt.Sprintf(
		`Not enough arguments in call %s have (%s) want (%s)`,
		funName, have, want,
	)
}

func CallAssignmentMismatch(left int, right string) string {
	return fmt.Sprintf(
		"The function call returns (%s) type, but %d variables on the left side. ",
		right, left,
	)
}

func CallAssignmentMismatchDropError(left int, right string) string {
	return fmt.Sprintf(
		"The function call with ~ returns (%s) type, but %d variables on the left side. ",
		right, left,
	)
}

func BlockUnreachable() string {
	return "This block unreachable!"

}

func FunctionContReturnError() string {
	return "This function cannot return error"
}

func ValueIsNull() string {
	return "This value is null"
}

func InvalidField(typ, key string) string {
	return fmt.Sprintf("Invalid operation: unable to access the member or index of variable of type {%s} with name or index {%s}.", typ, key)
}

func InvalidChanType(typ string) string {
	return fmt.Sprintf("iteration (variable of type chan %s) permits only one right variable", typ)

}
