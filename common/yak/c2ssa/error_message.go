package c2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "C"

func Unreachable() string {
	return "unreachable"
}

func ToDo() string {
	return "todo"
}

func TypeMismatch(t, t2 string) string {
	return fmt.Sprintf("Type %s and type %s do not match", t, t2)
}

func TypeLenMismatch() string {
	return "Type number does not match"
}
