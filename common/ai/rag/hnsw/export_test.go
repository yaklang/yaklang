package hnsw

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/utils"
	"gotest.tools/v3/assert"
)

func TestExportWithUIDMode(t *testing.T) {
	keyToVector := map[string][]float32{
		"1": {1},
		"2": {2},
		"3": {3},
		"4": {4},
		"5": {5},
	}

	keyToID := map[string]string{}
	idToKey := map[hnswspec.LazyNodeID]string{}
	for key := range keyToVector {
		id := "col_" + key
		keyToID[key] = id
		idToKey[id] = key
	}

	graph := NewGraph(WithConvertToUIDFunc(func(node hnswspec.LayerNode[string]) (hnswspec.LazyNodeID, error) {
		return hnswspec.LazyNodeID(keyToID[node.GetKey()]), nil
	}), WithNodeType[string](InputNodeTypeLazy))

	keys := make([]string, 0, len(keyToVector))
	for key := range keyToVector {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		graph.Add(MakeInputNodeFromID(key, hnswspec.LazyNodeID(keyToID[key]), func(uid hnswspec.LazyNodeID) ([]float32, error) {
			return keyToVector[idToKey[uid]], nil
		}))
	}

	midIns, err := ExportHNSWGraph(graph)
	if err != nil {
		t.Fatal(err)
	}

	var totalGraphNodes = 0
	for _, layer := range graph.Layers {
		totalGraphNodes += len(layer.Nodes)
	}

	assertMidIns := func(midIns *Persistent[string]) {
		assert.Equal(t, midIns.Dims, uint32(1))
		assert.Equal(t, midIns.Total, uint32(totalGraphNodes))
		codes := lo.Map(midIns.OffsetToKey[1:], func(item *PersistentNode[string], _ int) string {
			return utils.InterfaceToString(item.Code)
		})
		slices.Sort(codes)
		assert.Equal(t, strings.Join(codes, ","), "col_1,col_2,col_3,col_4,col_5")

		keys := lo.Map(midIns.OffsetToKey[1:], func(item *PersistentNode[string], _ int) string {
			return item.Key
		})
		slices.Sort(keys)
		assert.Equal(t, strings.Join(keys, ","), "1,2,3,4,5")
	}

	assertMidIns(midIns)
	binary, err := midIns.ToBinary(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	midIns, err = LoadBinary[string](binary)
	if err != nil {
		t.Fatal(err)
	}
	assertMidIns(midIns)
}

func TestExportNeighborRelationships(t *testing.T) {
	// 创建一个简单的图，包含明确的邻居关系
	keyToVector := map[string][]float32{
		"node1": {1.0, 0.0},
		"node2": {0.0, 1.0},
		"node3": {1.0, 1.0},
		"node4": {0.5, 0.5},
	}

	// 创建图
	graph := NewGraph[string](WithDeterministicRng[string](0))

	// 添加节点
	keys := make([]string, 0, len(keyToVector))
	for key := range keyToVector {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		vec := keyToVector[key]
		node := MakeInputNode(key, vec)
		graph.Add(node)
	}

	// 验证每一层节点的邻居关系
	verifyNeighbors := func(graph *Graph[string], testName string) {
		t.Logf("Verifying neighbors for %s", testName)
		for layerIdx, layer := range graph.Layers {
			t.Logf("Layer %d has %d nodes", layerIdx, len(layer.Nodes))
			for nodeKey, node := range layer.Nodes {
				actualNeighbors := node.GetNeighbors()
				var actualNeighborKeys []string
				for neighborKey := range actualNeighbors {
					actualNeighborKeys = append(actualNeighborKeys, neighborKey)
				}
				slices.Sort(actualNeighborKeys)

				t.Logf("  Node %s in layer %d has neighbors: [%s]",
					nodeKey, layerIdx, strings.Join(actualNeighborKeys, ", "))

				// 验证每个邻居在同一层中确实存在
				for _, neighborKey := range actualNeighborKeys {
					if _, exists := layer.Nodes[neighborKey]; !exists {
						t.Errorf("%s: Layer %d - Node %s has neighbor %s that doesn't exist in the same layer",
							testName, layerIdx, nodeKey, neighborKey)
					}
				}

				// 验证邻居关系的双向性（如果A是B的邻居，B也应该是A的邻居）
				for _, neighborKey := range actualNeighborKeys {
					if neighborNode, exists := layer.Nodes[neighborKey]; exists {
						neighborNeighbors := neighborNode.GetNeighbors()
						if _, hasReverse := neighborNeighbors[nodeKey]; !hasReverse {
							t.Logf("Warning: %s: Layer %d - Neighbor relationship is not bidirectional between %s and %s",
								testName, layerIdx, nodeKey, neighborKey)
						}
					}
				}
			}
		}
	}

	// 记录原始图的结构
	t.Logf("Original graph has %d layers", len(graph.Layers))
	verifyNeighbors(graph, "Original Graph")

	// 导出图
	persistent, err := ExportHNSWGraph(graph)
	if err != nil {
		t.Fatalf("Failed to export graph: %v", err)
	}

	// 转换为二进制并重新加载
	binary, err := persistent.ToBinary(context.Background())
	if err != nil {
		t.Fatalf("Failed to convert to binary: %v", err)
	}

	reloadedPersistent, err := LoadBinary[string](binary)
	if err != nil {
		t.Fatalf("Failed to load from binary: %v", err)
	}

	// 重新构建图并验证邻居关系是否正确恢复
	rebuiltGraph, err := reloadedPersistent.BuildGraph()
	if err != nil {
		t.Fatalf("Failed to rebuild graph: %v", err)
	}

	t.Logf("Rebuilt graph has %d layers", len(rebuiltGraph.Layers))

	// 验证层数是否一致
	if len(graph.Layers) != len(rebuiltGraph.Layers) {
		t.Errorf("Layer count mismatch: original has %d layers, rebuilt has %d layers",
			len(graph.Layers), len(rebuiltGraph.Layers))
	}

	// 验证每一层的节点数量是否一致
	for layerIdx := 0; layerIdx < len(graph.Layers) && layerIdx < len(rebuiltGraph.Layers); layerIdx++ {
		originalCount := len(graph.Layers[layerIdx].Nodes)
		rebuiltCount := len(rebuiltGraph.Layers[layerIdx].Nodes)
		if originalCount != rebuiltCount {
			t.Errorf("Layer %d node count mismatch: original has %d nodes, rebuilt has %d nodes",
				layerIdx, originalCount, rebuiltCount)
		}
	}

	// 验证重建图的邻居关系
	verifyNeighbors(rebuiltGraph, "Rebuilt Graph")

	// 比较原始图和重建图的邻居关系一致性
	compareGraphNeighbors := func(original, rebuilt *Graph[string]) {
		for layerIdx := 0; layerIdx < len(original.Layers) && layerIdx < len(rebuilt.Layers); layerIdx++ {
			originalLayer := original.Layers[layerIdx]
			rebuiltLayer := rebuilt.Layers[layerIdx]

			for nodeKey := range originalLayer.Nodes {
				if rebuiltNode, exists := rebuiltLayer.Nodes[nodeKey]; exists {
					originalNode := originalLayer.Nodes[nodeKey]

					originalNeighbors := originalNode.GetNeighbors()
					rebuiltNeighbors := rebuiltNode.GetNeighbors()

					var originalKeys, rebuiltKeys []string
					for key := range originalNeighbors {
						originalKeys = append(originalKeys, key)
					}
					for key := range rebuiltNeighbors {
						rebuiltKeys = append(rebuiltKeys, key)
					}

					slices.Sort(originalKeys)
					slices.Sort(rebuiltKeys)

					if strings.Join(originalKeys, ",") != strings.Join(rebuiltKeys, ",") {
						t.Errorf("Layer %d - Node %s neighbor mismatch: original [%s], rebuilt [%s]",
							layerIdx, nodeKey,
							strings.Join(originalKeys, ", "),
							strings.Join(rebuiltKeys, ", "))
					}
				} else {
					t.Errorf("Layer %d - Node %s exists in original but not in rebuilt graph", layerIdx, nodeKey)
				}
			}
		}
	}

	compareGraphNeighbors(graph, rebuiltGraph)
	t.Logf("Successfully verified neighbor relationships through export/import cycle")
}
