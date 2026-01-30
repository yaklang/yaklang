package workflowdag

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMUSTPASS_ParseToolCallNodes_SingleJSON(t *testing.T) {
	input := `{"call_id": "a1", "tool_name": "search", "call_intent": "search for data", "depends_on": ["b1", "c1"], "allow_failed": true}`

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	node := nodes[0]
	assert.Equal(t, "a1", node.CallID)
	assert.Equal(t, "search", node.ToolName)
	assert.Equal(t, "search for data", node.CallIntent)
	assert.Equal(t, []string{"b1", "c1"}, node.DependsOn())
	assert.True(t, node.AllowFailed())
}

func TestMUSTPASS_ParseToolCallNodes_JSONArray(t *testing.T) {
	input := `[
		{"call_id": "a1", "tool_name": "search", "depends_on": ["b1"]},
		{"call_id": "b1", "tool_name": "fetch"},
		{"call_id": "c1", "tool_name": "process", "depends_on": ["a1", "b1"]}
	]`

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	// Verify all nodes are parsed
	nodeMap := make(map[string]*ToolCallNode)
	for _, n := range nodes {
		nodeMap[n.CallID] = n
	}

	assert.Contains(t, nodeMap, "a1")
	assert.Contains(t, nodeMap, "b1")
	assert.Contains(t, nodeMap, "c1")

	assert.Equal(t, "search", nodeMap["a1"].ToolName)
	assert.Equal(t, []string{"b1"}, nodeMap["a1"].DependsOn())
	assert.Empty(t, nodeMap["b1"].DependsOn())
	assert.Equal(t, []string{"a1", "b1"}, nodeMap["c1"].DependsOn())
}

func TestMUSTPASS_ParseToolCallNodes_JSONMap(t *testing.T) {
	input := `{
		"step1": {"tool_name": "init", "call_intent": "initialize"},
		"step2": {"tool_name": "process", "depends_on": ["step1"]},
		"step3": {"tool_name": "finish", "depends_on": ["step2"]}
	}`

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	nodeMap := make(map[string]*ToolCallNode)
	for _, n := range nodes {
		nodeMap[n.CallID] = n
	}

	assert.Contains(t, nodeMap, "step1")
	assert.Contains(t, nodeMap, "step2")
	assert.Contains(t, nodeMap, "step3")
}

func TestMUSTPASS_ParseToolCallNodes_LineSeparated(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "first"}
{"call_id": "b", "tool_name": "second", "depends_on": ["a"]}
{"call_id": "c", "tool_name": "third", "depends_on": ["b"]}`

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 3)
}

func TestMUSTPASS_ParseToolCallNodes_AllowFailedConflict(t *testing.T) {
	// When both allow_failed and disallow_failed are present, allow_failed wins
	input := `{"call_id": "a", "tool_name": "test", "allow_failed": true, "disallow_failed": true}`

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	// allow_failed wins (more permissive)
	assert.True(t, nodes[0].AllowFailed())
}

func TestMUSTPASS_ParseToolCallNodes_DisallowFailed(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "disallow_failed": true}`

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 1)

	// disallow_failed=true means AllowFailed()=false
	assert.False(t, nodes[0].AllowFailed())
}

