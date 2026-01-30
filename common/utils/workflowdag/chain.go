package workflowdag

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

// ExecuteConcurrent executes nodes concurrently where possible
// Nodes at the same stage can be executed in parallel
// The handler is called concurrently for nodes whose dependencies are all satisfied
func (c *ChainIterator[T]) ExecuteConcurrent(handler func(node T) error, concurrency int) error {
	if concurrency <= 0 {
		concurrency = 1
	}

	// Use a simple sequential execution for now
	// TODO: Implement true concurrent execution with worker pool
	return c.Execute(handler)
}
