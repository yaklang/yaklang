package syntaxflow_scan

import (
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// riskBatchSize is how many risk decisions accumulate before the saver writes
// them in one batch. A scan produces findings rule by rule, so a batch turns
// hundreds of single-row inserts into a few statements.
const riskBatchSize = 500

// dbSaver is the scan consumer that owns the syntaxflow result rows, the audit
// graph and the ssa_risk rows. It is the only place on the scan path that
// writes to the database; the scanning code and the query engine do not.
//
// Risk decisions are serialized into a pending batch when they arrive and
// flushed with one batch insert (creates) or one transaction (rewrites) when
// the batch is full or the scan ends, so the hot path does not pay a database
// round trip per finding.
type dbSaver struct {
	kind   schema.SyntaxflowResultKind
	task   string
	noRisk bool

	mu      sync.Mutex
	pending []schema.RiskUpdateItem
	// wake asks the worker to write a full batch; flush carries a channel the
	// worker closes after it drained every pending decision.
	wake    chan struct{}
	flushCh chan chan struct{}
	closeCh chan struct{}
	wg      sync.WaitGroup
	err     error
}

func newDBSaver(kind schema.SyntaxflowResultKind, taskID string, noRisk bool) *dbSaver {
	s := &dbSaver{
		kind:    kind,
		task:    taskID,
		noRisk:  noRisk,
		wake:    make(chan struct{}, 1),
		flushCh: make(chan chan struct{}),
		closeCh: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.worker()
	return s
}

// ApplyResult writes one finished result. Persisting the result also persists
// the value graph of every alert; each risk submission inside that save runs
// through the scan runtime, so no risk row is created here.
func (s *dbSaver) ApplyResult(res *ssaapi.SyntaxFlowResult) error {
	if s == nil || res == nil {
		return nil
	}
	if _, err := res.Save(s.kind, s.task); err != nil {
		return err
	}
	return nil
}

// ApplyRiskUpdate creates or rewrites one risk row. A rewrite keeps the row id
// so disposal history stays attached to the finding.
func (s *dbSaver) ApplyRiskUpdate(item schema.RiskUpdateItem) error {
	if s == nil || item.Risk == nil || s.noRisk {
		return nil
	}
	item = s.resolveCoveredPending(item)
	s.mu.Lock()
	s.pending = append(s.pending, item)
	full := len(s.pending) >= riskBatchSize
	s.mu.Unlock()
	if full {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return nil
}

// resolveCoveredPending handles a cover whose previous row has not been
// written yet: the superseded decision is removed from the batch instead of
// reaching the database. A row that was already written is rewritten in place.
func (s *dbSaver) resolveCoveredPending(item schema.RiskUpdateItem) schema.RiskUpdateItem {
	if item.OldID != 0 || item.OldHash == "" {
		return item
	}
	s.mu.Lock()
	for i := range s.pending {
		if s.pending[i].Risk != nil && s.pending[i].Risk.Hash == item.OldHash {
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			s.mu.Unlock()
			// The superseded row never reached the database, so the new
			// finding is created instead of rewriting anything.
			return schema.RiskUpdateItem{Risk: item.Risk}
		}
	}
	s.mu.Unlock()
	// The previous row may already be committed; find its id so the cover
	// rewrites that row instead of inserting a second one.
	if db := ssadb.GetDB(); db != nil {
		if stored, err := yakit.GetSSARiskByHash(db, item.OldHash); err == nil && stored != nil {
			item.OldID = stored.ID
		}
	}
	return item
}

// worker writes risk batches in the background so the scanning goroutine never
// waits for a database round trip.
func (s *dbSaver) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.closeCh:
			s.drain(0)
			return
		case done := <-s.flushCh:
			s.drain(0)
			close(done)
		case <-s.wake:
			s.drain(riskBatchSize)
		}
	}
}

// drain writes pending decisions. A limit > 0 writes at most that many; 0
// writes everything.
func (s *dbSaver) drain(limit int) {
	for {
		batch := s.takePending(limit)
		if len(batch) == 0 {
			return
		}
		if err := s.write(batch); err != nil {
			s.mu.Lock()
			if s.err == nil {
				s.err = err
			}
			s.mu.Unlock()
			log.Errorf("write risk batch failed: %v", err)
		}
		if limit > 0 && len(batch) < limit {
			return
		}
	}
}

func (s *dbSaver) takePending(limit int) []schema.RiskUpdateItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	n := len(s.pending)
	if limit > 0 && n > limit {
		n = limit
	}
	batch := make([]schema.RiskUpdateItem, n)
	copy(batch, s.pending[:n])
	s.pending = append([]schema.RiskUpdateItem(nil), s.pending[n:]...)
	return batch
}

