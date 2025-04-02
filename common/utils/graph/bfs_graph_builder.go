package graph

type BFSGraphBuilder[K, T comparable] struct {
	// Function to generate a unique key for a given node
	getNodeKey func(T) (K, error)
	// Function to retrieve the neighboring nodes and their edge types for a given node key
	getNeighbors func(T) map[T]*Neighbor[T]
	// Function to process an edge between two nodes (e.g., store it, compute on it, etc.)
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any)
}

func NewBFSGraphBuilder[K, T comparable](
	getNodeKey func(T) (K, error),
	getNeighbors func(T) map[T]*Neighbor[T],
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any),
) *BFSGraphBuilder[K, T] {
	return &BFSGraphBuilder[K, T]{
		getNodeKey:   getNodeKey,
		getNeighbors: getNeighbors,
		handleEdge:   handleEdge,
	}
}

func (g *BFSGraphBuilder[K, T]) bfs(from, to T) error {
	var buildFrom, buildTo func(T)
	nodeList := make(map[T][]T)
	visitedKey := make(map[T]K)

	getVisitedKey := func(node T) K {
		if k, ok := visitedKey[node]; ok {
			return k
		}
		k, _ := g.getNodeKey(node)
		visitedKey[node] = k
		return k
	}

	buildFrom = func(node T) {
		neighbors := g.getNeighbors(node)
		for _, neighbor := range neighbors {
			neighborNode := neighbor.Node
			nodeList[neighborNode] = append(nodeList[neighborNode], node)
		}
		for _, neighbor := range neighbors {
			buildFrom(neighbor.Node)
		}
	}
	buildFrom(from)

	buildTo = func(node T) {
		nodeKey := getVisitedKey(node)
		for _, neighborNode := range nodeList[node] {
			neighborKey := getVisitedKey(neighborNode)

			neighbors := g.getNeighbors(neighborNode)
			g.handleEdge(neighborKey, nodeKey, neighbors[node].EdgeType, neighbors[node].ExtraMsg)
		}
		for _, neighborNode := range nodeList[node] {
			buildTo(neighborNode)
		}
	}
	buildTo(to)

	return nil
}

func (g *BFSGraphBuilder[K, T]) BuildGraph(from, to T) error {
	err := g.bfs(from, to)
	return err
}
