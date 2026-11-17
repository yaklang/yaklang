//go:build windows

package main

/*
#include <windows.h>
#include <stdlib.h>

typedef void (*yak_ctx_fn_t)(void*);

// GetProcAddress searches the module's export table, so it only finds symbols
// the executable or a loaded DLL exports. That matches how the ssa2llvm
// runtime registers its VM entry points.
static const char* yak_invoke_ctx_symbol(void* ctx, const char* name) {
	HMODULE mod = GetModuleHandleA(NULL);
	if (mod == NULL) {
		return "GetModuleHandle failed";
	}
	FARPROC sym = GetProcAddress(mod, name);
	if (sym == NULL) {
		return "symbol not found";
	}
	((yak_ctx_fn_t)sym)(ctx);
	return NULL;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func callCtxSymbol(ctx unsafe.Pointer, name string) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	if errMsg := C.yak_invoke_ctx_symbol(ctx, cName); errMsg != nil {
		return errors.New(C.GoString(errMsg))
	}
	return nil
}
