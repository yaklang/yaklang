package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntime_SliceMemberAccess(t *testing.T) {
	code := `
func main() {
	a = make([]int, 10)
	v1 = a[1]
	println(v1)

	a[1] = 20
	v2 = a[1]
	println(v2)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "0\n20\n", output)
}

func TestRuntime_AppendBuiltin(t *testing.T) {
	code := `
func main() {
	a = make([]int, 10)
	a[1] = 1
	a = append(a, 1)
	print(a)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Contains(t, output, "[0 1 0 0 0 0 0 0 0 0 1]")
}

func TestRuntime_ShadowLifecycle(t *testing.T) {
	// Boehm only reclaims a shadow once no stack root still points at it. A
	// single `sync.NewLock()` overwritten by `a = 0` leaves the shadow reachable
	// from a stale stack slot on some builds, so the finalizer never ran and the
	// assertion below failed on slower CI runners. Allocating in a loop (the
	// same shape TestRuntime_LoopGC uses) keeps shadows churning so the sweep at
	// exit has collected the earlier ones; the last assignment still exercises
	// the same `a = 0` overwrite path.
	code := `
func main() {
	i = 0
	for {
		if i > 64 { break }
		a = sync.NewLock()
		a = 0
		i = i + 1
	}
}
`
	output := runBinaryWithEnv(t, code, "main", map[string]string{"GCLOG": "1"}, withDebugRuntimeLib())
	require.Contains(t, output, "[Yak GC] Finalizer triggered")
}
