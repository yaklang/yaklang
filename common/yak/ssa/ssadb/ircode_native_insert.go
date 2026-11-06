package ssadb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/gorm"
)

// irCodeInsertColumns is the physical column order of ir_codes without the
// auto-increment primary key. Keep it in sync with IrCode and the AutoMigrate
// schema; the native writer bypasses gorm's field reflection entirely.
var irCodeInsertColumns = []string{
	"created_at", "updated_at", "deleted_at",
	"code_id", "program_name", "version",
	"source_code_start_offset", "source_code_end_offset", "source_code_hash",
	"opcode", "opcode_name", "opcode_operator",
	"name", "verbose_name", "short_verbose_name", "string", "readable_name",
	"readable_name_short", "current_block", "current_function",
	"is_function", "formal_args", "free_values", "member_call_args",
	"side_effects", "is_variadic", "return_codes", "is_external",
	"code_blocks", "enter_block", "exit_block", "defer_block",
	"children_function", "parent_function",
	"is_block", "pred_block", "succ_block", "phis",
	"has_defs", "users", "occulatation",
	"is_object", "object_members", "object_member_pairs", "is_object_member",
	"object_parent", "object_key", "object_owner_pairs",
	"masked_codes", "is_masked", "variable",
	"program_compile_hash", "type_id", "point", "pointer",
	"extra_information", "const_type",
}

// nativeInsertIrCodes inserts new IR rows with one multi-row INSERT per chunk.
// GORM's batch path builds a scope and resolves every field by reflection for
// every row (Scope.Fields / AddToVars / prepareBatchCreateRow), which is a
// large allocation source on Python/Java projects with millions of IR rows.
//
// Only SQLite and PostgreSQL are routed here. Other dialects keep the GORM
// path so their quoting and upsert behavior stay untouched.
func nativeInsertIrCodes(db *gorm.DB, items []*IrCode, batchSize int) (bool, error) {
	name := ""
	if db != nil {
		name = strings.ToLower(db.Dialect().GetName())
	}
	if name != "sqlite3" && name != "postgres" && name != "postgresql" && name != "cloudsqlpostgres" {
		return false, nil
	}
	if len(items) == 0 {
		return true, nil
	}
	if batchSize < 1 {
		batchSize = len(items)
	}
	if batchSize > 500 {
		batchSize = 500
	}

	columnList := `"` + strings.Join(irCodeInsertColumns, `", "`) + `"`
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?,", len(irCodeInsertColumns)), ",") + ")"
	now := time.Now()
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]
		rowCount := 0
		for _, item := range chunk {
			if item != nil {
				rowCount++
			}
		}
		if rowCount == 0 {
			continue
		}
		args := make([]interface{}, 0, rowCount*len(irCodeInsertColumns))
		for _, item := range chunk {
			if item == nil {
				continue
			}
			if item.CreatedAt.IsZero() {
				item.CreatedAt = now
			}
			if item.UpdatedAt.IsZero() {
				item.UpdatedAt = now
			}
			// GORM runs BeforeSave on every element of CreateInBatches. The
			// native writer keeps that sanitization on every dialect so the
			// stored bytes match the GORM path, including SQLite tests.
			sanitizeStructStringsForPG(item)
			args = appendIrCodeInsertArgs(args, item)
		}
		values := strings.TrimSuffix(strings.Repeat(rowPlaceholder+",", rowCount), ",")
		query := bindSQLPlaceholdersDB(db,
			`INSERT INTO "`+TableIrCodes+`" (`+columnList+`) VALUES `+values)
		ctx, cancel := nativeQueryContext(context.Background())
		started := time.Now()
		_, err := nativeExecContextDB(db, ctx, query, args...)
		cancel()
		RecordDBOp(DBOpCreate, time.Since(started), err != nil)
		if err != nil {
			return true, fmt.Errorf("insert IR rows: %w", err)
		}
	}
	for program, rows := range irCodeRowsByProgram(items) {
		irCodeWriteCounterFor(program).insertRows.Add(int64(rows))
	}
	return true, nil
}

func appendIrCodeInsertArgs(args []interface{}, item *IrCode) []interface{} {
	args = append(args,
		item.CreatedAt, item.UpdatedAt, item.DeletedAt,
		item.CodeID, item.ProgramName, item.Version,
		item.SourceCodeStartOffset, item.SourceCodeEndOffset, item.SourceCodeHash,
		item.Opcode, item.OpcodeName, item.OpcodeOperator,
		item.Name, item.VerboseName, item.ShortVerboseName, item.String, item.ReadableName,
		item.ReadableNameShort, item.CurrentBlock, item.CurrentFunction,
		item.IsFunction, item.FormalArgs, item.FreeValues, item.MemberCallArgs,
		item.SideEffects, item.IsVariadic, item.ReturnCodes, item.IsExternal,
		item.CodeBlocks, item.EnterBlock, item.ExitBlock, item.DeferBlock,
		item.ChildrenFunction, item.ParentFunction,
		item.IsBlock, item.PredBlock, item.SuccBlock, item.Phis,
		item.HasDefs, item.Users, item.Occulatation,
		item.IsObject, item.ObjectMembers, item.ObjectMemberPairs, item.IsObjectMember,
		item.ObjectParent, item.ObjectKey, item.ObjectOwnerPairs,
		item.MaskedCodes, item.IsMasked, item.Variable,
		item.ProgramCompileHash, item.TypeID, item.Point, item.Pointer,
		item.ExtraInformation, item.ConstType,
	)
	return args
}
