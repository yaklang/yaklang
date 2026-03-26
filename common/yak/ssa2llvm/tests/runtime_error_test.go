package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeError_MakeSliceIndexPanicLogged(t *testing.T) {
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
	require.Contains(t, output, `[yak-runtime] panic: index "1" out of range`)
	require.Contains(t, output, "1\n")
}

func TestRuntimeError_ShadowMethodPanicLogged(t *testing.T) {
	code := `
func main() {
	l = sync.NewLock()
	l.aaaUndefine()
	println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Contains(t, output, `[yak-runtime] panic: method "aaaUndefine" not found`)
	require.Contains(t, output, "1\n")
}
