package ssa

import "fmt"

func BindingNotFound(v string) string {
	return fmt.Sprintf("call target closure binding variable not found: %s", v)
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
