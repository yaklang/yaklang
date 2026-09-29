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
	creates []*schema.SSARisk
	updates []schema.RiskUpdateItem
}

func newDBSaver(kind schema.SyntaxflowResultKind, taskID string, noRisk bool) *dbSaver {
	return &dbSaver{kind: kind, task: taskID, noRisk: noRisk}
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
	// The decision is already a plain row snapshot; the batch keeps the same
	// pointer because the write-back of the row id is what lets a later scan
	// mode rewrite this finding instead of inserting a second row.
	pending := schema.RiskUpdateItem{
		Risk:    item.Risk,
		OldID:   item.OldID,
		OldHash: item.OldHash,
	}
	s.mu.Lock()
	if item.OldID == 0 {
		s.creates = append(s.creates, pending.Risk)
	} else {
		s.updates = append(s.updates, pending)
	}
	full := len(s.creates)+len(s.updates) >= riskBatchSize
	s.mu.Unlock()
	if full {
		return s.Flush()
	}
	return nil
}

// Flush writes every pending decision. A scan calls it when it ends; the saver
// also calls it by itself once a batch is full.
func (s *dbSaver) Flush() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	creates := s.creates
	updates := s.updates
	s.creates = nil
	s.updates = nil
	s.mu.Unlock()

	if len(creates) == 0 && len(updates) == 0 {
		return nil
	}
	db := ssadb.GetDB()
	if db == nil {
		return nil
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
