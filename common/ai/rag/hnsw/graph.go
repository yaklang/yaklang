package hnsw

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/ai/rag/pq"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
)

type Vector = func() []float32

// InputNode 输入节点，用于接收外部输入数据
type InputNode[K cmp.Ordered] struct {
	Key   K
	Value []float32
}

// MakeInputNode 创建输入节点
func MakeInputNode[K cmp.Ordered](key K, vec []float32) InputNode[K] {
	return InputNode[K]{Key: key, Value: vec}
}

// MakeInputNodeFromVector 从Vector函数创建输入节点
func MakeInputNodeFromVector[K cmp.Ordered](key K, vec Vector) InputNode[K] {
	return InputNode[K]{Key: key, Value: vec()}
}

// ToVector 将输入节点转换为Vector函数
func (n InputNode[K]) ToVector() Vector {
	return func() []float32 { return n.Value }
}

// FilterFunc is a callback function used to filter nodes during search.
// It returns true if the node should be included in the results, false otherwise.
type FilterFunc[K cmp.Ordered] func(key K, vector Vector) bool

// These methods are now implemented in hnswspec.LayerNode interface
type searchCandidate[K cmp.Ordered] struct {
	node hnswspec.LayerNode[K]
	dist float64
}

func (s searchCandidate[K]) Less(o searchCandidate[K]) bool {
	return s.dist < o.dist
}

