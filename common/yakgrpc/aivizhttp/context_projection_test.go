package aivizhttp

import (
	"encoding/json"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestContextProjector_ToolCallDurationInlined(t *testing.T) {
	proj := NewContextProjector()
	events := []*schema.AiOutputEvent{
		{
			Model:      gorm.Model{ID: 1},
			Type:       schema.EVENT_TOOL_CALL_START,
			NodeId:     "tc-1",
			CallToolID: "call-1",
			Content:    mustJSON(map[string]any{"call_tool_id": "call-1", "tool": map[string]any{"name": "tree"}}),
		},
		{
			Model:      gorm.Model{ID: 2},
			Type:       schema.EVENT_TOOL_CALL_PARAM,
			CallToolID: "call-1",
			Content:    mustJSON(map[string]any{"call_tool_id": "call-1", "params": map[string]any{"path": "."}}),
		},
		{
			Model:      gorm.Model{ID: 3},
			Type:       schema.EVENT_TOOL_CALL_DONE,
			CallToolID: "call-1",
			Content:    mustJSON(map[string]any{"call_tool_id": "call-1", "duration_ms": 281, "duration_seconds": 0.281}),
		},
	}
	resp := proj.ProjectEvents(events)
	require.Len(t, resp.Blocks, 1)
	b := resp.Blocks[0]
	require.Equal(t, ProjectedToolCall, b.Type)
	require.Equal(t, "tree", b.ToolName)
	require.Equal(t, int64(281), b.ToolDurationMs)
	require.Equal(t, int64(1), b.LineNo, "tool_call block must keep tool_call_start line number")
}

func TestContextProjector_ResultArrivesBeforeStart(t *testing.T) {
	proj := NewContextProjector()
	events := []*schema.AiOutputEvent{
		{
			Model:      gorm.Model{ID: 1},
			Type:       schema.EVENT_TOOL_CALL_RESULT,
			CallToolID: "call-1",
			Content:    mustJSON(map[string]any{"call_tool_id": "call-1", "result": "dir content"}),
		},
		{
			Model:      gorm.Model{ID: 2},
			Type:       schema.EVENT_TOOL_CALL_START,
			CallToolID: "call-1",
			Content:    mustJSON(map[string]any{"call_tool_id": "call-1", "tool": map[string]any{"name": "tree"}}),
		},
	}
	resp := proj.ProjectEvents(events)
	require.Len(t, resp.Blocks, 1)
	b := resp.Blocks[0]
	require.Equal(t, ProjectedToolCall, b.Type)
	require.Equal(t, int64(2), b.LineNo, "tool_call block must appear at start position")
	require.Equal(t, "dir content", b.ToolResult)
}

func TestContextProjector_SeparatesThinkAndAssistant(t *testing.T) {
	proj := NewContextProjector()
	events := []*schema.AiOutputEvent{
		{
			Model:       gorm.Model{ID: 1},
			Type:        schema.EVENT_TYPE_STREAM_START,
			NodeId:      "re-act-loop-thought",
			EventUUID:   "writer-1",
			IsReason:    true,
			ContentType: "text/plain",
		},
		{
			Model:       gorm.Model{ID: 2},
			Type:        schema.EVENT_TYPE_STREAM,
			NodeId:      "re-act-loop-thought",
			EventUUID:   "writer-1",
			StreamDelta: []byte("I should use the tree tool."),
		},
		{
			Model:       gorm.Model{ID: 3},
			Type:        schema.EVENT_TYPE_STREAM_START,
			NodeId:      "directly_call_tool_params",
			EventUUID:   "writer-2",
			ContentType: "text/plain",
		},
		{
			Model:       gorm.Model{ID: 4},
			Type:        schema.EVENT_TYPE_STREAM,
			NodeId:      "directly_call_tool_params",
			EventUUID:   "writer-2",
			StreamDelta: []byte("Calling tree with path '.'"),
		},
	}
	resp := proj.ProjectEvents(events)
	require.Len(t, resp.Blocks, 2)
	require.Equal(t, ProjectedThink, resp.Blocks[0].Type)
	require.Equal(t, "I should use the tree tool.", resp.Blocks[0].Content)
	require.Equal(t, ProjectedAssistant, resp.Blocks[1].Type)
	require.Equal(t, "Calling tree with path '.'", resp.Blocks[1].Content)
}
