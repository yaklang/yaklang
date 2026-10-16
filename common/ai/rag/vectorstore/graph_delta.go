package vectorstore

import (
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"github.com/golang/groupcache/lru"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const graphDeltaMagic = "YAKDELTA"

var graphDeltaMigrationMu sync.Mutex

func ensureGraphDeltaTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("graph delta database is nil")
	}
	graphDeltaMigrationMu.Lock()
	defer graphDeltaMigrationMu.Unlock()
	if db.HasTable(&schema.VectorStoreGraphDelta{}) {
		return nil
	}
	return db.AutoMigrate(&schema.VectorStoreGraphDelta{}).Error
}

func encodeAdjacencyDelta(changes []hnsw.AdjacencyChange[string]) ([]byte, error) {
	var buf []byte
	buf = append(buf, graphDeltaMagic...)
	buf = binary.LittleEndian.AppendUint32(buf, 1)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(changes)))
	for _, change := range changes {
		if change.Layer < 0 || change.Layer >= 1<<16 {
			return nil, fmt.Errorf("invalid hnsw graph delta layer %d", change.Layer)
		}
		buf = binary.LittleEndian.AppendUint32(buf, uint32(change.Layer))
		if change.Deleted {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
		buf = appendCountedString(buf, change.Key)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(len(change.Neighbors)))
		for _, neighbor := range change.Neighbors {
			buf = appendCountedString(buf, neighbor)
		}
	}
	return buf, nil
}

func decodeAdjacencyDelta(payload []byte) ([]hnsw.AdjacencyChange[string], error) {
	if len(payload) < len(graphDeltaMagic)+8 {
		return nil, fmt.Errorf("hnsw graph delta is too short")
	}
	if string(payload[:len(graphDeltaMagic)]) != graphDeltaMagic {
		return nil, fmt.Errorf("hnsw graph delta has an unknown header")
	}
	offset := len(graphDeltaMagic)
	version := binary.LittleEndian.Uint32(payload[offset:])
	offset += 4
	if version != 1 {
		return nil, fmt.Errorf("hnsw graph delta version %d is unsupported", version)
	}
	count := binary.LittleEndian.Uint32(payload[offset:])
	offset += 4
	if uint64(count) > uint64(len(payload)-offset)/13 {
		return nil, fmt.Errorf("invalid hnsw graph delta change count %d", count)
	}
	changes := make([]hnsw.AdjacencyChange[string], 0, count)
	for i := uint32(0); i < count; i++ {
		if offset+5 > len(payload) {
			return nil, fmt.Errorf("hnsw graph delta truncated at change %d", i)
		}
		layer := int(binary.LittleEndian.Uint32(payload[offset:]))
		offset += 4
		if layer < 0 || layer >= 1<<16 || payload[offset] > 1 {
			return nil, fmt.Errorf("invalid hnsw graph delta layer or deletion flag")
		}
		deleted := payload[offset] == 1
		offset++
		key, next, err := readCountedString(payload, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		if offset+4 > len(payload) {
			return nil, fmt.Errorf("hnsw graph delta truncated neighbors")
		}
		neighborCount := binary.LittleEndian.Uint32(payload[offset:])
		offset += 4
		if uint64(neighborCount) > uint64(len(payload)-offset)/4 {
			return nil, fmt.Errorf("invalid hnsw graph delta neighbor count %d", neighborCount)
		}
		neighbors := make([]string, 0, neighborCount)
		for n := uint32(0); n < neighborCount; n++ {
			neighbor, next, err := readCountedString(payload, offset)
			if err != nil {
				return nil, err
			}
			offset = next
			neighbors = append(neighbors, neighbor)
		}
		changes = append(changes, hnsw.AdjacencyChange[string]{
			Layer: layer, Key: key, Neighbors: neighbors, Deleted: deleted,
		})
	}
	if offset != len(payload) {
		return nil, fmt.Errorf("trailing data in hnsw graph delta")
	}
	return changes, nil
}

func appendCountedString(buf []byte, value string) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(value)))
	return append(buf, value...)
}

func readCountedString(payload []byte, offset int) (string, int, error) {
	if offset < 0 || offset > len(payload)-4 {
		return "", 0, fmt.Errorf("hnsw graph delta truncated string length")
	}
	size := int(binary.LittleEndian.Uint32(payload[offset:]))
	offset += 4
	if size < 0 || size > len(payload)-offset {
		return "", 0, fmt.Errorf("hnsw graph delta truncated string")
	}
	return string(payload[offset : offset+size]), offset + size, nil
}

func (p *graphPersister) loadNextSeq() error {
	var maxSeq uint64
	row := p.db.Model(&schema.VectorStoreGraphDelta{}).
		Where("collection_uuid = ?", p.uuid).
		Select("COALESCE(MAX(seq), 0)").Row()
	if err := row.Scan(&maxSeq); err != nil {
		return utils.Wrap(err, "load hnsw graph delta sequence")
	}
	p.nextSeq = maxSeq
	p.needsSnapshot = maxSeq > 0
	return nil
}

func (p *graphPersister) insertDelta(tx *gorm.DB, changes []hnsw.AdjacencyChange[string]) error {
	if len(changes) == 0 {
		return nil
	}
	payload, err := encodeAdjacencyDelta(changes)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.nextSeq++
	seq := p.nextSeq
	p.needsSnapshot = true
	p.mu.Unlock()
	row := &schema.VectorStoreGraphDelta{
		CollectionUUID: p.uuid,
		Seq:            seq,
		Payload:        payload,
	}
	if err := tx.Create(row).Error; err != nil {
		return utils.Wrap(err, "insert hnsw graph delta")
	}
	log.Debugf("appended hnsw graph delta collection=%s seq=%d changes=%d bytes=%d", p.collectionName, seq, len(changes), len(payload))
	return nil
}

