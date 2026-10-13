//go:build linux && cgo && musl

package main

/*
#include <features.h>

#if defined(__GLIBC__) || defined(__UCLIBC__) || defined(__BIONIC__)
#error "The musl Node build requires musl headers and libraries"
#endif

// A build tag alone cannot establish which C library was linked. This symbol
// is supplied by musl itself; using glibc under the musl tag must fail to link.
extern const char __libc_version[];

static const char *legion_node_musl_version(void) {
	return __libc_version;
}
*/
import "C"

import "log"

func init() {
	log.Printf("component=legion-node event=libc c_library=musl version=%s", C.GoString(C.legion_node_musl_version()))
}
