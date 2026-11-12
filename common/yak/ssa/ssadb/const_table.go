package ssadb

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/gorm"
)

// Const storage.
//
// A const instruction used to carry its rendered text in two places: the
// ir_codes row and a const table keyed by code_id. Both copies were the same
// text, and on a yaklang scan that is 2.8 GB for 1.77M instructions over only
// 180k distinct values.
//
// Now ir_codes.const_id points at one row per distinct value per program. The
// pool row holds the text once; ir_codes keeps only the 64-bit id. Restore
// fills the text back from the pool in batches, so no per-instruction lookup
// is issued.
//
// The legacy table (ir_consts_v1) is still read: a database written before the
// pool has no const_id and keeps its text in extra_information.const_value.
const (
	irConstLookupIndex        = "idx_ir_consts_lookup"
	irConstProgramCodeIndex   = "ux_ir_consts_program_code"
	irConstPoolProgramIDIndex = "ux_ir_const_pool_program_const"
	irConstPoolSizeIndex      = "idx_ir_const_pool_program_size"
	irCodeConstIDIndex        = "idx_ir_codes_program_const"
	irConstBatchChunk         = 500
)

// IrConstKind is the Go type of the constant's raw value. The pool keeps the
// rendered text only, and "1" is ambiguous: it can be int64(1) or "1".
type IrConstKind int64

const (
	IrConstKindNil IrConstKind = iota
	IrConstKindBool
	IrConstKindInt64
	IrConstKindFloat
	IrConstKindString
)

// IrConstValue is what a reader fills on an IrCode from the pool. The write
// path never sets it: a writer sets ConstID and the pool row instead.
type IrConstValue struct {
	Kind IrConstKind
	Text string
}

// Raw rebuilds the raw value a Const is created from.
func (v *IrConstValue) Raw() any {
	if v == nil {
		return nil
	}
	switch v.Kind {
	case IrConstKindBool:
		return v.Text == "true"
	case IrConstKindInt64:
		i, err := strconv.ParseInt(v.Text, 10, 64)
		if err != nil {
			return v.Text
		}
		return i
	case IrConstKindFloat:
		f, err := strconv.ParseFloat(v.Text, 64)
		if err != nil {
			return v.Text
		}
		return f
	case IrConstKindNil:
		return nil
	default:
		return v.Text
	}
}

// IrConst is one program's constant value. ConstID is a 64-bit hash of the
// rendered text, and the unique (program_name, const_id) index is what makes
// the pool a deduplicating store.
type IrConst struct {
	ID          int64  `gorm:"primary_key" json:"id"`
	ProgramName string `json:"program_name" gorm:"not null"`
	ConstID     int64  `json:"const_id" gorm:"not null"`
	Kind        int64  `json:"kind" gorm:"not null"`
	// TextSize lets a reader load the short values without touching the page
	// chain that holds the multi-kilobyte constants.
	TextSize int64  `json:"text_size" gorm:"not null"`
	String   string `json:"string" gorm:"type:text"`
}

func (*IrConst) TableName() string {
	return TableIrConstPool
}

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// IrConstHash is the pool key. Two different texts in one program hashing to
// the same value would merge two constants, so the caller must not rely on it
// as a collision-free identity: at 180k values the probability is about
// 9e-10 per program, below the rate of unrelated restore faults.
func IrConstHash(text string) int64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(text); i++ {
		h ^= uint64(text[i])
		h *= fnvPrime64
	}
	return int64(h)
}

// ensureIrConstIndexes creates the pool indexes and the ir_codes lookup the
// ConstType search joins on. The lookup is partial: only const rows carry a
// const_id, so indexing the other 8M rows would cost more than it saves.
func ensureIrConstIndexes(db *gorm.DB) {
	if db == nil || db.Dialect() == nil || !db.HasTable(TableIrConstPool) {
		return
	}
	queries := []struct {
		name  string
		query string
	}{
		{
			irConstPoolProgramIDIndex,
			`CREATE UNIQUE INDEX IF NOT EXISTS "` + irConstPoolProgramIDIndex + `" ON "` + TableIrConstPool + `" ("program_name", "const_id");`,
		},
		{
			irConstPoolSizeIndex,
			`CREATE INDEX IF NOT EXISTS "` + irConstPoolSizeIndex + `" ON "` + TableIrConstPool + `" ("program_name", "text_size");`,
		},
		{
			irCodeConstIDIndex,
			`CREATE INDEX IF NOT EXISTS "` + irCodeConstIDIndex + `" ON "` + TableIrCodes + `" ("program_name", "const_id") WHERE const_id <> 0;`,
		},
	}
	for _, idx := range queries {
		table := TableIrConstPool
		if idx.name == irCodeConstIDIndex {
			table = TableIrCodes
		}
		if hasNamedIndex(db, table, idx.name) {
			continue
		}
		if err := db.Exec(idx.query).Error; err != nil {
			log.Warnf("failed to add index %s: %v", idx.name, err)
		}
	}
}

