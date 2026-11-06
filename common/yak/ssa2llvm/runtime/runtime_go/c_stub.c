#include "local_gc.h"
#include "_cgo_export.h"

// Proxy function to call Go finalizer from C
// This matches GC_finalization_proc signature: void (*)(void *obj, void *client_data)
void yak_finalizer_proxy(void* obj, void* client_data) {
    // Call the exported Go function
    yak_internal_release_shadow(obj);
}

// GC control wrappers: automatic Boehm collections cannot see shadow
// references held on Go's cgo callback stacks, so collections stay explicit
// (yak_runtime_gc enables, collects, and disables again).
void yak_gc_disable(void) {
    GC_disable();
}

void yak_gc_enable(void) {
    GC_enable();
}
