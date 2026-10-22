package ssaapi

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/damenggorm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func openLiveDMIR(t *testing.T) {
	openLiveIR(t, "dm")
}

func openLiveIR(t *testing.T, backend string) {
	t.Helper()
	file := os.Getenv("YAK_SSA_DM_URL_FILE")
	if backend == "postgres" {
		file = os.Getenv("YAK_SSA_PG_URL_FILE")
	}
	if file == "" {
		t.Skip("set a private isolated DM IR URL file")
	}
	require.Equal(t, "1", os.Getenv("YAK_SSA_DM_ALLOW_FIXTURE"))
	info, err := os.Lstat(file)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	u, err := url.Parse(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	require.Equal(t, backend, u.Scheme)
	if backend == "dm" {
		require.True(t, strings.HasPrefix(u.Query().Get("schema"), "LEGION_SSA_IR_"), "test writes require the dedicated fixture prefix")
	} else {
		require.True(t, strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "legion_ssa_ir_fixture_"))
	}
	db, err := consts.CreateSSAProjectDatabaseRaw(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	if backend == "dm" {
		require.True(t, damenggorm.IsDM(db), "SSA DM URLs must use the registered SSA dialect policy")
	} else {
		require.Equal(t, backend, db.Dialect().GetName())
	}
	ssadb.SetDB(db)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
}

const dmIRQuery = `Runtime.getRuntime().exec(* as $command) as $call
alert $call`

func TestDMIRCompileAndCrossProcessReload(t *testing.T) {
	openLiveDMIR(t)
	checkIRCompileAndCrossProcessReload(t, "dm")
}

func TestPostgresIRCompileAndCrossProcessReload(t *testing.T) {
	openLiveIR(t, "postgres")
	checkIRCompileAndCrossProcessReload(t, "postgres")
}

func checkIRCompileAndCrossProcessReload(t *testing.T, backend string) {
	name := "dm-live-" + uuid.NewString()
	vf := filesys.NewVirtualFs()
	source := `import other.Consumer; class Run { void unsafe(String command) throws Exception { String derived = new Consumer().value(command); String banner = "` + strings.Repeat("安全", 2000) + `"; Runtime.getRuntime().exec(command); } }`
	vf.AddFile("src/Run.java", source)
	vf.AddFile("src/helpers/Helper.java", `package helpers; public class Helper { public static String value(String input) { return input; } }`)
	vf.AddFile("src/other/Consumer.java", `package other; import helpers.Helper; public class Consumer { public String value(String input) { return Helper.value(input); } }`)
	programs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA), WithProgramName(name), ssaconfig.WithCompileConcurrency(2))
	require.NoError(t, err)
	require.Len(t, programs, 1)
	result, err := programs[0].SyntaxFlowWithError(dmIRQuery)
	require.NoError(t, err)
	require.Equal(t, 1, result.GetValues("call").Len())
	var codes int64
	require.NoError(t, ssadb.GetDB().Table(ssadb.TableIrCodes).Where("program_name = ?", name).Count(&codes).Error)
	require.Positive(t, codes)
	row, err := ssadb.GetProgram(name, ssadb.Application)
	require.NoError(t, err)
	require.NotEmpty(t, row.FileList, "JSON map must survive CLOB storage without hex conversion")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIRBackendReloadProcess$", "-test.v")
	child.Env = append(os.Environ(), "YAK_SSA_IR_RELOAD_PROGRAM="+name, "YAK_SSA_IR_RELOAD_BACKEND="+backend)
	output, err := child.CombinedOutput()
	require.NoError(t, err, "fresh-process reload: %s", output)
	require.Contains(t, string(output), "FRESH_IR_RELOAD_OK", "the child must execute, not skip")
	t.Logf("IR persisted backend=%s program=%s code_rows=%d fresh_process=%s", backend, name, codes, strings.TrimSpace(string(output)))

	// Exercise persisted base lookup and an incremental layer, then reload that
	// layer in another process rather than accepting the in-memory result.
	diffName := name + "-diff"
	next := filesys.NewVirtualFs()
	next.AddFile("src/Run.java", source+"\n// incremental change")
	next.AddFile("src/helpers/Helper.java", `package helpers; public class Helper { public static String value(String input) { return input; } }`)
	next.AddFile("src/other/Consumer.java", `package other; import helpers.Helper; public class Consumer { public String value(String input) { return Helper.value(input); } }`)
	diff, err := ParseProjectWithFS(next, WithLanguage(ssaconfig.JAVA), WithProgramName(diffName), WithBaseProgramName(name), ssaconfig.WithCompileConcurrency(2))
	require.NoError(t, err)
	require.Len(t, diff, 1)
	diffRow, err := ssadb.GetProgram(diffName, ssadb.Application)
	require.NoError(t, err)
	require.Equal(t, name, diffRow.BaseProgramName)
	child = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestIRBackendReloadProcess$", "-test.v")
	child.Env = append(os.Environ(), "YAK_SSA_IR_RELOAD_PROGRAM="+diffName, "YAK_SSA_IR_RELOAD_BACKEND="+backend)
	output, err = child.CombinedOutput()
	require.NoError(t, err, "fresh incremental reload: %s", output)
	require.Contains(t, string(output), "FRESH_IR_RELOAD_OK")
	require.NoError(t, ssadb.DeleteProgramChecked(ssadb.GetDB(), diffName))
	require.NoError(t, ssadb.DeleteProgramChecked(ssadb.GetDB(), name))
	var remaining int64
	require.NoError(t, ssadb.GetDB().Table(ssadb.TableIrCodes).Where("program_name = ?", name).Count(&remaining).Error)
	require.Zero(t, remaining)
}

