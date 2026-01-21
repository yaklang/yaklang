package tests

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
    int64_t* data;
    int size;
    int capacity;
} PrintBuffer;

static PrintBuffer g_buffer = {NULL, 0, 0};

void hook_init() {
    if (g_buffer.data != NULL) {
        free(g_buffer.data);
    }
    g_buffer.data = (int64_t*)malloc(1024 * sizeof(int64_t));
    g_buffer.capacity = 1024;
    g_buffer.size = 0;
}

void test_print_hook(int64_t n) {
    if (g_buffer.size >= g_buffer.capacity) {
        int new_cap = g_buffer.capacity * 2;
        int64_t* new_data = (int64_t*)realloc(g_buffer.data, new_cap * sizeof(int64_t));
        if (new_data == NULL) return; // Allocation failed
        g_buffer.data = new_data;
        g_buffer.capacity = new_cap;
    }
    g_buffer.data[g_buffer.size++] = n;
}

int64_t get_buffered_val(int index) {
    if (index >= 0 && index < g_buffer.size) {
        return g_buffer.data[index];
    }
    return 0;
}

int get_buffer_size() {
    return g_buffer.size;
}

void* get_hook_addr() {
    return (void*)test_print_hook;
}
*/
import "C"
import (
	"sync"
	"unsafe"
)

var jitLock sync.Mutex

func getHookAddr() unsafe.Pointer {
	return unsafe.Pointer(C.get_hook_addr())
}

// SetupJITHook locks the JIT hook, resets the buffer, and returns a function
// that collects the results and unlocks the hook.
func SetupJITHook() func() []int64 {
	jitLock.Lock()
	C.hook_init()

	return func() []int64 {
		defer jitLock.Unlock()

		size := int(C.get_buffer_size())
		if size == 0 {
			return nil
		}

		res := make([]int64, size)
		for i := 0; i < size; i++ {
			res[i] = int64(C.get_buffered_val(C.int(i)))
		}
		return res
	}
}
