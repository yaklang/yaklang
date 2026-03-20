package workflowdag

import "sync"

// ChainEntry represents an execution chain entry point
type ChainEntry[T DAGNode] interface {
	// GetEntryNode returns the entry node of this chain
	GetEntryNode() T

	// NextNodes returns the next batch of executable nodes
	// after a node completes execution
	NextNodes(completedNodeID string) []T

	// IsDone returns true if all nodes in this chain are executed
	IsDone() bool

	// GetAllNodes returns all nodes in this chain
	GetAllNodes() []T

	// GetStageIndex returns the execution stage of a node
	GetStageIndex(nodeID string) int
}

// ChainIterator implements ChainEntry and provides iteration over a chain
type ChainIterator[T DAGNode] struct {
	dag       *WorkflowDAG[T]
	entryNode T
	visiting  map[string]bool // tracks nodes currently in the recursion stack
}

// NewChainIterator creates a new chain iterator starting from the given entry node
func NewChainIterator[T DAGNode](dag *WorkflowDAG[T], entry T) *ChainIterator[T] {
	return &ChainIterator[T]{
		dag:       dag,
		entryNode: entry,
		visiting:  make(map[string]bool),
	}
}

// GetEntryNode returns the entry node of this chain
func (c *ChainIterator[T]) GetEntryNode() T {
	return c.entryNode
}

// NextNodes returns the next batch of nodes that can be executed
// after the given node completes
// Returns nil if all dependent nodes are already executed (cycle termination)
func (c *ChainIterator[T]) NextNodes(completedNodeID string) []T {
	c.dag.mu.RLock()
	defer c.dag.mu.RUnlock()

	var readyNodes []T

	// Get all nodes that depend on the completed node
	dependents := c.dag.rdeps[completedNodeID]

	for _, depID := range dependents {
		// Skip if already executed (cycle handling)
		if c.dag.executed.Has(depID) {
			continue
		}

		// Check if all dependencies are satisfied
		allDepsSatisfied := true
		for _, reqDepID := range c.dag.deps[depID] {
			// Check if the required dependency exists and is executed
			if _, exists := c.dag.nodes[reqDepID]; exists {
				if !c.dag.executed.Has(reqDepID) {
					allDepsSatisfied = false
					break
				}
			}
		}

		if allDepsSatisfied {
			if node, ok := c.dag.nodes[depID]; ok {
				readyNodes = append(readyNodes, node)
			}
		}
	}

	return readyNodes
}

// IsDone returns true if there are no more nodes to execute
func (c *ChainIterator[T]) IsDone() bool {
	c.dag.mu.RLock()
	defer c.dag.mu.RUnlock()

	// Check if all nodes reachable from entry are executed
	allNodes := c.collectReachableNodes()
	for _, node := range allNodes {
		if !c.dag.executed.Has(node.GetID()) {
			return false
		}
	}
	return true
}

// GetAllNodes returns all nodes reachable from this entry
func (c *ChainIterator[T]) GetAllNodes() []T {
	c.dag.mu.RLock()
	defer c.dag.mu.RUnlock()
	return c.collectReachableNodes()
}

// collectReachableNodes collects all nodes reachable from the entry node
// by following the dependency chain (entry -> dependents)
func (c *ChainIterator[T]) collectReachableNodes() []T {
	visited := make(map[string]bool)
	var result []T

	var collect func(nodeID string)
	collect = func(nodeID string) {
		if visited[nodeID] {
			return
		}
		visited[nodeID] = true

		if node, ok := c.dag.nodes[nodeID]; ok {
			result = append(result, node)

			// Follow reverse dependencies (nodes that depend on this node)
			for _, depID := range c.dag.rdeps[nodeID] {
				collect(depID)
			}
		}
	}

	collect(c.entryNode.GetID())
	return result
}

// GetStageIndex returns the stage index of a node
func (c *ChainIterator[T]) GetStageIndex(nodeID string) int {
	c.dag.mu.RLock()
	defer c.dag.mu.RUnlock()

	if stage, ok := c.dag.stages[nodeID]; ok {
		return stage
	}
	return -1
}

// Execute is a convenience method that executes nodes following the dependency chain
// It calls the provided handler for each node and automatically manages execution flow
// The handler should return nil on success, or an error to stop execution
// 
// Execution order: Dependencies are executed first (depth-first)
// For A->B->C (A depends on B, B depends on C), execution order is: C, B, A
func (c *ChainIterator[T]) Execute(handler func(node T) error) error {
	// Use recursive execution starting from entry node
	return c.executeNode(c.entryNode.GetID(), handler)
}

