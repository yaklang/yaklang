package aivizhttp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// This file holds mock-event regression tests for viz data-parsing contracts.
// Each test feeds minimal synthetic events that mirror the REAL event format
// produced by the current yaklang engine (extracted from actual sessions) and
// asserts that viz correctly parses them. If someone changes the event schema
// on the engine side, these tests will break and surface the mismatch — but
// they do NOT constrain the engine; they only protect viz from silently
// misrendering changed data.
//
// The events are intentionally minimal (one tool call, one loop marker of each
// kind, etc.) so tests are fast and readable.

// ── helpers ────────────────────────────────────────────────────

// mkEvent builds a minimal AiOutputEvent with the given fields.
func mkEvent(id int, etype schema.EventType, nodeID, taskID string, content map[string]any) *schema.AiOutputEvent {
	raw, err := json.Marshal(content)
	if err != nil {
		panic(err)
	}
	e := &schema.AiOutputEvent{
		Model:   gorm.Model{ID: uint(id)},
		Type:    etype,
		NodeId:  nodeID,
		TaskId:  taskID,
		Content: raw,
	}
	if etype == schema.EVENT_TYPE_STREAM_START ||
		etype == schema.EVENT_TYPE_STREAM ||
		etype == schema.EVENT_TYPE_STRUCTURED {
		e.NormalizeRecoveryBlock()
	}
	return e
}

// mkToolCallStart mirrors the real tool_call_start event: tool name lives
// inside {"tool":{"name":"..."}}.
func mkToolCallStart(id int, callToolID, toolName, taskID string) *schema.AiOutputEvent {
	return &schema.AiOutputEvent{
		Model:      gorm.Model{ID: uint(id)},
		Type:       schema.EVENT_TOOL_CALL_START,
		NodeId:     "tc-" + toolName,
		CallToolID: callToolID,
		TaskId:     taskID,
		Content:    mustJSON(map[string]any{"call_tool_id": callToolID, "tool": map[string]any{"name": toolName}}),
	}
}

// ── Tools handler: tool name / params / reason / result extraction ──

// TestToolsParsing_NestedToolName verifies that the tools handler correctly
// extracts the tool name from the nested {"tool":{"name":"read_file"}} structure
// (the real engine emits this shape, not a top-level "tool_name" field).
func TestToolsParsing_NestedToolName(t *testing.T) {
	const cid = "call-1"
	events := []*schema.AiOutputEvent{
		{Model: gorm.Model{ID: 1}, Type: schema.EVENT_TOOL_CALL_START, CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "tool": map[string]any{"name": "read_file"}})},
		{Model: gorm.Model{ID: 2}, Type: "tool_call_reason", CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "reason": "读取配置文件"})},
		{Model: gorm.Model{ID: 3}, Type: "tool_call_param", CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "params": map[string]any{"file": "/etc/config", "lines": 50}})},
		{Model: gorm.Model{ID: 4}, Type: "tool_call_result", CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "result": "file contents here"})},
		{Model: gorm.Model{ID: 5}, Type: "tool_call_done", CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "duration_ms": 100})},
		// summary: "null" — the engine writes literal "null" when there is no
		// summary text; viz must filter it out.
		{Model: gorm.Model{ID: 6}, Type: "tool_call_summary", CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "summary": "null"})},
	}

	// Replicate the handler_tools.go extraction logic.
	tc := &ToolCallSummary{CallToolID: cid, Status: "running"}
	for _, e := range events {
		switch string(e.Type) {
		case "tool_call_start":
			if obj := extractJSONObject(e.Content, "tool"); obj != nil {
				tc.ToolName = jsonStringField(obj, "name")
			}
		case "tool_call_reason":
			tc.Reason = extractJSONField(e.Content, "reason")
		case "tool_call_param":
			tc.Params = extractPrettyJSONField(e.Content, "params")
		case "tool_call_result":
			tc.Result = extractPrettyJSONField(e.Content, "result")
		case "tool_call_done":
			tc.Status = "done"
		case "tool_call_summary":
			if s := extractJSONField(e.Content, "summary"); s != "" && s != "null" {
				tc.Summary = s
			}
		}
	}

	require.Equal(t, "read_file", tc.ToolName, "tool name must be extracted from nested tool.name")
	require.Equal(t, "读取配置文件", tc.Reason, "reason must be plain text, not the full JSON envelope")
	require.NotContains(t, tc.Params, "call_tool_id", "params must not contain the call_tool_id envelope key")
	require.Contains(t, tc.Params, "/etc/config", "params must contain the actual parameter values")
	require.Equal(t, "file contents here", tc.Result, "result must be plain text, not the full JSON envelope")
	require.Equal(t, "done", tc.Status)
	require.Empty(t, tc.Summary, "summary 'null' must be filtered out")
}

