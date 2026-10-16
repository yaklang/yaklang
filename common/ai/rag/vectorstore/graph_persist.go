package vectorstore

import (
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Below this size every mutation is folded into the full snapshot immediately.
// Larger graphs append an adjacency delta and compact the snapshot in the background.
var (
	graphPersistSyncBelow = 1024
	graphPersistInterval  = time.Second
)

type graphPersister struct {
	db             *gorm.DB
	uuid           string
	collectionName string

	mu            sync.Mutex
	lastFlush     time.Time
	needsSnapshot bool
	timer         *time.Timer
	nextSeq       uint64
}

func newGraphPersister(db *gorm.DB, uuid, name string) *graphPersister {
	persister := &graphPersister{
		db:             db,
		uuid:           uuid,
		collectionName: name,
		lastFlush:      time.Now(),
	}
	if err := ensureGraphDeltaTable(db); err != nil {
		log.Errorf("migrate hnsw graph delta table for %s: %v", name, err)
	} else {
		persister.loadNextSeq()
	}
	return persister
}

func (p *graphPersister) noteMutation(wrapper *GraphWrapper[string]) {
	changes := wrapper.graph.TakeAdjacencyChanges()
	if len(changes) == 0 {
		return
	}
	if err := p.insertDelta(p.db, changes); err != nil {
		wrapper.graph.RestoreAdjacencyChanges(changes)
		log.Errorf("persist hnsw graph delta for %s: %v", p.collectionName, err)
		return
	}
	if graphPersistSyncBelow > 0 && wrapper.graph.Len() < graphPersistSyncBelow {
		if err := p.persist(wrapper, true); err != nil {
			log.Errorf("compact small hnsw graph snapshot for %s: %v", p.collectionName, err)
		}
		return
	}
	p.scheduleSnapshot(wrapper)
}

func (p *graphPersister) scheduleSnapshot(wrapper *GraphWrapper[string]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armTimerLocked(wrapper)
}

func (p *graphPersister) persist(wrapper *GraphWrapper[string], force bool) error {
	p.mu.Lock()
	if !force && !p.needsSnapshot {
		p.stopTimerLocked()
		p.mu.Unlock()
		return nil
	}
	p.needsSnapshot = false
	p.lastFlush = time.Now()
	through := p.nextSeq
	p.stopTimerLocked()
	p.mu.Unlock()

	if err := p.writeSnapshot(wrapper, through); err != nil {
		p.mu.Lock()
		p.needsSnapshot = true
		p.mu.Unlock()
		return err
	}
	return nil
}

func (p *graphPersister) writeSnapshot(wrapper *GraphWrapper[string], throughSeq uint64) error {
	started := time.Now()
	err := updateDatabaseGraphInfoInLock(p.db, p.uuid, wrapper)
	if err != nil {
		return err
	}
	if err := clearGraphDeltas(p.db, p.uuid, throughSeq); err != nil {
		return err
	}
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		log.Warnf("hnsw graph snapshot compaction took %s for collection %s (%d nodes, through seq %d)", elapsed, p.collectionName, wrapper.graph.Len(), throughSeq)
	} else {
		log.Debugf("compacted hnsw graph snapshot collection=%s nodes=%d through_seq=%d duration=%s", p.collectionName, wrapper.graph.Len(), throughSeq, elapsed)
	}
	return nil
}

func (p *graphPersister) armTimerLocked(wrapper *GraphWrapper[string]) {
	if p.timer != nil {
		return
	}
	p.timer = time.AfterFunc(graphPersistInterval, func() {
		if err := wrapper.queuePersist(false); err != nil && !wrapper.closed.Load() {
			log.Errorf("delayed hnsw graph persist for %s: %v", p.collectionName, err)
		}
	})
}

func (p *graphPersister) stopTimerLocked() {
	if p.timer == nil {
		return
	}
	p.timer.Stop()
	p.timer = nil
}

// QueryCollectionForExport flushes a cached graph, then re-reads the row so
// the exported binary matches the in-memory HNSW graph.
func QueryCollectionForExport(db *gorm.DB, name string) (*schema.VectorStoreCollection, error) {
	collection, err := yakit.QueryRAGCollectionByName(db, name)
	if err != nil || collection == nil {
		return collection, err
	}
	if err := GraphWrapperManager.PersistIfCached(db, collection); err != nil {
		return nil, err
	}
	return yakit.QueryRAGCollectionByName(db, name)
}
