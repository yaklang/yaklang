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