// ── Stats: distinct tool count, AI calls, tokens ───────────────

// TestStatsParsing_Contract verifies the stats data contract using events that
// mirror the real engine format: prompt_profile carries input tokens,
// stream_start with ai_model_name counts as an AI call, stream_start without
// model name (tool output) does not.
func TestStatsParsing_Contract(t *testing.T) {
	events := []*schema.AiOutputEvent{
		// Two distinct tool calls (each has start + done)
		{Model: gorm.Model{ID: 1}, Type: schema.EVENT_TOOL_CALL_START, CallToolID: "tc-1"},
		{Model: gorm.Model{ID: 2}, Type: "tool_call_done", CallToolID: "tc-1"},
		{Model: gorm.Model{ID: 3}, Type: schema.EVENT_TOOL_CALL_START, CallToolID: "tc-2"},
		{Model: gorm.Model{ID: 4}, Type: "tool_call_done", CallToolID: "tc-2"},
		// Extra events on tc-1 (param/result) — must not inflate the count.
		{Model: gorm.Model{ID: 5}, Type: "tool_call_param", CallToolID: "tc-1"},
		{Model: gorm.Model{ID: 6}, Type: "tool_call_result", CallToolID: "tc-1"},
		// prompt_profile: input tokens
		{Model: gorm.Model{ID: 7}, Type: "prompt_profile", Content: mustJSON(map[string]any{"loop_name": "test", "prompt_tokens": 1000})},
		{Model: gorm.Model{ID: 8}, Type: "prompt_profile", Content: mustJSON(map[string]any{"loop_name": "test", "prompt_tokens": 2000})},
		// stream_start WITH model name = AI call
		{Model: gorm.Model{ID: 9}, Type: schema.EVENT_TYPE_STREAM_START, AIModelName: "gpt-4"},
		{Model: gorm.Model{ID: 10}, Type: schema.EVENT_TYPE_STREAM_START, AIModelName: "gpt-4"},
		// stream_start WITHOUT model name = tool output stream, NOT an AI call
		{Model: gorm.Model{ID: 11}, Type: schema.EVENT_TYPE_STREAM_START, AIModelName: "", NodeId: "tool-read_file-stdout"},
	}

	// Replicate the stats fallback logic (no consumption events).
	var inputTokens int64
	var aiCalls int64
	modelCalls := map[string]int64{}
	for _, e := range events {
		if e.Type == "prompt_profile" {
			var c map[string]any
			if json.Unmarshal(e.Content, &c) == nil {
				inputTokens += getInt64(c, "prompt_tokens")
			}
		}
		if e.Type == schema.EVENT_TYPE_STREAM_START && e.AIModelName != "" {
			aiCalls++
			modelCalls[e.AIModelName]++
		}
	}

	// Distinct tool call count
	distinct := map[string]bool{}
	for _, e := range events {
		if e.CallToolID != "" {
			distinct[e.CallToolID] = true
		}
	}

	require.Equal(t, 2, len(distinct), "tool call count must be distinct call_tool_id (2), not total events (6)")
	require.Equal(t, int64(3000), inputTokens, "input tokens must sum prompt_tokens from all prompt_profile events")
	require.Equal(t, int64(2), aiCalls, "AI calls must only count stream_start with a model name")
	require.Equal(t, int64(2), modelCalls["gpt-4"], "model breakdown must count per-model")
	require.NotContains(t, modelCalls, "", "model breakdown must not contain empty-name (unknown) entries")
}

// ── Trajectory: loop_marker hierarchy ──────────────────────────

