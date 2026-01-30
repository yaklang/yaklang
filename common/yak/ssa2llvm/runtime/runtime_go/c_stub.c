#include "local_gc.h"
#include "_cgo_export.h"

// Proxy function to call Go finalizer from C
// This matches GC_finalization_proc signature: void (*)(void *obj, void *client_data)
void yak_finalizer_proxy(void* obj, void* client_data) {
    // Call the exported Go function
    yak_internal_release_shadow(obj);
}
