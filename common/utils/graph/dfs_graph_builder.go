package graph

import (
	"context"
)

// DFSGraphBuilder is a utility struct to construct a graph using Depth-First Search (DFS) traversal.
type DFSGraphBuilder[K, T comparable] struct {
	ctx context.Context
	// Function to generate a unique key for a given node
	getNodeKey func(T) (K, error)

	// Function to retrieve the neighboring nodes and their edge types for a given node key
	getNeighbors func(T) []*Neighbor[T]

	// Function to process an edge between two nodes (e.g., store it, compute on it, etc.)
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any)

	// Map to track visited nodes during DFS traversal
	visited map[T]K
}

// Neighbor Structure combining neighboring nodes with their edge types
type Neighbor[T any] struct {
	// The neighboring node
	Node T
	// The type of edge connecting the current node to the neighbor
	EdgeType string
	ExtraMsg map[string]any
}

func (n *Neighbor[T]) AddExtraMsg(k string, v any) {
	if n.ExtraMsg == nil {
		n.ExtraMsg = map[string]any{}
	}
	n.ExtraMsg[k] = v
}

// BuildGraph Depth-First Search driven graph construction method
func (g *DFSGraphBuilder[K, T]) BuildGraph(startNode T) error {
	_, err := g.dfs(startNode, true)
	return err
}

// Internal implementation of Depth-First Search (DFS)
func (g *DFSGraphBuilder[K, T]) dfs(node T, isroots ...bool) (K, error) {
	select {
	case <-g.ctx.Done():
		return g.visited[node], g.ctx.Err()
	default:
		// Continue with DFS traversal
	}

	isroot := false
	// if len(isroots) > 0 {
	// 	isroot = isroots[0]
	// }

	// Check if the node has already been visited
	if key, ok := g.GetVisited(node); ok {
		return key, nil
	}

	// Generate a unique key for the current node
	nodeKey, err := g.getNodeKey(node)
	if err != nil {
		return nodeKey, err
	}
	// Mark the current node as visited
	g.visited[node] = nodeKey

	// Retrieve the neighboring nodes and their edge types
	neighbors := g.getNeighbors(node)

	// Traverse each neighboring node
	for _, neighbor := range neighbors {
		// Recursively traverse the neighbor node
		neighborKey, err := g.dfs(neighbor.Node)
		if err != nil {
			return neighborKey, err
		}
		// Process the edge between the current node and its neighbor
		if isroot {
			continue
		}
		g.handleEdge(nodeKey, neighborKey, neighbor.EdgeType, neighbor.ExtraMsg)
	}
	return nodeKey, nil
}

func (g *DFSGraphBuilder[K, T]) GetVisited(node T) (key K, ok bool) {
	if key, ok = g.visited[node]; ok {
		return key, true
	}

	return key, false
}

func NewDFSGraphBuilder[K comparable, T comparable](
	ctx context.Context,
	getNodeKey func(T) (K, error), // Function to generate a unique key for a node
	getNeighbors func(T) []*Neighbor[T], // Function to retrieve neighboring nodes and edge types
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any), // Function to process an edge
) *DFSGraphBuilder[K, T] {
	// Initialize and return a new GraphBuilder instance
	builder := &DFSGraphBuilder[K, T]{
		ctx:          context.Background(),
		getNodeKey:   getNodeKey,
		getNeighbors: getNeighbors,
		handleEdge:   handleEdge,
		visited:      make(map[T]K),
	}
	if ctx != nil {
		builder.ctx = ctx
	}
	return builder
}

func BuildGraphWithDFS[K comparable, T comparable](
	ctx context.Context,
	startNode T,
	getNodeKey func(T) (K, error),
	getNeighbors func(T) []*Neighbor[T],
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any),
) error {
	builder := NewDFSGraphBuilder[K, T](ctx, getNodeKey, getNeighbors, handleEdge)
	return builder.BuildGraph(startNode)
}

func NewNeighbor[T any](node T, edgeType string) *Neighbor[T] {
	return &Neighbor[T]{
		Node:     node,
		EdgeType: edgeType,
		ExtraMsg: map[string]any{},
	}
}
