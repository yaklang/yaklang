package hnsw

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"math/rand"
	"slices"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
)

type Vector = func() []float32

// FilterFunc is a callback function used to filter nodes during search.
// It returns true if the node should be included in the results, false otherwise.
type FilterFunc[K cmp.Ordered] func(key K, vector Vector) bool

// Node is a node in the graph.
type Node[K cmp.Ordered] struct {
	Key   K
	Value Vector
}

func MakeNode[K cmp.Ordered](key K, vec []float32) Node[K] {
	return Node[K]{Key: key, Value: func() []float32 { return vec }}
}
func MakeNodeFuncVec[K cmp.Ordered](key K, vec Vector) Node[K] {
	return Node[K]{Key: key, Value: vec}
}

// LayerNode is a node in a layer of the graph.
type LayerNode[K cmp.Ordered] struct {
	Node[K]

	// Neighbors is map of neighbor keys to neighbor nodes.
	// It is a map and not a slice to allow for efficient deletes, esp.
	// when M is high.
	Neighbors map[K]*LayerNode[K]
}

// addNeighbor adds a o neighbor to the node, replacing the neighbor
// with the worst distance if the neighbor set is full.
func (n *LayerNode[K]) addNeighbor(newNode *LayerNode[K], m int, dist DistanceFunc) {
	if n.Neighbors == nil {
		n.Neighbors = make(map[K]*LayerNode[K], m)
	}

	n.Neighbors[newNode.Key] = newNode
	if len(n.Neighbors) <= m {
		return
	}

	// Find the neighbor with the worst distance.
	var (
		worstDist = float32(math.Inf(-1))
		worst     *LayerNode[K]
	)
	for _, neighbor := range n.Neighbors {
		d := dist(neighbor.Value, n.Value)
		// d > worstDist may always be false if the distance function
		// returns NaN, e.g., when the embeddings are zero.
		if d > worstDist || worst == nil {
			worstDist = d
			worst = neighbor
		}
	}

	delete(n.Neighbors, worst.Key)
	// Delete backlink from the worst neighbor.
	delete(worst.Neighbors, n.Key)
	worst.replenish(m)
}

type searchCandidate[K cmp.Ordered] struct {
	node *LayerNode[K]
	dist float32
}

func (s searchCandidate[K]) Less(o searchCandidate[K]) bool {
	return s.dist < o.dist
}

// search returns the layer node closest to the target node
// within the same layer.
func (n *LayerNode[K]) search(
	// k is the number of candidates in the result set.
	k int,
	efSearch int,
	target Vector,
	distance DistanceFunc,
	filter FilterFunc[K],
) []searchCandidate[K] {
	// This is a basic greedy algorithm to find the entry point at the given level
	// that is closest to the target node.
	candidates := NewHeap[searchCandidate[K]]()
	candidates.Init(make([]searchCandidate[K], 0, efSearch))
	candidates.Push(
		searchCandidate[K]{
			node: n,
			dist: distance(n.Value, target),
		},
	)
	var (
		result  = NewHeap[searchCandidate[K]]()
		visited = make(map[K]bool)
	)
	result.Init(make([]searchCandidate[K], 0, k))

	// Begin with the entry node in the result set (if it passes the filter).
	if candidates.Len() > 0 {
		entryCandidate := candidates.Min()
		if filter == nil || filter(entryCandidate.node.Key, entryCandidate.node.Value) {
			result.Push(entryCandidate)
		}
	}
	visited[n.Key] = true

	for candidates.Len() > 0 {
		var (
			current  = candidates.Pop().node
			improved = false
		)

		// We iterate the map in a sorted, deterministic fashion for
		// tests.
		neighborKeys := maps.Keys(current.Neighbors)
		slices.Sort(neighborKeys)
		for _, neighborID := range neighborKeys {
			neighbor := current.Neighbors[neighborID]
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			// Apply filter if provided
			if filter != nil && !filter(neighbor.Key, neighbor.Value) {
				continue
			}

			dist := distance(neighbor.Value, target)
			if result.Len() > 0 {
				improved = improved || dist < result.Min().dist
			} else {
				improved = true
			}
			if result.Len() < k {
				result.Push(searchCandidate[K]{node: neighbor, dist: dist})
			} else if dist < result.Max().dist {
				result.PopLast()
				result.Push(searchCandidate[K]{node: neighbor, dist: dist})
			}

			candidates.Push(searchCandidate[K]{node: neighbor, dist: dist})
			// Always store candidates if we haven't reached the limit.
			if candidates.Len() > efSearch {
				candidates.PopLast()
			}
		}

		// Termination condition: no improvement in distance and at least
		// kMin candidates in the result set.
		if !improved && result.Len() >= k {
			break
		}

		// Early termination if we can't find any more valid candidates
		if result.Len() == 0 && candidates.Len() == 0 {
			break
		}
	}

	return result.Slice()
}

