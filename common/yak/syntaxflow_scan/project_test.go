package syntaxflow_scan_test

import (
	"context"
	"fmt"
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

func TestScanProject_EmitsProductStages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("eval(user)\n"), 0o644))

	var stages []string
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(t.Name()),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: python, title: "stage source")
${*.py}.pattern_regex(/eval\s*\(/) as $hit
alert $hit`,
			Language: "python",
		}),
		syntaxflow_scan.WithStageCallback(func(stage syntaxflow_scan.ProductStage, overall, progress float64, info *syntaxflow_scan.RuleProcessInfoList) {
			if progress == 0 || progress == 1 {
				stages = append(stages, string(stage))
			}
		}),
		ssaconfig.WithScanIgnoreLanguage(true),
	)
	require.NoError(t, err)
	require.Contains(t, stages, string(syntaxflow_scan.StageCollect))
	require.Contains(t, stages, string(syntaxflow_scan.StageInspect))
	require.Equal(t, "收集代码", syntaxflow_scan.StageCollect.DisplayName())
	require.Equal(t, "代码检测", syntaxflow_scan.StageInspect.DisplayName())
	require.Equal(t, "语义检测", syntaxflow_scan.StageReview.DisplayName())
	require.Equal(t, "深度分析", syntaxflow_scan.StageAnalyze.DisplayName())
}

func TestScanProject_ExternalStructRule(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.py"), []byte("def run(x):\n    return eval(x)\n"), 0o644))

	var alerts int
	err := syntaxflow_scan.ScanProject(context.Background(),
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(dir),
		ssaconfig.WithProjectRawLanguage("python"),
		ssaconfig.WithSetProgramName(t.Name()),
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(
	mode: "struct"
	language: "python"
	title: "external struct eval"
)
eval(* as $arg) as $call
alert $call`,
			Language: "python",
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

func TestScanProjectFromJSON_UsesConfigBlob(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.yak"), []byte("key = \"AKIAIOSFODNN7EXAMPLE\"\n"), 0o644))

	raw := fmt.Sprintf(`{
		"Mode": 127,
		"CodeSource": {"kind": "local", "local_file": %q},
		"BaseInfo": {"language": "yak", "program_names": [%q]}
	}`, dir, t.Name())

	var alerts int
	err := syntaxflow_scan.ScanProjectFromJSON(context.Background(), raw,
		ssaconfig.WithRuleInput(&ypb.SyntaxFlowRuleInput{
			Content: `desc(mode: "source", language: general, title: "json source")
${*}.pattern_regex(/AKIA[0-9A-Z]{16}/) as $hit
alert $hit`,
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
