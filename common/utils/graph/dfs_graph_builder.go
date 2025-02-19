package graph

// DFSGraphBuilder is a utility struct to construct a graph using Depth-First Search (DFS) traversal.
type DFSGraphBuilder[K comparable, T any] struct {
	// Function to generate a unique key for a given node
	getNodeKey func(T) (K, error)

	// Function to retrieve the neighboring nodes and their edge types for a given node key
	getNeighbors func(K) []*NeighborWithEdgeType[T]

	// Function to process an edge between two nodes (e.g., store it, compute on it, etc.)
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any)

	// Map to track visited nodes during DFS traversal
	visited map[K]bool
}

// NeighborWithEdgeType Structure combining neighboring nodes with their edge types
type NeighborWithEdgeType[T any] struct {
	// The neighboring node
	Node T

	// The type of edge connecting the current node to the neighbor
	EdgeType string
	ExtraMsg map[string]any
}

func (n *NeighborWithEdgeType[T]) AddExtraMsg(k string, v any) {
	if n.ExtraMsg == nil {
		n.ExtraMsg = map[string]any{}
	}
	n.ExtraMsg[k] = v
}

// BuildGraph Depth-First Search driven graph construction method
func (g *DFSGraphBuilder[K, T]) BuildGraph(startNode T) {
	// Start the graph construction by creating the start node
	g.createNode(startNode)
}

// Helper function to create a node and initiate its DFS traversal
func (g *DFSGraphBuilder[K, T]) createNode(node T) (K, error) {
	nodeKey, err := g.getNodeKey(node)
	if err != nil {
		return nodeKey, err
	}
	g.dfs(nodeKey, node)
	return nodeKey, nil
}

// Internal implementation of Depth-First Search (DFS)
func (g *DFSGraphBuilder[K, T]) dfs(nodeKey K, node T) {
	// Check if the node has already been visited
	if g.visited[nodeKey] {
		return
	}

	// Mark the current node as visited
	g.visited[nodeKey] = true
	// Retrieve the neighboring nodes and their edge types
	neighbors := g.getNeighbors(nodeKey)

	// Traverse each neighboring node
	for _, neighbor := range neighbors {
		// Recursively traverse the neighbor node
		neighborKey, err := g.createNode(neighbor.Node)
		if err != nil {
			continue
		}
		// Process the edge between the current node and its neighbor
		g.handleEdge(nodeKey, neighborKey, neighbor.EdgeType, neighbor.ExtraMsg)
	}
}

// NewDFSGraphBuilder Utility function: Create a new graph builder instance
func NewDFSGraphBuilder[K comparable, T any](
	getNodeKey func(T) (K, error), // Function to generate a unique key for a node
	getNeighbors func(K) []*NeighborWithEdgeType[T], // Function to retrieve neighboring nodes and edge types
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any), // Function to process an edge
) *DFSGraphBuilder[K, T] {
	// Initialize and return a new GraphBuilder instance
	return &DFSGraphBuilder[K, T]{
		getNodeKey:   getNodeKey,
		getNeighbors: getNeighbors,
		handleEdge:   handleEdge,
		visited:      make(map[K]bool),
	}
}

func NewNeighbor[T any](node T, edgeType string) *NeighborWithEdgeType[T] {
	return &NeighborWithEdgeType[T]{
		Node:     node,
		EdgeType: edgeType,
		ExtraMsg: map[string]any{},
	}
}