func (n *LayerNode[K]) replenish(m int) {
	if len(n.Neighbors) >= m {
		return
	}

	// Restore connectivity by adding new neighbors.
	// This is a naive implementation that could be improved by
	// using a priority queue to find the best candidates.
	for _, neighbor := range n.Neighbors {
		for key, candidate := range neighbor.Neighbors {
			if _, ok := n.Neighbors[key]; ok {
				// do not add duplicates
				continue
			}
			if candidate == n {
				continue
			}
			n.addNeighbor(candidate, m, CosineDistance)
			if len(n.Neighbors) >= m {
				return
			}
		}
	}
}

// isolates remove the node from the graph by removing all connections
// to neighbors.
func (n *LayerNode[K]) isolate(m int) {
	for _, neighbor := range n.Neighbors {
		delete(neighbor.Neighbors, n.Key)
	}

	for _, neighbor := range n.Neighbors {
		neighbor.replenish(m)
	}
}

type Layer[K cmp.Ordered] struct {
	// Nodes is a map of Nodes IDs to Nodes.
	// All Nodes in a higher layer are also in the lower layers, an essential
	// property of the graph.
	//
	// Nodes is exported for interop with encoding/gob.
	Nodes map[K]*LayerNode[K]
}

// entry returns the entry node of the layer.
// It doesn't matter which node is returned, even that the
// entry node is consistent, so we just return the first node
// in the map to avoid tracking extra state.
func (l *Layer[K]) entry() *LayerNode[K] {
	if l == nil {
		return nil
	}
	for _, node := range l.Nodes {
		return node
	}
	return nil
}

func (l *Layer[K]) size() int {
	if l == nil {
		return 0
	}
	return len(l.Nodes)
}

// Graph is a Hierarchical Navigable Small World graph.
// All public parameters must be set before adding nodes to the graph.
// K is cmp.Ordered instead of of comparable so that they can be sorted.
type Graph[K cmp.Ordered] struct {
	// Distance is the distance function used to compare embeddings.
	Distance DistanceFunc

	// Rng is used for level generation. It may be set to a deterministic value
	// for reproducibility. Note that deterministic number generation can lead to
	// degenerate graphs when exposed to adversarial inputs.
	Rng *rand.Rand

	// M is the maximum number of neighbors to keep for each node.
	// A good default for OpenAI embeddings is 16.
	M int

	// Ml is the level generation factor.
	// E.g., for Ml = 0.25, each layer is 1/4 the size of the previous layer.
	Ml float64

	// EfSearch is the number of nodes to consider in the search phase.
	// 20 is a reasonable default. Higher values improve search accuracy at
	// the expense of memory.
	EfSearch int

	// Layers is a slice of Layers in the graph.
	Layers []*Layer[K]

	// OnLayersChange is called when the layers change.
	OnLayersChange func(Layers []*Layer[K])
}

