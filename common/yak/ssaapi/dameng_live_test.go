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
	source := `class Run { void unsafe(String command) throws Exception { String banner = "` + strings.Repeat("安全", 2000) + `"; Runtime.getRuntime().exec(command); } }`
	vf.AddFile("src/Run.java", source)
	programs, err := ParseProjectWithFS(vf, WithLanguage(ssaconfig.JAVA), WithProgramName(name))
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
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
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
	diff, err := ParseProjectWithFS(next, WithLanguage(ssaconfig.JAVA), WithProgramName(diffName), WithBaseProgramName(name))
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
	err := ssadb.SaveIrCodeBatch(db, []*ssadb.IrCode{{ProgramName: name, CodeID: 1, Name: "replacement"}, {ProgramName: name, CodeID: 1, Name: "duplicate"}})
	require.Error(t, err, "the unique invariant must reject the second row")
	var row ssadb.IrCode
	require.NoError(t, db.Where("program_name=? AND code_id=?", name, 1).First(&row).Error)
	require.Equal(t, "original", row.Name, "failed upsert must roll back its delete and partial inserts")
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
