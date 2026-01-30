package tests

import (
	"testing"
)

func TestInterop_LoopGC(t *testing.T) {
	// This test verifies that objects allocated in a tight loop are garbage collected.
	// We rely on Boehm GC (libgc) scanning the LLVM stack (C stack).
	// If GC works, we should see "Releasing shadow" or "Releasing handle" logs
	// while the loop is running.

	code := `
	func main() {
		// Loop 5000 times.
		i = 0
		for {
			if i > 5000 { break }
			
			// getObject creates a new Go object and a new Shadow object (via GC_malloc).
			a = getObject(i)
			
			i = i + 1
		}
		println(999)
	}
	`

	// Enable GC logging in the runtime
	env := map[string]string{
		"GCLOG": "1",
		// We might need to tune GC frequency if 5000 isn't enough to trigger it.
		// GC_initial_heap_size could be small to force frequent GCs?
		// For now, rely on default.
	}

	// We expect "Releasing handle" logs to verify finalizers ran during the loop.
	checkRunBinary(t, code, "main", env, []string{"999", "Releasing handle"})
}
