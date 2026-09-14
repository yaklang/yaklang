package syntaxflow_scan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	_ "github.com/yaklang/yaklang/common/yak/ssa_compile"
)

func TestScanProject_CompilesAndRunsSourceFromSnapshot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte("key = \"AKIAIOSFODNN7EXAMPLE\"\n"), 0o644))

	var alerts int
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("yak"),
		ssaconfig.WithSetProgramName(t.Name()),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "scan-project source")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
			Language: string(ssaconfig.General),
		}),
		syntaxflow_scan.WithScanResultCallback(func(r *syntaxflow_scan.ScanResult) {
			if r != nil && r.Result != nil {
				alerts += len(r.Result.GetAlertVariables())
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Greater(t, alerts, 0)
}
