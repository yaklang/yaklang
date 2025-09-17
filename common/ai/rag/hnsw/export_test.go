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
