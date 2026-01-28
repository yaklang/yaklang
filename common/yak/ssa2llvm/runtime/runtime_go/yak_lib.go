package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
)

func main() {}

//export yak_internal_print_int
func yak_internal_print_int(n int64) {
	fmt.Println(n)
}

//export yak_internal_malloc
func yak_internal_malloc(size int64) uintptr {
	return uintptr(C.malloc(C.size_t(size)))
}