// IrConstPool interns one program's constants before they are flushed with the
// IR batch that references them. A compile emits the same literal many times;
// the map keeps the hash work at one lookup per instruction and one row per
// distinct value.
type IrConstPool struct {
	mu      sync.Mutex
	program string
	seen    map[int64]struct{}
	pending []*IrConst
}

var irConstPools sync.Map

// GetIrConstPool returns the process-wide pool of one program.
func GetIrConstPool(program string) *IrConstPool {
	if v, ok := irConstPools.Load(program); ok {
		return v.(*IrConstPool)
	}
	pool := &IrConstPool{program: program, seen: make(map[int64]struct{}, 64)}
	actual, _ := irConstPools.LoadOrStore(program, pool)
	return actual.(*IrConstPool)
}

// DropIrConstPool forgets a program's interned set. The caller drops it when
// the program is deleted, so a later compile does not skip rows that the
// delete already removed.
func DropIrConstPool(program string) {
	irConstPools.Delete(program)
}

// InternIrConst records a constant value for a program and returns its pool
// id. The id is zero when the program name is unknown, which keeps a
// caller-built IrCode without a program from writing a stray row.
func InternIrConst(program string, kind IrConstKind, text string) int64 {
	if program == "" {
		return 0
	}
	return GetIrConstPool(program).Intern(kind, text)
}

// Intern records one constant value and returns its pool id.
func (p *IrConstPool) Intern(kind IrConstKind, text string) int64 {
	if p == nil {
		return 0
	}
	id := IrConstHash(text)
	p.mu.Lock()
	if _, ok := p.seen[id]; !ok {
		p.pending = append(p.pending, &IrConst{
			ProgramName: p.program,
			ConstID:     id,
			Kind:        int64(kind),
			TextSize:    int64(len(text)),
			String:      text,
		})
	}
	p.mu.Unlock()
	return id
}

// Flush writes the values interned since the previous flush inside the
// caller's transaction, so a pool row exists before any IR row can reference
// it.
//
// A value joins the interned set only after its row is written. Marking it at
// Intern time would lose the row when the surrounding transaction rolls back:
// the batch is retried, intern sees the value as already known, and the IR row
// ends up pointing at a pool row that was never committed.
func (p *IrConstPool) Flush(db *gorm.DB) error {
	if p == nil || db == nil {
		return nil
	}
	p.mu.Lock()
	rows := p.pending
	p.pending = nil
	p.mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	if err := db.New().SaveInBatches(rows, irConstBatchChunk, "program_name", "const_id").Error; err != nil {
		// Put the rows back: the caller's transaction may be retried, and a
		// silent drop would leave ir_codes rows pointing at nothing.
		p.mu.Lock()
		p.pending = append(rows, p.pending...)
		p.mu.Unlock()
		return err
	}
	p.mu.Lock()
	for _, row := range rows {
		p.seen[row.ConstID] = struct{}{}
	}
	p.mu.Unlock()
	return nil
}

// flushIrConstPools is the write-side hook: every program in the batch flushes
// the values its instructions interned. A row built without the compile-time
// interning path (tests, an in-tree caller assembling IrCode directly) still
// carries its short text in String, so the pool is filled from it instead of
// leaving ir_codes.const_id zero.
//
// It runs before the IR insert: a row must never point at a pool id that the
// transaction has not written yet.
func flushIrConstPools(db *gorm.DB, items []*IrCode) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item == nil || item.ConstID != 0 {
			continue
		}
		if item.Opcode != irCodeConstOpcode || item.ConstType != irCodeConstTypeNormal {
			continue
		}
		text := item.ConstText
		if text == "" {
			text = item.String
		}
		if text == "" {
			continue
		}
		item.ConstID = InternIrConst(item.ProgramName, IrConstKind(item.ConstKind), text)
	}
	programs := make(map[string]struct{}, 1)
	for _, item := range items {
		if item == nil || item.ProgramName == "" {
			continue
		}
		programs[item.ProgramName] = struct{}{}
	}
	var errs []error
	for program := range programs {
		if err := GetIrConstPool(program).Flush(db); err != nil {
			errs = append(errs, err)
		}
	}
	return errorsJoin(errs)
}

