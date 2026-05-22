package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Runtime integration tests for features that are known to execute correctly
// through the ssa2llvm binary path today. Compile-time lowering for additional
// operators is covered in the compiler package.

func TestRuntimeOperator_PhiMergeAcrossBranches(t *testing.T) {
	check(t, `
check = () => {
	x = 1
	if false {
		x = 100
	} else {
		x = 200
	}
	return x
}
`, 200)
}

func TestRuntimeOperator_ClosureFreeValue(t *testing.T) {
	check(t, `
check = () => {
	base = 40
	addOne = () => base + 1
	return addOne()
}
`, 41)
}

func TestRuntimeOperator_ChanRecvClosedReturnsZero(t *testing.T) {
	check(t, `
check = () => {
	ch = make(chan int)
	close(ch)
	v = <-ch
	return v
}
`, 0)
}

func TestRuntimeOperator_YakitInfoCompilesAndRuns(t *testing.T) {
	checkVerify(t, `
check = () => {
	yakit.Info("hello")
	return 7
}
`, "yak")
	exitCode, _ := runBinaryExitCodeWithEnv(t, `
check = () => {
	yakit.Info("hello")
	return 7
}
`, "check", nil)
	require.Equal(t, 7, exitCode)
}
