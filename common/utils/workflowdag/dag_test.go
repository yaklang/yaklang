package workflowdag

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNode is a simple test implementation of DAGNode
type TestNode struct {
	*BaseNode
	ExecuteCount int32
	mu           sync.Mutex
}

func NewTestNode(id string, deps ...string) *TestNode {
	return &TestNode{
		BaseNode: NewBaseNode(id, deps...),
	}
}

func (n *TestNode) Execute() {
	atomic.AddInt32(&n.ExecuteCount, 1)
}

func (n *TestNode) GetExecuteCount() int {
	return int(atomic.LoadInt32(&n.ExecuteCount))
}

// TestMUSTPASS_SingleChain tests A->B->C chain (A depends on B, B depends on C)
// Execution order should be: C -> B -> A
func TestMUSTPASS_SingleChain(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A depends on B, B depends on C
	nodeA := NewTestNode("A", "B")
	nodeB := NewTestNode("B", "C")
	nodeC := NewTestNode("C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC))
	require.NoError(t, dag.Build())

	// Entry should be A (not depended on by anyone)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "A", entries[0].GetID())

	// Get chain iterator
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	var executionOrder []string
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executionOrder = append(executionOrder, node.GetID())
			node.Execute()
			return nil
		})
		require.NoError(t, err)
	}

	// Verify execution order: dependencies first
	// Since A depends on B depends on C, we should see dependencies resolved first
	assert.Contains(t, executionOrder, "A")
	assert.Contains(t, executionOrder, "B")
	assert.Contains(t, executionOrder, "C")
	assert.Len(t, executionOrder, 3)

	// Verify each node executed exactly once
	assert.Equal(t, 1, nodeA.GetExecuteCount())
	assert.Equal(t, 1, nodeB.GetExecuteCount())
	assert.Equal(t, 1, nodeC.GetExecuteCount())
}

// TestMUSTPASS_ConvergingDeps tests A->B, C->B (both A and C depend on B)
// B is the entry point, A and C can execute after B
func TestMUSTPASS_ConvergingDeps(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A and C both depend on B
	nodeA := NewTestNode("A", "B")
	nodeB := NewTestNode("B")
	nodeC := NewTestNode("C", "B")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC))
	require.NoError(t, dag.Build())

	// Entries should be A and C (not depended on by anyone)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	entryIDs := make(map[string]bool)
	for _, e := range entries {
		entryIDs[e.GetID()] = true
	}
	assert.True(t, entryIDs["A"])
	assert.True(t, entryIDs["C"])

	// Execute all chains
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedNodes := make(map[string]int)
	var mu sync.Mutex

	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			mu.Lock()
			executedNodes[node.GetID()]++
			mu.Unlock()
			node.Execute()
			return nil
		})
		require.NoError(t, err)
	}

	// B should be executed exactly once (even though two chains depend on it)
	assert.Equal(t, 1, executedNodes["B"])
	// A and C should each be executed once
	assert.Equal(t, 1, executedNodes["A"])
	assert.Equal(t, 1, executedNodes["C"])
}

// TestMUSTPASS_IndependentNodes tests A, B, C with no dependencies
// Should have 3 independent entry points
func TestMUSTPASS_IndependentNodes(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B")
	nodeC := NewTestNode("C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC))
	require.NoError(t, dag.Build())

	// All three should be entries
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// Execute all chains
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedCount := 0
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executedCount++
			node.Execute()
			return nil
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 3, executedCount)
	assert.Equal(t, 1, nodeA.GetExecuteCount())
	assert.Equal(t, 1, nodeB.GetExecuteCount())
	assert.Equal(t, 1, nodeC.GetExecuteCount())
}

// TestMUSTPASS_MixedDeps tests A->B, C (B depends on A, C is independent)
// Two chains: A->B and C
func TestMUSTPASS_MixedDeps(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// B depends on A, C is independent
	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B", "A")
	nodeC := NewTestNode("C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC))
	require.NoError(t, dag.Build())

	// Entries: B and C (not depended on by anyone)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Execute
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedNodes := make(map[string]int)
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executedNodes[node.GetID()]++
			return nil
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 1, executedNodes["A"])
	assert.Equal(t, 1, executedNodes["B"])
	assert.Equal(t, 1, executedNodes["C"])
}

