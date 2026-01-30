#ifndef LOCAL_GC_H
#define LOCAL_GC_H

#include <stddef.h>

// Manual declarations since gc.h is missing in the environment
extern void* GC_malloc(size_t size);
extern void GC_register_finalizer(void * obj, void (*fn)(void * obj, void * client_data), void * cd, void (*cd_proc)(void * cd, void * cd_data), void ** old_cd);
typedef void (*GC_finalization_proc)(void * obj, void * client_data);
extern void GC_gcollect(void);
extern size_t GC_get_heap_size(void);

#endif