func defaultRand() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// GraphConfig defines the configuration parameters for creating an HNSW graph
type GraphConfig[K cmp.Ordered] struct {
	// M is the maximum number of neighbors each node maintains
	// A good default for OpenAI embeddings is 16
	M int

	// Ml is the level generation factor
	// E.g., for Ml = 0.25, each layer is 1/4 the size of the previous layer
	Ml float64

	// Distance is the distance function used to compare embeddings
	Distance DistanceFunc

	// EfSearch is the number of nodes to consider in the search phase
	// 20 is a reasonable default. Higher values improve search accuracy at the expense of memory
	EfSearch int

	// Rng is used for level generation. It may be set to a deterministic value for reproducibility
	// Note: deterministic number generation can lead to degenerate graphs when exposed to adversarial inputs
	Rng *rand.Rand
}

// GraphOption defines the configuration option function type
type GraphOption[K cmp.Ordered] func(*GraphConfig[K])

// DefaultGraphConfig returns the default graph configuration
func DefaultGraphConfig[K cmp.Ordered]() *GraphConfig[K] {
	return &GraphConfig[K]{
		M:        16,
		Ml:       0.25,
		Distance: CosineDistance,
		EfSearch: 20,
		Rng:      defaultRand(),
	}
}

// WithM sets the maximum number of neighbors
func WithM[K cmp.Ordered](m int) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.M = m
	}
}

// WithMl sets the level generation factor
func WithMl[K cmp.Ordered](ml float64) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.Ml = ml
	}
}

// WithDistance sets the distance function
func WithDistance[K cmp.Ordered](distance DistanceFunc) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.Distance = distance
	}
}

// WithCosineDistance sets the distance function to cosine distance
func WithCosineDistance[K cmp.Ordered]() GraphOption[K] {
	return WithDistance[K](CosineDistance)
}

// WithEuclideanDistance sets the distance function to Euclidean distance
func WithEuclideanDistance[K cmp.Ordered]() GraphOption[K] {
	return WithDistance[K](EuclideanDistance)
}

// WithEfSearch sets the number of candidate nodes during search
func WithEfSearch[K cmp.Ordered](efSearch int) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.EfSearch = efSearch
	}
}

// WithRng sets the random number generator
func WithRng[K cmp.Ordered](rng *rand.Rand) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.Rng = rng
	}
}

// WithDeterministicRng sets a deterministic random number generator (for testing and reproducibility)
func WithDeterministicRng[K cmp.Ordered](seed int64) GraphOption[K] {
	return WithRng[K](rand.New(rand.NewSource(seed)))
}

// WithHNSWParameters sets the core HNSW parameters in batch
func WithHNSWParameters[K cmp.Ordered](m int, ml float64, efSearch int) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.M = m
		config.Ml = ml
		config.EfSearch = efSearch
	}
}

// NewGraphWithConfig creates a new HNSW graph with the specified configuration
func NewGraph[K cmp.Ordered](options ...GraphOption[K]) *Graph[K] {
	config := DefaultGraphConfig[K]()

	// Apply all configuration options
	for _, option := range options {
		option(config)
	}

	// Validate configuration parameters
	if config.M <= 0 {
		panic("M (max neighbors) must be greater than 0")
	}
	if config.Ml <= 0 || config.Ml > 1 {
		panic("Ml (level generation factor) must be between 0 and 1")
	}
	if config.EfSearch <= 0 {
		panic("EfSearch must be greater than 0")
	}
	if config.Distance == nil {
		panic("Distance function must be set")
	}
	if config.Rng == nil {
		config.Rng = defaultRand()
	}

	return &Graph[K]{
		M:        config.M,
		Ml:       config.Ml,
		Distance: config.Distance,
		EfSearch: config.EfSearch,
		Rng:      config.Rng,
	}
}

// maxLevel returns an upper-bound on the number of levels in the graph
// based on the size of the base layer.
func maxLevel(ml float64, numNodes int) int {
	if ml == 0 {
		panic("ml must be greater than 0")
	}

	if numNodes == 0 {
		return 1
	}

	l := math.Log(float64(numNodes))
	l /= math.Log(1 / ml)

	m := int(math.Round(l)) + 1

	return m
}

