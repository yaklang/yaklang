package ssadb

import (
	"database/sql"
	"sync/atomic"
	"time"

	"github.com/yaklang/gorm"
)

// nativeIrCodeBatchReads is a test-only counter incremented every time
// nativeGetIrCodesByIds performs a batch read. Tests use it to prove that
// yieldIrCodes actually routes cold-cache misses through the native-SQL path
// (A2) instead of the old GORM FastPagination path.
var nativeIrCodeBatchReads atomic.Int64

// NativeIrCodeBatchReads returns the test-only batch-read counter.
func NativeIrCodeBatchReads() int64 {
	return nativeIrCodeBatchReads.Load()
}

// nativeConstTypeIDQueries is a test-only counter incremented every time
// nativeGetIrCodeIDsByConstType performs a ConstType ID query. Tests use it to
// prove that SearchVariableWithFileFilter routes the ConstType hot path
// (3.59M calls on hadoop) through native SQL instead of GORM Pluck.
var nativeConstTypeIDQueries atomic.Int64

// NativeConstTypeIDQueries returns the test-only ConstType query counter.
func NativeConstTypeIDQueries() int64 {
	return nativeConstTypeIDQueries.Load()
}

// Native-SQL fast paths for the hot single-row reads (GetIrTypeItemById /
// GetIrCodeItemById). GORM's First() builds a heavy query chain per call
// (Scope.Fields / DB.clone / search.clone / buildScanPlan — ~45GB combined in
// the hadoop scan window) that the native path skips entirely. The column list
// below is the exact GORM AutoMigrate column order, and the row scan populates
// the same fields (including the custom Int64Slice/Int64Map/StringSlice
// scanners and the soft-delete DeletedAt), so results are identical to GORM.
//
// Soft-delete semantics are preserved: the query adds "deleted_at IS NULL"
// exactly like GORM's non-Unscoped First().
func nativeGetIrTypeItemById(db *gorm.DB, progName string, id int64) *IrType {
	if db == nil || id < 0 {
		return nil
	}
	const q = `SELECT id, created_at, updated_at, deleted_at, type_id, kind, program_name, "string", extra_information ` +
		`FROM ` + TableIrTypes + ` WHERE type_id = ? AND program_name = ? AND deleted_at IS NULL LIMIT 1`
	row := db.CommonDB().QueryRow(q, id, progName)
	ir := &IrType{}
	var (
		deletedAt sql.NullTime
		createdAt time.Time
		updatedAt time.Time
	)
	if err := row.Scan(&ir.ID, &createdAt, &updatedAt, &deletedAt,
		&ir.TypeId, &ir.Kind, &ir.ProgramName, &ir.String, &ir.ExtraInformation); err != nil {
		return nil
	}
	ir.CreatedAt = createdAt
	ir.UpdatedAt = updatedAt
	if deletedAt.Valid {
		t := deletedAt.Time
		ir.DeletedAt = &t
	}
	return ir
}

