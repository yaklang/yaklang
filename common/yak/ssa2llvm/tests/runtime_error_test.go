package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Panic recovery stderr lines require libyak.debug.a (see withDebugRuntimeLib in tests).
func TestRuntimeError_MakeSliceIndexPanicRecovered(t *testing.T) {
	code := `
func main() {
	println("Hello Yak World!")
	b = make([]int)
	b[1] = 1
	println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil, withDebugRuntimeLib())
	require.Contains(t, output, "Hello Yak World!\n")
	require.Contains(t, output, "1\n")
	require.Contains(t, output, "[yak-runtime] panic:")
}

func TestRuntimeError_ShadowMethodPanicRecovered(t *testing.T) {
	code := `
func main() {
	l = sync.NewLock()
	l.aaaUndefine()
	println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil, withDebugRuntimeLib())
	require.Contains(t, output, "1\n")
	require.Contains(t, output, "[yak-runtime] panic:")
}
