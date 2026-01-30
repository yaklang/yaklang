package workflowdag

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// WorkflowDAG is a generic DAG-based workflow engine
// It supports multiple entry points, dependency management, and concurrent execution
type WorkflowDAG[T DAGNode] struct {
	ctx context.Context

	// nodes stores all nodes by ID
	nodes map[string]T

	// deps stores dependency relationships: nodeID -> IDs of nodes it depends on
	deps map[string][]string

	// rdeps stores reverse dependencies: nodeID -> IDs of nodes that depend on it
	rdeps map[string][]string

	// stages stores the stage/level of each node for visualization
	stages map[string]int

	// executed tracks which nodes have been executed (for cycle handling)
	executed *utils.Set[string]

	// entries stores all entry nodes (nodes not depended on by others)
	entries []T

	// built indicates whether Build() has been called
	built bool

	mu sync.RWMutex
}

// New creates a new WorkflowDAG instance
func New[T DAGNode](ctx context.Context) *WorkflowDAG[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	return &WorkflowDAG[T]{
		ctx:      ctx,
		nodes:    make(map[string]T),
		deps:     make(map[string][]string),
		rdeps:    make(map[string][]string),
		stages:   make(map[string]int),
		executed: utils.NewSet[string](),
		entries:  nil,
		built:    false,
	}
}

// AddNode adds a single node to the DAG
func (dag *WorkflowDAG[T]) AddNode(node T) error {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	id := node.GetID()
	if _, exists := dag.nodes[id]; exists {
		return ErrDuplicateNode
	}

	dag.nodes[id] = node
	dag.deps[id] = node.DependsOn()
	dag.built = false
	return nil
}

// AddNodes adds multiple nodes to the DAG
func (dag *WorkflowDAG[T]) AddNodes(nodes ...T) error {
	for _, node := range nodes {
		if err := dag.AddNode(node); err != nil {
			return err
		}
	}
	return nil
}

// GetNode returns a node by its ID
func (dag *WorkflowDAG[T]) GetNode(id string) (T, bool) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	node, ok := dag.nodes[id]
	return node, ok
}

// GetAllNodes returns all nodes in the DAG
func (dag *WorkflowDAG[T]) GetAllNodes() []T {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	nodes := make([]T, 0, len(dag.nodes))
	for _, node := range dag.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// Build validates the DAG and prepares it for execution
// It calculates reverse dependencies, finds entry nodes, and computes stages
func (dag *WorkflowDAG[T]) Build() error {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	if len(dag.nodes) == 0 {
		return ErrEmptyDAG
	}

	// Build reverse dependencies
	dag.rdeps = make(map[string][]string)
	for id := range dag.nodes {
		dag.rdeps[id] = []string{}
	}

	for id, deps := range dag.deps {
		for _, depID := range deps {
			// Note: We don't error on missing dependencies
			// This allows for flexible graph construction
			if _, exists := dag.nodes[depID]; exists {
				dag.rdeps[depID] = append(dag.rdeps[depID], id)
			}
		}
	}

	// Find entry nodes (nodes not depended on by any other node)
	dag.entries = dag.findEntries()

	// Calculate stages for visualization
	dag.calculateStages()

	dag.built = true
	return nil
}

// findEntries finds all entry nodes (nodes that are not depended on by others)
// In terms of execution, these are the starting points
// For disconnected components with cycles, each component gets one entry point
func (dag *WorkflowDAG[T]) findEntries() []T {
	// Collect all nodes that are depended on
	dependedNodes := make(map[string]bool)
	for _, deps := range dag.deps {
		for _, depID := range deps {
			dependedNodes[depID] = true
		}
	}

	// Find nodes that are not depended on
	var entries []T
	for id, node := range dag.nodes {
		if !dependedNodes[id] {
			entries = append(entries, node)
		}
	}

	// If no entries found (all nodes have dependencies, likely cycles),
	// find one entry per connected component
	if len(entries) == 0 && len(dag.nodes) > 0 {
		entries = dag.findComponentEntries()
	}

	return entries
}

// findComponentEntries finds one entry node per disconnected component
// This is used when all nodes are in cycles (no natural entry points)
func (dag *WorkflowDAG[T]) findComponentEntries() []T {
	visited := make(map[string]bool)
	var entries []T

	// Helper function to visit all connected nodes starting from a node
	// Uses both deps and rdeps to find all nodes in the same component
	var visitComponent func(nodeID string)
	visitComponent = func(nodeID string) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		// Visit nodes this node depends on
		for _, depID := range dag.deps[nodeID] {
			if _, exists := dag.nodes[depID]; exists {
				visitComponent(depID)
			}
		}

		// Visit nodes that depend on this node
		for _, rdepID := range dag.rdeps[nodeID] {
			visitComponent(rdepID)
		}
	}

	// Find all disconnected components
	for id, node := range dag.nodes {
		if !visited[id] {
			// This node is in a new component, use it as entry
			entries = append(entries, node)
			// Mark all nodes in this component as visited
			visitComponent(id)
		}
	}

	return entries
}