// randomLevel generates a random level for a new node.
func (h *Graph[K]) randomLevel() int {
	// max avoids having to accept an additional parameter for the maximum level
	// by calculating a probably good one from the size of the base layer.
	max := 1
	if len(h.Layers) > 0 {
		if h.Ml == 0 {
			panic("(*Graph).Ml must be greater than 0")
		}
		max = maxLevel(h.Ml, h.Layers[0].size())
	}

	for level := 0; level < max; level++ {
		if h.Rng == nil {
			h.Rng = defaultRand()
		}
		r := h.Rng.Float64()
		if r > h.Ml {
			return level
		}
	}

	return max
}

func (g *Graph[K]) assertDims(n Vector) {
	if len(g.Layers) == 0 {
		return
	}
	hasDims := g.Dims()
	if hasDims != len(n()) {
		panic(fmt.Sprint("embedding dimension mismatch: ", hasDims, " != ", len(n())))
	}
}

// Dims returns the number of dimensions in the graph, or
// 0 if the graph is empty.
func (g *Graph[K]) Dims() int {
	if len(g.Layers) == 0 {
		return 0
	}
	return len(g.Layers[0].entry().Value())
}

func ptr[T any](v T) *T {
	return &v
}

// Add inserts nodes into the graph.
// If another node with the same ID exists, it is replaced.
func (g *Graph[K]) Add(nodes ...Node[K]) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("recover from panic when adding nodes: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
		if g.OnLayersChange != nil {
			g.OnLayersChange(g.Layers)
		}
	}()
	for _, node := range nodes {
		key := node.Key
		vec := node.Value

		g.assertDims(vec)
		insertLevel := g.randomLevel()
		// Create layers that don't exist yet.
		for insertLevel >= len(g.Layers) {
			g.Layers = append(g.Layers, &Layer[K]{})
		}

		if insertLevel < 0 {
			panic("invalid level")
		}

		var elevator *K

		preLen := g.Len()

		// Insert node at each layer, beginning with the highest.
		for i := len(g.Layers) - 1; i >= 0; i-- {
			layer := g.Layers[i]
			newNode := &LayerNode[K]{
				Node: Node[K]{
					Key:   key,
					Value: vec,
				},
			}

			// Insert the new node into the layer.
			if layer.entry() == nil {
				layer.Nodes = map[K]*LayerNode[K]{key: newNode}
				continue
			}

			// Now at the highest layer with more than one node, so we can begin
			// searching for the best way to enter the graph.
			searchPoint := layer.entry()

			// On subsequent layers, we use the elevator node to enter the graph
			// at the best point.
			if elevator != nil {
				searchPoint = layer.Nodes[*elevator]
			}

			if g.Distance == nil {
				panic("(*Graph).Distance must be set")
			}

			neighborhood := searchPoint.search(g.M, g.EfSearch, vec, g.Distance, nil)
			if len(neighborhood) == 0 {
				// This should never happen because the searchPoint itself
				// should be in the result set.
				panic("no nodes found")
			}

			// Re-set the elevator node for the next layer.
			elevator = ptr(neighborhood[0].node.Key)

			if insertLevel >= i {
				if _, ok := layer.Nodes[key]; ok {
					g.Delete(key)
				}
				// Insert the new node into the layer.
				layer.Nodes[key] = newNode
				for _, node := range neighborhood {
					// Create a bi-directional edge between the new node and the best node.
					node.node.addNeighbor(newNode, g.M, g.Distance)
					newNode.addNeighbor(node.node, g.M, g.Distance)
				}
			}
		}

		// Invariant check: the node should have been added to the graph.
		if g.Len() != preLen+1 {
			if len(g.Layers) > 0 && g.Layers[len(g.Layers)-1].entry() == nil {
				g.Layers = g.Layers[:len(g.Layers)-1]
			}
		}
	}
}

