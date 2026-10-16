package vectorstore

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type graphPersister struct {
	db             *gorm.DB
	uuid           string
	collectionName string
	syncBelow      int
	interval       time.Duration

	mu            sync.Mutex
	needsSnapshot bool
	timer         *time.Timer
	nextSeq       uint64
}

func newGraphPersister(db *gorm.DB, uuid, name string, config *CollectionConfig) (*graphPersister, error) {
	persister := &graphPersister{
		db:             db,
		uuid:           uuid,
		collectionName: name,
		syncBelow:      config.GraphPersistSyncBelow,
		interval:       config.GraphPersistInterval,
	}
	if persister.interval <= 0 {
		persister.interval = time.Second
	}
	if err := persister.loadNextSeq(); err != nil {
		return nil, err
	}
	return persister, nil
}

func (p *graphPersister) afterMutation(wrapper *GraphWrapper[string]) {
	if p.syncBelow > 0 && wrapper.graph.Len() < p.syncBelow {
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
	if p.needsSnapshot && !wrapper.closed.Load() {
		p.armTimerLocked(wrapper)
	}
}

func (p *graphPersister) persist(wrapper *GraphWrapper[string], force bool) (err error) {
	p.mu.Lock()
	if !force && !p.needsSnapshot {
		p.stopTimerLocked()
		p.mu.Unlock()
		return nil
	}
	p.needsSnapshot = false
	through := p.nextSeq
	p.stopTimerLocked()
	p.mu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("compact hnsw graph snapshot: %v", recovered)
		}
		if err != nil {
			p.mu.Lock()
			p.needsSnapshot = true
			if !wrapper.closed.Load() {
				p.armTimerLocked(wrapper)
			}
			p.mu.Unlock()
		}
	}()
	return p.writeSnapshot(wrapper, through)
}

func (p *graphPersister) writeSnapshot(wrapper *GraphWrapper[string], throughSeq uint64) error {
	started := time.Now()
	// Encoding may read lazy vectors through p.db. Do it before reserving a
	// transaction connection, including when the pool contains only one.
	updates, err := graphDatabaseUpdates(wrapper)
	if err != nil {
		return err
	}
	err = utils.GormTransaction(p.db, func(tx *gorm.DB) error {
		if err := writeGraphDatabaseUpdates(tx, p.uuid, updates); err != nil {
			return err
		}
		if throughSeq == 0 {
			return nil
		}
		return clearGraphDeltas(tx, p.uuid, throughSeq)
	})
	if err != nil {
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
	p.timer = time.AfterFunc(p.interval, func() {
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
