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
	code := `
func main() {
	a = sync.NewLock()
	a = 0
}
`
	output := runBinaryWithEnv(t, code, "main", map[string]string{"GCLOG": "1"})
	require.Contains(t, output, "[Yak GC] Finalizer triggered")
	require.Contains(t, output, "Releasing handle")
}
