package ssaapi

import (
	"strings"
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

const riskBatchSize = 500

// RiskDBSaver is a RiskUpdateHandler, just like a report. It owns its worker,
// not the scan runtime. Direct result saves use SaveRiskUpdates synchronously.
type RiskDBSaver struct {
	mu      sync.Mutex
	pending []schema.RiskUpdateItem
	wake    chan struct{}
	flushCh chan chan struct{}
	closeCh chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
	err     error
	closed  bool
}

func NewRiskDBSaver() *RiskDBSaver {
	s := &RiskDBSaver{
		wake: make(chan struct{}, 1), flushCh: make(chan chan struct{}),
		closeCh: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.worker()
	return s
}

var _ schema.RiskUpdateHandler = (*RiskDBSaver)(nil)

func (s *RiskDBSaver) ApplyRiskUpdate(item schema.RiskUpdateItem) error {
	if s == nil || item.Risk == nil {
		return nil
	}
	// The worker writes a copy so GORM cannot mutate the caller's object
	// while a reporter still reads it.
	row := *item.Risk
	row.CWE = append(schema.StringArray(nil), row.CWE...)
	item.Risk = &row
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return utils.Error("risk saver is closed")
	}
	s.pending = append(s.pending, item)
	if len(s.pending) >= riskBatchSize {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return nil
}

func (s *RiskDBSaver) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.closeCh:
			s.drain()
			return
		case done := <-s.flushCh:
			s.drain()
			close(done)
		case <-s.wake:
			s.drain()
		}
	}
}

func (s *RiskDBSaver) drain() {
	s.mu.Lock()
	batch := s.pending
	s.pending = nil
	s.mu.Unlock()
	// Fold the batch before it is committed. A finding covered before it is
	// written must not be inserted and deleted by the next batch.
	if err := SaveRiskUpdates(ssadb.GetDB(), batch...); err != nil {
		s.mu.Lock()
		s.err = utils.JoinErrors(s.err, err)
		s.mu.Unlock()
	}
}

func (s *RiskDBSaver) Flush() error {
	if s == nil {
		return nil
	}
	done := make(chan struct{})
	select {
	case s.flushCh <- done:
		<-done
	case <-s.closeCh:
		s.wg.Wait()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *RiskDBSaver) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.closeCh)
	})
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// SaveRisk inserts new findings and rewrites rows the caller already points
// at by id. A row with an id is an update; a row without one is a create.
func SaveRisk(db *gorm.DB, risks ...*schema.SSARisk) error {
	if len(risks) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	updates := make([]schema.RiskUpdateItem, 0, len(risks))
	for _, risk := range risks {
		if risk == nil {
			continue
		}
		updates = append(updates, schema.RiskUpdateItem{Risk: risk, OldID: risk.ID})
	}
	return saveRiskDecisions(db, updates)
}

// SaveRiskUpdates applies cover decisions. OldID names the row to rewrite;
// OldHash names the previous hash whose audit nodes must go. A decision that
// only exists to be replaced inside this batch never reaches the database.
func SaveRiskUpdates(db *gorm.DB, updates ...schema.RiskUpdateItem) error {
	if len(updates) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	return saveRiskDecisions(db, updates)
}

func saveRiskDecisions(db *gorm.DB, updates []schema.RiskUpdateItem) error {
	updates = coalesceRiskUpdates(updates)
	return db.Transaction(func(tx *gorm.DB) error {
		creates, rewrites, oldHashes := splitRiskUpdates(updates)
		if err := deleteAuditNodesByRiskHashes(tx, oldHashes); err != nil {
			return err
		}
		// A known id is rewritten in place. Everything else is inserted, or
		// overwrites the row that already has this hash.
		if err := tx.UpdateInBatches(rewrites, riskBatchSize, "id").Error; err != nil {
			return err
		}
		if err := tx.UpdateInBatches(creates, riskBatchSize, "hash").Error; err != nil {
			return err
		}
		return deleteRisksByHashes(tx, oldHashes)
	})
}

