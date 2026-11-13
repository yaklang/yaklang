package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

// The CLI's default project scan streams in-memory results. Exporting a data
// flow must use that live result, not look up its numeric memory id in SQLite.
func TestScanProjectMemoryReportDataflow(t *testing.T) {
	for _, withPaths := range []bool{false, true} {
		name := "without-paths"
		if withPaths {
			name = "with-paths"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.js"), []byte(`
function run(req) {
    const query = "select " + req.query.name;
    sink(query);
}
`), 0600))
			program := uuid.NewString()
			t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), program) })
			report := sfreport.NewReport(sfreport.IRifyReportType, sfreport.WithDataflowPath(withPaths))
			_, err := syntaxflow_scan.ScanProject(context.Background(),
				ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
				ssaconfig.WithCodeSourceLocalFile(dir),
				ssaconfig.WithProjectRawLanguage("js"),
				ssaconfig.WithSetProgramName(program),
				ssaconfig.WithNoSaveRisk(true),
				ssaconfig.WithRuleInputRaw(`desc(mode: "ssa", language: js, title: "memory path regression")
sink(* as $arg)
$arg #-> * as $source
alert $source`),
				syntaxflow_scan.WithMode(syntaxflow_scan.SSAMode),
				syntaxflow_scan.WithReporter(report),
			)
			require.NoError(t, err)
			require.NotEmpty(t, report.Risks)
			for _, risk := range report.Risks {
				if withPaths {
					require.NotEmpty(t, risk.DataFlowPaths, risk.Title)
					for _, flow := range risk.DataFlowPaths {
						require.NotEmpty(t, flow.Nodes)
						require.NotEmpty(t, flow.Edges)
					}
				} else {
					require.Empty(t, risk.DataFlowPaths)
				}
			}
			require.Zero(t, countAuditRowsForProgram(t, program))
			require.Zero(t, countSSARisksForProgram(t, program))
		})
	}
}
