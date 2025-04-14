package graph

type BFSGraphBuilder[K, T comparable] struct {
	// Function to generate a unique key for a given node
	getNodeKey           func(T) (K, error)
	getNeighborsDependOn func(T) map[T]*Neighbor[T]
	getNeighborsEffectOn func(T) map[T]*Neighbor[T]
	// Function to process an edge between two nodes (e.g., store it, compute on it, etc.)
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any)

	visited *map[T]map[T]bool
}

func NewBFSGraphBuilder[K, T comparable](
	getNodeKey func(T) (K, error),
	getNeighborsDependOn func(T) map[T]*Neighbor[T],
	getNeighborsEffectOn func(T) map[T]*Neighbor[T],
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any),
	visited *map[T]map[T]bool,
) *BFSGraphBuilder[K, T] {
	return &BFSGraphBuilder[K, T]{
		getNodeKey:           getNodeKey,
		getNeighborsDependOn: getNeighborsDependOn,
		getNeighborsEffectOn: getNeighborsEffectOn,
		handleEdge:           handleEdge,
		visited:              visited,
	}
}

func BuildGraphWithBFS[K comparable, T comparable](
	from, to T,
	getNodeKey func(T) (K, error),
	getNeighborsDependOn func(T) map[T]*Neighbor[T],
	getNeighborsEffectOn func(T) map[T]*Neighbor[T],
	handleEdge func(from K, to K, edgeType string, extraMsg map[string]any),
	visited *map[T]map[T]bool,
) error {
	builder := NewBFSGraphBuilder[K, T](getNodeKey, getNeighborsDependOn, getNeighborsEffectOn, handleEdge, visited)
	return builder.BuildGraph(from, to)
}

func (g *BFSGraphBuilder[K, T]) ResetVisited() {
	g.visited = &map[T]map[T]bool{}
}

func (g *BFSGraphBuilder[K, T]) bfs(from, to T) error {
	var buildNodeList, buildEdge func(T)
	var getNeighbors func(T) map[T]*Neighbor[T]
	var visitedNode map[T]bool
	var visitedEdge *map[T]map[T]bool
	var nodeList map[T][]T

	if g.visited != nil {
		visitedEdge = g.visited
	} else {
		visitedEdge = &map[T]map[T]bool{}
	}

	buildNodeList = func(node T) {
		if visitedNode[node] {
			return
		}
		visitedNode[node] = true
		neighbors := getNeighbors(node)
		for _, neighbor := range neighbors {
			neighborNode := neighbor.Node
			nodeList[neighborNode] = append(nodeList[neighborNode], node)
		}
		for _, neighbor := range neighbors {
			buildNodeList(neighbor.Node)
		}
	}

	buildEdge = func(node T) {
		nodeKey, _ := g.getNodeKey(node)
		for _, neighborNode := range nodeList[node] {
			if (*visitedEdge)[node][neighborNode] {
				return
			}
			if m, ok := (*visitedEdge)[node]; ok {
				m[neighborNode] = true
			} else {
				(*visitedEdge)[node] = map[T]bool{neighborNode: true}
			}

			neighborKey, _ := g.getNodeKey(neighborNode)
			neighbors := getNeighbors(neighborNode)
			g.handleEdge(neighborKey, nodeKey, neighbors[node].EdgeType, neighbors[node].ExtraMsg)

			buildEdge(neighborNode)
		}
	}

	first := g.getNeighborsDependOn
	next := g.getNeighborsEffectOn
	if len(g.getNeighborsDependOn(from)) == 0 {
		first = g.getNeighborsEffectOn
		next = g.getNeighborsDependOn
	}

	nodeList = make(map[T][]T)
	visitedNode = make(map[T]bool)
	getNeighbors = first
	buildNodeList(from)
	buildEdge(to)

	nodeList = make(map[T][]T)
	visitedNode = make(map[T]bool)
	getNeighbors = next
	buildNodeList(to)
	buildEdge(from)

	return nil
}

func (g *BFSGraphBuilder[K, T]) BuildGraph(from, to T) error {
	err := g.bfs(from, to)
	return err
}
