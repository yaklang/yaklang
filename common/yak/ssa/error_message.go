package ssa

import "fmt"

func BindingNotFound(v Value) string {
	return fmt.Sprintf("call target closure binding variable not found: %s", v)
}
