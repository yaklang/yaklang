package compiler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func TestRuntimeLowering_InOperatorUsesRuntimeDispatch(t *testing.T) {
	code := `check = () => { return 2 in [1,2,3] ? 1 : 0 }`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, fmt.Sprintf("store i64 %d", abi.IDRuntimeIn))
	require.Contains(t, ir, "call void @"+abi.InvokeSymbol)
}

func TestRuntimeLowering_ChanRecvUsesRuntimeDispatch(t *testing.T) {
	code := `
check = () => {
	ch = make(chan int, 1)
	ch <- 42
	return <-ch
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, fmt.Sprintf("store i64 %d", abi.IDRuntimeChanRecv))
}
