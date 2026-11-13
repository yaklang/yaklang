package ssadb

import (
	"context"
	"encoding/json"
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

// IrConstValue is one const text resolved from the pool. The write
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
// kind and the rendered text, and the unique (program_name, const_id) index is
// what makes the pool a deduplicating store.
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

// IrConstHash is the legacy text-only key of the ir_consts_v1 table. It is
// kept unchanged so databases written before the pool still resolve their
// const_hash column. Two different texts in one program hashing to the same
// value would merge two constants, so the caller must not rely on it as a
// collision-free identity: at 180k values the probability is about 9e-10 per
// program, below the rate of unrelated restore faults.
func IrConstHash(text string) int64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(text); i++ {
		h ^= uint64(text[i])
		h *= fnvPrime64
	}
	return int64(h)
}

// IrConstPoolID is the pool key of one constant. The kind is part of the key
// because the rendered text alone is ambiguous: nil, the string "nil", int 1
// and the string "1" all render the same way, and a text-only key would make
// one of them restore with another kind.
func IrConstPoolID(kind IrConstKind, text string) int64 {
	h := uint64(fnvOffset64)
	// kind 0 is valid (nil), so bias it to keep the prefix non-zero.
	h ^= uint64(kind) + 1
	h *= fnvPrime64
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
	if db == nil || db.Dialect() == nil {
		return
	}
	if !db.HasTable(TableIrConstPool) {
		// AutoMigrate registers IrConst, so the pool exists on every database
		// this build opens. A missing pool means the migration did not run or
		// failed, and skipping the indexes quietly would surface later as an
		// unrelated "no such table" on the first const write.
		log.Errorf("SSA const pool %q is missing after migrate: const lookups and deletes will fail", TableIrConstPool)
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
	id := IrConstPoolID(kind, text)
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

// flushIrConstPools writes the values a program's instructions interned. The
// codec interns at marshal time, under the row's final program name, so this
// hook mostly has to flush the pending rows of every program in the batch.
//
// A row assembled outside the codec (a caller building an IrCode directly)
// may have no const id but still carries const_value in extra_information.
// That raw value is the only safe source of the kind, so the fallback interns
// from it. A row without const_value is left unpooled instead of being guessed
// as a string: "nil" and "1" render the same for several kinds, and a wrong
// kind restores the wrong value.
//
// It runs before the IR insert: a row must never point at a pool id that the
// transaction has not written yet.
func flushIrConstPools(db *gorm.DB, items []*IrCode) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	for _, item := range items {
		if item == nil || item.ConstID != 0 || item.ProgramName == "" {
			continue
		}
		if item.Opcode != irCodeConstOpcode || item.ConstType != irCodeConstTypeNormal {
			continue
		}
		raw, ok := irCodeConstRawValue(item.ExtraInformation)
		if !ok {
			continue
		}
		kind, text, ok := irConstKindAndText(raw)
		if !ok {
			continue
		}
		item.ConstID = InternIrConst(item.ProgramName, kind, text)
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

// irCodeConstRawValue returns const_value from a row's extra_information.
// ok is false when the key is absent: a null value is a real constant, so the
// caller cannot treat a nil result as missing.
func irCodeConstRawValue(extra string) (any, bool) {
	if extra == "" {
		return nil, false
	}
	dec := json.NewDecoder(strings.NewReader(extra))
	dec.UseNumber()
	var params map[string]any
	if err := dec.Decode(&params); err != nil {
		return nil, false
	}
	raw, ok := params["const_value"]
	return raw, ok
}

// irConstKindAndText rebuilds the kind and rendered text of a raw const value.
// The rendering matches ssa.Const.String; this package cannot import ssa, so
// the few scalar forms are repeated here.
func irConstKindAndText(raw any) (IrConstKind, string, bool) {
	switch v := raw.(type) {
	case nil:
		return IrConstKindNil, "nil", true
	case bool:
		return IrConstKindBool, strconv.FormatBool(v), true
	case string:
		return IrConstKindString, v, true
	case json.Number:
		text := v.String()
		if strings.ContainsAny(text, ".eE") {
			return IrConstKindFloat, text, true
		}
		return IrConstKindInt64, text, true
	case float64:
		return IrConstKindFloat, strconv.FormatFloat(v, 'g', -1, 64), true
	case int:
		return IrConstKindInt64, strconv.Itoa(v), true
	case int64:
		return IrConstKindInt64, strconv.FormatInt(v, 10), true
	default:
		return 0, "", false
	}
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

// warmIrConstValues loads a program's short constants once, so a reader that
// then calls GetIrConstValue for each row answers from memory. Long values stay
// in the database: eagerly loading them would hold the whole const payload,
// 1.3 GB on a yaklang scan, for the entire run, and a long value is referenced
// by 0.48% of the rows, so a batch would barely dedupe anything.
func warmIrConstValues(db *gorm.DB, progName string, rows []*IrCode) {
	if db == nil || progName == "" || len(rows) == 0 {
		return
	}
	// A program without const rows (no const instructions, or one written by
	// an older build) must not trigger the pool preload: an unnecessary SELECT
	// on every IR batch is a real cost, and a test counts read queries.
	for _, ir := range rows {
		if ir != nil && ir.ConstID != 0 {
			getIrConstValueCache(progName).preloadShortConsts(db, progName)
			return
		}
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
