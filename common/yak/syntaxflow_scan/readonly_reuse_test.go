package syntaxflow_scan_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// A fresh scan process has only persisted IR and a read-only company connection.
// Count rejected SQL writes too: scan task save errors can otherwise be logged
// and swallowed while the scan appears successful.
func TestResilienceScanProjectReadOnlyNamedProgram(t *testing.T) {
	oldDB := ssadb.GetDB()
	dbPath := filepath.Join(t.TempDir(), "company-ir.db")
	writer, err := gorm.Open("sqlite3", dbPath)
	require.NoError(t, err)
	require.NoError(t, writer.AutoMigrate(ssadb.SSAProjectTables...).Error)
	ssadb.SetDB(writer)
	t.Cleanup(func() { ssadb.SetDB(oldDB) })

	programName := t.Name()
	vf := filesys.NewVirtualFs()
	vf.AddFile("app.py", "def run(x):\n    return eval(x)\n")
	progs, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.PYTHON), ssaapi.WithProgramName(programName))
	require.NoError(t, err)
	require.NotEmpty(t, progs)
	require.NoError(t, writer.Close())

	var writes, compiles atomic.Int64
	driverName := "readonly-reuse-" + uuid.NewString()
	sql.Register(driverName, &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
		conn.RegisterAuthorizer(func(op int, _, _, _ string) int {
			switch op {
			case sqlite3.SQLITE_INSERT, sqlite3.SQLITE_UPDATE, sqlite3.SQLITE_DELETE,
				sqlite3.SQLITE_CREATE_TABLE, sqlite3.SQLITE_DROP_TABLE, sqlite3.SQLITE_ALTER_TABLE:
				writes.Add(1)
				return sqlite3.SQLITE_DENY
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}})
	readSQL, err := sql.Open(driverName, "file:"+dbPath+"?mode=ro")
	require.NoError(t, err)
	reader, err := gorm.Open("sqlite3", readSQL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	ssadb.SetDB(reader)
	// Prove this fixture rejects writes before observing the real scan.
	require.Error(t, reader.Exec("DELETE FROM ir_programs").Error)
	require.Positive(t, writes.Swap(0))

	originalCompile := syntaxflow_scan.CompileProject
	t.Cleanup(func() { syntaxflow_scan.CompileProject = originalCompile })
	syntaxflow_scan.CompileProject = func(context.Context, *ssaconfig.Config, ...ssaconfig.Option) (*ssaapi.Program, error) {
		compiles.Add(1)
		return nil, fmt.Errorf("named IR reuse must not compile")
	}
	for _, mode := range []string{"struct", "ssa"} {
		t.Run(mode, func(t *testing.T) {
			// Evict the producer's cached program: this must reload the saved IR,
			// including its source snapshot, through the reader connection.
			ssaapi.ProgramCache.Remove(programName)
			ssadb.GetIrCodeCache(programName).Purge()
			ssadb.GetIrTypeCache(programName).Purge()
			var mu sync.Mutex
			var parts []*sfreport.SSAResultParts
			var conversionErrors []error
			var riskCount int
			result, scanErr := syntaxflow_scan.ScanProject(context.Background(),
				ssaconfig.WithProgramNames(programName),
				ssaconfig.WithJsonRawConfig([]byte(`{"SyntaxFlow":{"no_save_risk":true},"SyntaxFlowScan":{"no_save_task":true}}`)),
				syntaxflow_scan.WithMode(syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
				ssaconfig.WithScanIgnoreLanguage(true),
				ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
					Content: fmt.Sprintf(`desc(mode: "%s", language: "python", title: "read-only eval")
eval(* as $arg) as $call
alert $call`, mode), Language: "python",
				}),
				syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
					if r == nil || r.Result == nil {
						return
					}
					part, conversionErr := sfreport.ConvertSingleResultToSSAResultParts(r.Result, sfreport.NewStreamPartsOptions())
					mu.Lock()
					defer mu.Unlock()
					riskCount += r.Result.RiskCount()
					if conversionErr != nil {
						conversionErrors = append(conversionErrors, conversionErr)
					}
					if part != nil {
						parts = append(parts, part)
					}
				}),
			)
			require.NoError(t, scanErr)
			require.True(t, result.Succeeded)
			require.Zero(t, compiles.Load())
			require.Zero(t, writes.Swap(0), "no task, risk, audit or IR write may even be attempted")
			require.Empty(t, conversionErrors)
			require.Positive(t, riskCount)
			var risks, files, flows int
			for _, part := range parts {
				risks += len(part.Risks)
				files += len(part.Files)
				flows += len(part.Dataflows)
			}
			require.Equal(t, riskCount, risks, "memory alerts must survive conversion to node stream events")
			require.Positive(t, files)
			require.Positive(t, flows)
		})
	}
	ssaapi.ProgramCache.Remove(programName)
}

func TestScanNoSaveTaskRejectsPersistedLifecycle(t *testing.T) {
	for _, mode := range []ssaconfig.ControlMode{ssaconfig.ControlModeResume, ssaconfig.ControlModeStatus} {
		t.Run(string(mode), func(t *testing.T) {
			err := syntaxflow_scan.Scan(context.Background(),
				ssaconfig.WithNoSaveTask(true), ssaconfig.WithScanControlMode(mode))
			require.ErrorContains(t, err, "no_save_task does not support persisted task")
		})
	}
}
