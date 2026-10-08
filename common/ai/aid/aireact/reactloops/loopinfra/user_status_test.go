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
	require.Equal(t, "正在调用 2 个工具：工具、工具", statuses[0].Value)
	require.Equal(t, &aicommon.StatusProgress{Current: 0, Total: 2, Unit: "tool"}, statuses[0].Progress)
	require.Equal(t, []string{"read_file", "grep_files"}, []string{statuses[0].Tools[0].Name, statuses[0].Tools[1].Name})
	require.Equal(t, "Calling 2 tools: tool, tool", statuses[0].ValueI18n.En)
}

func TestBuildToolCallGroupResultToolsPreservesActualState(t *testing.T) {
	request := &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
		{Index: 0, ToolName: "read_file"},
		{Index: 1, ToolName: "grep"},
		{Index: 2, ToolName: "read_file"},
	}}
	outcomes := []aicommon.ToolCallOutcome{
		{Index: 0, FinalTool: "read_file", Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Success: true}},
		{Index: 1, FinalTool: "grep_files", Stage: aicommon.ToolCallStageDone, Result: &aitool.ToolResult{Success: true}},
		{Index: 2, Stage: aicommon.ToolCallStageInvokeFailed, Err: errors.New("failed")},
	}

	tools, successful := buildToolCallGroupResultTools(nil, request, outcomes)
	require.Equal(t, 2, successful)
	require.Len(t, tools, 3)
	require.Equal(t, "read_file", tools[0].Name)
	require.Equal(t, aicommon.StatusStateSuccess, tools[0].State)
	require.Equal(t, "grep_files", tools[1].Name)
	require.Equal(t, aicommon.StatusStateSuccess, tools[1].State)
	require.Equal(t, "read_file", tools[2].Name)
	require.Equal(t, aicommon.StatusStateError, tools[2].State)
}

func TestBatchStatusAggregatesNamesWithoutAggregatingCalls(t *testing.T) {
	for _, preparing := range []bool{false, true} {
		var statuses []aicommon.StatusPayload
		emitter := aicommon.NewEmitter("repeated-tool-status", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
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
		names := []string{"read_file", "grep", "read_file", "grep", "read_file"}
		if preparing {
			emitToolsPreparingStatus(loop, names)
		} else {
			emitToolBatchRunningStatus(loop, names)
		}
		require.Len(t, statuses, 1)
		status := statuses[0]
		require.Contains(t, status.Value, "工具 × 3、工具 × 2")
		require.Contains(t, status.ValueI18n.En, "tool × 3, tool × 2")
		require.Equal(t, &aicommon.StatusProgress{Current: 0, Total: 5, Unit: "tool"}, status.Progress)
		require.Len(t, status.Tools, 5)
		for i, name := range names {
			require.Equal(t, name, status.Tools[i].Name)
		}
	}
}

func TestStatusToolNamesGroupsByToolIdentityBeforeTruncating(t *testing.T) {
	tools := []aicommon.StatusTool{
		{Name: "read_file", DisplayName: "读取文件", DisplayNameI18n: &schema.I18n{En: "Read file"}},
		{Name: "grep", DisplayName: "搜索", DisplayNameI18n: &schema.I18n{En: "Search"}},
		{Name: "read_file", DisplayName: "读取文件", DisplayNameI18n: &schema.I18n{En: "Read file"}},
		{Name: "web_search", DisplayName: "搜索", DisplayNameI18n: &schema.I18n{En: "Search"}},
		{Name: "write_file", DisplayName: "写文件"},
	}
	require.Equal(t, "读取文件 × 2、搜索、搜索等 1 个工具", statusToolNames(tools, false))
	require.Equal(t, "Read file × 2, Search, Search and 1 more", statusToolNames(tools, true))
	require.Equal(t, "unnamed × 2", statusToolNames([]aicommon.StatusTool{{DisplayName: "unnamed"}, {DisplayName: "unnamed"}}, false))
	require.Empty(t, statusToolNames(nil, false))
}

func TestStatusToolNamesRejectsIdentifierDisplayFallback(t *testing.T) {
	tools := []aicommon.StatusTool{{Name: "do_http_request", DisplayName: "do_http_request",
		DisplayNameI18n: &schema.I18n{Zh: "do_http_request", En: "HTTP Request"}}}
	require.Equal(t, "HTTP Request", statusToolNames(tools, false))
	require.Equal(t, "HTTP Request", statusToolNames(tools, true))
	tools[0].DisplayNameI18n.En = "do_http_request"
	require.Equal(t, "工具", statusToolNames(tools, false))
	require.Equal(t, "tool", statusToolNames(tools, true))
}
