package ssaapi

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"sync"
	"time"

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
	if s == nil {
		return nil
	}
	// The worker runs on its own goroutine, and GORM writes the assigned id
	// and created_at into the struct it is handed. Give it a copy: the risk
	// collect and the report keep reading the published one.
	if item.Risk != nil {
		row := *item.Risk
		row.CWE = append(schema.StringArray(nil), row.CWE...)
		item.Risk = &row
	}
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

// SaveRisk writes the rows it is given. It is not a cover decision: a risk
// with no id is inserted, and a risk that already has one is updated in place
// by its primary key. Callers that create, update, or delete by mode use
// SaveRiskUpdates.
func SaveRisk(db *gorm.DB, risks ...*schema.SSARisk) error {
	if len(risks) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	creates := make([]*schema.SSARisk, 0, len(risks))
	updates := make([]*schema.SSARisk, 0)
	for _, risk := range risks {
		if risk == nil {
			continue
		}
		prepareRiskRow(risk)
		if risk.ID == 0 {
			creates = append(creates, risk)
		} else {
			updates = append(updates, risk)
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.CreateInBatches(creates, riskBatchSize).Error; err != nil {
			return err
		}
		// One chunk must not repeat a conflict key. The id is that key here.
		return tx.SaveInBatches(dedupeRisksByID(updates), riskBatchSize, "id").Error
	})
}

// SaveRiskUpdates runs each decision the way its mode says. The scan runtime
// already prepared the risk and chose create, update, or delete. An update or
// a delete matches runtime id plus feature hash, so another task's row is
// left alone. Audit writers run in the same transaction as the risk rows.
func SaveRiskUpdates(db *gorm.DB, updates ...schema.RiskUpdateItem) error {
	if len(updates) == 0 {
		return nil
	}
	if db == nil {
		return utils.Error("save risks: database is nil")
	}
	batch, err := batchRiskDecisions(updates)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := deleteAuditByFeature(tx, batch.dropAudit); err != nil {
			return err
		}
		if err := deleteAuditByRiskHashes(tx, batch.hashAudit); err != nil {
			return err
		}
		if err := tx.CreateInBatches(batch.creates, riskBatchSize).Error; err != nil {
			return err
		}
		if err := updateRiskRows(tx, batch.rewrites); err != nil {
			return err
		}
		if err := deleteRisksByHashes(tx, batch.dropHashes); err != nil {
			return err
		}
		if err := deleteRisksByFeature(tx, batch.dropRisk); err != nil {
			return err
		}
		return runAuditSaves(tx, batch.audits)
	})
}

type featureKey struct {
	runtime string
	feature string
}

type riskBatch struct {
	creates    []*schema.SSARisk
	rewrites   []*schema.SSARisk
	dropAudit  []featureKey
	dropRisk   []featureKey
	dropHashes []string
	hashAudit  []string
	audits     [][]func(*gorm.DB) error
}

// batchRiskDecisions groups a batch by the mode on each item. It does not
// change the risks or invent a mode. The last audit writers for one feature
// are the ones that get saved.
func batchRiskDecisions(updates []schema.RiskUpdateItem) (riskBatch, error) {
	var batch riskBatch
	batch.creates = make([]*schema.SSARisk, 0, len(updates))
	jobs := make(map[string][]func(*gorm.DB) error)
	var order []string
	put := func(key string, fns []func(*gorm.DB) error) {
		if _, ok := jobs[key]; !ok {
			order = append(order, key)
		}
		jobs[key] = fns
	}
	for i, item := range updates {
		switch item.Mode {
		case schema.RiskUpdateDelete:
			key, err := featureOf(item.Risk)
			if err != nil {
				return riskBatch{}, err
			}
			batch.dropRisk = append(batch.dropRisk, key)
			batch.dropAudit = append(batch.dropAudit, key)
			put(key.runtime+"\x00"+key.feature, nil)
		case schema.RiskUpdateUpdate:
			key, err := featureOf(item.Risk)
			if err != nil {
				return riskBatch{}, err
			}
			batch.rewrites = append(batch.rewrites, item.Risk)
			batch.dropAudit = append(batch.dropAudit, key)
			if item.OldHash != "" && item.OldHash != item.Risk.Hash {
				batch.hashAudit = append(batch.hashAudit, item.OldHash)
			}
			put(key.runtime+"\x00"+key.feature, item.Audit)
		case schema.RiskUpdateCreate:
			if item.Risk == nil {
				continue
			}
			batch.creates = append(batch.creates, item.Risk)
			if item.OldHash != "" && item.OldHash != item.Risk.Hash {
				batch.dropHashes = append(batch.dropHashes, item.OldHash)
				batch.hashAudit = append(batch.hashAudit, item.OldHash)
			}
			runtime := strings.TrimSpace(item.Risk.RuntimeId)
			feature := strings.TrimSpace(item.Risk.RiskFeatureHash)
			if runtime == "" || feature == "" {
				// An empty feature hash never covers another row, so its graph is
				// not grouped with anyone else's.
				put(fmt.Sprintf("\x00%d", i), item.Audit)
				continue
			}
			put(runtime+"\x00"+feature, item.Audit)
		default:
			return riskBatch{}, utils.Errorf("risk update: unknown mode %q", item.Mode)
		}
	}
	for _, key := range order {
		if len(jobs[key]) > 0 {
			batch.audits = append(batch.audits, jobs[key])
		}
	}
	return batch, nil
}

