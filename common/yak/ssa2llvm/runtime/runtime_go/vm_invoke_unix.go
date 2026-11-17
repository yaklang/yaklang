//go:build !windows

package main

/*
#cgo linux CFLAGS: -D_GNU_SOURCE
#cgo linux LDFLAGS: -ldl
#include <stdlib.h>
#include <dlfcn.h>

typedef void (*yak_ctx_fn_t)(void*);

static const char* yak_invoke_ctx_symbol(void* ctx, const char* name) {
	dlerror();
	void* sym = dlsym(RTLD_DEFAULT, name);
	const char* err = dlerror();
	if (err != NULL) {
		return err;
	}
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
