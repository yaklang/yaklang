package hnsw

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

// TestHNSWLayerStructure tests that the HNSW layer structure follows correct algorithm rules
func TestHNSWLayerStructure(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	graph := NewGraph[int]()

	// Add many nodes to build a multi-layer structure
	nodeCount := 100
	for i := 0; i < nodeCount; i++ {
		// Create diverse vectors to ensure good distribution
		vec := make([]float32, 10)
		for j := 0; j < 10; j++ {
			vec[j] = float32(i*10+j) / 100.0
		}
		graph.Add(MakeInputNode(i, vec))
	}

	t.Logf("Built graph with %d nodes across %d layers", graph.Len(), len(graph.Layers))

	// Verify layer structure properties
	if len(graph.Layers) == 0 {
		t.Fatal("Graph should have at least one layer")
	}

	// Layer 0 should have the most nodes (all nodes)
	layer0Size := len(graph.Layers[0].Nodes)
	if layer0Size != nodeCount {
		t.Errorf("Layer 0 should have all %d nodes, but has %d", nodeCount, layer0Size)
	}

	// Higher layers should have fewer nodes (pyramid structure)
	for i := 1; i < len(graph.Layers); i++ {
		currentLayerSize := len(graph.Layers[i].Nodes)
		previousLayerSize := len(graph.Layers[i-1].Nodes)

		if currentLayerSize > previousLayerSize {
			t.Errorf("Layer %d has %d nodes, which is more than layer %d with %d nodes. HNSW layers should form a pyramid.",
				i, currentLayerSize, i-1, previousLayerSize)
		}

		// Verify all nodes in higher layer also exist in lower layers
		for nodeKey := range graph.Layers[i].Nodes {
			if _, exists := graph.Layers[i-1].Nodes[nodeKey]; !exists {
				t.Errorf("Node %v exists in layer %d but not in layer %d. All nodes in higher layers must exist in lower layers.",
					nodeKey, i, i-1)
			}
		}

		t.Logf("Layer %d: %d nodes (%.1f%% of layer %d)",
			i, currentLayerSize,
			float64(currentLayerSize)*100.0/float64(previousLayerSize),
			i-1)
	}
}

// TestHNSWInsertionLevelRespected tests that nodes are only inserted at their assigned level and below
func TestHNSWInsertionLevelRespected(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	graph := NewGraph[int]()

	// Add a few nodes first to establish some layers
	for i := 0; i < 10; i++ {
		vec := make([]float32, 5)
		for j := 0; j < 5; j++ {
			vec[j] = float32(i*5+j) / 25.0
		}
		graph.Add(MakeInputNode(i, vec))
	}

	initialLayers := len(graph.Layers)
	t.Logf("Initial graph has %d layers", initialLayers)

	// Now add nodes one by one and track their distribution
	for i := 10; i < 20; i++ {
		preAddState := make(map[int]map[int]bool) // layer -> nodeKey -> exists
		for layerIdx, layer := range graph.Layers {
			preAddState[layerIdx] = make(map[int]bool)
			for nodeKey := range layer.Nodes {
				preAddState[layerIdx][nodeKey] = true
			}
		}

		vec := make([]float32, 5)
		for j := 0; j < 5; j++ {
			vec[j] = float32(i*5+j) / 25.0
		}
		graph.Add(MakeInputNode(i, vec))

		// Check that the new node was added correctly
		nodeFound := false
		for layerIdx, layer := range graph.Layers {
			if _, exists := layer.Nodes[i]; exists {
				nodeFound = true
				t.Logf("Node %d found in layer %d", i, layerIdx)

				// Verify the node exists in all lower layers
				for lowerLayerIdx := 0; lowerLayerIdx < layerIdx; lowerLayerIdx++ {
					if _, existsInLower := graph.Layers[lowerLayerIdx].Nodes[i]; !existsInLower {
						t.Errorf("Node %d exists in layer %d but not in lower layer %d", i, layerIdx, lowerLayerIdx)
					}
				}
			}
		}

		if !nodeFound {
			t.Errorf("Node %d was not found in any layer after insertion", i)
		}
	}
}

// TestHNSWConnectivityIntegrity tests that the graph maintains proper connectivity
func TestHNSWConnectivityIntegrity(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	graph := NewGraph[int]()

	// Add nodes to build a connected graph
	nodeCount := 50
	for i := 0; i < nodeCount; i++ {
		vec := make([]float32, 8)
		for j := 0; j < 8; j++ {
			vec[j] = float32(i*8+j) / 400.0
		}
		graph.Add(MakeInputNode(i, vec))
	}

	// Verify connectivity in each layer
	for layerIdx, layer := range graph.Layers {
		if len(layer.Nodes) <= 1 {
			continue // Skip layers with 0 or 1 nodes
		}

		totalConnections := 0
		for nodeKey, node := range layer.Nodes {
			neighbors := node.GetNeighbors()
			connectionCount := len(neighbors)
			totalConnections += connectionCount

			t.Logf("Layer %d, Node %v: %d connections", layerIdx, nodeKey, connectionCount)

			// Verify all neighbors exist in the same layer
			for neighborKey := range neighbors {
				if _, exists := layer.Nodes[neighborKey]; !exists {
					t.Errorf("Layer %d: Node %v has neighbor %v that doesn't exist in the same layer",
						layerIdx, nodeKey, neighborKey)
				}
			}

			// Verify bi-directional connectivity
			for neighborKey, neighborNode := range neighbors {
				neighborNeighbors := neighborNode.GetNeighbors()
				if _, isConnectedBack := neighborNeighbors[nodeKey]; !isConnectedBack {
					t.Errorf("Layer %d: Node %v is connected to %v, but %v is not connected back to %v",
						layerIdx, nodeKey, neighborKey, neighborKey, nodeKey)
				}
			}
		}

		avgConnections := float64(totalConnections) / float64(len(layer.Nodes))
		t.Logf("Layer %d: Average %.2f connections per node", layerIdx, avgConnections)

		// Each node should have at least some connections (except in very small layers)
		if len(layer.Nodes) > 3 && avgConnections == 0 {
			t.Errorf("Layer %d has no connections, which indicates a connectivity problem", layerIdx)
		}
	}
}