// Search finds the k nearest neighbors from the target node.
func (h *Graph[K]) Search(near []float32, k int) []Node[K] {
	sr := h.search(func() []float32 { return near }, k, nil)
	out := make([]Node[K], len(sr))
	for i, node := range sr {
		out[i] = node.Node
	}
	return out
}

// SearchWithDistance finds the k nearest neighbors from the target node
// and returns the distance.
func (h *Graph[K]) SearchWithDistance(near []float32, k int) []SearchResult[K] {
	return h.search(func() []float32 { return near }, k, nil)
}

// SearchWithFilter finds the k nearest neighbors from the target node with a filter function.
// The filter function is called for each candidate node and should return true if the node
// should be included in the results.
func (h *Graph[K]) SearchWithFilter(near []float32, k int, filter FilterFunc[K]) []Node[K] {
	sr := h.search(func() []float32 { return near }, k, filter)
	out := make([]Node[K], len(sr))
	for i, node := range sr {
		out[i] = node.Node
	}
	return out
}

// SearchWithDistanceAndFilter finds the k nearest neighbors from the target node with a filter function
// and returns the distance.
func (h *Graph[K]) SearchWithDistanceAndFilter(near []float32, k int, filter FilterFunc[K]) []SearchResult[K] {
	return h.search(func() []float32 { return near }, k, filter)
}

type SearchResult[T cmp.Ordered] struct {
	Node[T]
	Distance float32
}

func (h *Graph[K]) search(near Vector, k int, filter FilterFunc[K]) []SearchResult[K] {
	h.assertDims(near)
	if len(h.Layers) == 0 {
		return nil
	}

	var (
		efSearch = h.EfSearch

		elevator *K
	)

	for layer := len(h.Layers) - 1; layer >= 0; layer-- {
		searchPoint := h.Layers[layer].entry()
		if elevator != nil {
			searchPoint = h.Layers[layer].Nodes[*elevator]
		}

		// Descending hierarchies
		if layer > 0 {
			nodes := searchPoint.search(1, efSearch, near, h.Distance, nil)
			elevator = ptr(nodes[0].node.Key)
			continue
		}

		nodes := searchPoint.search(k, efSearch, near, h.Distance, filter)
		out := make([]SearchResult[K], 0, len(nodes))

		for _, node := range nodes {
			out = append(out, SearchResult[K]{
				Node:     node.node.Node,
				Distance: node.dist,
			})
		}

		return out
	}

	panic("unreachable")
}

// Len returns the number of nodes in the graph.
func (h *Graph[K]) Len() int {
	if len(h.Layers) == 0 {
		return 0
	}
	return h.Layers[0].size()
}

// Delete removes a node from the graph by key.
// It tries to preserve the clustering properties of the graph by
// replenishing connectivity in the affected neighborhoods.
func (h *Graph[K]) Delete(key K) bool {
	defer func() {
		if h.OnLayersChange != nil {
			h.OnLayersChange(h.Layers)
		}
	}()
	if len(h.Layers) == 0 {
		return false
	}

	var deleteLayer = map[int]struct{}{}
	var deleted bool
	for i, layer := range h.Layers {
		node, ok := layer.Nodes[key]
		if !ok {
			continue
		}
		delete(layer.Nodes, key)
		if len(layer.Nodes) == 0 {
			deleteLayer[i] = struct{}{}
		}
		node.isolate(h.M)
		deleted = true
	}

	if len(deleteLayer) > 0 {
		var newLayers = make([]*Layer[K], 0, len(h.Layers)-len(deleteLayer))
		for i, layer := range h.Layers {
			if _, ok := deleteLayer[i]; ok {
				continue
			}
			newLayers = append(newLayers, layer)
		}

		h.Layers = newLayers
	}

	return deleted
}

