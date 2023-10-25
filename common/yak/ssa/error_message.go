package ssa

import "fmt"

func BindingNotFound(v string) string {
	return fmt.Sprintf("call target closure binding variable not found: %s", v)
}

func ExternLibError(name, key, want string) string {
	return fmt.Sprintf("ExternLib [%s] don't has [%s], maybe you meant %s ?", name, key, want)
}
func ContAssignExtern(name string) string {
	return fmt.Sprintf("cannot assign to  %s, this is extern-instance", name)
}