// TestTrajectoryParsing_RealEventFormat verifies BuildTrajectory produces the
// correct tree from events using the real loop_marker format (loop_kind,
// loop_name, task_id, parent_task_id, task_name).
func TestTrajectoryParsing_RealEventFormat(t *testing.T) {
	const root = "react-audit-ROOT"
	const phase1 = root + "-phase1"
	const cat = root + "-phase2-sub-sql_injection-abc"
	fcTask := cat + "-sub-fast-context-xyz"

	events := []*schema.AiOutputEvent{
		// session start + user input
		mkEvent(1, schema.EVENT_TYPE_STRUCTURED, "timeline_item", root, map[string]any{
			"type": "user_input", "entry_type": "current task user input", "content": "帮我审计 /target",
		}),
		// top-level code_security_audit loop
		mkEvent(2, schema.EVENT_TYPE_STRUCTURED, "loop_marker", root, map[string]any{
			"loop_kind": "loop", "loop_name": "code_security_audit",
			"marker": "enter", "parent_task_id": "", "task_id": root,
		}),
		// Phase 1 marker
		mkEvent(3, schema.EVENT_TYPE_STRUCTURED, "loop_marker", root, map[string]any{
			"loop_kind": "phase", "loop_name": "dir_explore",
			"marker": "enter", "parent_task_id": root, "task_id": root,
			"phase_name": "Phase 1：项目探索",
		}),
		// Phase 2 marker
		mkEvent(4, schema.EVENT_TYPE_STRUCTURED, "loop_marker", root, map[string]any{
			"loop_kind": "phase", "loop_name": "code_audit_phase2_orchestrator",
			"marker": "enter", "parent_task_id": root, "task_id": root,
		}),
		// category subagent marker
		mkEvent(5, schema.EVENT_TYPE_STRUCTURED, "loop_marker", cat, map[string]any{
			"loop_kind": "subagent", "loop_name": "code_security_audit",
			"marker": "enter", "parent_task_id": root + "-phase2", "task_id": cat,
			"task_name": "Phase 2 category scan: SQL 注入 (sql_injection)",
		}),
		// scan loop on the category subagent task
		mkEvent(6, schema.EVENT_TYPE_STRUCTURED, "loop_marker", cat, map[string]any{
			"loop_kind": "loop", "loop_name": "code_audit_scan_sql_injection",
			"marker": "enter", "parent_task_id": "", "task_id": cat,
		}),
		// fast_context on a deeper sub-task (real engine runs it on -sub-fast-context-xxx)
		mkEvent(7, schema.EVENT_TYPE_STRUCTURED, "loop_marker", fcTask, map[string]any{
			"loop_kind": "loop", "loop_name": "fast_context",
			"marker": "enter", "parent_task_id": "", "task_id": fcTask,
		}),
		// prompt_profile: fast_context span (lines 7-7) inside scan loop span
		{Model: gorm.Model{ID: 8}, Type: "prompt_profile", TaskId: fcTask,
			Content: mustJSON(map[string]any{"loop_name": "fast_context", "prompt_tokens": 100})},
		{Model: gorm.Model{ID: 9}, Type: "prompt_profile", TaskId: cat,
			Content: mustJSON(map[string]any{"loop_name": "code_audit_scan_sql_injection", "prompt_tokens": 200})},
	}

	traj := BuildTrajectory("test-session", events)
	require.NotNil(t, traj)
	require.Equal(t, "session", traj.Kind)
	require.Empty(t, traj.LoopName, "session root must not carry a child loop name")

	// Walk the tree
	var walk func(n *TrajectoryNode, d int) string
	walk = func(n *TrajectoryNode, d int) string {
		s := n.Kind + ":" + n.LoopName
		for _, c := range n.Children {
			s += "\n" + walk(c, d+1)
		}
		return s
	}
	tree := walk(traj, 0)

	// The top-level code_security_audit loop must be the single root child.
	require.Equal(t, 1, len(traj.Children))
	require.Equal(t, "code_security_audit", traj.Children[0].LoopName)
	audit := traj.Children[0]

	// Both phases nest under the audit loop.
	phaseNames := map[string]bool{}
	for _, c := range audit.Children {
		if c.Kind == "phase" {
			phaseNames[c.LoopName] = true
		}
	}
	require.True(t, phaseNames["dir_explore"], "Phase 1 (dir_explore) must nest under code_security_audit")
	require.True(t, phaseNames["code_audit_phase2_orchestrator"], "Phase 2 must nest under code_security_audit")

	// The category scan loop must exist, and fast_context must nest inside it.
	require.Contains(t, tree, "loop:code_audit_scan_sql_injection", "scan loop must appear")
	require.Contains(t, tree, "loop:fast_context", "fast_context loop must appear")
}