// Flush writes every pending decision and waits for the worker. A scan calls
// it when it ends; the worker also writes by itself once a batch is full.
func (s *dbSaver) Flush() error {
	if s == nil {
		return nil
	}
	done := make(chan struct{})
	select {
	case s.flushCh <- done:
	case <-s.closeCh:
		return nil
	}
	<-done
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	return err
}

// Close flushes the remaining decisions and stops the worker. It is safe to
// call more than once.
func (s *dbSaver) Close() error {
	if s == nil {
		return nil
	}
	select {
	case <-s.closeCh:
	default:
		close(s.closeCh)
	}
	s.wg.Wait()
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	return err
}

// write persists one batch: creates in one statement, rewrites in one
// transaction.
func (s *dbSaver) write(batch []schema.RiskUpdateItem) error {
	db := ssadb.GetDB()
	if db == nil {
		return nil
	}
	creates := make([]*schema.SSARisk, 0, len(batch))
	updates := make([]schema.RiskUpdateItem, 0, len(batch))
	for _, item := range batch {
		if item.Risk == nil {
			continue
		}
		if item.OldID == 0 {
			creates = append(creates, item.Risk)
		} else {
			updates = append(updates, item)
		}
	}
	if len(creates) > 0 {
		if err := yakit.CreateSSARisksInBatches(db, creates); err != nil {
			return err
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, item := range updates {
			if err := rewriteSSARiskRowWithDB(tx, item.OldID, item.Risk); err != nil {
				return err
			}
			// The finding moved to a later mode: its old audit graph described
			// a different result, so drop it instead of leaving a stale path.
			if item.OldHash != "" && item.OldHash != item.Risk.Hash {
				if err := deleteAuditNodesByRiskHashWithDB(tx, item.OldHash); err != nil {
					log.Warnf("drop covered audit graph %s failed: %v", item.OldHash, err)
				}
			}
		}
		return nil
	})
}

// rewriteSSARiskRow updates the covered row in place and keeps its id.
func rewriteSSARiskRow(id uint, risk *schema.SSARisk) error {
	return rewriteSSARiskRowWithDB(ssadb.GetDB(), id, risk)
}

func rewriteSSARiskRowWithDB(db *gorm.DB, id uint, risk *schema.SSARisk) error {
	if risk == nil || id == 0 {
		return nil
	}
	if db == nil {
		return nil
	}
	row := *risk
	row.ID = id
	row.PrepareForSave()
	err := db.Model(&schema.SSARisk{}).Where("id = ?", id).Updates(map[string]interface{}{
		"hash":              row.Hash,
		"title":             row.Title,
		"title_verbose":     row.TitleVerbose,
		"description":       row.Description,
		"solution":          row.Solution,
		"risk_type":         row.RiskType,
		"details":           row.Details,
		"severity":          row.Severity,
		"language":          row.Language,
		"cve":               row.CVE,
		"cwe":               row.CWE,
		"from_rule":         row.FromRule,
		"scan_mode":         row.ScanMode,
		"program_name":      row.ProgramName,
		"code_source_url":   row.CodeSourceUrl,
		"code_range":        row.CodeRange,
		"code_fragment":     row.CodeFragment,
		"function_name":     row.FunctionName,
		"line":              row.Line,
		"runtime_id":        row.RuntimeId,
		"result_id":         row.ResultID,
		"result_uuid":       row.ResultUUID,
		"variable":          row.Variable,
		"index":             row.Index,
		"risk_feature_hash": row.RiskFeatureHash,
		"ssa_project_id":    row.SSAProjectID,
	}).Error
	if err != nil {
		return err
	}
	risk.ID = row.ID
	risk.Hash = row.Hash
	return nil
}

// deleteAuditNodesByRiskHash removes the graph rows of a covered finding.
func deleteAuditNodesByRiskHash(hash string) error {
	return deleteAuditNodesByRiskHashWithDB(ssadb.GetDB(), hash)
}

func deleteAuditNodesByRiskHashWithDB(db *gorm.DB, hash string) error {
	if hash == "" {
		return nil
	}
	if db == nil {
		return nil
	}
	var resultIDs []uint
	if err := db.Model(&ssadb.AuditNode{}).Where("risk_hash = ?", hash).
		Pluck("result_id", &resultIDs).Error; err != nil {
		return err
	}
	if len(resultIDs) == 0 {
		return nil
	}
	if err := db.Where("risk_hash = ?", hash).Delete(&ssadb.AuditNode{}).Error; err != nil {
		return err
	}
	return db.Where("risk_hash = ?", hash).Delete(&ssadb.AuditEdge{}).Error
}
