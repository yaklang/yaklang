package obfuscation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterFunctionWrapperMarksBodyReplaced(t *testing.T) {
	ctx := &Context{}
	err := ctx.RegisterFunctionWrapper(&FunctionWrapper{
		Owner:         "test-obf",
		FuncName:      "check",
		RuntimeSymbol: "yak_runtime_test",
		Payload:       []string{"a", "b"},
	})
	require.NoError(t, err)
	require.True(t, ctx.IsBodyReplaced("check"))
	require.Contains(t, ctx.FunctionWrappers, "check")
	require.Equal(t, "test-obf", ctx.BodyReplacedFuncs["check"])
	require.Equal(t, []string{"a", "b"}, ctx.FunctionWrappers["check"].Payload)
}

func TestRegisterFunctionWrapperRejectsConflicts(t *testing.T) {
	ctx := &Context{}
	require.NoError(t, ctx.RegisterFunctionWrapper(&FunctionWrapper{
		Owner:         "virt",
		FuncName:      "check",
		RuntimeSymbol: "yak_runtime_invoke_vm",
	}))

	err := ctx.RegisterFunctionWrapper(&FunctionWrapper{
		Owner:         "other",
		FuncName:      "check",
		RuntimeSymbol: "yak_runtime_other",
	})
	require.Error(t, err)
}

func TestListByKindVirtualizeIsSSA(t *testing.T) {
	require.Contains(t, ListByKind(KindSSA), "virtualize")
	require.NotContains(t, ListByKind(KindHybrid), "virtualize")
}
