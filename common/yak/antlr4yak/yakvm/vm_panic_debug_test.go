package yakvm

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVMPanicDebugSuppressionIsPerVM(t *testing.T) {
	quiet, ordinary := NewVMConfig(), NewVMConfig()
	require.False(t, quiet.suppressPanicDebugStack)
	require.False(t, ordinary.suppressPanicDebugStack)
	quiet.SetSuppressPanicDebugStack(true)
	require.True(t, quiet.suppressPanicDebugStack)
	require.False(t, ordinary.suppressPanicDebugStack)
	frame := &Frame{vm: &VirtualMachine{config: quiet}}
	for _, input := range []any{"expected failure", errors.New("native failure"), &VMPanicSignal{Info: "signal", AdditionalInfo: 17}} {
		got := frame.newVMPanic(input)
		want := NewVMPanic(input)
		require.Equal(t, want.GetData(), got.GetData())
		require.Equal(t, want.Error(), got.Error())
	}
	quiet.SetSuppressPanicDebugStack(false)
	require.False(t, quiet.suppressPanicDebugStack)
}
