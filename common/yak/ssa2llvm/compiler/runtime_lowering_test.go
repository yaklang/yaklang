package compiler

import (
	"fmt"
	"strings"
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

func TestRuntimeLowering_UserFunctionsUsePrivateSymbols(t *testing.T) {
	code := `
read = () => { return 1 }
check = () => { return read() }
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, "@"+abi.UserSymbolPrefix+"read")
	require.NotContains(t, ir, "define void @read(")
}

func TestRuntimeLowering_SelectUsesRuntimeDispatch(t *testing.T) {
	code := `
check = () => {
	ch = make(chan int, 1)
	ch <- 7
	select {
	default:
		return 0
	case v = <-ch:
		return v
	}
}
`
	_, _, ir, err := compileToIRFromCodeWithExternBindings(code, "yak", nil)
	require.NoError(t, err)
	require.Contains(t, ir, fmt.Sprintf("store i64 %d", abi.IDRuntimeSelect))
	for _, line := range strings.Split(ir, "\n") {
		if strings.Contains(line, "select") || strings.Contains(line, fmt.Sprintf("i64 %d", abi.IDRuntimeSelect)) || strings.Contains(line, "yak_dispatch") {
			t.Log(line)
		}
	}
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