func TestDMIRBatchFailurePreservesPreviousRows(t *testing.T) {
	openLiveDMIR(t)
	name := "dm-live-atomic-" + uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { require.NoError(t, ssadb.DeleteProgramChecked(db, name)) })
	require.NoError(t, ssadb.SaveIrCodeBatch(db, []*ssadb.IrCode{{ProgramName: name, CodeID: 1, Name: "original"}}))
	var before ssadb.IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", name, 1).First(&before).Error)
	require.NotZero(t, before.ID)
	changed := before
	changed.ID += 1000000 // Natural-key MERGE must ignore stale caller identities.
	changed.CreatedAt = before.CreatedAt.Add(-24 * time.Hour)
	changed.Name = "updated"
	require.NoError(t, ssadb.SaveIrCodeBatch(db, []*ssadb.IrCode{&changed}))
	var committed ssadb.IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", name, 1).First(&committed).Error)
	require.Equal(t, before.ID, committed.ID)
	require.True(t, before.CreatedAt.Equal(committed.CreatedAt), "MERGE must preserve stored creation time")
	require.Equal(t, "updated", committed.Name)
	constraint := addDMIRFailureCheck(t, db, ssadb.TableIrCodes, "opcode", name)
	items := make([]*ssadb.IrCode, 0, 502)
	for id := int64(1); id <= 501; id++ {
		items = append(items, &ssadb.IrCode{ProgramName: name, CodeID: id, Name: "replacement"})
	}
	items = append(items, &ssadb.IrCode{ProgramName: name, CodeID: 502, Name: "rejected", Opcode: -1})
	err := ssadb.SaveIrCodeBatch(db, items)
	require.Error(t, err)
	require.Contains(t, strings.ToUpper(err.Error()), strings.ToUpper(constraint), "the final distinct row must fail at the named CHECK")
	var row ssadb.IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", name, 1).First(&row).Error)
	require.Equal(t, "updated", row.Name, "failed MERGE must roll back updates and inserts across all chunks")
	require.Equal(t, committed.ID, row.ID)
	require.True(t, committed.CreatedAt.Equal(row.CreatedAt))
	var count int64
	require.NoError(t, db.Model(&ssadb.IrCode{}).Where("program_name=?", name).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// A stale identity carried by a re-flushed struct must not replace the stored
// identity or alter another natural key. Failed writes must leave the cache alone.
func TestDMIRTypeBatchReuseAndRollback(t *testing.T) {
	openLiveDMIR(t)
	name := "dm-live-types-" + uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { require.NoError(t, ssadb.DeleteProgramChecked(db, name)) })
	row := &ssadb.IrType{ProgramName: name, TypeId: 1, Kind: 1, String: "original"}
	other := &ssadb.IrType{ProgramName: name, TypeId: 2, Kind: 1, String: "untouched"}
	require.NoError(t, ssadb.SaveIrTypeBatch(db, []*ssadb.IrType{nil, row, other}))
	require.Zero(t, row.ID, "natural-key SaveInBatches does not write generated identities back to caller structs")
	var before, beforeOther ssadb.IrType
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 1).First(&before).Error)
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 2).First(&beforeOther).Error)
	require.NotZero(t, before.ID)
	row.ID = beforeOther.ID
	row.CreatedAt = before.CreatedAt.Add(-24 * time.Hour)
	row.Kind = 2
	row.String = "updated"
	require.NoError(t, ssadb.SaveIrTypeBatch(db, []*ssadb.IrType{row, nil}))
	require.Equal(t, beforeOther.ID, row.ID, "the caller struct remains caller-owned")
	var count int64
	require.NoError(t, db.Model(&ssadb.IrType{}).Where("program_name=?", name).Count(&count).Error)
	require.EqualValues(t, 2, count)
	var stored, untouched ssadb.IrType
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 1).First(&stored).Error)
	require.Equal(t, "updated", stored.String)
	require.Equal(t, before.ID, stored.ID)
	require.True(t, before.CreatedAt.Equal(stored.CreatedAt))
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 2).First(&untouched).Error)
	require.Equal(t, "untouched", untouched.String)
	require.Equal(t, beforeOther.ID, untouched.ID)
	constraint := addDMIRFailureCheck(t, db, ssadb.TableIrTypes, "kind", name)
	items := make([]*ssadb.IrType, 0, 1002)
	for id := uint64(1); id <= 1001; id++ {
		items = append(items, &ssadb.IrType{ProgramName: name, TypeId: id, Kind: 1, String: "replacement"})
	}
	items = append(items, &ssadb.IrType{ProgramName: name, TypeId: 1002, Kind: -1, String: "rejected"})
	err := ssadb.SaveIrTypeBatch(db, items)
	require.Error(t, err)
	require.Contains(t, strings.ToUpper(err.Error()), strings.ToUpper(constraint), "the final distinct row must fail at the named CHECK")
	stored = ssadb.IrType{}
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 1).First(&stored).Error)
	require.Equal(t, "updated", stored.String, "failed type batch must preserve the previous snapshot")
	require.Equal(t, before.ID, stored.ID)
	require.True(t, before.CreatedAt.Equal(stored.CreatedAt))
	require.NoError(t, db.Model(&ssadb.IrType{}).Where("program_name=?", name).Count(&count).Error)
	require.EqualValues(t, 2, count, "earlier successful insert chunks must be rolled back")
	cached, ok := ssadb.GetIrTypeCache(name).Get(1)
	require.True(t, ok)
	require.Equal(t, "updated", cached.String, "failed batch must not warm the type cache")
	_, ok = ssadb.GetIrTypeCache(name).Get(3)
	require.False(t, ok, "rolled-back new types must not enter the cache")
	// Main now propagates index errors to the persistence barrier. Check the DM
	// adapter returns an actual database error rather than silently swallowing it.
	badIndex := &ssadb.IrIndex{ProgramName: name, ValueID: 1}
	require.NoError(t, db.Create(badIndex).Error)
	require.Error(t, ssadb.SaveIrIndexBatch(db, []*ssadb.IrIndex{nil, badIndex}), "reusing a primary key must propagate the insert conflict")
}

