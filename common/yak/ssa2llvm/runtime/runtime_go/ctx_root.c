#include "local_gc.h"

#include <stdint.h>

typedef struct yak_ctx_root yak_ctx_root;

struct yak_ctx_root {
    yak_ctx_root* next;
    void* ctx;
};

static yak_ctx_root* yak_ctx_root_head = 0;

uintptr_t yak_ctx_root_add(void* ctx) {
    yak_ctx_root* node = (yak_ctx_root*)GC_malloc(sizeof(yak_ctx_root));
    if (!node) {
        return 0;
    }
    node->ctx = ctx;
    node->next = yak_ctx_root_head;
    yak_ctx_root_head = node;
    return (uintptr_t)node;
}

void* yak_ctx_root_get(uintptr_t handle) {
    yak_ctx_root* node = (yak_ctx_root*)handle;
    if (!node) {
        return 0;
    }
    return node->ctx;
}

void yak_ctx_root_remove(uintptr_t handle) {
    yak_ctx_root* target = (yak_ctx_root*)handle;
    if (!target) {
        return;
    }

    yak_ctx_root** cur = &yak_ctx_root_head;
    while (*cur) {
        if (*cur == target) {
            *cur = target->next;
            return;
        }
        cur = &((*cur)->next);
    }
}