func TestMUSTPASS_BuildToolCallDAG(t *testing.T) {
	input := `[
		{"call_id": "fetch", "tool_name": "http_get"},
		{"call_id": "parse", "tool_name": "json_parse", "depends_on": ["fetch"]},
		{"call_id": "store", "tool_name": "db_save", "depends_on": ["parse"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	// Verify entries
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "store", entries[0].CallID)

	// Verify stages
	stage, ok := dag.GetStage("fetch")
	assert.True(t, ok)
	assert.Equal(t, 0, stage)

	stage, ok = dag.GetStage("parse")
	assert.True(t, ok)
	assert.Equal(t, 1, stage)

	stage, ok = dag.GetStage("store")
	assert.True(t, ok)
	assert.Equal(t, 2, stage)
}

func TestMUSTPASS_ToolCallDAG_Execute(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "first"},
		{"call_id": "b", "tool_name": "second", "depends_on": ["a"]},
		{"call_id": "c", "tool_name": "third", "depends_on": ["b"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	var executionOrder []string
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executionOrder = append(executionOrder, node.CallID)
		return nil
	})
	require.NoError(t, err)

	// Verify execution order: a -> b -> c
	require.Len(t, executionOrder, 3)
	assert.Equal(t, "a", executionOrder[0])
	assert.Equal(t, "b", executionOrder[1])
	assert.Equal(t, "c", executionOrder[2])

	// Verify all nodes are completed
	node, _ := dag.GetNodeByCallID("a")
	assert.Equal(t, NodeStatusCompleted, node.GetStatus())
	node, _ = dag.GetNodeByCallID("b")
	assert.Equal(t, NodeStatusCompleted, node.GetStatus())
	node, _ = dag.GetNodeByCallID("c")
	assert.Equal(t, NodeStatusCompleted, node.GetStatus())
}

func TestMUSTPASS_ToolCallDAG_GetDOT(t *testing.T) {
	input := `[
		{"call_id": "fetch", "tool_name": "http_get"},
		{"call_id": "parse", "tool_name": "json_parse", "depends_on": ["fetch"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	dot := dag.GetDOT()

	// Verify DOT format
	assert.Contains(t, dot, "digraph ToolCallDAG")
	assert.Contains(t, dot, "fetch(http_get)")
	assert.Contains(t, dot, "parse(json_parse)")
	assert.Contains(t, dot, "\"fetch\" -> \"parse\"")
}

func TestMUSTPASS_ToolCallDAG_GetGraphJSON(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "first", "call_intent": "do first thing"},
		{"call_id": "b", "tool_name": "second", "depends_on": ["a"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	graph := dag.GetGraphJSON()

	// Verify nodes
	assert.Len(t, graph.Nodes, 2)
	assert.Len(t, graph.Edges, 1)
	assert.Len(t, graph.Categories, 5) // pending, processing, completed, failed, skipped

	// Verify edge
	assert.Equal(t, "a", graph.Edges[0].Source)
	assert.Equal(t, "b", graph.Edges[0].Target)

	// Verify JSON string output
	jsonStr := dag.GetGraphJSONString()
	assert.Contains(t, jsonStr, "\"nodes\"")
	assert.Contains(t, jsonStr, "\"edges\"")
	assert.Contains(t, jsonStr, "\"categories\"")
}

func TestMUSTPASS_ToolCallDAG_CycleAutoTerminate(t *testing.T) {
	// Cycle: a -> b -> c -> a
	input := `[
		{"call_id": "a", "tool_name": "first", "depends_on": ["c"]},
		{"call_id": "b", "tool_name": "second", "depends_on": ["a"]},
		{"call_id": "c", "tool_name": "third", "depends_on": ["b"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	var executedNodes []string
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executedNodes = append(executedNodes, node.CallID)
		return nil
	})
	require.NoError(t, err)

	// Each node should be executed exactly once
	assert.Len(t, executedNodes, 3)

	// Verify no duplicate executions
	seen := make(map[string]bool)
	for _, id := range executedNodes {
		assert.False(t, seen[id], "node %s executed multiple times", id)
		seen[id] = true
	}
}

func TestMUSTPASS_ToolCallNode_DisplayName(t *testing.T) {
	node := NewToolCallNode("call123", "search_tool")
	assert.Equal(t, "call123(search_tool)", node.DisplayName())
}

func TestMUSTPASS_ToolCallDAG_SetExecuteFunc(t *testing.T) {
	input := `[{"call_id": "a", "tool_name": "test"}]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	executed := false
	err = dag.SetExecuteFunc("a", func(ctx context.Context, node *ToolCallNode) error {
		executed = true
		node.Result = "success"
		return nil
	})
	require.NoError(t, err)

	// Get entries and execute
	entries, _ := dag.Entries()
	for chain := range entries {
		chain.Execute(func(node *ToolCallNode) error {
			return node.Execute(ctx)
		})
	}

	assert.True(t, executed)
	node, _ := dag.GetNodeByCallID("a")
	assert.Equal(t, "success", node.Result)
}

func TestMUSTPASS_ToolCallDAG_ParallelChains(t *testing.T) {
	// Two independent chains: a->b and c->d
	input := `[
		{"call_id": "a", "tool_name": "first"},
		{"call_id": "b", "tool_name": "second", "depends_on": ["a"]},
		{"call_id": "c", "tool_name": "third"},
		{"call_id": "d", "tool_name": "fourth", "depends_on": ["c"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	// Should have 2 entries (b and d)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	var executedNodes []string
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executedNodes = append(executedNodes, node.CallID)
		return nil
	})
	require.NoError(t, err)

	// All 4 nodes should be executed
	assert.Len(t, executedNodes, 4)
}

func TestMUSTPASS_ToolCallDAG_DiamondDependency(t *testing.T) {
	// Diamond: d depends on b and c, both b and c depend on a
	input := `[
		{"call_id": "a", "tool_name": "base"},
		{"call_id": "b", "tool_name": "left", "depends_on": ["a"]},
		{"call_id": "c", "tool_name": "right", "depends_on": ["a"]},
		{"call_id": "d", "tool_name": "merge", "depends_on": ["b", "c"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	var executionOrder []string
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executionOrder = append(executionOrder, node.CallID)
		return nil
	})
	require.NoError(t, err)

	// a must come first, d must come last
	assert.Equal(t, "a", executionOrder[0])
	assert.Equal(t, "d", executionOrder[3])

	// b and c can be in any order in between
	assert.Contains(t, executionOrder[1:3], "b")
	assert.Contains(t, executionOrder[1:3], "c")
}

func TestMUSTPASS_ToolCallDAG_GetDOT_WithStatus(t *testing.T) {
	input := `[{"call_id": "a", "tool_name": "test"}]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	// Execute to change status
	dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		return nil
	})

	dot := dag.GetDOT()

	// Completed node should have green color
	assert.Contains(t, dot, "lightgreen")
}

func TestMUSTPASS_ParseToolCallNodes_ByteInput(t *testing.T) {
	input := []byte(`{"call_id": "a", "tool_name": "test"}`)

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "a", nodes[0].CallID)
}

func TestMUSTPASS_ParseToolCallNodes_StructInput(t *testing.T) {
	input := []map[string]any{
		{"call_id": "a", "tool_name": "first"},
		{"call_id": "b", "tool_name": "second", "depends_on": []string{"a"}},
	}

	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
}

func TestMUSTPASS_ToolCallDAG_GetGraphJSON_AfterExecution(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "test"}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	// Execute
	dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		return nil
	})

	graph := dag.GetGraphJSON()

	// Find node a
	var nodeA *GraphNode
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == "a" {
			nodeA = &graph.Nodes[i]
			break
		}
	}

	require.NotNil(t, nodeA)
	assert.Equal(t, "completed", nodeA.Status)
	assert.Equal(t, int(NodeStatusCompleted), nodeA.Category)
}

func TestMUSTPASS_ToolCallDAG_ComplexWorkflow(t *testing.T) {
	// Complex workflow with multiple dependencies
	input := `[
		{"call_id": "init", "tool_name": "initialize", "call_intent": "setup environment"},
		{"call_id": "fetch_a", "tool_name": "http_get", "call_intent": "get data A", "depends_on": ["init"]},
		{"call_id": "fetch_b", "tool_name": "http_get", "call_intent": "get data B", "depends_on": ["init"]},
		{"call_id": "process", "tool_name": "transform", "call_intent": "process both", "depends_on": ["fetch_a", "fetch_b"]},
		{"call_id": "validate", "tool_name": "check", "call_intent": "validate result", "depends_on": ["process"]},
		{"call_id": "store", "tool_name": "db_save", "call_intent": "save to database", "depends_on": ["validate"]},
		{"call_id": "notify", "tool_name": "send_email", "call_intent": "send notification", "depends_on": ["store"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	// Verify structure
	assert.Equal(t, 7, dag.NodeCount())

	// Verify stages
	stages, err := dag.GetStages()
	require.NoError(t, err)

	// init should be stage 0
	assert.Len(t, stages[0], 1)
	// fetch_a and fetch_b should be stage 1
	assert.Len(t, stages[1], 2)
	// process should be stage 2
	assert.Len(t, stages[2], 1)

	// Execute and verify
	var executedCount int
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executedCount++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 7, executedCount)

	// Verify DOT output contains all nodes
	dot := dag.GetDOT()
	assert.Contains(t, dot, "init(initialize)")
	assert.Contains(t, dot, "notify(send_email)")
	assert.True(t, strings.Count(dot, "->") >= 7) // At least 7 edges
}

// ==================== Edge Case Tests for ToolCallNode ====================

func TestMUSTPASS_ToolCallDAG_EmptyInput(t *testing.T) {
	_, err := ParseToolCallNodes("")
	assert.Error(t, err)
}

func TestMUSTPASS_ToolCallDAG_InvalidJSON(t *testing.T) {
	_, err := ParseToolCallNodes("not json")
	assert.Error(t, err)
}

func TestMUSTPASS_ToolCallDAG_MissingCallID(t *testing.T) {
	input := `{"tool_name": "test"}`
	_, err := ParseToolCallNodes(input)
	assert.Error(t, err)
}

func TestMUSTPASS_ToolCallDAG_EmptyCallID(t *testing.T) {
	input := `{"call_id": "", "tool_name": "test"}`
	_, err := ParseToolCallNodes(input)
	assert.Error(t, err)
}

func TestMUSTPASS_ToolCallDAG_WhitespaceInput(t *testing.T) {
	input := `   {"call_id": "a", "tool_name": "test"}   `
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Len(t, nodes, 1)
}

func TestMUSTPASS_ToolCallDAG_EmptyArray(t *testing.T) {
	input := `[]`
	_, err := ParseToolCallNodes(input)
	assert.Error(t, err)
}

func TestMUSTPASS_ToolCallDAG_EmptyMap(t *testing.T) {
	input := `{}`
	_, err := ParseToolCallNodes(input)
	assert.Error(t, err)
}

func TestMUSTPASS_ToolCallDAG_UnicodeCallID(t *testing.T) {
	input := `{"call_id": "任务_1", "tool_name": "搜索"}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Equal(t, "任务_1", nodes[0].CallID)
	assert.Equal(t, "搜索", nodes[0].ToolName)
}

func TestMUSTPASS_ToolCallDAG_SpecialCharacters(t *testing.T) {
	input := `{"call_id": "call-id_123.test", "tool_name": "tool.name-v2"}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Equal(t, "call-id_123.test", nodes[0].CallID)
}

func TestMUSTPASS_ToolCallDAG_LargeCallIntent(t *testing.T) {
	longIntent := strings.Repeat("x", 10000)
	input := fmt.Sprintf(`{"call_id": "a", "tool_name": "test", "call_intent": "%s"}`, longIntent)
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Len(t, nodes[0].CallIntent, 10000)
}

func TestMUSTPASS_ToolCallDAG_NullDependsOn(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "depends_on": null}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Empty(t, nodes[0].DependsOn())
}

func TestMUSTPASS_ToolCallDAG_EmptyDependsOnArray(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "depends_on": []}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Empty(t, nodes[0].DependsOn())
}

func TestMUSTPASS_ToolCallDAG_AllowFailedDefault(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test"}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	// Default should be false (strict mode)
	assert.False(t, nodes[0].AllowFailed())
}

func TestMUSTPASS_ToolCallDAG_DisallowFailedFalse(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "disallow_failed": false}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	// disallow_failed=false means AllowFailed()=true
	assert.True(t, nodes[0].AllowFailed())
}

func TestMUSTPASS_ToolCallDAG_DOTEscaping(t *testing.T) {
	input := `{"call_id": "a\"quote", "tool_name": "test\"name"}`
	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	dot := dag.GetDOT()
	// Should escape quotes properly
	assert.Contains(t, dot, "\\\"")
}

func TestMUSTPASS_ToolCallDAG_SelfReference(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "depends_on": ["a"]}`
	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	var executedCount int
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executedCount++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, executedCount) // Should execute once despite self-reference
}

func TestMUSTPASS_ToolCallDAG_NonExistentDependency(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "depends_on": ["nonexistent"]}`
	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err) // Should not error, just ignore missing dependency

	var executedCount int
	err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
		executedCount++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, executedCount)
}

func TestMUSTPASS_ToolCallDAG_DuplicateDependencies(t *testing.T) {
	input := `{"call_id": "a", "tool_name": "test", "depends_on": ["b", "b", "b"]}`
	nodes, err := ParseToolCallNodes(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "b", "b"}, nodes[0].DependsOn())
}

