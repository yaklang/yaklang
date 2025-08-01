package dot

import (
	"testing"
)

func TestGraphAnalysisConnectivity(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create a simple graph: A -> B -> C
	g.AddEdgeByLabel("A", "B", "edge1")
	g.AddEdgeByLabel("B", "C", "edge2")

	// Test connectivity
	if !g.IsConnected("A", "C") {
		t.Error("A should be connected to C")
	}

	if !g.IsConnected("A", "B") {
		t.Error("A should be connected to B")
	}

	if g.IsConnected("C", "A") {
		t.Error("C should not be connected to A in directed graph")
	}

	if !g.IsConnected("B", "C") {
		t.Error("B should be connected to C")
	}
}

func TestGraphAnalysisDirection(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create edges
	g.AddEdgeByLabel("A", "B", "forward")
	g.AddEdgeByLabel("B", "C", "forward")

	// Debug: Print all edges
	edges := g.GetAllEdges()
	t.Logf("All edges: %+v", edges)

	// Test direction
	if g.GetEdgeDirection("A", "B") != EdgeDirectionForward {
		t.Errorf("Expected forward direction, got %s", g.GetEdgeDirection("A", "B"))
	}

	if g.GetEdgeDirection("B", "A") != EdgeDirectionBackward {
		t.Errorf("Expected backward direction (since A->B exists), got %s", g.GetEdgeDirection("B", "A"))
	}

	// Test undirected graph
	g2 := New() // Undirected by default
	g2.AddEdgeByLabel("X", "Y", "bidirectional")

	if g2.GetEdgeDirection("X", "Y") != EdgeDirectionBidirectional {
		t.Errorf("Expected bidirectional direction in undirected graph, got %s", g2.GetEdgeDirection("X", "Y"))
	}
}

func TestGraphAnalysisEdgeLabels(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create edges with labels
	g.AddEdgeByLabel("A", "B", "label1")
	g.AddEdgeByLabel("B", "C", "label2")

	// Test single edge label
	if g.GetEdgeLabel("A", "B") != "label1" {
		t.Errorf("Expected label1, got %s", g.GetEdgeLabel("A", "B"))
	}

	if g.GetEdgeLabel("B", "C") != "label2" {
		t.Errorf("Expected label2, got %s", g.GetEdgeLabel("B", "C"))
	}

	// Test edge with specific label
	if !g.HasEdgeWithLabel("A", "B", "label1") {
		t.Error("Should have edge A->B with label1")
	}

	if g.HasEdgeWithLabel("A", "B", "wrong_label") {
		t.Error("Should not have edge A->B with wrong_label")
	}

	// Test multiple edges between same nodes
	g.AddEdgeByLabel("A", "B", "label3")
	labels := g.GetAllEdgeLabels("A", "B")
	if len(labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(labels))
	}

	expectedLabels := map[string]bool{"label1": true, "label3": true}
	for _, label := range labels {
		if !expectedLabels[label] {
			t.Errorf("Unexpected label: %s", label)
		}
	}
}

func TestGraphAnalysisShortestPath(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create a path: A -> B -> C -> D
	g.AddEdgeByLabel("A", "B", "edge1")
	g.AddEdgeByLabel("B", "C", "edge2")
	g.AddEdgeByLabel("C", "D", "edge3")

	// Test shortest path
	path := g.GetShortestPath("A", "D")
	expected := []string{"A", "B", "C", "D"}

	if len(path) != len(expected) {
		t.Errorf("Expected path length %d, got %d", len(expected), len(path))
	}

	for i, node := range path {
		if node != expected[i] {
			t.Errorf("Expected %s at position %d, got %s", expected[i], i, node)
		}
	}

	// Test no path
	noPath := g.GetShortestPath("D", "A")
	if noPath != nil {
		t.Error("Should not have path from D to A")
	}
}

func TestGraphAnalysisGetAllNodesAndEdges(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create a simple graph
	g.AddEdgeByLabel("A", "B", "edge1")
	g.AddEdgeByLabel("B", "C", "edge2")

	// Test GetAllNodes
	nodes := g.GetAllNodes()
	expectedNodes := map[string]bool{"A": true, "B": true, "C": true}

	if len(nodes) != len(expectedNodes) {
		t.Errorf("Expected %d nodes, got %d", len(expectedNodes), len(nodes))
	}

	for _, node := range nodes {
		if !expectedNodes[node] {
			t.Errorf("Unexpected node: %s", node)
		}
	}

	// Test GetAllEdges
	edges := g.GetAllEdges()
	if len(edges) != 2 {
		t.Errorf("Expected 2 edges, got %d", len(edges))
	}

	expectedEdges := map[string]bool{
		"A->B:edge1": true,
		"B->C:edge2": true,
	}

	for _, edge := range edges {
		edgeStr := edge.From + "->" + edge.To + ":" + edge.Label
		if !expectedEdges[edgeStr] {
			t.Errorf("Unexpected edge: %s", edgeStr)
		}
	}
}

