#include <stdint.h>

typedef void (*yak_callable_t)(void*);

void yak_invoke_callable(uintptr_t fn, void* ctx) {
    ((yak_callable_t)fn)(ctx);
}

static int yak_test_closure_hits;

static void yak_test_closure(void* ctx) {
    (void)ctx;
    yak_test_closure_hits++;
}

uintptr_t yak_test_closure_addr(void) {
    return (uintptr_t)(void*)yak_test_closure;
}

int yak_test_closure_hit_count(void) {
    return yak_test_closure_hits;
}