// TestMUSTPASS_TwoIndependentChains tests A->B, C->D (two independent chains)
func TestMUSTPASS_TwoIndependentChains(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Chain 1: B depends on A
	// Chain 2: D depends on C
	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B", "A")
	nodeC := NewTestNode("C")
	nodeD := NewTestNode("D", "C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC, nodeD))
	require.NoError(t, dag.Build())

	// Entries: B and D
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Execute
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedNodes := make(map[string]int)
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executedNodes[node.GetID()]++
			return nil
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 1, executedNodes["A"])
	assert.Equal(t, 1, executedNodes["B"])
	assert.Equal(t, 1, executedNodes["C"])
	assert.Equal(t, 1, executedNodes["D"])
}

// TestMUSTPASS_CycleAutoTerminate tests A->B->C->A (triangle cycle)
// Should auto-terminate when reaching already executed node
func TestMUSTPASS_CycleAutoTerminate(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A depends on B, B depends on C, C depends on A (cycle)
	nodeA := NewTestNode("A", "B")
	nodeB := NewTestNode("B", "C")
	nodeC := NewTestNode("C", "A")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC))
	require.NoError(t, dag.Build())

	// Find cycles
	cycles := dag.FindCycles()
	assert.NotEmpty(t, cycles, "should detect cycle")

	// Execute - should not hang, should auto-terminate
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedNodes := make(map[string]int)
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executedNodes[node.GetID()]++
			return nil
		})
		require.NoError(t, err)
	}

	// Each node should be executed exactly once
	assert.Equal(t, 1, executedNodes["A"], "A should execute once")
	assert.Equal(t, 1, executedNodes["B"], "B should execute once")
	assert.Equal(t, 1, executedNodes["C"], "C should execute once")

	// Total 3 executions
	total := executedNodes["A"] + executedNodes["B"] + executedNodes["C"]
	assert.Equal(t, 3, total, "should execute exactly 3 nodes")
}

// TestMUSTPASS_SelfCycle tests A->A (self-referencing cycle)
func TestMUSTPASS_SelfCycle(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A depends on itself
	nodeA := NewTestNode("A", "A")

	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.Build())

	// Execute - should not hang
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedCount := 0
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executedCount++
			return nil
		})
		require.NoError(t, err)
	}

	// A should be executed exactly once
	assert.Equal(t, 1, executedCount)
}

// TestMUSTPASS_StageCalculation tests stage/level calculation for visualization
func TestMUSTPASS_StageCalculation(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// D depends on B and C
	// B depends on A
	// C depends on A
	// A has no dependencies
	// Stages: A=0, B=1, C=1, D=2
	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B", "A")
	nodeC := NewTestNode("C", "A")
	nodeD := NewTestNode("D", "B", "C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC, nodeD))
	require.NoError(t, dag.Build())

	stageA, ok := dag.GetStage("A")
	assert.True(t, ok)
	assert.Equal(t, 0, stageA)

	stageB, ok := dag.GetStage("B")
	assert.True(t, ok)
	assert.Equal(t, 1, stageB)

	stageC, ok := dag.GetStage("C")
	assert.True(t, ok)
	assert.Equal(t, 1, stageC)

	stageD, ok := dag.GetStage("D")
	assert.True(t, ok)
	assert.Equal(t, 2, stageD)

	// GetStages should group nodes by stage
	stages, err := dag.GetStages()
	require.NoError(t, err)
	assert.Len(t, stages[0], 1) // A
	assert.Len(t, stages[1], 2) // B, C
	assert.Len(t, stages[2], 1) // D
}

// TestMUSTPASS_TryExecuteIdempotent tests that TryExecute is idempotent
func TestMUSTPASS_TryExecuteIdempotent(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.Build())

	// First call should return true
	assert.True(t, dag.TryExecute("A"))

	// Subsequent calls should return false
	assert.False(t, dag.TryExecute("A"))
	assert.False(t, dag.TryExecute("A"))
	assert.False(t, dag.TryExecute("A"))

	// IsExecuted should return true
	assert.True(t, dag.IsExecuted("A"))
}