func errorsJoin(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	}
	var b strings.Builder
	for i, err := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(err.Error())
	}
	return &constFlushError{msg: b.String()}
}

type constFlushError struct{ msg string }

func (e *constFlushError) Error() string { return e.msg }

// irConstValueCache is one program's short constants, loaded once. Long values
// (a generated protobuf constant can be hundreds of kilobytes) stay in the
// database: eagerly loading them would hold the whole const payload, 1.3 GB on
// a yaklang scan, for the entire run.
type irConstValueCache struct {
	mu     sync.RWMutex
	loaded bool
	values map[int64]*IrConstValue
}

var irConstValueCaches sync.Map

func getIrConstValueCache(program string) *irConstValueCache {
	if v, ok := irConstValueCaches.Load(program); ok {
		return v.(*irConstValueCache)
	}
	cache := &irConstValueCache{values: make(map[int64]*IrConstValue, 1024)}
	actual, _ := irConstValueCaches.LoadOrStore(program, cache)
	return actual.(*irConstValueCache)
}

// DropIrConstValueCache forgets the loaded short values of one program.
func DropIrConstValueCache(program string) {
	irConstValueCaches.Delete(program)
}

// preloadShortConsts reads every short const of one program in a single
// statement. Const decoding is no longer a database call per instruction: the
// cache answers from a map, and only the rare long value reads a row.
func (c *irConstValueCache) preloadShortConsts(db *gorm.DB, program string) {
	if c == nil || db == nil || program == "" {
		return
	}
	c.mu.RLock()
	loaded := c.loaded
	c.mu.RUnlock()
	if loaded {
		return
	}
	values := make(map[int64]*IrConstValue, 1024)
	query := bindSQLPlaceholdersDB(db, `SELECT const_id, kind, "string" FROM "`+TableIrConstPool+
		`" WHERE program_name = ? AND text_size <= ?`)
	rows, err := db.Raw(query, program, IrCodeInlineConstStringLimit).Rows()
	if err != nil {
		// A database written before the pool has no such table; the decode path
		// keeps the extra_information text in that case.
		log.Debugf("preload const pool: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind int64
		var text string
		if err := rows.Scan(&id, &kind, &text); err != nil {
			log.Debugf("scan const pool: %v", err)
			return
		}
		values[id] = &IrConstValue{Kind: IrConstKind(kind), Text: text}
	}
	if rows.Err() != nil {
		return
	}
	c.mu.Lock()
	c.values = values
	c.loaded = true
	c.mu.Unlock()
}

func (c *irConstValueCache) get(id int64) (*IrConstValue, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	value, ok := c.values[id]
	c.mu.RUnlock()
	return value, ok
}

// GetIrConstValue resolves one const id for a program. The instruction decode
// path calls it whenever a row came from the IR cache instead of a reader that
// already filled the value, so it must answer without a query in the common
// case: the program's short constants are loaded into memory once, and only a
// long value reads its own row.
func GetIrConstValue(db *gorm.DB, program string, constID int64) *IrConstValue {
	if constID == 0 {
		return nil
	}
	if db == nil {
		db = GetDB()
	}
	cache := getIrConstValueCache(program)
	cache.preloadShortConsts(db, program)
	if value, ok := cache.get(constID); ok {
		return value
	}
	if db == nil {
		return nil
	}
	query := bindSQLPlaceholdersDB(db, `SELECT kind, "string" FROM "`+TableIrConstPool+
		`" WHERE program_name = ? AND const_id = ? LIMIT 1`)
	rows, err := db.Raw(query, program, constID).Rows()
	if err != nil {
		log.Debugf("read const value: %v", err)
		return nil
	}
	defer rows.Close()
	if !rows.Next() {
		return nil
	}
	var kind int64
	var text string
	if err := rows.Scan(&kind, &text); err != nil {
		log.Debugf("scan const value: %v", err)
		return nil
	}
	return &IrConstValue{Kind: IrConstKind(kind), Text: text}
}

// fillIrConstValues attaches the pool text to rows a reader just loaded. Short
// values come from the per-program cache, so a restore reads each program's
// short consts once instead of once per instruction batch.
func fillIrConstValues(db *gorm.DB, progName string, rows []*IrCode) {
	if db == nil || progName == "" || len(rows) == 0 {
		return
	}
	// A program without const rows (no const instructions, or one written by
	// an older build) must not trigger the pool preload: an unnecessary SELECT
	// on every IR batch is a real cost, and a test counts read queries.
	hasConstID := false
	for _, ir := range rows {
		if ir != nil && ir.ConstID != 0 {
			hasConstID = true
			break
		}
	}
	if !hasConstID {
		return
	}
	cache := getIrConstValueCache(progName)
	cache.preloadShortConsts(db, progName)

	ids := make([]int64, 0, len(rows))
	seen := make(map[int64]struct{}, len(rows))
	for _, ir := range rows {
		if ir == nil || ir.ConstID == 0 {
			continue
		}
		if value, ok := cache.get(ir.ConstID); ok {
			ir.ConstValue = value
			continue
		}
		if _, ok := seen[ir.ConstID]; ok {
			continue
		}
		seen[ir.ConstID] = struct{}{}
		ids = append(ids, ir.ConstID)
	}
	if len(ids) == 0 {
		return
	}
	values := make(map[int64]*IrConstValue, len(ids))
	for start := 0; start < len(ids); start += irConstBatchChunk {
		end := start + irConstBatchChunk
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)+1)
		args = append(args, progName)
		for i, id := range chunk {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query := bindSQLPlaceholdersDB(db, `SELECT const_id, kind, "string" FROM "`+TableIrConstPool+
			`" WHERE program_name = ? AND const_id IN (`+strings.Join(placeholders, ",")+`)`)
		sqlRows, err := db.Raw(query, args...).Rows()
		if err != nil {
			// A database written before the pool has no such table; the rows
			// keep their extra_information text and the decode path uses it.
			log.Debugf("read const pool: %v", err)
			return
		}
		for sqlRows.Next() {
			var id, kind int64
			var text string
			if err := sqlRows.Scan(&id, &kind, &text); err != nil {
				log.Debugf("scan const pool: %v", err)
				sqlRows.Close()
				return
			}
			values[id] = &IrConstValue{Kind: IrConstKind(kind), Text: text}
		}
		sqlRows.Close()
	}
	for _, ir := range rows {
		if ir == nil || ir.ConstID == 0 {
			continue
		}
		ir.ConstValue = values[ir.ConstID]
	}
}