// This fixture-local CHECK creates a real database error after preceding MERGE
// statements have succeeded; duplicate input keys can legitimately merge twice.
func addDMIRFailureCheck(t *testing.T, db *gorm.DB, table, column, program string) string {
	t.Helper()
	require.NotContains(t, program, "'") // The generated fixture name is the only SQL literal below.
	constraint := "ck_ssa_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	quote := db.Dialect().Quote
	statement := "ALTER TABLE " + quote(table) + " ADD CONSTRAINT " + quote(constraint) +
		" CHECK (" + quote("program_name") + " <> '" + program + "' OR " + quote(column) + " >= 0)"
	require.NoError(t, db.Exec(statement).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("ALTER TABLE "+quote(table)+" DROP CONSTRAINT "+quote(constraint)).Error)
	})
	return constraint
}

func TestDMIRBatchRespectsCallerTransaction(t *testing.T) {
	openLiveDMIR(t)
	name := "dm-live-transaction-" + uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { require.NoError(t, ssadb.DeleteProgramChecked(db, name)) })
	tx := db.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, ssadb.SaveIrCodeBatch(tx, []*ssadb.IrCode{{ProgramName: name, CodeID: 1, Name: "rolled back"}}))
	require.NoError(t, tx.Rollback().Error, "a successful batch must leave the caller transaction open")
	var count int64
	require.NoError(t, db.Model(&ssadb.IrCode{}).Where("program_name=?", name).Count(&count).Error)
	require.Zero(t, count)
	tx = db.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, ssadb.SaveIrCodeBatch(tx, []*ssadb.IrCode{{ProgramName: name, CodeID: 1, Name: "first"}}))
	require.NoError(t, ssadb.SaveIrCodeBatch(tx, []*ssadb.IrCode{{ProgramName: name, CodeID: 2, Name: "second"}}))
	require.NoError(t, tx.Commit().Error)
	require.NoError(t, db.Model(&ssadb.IrCode{}).Where("program_name=?", name).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestDMIRTypeIndexUpgradePreservesRows(t *testing.T) {
	openLiveDMIR(t)
	name := "dm-live-type-index-" + uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { require.NoError(t, ssadb.DeleteProgramChecked(db, name)) })
	indexName := "ux_ir_types_program_type"
	if !db.Dialect().HasIndex(ssadb.TableIrTypes, indexName) {
		indexName = "idx_ir_types_program_type"
	}
	require.NoError(t, db.Exec("DROP INDEX "+db.Dialect().Quote(indexName)).Error)
	require.NoError(t, db.Exec("CREATE INDEX idx_ir_types_program_type ON ir_types(program_name,type_id)").Error)
	for _, kind := range []int{1, 2} {
		require.NoError(t, db.Create(&ssadb.IrType{ProgramName: name, TypeId: 1, Kind: kind}).Error)
	}
	raw, err := os.ReadFile(os.Getenv("YAK_SSA_DM_URL_FILE"))
	require.NoError(t, err)
	invalid, err := consts.CreateSSAProjectDatabaseRaw(strings.TrimSpace(string(raw)))
	require.Error(t, err, "duplicate rows must block the unique index upgrade")
	require.Nil(t, invalid)
	var count int64
	require.NoError(t, db.Model(&ssadb.IrType{}).Where("program_name=?", name).Count(&count).Error)
	require.EqualValues(t, 2, count, "migration must not discard duplicate historical rows")
	require.NoError(t, db.Where("program_name=? AND kind=?", name, 2).Unscoped().Delete(&ssadb.IrType{}).Error)
	upgraded, err := consts.CreateSSAProjectDatabaseRaw(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	defer upgraded.Close()
	require.Error(t, upgraded.Create(&ssadb.IrType{ProgramName: name, TypeId: 1, Kind: 3}).Error, "the reused index must now enforce uniqueness")
}

func TestIRBackendReloadProcess(t *testing.T) {
	name := os.Getenv("YAK_SSA_IR_RELOAD_PROGRAM")
	if name == "" {
		t.Skip("executed only as the fresh-process helper")
	}
	openLiveIR(t, os.Getenv("YAK_SSA_IR_RELOAD_BACKEND"))
	program, err := FromDatabase(name)
	require.NoError(t, err)
	result, err := program.SyntaxFlowWithError(dmIRQuery)
	require.NoError(t, err)
	require.Equal(t, 1, result.GetValues("call").Len())
	editors, err := ssadb.GetEditorByProgramName(name)
	require.NoError(t, err)
	found := false
	for _, editor := range editors {
		if editor != nil && strings.Contains(editor.GetSourceCode(), strings.Repeat("安全", 2000)) {
			found = true
		}
	}
	require.True(t, found, "the fresh process must recover the full Unicode CLOB source")
	fmt.Printf("FRESH_IR_RELOAD_OK backend=%s program=%s\n", os.Getenv("YAK_SSA_IR_RELOAD_BACKEND"), name)
}
