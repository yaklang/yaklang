#include <stdint.h>

typedef void (*yak_callable_t)(void*);

void yak_invoke_callable(uintptr_t fn, void* ctx) {
    ((yak_callable_t)fn)(ctx);
}