// ==================== Concurrent Tests for ToolCallDAG ====================

func TestMUSTPASS_ToolCallDAG_ConcurrentExecution(t *testing.T) {
	input := `[
		{"call_id": "base", "tool_name": "init"},
		{"call_id": "a", "tool_name": "task_a", "depends_on": ["base"]},
		{"call_id": "b", "tool_name": "task_b", "depends_on": ["base"]},
		{"call_id": "c", "tool_name": "task_c", "depends_on": ["base"]},
		{"call_id": "d", "tool_name": "task_d", "depends_on": ["base"]},
		{"call_id": "e", "tool_name": "task_e", "depends_on": ["base"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*ToolCallNode]) {
			defer wg.Done()
			c.Execute(func(node *ToolCallNode) error {
				executedNodes.Store(node.CallID, true)
				return nil
			})
		}(chain)
	}

	wg.Wait()

	// Count executed nodes
	var count int
	executedNodes.Range(func(key, value any) bool {
		count++
		return true
	})

	assert.Equal(t, 6, count)
}

func TestMUSTPASS_ToolCallDAG_ConcurrentGetDOT(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "test"},
		{"call_id": "b", "tool_name": "test", "depends_on": ["a"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dot := dag.GetDOT()
			assert.Contains(t, dot, "digraph")
		}()
	}

	wg.Wait()
}

func TestMUSTPASS_ToolCallDAG_ConcurrentGetGraphJSON(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "test"},
		{"call_id": "b", "tool_name": "test", "depends_on": ["a"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			graph := dag.GetGraphJSON()
			assert.NotNil(t, graph)
			assert.Len(t, graph.Nodes, 2)
		}()
	}

	wg.Wait()
}

func TestMUSTPASS_ToolCallDAG_ConcurrentExecutionWithErrors(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "test", "allow_failed": true},
		{"call_id": "b", "tool_name": "test", "depends_on": ["a"], "allow_failed": true},
		{"call_id": "c", "tool_name": "test", "depends_on": ["a"], "allow_failed": true}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*ToolCallNode]) {
			defer wg.Done()
			c.Execute(func(node *ToolCallNode) error {
				executedNodes.Store(node.CallID, true)
				// Simulate some errors
				if node.CallID == "a" {
					return fmt.Errorf("simulated error")
				}
				return nil
			})
		}(chain)
	}

	wg.Wait()

	// All nodes should still be executed due to allow_failed
	var count int
	executedNodes.Range(func(key, value any) bool {
		count++
		return true
	})

	assert.Equal(t, 3, count)
}

func TestMUSTPASS_ToolCallDAG_StressTest(t *testing.T) {
	// Build a large DAG
	var nodes []map[string]any
	nodeCount := 50

	for i := 0; i < nodeCount; i++ {
		node := map[string]any{
			"call_id":   fmt.Sprintf("node_%d", i),
			"tool_name": fmt.Sprintf("tool_%d", i),
		}
		if i > 0 {
			// Depend on previous 1-3 nodes
			var deps []string
			for j := 1; j <= 3 && i-j >= 0; j++ {
				deps = append(deps, fmt.Sprintf("node_%d", i-j))
			}
			node["depends_on"] = deps
		}
		nodes = append(nodes, node)
	}

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, nodes)
	require.NoError(t, err)

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*ToolCallNode]) {
			defer wg.Done()
			c.Execute(func(node *ToolCallNode) error {
				executedNodes.Store(node.CallID, true)
				return nil
			})
		}(chain)
	}

	wg.Wait()

	// Count executed nodes
	var count int
	executedNodes.Range(func(key, value any) bool {
		count++
		return true
	})

	assert.Equal(t, nodeCount, count)
}

func TestMUSTPASS_ToolCallDAG_RepeatedExecution(t *testing.T) {
	input := `[
		{"call_id": "a", "tool_name": "test"},
		{"call_id": "b", "tool_name": "test", "depends_on": ["a"]}
	]`

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, input)
	require.NoError(t, err)

	for round := 0; round < 10; round++ {
		dag.Reset()

		var executedCount int32
		err = dag.ExecuteWithHandler(func(ctx context.Context, node *ToolCallNode) error {
			atomic.AddInt32(&executedCount, 1)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, int32(2), executedCount, "round %d failed", round)
	}
}

func TestMUSTPASS_ToolCallDAG_ManyParallelChains(t *testing.T) {
	// Create 20 independent 3-node chains
	var nodes []map[string]any
	for i := 0; i < 20; i++ {
		chainBase := fmt.Sprintf("chain%d", i)
		nodes = append(nodes,
			map[string]any{"call_id": chainBase + "_a", "tool_name": "start"},
			map[string]any{"call_id": chainBase + "_b", "tool_name": "middle", "depends_on": []string{chainBase + "_a"}},
			map[string]any{"call_id": chainBase + "_c", "tool_name": "end", "depends_on": []string{chainBase + "_b"}},
		)
	}

	ctx := context.Background()
	dag, err := BuildToolCallDAG(ctx, nodes)
	require.NoError(t, err)

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 20) // 20 chain endpoints

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*ToolCallNode]) {
			defer wg.Done()
			c.Execute(func(node *ToolCallNode) error {
				executedNodes.Store(node.CallID, true)
				return nil
			})
		}(chain)
	}

	wg.Wait()

	// Count executed nodes
	var count int
	executedNodes.Range(func(key, value any) bool {
		count++
		return true
	})

	assert.Equal(t, 60, count) // 20 chains * 3 nodes
}
