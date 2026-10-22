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
	"github.com/yaklang/yaklang/common/consts"
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
	require.Equal(t, backend, db.Dialect().GetName())
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
	items := make([]*ssadb.IrCode, 0, 502)
	for id := int64(1); id <= 501; id++ {
		items = append(items, &ssadb.IrCode{ProgramName: name, CodeID: id, Name: "replacement"})
	}
	items = append(items, &ssadb.IrCode{ProgramName: name, CodeID: 501, Name: "duplicate"})
	err := ssadb.SaveIrCodeBatch(db, items)
	require.Error(t, err, "the unique invariant must reject the second row")
	var row ssadb.IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", name, 1).First(&row).Error)
	require.Equal(t, "original", row.Name, "failed upsert must roll back its delete and inserts across all chunks")
}

// A second flush carries the identity assigned by the previous write. DM must
// generate a fresh identity for its replacement without attempting IDENTITY_INSERT.
func TestDMIRTypeBatchReuseAndRollback(t *testing.T) {
	openLiveDMIR(t)
	name := "dm-live-types-" + uuid.NewString()
	db := ssadb.GetDB()
	t.Cleanup(func() { require.NoError(t, ssadb.DeleteProgramChecked(db, name)) })
	row := &ssadb.IrType{ProgramName: name, TypeId: 1, Kind: 1, String: "original"}
	require.NoError(t, ssadb.SaveIrTypeBatch(db, []*ssadb.IrType{nil, row}))
	require.NotZero(t, row.ID)
	row.Kind = 2
	row.String = "updated"
	require.NoError(t, ssadb.SaveIrTypeBatch(db, []*ssadb.IrType{row, nil}))
	var count int64
	require.NoError(t, db.Model(&ssadb.IrType{}).Where("program_name=?", name).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var stored ssadb.IrType
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 1).First(&stored).Error)
	require.Equal(t, "updated", stored.String)
	err := ssadb.SaveIrTypeBatch(db, []*ssadb.IrType{
		{ProgramName: name, TypeId: 1, String: "replacement"},
		{ProgramName: name, TypeId: 1, String: "duplicate"},
	})
	require.Error(t, err)
	stored = ssadb.IrType{}
	require.NoError(t, db.Where("program_name=? AND type_id=?", name, 1).First(&stored).Error)
	require.Equal(t, "updated", stored.String, "failed type batch must preserve the previous snapshot")
	cached, ok := ssadb.GetIrTypeCache(name).Get(1)
	require.True(t, ok)
	require.Equal(t, "updated", cached.String, "failed batch must not warm the type cache")
	// Main now propagates index errors to the persistence barrier. Check the DM
	// adapter returns an actual database error rather than silently swallowing it.
	badIndex := &ssadb.IrIndex{ProgramName: name, ValueID: 1}
	require.NoError(t, db.Create(badIndex).Error)
	require.Error(t, ssadb.SaveIrIndexBatch(db, []*ssadb.IrIndex{nil, badIndex}), "reusing a primary key must propagate the insert conflict")
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
