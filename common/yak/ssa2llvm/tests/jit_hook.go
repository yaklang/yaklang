package tests

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// buffer stores the captured output values
var (
	buffer     []int64
	jitLock    sync.Mutex
	bufferLock sync.Mutex
	cbOnce     sync.Once
	printCB    uintptr
	mallocCB   uintptr
)

// yakInternalPrintInt is the Go function we want to expose.
func yakInternalPrintInt(n int64) {
	bufferLock.Lock()
	defer bufferLock.Unlock()
	buffer = append(buffer, n)
}

func yakInternalMalloc(size int64) unsafe.Pointer {
	data := make([]byte, size)
	return unsafe.Pointer(&data[0])
}

func getHookAddr() unsafe.Pointer {
	cbOnce.Do(func() {
		// Create C-callable function pointers once to avoid callback churn.
		printCB = purego.NewCallback(yakInternalPrintInt)
		mallocCB = purego.NewCallback(yakInternalMalloc)
	})
	return unsafe.Pointer(printCB)
}

func getMallocHookAddr() unsafe.Pointer {
	cbOnce.Do(func() {
		printCB = purego.NewCallback(yakInternalPrintInt)
		mallocCB = purego.NewCallback(yakInternalMalloc)
	})
	return unsafe.Pointer(mallocCB)
}

// SetupJITHook locks the JIT hook, resets the buffer, and returns a function
// that collects the results and unlocks the hook.
func SetupJITHook() func() []int64 {
	jitLock.Lock()

	bufferLock.Lock()
	buffer = make([]int64, 0, 1024)
	bufferLock.Unlock()

	return func() []int64 {
		defer jitLock.Unlock()

		bufferLock.Lock()
		defer bufferLock.Unlock()

		// Return a copy of the buffer
		res := make([]int64, len(buffer))
		copy(res, buffer)
		return res
	}
}