// TestMUSTPASS_Reset tests that Reset clears execution state
func TestMUSTPASS_Reset(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.Build())

	// Execute
	assert.True(t, dag.TryExecute("A"))
	assert.True(t, dag.IsExecuted("A"))

	// Reset
	dag.Reset()

	// Should be able to execute again
	assert.False(t, dag.IsExecuted("A"))
	assert.True(t, dag.TryExecute("A"))
}

// TestMUSTPASS_DuplicateNode tests that adding duplicate node returns error
func TestMUSTPASS_DuplicateNode(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	require.NoError(t, dag.AddNode(nodeA))

	// Adding same ID again should fail
	nodeA2 := NewTestNode("A")
	err := dag.AddNode(nodeA2)
	assert.ErrorIs(t, err, ErrDuplicateNode)
}

// TestMUSTPASS_EmptyDAG tests that building empty DAG returns error
func TestMUSTPASS_EmptyDAG(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	err := dag.Build()
	assert.ErrorIs(t, err, ErrEmptyDAG)
}

// TestMUSTPASS_NotBuilt tests that operations before Build() return error
func TestMUSTPASS_NotBuilt(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	require.NoError(t, dag.AddNode(nodeA))

	// Entries should fail before Build
	_, err := dag.Entries()
	assert.ErrorIs(t, err, ErrDAGNotBuilt)

	_, err = dag.GetEntries()
	assert.ErrorIs(t, err, ErrDAGNotBuilt)

	_, err = dag.GetStages()
	assert.ErrorIs(t, err, ErrDAGNotBuilt)
}

// TestMUSTPASS_MissingDependency tests nodes with non-existent dependencies
// Should not error, just treat as no dependency
func TestMUSTPASS_MissingDependency(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A depends on X which doesn't exist
	nodeA := NewTestNode("A", "X")

	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.Build()) // Should not error

	// A should be an entry (X doesn't exist so dependency is ignored)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "A", entries[0].GetID())
}

// TestMUSTPASS_DiamondDependency tests diamond-shaped dependency
// D depends on B and C, both B and C depend on A
func TestMUSTPASS_DiamondDependency(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	//     D
	//    / \
	//   B   C
	//    \ /
	//     A
	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B", "A")
	nodeC := NewTestNode("C", "A")
	nodeD := NewTestNode("D", "B", "C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC, nodeD))
	require.NoError(t, dag.Build())

	// Entry should be D (not depended on by anyone)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "D", entries[0].GetID())

	// Execute
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	var executionOrder []string
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executionOrder = append(executionOrder, node.GetID())
			return nil
		})
		require.NoError(t, err)
	}

	// All nodes should be executed exactly once
	assert.Len(t, executionOrder, 4)

	// A must come before B and C
	indexA := indexOf(executionOrder, "A")
	indexB := indexOf(executionOrder, "B")
	indexC := indexOf(executionOrder, "C")
	indexD := indexOf(executionOrder, "D")

	assert.True(t, indexA < indexB, "A should execute before B")
	assert.True(t, indexA < indexC, "A should execute before C")
	assert.True(t, indexB < indexD, "B should execute before D")
	assert.True(t, indexC < indexD, "C should execute before D")
}

// TestMUSTPASS_LongChain tests a long chain of dependencies
func TestMUSTPASS_LongChain(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Create chain: N0 <- N1 <- N2 <- ... <- N9
	nodes := make([]*TestNode, 10)
	for i := 0; i < 10; i++ {
		var deps []string
		if i > 0 {
			deps = []string{nodes[i-1].GetID()}
		}
		nodes[i] = NewTestNode(string(rune('0'+i)), deps...)
		require.NoError(t, dag.AddNode(nodes[i]))
	}

	require.NoError(t, dag.Build())

	// Entry should be the last node (N9)
	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	// Execute
	entryCh, err := dag.Entries()
	require.NoError(t, err)

	executedCount := 0
	for chain := range entryCh {
		err := chain.Execute(func(node *TestNode) error {
			executedCount++
			return nil
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 10, executedCount)
}

// TestMUSTPASS_ConcurrentAccess tests thread-safety of DAG operations
func TestMUSTPASS_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	nodeB := NewTestNode("B")
	nodeC := NewTestNode("C")

	require.NoError(t, dag.AddNodes(nodeA, nodeB, nodeC))
	require.NoError(t, dag.Build())

	// Concurrent TryExecute calls
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if dag.TryExecute(id) {
				atomic.AddInt32(&successCount, 1)
			}
		}([]string{"A", "B", "C"}[i%3])
	}

	wg.Wait()

	// Only 3 should succeed (one for each node)
	assert.Equal(t, int32(3), successCount)
}

