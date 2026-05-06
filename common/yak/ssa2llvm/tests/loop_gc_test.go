package tests

import "testing"

func TestRuntime_LoopGC(t *testing.T) {
	code := `
func main() {
	i = 0
	for {
		if i > 5000 { break }
		a = sync.NewLock()
		i = i + 1
	}
	println(999)
}
`

	// GC handle release tracing requires libyak built with -tags ssa2llvm_runtime_debug.
	checkRunBinary(t, code, "main", nil, []string{"999"})
}
