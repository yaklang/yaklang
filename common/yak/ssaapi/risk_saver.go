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
	// The result and reporter may keep reading this object while the worker
	// writes. Never let GORM mutate the caller's risk in the background.
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
	// Resolve covers here, after the preceding batch was committed. Resolving
	// them on the producer races with a batch already taken by the worker.
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

// SaveRisk inserts risks and updates rows that already exist. A row matches
// by hash, or by ID when the caller already points at a stored row. Disposal
// status and created_at stay on that row.
func SaveRisk(db *gorm.DB, risks ...*schema.SSARisk) error {
	if len(risks) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return saveRiskRows(tx, risks)
	})
}

// SaveRiskUpdates applies cover decisions, then saves the resulting risks.
// A cover keeps the earlier row id; audit nodes of a replaced hash are removed
// in one batch.
func SaveRiskUpdates(db *gorm.DB, updates ...schema.RiskUpdateItem) error {
	if len(updates) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	updates = coalesceRiskUpdates(updates)
	return db.Transaction(func(tx *gorm.DB) error {
		risks, oldHashes, err := risksFromUpdates(tx, updates)
		if err != nil {
			return err
		}
		if err := deleteAuditNodesByRiskHashes(tx, oldHashes); err != nil {
			return err
		}
		return saveRiskRows(tx, risks)
	})
}

func risksFromUpdates(tx *gorm.DB, updates []schema.RiskUpdateItem) ([]*schema.SSARisk, []string, error) {
	oldHashes := make([]string, 0, len(updates))
	lookup := make([]string, 0, len(updates))
	risks := make([]*schema.SSARisk, 0, len(updates))
	for _, item := range updates {
		if item.Risk == nil {
			continue
		}
		if item.OldID != 0 {
			item.Risk.ID = item.OldID
		}
		if item.OldHash != "" && item.OldHash != item.Risk.Hash {
			oldHashes = append(oldHashes, item.OldHash)
			if item.Risk.ID == 0 {
				lookup = append(lookup, item.OldHash)
			}
		}
		risks = append(risks, item.Risk)
	}
	if len(lookup) == 0 {
		return risks, uniqueStrings(oldHashes), nil
	}
	var stored []*schema.SSARisk
	if err := tx.Select("id, hash").Where("hash IN (?)", uniqueStrings(lookup)).Find(&stored).Error; err != nil {
		return nil, nil, err
	}
	byHash := make(map[string]uint, len(stored))
	for _, row := range stored {
		byHash[row.Hash] = row.ID
	}
	for _, item := range updates {
		if item.Risk == nil || item.Risk.ID != 0 {
			continue
		}
		item.Risk.ID = byHash[item.OldHash]
	}
	return risks, uniqueStrings(oldHashes), nil
}

func saveRiskRows(tx *gorm.DB, risks []*schema.SSARisk) error {
	rows := make([]*schema.SSARisk, 0, len(risks))
	hashes := make([]string, 0, len(risks))
	ids := make([]uint, 0, len(risks))
	for _, risk := range risks {
		if risk == nil {
			continue
		}
		risk.PrepareForSave()
		if risk.TitleVerbose == "" {
			risk.TitleVerbose = risk.Title
		}
		rows = append(rows, risk)
		if risk.Hash != "" {
			hashes = append(hashes, risk.Hash)
		}
		if risk.ID != 0 {
			ids = append(ids, risk.ID)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	stored, err := loadStoredRisks(tx, ids, hashes)
	if err != nil {
		return err
	}
	byID := make(map[uint]*schema.SSARisk, len(stored))
	byHash := make(map[string]*schema.SSARisk, len(stored))
	for _, row := range stored {
		byID[row.ID] = row
		byHash[row.Hash] = row
	}
	for _, row := range rows {
		var prev *schema.SSARisk
		if row.ID != 0 {
			prev = byID[row.ID]
		}
		if prev == nil {
			prev = byHash[row.Hash]
		}
		if prev != nil {
			keepStoredRiskState(row, prev)
		}
		if row.LatestDisposalStatus == "" {
			row.LatestDisposalStatus = string(schema.SSARiskDisposalStatus_NotSet)
		}
	}
	return tx.UpdateInBatches(rows, riskBatchSize, "id").Error
}

func loadStoredRisks(tx *gorm.DB, ids []uint, hashes []string) ([]*schema.SSARisk, error) {
	if len(ids) == 0 && len(hashes) == 0 {
		return nil, nil
	}
	var stored []*schema.SSARisk
	query := tx
	switch {
	case len(ids) > 0 && len(hashes) > 0:
		query = query.Where("id IN (?) OR hash IN (?)", ids, uniqueStrings(hashes))
	default:
		if len(ids) > 0 {
			query = query.Where("id IN (?)", ids)
		} else {
			query = query.Where("hash IN (?)", uniqueStrings(hashes))
		}
	}
	if err := query.Find(&stored).Error; err != nil {
		return nil, err
	}
	return stored, nil
}

// keepStoredRiskState copies the columns a rescan must not reset: the row id,
// when it was first stored, and the user's disposal marks.
func keepStoredRiskState(row, stored *schema.SSARisk) {
	row.ID = stored.ID
	row.CreatedAt = stored.CreatedAt
	row.LatestDisposalStatus = stored.LatestDisposalStatus
	row.IsRead = stored.IsRead
	row.Ignore = stored.Ignore
	row.UploadOnline = stored.UploadOnline
	row.IsPotential = stored.IsPotential
	row.CveAccessVector = stored.CveAccessVector
	row.CveAccessComplexity = stored.CveAccessComplexity
	row.Tags = stored.Tags
}

// A pending source -> struct -> SSA chain must reach the database only once.
// If the chain started at a stored row, keep that original replacement target.
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
