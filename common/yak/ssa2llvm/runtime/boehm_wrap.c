/* yak_boehm_wrap.c — linked into every AOT binary with
 * --wrap=GC_malloc --wrap=GC_gcollect.
 *
 * Boehm aborts with "Collecting from unknown thread" when a collection
 * starts on a thread that never went through libgc's pthread_create.
 * The Go runtime inside libyak.a creates its own threads with clone, so
 * the collector never sets GC_need_to_lock. Until that flag is set,
 * GC_malloc updates the global freelist with no lock. Two goroutines
 * can be handed the same cell (a later iteration's integer shows up
 * twice), and the exit GC_gcollect walks the corrupted freelist and
 * faults in GC_find_header (addr 0x20).
 *
 * Shadows reachable only from Go stacks are invisible to Boehm, so
 * collections stay explicit: yak_runtime_gc enables, collects, and
 * disables. GC_disable nests, and that call site enables exactly once,
 * so automatic collection has to be turned off exactly once before the
 * first allocation. The same once sets GC_need_to_lock. Allocator
 * threads stay unregistered: registering them would make the explicit
 * collect try to suspend Go, which deadlocks. A later explicit
 * GC_gcollect may still run on a thread libgc does not know; register
 * that collector thread first.
 */

#include <pthread.h>
#include <stddef.h>

struct GC_stack_base {
    void* mem_base;
};

extern void GC_disable(void);
extern int GC_is_init_called(void);
extern int GC_thread_is_registered(void);
extern void GC_allow_register_threads(void);
extern int GC_get_stack_base(struct GC_stack_base* sb);
extern int GC_register_my_thread(const struct GC_stack_base* sb);

extern void* __real_GC_malloc(size_t size);
extern void __real_GC_gcollect(void);
/* GC_bool is int. Public GC_allow_register_threads also starts marker
 * threads and installs suspend signals; Go already owns those signals.
 * The flag alone is what makes LOCK() take the allocation lock. */
extern int GC_need_to_lock;

#define YAK_GC_SUCCESS 0
#define YAK_GC_DUPLICATE 1

static pthread_once_t yak_boehm_disable_once = PTHREAD_ONCE_INIT;

static void yak_boehm_disable_auto(void) {
    __atomic_store_n(&GC_need_to_lock, 1, __ATOMIC_RELEASE);
    GC_disable();
}

void* __wrap_GC_malloc(size_t size) {
    pthread_once(&yak_boehm_disable_once, yak_boehm_disable_auto);
    return __real_GC_malloc(size);
}

/* Register the collecting thread when libgc was already initialized on
 * some other thread. Before init, the real GC_gcollect registers the
 * caller itself. Returns 0 when this thread still cannot collect safely,
 * so the caller skips GC_gcollect instead of hitting Boehm's abort. */
static int yak_boehm_register_collector(void) {
    struct GC_stack_base sb;
    int rc;

    if (!GC_is_init_called() || GC_thread_is_registered()) {
        return 1;
    }
    GC_allow_register_threads();
    if (GC_get_stack_base(&sb) != YAK_GC_SUCCESS || sb.mem_base == 0) {
        return 0;
    }
    rc = GC_register_my_thread(&sb);
    if (rc == YAK_GC_SUCCESS || rc == YAK_GC_DUPLICATE || GC_thread_is_registered()) {
        return 1;
    }
    return 0;
}

void __wrap_GC_gcollect(void) {
    if (!yak_boehm_register_collector()) {
        return;
    }
    __real_GC_gcollect();
}