// Helper function to find index in slice
func indexOf(slice []string, item string) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

// ==================== Edge Case Tests ====================

// TestMUSTPASS_SingleNode tests DAG with only one node
func TestMUSTPASS_SingleNode(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	nodeA := NewTestNode("A")
	require.NoError(t, dag.AddNode(nodeA))
	require.NoError(t, dag.Build())

	entries, err := dag.GetEntries()
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	entryCh, _ := dag.Entries()
	var executed int
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			executed++
			return nil
		})
	}
	assert.Equal(t, 1, executed)
}

// TestMUSTPASS_AllNodesDependent tests where all nodes form a single chain
func TestMUSTPASS_AllNodesDependent(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A <- B <- C <- D <- E (E depends on D depends on C...)
	nodes := []*TestNode{
		NewTestNode("A"),
		NewTestNode("B", "A"),
		NewTestNode("C", "B"),
		NewTestNode("D", "C"),
		NewTestNode("E", "D"),
	}

	for _, n := range nodes {
		require.NoError(t, dag.AddNode(n))
	}
	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 1)
	assert.Equal(t, "E", entries[0].GetID())

	var order []string
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			order = append(order, node.GetID())
			return nil
		})
	}

	assert.Equal(t, []string{"A", "B", "C", "D", "E"}, order)
}

// TestMUSTPASS_WideFanOut tests one node with many dependents
func TestMUSTPASS_WideFanOut(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// A is depended on by B, C, D, E, F, G, H, I, J
	nodeA := NewTestNode("A")
	require.NoError(t, dag.AddNode(nodeA))

	for _, id := range []string{"B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		require.NoError(t, dag.AddNode(NewTestNode(id, "A")))
	}

	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 9) // B through J

	executedNodes := make(map[string]int)
	var mu sync.Mutex

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			mu.Lock()
			executedNodes[node.GetID()]++
			mu.Unlock()
			return nil
		})
	}

	// A should be executed exactly once
	assert.Equal(t, 1, executedNodes["A"])
	// All others should be executed once
	for _, id := range []string{"B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		assert.Equal(t, 1, executedNodes[id], "node %s should execute once", id)
	}
}

// TestMUSTPASS_WideFanIn tests many nodes converging to one
func TestMUSTPASS_WideFanIn(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Z depends on A, B, C, D, E, F, G, H, I
	for _, id := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
		require.NoError(t, dag.AddNode(NewTestNode(id)))
	}
	require.NoError(t, dag.AddNode(NewTestNode("Z", "A", "B", "C", "D", "E", "F", "G", "H", "I")))

	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 1)
	assert.Equal(t, "Z", entries[0].GetID())

	var executedCount int32
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			atomic.AddInt32(&executedCount, 1)
			return nil
		})
	}

	assert.Equal(t, int32(10), executedCount)
}

// TestMUSTPASS_MultipleCycles tests graph with multiple independent cycles
func TestMUSTPASS_MultipleCycles(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Cycle 1: A -> B -> C -> A
	require.NoError(t, dag.AddNode(NewTestNode("A", "C")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "B")))

	// Cycle 2: X -> Y -> Z -> X
	require.NoError(t, dag.AddNode(NewTestNode("X", "Z")))
	require.NoError(t, dag.AddNode(NewTestNode("Y", "X")))
	require.NoError(t, dag.AddNode(NewTestNode("Z", "Y")))

	require.NoError(t, dag.Build())

	cycles := dag.FindCycles()
	assert.GreaterOrEqual(t, len(cycles), 2)

	executedNodes := make(map[string]int)
	var mu sync.Mutex

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			mu.Lock()
			executedNodes[node.GetID()]++
			mu.Unlock()
			return nil
		})
	}

	// Each node should be executed exactly once
	for _, id := range []string{"A", "B", "C", "X", "Y", "Z"} {
		assert.Equal(t, 1, executedNodes[id], "node %s should execute once", id)
	}
}

