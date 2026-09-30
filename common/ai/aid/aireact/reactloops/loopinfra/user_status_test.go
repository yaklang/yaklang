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

	names := []string{"read_file", "grep_files", "read_file", "grep_files", "grep_files"}
	emitToolBatchRunningStatus(loop, names)
	require.Len(t, statuses, 2)
	require.Equal(t, "正在批量执行 5 个工具：read_file * 2、grep_files * 3", statuses[1].Value)
	require.Equal(t, "Running 5 tools: read_file * 2, grep_files * 3", statuses[1].ValueI18n.En)
	require.Equal(t, &aicommon.StatusProgress{Current: 0, Total: 5, Unit: "tool"}, statuses[1].Progress)
	require.Len(t, statuses[1].Tools, len(names))
	for index, tool := range statuses[1].Tools {
		require.Equal(t, names[index], tool.Name)
		require.Equal(t, aicommon.StatusStateRunning, tool.State)
	}
}

func TestStatusToolNamesAggregatesByToolName(t *testing.T) {
	for _, test := range []struct {
		name   string
		tools  []aicommon.StatusTool
		wantZh string
		wantEn string
	}{
		{
			name:   "empty",
			wantZh: "",
			wantEn: "",
		},
		{
			name: "interleaved repeated tools",
			tools: []aicommon.StatusTool{
				{Name: "A"}, {Name: "B"}, {Name: "A"}, {Name: "B"}, {Name: "B"},
			},
			wantZh: "A * 2、B * 3",
			wantEn: "A * 2, B * 3",
		},
		{
			name: "localized labels preserve first occurrence order",
			tools: []aicommon.StatusTool{
				{Name: "B", DisplayName: "乙", DisplayNameI18n: &schema.I18n{En: "Beta"}},
				{Name: "A", DisplayName: "甲", DisplayNameI18n: &schema.I18n{En: "Alpha"}},
				{Name: "B", DisplayName: "乙", DisplayNameI18n: &schema.I18n{En: "Beta"}},
				{Name: "A", DisplayName: "甲", DisplayNameI18n: &schema.I18n{En: "Alpha"}},
				{Name: "A", DisplayName: "甲", DisplayNameI18n: &schema.I18n{En: "Alpha"}},
			},
			wantZh: "乙 * 2、甲 * 3",
			wantEn: "Beta * 2, Alpha * 3",
		},
		{
			name: "distinct tools sharing a display label are not merged",
			tools: []aicommon.StatusTool{
				{Name: "A", DisplayName: "Shared"}, {Name: "B", DisplayName: "Shared"},
			},
			wantZh: "Shared、Shared",
			wantEn: "Shared, Shared",
		},
		{
			name: "visible limit applies after aggregation",
			tools: []aicommon.StatusTool{
				{Name: "A"}, {Name: "A"}, {Name: "B"}, {Name: "B"}, {Name: "C"}, {Name: "D"}, {Name: "D"},
			},
			wantZh: "A * 2、B * 2、C等 1 个工具",
			wantEn: "A * 2, B * 2, C and 1 more",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.wantZh, statusToolNames(test.tools, false))
			require.Equal(t, test.wantEn, statusToolNames(test.tools, true))
		})
	}
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