// Lookup returns the vector with the given key.
func (h *Graph[K]) Lookup(key K) (Vector, bool) {
	if len(h.Layers) == 0 {
		return nil, false
	}

	node, ok := h.Layers[0].Nodes[key]
	if !ok {
		return nil, false
	}
	return node.Value, ok
}

// Export writes the graph to a writer.
//
// T must implement io.WriterTo.
func (h *Graph[K]) Export(w io.Writer) error {
	distFuncName, ok := distanceFuncToName(h.Distance)
	if !ok {
		return fmt.Errorf("distance function %v must be registered with RegisterDistanceFunc", h.Distance)
	}
	_, err := multiBinaryWrite(
		w,
		encodingVersion,
		h.M,
		h.Ml,
		h.EfSearch,
		distFuncName,
	)
	if err != nil {
		return fmt.Errorf("encode parameters: %w", err)
	}
	_, err = binaryWrite(w, len(h.Layers))
	if err != nil {
		return fmt.Errorf("encode number of layers: %w", err)
	}
	for _, layer := range h.Layers {
		_, err = binaryWrite(w, len(layer.Nodes))
		if err != nil {
			return fmt.Errorf("encode number of nodes: %w", err)
		}
		for _, node := range layer.Nodes {
			_, err = multiBinaryWrite(w, node.Key, node.Value, len(node.Neighbors))
			if err != nil {
				return fmt.Errorf("encode node data: %w", err)
			}

			for neighbor := range node.Neighbors {
				_, err = binaryWrite(w, neighbor)
				if err != nil {
					return fmt.Errorf("encode neighbor %v: %w", neighbor, err)
				}
			}
		}
	}

	return nil
}

// Import reads the graph from a reader.
// T must implement io.ReaderFrom.
// The imported graph does not have to match the exported graph's parameters (except for
// dimensionality). The graph will converge onto the new parameters.
func (h *Graph[K]) Import(r io.Reader) error {
	var (
		version int
		dist    string
	)
	_, err := multiBinaryRead(r, &version, &h.M, &h.Ml, &h.EfSearch,
		&dist,
	)
	if err != nil {
		return err
	}

	var ok bool
	h.Distance, ok = distanceFuncs[dist]
	if !ok {
		return fmt.Errorf("unknown distance function %q", dist)
	}
	if h.Rng == nil {
		h.Rng = defaultRand()
	}

	if version != encodingVersion {
		return fmt.Errorf("incompatible encoding version: %d", version)
	}

	var nLayers int
	_, err = binaryRead(r, &nLayers)
	if err != nil {
		return err
	}

	h.Layers = make([]*Layer[K], nLayers)
	for i := 0; i < nLayers; i++ {
		var nNodes int
		_, err = binaryRead(r, &nNodes)
		if err != nil {
			return err
		}

		nodes := make(map[K]*LayerNode[K], nNodes)
		for j := 0; j < nNodes; j++ {
			var key K
			var vec Vector
			var nNeighbors int
			_, err = multiBinaryRead(r, &key, &vec, &nNeighbors)
			if err != nil {
				return fmt.Errorf("decoding node %d: %w", j, err)
			}

			neighbors := make([]K, nNeighbors)
			for k := 0; k < nNeighbors; k++ {
				var neighbor K
				_, err = binaryRead(r, &neighbor)
				if err != nil {
					return fmt.Errorf("decoding neighbor %d for node %d: %w", k, j, err)
				}
				neighbors[k] = neighbor
			}

			node := &LayerNode[K]{
				Node: Node[K]{
					Key:   key,
					Value: vec,
				},
				Neighbors: make(map[K]*LayerNode[K]),
			}

			nodes[key] = node
			for _, neighbor := range neighbors {
				node.Neighbors[neighbor] = nil
			}
		}
		// Fill in neighbor pointers
		for _, node := range nodes {
			for key := range node.Neighbors {
				node.Neighbors[key] = nodes[key]
			}
		}
		h.Layers[i] = &Layer[K]{Nodes: nodes}
	}

	return nil
}
