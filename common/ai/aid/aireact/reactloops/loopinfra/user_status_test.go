package loopinfra

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestBuildToolBatchResultToolsPreservesActualState(t *testing.T) {
	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Index: 0, ToolName: "read_file"},
		{Index: 1, ToolName: "grep"},
		{Index: 2, ToolName: "web_search"},
	}}
	outcomes := []aicommon.ToolCallOutcome{
		{Index: 0, FinalTool: "read_file", Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Success: true}},
		{Index: 1, FinalTool: "grep_files", Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Success: true}},
		{Index: 2, Stage: aicommon.ToolCallStageInvokeFailed, Err: errors.New("failed")},
	}

	tools, successful := buildToolBatchResultTools(nil, request, outcomes)
	require.Equal(t, 2, successful)
	require.Len(t, tools, 3)
	require.Equal(t, "read_file", tools[0].Name)
	require.Equal(t, aicommon.StatusStateSuccess, tools[0].State)
	require.Equal(t, "grep_files", tools[1].Name)
	require.Equal(t, aicommon.StatusStateSuccess, tools[1].State)
	require.Equal(t, "web_search", tools[2].Name)
	require.Equal(t, aicommon.StatusStateError, tools[2].State)
}