func nativeGetIrCodeItemById(db *gorm.DB, progName string, id int64) *IrCode {
	if db == nil || id < 0 {
		return nil
	}
	const q = `SELECT id, created_at, updated_at, deleted_at, code_id, program_name, version, ` +
		`source_code_start_offset, source_code_end_offset, source_code_hash, opcode, opcode_name, opcode_operator, ` +
		`name, verbose_name, short_verbose_name, "string", readable_name, readable_name_short, ` +
		`current_block, current_function, is_function, formal_args, free_values, member_call_args, ` +
		`side_effects, is_variadic, return_codes, is_external, code_blocks, enter_block, exit_block, defer_block, ` +
		`children_function, parent_function, is_block, pred_block, succ_block, phis, has_defs, users, occulatation, ` +
		`is_object, object_members, object_member_pairs, is_object_member, object_parent, object_key, object_owner_pairs, ` +
		`masked_codes, is_masked, variable, program_compile_hash, type_id, point, pointer, extra_information, const_type ` +
		`FROM ` + TableIrCodes + ` WHERE code_id = ? AND program_name = ? AND deleted_at IS NULL LIMIT 1`
	row := db.CommonDB().QueryRow(q, id, progName)
	ir := &IrCode{}
	var (
		deletedAt sql.NullTime
		createdAt time.Time
		updatedAt time.Time
	)
	err := row.Scan(
		&ir.ID, &createdAt, &updatedAt, &deletedAt,
		&ir.CodeID, &ir.ProgramName, &ir.Version,
		&ir.SourceCodeStartOffset, &ir.SourceCodeEndOffset, &ir.SourceCodeHash,
		&ir.Opcode, &ir.OpcodeName, &ir.OpcodeOperator,
		&ir.Name, &ir.VerboseName, &ir.ShortVerboseName, &ir.String, &ir.ReadableName, &ir.ReadableNameShort,
		&ir.CurrentBlock, &ir.CurrentFunction, &ir.IsFunction, &ir.FormalArgs, &ir.FreeValues, &ir.MemberCallArgs,
		&ir.SideEffects, &ir.IsVariadic, &ir.ReturnCodes, &ir.IsExternal, &ir.CodeBlocks,
		&ir.EnterBlock, &ir.ExitBlock, &ir.DeferBlock, &ir.ChildrenFunction, &ir.ParentFunction,
		&ir.IsBlock, &ir.PredBlock, &ir.SuccBlock, &ir.Phis, &ir.HasDefs, &ir.Users, &ir.Occulatation,
		&ir.IsObject, &ir.ObjectMembers, &ir.ObjectMemberPairs, &ir.IsObjectMember,
		&ir.ObjectParent, &ir.ObjectKey, &ir.ObjectOwnerPairs,
		&ir.MaskedCodes, &ir.IsMasked, &ir.Variable, &ir.ProgramCompileHash,
		&ir.TypeID, &ir.Point, &ir.Pointer, &ir.ExtraInformation, &ir.ConstType,
	)
	if err != nil {
		return nil
	}
	ir.CreatedAt = createdAt
	ir.UpdatedAt = updatedAt
	if deletedAt.Valid {
		t := deletedAt.Time
		ir.DeletedAt = &t
	}
	return ir
}

// nativeIrCodeColumns is the exact AutoMigrate column list for ir_codes, shared
// by the single-row and batch native reads so they stay in lockstep.
const nativeIrCodeColumns = `id, created_at, updated_at, deleted_at, code_id, program_name, version, ` +
	`source_code_start_offset, source_code_end_offset, source_code_hash, opcode, opcode_name, opcode_operator, ` +
	`name, verbose_name, short_verbose_name, "string", readable_name, readable_name_short, ` +
	`current_block, current_function, is_function, formal_args, free_values, member_call_args, ` +
	`side_effects, is_variadic, return_codes, is_external, code_blocks, enter_block, exit_block, defer_block, ` +
	`children_function, parent_function, is_block, pred_block, succ_block, phis, has_defs, users, occulatation, ` +
	`is_object, object_members, object_member_pairs, is_object_member, object_parent, object_key, object_owner_pairs, ` +
	`masked_codes, is_masked, variable, program_compile_hash, type_id, point, pointer, extra_information, const_type`

// nativeIrCodeBatchChunk bounds the number of IN-list placeholders per query to
// stay under SQLite's MAX_VARIABLE_NUMBER (250000). 1000 is far below that and
// keeps each statement small.
const nativeIrCodeBatchChunk = 1000