func featureOf(risk *schema.SSARisk) (featureKey, error) {
	if risk == nil {
		return featureKey{}, utils.Error("risk update: missing risk")
	}
	key := featureKey{
		runtime: strings.TrimSpace(risk.RuntimeId),
		feature: strings.TrimSpace(risk.RiskFeatureHash),
	}
	if key.runtime == "" || key.feature == "" {
		return featureKey{}, utils.Error("risk update: runtime id and feature hash are required")
	}
	return key, nil
}

// riskUpdateSkip stays out of a cover rewrite. The pair that finds the row
// is not a unique key, so SaveInBatches cannot do this update. Disposal and
// the read/ignore marks belong to the existing row, not to the new finding.
var riskUpdateSkip = map[string]bool{
	"id": true, "created_at": true, "deleted_at": true,
	"runtime_id": true, "risk_feature_hash": true,
	"latest_disposal_status": true, "is_read": true, "ignore": true,
	"tags": true, "upload_online": true,
}

// updateRiskRows rewrites every row of the batch in one statement per chunk,
// matched by runtime id and feature hash. Zero values are written. The
// columns come from the model, not from a hand-kept list.
func updateRiskRows(db *gorm.DB, risks []*schema.SSARisk) error {
	rows := make([]*schema.SSARisk, 0, len(risks))
	for _, risk := range risks {
		if risk == nil {
			continue
		}
		runtime := strings.TrimSpace(risk.RuntimeId)
		feature := strings.TrimSpace(risk.RiskFeatureHash)
		if runtime == "" || feature == "" {
			return utils.Errorf("update risk %q: runtime id and feature hash are required", risk.Hash)
		}
		row := *risk
		row.CWE = append(schema.StringArray(nil), risk.CWE...)
		row.RuntimeId, row.RiskFeatureHash = runtime, feature
		row.UpdatedAt = time.Now()
		rows = append(rows, &row)
	}
	rows = dedupeRisksByFeature(rows)
	if len(rows) == 0 {
		return nil
	}
	columns, err := riskUpdateColumns(db)
	if err != nil {
		return err
	}
	// WHEN runtime=? AND feature=? THEN ? is three placeholders per
	// column per row, plus two more in the WHERE. The old divisor of two
	// built a statement past SQLite's variable limit once a drain was large.
	chunk := 30000 / (len(columns)*3 + 2)
	if chunk > riskBatchSize {
		chunk = riskBatchSize
	}
	if chunk < 1 {
		chunk = 1
	}
	for start := 0; start < len(rows); start += chunk {
		end := start + chunk
		if end > len(rows) {
			end = len(rows)
		}
		if err := updateRiskChunk(db, columns, rows[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func riskUpdateColumns(db *gorm.DB) ([]*gorm.Field, error) {
	scope := db.NewScope(&schema.SSARisk{})
	var columns []*gorm.Field
	for _, field := range scope.Fields() {
		if !field.IsNormal || field.IsIgnored || field.IsPrimaryKey || riskUpdateSkip[field.DBName] {
			continue
		}
		columns = append(columns, field)
	}
	if len(columns) == 0 {
		return nil, utils.Error("update risk: model has no columns to write")
	}
	return columns, nil
}

func updateRiskChunk(db *gorm.DB, columns []*gorm.Field, risks []*schema.SSARisk) error {
	scope := db.NewScope(&schema.SSARisk{})
	var b strings.Builder
	args := make([]interface{}, 0, len(columns)*len(risks)*2+len(risks)*2)
	b.WriteString("UPDATE ")
	b.WriteString(scope.QuotedTableName())
	b.WriteString(" SET ")
	for i, field := range columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(scope.Quote(field.DBName))
		b.WriteString(" = CASE")
		for _, risk := range risks {
			value, err := riskColumnValue(db, risk, field.DBName)
			if err != nil {
				return err
			}
			b.WriteString(" WHEN ")
			b.WriteString(scope.Quote("runtime_id"))
			b.WriteString(" = ? AND ")
			b.WriteString(scope.Quote("risk_feature_hash"))
			b.WriteString(" = ? THEN ?")
			args = append(args, risk.RuntimeId, risk.RiskFeatureHash, value)
		}
		b.WriteString(" ELSE ")
		b.WriteString(scope.Quote(field.DBName))
		b.WriteString(" END")
	}
	b.WriteString(" WHERE ")
	for i, risk := range risks {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("(")
		b.WriteString(scope.Quote("runtime_id"))
		b.WriteString(" = ? AND ")
		b.WriteString(scope.Quote("risk_feature_hash"))
		b.WriteString(" = ?)")
		args = append(args, risk.RuntimeId, risk.RiskFeatureHash)
	}
	return db.Exec(b.String(), args...).Error
}

func riskColumnValue(db *gorm.DB, risk *schema.SSARisk, column string) (interface{}, error) {
	field, ok := db.NewScope(risk).FieldByName(column)
	if !ok {
		return nil, utils.Errorf("update risk: column %s is not on the model", column)
	}
	value := field.Field.Interface()
	if valuer, ok := value.(driver.Valuer); ok {
		converted, err := valuer.Value()
		if err != nil {
			return nil, err
		}
		return converted, nil
	}
	return value, nil
}

func dedupeRisksByFeature(risks []*schema.SSARisk) []*schema.SSARisk {
	last := make(map[featureKey]int, len(risks))
	for i, risk := range risks {
		last[featureKey{risk.RuntimeId, risk.RiskFeatureHash}] = i
	}
	out := make([]*schema.SSARisk, 0, len(last))
	for i, risk := range risks {
		if last[featureKey{risk.RuntimeId, risk.RiskFeatureHash}] == i {
			out = append(out, risk)
		}
	}
	return out
}

func dedupeRisksByID(risks []*schema.SSARisk) []*schema.SSARisk {
	if len(risks) == 0 {
		return risks
	}
	last := make(map[uint]int, len(risks))
	for i, risk := range risks {
		if risk != nil {
			last[risk.ID] = i
		}
	}
	out := make([]*schema.SSARisk, 0, len(last))
	for i, risk := range risks {
		if risk != nil && last[risk.ID] == i {
			out = append(out, risk)
		}
	}
	return out
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

func deleteAuditByFeature(db *gorm.DB, keys []featureKey) error {
	for runtime, features := range groupFeatures(keys) {
		if err := forEachChunk(features, func(part []string) error {
			if err := db.Where("task_id = ? AND risk_feature_hash IN (?)", runtime, part).Delete(&ssadb.AuditEdge{}).Error; err != nil {
				return err
			}
			return db.Where("task_id = ? AND risk_feature_hash IN (?)", runtime, part).Delete(&ssadb.AuditNode{}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func deleteRisksByFeature(db *gorm.DB, keys []featureKey) error {
	for runtime, features := range groupFeatures(keys) {
		if err := forEachChunk(features, func(part []string) error {
			// Unscoped: a soft-deleted hash still occupies the unique index.
			return db.Unscoped().Where("runtime_id = ? AND risk_feature_hash IN (?)", runtime, part).Delete(&schema.SSARisk{}).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func groupFeatures(keys []featureKey) map[string][]string {
	seen := make(map[featureKey]struct{}, len(keys))
	out := make(map[string][]string)
	for _, key := range keys {
		if key.runtime == "" || key.feature == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out[key.runtime] = append(out[key.runtime], key.feature)
	}
	return out
}

func forEachChunk(values []string, fn func([]string) error) error {
	const chunk = 500
	for start := 0; start < len(values); start += chunk {
		end := start + chunk
		if end > len(values) {
			end = len(values)
		}
		if err := fn(values[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func runAuditSaves(db *gorm.DB, groups [][]func(*gorm.DB) error) error {
	for _, group := range groups {
		for _, audit := range group {
			if audit == nil {
				continue
			}
			if err := audit(db); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteRisksByHashes removes rows a create decision named by the previous hash.
func deleteRisksByHashes(db *gorm.DB, hashes []string) error {
	return forEachChunk(uniqueStrings(hashes), func(part []string) error {
		// Unscoped: a soft-deleted hash still occupies the unique index.
		return db.Unscoped().Where("hash IN (?)", part).Delete(&schema.SSARisk{}).Error
	})
}

// deleteAuditByRiskHashes removes the graph stored under a row hash. Edges of
// the same finding are removed by feature hash; nothing is loaded by node id.
func deleteAuditByRiskHashes(db *gorm.DB, hashes []string) error {
	return forEachChunk(uniqueStrings(hashes), func(part []string) error {
		return db.Where("risk_hash IN (?)", part).Delete(&ssadb.AuditNode{}).Error
	})
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
