package tests

import (
	"testing"
)

func TestBinary_CustomExternBindingWithLinkedObject(t *testing.T) {
	code := `
func main() {
    v = getObject(7)
    println(v)
}
`
	goCode := `
package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func getObject(x int64) int64 {
	return x * 3
}

// Minimal dispatcher stub for tests that skip linking the full yak runtime.
// We only need builtin printing here.
func yak_std_call(funcID int64, argc int64, argv *C.uint64_t) int64 {
	if argc <= 0 || argv == nil {
		if funcID == 7 {
			fmt.Println()
		}
		return 0
	}
	args := unsafe.Slice((*uint64)(unsafe.Pointer(argv)), int(argc))
	switch funcID {
	case 5: // print
		for _, a := range args {
			fmt.Print(int64(a))
		}
	case 7: // println
		for i, a := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(int64(a))
		}
		fmt.Println()
	default:
		// ignore
	}
	return 0
}

func yak_internal_malloc(size int64) uintptr {
	if size <= 0 {
		size = 1
	}
	return uintptr(C.malloc(C.size_t(size)))
}

func yak_runtime_gc() {}
`

	output := runBinaryWithEnv(t, code, "main", nil, withRuntimeCode(goCode))
	if output != "21\n" {
		t.Fatalf("expected output 21, got %q", output)
	}
}
