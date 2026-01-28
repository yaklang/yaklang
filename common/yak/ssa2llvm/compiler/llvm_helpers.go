package compiler

/*
#include <llvm-c/Core.h>
#include <stdlib.h>
*/
import "C"
import (
	"unsafe"

	"github.com/yaklang/go-llvm"
)

func buildGlobalStringPtr(b llvm.Builder, str, name string) llvm.Value {
	cstr := C.CString(str)
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cstr))
	defer C.free(unsafe.Pointer(cname))
	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	val := C.LLVMBuildGlobalStringPtr(cb, cstr, cname)
	return valueFromC(val)
}

func buildSExt(b llvm.Builder, val llvm.Value, dest llvm.Type, name string) llvm.Value {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	cv := (C.LLVMValueRef)(unsafe.Pointer(val.C))
	ct := (C.LLVMTypeRef)(unsafe.Pointer(dest.C))
	res := C.LLVMBuildSExt(cb, cv, ct, cname)
	return valueFromC(res)
}

func buildZExt(b llvm.Builder, val llvm.Value, dest llvm.Type, name string) llvm.Value {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	cv := (C.LLVMValueRef)(unsafe.Pointer(val.C))
	ct := (C.LLVMTypeRef)(unsafe.Pointer(dest.C))
	res := C.LLVMBuildZExt(cb, cv, ct, cname)
	return valueFromC(res)
}

func buildTrunc(b llvm.Builder, val llvm.Value, dest llvm.Type, name string) llvm.Value {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	cb := (C.LLVMBuilderRef)(unsafe.Pointer(b.C))
	cv := (C.LLVMValueRef)(unsafe.Pointer(val.C))
	ct := (C.LLVMTypeRef)(unsafe.Pointer(dest.C))
	res := C.LLVMBuildTrunc(cb, cv, ct, cname)
	return valueFromC(res)
}

func valueFromC(v C.LLVMValueRef) llvm.Value {
	var ret llvm.Value
	*(*unsafe.Pointer)(unsafe.Pointer(&ret)) = unsafe.Pointer(v)
	return ret
}