// splitRiskUpdates reads each decision as the caller stated it. OldID 0 is a
// create; any other OldID rewrites that row in place. The previous hash of a
// rewrite is deleted from the audit graph.
func splitRiskUpdates(updates []schema.RiskUpdateItem) (creates, rewrites []*schema.SSARisk, oldHashes []string) {
	creates = make([]*schema.SSARisk, 0, len(updates))
	rewrites = make([]*schema.SSARisk, 0)
	oldHashes = make([]string, 0)
	for _, item := range updates {
		if item.Risk == nil {
			continue
		}
		prepareRiskRow(item.Risk)
		if item.OldHash != "" && item.OldHash != item.Risk.Hash {
			oldHashes = append(oldHashes, item.OldHash)
		}
		if item.OldID == 0 {
			item.Risk.ID = 0
			creates = append(creates, item.Risk)
			continue
		}
		item.Risk.ID = item.OldID
		rewrites = append(rewrites, item.Risk)
	}
	return creates, rewrites, uniqueStrings(oldHashes)
}

func prepareRiskRow(risk *schema.SSARisk) {
	risk.PrepareForSave()
	if risk.TitleVerbose == "" {
		risk.TitleVerbose = risk.Title
	}
	if risk.LatestDisposalStatus == "" {
		risk.LatestDisposalStatus = string(schema.SSARiskDisposalStatus_NotSet)
	}
}

// coalesceRiskUpdates folds a chain that one batch both writes and replaces.
// source -> struct -> ssa becomes the final ssa row, still aimed at the
// original stored id when the chain started from one.
func coalesceRiskUpdates(updates []schema.RiskUpdateItem) []schema.RiskUpdateItem {
	out := make([]schema.RiskUpdateItem, 0, len(updates))
	byHash := make(map[string]int)
	for _, item := range updates {
		if item.Risk == nil {
			continue
		}
		item.Risk.PrepareForSave()
		i, found := byHash[item.OldHash]
		if !found {
			i, found = byHash[item.Risk.Hash]
		}
		if found {
			old := out[i]
			delete(byHash, old.Risk.Hash)
			item.OldID, item.OldHash = old.OldID, old.OldHash
			out[i] = item
			byHash[item.Risk.Hash] = i
		} else {
			byHash[item.Risk.Hash] = len(out)
			out = append(out, item)
		}
	}
	return out
}

// deleteRisksByHashes removes findings a later decision replaced. An id
// rewrite has already changed that row's hash, so this does not touch it.
func deleteRisksByHashes(db *gorm.DB, hashes []string) error {
	hashes = uniqueStrings(hashes)
	const chunk = 500
	for start := 0; start < len(hashes); start += chunk {
		end := start + chunk
		if end > len(hashes) {
			end = len(hashes)
		}
		// Unscoped: a soft-deleted hash still occupies the unique index.
		if err := db.Unscoped().Where("hash IN (?)", hashes[start:end]).Delete(&schema.SSARisk{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteAuditNodesByRiskHashes(db *gorm.DB, hashes []string) error {
	hashes = uniqueStrings(hashes)
	if len(hashes) == 0 {
		return nil
	}
	const chunk = 500
	for start := 0; start < len(hashes); start += chunk {
		end := start + chunk
		if end > len(hashes) {
			end = len(hashes)
		}
		var nodeIDs []string
		if err := db.Model(&ssadb.AuditNode{}).Where("risk_hash IN (?)", hashes[start:end]).
			Pluck("node_id", &nodeIDs).Error; err != nil {
			return err
		}
		for i := 0; i < len(nodeIDs); i += chunk {
			j := i + chunk
			if j > len(nodeIDs) {
				j = len(nodeIDs)
			}
			ids := nodeIDs[i:j]
			// Edges refer to node ids; AuditEdge has no risk_hash column.
			if err := db.Where("from_node IN (?) OR to_node IN (?)", ids, ids).
				Delete(&ssadb.AuditEdge{}).Error; err != nil {
				return err
			}
		}
		if err := db.Where("risk_hash IN (?)", hashes[start:end]).Delete(&ssadb.AuditNode{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