// executeNode recursively executes a node and its dependencies
// Dependencies are executed first (depth-first traversal)
// Cycles are detected by tracking nodes in the current recursion stack
func (c *ChainIterator[T]) executeNode(nodeID string, handler func(node T) error) error {
	// Check if already executed globally (another chain or earlier in this chain)
	if c.dag.IsExecuted(nodeID) {
		return nil
	}

	// Check if we're currently visiting this node (cycle detection)
	if c.visiting[nodeID] {
		// We're in a cycle, skip to prevent infinite recursion
		return nil
	}

	node, exists := c.dag.GetNode(nodeID)
	if !exists {
		return nil
	}

	// Mark as visiting (in current recursion stack)
	c.visiting[nodeID] = true

	// First, execute all dependencies (depth-first)
	for _, depID := range c.dag.GetDependencies(nodeID) {
		if err := c.executeNode(depID, handler); err != nil {
			// Check if node allows failed dependencies
			if !node.AllowFailed() {
				c.visiting[nodeID] = false
				return err
			}
		}
	}

	// Unmark visiting
	c.visiting[nodeID] = false

	// Now try to execute this node
	if !c.dag.TryExecute(nodeID) {
		// Already executed by another path (concurrent or cycle)
		return nil
	}

	// Execute the node
	return handler(node)
}

// ExecuteConcurrent executes nodes concurrently where possible.
// Nodes at the same execution stage can run in parallel; later stages wait for
// all earlier-stage nodes to complete.
//
// concurrency controls the maximum number of nodes that may run simultaneously:
//   - > 0: at most concurrency nodes run in parallel
//   - 0 or negative: unlimited parallelism (all ready nodes start immediately)
func (c *ChainIterator[T]) ExecuteConcurrent(handler func(node T) error, concurrency int) error {
	if concurrency < 0 {
		concurrency = 0
	}

	// Collect all nodes that this entry will execute (the entry itself plus all
	// of its transitive dependencies, following the deps direction).
	execNodes := c.collectDepsNodes(c.entryNode.GetID(), make(map[string]bool))
	if len(execNodes) == 0 {
		return nil
	}

	// Group nodes by their execution stage so that independent nodes at the
	// same stage can run concurrently.
	stageMap := make(map[int][]T)
	maxStage := 0
	for _, node := range execNodes {
		stage := c.GetStageIndex(node.GetID())
		if stage < 0 {
			stage = 0
		}
		stageMap[stage] = append(stageMap[stage], node)
		if stage > maxStage {
			maxStage = stage
		}
	}

	// Execute each stage in ascending order; nodes within a stage run concurrently.
	for stage := 0; stage <= maxStage; stage++ {
		nodesAtStage := stageMap[stage]

		var wg sync.WaitGroup
		var firstErr error
		var errMu sync.Mutex

		// sem limits parallelism; a nil channel means unlimited concurrency.
		var sem chan struct{}
		if concurrency > 0 {
			sem = make(chan struct{}, concurrency)
		}

		for _, node := range nodesAtStage {
			nodeID := node.GetID()

			// Claim exclusive execution rights for this node.
			// If another goroutine (or a prior chain) already claimed it, skip.
			if !c.dag.TryExecute(nodeID) {
				continue
			}

			wg.Add(1)
			n := node

			// Acquire a slot from the semaphore before spawning the goroutine so
			// that we never have more than concurrency goroutines running at once.
			if sem != nil {
				sem <- struct{}{}
			}

			go func() {
				defer wg.Done()
				if sem != nil {
					defer func() { <-sem }()
				}

				if err := handler(n); err != nil {
					if !n.AllowFailed() {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
					}
				}
			}()
		}

		// Wait for all goroutines in this stage before advancing.
		wg.Wait()

		errMu.Lock()
		err := firstErr
		errMu.Unlock()
		if err != nil {
			return err
		}
	}

	return nil
}

// collectDepsNodes collects the given node plus all of its transitive
// dependencies (following the deps/DependsOn direction).  Already-executed
// nodes are excluded, and visiting is tracked to handle cycles safely.
func (c *ChainIterator[T]) collectDepsNodes(nodeID string, visiting map[string]bool) []T {
	// Skip already-executed nodes (claimed by another chain or stage).
	if c.dag.IsExecuted(nodeID) {
		return nil
	}

	// Cycle guard.
	if visiting[nodeID] {
		return nil
	}

	node, exists := c.dag.GetNode(nodeID)
	if !exists {
		return nil
	}

	visiting[nodeID] = true
	defer func() { visiting[nodeID] = false }()

	var result []T

	// Recursively collect dependencies first.
	for _, depID := range c.dag.GetDependencies(nodeID) {
		result = append(result, c.collectDepsNodes(depID, visiting)...)
	}

	// Append the node itself after its dependencies so each node appears at
	// most once and always after the nodes it depends on.
	result = append(result, node)
	return result
}