// calculateStages calculates the execution stage for each node
// Stage 0: Nodes with no dependencies (executed first)
// Stage N: Nodes depending on Stage N-1 nodes
// For cycles, nodes in a cycle get the same stage as the first node encountered
func (dag *WorkflowDAG[T]) calculateStages() {
	// Initialize all stages to -1 (not calculated)
	for id := range dag.nodes {
		dag.stages[id] = -1
	}

	// Track visiting state for cycle detection: 0=unvisited, 1=visiting, 2=done
	visiting := make(map[string]int)

	// Calculate stage for each node using DFS with cycle detection
	var calcStage func(id string) int
	calcStage = func(id string) int {
		// Already calculated
		if dag.stages[id] >= 0 {
			return dag.stages[id]
		}

		// Cycle detection: if we're revisiting a node in the current path, return 0
		if visiting[id] == 1 {
			// We're in a cycle, assign stage 0 to break the cycle
			dag.stages[id] = 0
			return 0
		}

		visiting[id] = 1 // Mark as visiting

		deps := dag.deps[id]
		if len(deps) == 0 {
			dag.stages[id] = 0
			visiting[id] = 2
			return 0
		}

		maxDepStage := -1
		for _, depID := range deps {
			if _, exists := dag.nodes[depID]; !exists {
				continue
			}
			depStage := calcStage(depID)
			if depStage > maxDepStage {
				maxDepStage = depStage
			}
		}

		dag.stages[id] = maxDepStage + 1
		visiting[id] = 2 // Mark as done
		return dag.stages[id]
	}

	for id := range dag.nodes {
		if visiting[id] == 0 {
			calcStage(id)
		}
	}
}

// Entries returns a channel of entry chain iterators
// Each entry can be consumed concurrently
func (dag *WorkflowDAG[T]) Entries() (<-chan *ChainIterator[T], error) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	if !dag.built {
		return nil, ErrDAGNotBuilt
	}

	ch := make(chan *ChainIterator[T], len(dag.entries))
	for _, entry := range dag.entries {
		ch <- NewChainIterator(dag, entry)
	}
	close(ch)

	return ch, nil
}

// GetEntries returns all entry nodes
func (dag *WorkflowDAG[T]) GetEntries() ([]T, error) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	if !dag.built {
		return nil, ErrDAGNotBuilt
	}

	result := make([]T, len(dag.entries))
	copy(result, dag.entries)
	return result, nil
}

// TryExecute attempts to mark a node as executed
// Returns true if this is the first execution (should proceed)
// Returns false if already executed (should skip - cycle point)
func (dag *WorkflowDAG[T]) TryExecute(nodeID string) bool {
	dag.mu.Lock()
	defer dag.mu.Unlock()

	if dag.executed.Has(nodeID) {
		return false
	}
	dag.executed.Add(nodeID)
	return true
}

// IsExecuted checks if a node has been executed
func (dag *WorkflowDAG[T]) IsExecuted(nodeID string) bool {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.executed.Has(nodeID)
}

// Reset clears the execution state, allowing the DAG to be run again
func (dag *WorkflowDAG[T]) Reset() {
	dag.mu.Lock()
	defer dag.mu.Unlock()
	dag.executed = utils.NewSet[string]()
}

// GetStage returns the stage/level of a node
func (dag *WorkflowDAG[T]) GetStage(nodeID string) (int, bool) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	stage, ok := dag.stages[nodeID]
	return stage, ok && stage >= 0
}

// GetStages returns all nodes grouped by their stages
func (dag *WorkflowDAG[T]) GetStages() (map[int][]T, error) {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	if !dag.built {
		return nil, ErrDAGNotBuilt
	}

	result := make(map[int][]T)
	for id, stage := range dag.stages {
		if stage >= 0 {
			result[stage] = append(result[stage], dag.nodes[id])
		}
	}
	return result, nil
}

// GetDependencies returns the IDs of nodes that a given node depends on
func (dag *WorkflowDAG[T]) GetDependencies(nodeID string) []string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.deps[nodeID]
}

// GetDependents returns the IDs of nodes that depend on a given node
func (dag *WorkflowDAG[T]) GetDependents(nodeID string) []string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return dag.rdeps[nodeID]
}

// FindCycles finds and returns all cycles in the DAG
// This is for informational purposes only; cycles don't cause errors
func (dag *WorkflowDAG[T]) FindCycles() [][]string {
	dag.mu.RLock()
	defer dag.mu.RUnlock()

	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: done
	var cycles [][]string
	var path []string

	var dfs func(nodeID string)
	dfs = func(nodeID string) {
		if visited[nodeID] == 1 {
			// Found a cycle, extract the cycle path
			cycleStart := -1
			for i, id := range path {
				if id == nodeID {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				cycle := make([]string, len(path)-cycleStart+1)
				copy(cycle, path[cycleStart:])
				cycle[len(cycle)-1] = nodeID
				cycles = append(cycles, cycle)
			}
			return
		}
		if visited[nodeID] == 2 {
			return
		}

		visited[nodeID] = 1
		path = append(path, nodeID)

		for _, depID := range dag.deps[nodeID] {
			if _, exists := dag.nodes[depID]; exists {
				dfs(depID)
			}
		}

		path = path[:len(path)-1]
		visited[nodeID] = 2
	}

	for nodeID := range dag.nodes {
		if visited[nodeID] == 0 {
			dfs(nodeID)
		}
	}

	return cycles
}

// NodeCount returns the number of nodes in the DAG
func (dag *WorkflowDAG[T]) NodeCount() int {
	dag.mu.RLock()
	defer dag.mu.RUnlock()
	return len(dag.nodes)
}
