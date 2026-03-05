package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPushConstOps_ShouldIgnoreEmptySourceWithoutPanic(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*SFI)
	}{
		{
			name: "push string",
			setup: func(i *SFI) {
				i.OpCode = OpPushString
				i.UnaryStr = "false"
			},
		},
		{
			name: "push bool",
			setup: func(i *SFI) {
				i.OpCode = OpPushBool
				i.UnaryBool = false
			},
		},
		{
			name: "push number",
			setup: func(i *SFI) {
				i.OpCode = OpPushNumber
				i.UnaryInt = 1
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := newSfFrameEx(nil, "", nil, nil, NewConfig())
			frame.config = NewConfig()
			frame.Flush()
			frame.stack.Push(NewEmptyValues())

			i := &SFI{}
			tt.setup(i)

			require.NotPanics(t, func() {
				ok, err := frame.execSyntaxFlowOp(i)
				require.True(t, ok)
				require.NoError(t, err)
			})
			require.Equal(t, 2, frame.stack.Len())
			require.True(t, frame.stack.Peek().IsEmpty())
		})
	}
}