func TestGraphAnalysisComplexGraph(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create a more complex graph
	// A -> B -> D
	// A -> C -> D
	// B -> C
	g.AddEdgeByLabel("A", "B", "edge1")
	g.AddEdgeByLabel("A", "C", "edge2")
	g.AddEdgeByLabel("B", "C", "edge3")
	g.AddEdgeByLabel("B", "D", "edge4")
	g.AddEdgeByLabel("C", "D", "edge5")

	// Test connectivity
	if !g.IsConnected("A", "D") {
		t.Error("A should be connected to D")
	}

	if !g.IsConnected("A", "C") {
		t.Error("A should be connected to C")
	}

	if !g.IsConnected("B", "D") {
		t.Error("B should be connected to D")
	}

	// Test multiple paths
	path1 := g.GetShortestPath("A", "D")
	if len(path1) != 3 { // A -> B -> D or A -> C -> D
		t.Errorf("Expected path length 3, got %d", len(path1))
	}

	// Test edge labels
	if g.GetEdgeLabel("A", "B") != "edge1" {
		t.Errorf("Expected edge1, got %s", g.GetEdgeLabel("A", "B"))
	}

	if g.GetEdgeLabel("B", "C") != "edge3" {
		t.Errorf("Expected edge3, got %s", g.GetEdgeLabel("B", "C"))
	}
}

func TestGraphAnalysisUndirectedGraph(t *testing.T) {
	g := New() // Undirected by default

	// Create undirected graph
	g.AddEdgeByLabel("A", "B", "edge1")
	g.AddEdgeByLabel("B", "C", "edge2")

	// Test connectivity in undirected graph
	if !g.IsConnected("A", "C") {
		t.Error("A should be connected to C in undirected graph")
	}

	if !g.IsConnected("C", "A") {
		t.Error("C should be connected to A in undirected graph")
	}

	// Test direction in undirected graph
	if g.GetEdgeDirection("A", "B") != EdgeDirectionBidirectional {
		t.Errorf("Expected bidirectional direction, got %s", g.GetEdgeDirection("A", "B"))
	}

	// Test shortest path in undirected graph
	path := g.GetShortestPath("A", "C")
	if len(path) != 3 {
		t.Errorf("Expected path length 3, got %d", len(path))
	}
}

func TestGraphAnalysisNodeLabels(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create nodes with specific labels
	g.AddEdgeByLabel("NodeA", "NodeB", "connection")

	// Test GetNodeLabel
	if g.GetNodeLabel("NodeA") != "NodeA" {
		t.Errorf("Expected NodeA, got %s", g.GetNodeLabel("NodeA"))
	}

	if g.GetNodeLabel("NodeB") != "NodeB" {
		t.Errorf("Expected NodeB, got %s", g.GetNodeLabel("NodeB"))
	}

	if g.GetNodeLabel("NonExistent") != "" {
		t.Error("Should return empty string for non-existent node")
	}
}

func TestGraphAnalysisEdgeWithSpecificLabel(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create edges with specific labels
	g.AddEdgeByLabel("A", "B", "data_flow")
	g.AddEdgeByLabel("B", "C", "control_flow")

	// Test HasEdgeWithLabel
	if !g.HasEdgeWithLabel("A", "B", "data_flow") {
		t.Error("Should have edge A->B with label 'data_flow'")
	}

	if !g.HasEdgeWithLabel("B", "C", "control_flow") {
		t.Error("Should have edge B->C with label 'control_flow'")
	}

	if g.HasEdgeWithLabel("A", "B", "wrong_label") {
		t.Error("Should not have edge A->B with wrong label")
	}

	if g.HasEdgeWithLabel("A", "C", "any_label") {
		t.Error("Should not have edge A->C")
	}
}

func TestGraphAnalysisNeighbors(t *testing.T) {
	g := New()
	g.MakeDirected()

	// Create a simple graph: A -> B -> C
	g.AddEdgeByLabel("A", "B", "edge1")
	g.AddEdgeByLabel("B", "C", "edge2")

	// Test direct neighbors
	if !g.IsNeighbor("A", "B") {
		t.Error("A and B should be neighbors")
	}

	if !g.IsNeighbor("B", "C") {
		t.Error("B and C should be neighbors")
	}

	// Test non-neighbors
	if g.IsNeighbor("A", "C") {
		t.Error("A and C should not be neighbors (they are connected but not directly)")
	}

	// Test non-existent nodes
	if g.IsNeighbor("A", "D") {
		t.Error("A and D should not be neighbors (D doesn't exist)")
	}

	if g.IsNeighbor("D", "E") {
		t.Error("D and E should not be neighbors (neither exists)")
	}

	// Test undirected graph
	g2 := New() // Undirected by default
	g2.AddEdgeByLabel("X", "Y", "bidirectional")

	if !g2.IsNeighbor("X", "Y") {
		t.Error("X and Y should be neighbors in undirected graph")
	}

	if !g2.IsNeighbor("Y", "X") {
		t.Error("Y and X should be neighbors in undirected graph")
	}

	// Test self-connection (if any)
	if g.IsNeighbor("A", "A") {
		t.Error("A should not be neighbor with itself")
	}
}
