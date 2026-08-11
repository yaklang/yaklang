package ssadb

import (
	"database/sql"
	"time"

	"github.com/yaklang/gorm"
)

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
