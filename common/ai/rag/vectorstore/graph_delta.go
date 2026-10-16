package vectorstore

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw"
	"github.com/yaklang/yaklang/common/ai/rag/hnsw/hnswspec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const graphDeltaMagic = "YAKDELTA"

func ensureGraphDeltaTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("graph delta database is nil")
	}
	return db.AutoMigrate(&schema.VectorStoreGraphDelta{}).Error
}

func encodeAdjacencyDelta(changes []hnsw.AdjacencyChange[string]) ([]byte, error) {
	var buf []byte
	buf = append(buf, graphDeltaMagic...)
	buf = binary.LittleEndian.AppendUint32(buf, 1)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(changes)))
	for _, change := range changes {
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
	changes := make([]hnsw.AdjacencyChange[string], 0, count)
	for i := uint32(0); i < count; i++ {
		if offset+5 > len(payload) {
			return nil, fmt.Errorf("hnsw graph delta truncated at change %d", i)
		}
		layer := int(binary.LittleEndian.Uint32(payload[offset:]))
		offset += 4
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
	return changes, nil
}

func appendCountedString(buf []byte, value string) []byte {
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(value)))
	return append(buf, value...)
}

func readCountedString(payload []byte, offset int) (string, int, error) {
	if offset+4 > len(payload) {
		return "", 0, fmt.Errorf("hnsw graph delta truncated string length")
	}
	size := int(binary.LittleEndian.Uint32(payload[offset:]))
	offset += 4
	if size < 0 || offset+size > len(payload) {
		return "", 0, fmt.Errorf("hnsw graph delta truncated string")
	}
	return string(payload[offset : offset+size]), offset + size, nil
}

func (p *graphPersister) loadNextSeq() {
	var maxSeq uint64
	row := p.db.Model(&schema.VectorStoreGraphDelta{}).
		Where("collection_uuid = ?", p.uuid).
		Select("COALESCE(MAX(seq), 0)").Row()
	if row != nil {
		_ = row.Scan(&maxSeq)
	}
	p.nextSeq = maxSeq
	var pending int64
	_ = p.db.Model(&schema.VectorStoreGraphDelta{}).Where("collection_uuid = ?", p.uuid).Count(&pending).Error
	p.needsSnapshot = pending > 0
}

func (p *graphPersister) insertDelta(tx *gorm.DB, changes []hnsw.AdjacencyChange[string]) error {
	if len(changes) == 0 {
		return nil
	}
	if err := ensureGraphDeltaTable(tx); err != nil {
		return err
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
	var rows []schema.VectorStoreGraphDelta
	if err := db.Where("collection_uuid = ?", collectionUUID).Order("seq asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	deltas := make([][]hnsw.AdjacencyChange[string], 0, len(rows))
	for _, row := range rows {
		changes, err := decodeAdjacencyDelta(row.Payload)
		if err != nil {
			return nil, utils.Wrapf(err, "decode hnsw graph delta seq %d", row.Seq)
		}
		deltas = append(deltas, changes)
	}
	return deltas, nil
}

func clearGraphDeltas(tx *gorm.DB, collectionUUID string, throughSeq uint64) error {
	if collectionUUID == "" {
		return nil
	}
	if err := ensureGraphDeltaTable(tx); err != nil {
		return err
	}
	query := tx.Where("collection_uuid = ?", collectionUUID)
	if throughSeq > 0 {
		query = query.Where("seq <= ?", throughSeq)
	}
	return query.Unscoped().Delete(&schema.VectorStoreGraphDelta{}).Error
}

func applyStoredGraphDeltas(db *gorm.DB, collection *schema.VectorStoreCollection, graph *hnsw.Graph[string]) error {
	if collection == nil || graph == nil || collection.UUID == "" {
		return nil
	}
	deltas, err := loadGraphDeltas(db, collection.UUID)
	if err != nil || len(deltas) == 0 {
		return err
	}
	create, err := newDeltaNodeFactory(db, collection)
	if err != nil {
		return err
	}
	for _, changes := range deltas {
		if err := graph.ApplyAdjacencyChanges(changes, create); err != nil {
			return utils.Wrap(err, "replay hnsw graph delta")
		}
	}
	log.Infof("replayed %d hnsw graph deltas for collection %s (%d nodes)", len(deltas), collection.Name, graph.Len())
	return nil
}

func newDeltaNodeFactory(db *gorm.DB, collection *schema.VectorStoreCollection) (func(string) (hnswspec.LayerNode[string], error), error) {
	refs, err := loadGraphDocumentRefs(db, collection.ID)
	if err != nil {
		return nil, err
	}
	cache := &sync.Map{}
	pqMode := collection.EnablePQMode
	return func(key string) (hnswspec.LayerNode[string], error) {
		stored, exists := refs[key]
		if !exists {
			return hnswspec.NewStandardLayerNode(key, func() []float32 { return []float32{} }), nil
		}
		if pqMode {
			return hnswspec.NewLazyRawPQLayerNode(stored.documentID, func() ([]byte, error) {
				if value, loaded := cache.Load(stored.id); loaded {
					return value.([]byte), nil
				}
				doc, err := getVectorDocumentByLazyNodeID(db.Where("collection_id = ?", collection.ID).Select("pq_code"), uint64(stored.id))
				if err != nil {
					return nil, err
				}
				cache.Store(stored.id, doc.PQCode)
				return doc.PQCode, nil
			}), nil
		}
		return hnswspec.NewStandardLayerNode(stored.documentID, func() []float32 {
			if value, loaded := cache.Load(stored.id); loaded {
				return value.([]float32)
			}
			doc, err := getVectorDocumentByLazyNodeID(db.Where("collection_id = ?", collection.ID).Select("embedding"), uint64(stored.id))
			if err != nil {
				log.Errorf("load embedding for hnsw delta node %s: %v", key, err)
				return nil
			}
			vector := []float32(doc.Embedding)
			cache.Store(stored.id, vector)
			return vector
		}), nil
	}, nil
}