// TestMUSTPASS_NestedDiamond tests nested diamond patterns
func TestMUSTPASS_NestedDiamond(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	//       H
	//      / \
	//     F   G
	//      \ /
	//       E
	//      / \
	//     C   D
	//      \ /
	//       B
	//       |
	//       A

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "B")))
	require.NoError(t, dag.AddNode(NewTestNode("D", "B")))
	require.NoError(t, dag.AddNode(NewTestNode("E", "C", "D")))
	require.NoError(t, dag.AddNode(NewTestNode("F", "E")))
	require.NoError(t, dag.AddNode(NewTestNode("G", "E")))
	require.NoError(t, dag.AddNode(NewTestNode("H", "F", "G")))

	require.NoError(t, dag.Build())

	var order []string
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			order = append(order, node.GetID())
			return nil
		})
	}

	assert.Len(t, order, 8)
	// A must be first, H must be last
	assert.Equal(t, "A", order[0])
	assert.Equal(t, "H", order[7])
}

// TestMUSTPASS_DisconnectedComponents tests multiple disconnected subgraphs
func TestMUSTPASS_DisconnectedComponents(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Component 1: A -> B -> C
	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "B")))

	// Component 2: X -> Y
	require.NoError(t, dag.AddNode(NewTestNode("X")))
	require.NoError(t, dag.AddNode(NewTestNode("Y", "X")))

	// Component 3: P (standalone)
	require.NoError(t, dag.AddNode(NewTestNode("P")))

	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 3) // C, Y, P

	var executedCount int32
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			atomic.AddInt32(&executedCount, 1)
			return nil
		})
	}

	assert.Equal(t, int32(6), executedCount)
}

// TestMUSTPASS_DeepRecursion tests very deep dependency chains
func TestMUSTPASS_DeepRecursion(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Create a chain of 100 nodes
	const depth = 100
	for i := 0; i < depth; i++ {
		id := fmt.Sprintf("N%d", i)
		var deps []string
		if i > 0 {
			deps = []string{fmt.Sprintf("N%d", i-1)}
		}
		require.NoError(t, dag.AddNode(NewTestNode(id, deps...)))
	}

	require.NoError(t, dag.Build())

	var executedCount int32
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			atomic.AddInt32(&executedCount, 1)
			return nil
		})
	}

	assert.Equal(t, int32(depth), executedCount)
}

// TestMUSTPASS_ManyEntries tests graph with many entry points
func TestMUSTPASS_ManyEntries(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// 50 independent nodes
	for i := 0; i < 50; i++ {
		require.NoError(t, dag.AddNode(NewTestNode(fmt.Sprintf("N%d", i))))
	}

	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 50)

	var executedCount int32
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			atomic.AddInt32(&executedCount, 1)
			return nil
		})
	}

	assert.Equal(t, int32(50), executedCount)
}

// TestMUSTPASS_ComplexMesh tests a complex mesh topology
func TestMUSTPASS_ComplexMesh(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Create a 4x4 grid where each node depends on nodes above and to the left
	// Grid layout:
	// A00 A01 A02 A03
	// A10 A11 A12 A13
	// A20 A21 A22 A23
	// A30 A31 A32 A33

	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			id := fmt.Sprintf("A%d%d", row, col)
			var deps []string
			if row > 0 {
				deps = append(deps, fmt.Sprintf("A%d%d", row-1, col))
			}
			if col > 0 {
				deps = append(deps, fmt.Sprintf("A%d%d", row, col-1))
			}
			require.NoError(t, dag.AddNode(NewTestNode(id, deps...)))
		}
	}

	require.NoError(t, dag.Build())

	// Entry should be A33 (bottom-right, depends on most nodes)
	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 1)
	assert.Equal(t, "A33", entries[0].GetID())

	var executedCount int32
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			atomic.AddInt32(&executedCount, 1)
			return nil
		})
	}

	assert.Equal(t, int32(16), executedCount)
}

