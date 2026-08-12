#define _GNU_SOURCE

#include "local_gc.h"

#include <elf.h>
#include <link.h>
#include <stdint.h>

typedef struct yak_ctx_root yak_ctx_root;

struct yak_ctx_root {
    yak_ctx_root* next;
    void* ctx;
};

static yak_ctx_root* yak_ctx_root_head = 0;

/* In a fully static binary the weak dl_iterate_phdr from libc.a is not
 * pulled in, so Boehm GC's GC_register_main_static_data() sees it as NULL
 * and falls back to probing the data segment with a temporary SIGSEGV
 * handler.  That conflicts with Go's gsignal stack (SA_ONSTACK) and aborts
 * the runtime.  Provide a strong dl_iterate_phdr that reports the main
 * executable program headers; bdwgc then registers writable PT_LOAD segments
 * as roots through its normal dl_iterate_phdr path, without any probing. */
extern const ElfW(Ehdr) __ehdr_start;

int dl_iterate_phdr(int (*callback)(struct dl_phdr_info*, size_t, void*),
                    void* data) {
    const ElfW(Ehdr)* ehdr = &__ehdr_start;
    if (ehdr->e_ident[EI_MAG0] != ELFMAG0
        || ehdr->e_ident[EI_MAG1] != ELFMAG1
        || ehdr->e_ident[EI_MAG2] != ELFMAG2
        || ehdr->e_ident[EI_MAG3] != ELFMAG3
        || ehdr->e_phnum == 0) {
        return -1;
    }
    struct dl_phdr_info info;
    /* For ET_EXEC (the static AOT output) p_vaddr is already absolute and
     * the load bias is zero; for ET_DYN/PIE it is the image base. */
    info.dlpi_addr = ehdr->e_type == ET_DYN ? (ElfW(Addr))ehdr : 0;
    info.dlpi_name = "";
    info.dlpi_phdr = (const ElfW(Phdr)*)((const char*)ehdr + ehdr->e_phoff);
    info.dlpi_phnum = ehdr->e_phnum;
    info.dlpi_adds = 0;
    info.dlpi_tls_modid = 0;
    info.dlpi_tls_data = NULL;
    return callback(&info, sizeof(info), data);
}

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
