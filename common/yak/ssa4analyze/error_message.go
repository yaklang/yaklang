package ssa4analyze

import (
	"fmt"
)

func ErrorUnhandled() string {
	return "Error Unhandled"
}

func ValueUndefined(v string) string {
	return fmt.Sprintf("value undefined:%s", v)
}

func NotEnoughArgument(funName string, have, want string) string {
	return fmt.Sprintf(
		`not enough arguments in call %s have (%s) want (%s)`,
		funName, have, want,
	)
}

func CallAssignmentMismatch(leftLen, rightLen int) string {
	return fmt.Sprintf(
		"function call assignment mismatch: left: %d variable but right return %d values",
		leftLen, rightLen,
	)
}