// irConstPoolHasProgram tells the ConstType search whether this program wrote
// the pool. Databases from before the pool only have ir_consts_v1, and a
// program compiled by an older build keeps its rows there.
//
// One ctx-bound statement answers it. A schema probe (HasTable) would take a
// connection out of the pool outside this context, and a caller that already
// holds the only connection (a rule deadline test, a single-conn pool) would
// deadlock instead of seeing its deadline.
func irConstPoolHasProgram(ctx context.Context, db *gorm.DB, progName string) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var one int
	row := nativeQueryRowContext(db, ctx,
		`SELECT 1 FROM "`+TableIrConstPool+`" WHERE program_name = ? LIMIT 1`, progName)
	if row == nil {
		return false
	}
	return row.Scan(&one) == nil
}

// irConstTableHasProgram reports whether an older build wrote this program's
// const rows to the legacy per-code table. A database created by this build
// has no such table, and the probe returns that error as "no program rows".
// It must stay a ctx-bound statement: HasTable takes a connection out of the
// pool outside the context and deadlocks a caller that holds the only one.
func irConstTableHasProgram(ctx context.Context, db *gorm.DB, progName string) bool {
	if db == nil || db.Dialect() == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var one int
	row := nativeQueryRowContext(db, ctx,
		`SELECT 1 FROM "`+TableIrConsts+`" WHERE program_name = ? LIMIT 1`, progName)
	if row == nil {
		return false
	}
	return row.Scan(&one) == nil
}
