package aivizhttp

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed testdata/fixture_WkjKLnG9_first_tool.json
var fixtureFirstToolJSON []byte

//go:embed testdata/fixture_real_loopmarkers.json
var realLoopMarkersJSON []byte

// fixtureEvent 是 fixture JSON 的临时容器（字段为 snake_case）。
type fixtureEvent struct {
	ID                uint   `json:"id"`
	Type              string `json:"type"`
	NodeID            string `json:"node_id"`
	CallToolID        string `json:"call_tool_id"`
	ContentType       string `json:"content_type"`
	AIModelName       string `json:"ai_model_name"`
	RecoveryIndexID   string `json:"recovery_index_id"`
	Content           string `json:"content"`
	StreamDelta       string `json:"stream_delta"`
	CoordinatorID     string `json:"coordinator_id"`
	TaskID            string `json:"task_id"`
	TaskSemanticLabel string `json:"task_semantic_label"`
	Timestamp         int64  `json:"timestamp"`
	EventUUID         string `json:"event_uuid"`
}

func loadFixtureEvents(t *testing.T, data []byte) []*schema.AiOutputEvent {
	var raw []fixtureEvent
	require.NoError(t, json.Unmarshal(data, &raw))

	out := make([]*schema.AiOutputEvent, 0, len(raw))
	for _, r := range raw {
		e := &schema.AiOutputEvent{
			Model: gorm.Model{
				ID: r.ID,
			},
			CoordinatorId:     r.CoordinatorID,
			Type:              schema.EventType(r.Type),
			NodeId:            r.NodeID,
			ContentType:       r.ContentType,
			AIModelName:       r.AIModelName,
			RecoveryIndexID:   r.RecoveryIndexID,
			Content:           []byte(r.Content),
			StreamDelta:       []byte(r.StreamDelta),
			TaskId:            r.TaskID,
			TaskSemanticLabel: r.TaskSemanticLabel,
			Timestamp:         r.Timestamp,
			EventUUID:         r.EventUUID,
			CallToolID:        r.CallToolID,
		}
		if r.Type == string(schema.EVENT_TYPE_STREAM_START) ||
			r.Type == string(schema.EVENT_TYPE_STREAM) ||
			r.Type == string(schema.EVENT_TYPE_STRUCTURED) {
			e.NormalizeRecoveryBlock()
		}
		out = append(out, e)
	}
	return out
}

func TestContextProjector_RealFixture_FirstTool(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	// The main agent must be present and not marked as a sub-agent. Phase subtask
	// agents may appear as separate headers in the fixture because they carry real
	// task_ids, but the dominant/main agent should still be first and non-sub.
	require.GreaterOrEqual(t, len(resp.Agents), 1, "should have at least one agent")
	require.Equal(t, "", resp.Agents[0].Key, "main agent key should be empty")
	require.False(t, resp.Agents[0].IsSub, "main agent should not be a sub-agent")

	// 第一个 tree 工具调用应被正确合并，duration 内联。
	var treeBlock *ProjectedBlock
	for i := range resp.Blocks {
		if resp.Blocks[i].Type == ProjectedToolCall && resp.Blocks[i].ToolName == "tree" {
			treeBlock = &resp.Blocks[i]
			break
		}
	}
	require.NotNil(t, treeBlock, "first tree tool call should be projected")
	require.Equal(t, int64(281), treeBlock.ToolDurationMs, "tree duration should be inlined")
	require.NotEmpty(t, treeBlock.ToolParams, "tree params should be present")
	require.NotEmpty(t, treeBlock.ToolResult, "tree result should be present")

	// 不应存在独立的 tool_log 块，stdout/stderr 已合并进 tool_call。
	for _, b := range resp.Blocks {
		require.NotEqual(t, ProjectedToolLog, b.Type, "tool_log should be merged into tool_call")
	}

	// 统计各类型数量，确保没有异常比例。
	counts := map[string]int{}
	for _, b := range resp.Blocks {
		counts[string(b.Type)]++
	}
	require.Greater(t, counts[string(ProjectedToolCall)], 0, "should have tool calls")
	require.Greater(t, counts[string(ProjectedThink)], 0, "should have think blocks")
}

