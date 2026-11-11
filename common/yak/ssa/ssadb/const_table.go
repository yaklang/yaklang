package ssadb

import (
	"context"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

// Const lookups used to ride on a partial index over ir_codes
// (program_name, string, code_id). That index duplicated every const text:
// 96% of its 1.4 GB payload was long, unique text from generated protobuf
// constants, and identical strings appeared only once. Consts now live in this
// narrow table, so the text is stored once and equality lookups seek on a
// 64-bit hash instead of the text.
const (
	irConstLookupIndex      = "idx_ir_consts_lookup"
	irConstProgramCodeIndex = "ux_ir_consts_program_code"
	irConstBatchChunk       = 500
)

// IrConst is one normal ConstType cell. Placeholder consts are member-call
// keys, are never searchable, and stay in ir_codes only.
type IrConst struct {
	ID          int64  `gorm:"primary_key" json:"id"`
	ProgramName string `json:"program_name"`
	CodeID      int64  `json:"code_id"`
	ConstHash   int64  `json:"const_hash"`
	String      string `json:"string" gorm:"type:text"`
}

func (*IrConst) TableName() string {
	return TableIrConsts
}

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// IrConstHash is the exact-match key. A collision only adds one candidate row;
// the query still compares the text.
func IrConstHash(text string) int64 {
	h := uint64(fnvOffset64)
	for i := 0; i < len(text); i++ {
		h ^= uint64(text[i])
		h *= fnvPrime64
	}
	return int64(h)
}

// ensureIrConstIndexes creates the const table indexes. The lookup index is
// (program_name, const_hash) instead of the text: exact lookups seek it, a
// regexp scan reads the table in row order.
func ensureIrConstIndexes(db *gorm.DB) {
	if db == nil || db.Dialect() == nil || !db.HasTable(TableIrConsts) {
		return
	}
	queries := []struct {
		name  string
		query string
	}{
		{
			irConstProgramCodeIndex,
			`CREATE UNIQUE INDEX IF NOT EXISTS "` + irConstProgramCodeIndex + `" ON "` + TableIrConsts + `" ("program_name", "code_id");`,
		},
		{
			irConstLookupIndex,
			`CREATE INDEX IF NOT EXISTS "` + irConstLookupIndex + `" ON "` + TableIrConsts + `" ("program_name", "const_hash");`,
		},
	}
	for _, idx := range queries {
		if hasNamedIndex(db, TableIrConsts, idx.name) {
			continue
		}
		if err := db.Exec(idx.query).Error; err != nil {
			log.Warnf("failed to add index %s: %v", idx.name, err)
		}
	}
}

// irConstText reports the const text carried by an IR row. Callers pass rows
// already filtered to opcode=5 and ConstType=normal, so an empty result is a
// real empty-string constant, not a missing one: ConstText is the payload this
// flush carries, String is the legacy/short column a caller-built IrCode
// (tests, older callers) provides instead.
func irConstText(ir *IrCode) string {
	if ir.ConstText != "" {
		return ir.ConstText
	}
	return ir.String
}

func irConstRowsFromIrCodes(items []*IrCode) []*IrConst {
	rows := make([]*IrConst, 0, 16)
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.Opcode != irCodeConstOpcode || item.ConstType != irCodeConstTypeNormal {
			continue
		}
		text := irConstText(item)
		rows = append(rows, &IrConst{
			ProgramName: item.ProgramName,
			CodeID:      item.CodeID,
			ConstHash:   IrConstHash(text),
			String:      text,
		})
	}
	if len(rows) <= 1 {
		return rows
	}
	// PostgreSQL rejects a statement that names the same conflict key twice.
	// The key is the conflict key only: hashing a 366 KiB const text per row
	// would walk the whole const payload once per flush.
	type constKey struct {
		program string
		codeID  int64
	}
	seen := make(map[constKey]int, len(rows))
	deduped := rows[:0]
	for _, row := range rows {
		key := constKey{program: row.ProgramName, codeID: row.CodeID}
		if at, ok := seen[key]; ok {
			deduped[at] = row
			continue
		}
		seen[key] = len(deduped)
		deduped = append(deduped, row)
	}
	return deduped
}

// saveIrConsts mirrors the const payload of one ir_codes batch into the const
// table inside the caller's transaction.
func saveIrConsts(db *gorm.DB, items []*IrCode) error {
	if db == nil || len(items) == 0 {
		return nil
	}
	rows := irConstRowsFromIrCodes(items)
	if len(rows) == 0 {
		return nil
	}
	if handled, err := nativeUpsertIrConsts(db, rows); handled {
		return err
	}
	return db.New().SaveInBatches(rows, irConstBatchChunk, "program_name", "code_id").Error
}

// nativeUpsertIrConsts keeps the const table in the same multi-row INSERT shape
// as the IR rows it mirrors: gorm's batch path resolves every field by
// reflection for every row. It returns handled=false for dialects that keep the
// GORM path.
func nativeUpsertIrConsts(db *gorm.DB, rows []*IrConst) (bool, error) {
	name := ""
	if db != nil && db.Dialect() != nil {
		name = strings.ToLower(db.Dialect().GetName())
	}
	if name != "sqlite3" && name != "sqlite" && name != "postgres" && name != "postgresql" && name != "cloudsqlpostgres" {
		return false, nil
	}
	const columns = `("program_name", "code_id", "const_hash", "string")`
	rowPlaceholder := "(?,?,?,?)"
	conflict := ` ON CONFLICT ("program_name", "code_id") DO UPDATE SET "const_hash" = excluded."const_hash", "string" = excluded."string"`
	for start := 0; start < len(rows); start += irConstBatchChunk {
		end := start + irConstBatchChunk
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		args := make([]interface{}, 0, len(chunk)*4)
		for _, row := range chunk {
			sanitizeStructStringsForPG(row)
			args = append(args, row.ProgramName, row.CodeID, row.ConstHash, row.String)
		}
		values := strings.TrimSuffix(strings.Repeat(rowPlaceholder+",", len(chunk)), ",")
		query := `INSERT INTO "` + TableIrConsts + `" ` + columns + ` VALUES ` + values + conflict
		ctx, cancel := nativeQueryContext(context.Background())
		started := time.Now()
		_, err := nativeExecContextDB(db, ctx, bindSQLPlaceholdersDB(db, query), args...)
		cancel()
		RecordDBOp(DBOpCreate, time.Since(started), err != nil)
		if err != nil {
			return true, err
		}
	}
	return true, nil
}

// irConstTableHasProgram tells the const lookup whether this program writes
// ir_consts_v1. Databases from before the const table only have ir_codes.string,
// and the lookup must keep working for them.
//
// One ctx-bound statement answers it. A schema probe (HasTable) would take a
// connection out of the pool outside this context, and a caller that already
// holds the only connection (a rule deadline test, a single-conn pool) would
// deadlock instead of seeing its deadline.
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
