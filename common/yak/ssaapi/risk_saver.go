package ssaapi

import (
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

// SaveRiskUpdates is the shared persistence path for scans and direct
// SyntaxFlowResult.Save calls. It does not create a runtime or collect:
// decisions have already been made by the caller. Covers preserve the row id
// and disposal history; repeated saves of the same hash update its metadata.
func SaveRiskUpdates(db *gorm.DB, updates ...schema.RiskUpdateItem) error {
	if len(updates) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	updates = coalesceRiskUpdates(updates)
	return db.Transaction(func(tx *gorm.DB) error {
		hashes := make([]string, 0, len(updates)*2)
		for _, item := range updates {
			hashes = append(hashes, item.Risk.Hash)
			if item.OldHash != "" {
				hashes = append(hashes, item.OldHash)
			}
		}
		var stored []*schema.SSARisk
		if len(hashes) > 0 {
			if err := tx.Where("hash IN (?)", hashes).Find(&stored).Error; err != nil {
				return err
			}
		}
		byHash := make(map[string]uint, len(stored))
		for _, row := range stored {
			byHash[row.Hash] = row.ID
		}
		creates := make([]*schema.SSARisk, 0, len(updates))
		for _, item := range updates {
			id := item.OldID
			if id == 0 {
				id = byHash[item.OldHash]
			}
			if id == 0 {
				id = byHash[item.Risk.Hash]
			}
			if id == 0 {
				item.Risk.ID = 0
				creates = append(creates, item.Risk)
				continue
			}
			if err := rewriteSSARiskRow(tx, id, item.Risk); err != nil {
				return err
			}
			if item.OldHash != "" && item.OldHash != item.Risk.Hash {
				if err := deleteAuditNodesByRiskHash(tx, item.OldHash); err != nil {
					return err
				}
			}
		}
		return yakit.CreateSSARisksInBatches(tx, creates)
	})
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

func rewriteSSARiskRow(db *gorm.DB, id uint, risk *schema.SSARisk) error {
	row := *risk
	row.ID = id
	row.PrepareForSave()
	// Do not include disposal columns: covering a finding keeps its history.
	if err := db.Model(&row).Where("id = ?", id).Updates(map[string]interface{}{
		"hash": row.Hash, "title": row.Title, "title_verbose": row.TitleVerbose,
		"description": row.Description, "solution": row.Solution,
		"risk_type": row.RiskType, "details": row.Details, "severity": row.Severity,
		"language": row.Language, "cve": row.CVE, "cwe": row.CWE,
		"from_rule": row.FromRule, "scan_mode": row.ScanMode,
		"program_name": row.ProgramName, "code_source_url": row.CodeSourceUrl,
		"code_range": row.CodeRange, "code_fragment": row.CodeFragment,
		"function_name": row.FunctionName, "line": row.Line,
		"runtime_id": row.RuntimeId, "result_id": row.ResultID,
		"result_uuid": row.ResultUUID, "variable": row.Variable, "index": row.Index,
		"risk_feature_hash": row.RiskFeatureHash, "ssa_project_id": row.SSAProjectID,
	}).Error; err != nil {
		return err
	}
	risk.ID = id
	return nil
}

func deleteAuditNodesByRiskHash(db *gorm.DB, hash string) error {
	var nodeIDs []string
	if err := db.Model(&ssadb.AuditNode{}).Where("risk_hash = ?", hash).
		Pluck("node_id", &nodeIDs).Error; err != nil {
		return err
	}
	if len(nodeIDs) == 0 {
		return nil
	}
	// Edges refer to node ids; AuditEdge has no risk_hash column.
	if err := db.Where("from_node IN (?) OR to_node IN (?)", nodeIDs, nodeIDs).
		Delete(&ssadb.AuditEdge{}).Error; err != nil {
		return err
	}
	return db.Where("risk_hash = ?", hash).Delete(&ssadb.AuditNode{}).Error
}
