package syntaxflow_scan_test

import (
	"bytes"
	"context"
	"encoding/json"
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

type countingProjectReport struct {
	sfreport.IReport
	saves int
}

func (r *countingProjectReport) Save() error {
	r.saves++
	return r.IReport.Save()
}

func TestScanProjectReportIncludesAllStages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "A.java"), []byte(`class A { void run(String cmd) { Runtime.getRuntime().exec(cmd); } }`), 0600))
	name := uuid.NewString()
	t.Cleanup(func() { ssadb.DeleteProgram(ssadb.GetDB(), name) })
	sarif, err := sfreport.NewSarifReport()
	require.NoError(t, err)
	reporter := &countingProjectReport{IReport: sarif}
	var out bytes.Buffer
	require.NoError(t, reporter.SetWriter(&out))
	_, err = syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("java"),
		ssaconfig.WithSetProgramName(name),
		syntaxflow_scan.WithMode(syntaxflow_scan.SourceMode, syntaxflow_scan.StructMode, syntaxflow_scan.SSAMode),
		syntaxflow_scan.WithReporter(reporter),
		ssaconfig.WithRuleInputRaw(`desc(mode: "source", language: general, title: "report-source")
${*}.pattern_regex(/Runtime/) as $hit
alert $hit`),
		ssaconfig.WithRuleInputRaw(`desc(mode: "struct", language: java, title: "report-struct")
Runtime.getRuntime().exec(* as $hit)
alert $hit`),
		ssaconfig.WithRuleInputRaw(`desc(mode: "ssa", language: java, title: "report-ssa")
Runtime.getRuntime().exec(* as $hit)
alert $hit`),
	)
	require.NoError(t, err)
	require.Positive(t, reporter.saves, "the project must write the finished report")
	var doc map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc), "stage snapshots must rewrite one complete JSON document")
	for _, title := range []string{"report-source", "report-struct", "report-ssa"} {
		require.Contains(t, out.String(), title)
	}
}
