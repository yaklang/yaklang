package reactloops

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestValidateActionToolNames(t *testing.T) {
	require.ErrorContains(t, ValidateActionToolNames([]*LoopAction{{ActionType: "read_file"}}, []string{"read_file"}), "conflicts")
	require.ErrorContains(t, ValidateActionToolNames([]*LoopAction{{ActionType: "finish"}, {ActionType: "finish"}}, nil), "duplicate")
	require.ErrorContains(t, ValidateActionToolNames([]*LoopAction{{ActionType: "invalid/name"}}, nil), "invalid")
	tool, err := aitool.New("read_file", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { return nil, nil }))
	require.NoError(t, err)
	require.NoError(t, ValidateActionToolNames([]*LoopAction{ConvertAIToolToLoopAction(tool), {ActionType: "finish"}}, []string{"read_file"}))
}

func TestDuplicateActionRegistrationFails(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	_, err := NewReActLoop("duplicate", invoker,
		WithRegisterLoopAction("same", "first", nil, nil, nil),
		WithRegisterLoopAction("same", "second", nil, nil, nil))
	require.ErrorContains(t, err, "already registered")
}
