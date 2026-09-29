package loopinfra

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEmitToolBatchRunningStatusNamesAndProgress(t *testing.T) {
	var statuses []aicommon.StatusPayload
	emitter := aicommon.NewEmitter("batch-status-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.NodeId == "status" {
			var status aicommon.StatusPayload
			if err := json.Unmarshal(event.Content, &status); err != nil {
				return nil, err
			}
			statuses = append(statuses, status)
		}
		return event, nil
	})
	loop := reactloops.NewMinimalReActLoop(&aicommon.Config{Emitter: emitter}, nil)
	emitToolBatchRunningStatus(loop, []string{"read_file", "grep_files"})
	require.Len(t, statuses, 1)
	require.Equal(t, "tool.batch.running", statuses[0].Code)
	require.Equal(t, "正在批量执行 2 个工具：read_file、grep_files", statuses[0].Value)
	require.Equal(t, &aicommon.StatusProgress{Current: 0, Total: 2, Unit: "tool"}, statuses[0].Progress)
	require.Equal(t, []string{"read_file", "grep_files"}, []string{statuses[0].Tools[0].Name, statuses[0].Tools[1].Name})
	require.Equal(t, "Running 2 tools: read_file, grep_files", statuses[0].ValueI18n.En)
}

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
