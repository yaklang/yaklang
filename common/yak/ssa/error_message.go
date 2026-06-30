package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils/memedit"
)

// closureCaptureHint is shared by the closure binding errors so humans and AI agents can
// fix the issue straight from the message: a top-level named function does NOT implicitly
// capture outer local variables.
const closureCaptureHint = " hint: a top-level named function (func f(){...}) does NOT implicitly " +
	"capture outer local variables; if its body uses an outer variable like [%s], pass it in as a " +
	"parameter (func f(%s){...} then f(%s)), or use an inline closure `go func(){ ... }()` / " +
	"`go func(){ f() }()` which captures the surrounding scope."

func BindingNotFound(v string, r *memedit.Range) string {
	return fmt.Sprintf("The closure function expects to capture variable [%s], but it was not found at the calling location [%s--%s].", v, r.GetStart(), r.GetEnd()) +
		fmt.Sprintf(closureCaptureHint, v, v, v)
}

func BindingNotFoundInCall(v string) string {
	return fmt.Sprintf("The closure function expects to capture variable [%s], but it was not found at the call", v) +
		fmt.Sprintf(closureCaptureHint, v, v, v)
}

func ValueNotMember(op Opcode, name, key string, r *memedit.Range) string {
	return fmt.Sprintf(
		"The %s %s unable to access the member with name or index {%s} at the calling location [%s--%s].",
		SSAOpcode2Name[op], name, key, r.GetStart(), r.GetEnd(),
	)
}

func ValueNotMemberInCall(name, key string) string {
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
		"The function call returns (%s) type, but %d variables on the left side. "+
			"fix: match the number of variables to the function's real return values; if it returns one value, "+
			"use a single variable. note: Yaklang has no Go comma-ok form, so `v, ok := f()` only works when f "+
			"actually returns 2 values.",
		right, left,
	)
}

func CallAssignmentMismatchDropError(left int, right string) string {
	return fmt.Sprintf(
		"The function call with ~ returns (%s) type, but %d variables on the left side. "+
			"note: the wavy `~` already drops the trailing error, so do NOT add an extra `err` variable for it; "+
			"match the remaining variable count to the non-error return values.",
		right, left,
	)
}

func PhiEdgeLengthMisMatch() string {
	return "Phi edges length < 2"
}

func InvalidField(typ, key string) string {
	return fmt.Sprintf("Invalid operation: unable to access the member or index of variable of type {%s} with name or index {%s}.", typ, key)
}