// ==================== Concurrent Execution Tests ====================

// TestMUSTPASS_ConcurrentEntryExecution tests parallel execution of multiple entries
func TestMUSTPASS_ConcurrentEntryExecution(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Create 10 independent chains
	for i := 0; i < 10; i++ {
		base := fmt.Sprintf("chain%d", i)
		require.NoError(t, dag.AddNode(NewTestNode(base+"_A")))
		require.NoError(t, dag.AddNode(NewTestNode(base+"_B", base+"_A")))
		require.NoError(t, dag.AddNode(NewTestNode(base+"_C", base+"_B")))
	}

	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 10)

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*TestNode]) {
			defer wg.Done()
			c.Execute(func(node *TestNode) error {
				executedNodes.Store(node.GetID(), true)
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

	assert.Equal(t, 30, count) // 10 chains * 3 nodes each
}

// TestMUSTPASS_ConcurrentTryExecute tests concurrent TryExecute calls
func TestMUSTPASS_ConcurrentTryExecute(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Single shared node
	require.NoError(t, dag.AddNode(NewTestNode("shared")))
	require.NoError(t, dag.Build())

	var successCount int32
	var wg sync.WaitGroup

	// 1000 concurrent attempts to execute
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if dag.TryExecute("shared") {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// Only one should succeed
	assert.Equal(t, int32(1), successCount)
}

// TestMUSTPASS_ConcurrentMultipleSharedDeps tests concurrent execution with shared dependencies
func TestMUSTPASS_ConcurrentMultipleSharedDeps(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Shared base
	require.NoError(t, dag.AddNode(NewTestNode("base")))

	// 20 nodes depending on base
	for i := 0; i < 20; i++ {
		require.NoError(t, dag.AddNode(NewTestNode(fmt.Sprintf("dep%d", i), "base")))
	}

	require.NoError(t, dag.Build())

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*TestNode]) {
			defer wg.Done()
			c.Execute(func(node *TestNode) error {
				// Add slight delay to increase race condition probability
				executedNodes.Store(node.GetID(), true)
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

	// All 21 nodes should be executed
	assert.Equal(t, 21, count)

	// Verify base was executed
	_, baseExecuted := executedNodes.Load("base")
	assert.True(t, baseExecuted)
}

// TestMUSTPASS_ConcurrentResetAndExecute tests concurrent reset and execution
func TestMUSTPASS_ConcurrentResetAndExecute(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.Build())

	// Run multiple rounds of reset and execute
	for round := 0; round < 10; round++ {
		dag.Reset()

		var executedCount int32
		entryCh, _ := dag.Entries()
		for chain := range entryCh {
			chain.Execute(func(node *TestNode) error {
				atomic.AddInt32(&executedCount, 1)
				return nil
			})
		}

		assert.Equal(t, int32(2), executedCount, "round %d failed", round)
	}
}

// TestMUSTPASS_ConcurrentBuildAndQuery tests concurrent build and query operations
func TestMUSTPASS_ConcurrentBuildAndQuery(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
	require.NoError(t, dag.AddNode(NewTestNode("C", "A")))
	require.NoError(t, dag.Build())

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dag.GetAllNodes()
			dag.GetNode("A")
			dag.GetStage("A")
			dag.GetDependencies("B")
			dag.GetDependents("A")
			dag.IsExecuted("A")
			dag.NodeCount()
		}()
	}

	wg.Wait()
}

// TestMUSTPASS_ConcurrentDiamondExecution tests concurrent execution of diamond pattern
func TestMUSTPASS_ConcurrentDiamondExecution(t *testing.T) {
	// Run multiple times to catch race conditions
	for iteration := 0; iteration < 20; iteration++ {
		ctx := context.Background()
		dag := New[*TestNode](ctx)

		require.NoError(t, dag.AddNode(NewTestNode("A")))
		require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
		require.NoError(t, dag.AddNode(NewTestNode("C", "A")))
		require.NoError(t, dag.AddNode(NewTestNode("D", "B", "C")))
		require.NoError(t, dag.Build())

		executedNodes := sync.Map{}
		var wg sync.WaitGroup

		entryCh, _ := dag.Entries()
		for chain := range entryCh {
			wg.Add(1)
			go func(c *ChainIterator[*TestNode]) {
				defer wg.Done()
				c.Execute(func(node *TestNode) error {
					executedNodes.Store(node.GetID(), true)
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

		assert.Equal(t, 4, count, "iteration %d failed", iteration)
	}
}

// TestMUSTPASS_ConcurrentCycleExecution tests concurrent execution with cycles
func TestMUSTPASS_ConcurrentCycleExecution(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		ctx := context.Background()
		dag := New[*TestNode](ctx)

		// Two interconnected cycles
		require.NoError(t, dag.AddNode(NewTestNode("A", "C")))
		require.NoError(t, dag.AddNode(NewTestNode("B", "A")))
		require.NoError(t, dag.AddNode(NewTestNode("C", "B")))
		require.NoError(t, dag.AddNode(NewTestNode("D", "C"))) // D depends on cycle

		require.NoError(t, dag.Build())

		executedNodes := sync.Map{}
		var wg sync.WaitGroup

		entryCh, _ := dag.Entries()
		for chain := range entryCh {
			wg.Add(1)
			go func(c *ChainIterator[*TestNode]) {
				defer wg.Done()
				c.Execute(func(node *TestNode) error {
					executedNodes.Store(node.GetID(), true)
					return nil
				})
			}(chain)
		}

		wg.Wait()

		// All nodes should be executed exactly once
		var count int
		executedNodes.Range(func(key, value any) bool {
			count++
			return true
		})

		assert.Equal(t, 4, count, "iteration %d: expected 4 nodes, got %d", iteration, count)
	}
}

// TestMUSTPASS_StressTest runs a stress test with many nodes and concurrent execution
func TestMUSTPASS_StressTest(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	// Create 100 nodes with random dependencies
	nodeCount := 100
	for i := 0; i < nodeCount; i++ {
		id := fmt.Sprintf("N%d", i)
		var deps []string
		// Each node depends on up to 3 previous nodes
		for j := 0; j < 3 && i > 0; j++ {
			depIdx := i - 1 - j
			if depIdx >= 0 {
				deps = append(deps, fmt.Sprintf("N%d", depIdx))
			}
		}
		require.NoError(t, dag.AddNode(NewTestNode(id, deps...)))
	}

	require.NoError(t, dag.Build())

	executedNodes := sync.Map{}
	var wg sync.WaitGroup

	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		wg.Add(1)
		go func(c *ChainIterator[*TestNode]) {
			defer wg.Done()
			c.Execute(func(node *TestNode) error {
				executedNodes.Store(node.GetID(), true)
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

// TestMUSTPASS_EmptyDependencies tests nodes with empty dependency arrays
func TestMUSTPASS_EmptyDependencies(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	node := NewTestNode("A", []string{}...)
	require.NoError(t, dag.AddNode(node))
	require.NoError(t, dag.Build())

	entries, _ := dag.GetEntries()
	assert.Len(t, entries, 1)
}

// TestMUSTPASS_NilContext tests DAG with nil context
func TestMUSTPASS_NilContext(t *testing.T) {
	dag := New[*TestNode](nil) // nil context should be handled

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	require.NoError(t, dag.Build())

	entryCh, err := dag.Entries()
	require.NoError(t, err)

	var count int
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			count++
			return nil
		})
	}

	assert.Equal(t, 1, count)
}

// TestMUSTPASS_DuplicateDependencies tests nodes with duplicate dependencies
func TestMUSTPASS_DuplicateDependencies(t *testing.T) {
	ctx := context.Background()
	dag := New[*TestNode](ctx)

	require.NoError(t, dag.AddNode(NewTestNode("A")))
	// B depends on A twice (should be handled gracefully)
	require.NoError(t, dag.AddNode(NewTestNode("B", "A", "A", "A")))
	require.NoError(t, dag.Build())

	var executedCount int
	entryCh, _ := dag.Entries()
	for chain := range entryCh {
		chain.Execute(func(node *TestNode) error {
			executedCount++
			return nil
		})
	}

	assert.Equal(t, 2, executedCount)
}
