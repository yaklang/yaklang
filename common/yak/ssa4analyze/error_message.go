package ssa4analyze

import (
	"fmt"
)

func FreeValueUndefine(name string) string {
	return fmt.Sprintf("Can't find definition of this variable %s both inside and outside the function.", name)
}

func ErrorUnhandled() string {
	return "Error Unhandled "
}
func ErrorUnhandledWithType(typ string) string {
	return fmt.Sprintf("The value is (%s) type, has unhandled error", typ)
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

func BlockUnreachable() string {
	return "This block unreachable!"
}