// nativeGetIrCodesByIds reads the IrCodes for the given ids via a parameterized
// native-SQL IN query, chunked to respect SQLite's variable limit. It returns
// rows in stable code_id order (matching GORM Find's PK-ordered result for the
// same IN predicate), with missing ids simply absent and duplicates collapsed.
// Soft-delete semantics match GORM's non-Unscoped Find (deleted_at IS NULL).
//
// It returns (rows, error): a non-nil error is returned on any query/scan
// failure so callers can fall back to GORM instead of mistaking a DB error for
// an empty result. nil error + nil rows means "no matching rows".
func nativeGetIrCodesByIds(db *gorm.DB, progName string, ids []int64) ([]*IrCode, error) {
	if db == nil || len(ids) == 0 {
		return nil, nil
	}
	// Dedup + drop non-positive ids, preserving first-seen order.
	seen := make(map[int64]struct{}, len(ids))
	clean := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return nil, nil
	}
	nativeIrCodeBatchReads.Add(1)

	common := db.CommonDB()
	var out []*IrCode
	for start := 0; start < len(clean); start += nativeIrCodeBatchChunk {
		end := start + nativeIrCodeBatchChunk
		if end > len(clean) {
			end = len(clean)
		}
		chunk := clean[start:end]
		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)+1)
		args = append(args, progName)
		for i, id := range chunk {
			placeholders[i] = "?"
			args = append(args, id)
		}
		q := `SELECT ` + nativeIrCodeColumns + ` FROM ` + TableIrCodes +
			` WHERE program_name = ? AND code_id IN (` + joinPlaceholders(placeholders) + `) AND deleted_at IS NULL ORDER BY code_id`
		rows, err := common.Query(q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			ir := &IrCode{}
			var (
				deletedAt sql.NullTime
				createdAt time.Time
				updatedAt time.Time
			)
			if err := rows.Scan(
				&ir.ID, &createdAt, &updatedAt, &deletedAt,
				&ir.CodeID, &ir.ProgramName, &ir.Version,
				&ir.SourceCodeStartOffset, &ir.SourceCodeEndOffset, &ir.SourceCodeHash,
				&ir.Opcode, &ir.OpcodeName, &ir.OpcodeOperator,
				&ir.Name, &ir.VerboseName, &ir.ShortVerboseName, &ir.String, &ir.ReadableName, &ir.ReadableNameShort,
				&ir.CurrentBlock, &ir.CurrentFunction, &ir.IsFunction, &ir.FormalArgs, &ir.FreeValues, &ir.MemberCallArgs,
				&ir.SideEffects, &ir.IsVariadic, &ir.ReturnCodes, &ir.IsExternal, &ir.CodeBlocks,
				&ir.EnterBlock, &ir.ExitBlock, &ir.DeferBlock, &ir.ChildrenFunction, &ir.ParentFunction,
				&ir.IsBlock, &ir.PredBlock, &ir.SuccBlock, &ir.Phis, &ir.HasDefs, &ir.Users, &ir.Occulatation,
				&ir.IsObject, &ir.ObjectMembers, &ir.ObjectMemberPairs, &ir.IsObjectMember,
				&ir.ObjectParent, &ir.ObjectKey, &ir.ObjectOwnerPairs,
				&ir.MaskedCodes, &ir.IsMasked, &ir.Variable, &ir.ProgramCompileHash,
				&ir.TypeID, &ir.Point, &ir.Pointer, &ir.ExtraInformation, &ir.ConstType,
			); err != nil {
				rows.Close()
				return nil, err
			}
			ir.CreatedAt = createdAt
			ir.UpdatedAt = updatedAt
			if deletedAt.Valid {
				t := deletedAt.Time
				ir.DeletedAt = &t
			}
			out = append(out, ir)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// joinPlaceholders joins a slice of "?" placeholders with commas.
func joinPlaceholders(ps []string) string {
	if len(ps) == 0 {
		return ""
	}
	b := make([]byte, 0, len(ps)*2)
	for i, p := range ps {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, p...)
	}
	return string(b)
}

// nativeGetIrCodeIDsByConstType returns the code_ids matching the ConstType
// search predicate (opcode=5, const_type='normal', string exact or REGEXP),
// mirroring the GORM Model(&IrCode{}) path used by searchVariableWithFileFilter
// (including soft-delete deleted_at IS NULL). It is the A3 fast path: the GORM
// version built Model+Where+Pluck+YieldIrCode per call (3.59M calls on hadoop).
func nativeGetIrCodeIDsByConstType(db *gorm.DB, progName string, compareMode CompareMode, value string) ([]int64, error) {
	if db == nil || progName == "" {
		return nil, nil
	}
	nativeConstTypeIDQueries.Add(1)
	var q string
	var args []interface{}
	if compareMode == ExactCompare {
		q = `SELECT code_id FROM ` + TableIrCodes +
			` WHERE program_name = ? AND opcode = ? AND const_type = ? AND "string" = ? AND deleted_at IS NULL`
		args = []interface{}{progName, 5, "normal", value}
	} else {
		q = `SELECT code_id FROM ` + TableIrCodes +
			` WHERE program_name = ? AND opcode = ? AND const_type = ? AND "string" REGEXP ? AND deleted_at IS NULL`
		args = []interface{}{progName, 5, "normal", value}
	}
	rows, err := db.CommonDB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