func loadGraphDeltas(db *gorm.DB, collectionUUID string) ([][]hnsw.AdjacencyChange[string], error) {
	if err := ensureGraphDeltaTable(db); err != nil {
		return nil, err
	}
	rows, err := db.Model(&schema.VectorStoreGraphDelta{}).Where("collection_uuid = ?", collectionUUID).
		Select("seq, payload").Order("seq ASC, id ASC").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type ref struct {
		layer int
		key   string
	}
	latest := make(map[ref]hnsw.AdjacencyChange[string])
	for rows.Next() {
		var seq uint64
		var payload []byte
		if err := rows.Scan(&seq, &payload); err != nil {
			return nil, err
		}
		changes, err := decodeAdjacencyDelta(payload)
		if err != nil {
			return nil, utils.Wrapf(err, "decode hnsw graph delta seq %d", seq)
		}
		for _, change := range changes {
			latest[ref{change.Layer, change.Key}] = change
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(latest) == 0 {
		return nil, nil
	}
	changes := make([]hnsw.AdjacencyChange[string], 0, len(latest))
	for _, change := range latest {
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Layer != changes[j].Layer {
			return changes[i].Layer < changes[j].Layer
		}
		return changes[i].Key < changes[j].Key
	})
	return [][]hnsw.AdjacencyChange[string]{changes}, nil
}
func clearGraphDeltas(tx *gorm.DB, collectionUUID string, throughSeq uint64) error {
	if collectionUUID == "" {
		return nil
	}
	query := tx.Where("collection_uuid = ?", collectionUUID)
	if throughSeq > 0 {
		query = query.Where("seq <= ?", throughSeq)
	}
	return query.Unscoped().Delete(&schema.VectorStoreGraphDelta{}).Error
}

func applyGraphDeltas(db *gorm.DB, collection *schema.VectorStoreCollection, graph *hnsw.Graph[string], deltas [][]hnsw.AdjacencyChange[string], cacheSize int) error {
	if len(deltas) == 0 {
		return nil
	}
	create, err := newDeltaNodeFactory(db, collection, cacheSize)
	if err != nil {
		return err
	}
	for _, changes := range deltas {
		refreshed := make(map[int]bool)
		// Full-vector legacy snapshots may contain the old embedding for a
		// replaced document. Rebind changed nodes to their current durable data.
		for _, change := range changes {
			if change.Deleted || change.Layer >= len(graph.Layers) || graph.Layers[change.Layer] == nil {
				continue
			}
			layer := graph.Layers[change.Layer]
			if layer.Nodes[change.Key] == nil {
				continue
			}
			node, err := create(change.Key)
			if err != nil {
				return err
			}
			layer.Nodes[change.Key] = node
			refreshed[change.Layer] = true
		}
		if err := graph.ApplyAdjacencyChanges(changes, create); err != nil {
			return utils.Wrap(err, "replay hnsw graph delta")
		}
		for level := range refreshed {
			if level >= len(graph.Layers) {
				continue
			}
			layer := graph.Layers[level]
			for _, node := range layer.Nodes {
				for key := range node.GetNeighbors() {
					if layer.Nodes[key] == nil {
						return fmt.Errorf("missing hnsw neighbor %s on layer %d", key, level)
					}
					node.GetNeighbors()[key] = layer.Nodes[key]
				}
			}
		}
	}
	return nil
}

func newDeltaNodeFactory(db *gorm.DB, collection *schema.VectorStoreCollection, cacheSizes ...int) (func(string) (hnswspec.LayerNode[string], error), error) {
	refs, err := loadGraphDocumentRefs(db, collection.ID)
	if err != nil {
		return nil, err
	}
	cacheSize := 10000
	if len(cacheSizes) > 0 && cacheSizes[0] > 0 {
		cacheSize = cacheSizes[0]
	}
	cache := &lru.Cache{MaxEntries: cacheSize}
	var cacheMu sync.Mutex
	load := func(id uint, column string) (any, error) {
		cacheMu.Lock()
		value, found := cache.Get(id)
		cacheMu.Unlock()
		if found {
			return value, nil
		}
		doc, err := getVectorDocumentByLazyNodeID(db.Where("collection_id = ?", collection.ID).Select(column), uint64(id))
		if err != nil {
			return nil, err
		}
		if column == "pq_code" {
			value = doc.PQCode
		} else {
			value = []float32(doc.Embedding)
		}
		cacheMu.Lock()
		cache.Add(id, value)
		cacheMu.Unlock()
		return value, nil
	}
	pqMode := collection.EnablePQMode
	return func(key string) (hnswspec.LayerNode[string], error) {
		stored, exists := refs[key]
		if !exists {
			return nil, utils.Wrapf(gorm.ErrRecordNotFound, "missing hnsw delta document %s", key)
		}
		var node hnswspec.LayerNode[string]
		if pqMode {
			node = hnswspec.NewLazyRawPQLayerNode(stored.documentID, func() ([]byte, error) {
				value, err := load(stored.id, "pq_code")
				if err != nil {
					return nil, err
				}
				return value.([]byte), nil
			})
		} else {
			node = hnswspec.NewStandardLayerNode(stored.documentID, func() []float32 {
				value, err := load(stored.id, "embedding")
				if err != nil {
					log.Errorf("load embedding for hnsw delta node %s: %v", key, err)
					return nil
				}
				return value.([]float32)
			})
		}
		return hnswspec.NewLazyLayerNode[string](key, func(hnswspec.LazyNodeID) (hnswspec.LayerNode[string], error) { return node, nil }), nil
	}, nil
}
