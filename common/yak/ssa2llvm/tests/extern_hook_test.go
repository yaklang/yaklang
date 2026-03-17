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

	func getObject(ctx unsafe.Pointer) {
		const (
			wordArgc    = 5
			wordRet     = 6
			headerWords = 10
		)
		if ctx == nil {
			return
		}
		argc := int(int64(*(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(wordArgc)*8))))
		var x int64
		if argc > 0 {
			arg0 := *(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(headerWords)*8))
			x = int64(arg0)
		}
		*(*uint64)(unsafe.Pointer(uintptr(ctx) + uintptr(wordRet)*8)) = uint64(x * 3)
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
