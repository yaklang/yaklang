package ssa

import "fmt"

func BindingNotFound(v string, r *Range) string {
	return fmt.Sprintf("The closure function expects to capture variable [%s], but it was not found at the calling location [%s--%s].", v, r.GetStart(), r.GetEnd())
}

func BindingNotFoundInCall(v string) string {
	return fmt.Sprintf("The closure function expects to capture variable [%s], but it was not found at the call", v)
}

func FreeValueNotMember(name, key string, r *Range) string {
	return fmt.Sprintf(
		"The FreeValue %s unable to access the member with name or index {%s} at the calling location [%s--%s].",
		name, key, r.GetStart(), r.GetEnd(),
	)
}

func FreeValueNotMemberInCall(name, key string) string {
	return fmt.Sprintf(
		"The value %s unable to access the member with name or index {%s} at the call.",
		name, key,
	)
}

func ExternFieldError(instance, name, key, want string) string {
	return fmt.Sprintf("Extern%s [%s] don't has [%s], maybe you meant %s ?", instance, name, key, want)
}

func ContAssignExtern(name string) string {
	return fmt.Sprintf("cannot assign to  %s, this is extern-instance", name)
}

func NoCheckMustInFirst() string {
	return "@ssa-nocheck must be the first line in the file"
}

func ValueUndefined(v string) string {
	return fmt.Sprintf("Value undefined:%s", v)
}

func ValueIsNull() string {
	return "This value is null"
}

func FunctionContReturnError() string {
	return "This function cannot return error"
}

func GenericTypeError(symbol, generic, want, got Type) string {
	symbolStr, genericStr := symbol.String(), generic.String()
	if symbolStr != "" && symbolStr == genericStr {
		return fmt.Sprintf("%s should be %s, but got %s", symbolStr, want, got)
	} else {
		return fmt.Sprintf("%s of %s should be %s, but got %s", symbol, generic, want, got)
	}
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
