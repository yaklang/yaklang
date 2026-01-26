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
)

// yakInternalPrintInt is the Go function we want to expose.
func yakInternalPrintInt(n int64) {
	bufferLock.Lock()
	defer bufferLock.Unlock()
	buffer = append(buffer, n)
}

func getHookAddr() unsafe.Pointer {
	// Create a C-callable function pointer from the Go function using purego.
	// This avoids using cgo's //export mechanism and C preambles.
	cb := purego.NewCallback(yakInternalPrintInt)
	return unsafe.Pointer(cb)
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
