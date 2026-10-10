package loop_smart_qa

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestReadFileActionUsesRealToolFields(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("read-file-fields", invoker, reactloops.WithAllowToolCall(false), readFileAction(invoker))
	require.NoError(t, err)
	action, err := loop.GetActionHandler("read_file")
	require.NoError(t, err)
	definition := aitool.NewWithoutCallback("read_file", action.Options...)
	require.Equal(t, []string{"file", "offset", "chunk-size"}, definition.InputSchema.Properties.Keys())
	require.Contains(t, definition.InputSchema.Required, "file")
	params := map[string]any{"file": "/abs/source.go"}
	valid, failures := definition.ValidateParams(params)
	require.True(t, valid, failures)
	require.EqualValues(t, 0, params["chunk-size"])
	require.Contains(t, outputExample, `"file": "/path/to/file"`)
	require.NotContains(t, outputExample, `"path": "/path/to/file"`)
}
