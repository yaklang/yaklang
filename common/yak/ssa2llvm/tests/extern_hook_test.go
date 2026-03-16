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

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

func getObject(x int64) int64 {
	return x * 3
}
` + yakStdCallStubGoCode() + `

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