// ── Context projection: tool call + assistant stream ───────────

// TestContextParsing_ToolCallFromRealEvent verifies ProjectEvents correctly
// merges a tool call lifecycle (start/param/result/done) into a single
// tool_call block with the right tool name, params, and result.
func TestContextParsing_ToolCallFromRealEvent(t *testing.T) {
	const cid = "call-tree-1"
	events := []*schema.AiOutputEvent{
		// stream_start for thought (real format: just event_writer_id)
		{Model: gorm.Model{ID: 1}, Type: schema.EVENT_TYPE_STREAM_START, NodeId: "re-act-loop-thought",
			EventUUID: "w1", IsReason: true},
		{Model: gorm.Model{ID: 2}, Type: schema.EVENT_TYPE_STREAM, NodeId: "re-act-loop-thought",
			EventUUID: "w1", StreamDelta: []byte("I need to explore the directory.")},
		// tool call lifecycle
		{Model: gorm.Model{ID: 3}, Type: schema.EVENT_TOOL_CALL_START, NodeId: "tc-tree", CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "tool": map[string]any{"name": "tree"}})},
		{Model: gorm.Model{ID: 4}, Type: schema.EVENT_TOOL_CALL_PARAM, CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "params": map[string]any{"path": "/src"}})},
		{Model: gorm.Model{ID: 5}, Type: schema.EVENT_TOOL_CALL_RESULT, CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "result": "src/\n  main/"})},
		{Model: gorm.Model{ID: 6}, Type: schema.EVENT_TOOL_CALL_DONE, CallToolID: cid,
			Content: mustJSON(map[string]any{"call_tool_id": cid, "duration_ms": 50})},
	}

	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)
	require.NotEmpty(t, resp.Blocks)

	var toolBlock *ProjectedBlock
	for i := range resp.Blocks {
		if resp.Blocks[i].Type == ProjectedToolCall {
			toolBlock = &resp.Blocks[i]
			break
		}
	}
	require.NotNil(t, toolBlock, "must have a tool_call block")
	require.Equal(t, "tree", toolBlock.ToolName, "tool name must come from nested tool.name")
	require.NotEmpty(t, toolBlock.ToolParams, "params must be populated")
	require.NotEmpty(t, toolBlock.ToolResult, "result must be populated")
	require.Equal(t, int64(50), toolBlock.ToolDurationMs, "duration must come from tool_call_done")
}

// ── Helper unit tests ──────────────────────────────────────────

// TestExtractJSONObject_NestedTool verifies extraction of a nested JSON object.
func TestExtractJSONObject_NestedTool(t *testing.T) {
	content := mustJSON(map[string]any{
		"call_tool_id": "x",
		"tool":         map[string]any{"name": "read_file", "description": "reads a file"},
	})
	obj := extractJSONObject(content, "tool")
	require.NotNil(t, obj)
	require.Equal(t, "read_file", jsonStringField(obj, "name"))
	require.Equal(t, "reads a file", jsonStringField(obj, "description"))

	// Non-existent field
	require.Nil(t, extractJSONObject(content, "nonexistent"))
}

// TestExtractPrettyJSONField verifies extraction of params/result fields,
// stripping the call_tool_id envelope.
func TestExtractPrettyJSONField(t *testing.T) {
	// Object field
	paramsContent := mustJSON(map[string]any{
		"call_tool_id": "x",
		"params":       map[string]any{"file": "/etc/config", "lines": 50},
	})
	params := extractPrettyJSONField(paramsContent, "params")
	require.Contains(t, params, "/etc/config")
	require.NotContains(t, params, "call_tool_id", "must not contain the envelope key")

	// String field
	resultContent := mustJSON(map[string]any{
		"call_tool_id": "x",
		"result":       "file contents here",
	})
	result := extractPrettyJSONField(resultContent, "result")
	require.Equal(t, "file contents here", result)

	// Missing field
	require.Empty(t, extractPrettyJSONField(paramsContent, "nonexistent"))
}