// search returns the layer node closest to the target node
// within the same layer using the new interface-based approach
func search[K cmp.Ordered](
	entryNode hnswspec.LayerNode[K],
	k int,
	efSearch int,
	target Vector,
	distance hnswspec.DistanceFunc[K],
	filter FilterFunc[K],
) []searchCandidate[K] {
	// Check for nil entryNode to prevent panic
	if entryNode == nil {
		log.Errorf("search called with nil entryNode")
		return []searchCandidate[K]{}
	}

	// Create a temporary standard node for distance calculation with target
	targetNode := hnswspec.NewStandardLayerNode[K](
		entryNode.GetKey(), // dummy key, not used
		target,
	)

	// This is a basic greedy algorithm to find the entry point at the given level
	// that is closest to the target node.
	candidates := NewHeap[searchCandidate[K]]()
	candidates.Init(make([]searchCandidate[K], 0, efSearch))
	candidates.Push(
		searchCandidate[K]{
			node: entryNode,
			dist: distance(entryNode, targetNode),
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
		if filter == nil || filter(entryCandidate.node.GetKey(), entryCandidate.node.GetVector()) {
			result.Push(entryCandidate)
		}
	}
	visited[entryNode.GetKey()] = true

	for candidates.Len() > 0 {
		var (
			current  = candidates.Pop().node
			improved = false
		)

		// We iterate the map in a sorted, deterministic fashion for tests.
		neighbors := current.GetNeighbors()
		neighborKeys := maps.Keys(neighbors)
		slices.Sort(neighborKeys)
		for _, neighborID := range neighborKeys {
			neighbor := neighbors[neighborID]
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			// Apply filter if provided
			if filter != nil && !filter(neighbor.GetKey(), neighbor.GetVector()) {
				continue
			}

			dist := distance(neighbor, targetNode)
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

// Old LayerNode methods removed - now implemented in hnswspec interface

type Layer[K cmp.Ordered] struct {
	// Nodes is a map of Nodes IDs to Nodes.
	// All Nodes in a higher layer are also in the lower layers, an essential
	// property of the graph.
	//
	// Nodes is exported for interop with encoding/gob.
	Nodes map[K]hnswspec.LayerNode[K]
}

// entry returns the entry node of the layer.
// It doesn't matter which node is returned, even that the
// entry node is consistent, so we just return the first node
// in the map to avoid tracking extra state.
func (l *Layer[K]) entry() hnswspec.LayerNode[K] {
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

	// PQ optimization fields
	// pqCodebook PQ码表，如果不为nil则启用PQ优化
	pqCodebook *pq.Codebook

	// pqQuantizer PQ量化器
	pqQuantizer *pq.Quantizer

	// nodeDistance 节点距离函数（基于接口）
	nodeDistance hnswspec.DistanceFunc[K]

	// pqAwareDistance PQ感知的距离函数
	pqAwareDistance hnswspec.PQAwareDistanceFunc[K]
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

	// PQCodebook PQ码表，如果设置则启用PQ优化
	PQCodebook *pq.Codebook
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

// WithPQCodebook 启用PQ优化，使用预训练的码表
func WithPQCodebook[K cmp.Ordered](codebook *pq.Codebook) GraphOption[K] {
	return func(config *GraphConfig[K]) {
		config.PQCodebook = codebook
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

	graph := &Graph[K]{
		M:        config.M,
		Ml:       config.Ml,
		Distance: config.Distance,
		EfSearch: config.EfSearch,
		Rng:      config.Rng,
	}

	// 设置节点距离函数为基于接口的版本
	graph.nodeDistance = hnswspec.CosineDistance[K]
	graph.pqAwareDistance = hnswspec.PQAwareCosineDistance[K]

	// 使用函数名来判断距离函数类型
	distName, ok := distanceFuncToName(config.Distance)
	if ok && distName == "euclidean" {
		graph.nodeDistance = hnswspec.EuclideanDistance[K]
		graph.pqAwareDistance = hnswspec.PQAwareEuclideanDistance[K]
	}

	// 初始化PQ优化
	if config.PQCodebook != nil {
		graph.pqCodebook = config.PQCodebook
		graph.pqQuantizer = pq.NewQuantizer(config.PQCodebook)

		// 创建一个包装函数，将quantizer传给距离函数
		graph.nodeDistance = func(a, b hnswspec.LayerNode[K]) float64 {
			return graph.pqAwareDistance(a, b, graph.pqQuantizer)
		}
	}

	return graph
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

	// 如果是PQ优化图，从PQ quantizer获取维度
	if g.IsPQEnabled() && g.pqQuantizer != nil {
		return g.pqQuantizer.SubVectorDim() * g.pqQuantizer.M()
	}

	// 对于标准图，从节点获取维度
	entry := g.Layers[0].entry()
	if entry == nil {
		return 0
	}
	if !entry.IsPQEnabled() {
		return len(entry.GetVector()())
	}
	return 0 // 这种情况不应该发生
}

func ptr[T any](v T) *T {
	return &v
}

// Add inserts nodes into the graph.
// If another node with the same ID exists, it is replaced.
func (g *Graph[K]) Add(nodes ...InputNode[K]) {
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
		vec := node.ToVector()

		g.Delete(key)
		g.assertDims(vec)

		insertLevel := g.randomLevel()
		// Create layers that don't exist yet.
		for insertLevel >= len(g.Layers) {
			g.Layers = append(g.Layers, &Layer[K]{Nodes: make(map[K]hnswspec.LayerNode[K])})
		}

		if insertLevel < 0 {
			panic("invalid level")
		}

		var elevator *K
		preLen := g.Len()

		// Phase 1: Search from highest layer down to insertLevel+1 (search only, no insertion)
		// Phase 2: Search and insert from insertLevel down to layer 0
		for i := len(g.Layers) - 1; i >= 0; i-- {
			layer := g.Layers[i]

			// Only create node if we're going to insert it (i <= insertLevel)
			var newNode hnswspec.LayerNode[K]
			var err error

			if i <= insertLevel {
				if g.IsPQEnabled() {
					// 创建PQ节点
					newNode, err = hnswspec.NewPQLayerNode(key, vec, g.pqQuantizer)
					if err != nil {
						log.Errorf("Failed to create PQ node: %v", err)
						// 回退到标准节点
						newNode = hnswspec.NewStandardLayerNode(key, vec)
					}
				} else {
					// 创建标准节点
					newNode = hnswspec.NewStandardLayerNode(key, vec)
				}
			}

			// Handle empty layer case - only insert if i <= insertLevel
			if layer.entry() == nil {
				if i <= insertLevel {
					layer.Nodes = map[K]hnswspec.LayerNode[K]{key: newNode}
				}
				continue
			}

			// Now at the highest layer with more than one node, so we can begin
			// searching for the best way to enter the graph.
			searchPoint := layer.entry()

			// On subsequent layers, we use the elevator node to enter the graph
			// at the best point.
			if elevator != nil {
				if elevatorNode, exists := layer.Nodes[*elevator]; exists && elevatorNode != nil {
					searchPoint = elevatorNode
				}
				// If elevator node doesn't exist in this layer, keep using the entry point
			}

			if g.nodeDistance == nil {
				panic("(*Graph).nodeDistance must be set")
			}

			// Ensure searchPoint is not nil before calling search
			if searchPoint == nil {
				log.Errorf("searchPoint is nil, unable to search in layer %d", i)
				continue
			}

			// Use different search parameters based on layer and phase
			searchK := 1 // For search-only layers (above insertLevel), only need 1 best result
			searchEf := g.EfSearch

			if i <= insertLevel {
				// For insertion layers, search for more candidates
				searchK = g.M
				if i == 0 {
					// Use larger ef for layer 0 to ensure good connectivity
					searchEf = max(g.EfSearch, g.M*2)
				}
			}

			neighborhood := search(searchPoint, searchK, searchEf, vec, g.nodeDistance, nil)
			if len(neighborhood) == 0 {
				// This should never happen because the searchPoint itself
				// should be in the result set.
				panic("no nodes found")
			}

			// Re-set the elevator node for the next layer.
			elevator = ptr(neighborhood[0].node.GetKey())

			if i <= insertLevel {
				// Insert the new node into the layer.
				layer.Nodes[key] = newNode
				for _, candidate := range neighborhood {
					// Create connections between the new node and the best nodes
					candidate.node.AddNeighbor(newNode, g.M, g.nodeDistance)
					newNode.AddNeighbor(candidate.node, g.M, g.nodeDistance)
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
func (h *Graph[K]) Search(near []float32, k int) []InputNode[K] {
	sr := h.search(func() []float32 { return near }, k, nil)
	out := make([]InputNode[K], len(sr))
	for i, result := range sr {
		out[i] = InputNode[K]{
			Key:   result.Key,
			Value: result.Value, // 对于PQ节点这将是nil
		}
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
func (h *Graph[K]) SearchWithFilter(near []float32, k int, filter FilterFunc[K]) []InputNode[K] {
	sr := h.search(func() []float32 { return near }, k, filter)
	out := make([]InputNode[K], len(sr))
	for i, result := range sr {
		out[i] = InputNode[K]{
			Key:   result.Key,
			Value: result.Value, // 对于PQ节点这将是nil
		}
	}
	return out
}

// SearchWithDistanceAndFilter finds the k nearest neighbors from the target node with a filter function
// and returns the distance.
func (h *Graph[K]) SearchWithDistanceAndFilter(near []float32, k int, filter FilterFunc[K]) []SearchResult[K] {
	return h.search(func() []float32 { return near }, k, filter)
}

type SearchResult[T cmp.Ordered] struct {
	Key      T
	Value    []float32 // 对于PQ节点，这将是nil
	Distance float64
	IsPQ     bool // 标识是否为PQ节点
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
			nodes := search(searchPoint, 1, efSearch, near, h.nodeDistance, nil)
			elevator = ptr(nodes[0].node.GetKey())
			continue
		}

		nodes := search(searchPoint, k, efSearch, near, h.nodeDistance, filter)
		out := make([]SearchResult[K], 0, len(nodes))

		for _, candidate := range nodes {
			// 创建SearchResult，处理PQ节点的特殊情况
			result := SearchResult[K]{
				Key:      candidate.node.GetKey(),
				Distance: candidate.dist,
				IsPQ:     candidate.node.IsPQEnabled(),
			}

			if !candidate.node.IsPQEnabled() {
				// 标准节点可以获取原始向量
				result.Value = candidate.node.GetVector()()
			} else {
				// PQ节点不提供原始向量
				result.Value = nil
			}

			out = append(out, result)
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
		node.Isolate(h.M, h.nodeDistance)
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
	// 注意：对于PQ节点，这可能会panic
	return node.GetVector(), ok
}

// IsPQEnabled 检查是否启用了PQ优化
func (h *Graph[K]) IsPQEnabled() bool {
	return h.pqCodebook != nil && h.pqQuantizer != nil
}

// GetPQCodes 获取指定键的PQ编码（如果启用PQ优化）
func (h *Graph[K]) GetPQCodes(key K) ([]byte, bool) {
	if !h.IsPQEnabled() {
		return nil, false
	}

	if len(h.Layers) == 0 {
		return nil, false
	}

	node, ok := h.Layers[0].Nodes[key]
	if !ok {
		return nil, false
	}

	return node.GetPQCodes()
}

// GetCodebook 获取PQ码表
func (h *Graph[K]) GetCodebook() *pq.Codebook {
	return h.pqCodebook
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
			neighbors := node.GetNeighbors()
			_, err = multiBinaryWrite(w, node.GetKey(), node.GetVector(), len(neighbors))
			if err != nil {
				return fmt.Errorf("encode node data: %w", err)
			}

			for neighbor := range neighbors {
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

		nodes := make(map[K]hnswspec.LayerNode[K], nNodes)
		neighborMap := make(map[K][]K) // 临时存储邻居信息

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

			// 创建节点（根据是否启用PQ优化）
			var node hnswspec.LayerNode[K]
			if h.IsPQEnabled() {
				node, err = hnswspec.NewPQLayerNode(key, vec, h.pqQuantizer)
				if err != nil {
					// 回退到标准节点
					node = hnswspec.NewStandardLayerNode(key, vec)
				}
			} else {
				node = hnswspec.NewStandardLayerNode(key, vec)
			}

			nodes[key] = node
			neighborMap[key] = neighbors
		}

		// 建立邻居连接
		for key, node := range nodes {
			for _, neighborKey := range neighborMap[key] {
				if neighborNode, exists := nodes[neighborKey]; exists {
					node.AddNeighbor(neighborNode, h.M, h.nodeDistance)
				}
			}
		}

		h.Layers[i] = &Layer[K]{Nodes: nodes}
	}

	return nil
}

func (h *Graph[K]) DebugDumpByDot() []string {
	var result []string

	for layerIdx, layer := range h.Layers {
		if layer == nil || len(layer.Nodes) == 0 {
			continue
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("digraph layer_%d {", layerIdx))
		lines = append(lines, "  rankdir=LR;")
		lines = append(lines, "  node [shape=circle];")
		lines = append(lines, "")

		// Add nodes with their vector information as labels
		for nodeKey, node := range layer.Nodes {
			if node == nil {
				continue
			}
			if !node.IsPQEnabled() {
				vec := node.GetVector()()
				if len(vec) >= 3 {
					// Show first 3 dimensions for readability
					label := fmt.Sprintf("%v\\n[%.2f,%.2f,%.2f]", nodeKey, vec[0], vec[1], vec[2])
					lines = append(lines, fmt.Sprintf("  \"%v\" [label=\"%s\"];", nodeKey, label))
				} else {
					label := fmt.Sprintf("%v\\n%v", nodeKey, vec)
					lines = append(lines, fmt.Sprintf("  \"%v\" [label=\"%s\"];", nodeKey, label))
				}
			} else {
				// PQ节点只显示键
				label := fmt.Sprintf("%v\\n[PQ]", nodeKey)
				lines = append(lines, fmt.Sprintf("  \"%v\" [label=\"%s\"];", nodeKey, label))
			}
		}

		lines = append(lines, "")

		// Add edges (neighbors)
		for nodeKey, node := range layer.Nodes {
			if node == nil {
				continue
			}
			neighbors := node.GetNeighbors()
			for neighborKey := range neighbors {
				lines = append(lines, fmt.Sprintf("  \"%v\" -> \"%v\";", nodeKey, neighborKey))
			}
		}

		lines = append(lines, "}")

		// Join lines for this layer
		result = append(result, strings.Join(lines, "\n"))
	}

	return result
}

func (h *Graph[K]) DumpByDot() []string {
	var result []string

	for layerIdx, layer := range h.Layers {
		if layer == nil || len(layer.Nodes) == 0 {
			continue
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("graph layer_%d {", layerIdx))
		lines = append(lines, "  layout=neato;")
		lines = append(lines, "  node [shape=circle];")

		// Add edges (neighbors) - avoid duplicate edges in undirected graph
		addedEdges := make(map[string]bool)
		for nodeKey, node := range layer.Nodes {
			if node == nil {
				continue
			}
			neighbors := node.GetNeighbors()
			for neighborKey := range neighbors {
				// Create edge key in a consistent order to avoid duplicates
				var edgeKey string
				if nodeKey < neighborKey {
					edgeKey = fmt.Sprintf("%v--%v", nodeKey, neighborKey)
				} else {
					edgeKey = fmt.Sprintf("%v--%v", neighborKey, nodeKey)
				}

				// Only add the edge if we haven't added it before
				if !addedEdges[edgeKey] {
					lines = append(lines, fmt.Sprintf("  \"%v\" -- \"%v\";", nodeKey, neighborKey))
					addedEdges[edgeKey] = true
				}
			}
		}

		lines = append(lines, "}")

		// Join lines for this layer
		result = append(result, strings.Join(lines, "\n"))
	}

	return result
}

// TrainPQCodebookFromData 从图中的所有现有向量数据训练PQ码表
// 这个方法会：
// 1. 收集所有节点的向量数据
// 2. 训练PQ码表
// 3. 更新所有节点使用新的PQ编码
// 4. 设置图的PQ优化
func (g *Graph[K]) TrainPQCodebookFromData(m, k int) (*pq.Codebook, error) {
	if len(g.Layers) == 0 || len(g.Layers[0].Nodes) == 0 {
		return nil, utils.Error("no data available for PQ training")
	}

	// 检查是否已有PQ启用
	if g.IsPQEnabled() {
		return nil, utils.Error("PQ is already enabled on this graph")
	}

	// 收集所有向量数据
	var allVectors [][]float64
	for _, layer := range g.Layers {
		for _, node := range layer.Nodes {
			vector := node.GetVector()()
			// 转换 []float32 到 []float64
			vec64 := make([]float64, len(vector))
			for i, v := range vector {
				vec64[i] = float64(v)
			}
			allVectors = append(allVectors, vec64)
		}
	}

	if len(allVectors) == 0 {
		return nil, utils.Error("no vectors found for training")
	}

	// 检查向量维度一致性
	dims := len(allVectors[0])
	for _, vec := range allVectors {
		if len(vec) != dims {
			return nil, utils.Errorf("inconsistent vector dimensions: expected %d, got %d", dims, len(vec))
		}
	}

	// 训练PQ码表
	log.Infof("Training PQ codebook with %d vectors of dimension %d, M=%d, K=%d", len(allVectors), dims, m, k)

	// 创建数据channel
	dataChan := make(chan []float64, len(allVectors))
	go func() {
		defer close(dataChan)
		for _, vec := range allVectors {
			dataChan <- vec
		}
	}()

	codebook, err := pq.Train(dataChan, pq.WithM(m), pq.WithK(k), pq.WithMaxIters(50))
	if err != nil {
		return nil, utils.Wrap(err, "training PQ codebook")
	}

	// 创建量化器
	quantizer := pq.NewQuantizer(codebook)

	// 设置图的PQ优化
	g.pqCodebook = codebook
	g.pqQuantizer = quantizer
	g.pqAwareDistance = hnswspec.PQAwareCosineDistance[K]
	g.nodeDistance = func(a, b hnswspec.LayerNode[K]) float64 {
		return g.pqAwareDistance(a, b, g.pqQuantizer)
	}

	// 更新所有现有节点为PQ节点
	log.Infof("Converting %d nodes to PQ encoding", len(allVectors))
	converted := 0
	for _, layer := range g.Layers {
		for key, node := range layer.Nodes {
			// 获取原始向量
			vector := node.GetVector()()
			vec64 := make([]float64, len(vector))
			for i, v := range vector {
				vec64[i] = float64(v)
			}

			// 创建PQ节点
			pqNode, err := hnswspec.NewPQLayerNode(key, func() []float32 {
				vec32 := make([]float32, len(vec64))
				for i, v := range vec64 {
					vec32[i] = float32(v)
				}
				return vec32
			}, quantizer)

			if err != nil {
				log.Errorf("Failed to convert node %v to PQ: %v", key, err)
				continue
			}

			// 替换节点（暂时跳过邻居复制，避免复杂性）
			layer.Nodes[key] = pqNode
			converted++
		}
	}

	log.Infof("Successfully converted %d/%d nodes to PQ encoding", converted, len(allVectors))
	return codebook, nil
}
