package ssa

import "fmt"

func BindingNotFound(v string, r *Range) string {
	return fmt.Sprintf("The closure function expects to capture variable [%s], but it was not found at the calling location [%s--%s].", v, r.Start, r.End)
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