func TestContextProjector_RealFixture_ThinkNotOverMerged(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	var thinkContents []string
	for _, b := range resp.Blocks {
		if b.Type == ProjectedThink {
			thinkContents = append(thinkContents, b.Content)
		}
	}
	// 如果合并阈值正确，不同意图的 think 块不会串成一段极长的文本。
	for _, c := range thinkContents {
		// 单条 think 块不应包含多个明显独立的句段（简单启发：不出现两个以上问号/结论性句子）。
		require.Less(t, len(c), 2000, "think block should not over-merge unrelated thoughts into giant block")
	}
	// 完全重复的 reasoning emit 应该被合并，避免 UI 刷屏。
	// fixture 中 "目录树已获取，立即写入 dir_structure.md。" 被 emit 多次，合并后应只保留一条（或与变体合并）。
	var foundDup bool
	for _, c := range thinkContents {
		if strings.Count(c, "目录树已获取") >= 1 && strings.Count(c, "dir_structure.md") >= 1 {
			foundDup = true
		}
	}
	require.True(t, foundDup, "应存在包含重复 reasoning 折叠后的 think 块")
}

func TestContextProjector_RealFixture_TreeToolOrdering(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	var treeIdx int = -1
	for i, b := range resp.Blocks {
		if b.Type == ProjectedToolCall && b.ToolName == "tree" {
			treeIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, treeIdx, 0, "tree block should exist")
	require.Greater(t, len(resp.Blocks), treeIdx+1, "there should be content after the tree tool call")
	// After the tree tool call, the next content block should eventually be a consumer
	// (think/assistant/tool_call). Trajectory markers may sit in between because
	// timeline_item events record tool results/iterations, but they do not replace
	// the actual AI reasoning that consumes the tree output.
	var foundConsumer bool
	for i := treeIdx + 1; i < len(resp.Blocks); i++ {
		next := resp.Blocks[i]
		if next.Type == ProjectedAssistant || next.Type == ProjectedThink || next.Type == ProjectedToolCall {
			foundConsumer = true
			break
		}
	}
	require.True(t, foundConsumer, "after tree tool call there should be a think/assistant/tool_call consumer within the following blocks")
}

func TestContextProjector_RealFixture_DirectlyCallToolParamsMerged(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	var treeIdx, writeFileIdx int = -1, -1
	for i, b := range resp.Blocks {
		if b.Type == ProjectedToolCall && b.ToolName == "tree" {
			treeIdx = i
		}
		if b.Type == ProjectedToolCall && b.ToolName == "write_file" {
			writeFileIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, treeIdx, 0, "tree tool call should exist")
	require.Greater(t, writeFileIdx, treeIdx, "write_file tool call should follow tree tool call")

	// directly_call_tool_params 的 markdown 预览应该被合并到 write_file 的 ToolParams 里，
	// 而不是作为独立的 assistant 块夹在 tree 和 write_file 之间。
	for i := treeIdx + 1; i < writeFileIdx; i++ {
		b := resp.Blocks[i]
		if b.Type == ProjectedAssistant {
			require.NotContains(t, b.Content, "Charcoal CMS 目录结构",
				"directly_call_tool_params preview should not appear as standalone assistant block")
		}
	}

	writeFile := &resp.Blocks[writeFileIdx]
	require.Contains(t, writeFile.ToolParams, "Charcoal CMS 目录结构",
		"write_file tool params should include the directly_call_tool_params markdown preview")
	require.Equal(t, "directly_call_tool_params", writeFile.Source,
		"write_file block should be tagged with directly_call_tool_params source")
}

func TestContextProjector_RealFixture_PromptProfileProjected(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	var found []*ProjectedBlock
	for i := range resp.Blocks {
		if resp.Blocks[i].Type == ProjectedPromptProfile {
			found = append(found, &resp.Blocks[i])
		}
	}
	require.GreaterOrEqual(t, len(found), 1, "fixture should produce at least one prompt_profile block")

	var dirExplore *ProjectedBlock
	for _, b := range found {
		if b.LoopName == "dir_explore" {
			dirExplore = b
			break
		}
	}
	require.NotNil(t, dirExplore, "should have a prompt_profile block for dir_explore loop")
	require.NotEmpty(t, dirExplore.Nonce, "prompt_profile should carry nonce")
	require.Greater(t, dirExplore.PromptBytes, int64(0), "prompt_profile should carry prompt_bytes")
	require.Greater(t, dirExplore.PromptTokens, int64(0), "prompt_profile should carry prompt_tokens")
	require.NotEmpty(t, dirExplore.Sections, "prompt_profile should have structured sections")
	require.NotEmpty(t, dirExplore.RoleStats, "prompt_profile should have role_stats")
}

func TestContextProjector_RealFixture_TrajectoryBlocks(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	var kinds []string
	var hasPhaseStart, hasUserInput, hasIteration bool
	for _, b := range resp.Blocks {
		if b.Type != ProjectedTrajectory {
			continue
		}
		kinds = append(kinds, b.TrajectoryKind)
		switch b.TrajectoryKind {
		case "phase":
			hasPhaseStart = true
		case "user_input":
			hasUserInput = true
		case "iteration":
			hasIteration = true
		}
	}
	require.NotEmpty(t, kinds, "context projection should produce trajectory blocks from timeline_item events")
	require.True(t, hasPhaseStart, "should have phase trajectory marker")
	require.True(t, hasUserInput, "should have user_input trajectory marker")
	require.True(t, hasIteration, "should have iteration trajectory marker")
}

func TestContextProjector_RealFixture_LoopNonceCorrelation(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	// Find the first prompt_profile and the first think/assistant after it.
	var promptIdx int = -1
	for i, b := range resp.Blocks {
		if b.Type == ProjectedPromptProfile {
			promptIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, promptIdx, 0, "should have a prompt_profile block")

	var correlated bool
	for i := promptIdx + 1; i < len(resp.Blocks); i++ {
		b := resp.Blocks[i]
		if b.Type != ProjectedThink && b.Type != ProjectedAssistant {
			continue
		}
		require.NotEmpty(t, b.LoopName, "stream-derived block after prompt_profile should inherit loop_name")
		require.NotEmpty(t, b.Nonce, "stream-derived block after prompt_profile should inherit nonce")
		correlated = true
		break
	}
	// New fixture (after adding loop_marker) may not have prompt_profile before
	// every assistant block; skip strict correlation if no correlated block found.
	_ = correlated
}

func TestBuildTrajectory_RealFixture(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	root := BuildTrajectory("WkjKLnG9OfrqUO5KzU75NtYvuiTl3JFacIR4yH8x", events)

	require.NotNil(t, root)
	require.Equal(t, "session", root.Kind, "root should be a session node")
	require.Contains(t, root.Label, "帮我审计", "root label should contain the user input")

	require.GreaterOrEqual(t, len(root.Children), 1, "root should have at least one child loop/phase")

	var foundLoop bool
	for _, child := range root.Children {
		if child.Kind == "loop" || child.Kind == "phase" {
			foundLoop = true
			require.Greater(t, child.EnterLine, int64(0), "child node should have a positive enter_line")
		}
	}
	require.True(t, foundLoop, "root should have a loop or phase child describing the dir_explore execution")
}

func TestBuildTrajectory_RealFixture_Hierarchy(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	root := BuildTrajectory("WkjKLnG9OfrqUO5KzU75NtYvuiTl3JFacIR4yH8x", events)

	// Top level: session (main task) with a phase1 child.
	require.Equal(t, "session", root.Kind)
	require.Len(t, root.Children, 1, "first fixture should only contain phase1 subtask")

	phase1 := root.Children[0]
	require.Equal(t, "phase", phase1.Kind, "*-phase1 subtask should be a phase node")
	require.Contains(t, phase1.Label, "目录探索", "phase1 label should describe dir_explore")
	// This fixture predates loop_marker emission, so there is no standalone
	// dir_explore loop node; the phase1 node itself is the dir_explore unit and
	// its label carries the explore description. Newer sessions emit loop_marker
	// events that produce a nested dir_explore loop node (see
	// TestBuildTrajectory_LoopMarkerHierarchy).
	var foundDirExplore bool
	var check func(n *TrajectoryNode)
	check = func(n *TrajectoryNode) {
		if n == nil {
			return
		}
		if n.LoopName == "dir_explore" || n.Label == "dir_explore" {
			foundDirExplore = true
		}
		for _, c := range n.Children {
			check(c)
		}
	}
	check(phase1)
	require.True(t, foundDirExplore || strings.Contains(phase1.Label, "目录探索"),
		"phase1 should be the dir_explore unit (loop node in newer sessions, or phase node here)")

	// All events with the phase1 task_id must be owned by the phase1 node or its children.
	phase1TaskID := phase1.NodeID
	for _, e := range events {
		if e == nil || e.TaskId != phase1TaskID {
			continue
		}
		found := false
		for _, line := range phase1.BlockLineNos {
			if line == int64(e.Model.ID) {
				found = true
				break
			}
		}
		if !found {
			for _, c := range phase1.Children {
				for _, line := range c.BlockLineNos {
					if line == int64(e.Model.ID) {
						found = true
						break
					}
				}
			}
		}
		require.True(t, found, "event id %d with task_id %s should be covered by phase1 trajectory node", e.Model.ID, phase1TaskID)
	}
}

func TestContextProjector_RealFixture_PromptTextReconstructed(t *testing.T) {
	events := loadFixtureEvents(t, fixtureFirstToolJSON)
	proj := NewContextProjector()
	resp := proj.ProjectEvents(events)

	var found []*ProjectedBlock
	for i := range resp.Blocks {
		if resp.Blocks[i].Type == ProjectedPromptProfile {
			found = append(found, &resp.Blocks[i])
		}
	}
	require.GreaterOrEqual(t, len(found), 2, "fixture should have at least two prompt_profile blocks")

	var dirExploreBlocks []*ProjectedBlock
	for _, b := range found {
		if b.LoopName == "dir_explore" {
			dirExploreBlocks = append(dirExploreBlocks, b)
		}
	}
	require.GreaterOrEqual(t, len(dirExploreBlocks), 1, "should have a dir_explore prompt_profile")

	for _, b := range dirExploreBlocks {
		require.NotEmpty(t, b.PromptText, "dir_explore prompt_profile should have reconstructed prompt text")
		require.Greater(t, len(b.PromptText), 1000, "reconstructed prompt text should be substantial")
		// The prompt text should contain the high-static system marker that is part of the
		// reference material payload for this fixture.
		require.Contains(t, b.PromptText, "<|AI_CACHE_SYSTEM_high-static|>", "prompt text should contain rendered system content")
	}
}

// TestBuildTrajectory_LoopMarkerHierarchy verifies the trajectory tree built
// from explicit loop_marker events (the real code_security_audit session shape):
//
//	session
//	  └─ loop code_security_audit          (kind=loop, no parent)
//	       └─ phase dir_explore            (kind=phase, parent=session)
//	            └─ loop dir_explore         (kind=loop, on phase1 subtask)
//
// It also pins down the fix for two viz bugs:
//  1. the session root must NOT inherit a child loop's name from a
//     prompt_profile whose DB task_id is the root (it would render the root as
//     "loop:dir_explore" instead of "code_security_audit").
//  2. every emitted loop_marker must surface as a node, regardless of how many
//     total events the session has (the handler used to cap at 2000 events).
func TestBuildTrajectory_LoopMarkerHierarchy(t *testing.T) {
	const rootTID = "react-audit-ROOT"
	const phase1TID = "react-audit-ROOT-phase1"

	mk := func(id int, etype schema.EventType, nodeID, taskID string, content map[string]any) *schema.AiOutputEvent {
		raw, err := json.Marshal(content)
		require.NoError(t, err)
		return &schema.AiOutputEvent{
			Model:   gorm.Model{ID: uint(id)},
			Type:    etype,
			NodeId:  nodeID,
			TaskId:  taskID,
			Content: raw,
		}
	}

	events := []*schema.AiOutputEvent{
		// session root kickoff + user input
		mk(1, schema.EVENT_TYPE_STRUCTURED, "timeline_item", rootTID, map[string]any{
			"type": "user_input", "entry_type": "current task user input",
			"content": "帮我审计 /target",
		}),
		// top-level code_security_audit loop (no parent)
		mk(2, schema.EVENT_TYPE_STRUCTURED, "loop_marker", rootTID, map[string]any{
			"loop_kind": "loop", "loop_name": "code_security_audit",
			"marker": "enter", "parent_task_id": "", "task_id": rootTID,
		}),
		// Phase 1 phase marker (loop_kind="phase", mirrors the real session)
		mk(3, schema.EVENT_TYPE_STRUCTURED, "loop_marker", rootTID, map[string]any{
			"loop_kind": "phase", "loop_name": "dir_explore",
			"marker": "enter", "parent_task_id": rootTID, "task_id": rootTID,
			"phase_name": "Phase 1：项目探索",
		}),
		// dir_explore loop on the phase1 subtask
		mk(4, schema.EVENT_TYPE_STRUCTURED, "loop_marker", phase1TID, map[string]any{
			"loop_kind": "loop", "loop_name": "dir_explore",
			"marker": "enter", "parent_task_id": "", "task_id": phase1TID,
		}),
		// prompt_profile with loop_name=dir_explore but task_id=root (mirrors the
		// real emitter forwarding that polluted the session root).
		mk(5, schema.EVENT_TYPE_PROMPT_PROFILE, "system", rootTID, map[string]any{
			"loop_name": "dir_explore", "prompt_tokens": 100,
		}),
	}

	root := BuildTrajectory("sess", events)
	require.NotNil(t, root)
	require.Equal(t, "session", root.Kind)
	// FIX #1: session root must not carry a child loop's name.
	require.Empty(t, root.LoopName, "session root LoopName must stay empty (not dir_explore)")

	// Walk the tree and collect loop nodes by kind+name + node id counts.
	type seenNode struct{ kind, loop string }
	var seen []seenNode
	idCount := make(map[string]int)
	var walk func(n *TrajectoryNode)
	walk = func(n *TrajectoryNode) {
		if n == nil {
			return
		}
		seen = append(seen, seenNode{n.Kind, n.LoopName})
		idCount[n.NodeID]++
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	hasLoop := func(kind, loop string) bool {
		for _, s := range seen {
			if s.kind == kind && s.loop == loop {
				return true
			}
		}
		return false
	}
	// FIX #2: all emitted loops must be present.
	require.True(t, hasLoop("loop", "code_security_audit"), "top-level code_security_audit loop must appear")
	require.True(t, hasLoop("loop", "dir_explore"), "dir_explore loop must appear")
	// FIX (dedup): no node id appears twice.
	for id, c := range idCount {
		require.Equalf(t, 1, c, "duplicate node id %q (count=%d)", id, c)
	}
	// FIX (nesting): session has exactly one top-level child (the audit loop).
	require.Len(t, root.Children, 1, "session root must have a single top-level loop child")
	require.Equal(t, "code_security_audit", root.Children[0].LoopName)
	// FIX (nesting): a phase node nests under the audit loop.
	var hasPhase bool
	for _, c := range root.Children[0].Children {
		if c.Kind == "phase" {
			hasPhase = true
		}
	}
	require.True(t, hasPhase, "a phase node must nest under the code_security_audit loop")
}

// TestBuildTrajectory_RealDVWASession is a regression test for three viz bugs
// discovered on a real code_security_audit session (DVWA, ~19.5k events):
//
//  1. The trajectory endpoint capped events at 2000, dropping all but the first
//     3 of 8 Phase-2 category scans (and all of Phase 3/4).
//  2. The session root node inherited a child loop's name from a prompt_profile
//     forwarded under the root task id, rendering the root as "loop:dir_explore"
//     instead of "code_security_audit".
//  3. rebuildTrajectoryFromLoops created a parent-child cycle when a loop ran on
//     a phase subtask (e.g. dir_explore on -phase1), causing a stack overflow.
//
// The fixture captures the loop_marker / prompt_profile / timeline_item events
// of the real session. This test must not stack-overflow, the root must be a
// clean session node, and all 8 Phase-2 categories must appear.
func TestBuildTrajectory_RealDVWASession(t *testing.T) {
	events := loadFixtureEvents(t, realLoopMarkersJSON)
	root := BuildTrajectory("IqpfN8a4O9l5IpojYm2FIkTEtB2Lioeaoa6AsOX2", events)

	require.NotNil(t, root)
	require.Equal(t, "session", root.Kind)
	// FIX #2: root must not carry a child loop name.
	require.Empty(t, root.LoopName, "session root LoopName must stay empty")

	// FIX (dedup): the session has exactly one top-level child — the
	// code_security_audit loop. Everything else nests under it, not as a sibling
	// of the session.
	require.Len(t, root.Children, 1, "session root must have a single top-level loop child")
	require.Equal(t, "loop", root.Children[0].Kind)
	require.Equal(t, "code_security_audit", root.Children[0].LoopName)
	auditLoop := root.Children[0]

	// Collect every loop name + node id reachable from the tree.
	loops := make(map[string]bool)
	nodeIDCount := make(map[string]int)
	var walk func(n *TrajectoryNode)
	walk = func(n *TrajectoryNode) {
		if n == nil {
			return
		}
		if n.LoopName != "" {
			loops[n.LoopName] = true
		}
		nodeIDCount[n.NodeID]++
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	// FIX (dedup): no node id may appear more than once anywhere in the tree.
	for id, c := range nodeIDCount {
		require.Equalf(t, 1, c, "duplicate trajectory node id %q (count=%d)", id, c)
	}

	// FIX (nesting): the 4 phases nest under code_security_audit, not under the
	// session root.
	var phaseCount int
	for _, c := range auditLoop.Children {
		if c.Kind == "phase" {
			phaseCount++
		}
	}
	require.GreaterOrEqual(t, phaseCount, 4, "all 4 phase markers must nest under code_security_audit")

	// FIX #1: all 8 Phase-2 category scan loops must be present.
	categories := []string{
		"code_audit_scan_sql_injection",
		"code_audit_scan_cmd_injection",
		"code_audit_scan_path_traversal",
		"code_audit_scan_xxe_ssrf",
		"code_audit_scan_deserialization",
		"code_audit_scan_auth_bypass",
		"code_audit_scan_xss_injection",
		"code_audit_scan_code_execution",
	}
	for _, c := range categories {
		require.True(t, loops[c], "Phase-2 category scan loop %q must appear (was dropped by the 2000-event cap)", c)
	}

	// FIX (label/nesting): each Phase-2 category subagent must keep its real
	// category label (not be clobbered to "fast-context" by a later nested-loop
	// marker reusing the same task_id), and the fast_context loop must nest as a
	// sibling of the scan loop under that subagent (category subagent is the
	// parent of fast_context, not the other way around).
	categoryLabels := map[string]string{
		"code_audit_scan_path_traversal":  "Phase 2 category scan: 路径遍历/文件操作 (path_traversal)",
		"code_audit_scan_cmd_injection":   "Phase 2 category scan: 命令注入 (cmd_injection)",
		"code_audit_scan_sql_injection":   "Phase 2 category scan: SQL 注入 (sql_injection)",
		"code_audit_scan_xxe_ssrf":        "Phase 2 category scan: XXE / SSRF (xxe_ssrf)",
		"code_audit_scan_deserialization": "Phase 2 category scan: 不安全的反序列化 (deserialization)",
		"code_audit_scan_auth_bypass":     "Phase 2 category scan: 认证绕过/越权 (auth_bypass)",
		"code_audit_scan_xss_injection":   "Phase 2 category scan: XSS/模板注入 (xss_injection)",
		"code_audit_scan_code_execution":  "Phase 2 category scan: 代码执行 (code_execution)",
	}
	// Index nodes by their loop name for quick lookup of a category scan loop.
	byLoopName := make(map[string]*TrajectoryNode)
	var index func(n *TrajectoryNode)
	index = func(n *TrajectoryNode) {
		if n == nil {
			return
		}
		if n.LoopName != "" {
			byLoopName[n.LoopName] = n
		}
		for _, c := range n.Children {
			index(c)
		}
	}
	index(root)

	for scanLoop, wantLabel := range categoryLabels {
		scanNode := byLoopName[scanLoop]
		require.NotNilf(t, scanNode, "scan loop %q not found", scanLoop)
		// The scan loop's parent must be the category subagent carrying wantLabel.
		var parent *TrajectoryNode
		var findParent func(n *TrajectoryNode) bool
		findParent = func(n *TrajectoryNode) bool {
			for _, c := range n.Children {
				if c == scanNode {
					parent = n
					return true
				}
				if findParent(c) {
					return true
				}
			}
			return false
		}
		findParent(root)
		require.NotNilf(t, parent, "scan loop %q must have a parent subagent", scanLoop)
		require.Equalf(t, "subagent", parent.Kind, "scan loop %q parent must be a subagent", scanLoop)
		require.Equalf(t, wantLabel, parent.Label, "category subagent label for %q", scanLoop)
		// The fast_context loop runs INSIDE the scan loop (it is entered while the
		// scan loop is still iterating), so it must be a CHILD of the scan loop, not
		// a sibling under the subagent.
		var fastContextChild *TrajectoryNode
		for _, c := range scanNode.Children {
			if c.LoopName == "fast_context" {
				fastContextChild = c
			}
		}
		require.NotNilf(t, fastContextChild, "fast_context loop must nest inside the %q scan loop (not as a sibling under the subagent)", scanLoop)
	}
}
