package hnsw

import (
	"testing"

	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
)

// TestSearchWithNilEntry tests the search function with nil entryNode
func TestSearchWithNilEntry(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	// Test search function directly with nil entryNode
	results := search[string](
		nil, // nil entryNode should not cause panic
		5,   // k
		10,  // efSearch
		func() []float32 { return []float32{1.0, 2.0, 3.0} }, // target vector
		hnswspec.EuclideanDistance[string],                   // distance function
		nil,                                                  // filter
	)

	// Should return empty results instead of panicking
	if len(results) != 0 {
		t.Errorf("Expected empty results for nil entryNode, got %d results", len(results))
	}
}

// TestGraphAddWithRepeatedOperations tests the Add method with repeated operations
// that previously caused nil pointer panics
func TestGraphAddWithRepeatedOperations(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	graph := NewGraph[string]()

	// Add nodes multiple times to trigger the bug
	for i := 0; i < 5; i++ {
		graph.Add(
			MakeInputNode("test1", []float32{1.0, 2.0, 3.0, 4.0, 5.0}),
			MakeInputNode("test2", []float32{2.0, 3.0, 4.0, 5.0, 6.0}),
			MakeInputNode("test3", []float32{3.0, 4.0, 5.0, 6.0, 7.0}),
		)

		// Try searching after each add operation
		results := graph.Search([]float32{1.5, 2.5, 3.5, 4.5, 5.5}, 5)

		if len(results) == 0 {
			t.Errorf("Expected search results after iteration %d, got none", i)
		}
	}
}

// TestGraphWithEmptyLayers tests operations on graph with empty layers
func TestGraphWithEmptyLayers(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	graph := NewGraph[string]()

	// First add a node to establish dimension
	graph.Add(MakeInputNode("first", []float32{1.0, 2.0, 3.0}))

	// Create empty layers manually to test edge cases
	graph.Layers = append(graph.Layers, &Layer[string]{Nodes: make(map[string]hnswspec.LayerNode[string])})

	// Try to add another node when there are empty layers
	graph.Add(MakeInputNode("test", []float32{4.0, 5.0, 6.0}))

	// Should not panic and should add the node successfully
	if graph.Len() != 2 {
		t.Errorf("Expected 2 nodes after adding to graph with empty layers, got %d", graph.Len())
	}
}

// TestLayerEntry tests the Layer.entry() method edge cases
func TestLayerEntry(t *testing.T) {
	// Test nil layer
	var layer *Layer[string] = nil
	entry := layer.entry()
	if entry != nil {
		t.Errorf("Expected nil entry for nil layer, got %v", entry)
	}

	// Test empty layer
	emptyLayer := &Layer[string]{Nodes: make(map[string]hnswspec.LayerNode[string])}
	entry = emptyLayer.entry()
	if entry != nil {
		t.Errorf("Expected nil entry for empty layer, got %v", entry)
	}

	// Test layer with nodes
	node := hnswspec.NewStandardLayerNode("test", func() []float32 { return []float32{1.0, 2.0} })
	layerWithNode := &Layer[string]{Nodes: map[string]hnswspec.LayerNode[string]{"test": node}}
	entry = layerWithNode.entry()
	if entry == nil {
		t.Error("Expected non-nil entry for layer with nodes")
	}
	if entry.GetKey() != "test" {
		t.Errorf("Expected entry key 'test', got '%s'", entry.GetKey())
	}
}
