package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Default libyak is built without ssa2llvm_runtime_debug: recovered panics do not
// print [yak-runtime] lines to stderr. These tests only assert recovery + stdout.
//
// To assert stderr diagnostics, rebuild with: SSA2LLVM_RUNTIME_DEBUG=1 ./common/yak/ssa2llvm/scripts/build_runtime_go.sh
// and run tests with: go test -tags ssa2llvm_runtime_debug ./...
func TestRuntimeError_MakeSliceIndexPanicRecovered(t *testing.T) {
	code := `
func main() {
	println("Hello Yak World!")
	b = make([]int)
	b[1] = 1
	println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Contains(t, output, "Hello Yak World!\n")
	require.Contains(t, output, "1\n")
}

func TestRuntimeError_ShadowMethodPanicRecovered(t *testing.T) {
	code := `
func main() {
	l = sync.NewLock()
	l.aaaUndefine()
	println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Contains(t, output, "1\n")
}
