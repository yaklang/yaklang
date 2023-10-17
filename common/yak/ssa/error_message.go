package ssa

import "fmt"

func BindingNotFound(v string) string {
	return fmt.Sprintf("call target closure binding variable not found: %s", v)
}
